// Package sparqlproxy forwards raw SPARQL read queries from the browser to a
// configured endpoint, same-origin.
//
// It exists because Graph Explorer runs entirely in the browser and POSTs its
// queries directly to an endpoint URL. That cannot work for the local QLever
// instance, which lives on a private Docker network (http://qlever:7001,
// exposed but never published) that no browser can resolve. Routing every
// browser-side query through this package means the page talks to Visoto and
// Visoto talks to the endpoint, so neither the upstream host nor its
// credentials ever reach the client.
//
// It is deliberately NOT part of internal/sparql. That package preprocesses
// queries (prefix expansion, magic properties, paging); this one passes bytes
// through verbatim and must never be tempted to rewrite what Graph Explorer
// sent.
package sparqlproxy

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
)

const (
	// Path is the route this proxy is served at. Exported so the handler that
	// builds the browser-facing URL cannot drift from the registration.
	Path = "/api/sparql"

	// queryContentType is the only request body form accepted. Graph Explorer
	// sends exactly this (see static/js/sparql-graph.js); the form-encoded
	// variant is deliberately not accepted, as no caller uses it and it would
	// double the parsing surface in front of the read-only check.
	queryContentType = "application/sparql-query"

	// maxQueryBytes bounds the request body. Real graph queries reach ~9 KB
	// (see the 431-cliff comment in static/js/sparql-graph.js), so this is
	// generous headroom while keeping memory bounded.
	maxQueryBytes = 64 << 10

	// maxResponseBytes bounds what is streamed back, so a pathological
	// DESCRIBE cannot relay unbounded bytes. Truncation yields a JSON parse
	// error in the client, which is an acceptable failure for such a query.
	maxResponseBytes = 32 << 20
)

// acceptable result media types. A client Accept header is forwarded only when
// it matches one of these; anything else falls back to SPARQL JSON. Blind
// forwarding would be low-risk, but an allowlist costs little and keeps
// upstream content negotiation predictable.
var acceptableTypes = map[string]bool{
	"application/sparql-results+json": true,
	"application/sparql-results+xml":  true,
	"application/json":                true,
	"application/ld+json":             true,
	"application/n-triples":           true,
	"application/rdf+xml":             true,
	"text/turtle":                     true,
}

// Proxy forwards browser-originated SPARQL read queries to configured endpoints.
type Proxy struct {
	cfg    *config.ApplicationConfig
	client *http.Client
}

// New builds a Proxy. timeout bounds each upstream query.
func New(cfg *config.ApplicationConfig, timeout time.Duration) *Proxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// A single graph render bursts several queries at one host.
	transport.MaxIdleConnsPerHost = 8

	return &Proxy{
		cfg: cfg,
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			// A SPARQL endpoint that redirects is not worth chasing to a new
			// host: that would sidestep the config allowlist below.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Handler returns the gin handler for POST /api/sparql?endpoint=<slug>.
//
// Note on SSRF: /api/upload guards against private and loopback hosts because
// it fetches a URL the *user* supplies — an open fetch primitive. This route
// takes no URL at all. Its destination comes from resolveEndpoint below, which
// only ever returns an entry from visoto.config, a set an operator controls and
// which has a handful of members. Reaching a private host is the entire point
// here, so validateRemoteURL must NOT be applied and this must not be coupled
// to allow_private_upload_urls — doing so would make an upload setting silently
// break the graph.
func (p *Proxy) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()

		// Never cached: the response depends on the request body, so it is not
		// a function of the URL.
		c.Header("Cache-Control", "no-store")

		ep, err := p.resolveEndpoint(c.Query("endpoint"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if !hasContentType(c.GetHeader("Content-Type"), queryContentType) {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{
				"error": "Content-Type must be " + queryContentType,
			})
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxQueryBytes)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "query too large"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not read request body"})
			return
		}

		// The security boundary. Everything below this point may carry the
		// endpoint's credentials, so this check and ApplyAuth must never be
		// reordered: the token enables SPARQL UPDATE on QLever, and the query
		// text is fully attacker-controlled.
		if !IsReadOnlyQuery(string(body)) {
			kw, _ := leadingKeyword(string(body))
			log.Warn("rejected non-read SPARQL query from browser",
				slog.String("endpoint", ep.Slug),
				slog.String("keyword", kw))
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "only SELECT, ASK, CONSTRUCT and DESCRIBE queries are allowed",
			})
			return
		}

		status, err := p.forward(c, ep, body)
		if err != nil {
			// The upstream URL is exactly what this proxy exists to hide, so it
			// is logged but never returned.
			log.Error("SPARQL proxy upstream failed",
				slog.String("endpoint", ep.Slug),
				slog.String("url", ep.URL),
				slog.Any("error", err))
			c.JSON(status, gin.H{"error": "endpoint " + ep.Slug + " could not be reached"})
			return
		}
	}
}

// forward sends body upstream and streams the response back. On failure it
// returns the status to report to the client; on success it has already written
// the response and returns a nil error.
func (p *Proxy) forward(c *gin.Context, ep *config.SparqlEndpoint, body []byte) (int, error) {
	// The gin request's context means a client that navigates away — panning a
	// graph cancels in-flight queries — cancels the upstream query too.
	req, err := http.NewRequestWithContext(
		c.Request.Context(), http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// Header boundary: only these are sent. No cookies, no client
	// Authorization, no Referer, nothing else the browser attached.
	req.Header.Set("Content-Type", queryContentType)
	req.Header.Set("Accept", negotiateAccept(c.GetHeader("Accept")))
	ep.ApplyAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		if ctxErr := c.Request.Context().Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			// Client went away mid-query; the connection is gone, so there is
			// nothing to write back.
			c.Abort()
			return http.StatusGatewayTimeout, nil
		}
		if isTimeout(err) {
			return http.StatusGatewayTimeout, err
		}
		return http.StatusBadGateway, err
	}
	defer resp.Body.Close()

	// Pass the upstream content type through: the client parses SPARQL JSON.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Header("Content-Type", ct)
	}
	// The body is third-party content served from Visoto's own origin, which is
	// the one genuinely new risk this route introduces. nosniff stops a hostile
	// upstream getting its response treated as HTML here.
	c.Header("X-Content-Type-Options", "nosniff")

	// Upstream 4xx/5xx pass through with their body on purpose: a SPARQL syntax
	// error from the endpoint is what someone debugging a graph query needs,
	// and it is what the browser saw before this proxy existed.
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, io.LimitReader(resp.Body, maxResponseBytes))
	return resp.StatusCode, nil
}

// resolveEndpoint maps a slug to a configured endpoint.
//
// It reads the slug from the URL and nothing else — never the endpoint cookie.
// Resolving here rather than via the shared middleware keeps that provable
// locally instead of by convention.
//
// An unknown slug is an error rather than a fallback: a graph pointed at a slug
// that no longer exists should say so, not silently query a different dataset.
func (p *Proxy) resolveEndpoint(slug string) (*config.SparqlEndpoint, error) {
	if slug != "" {
		ep := p.cfg.GetEndpointBySlug(slug)
		if ep == nil {
			return nil, errors.New("unknown endpoint: " + slug)
		}
		return ep, nil
	}

	if ep := p.cfg.DefaultEndpoint(); ep != nil {
		return ep, nil
	}

	// Bare config: no sparqlEndpoints list at all, just the scalar setting.
	if p.cfg.SparqlEndpoint != "" {
		return &config.SparqlEndpoint{Name: "default", URL: p.cfg.SparqlEndpoint}, nil
	}
	return nil, errors.New("no SPARQL endpoint configured")
}

// hasContentType reports whether the header names want, ignoring parameters so
// "application/sparql-query; charset=utf-8" is accepted.
func hasContentType(header, want string) bool {
	mt, _, err := mime.ParseMediaType(header)
	if err != nil {
		return false
	}
	return strings.EqualFold(mt, want)
}

// negotiateAccept returns the Accept header to send upstream: the client's own
// when it names a known SPARQL result type, otherwise SPARQL JSON.
//
// "*/*" is deliberately NOT treated as a preference. It means "no opinion", and
// forwarding it lets the endpoint pick its own default — LINDAS answers CSV,
// which Graph Explorer cannot parse. Browsers and fetch() send */* constantly,
// so this case is the common one, not the edge case.
func negotiateAccept(clientAccept string) string {
	const fallback = "application/sparql-results+json"
	if clientAccept == "" {
		return fallback
	}
	// Take the first acceptable type the client lists, dropping q-values.
	for _, part := range strings.Split(clientAccept, ",") {
		mt, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		if acceptableTypes[strings.ToLower(mt)] {
			return clientAccept
		}
	}
	return fallback
}

func isTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}
