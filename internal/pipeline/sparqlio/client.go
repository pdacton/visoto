// Package sparqlio is the pipeline's SPARQL protocol client: typed SELECT
// results in, UPDATE requests out.
//
// It exists alongside internal/sparql rather than inside it because the two have
// opposite needs. internal/sparql serves the web UI and flattens bindings to
// display strings; the pipeline must keep the term type, datatype and language
// tag intact, since those are exactly what it writes back into the triplestore.
package sparqlio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hutzli.org/visoto/internal/pipeline/rdf"
)

// Client talks SPARQL 1.1 Protocol to one endpoint.
type Client struct {
	endpoint    string
	http        *http.Client
	userAgent   string
	accessToken string
	username    string
	password    string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies the underlying client, so callers control timeouts,
// proxies and transport reuse.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithUserAgent sets the User-Agent. The pipeline identifies itself to every
// portal it touches (R-FET-7).
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithBearer authenticates with a bearer token, as the write endpoints need.
func WithBearer(token string) Option {
	return func(c *Client) { c.accessToken = token }
}

// WithBasicAuth authenticates with username and password.
func WithBasicAuth(user, pass string) Option {
	return func(c *Client) { c.username, c.password = user, pass }
}

// NewClient returns a client for endpoint.
func NewClient(endpoint string, opts ...Option) *Client {
	c := &Client{
		endpoint:  endpoint,
		http:      &http.Client{Timeout: 120 * time.Second},
		userAgent: "visoto-harvest",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Endpoint returns the URL this client talks to, for logging.
func (c *Client) Endpoint() string { return c.endpoint }

// Results is a parsed SPARQL SELECT response with term types preserved.
type Results struct {
	Vars     []string
	Bindings []Binding
}

// Binding is one solution. A variable unbound in this solution is simply absent.
type Binding map[string]rdf.Term

// Term returns the bound term and whether the variable was bound.
func (b Binding) Term(v string) (rdf.Term, bool) {
	t, ok := b[v]
	return t, ok
}

// Str returns the lexical value of a variable, or "" when unbound. Most call
// sites want the string and can treat unbound and empty alike.
func (b Binding) Str(v string) string { return b[v].Value }

// IRI returns the variable's value only when it is bound to an IRI. Source data
// puts literals where IRIs belong often enough that this needs to be explicit.
func (b Binding) IRI(v string) string {
	t, ok := b[v]
	if !ok || t.Kind != rdf.KindIRI {
		return ""
	}
	return t.Value
}

// Select runs a SELECT query and returns typed bindings.
func (c *Client) Select(ctx context.Context, query string) (*Results, error) {
	body, err := c.post(ctx, "application/sparql-query", query, "application/sparql-results+json")
	if err != nil {
		return nil, err
	}
	return parseResults(body)
}

// Ask runs an ASK query.
func (c *Client) Ask(ctx context.Context, query string) (bool, error) {
	body, err := c.post(ctx, "application/sparql-query", query, "application/sparql-results+json")
	if err != nil {
		return false, err
	}
	var payload struct {
		Boolean bool `json:"boolean"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, fmt.Errorf("parse ASK response: %w", err)
	}
	return payload.Boolean, nil
}

// Update runs a SPARQL UPDATE request.
func (c *Client) Update(ctx context.Context, update string) error {
	_, err := c.post(ctx, "application/sparql-update", update, "*/*")
	return err
}

// post sends one request and returns the body, mapping a non-2xx status to an
// error that carries enough of the response to be diagnosable from a log.
func (c *Client) post(ctx context.Context, contentType, payload, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)
	switch {
	case c.accessToken != "":
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	case c.username != "":
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sparql request to %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", c.endpoint, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &StatusError{Endpoint: c.endpoint, Code: resp.StatusCode, Body: excerpt(body)}
	}
	return body, nil
}

// maxResponseBytes caps a single SPARQL response. Paged queries stay far below
// it; an endpoint that ignores LIMIT should fail rather than exhaust memory.
const maxResponseBytes = 256 << 20

// StatusError is a non-2xx SPARQL response. It is a distinct type so callers can
// react to 429 and 5xx (R-FET-5) without string matching.
type StatusError struct {
	Endpoint string
	Code     int
	Body     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sparql endpoint %s returned HTTP %d: %s", e.Endpoint, e.Code, e.Body)
}

// Retryable reports whether retrying the same request could plausibly succeed.
func (e *StatusError) Retryable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= 500
}

func excerpt(b []byte) string {
	const max = 300
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// sparqlJSON mirrors the SPARQL 1.1 Query Results JSON Format.
type sparqlJSON struct {
	Head struct {
		Vars []string `json:"vars"`
	} `json:"head"`
	Results struct {
		Bindings []map[string]jsonTerm `json:"bindings"`
	} `json:"results"`
}

type jsonTerm struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Datatype string `json:"datatype"`
	Lang     string `json:"xml:lang"`
}

func parseResults(body []byte) (*Results, error) {
	var payload sparqlJSON
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse SELECT response: %w", err)
	}
	out := &Results{
		Vars:     payload.Head.Vars,
		Bindings: make([]Binding, 0, len(payload.Results.Bindings)),
	}
	for _, row := range payload.Results.Bindings {
		b := make(Binding, len(row))
		for v, t := range row {
			term, ok := t.toTerm()
			if !ok {
				continue // quoted triples and unknown types are not representable
			}
			b[v] = term
		}
		out.Bindings = append(out.Bindings, b)
	}
	return out, nil
}

func (t jsonTerm) toTerm() (rdf.Term, bool) {
	switch t.Type {
	case "uri":
		return rdf.IRI(t.Value), true
	case "bnode":
		return rdf.Blank(t.Value), true
	case "literal", "typed-literal":
		switch {
		case t.Lang != "":
			return rdf.LangLiteral(t.Value, t.Lang), true
		case t.Datatype != "":
			return rdf.TypedLiteral(t.Value, t.Datatype), true
		default:
			return rdf.Literal(t.Value), true
		}
	default:
		return rdf.Term{}, false
	}
}

// Escape renders s as a SPARQL string literal body, so values interpolated into
// generated queries cannot terminate the literal or inject a clause.
func Escape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return r.Replace(s)
}

// EscapeIRI renders an IRI for interpolation into a generated query. An IRI
// carrying characters that would close the angle brackets is rejected rather
// than escaped, because a source IRI that malformed is not worth querying for.
func EscapeIRI(iri string) (string, bool) {
	if iri == "" || strings.ContainsAny(iri, "<>\"{}|^`\\ \n\r\t") {
		return "", false
	}
	if _, err := url.Parse(iri); err != nil {
		return "", false
	}
	return iri, true
}
