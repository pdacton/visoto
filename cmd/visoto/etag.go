package main

// Response caching: ETag, Vary and Cache-Control for every rendered route.
//
// There are two tiers, because the two kinds of route learn their language
// differently.
//
// Page tier (/, /:page, /search, /resource) — the language comes from a cookie,
// not the URL, which means a browser that cached a page cannot tell that a
// language switch invalidated it. Vary: X-Site-Lang separates the variants in the
// *shared* cache (Caddy/Souin, whose key includes that header), but a browser
// never sends X-Site-Lang, so to a private cache every variant looks identical.
// Left alone, switching to French and reloading would re-serve the cached English
// page. The fix is to make browsers always revalidate and to make revalidation
// cheap: every response carries an ETag, and Cache-Control tells private caches to
// revalidate while still letting Souin serve for hours. In production those
// revalidations are answered by Souin from its own store, so the origin does not
// re-run the SPARQL query.
//
// API tier (the /api/* fragment routes, marked by markURLPure) — both identities
// are on the URL: the endpoint as ?endpoint= and the language as ?lang= (see
// resolveQueryLang). That makes the response a complete function of its URL, so
// there is nothing left to negotiate: no ETag, no Vary, and a long max-age that
// browsers and Souin can both serve from directly. A language switch changes the
// URL, so a stale entry becomes unreachable rather than merely revalidated away.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/lang"
)

// cacheableKey marks a response as safe for the shared cache to hold. Handlers
// set it via markCacheable; it replaces the old cacheControlPublic header
// constant, since the Cache-Control value is now assembled in one place.
const cacheableKey = "cacheable"

// sharedMaxAge is how long Souin may serve a cacheable response without asking
// the origin, in seconds (6h — unchanged from the previous max-age).
const sharedMaxAge = "21600"

// markCacheable records that this response may be stored by the shared cache.
// Only for responses that are a pure function of the URL plus the negotiated
// language: no cookie-dependent content, no Set-Cookie, no transient errors.
func markCacheable(c *gin.Context) {
	c.Set(cacheableKey, true)
}

// isCacheable reports whether a handler called markCacheable.
func isCacheable(c *gin.Context) bool {
	v, ok := c.Get(cacheableKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// urlPureKey marks a route whose response is a complete function of its URL:
// both the endpoint and the language arrive as query params, and no cookie or
// content-negotiation header is read. Set by resolveQueryLang, so the route
// table is the single place that decides which routes qualify — a route added
// without that middleware fails safe into the page tier.
const urlPureKey = "urlPure"

// markURLPure records that this response depends on nothing but its URL, so it
// needs no ETag and no Vary — see the API tier note in this file's header.
func markURLPure(c *gin.Context) {
	c.Set(urlPureKey, true)
}

// isURLPure reports whether the route declared itself URL-pure.
func isURLPure(c *gin.Context) bool {
	v, ok := c.Get(urlPureKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// apiMaxAge is how long a URL-pure /api fragment may be served without asking the
// origin, in seconds (6h, matching the page tier's shared TTL).
const apiMaxAge = "21600"

// apiCacheControl builds the Cache-Control for a URL-pure /api response.
//
// Unlike the page tier this max-age applies to the *browser* too, which is the
// point: with the language on the URL there is nothing a revalidation could
// discover that a plain cache hit would get wrong, so the conditional round-trip
// every fragment costs today disappears.
//
// Deliberately no "immutable": the underlying SPARQL data does change, and
// immutable would defeat the force-reload escape hatch the table JS relies on
// (fetchOptions sets cache: "reload" on a reload navigation).
func apiCacheControl(cacheable bool) string {
	if cacheable {
		return "public, max-age=" + apiMaxAge + ", s-maxage=" + apiMaxAge
	}
	// On this tier "not cacheable" only ever means a transient or error
	// response, and there is no revalidation machinery left to make max-age=0
	// useful — so say no-store outright.
	return "no-store"
}

// cacheControlFor builds the Cache-Control value.
//
// max-age=0 + must-revalidate makes every browser re-ask before reusing a page,
// which is what keeps a language switch honest. s-maxage is what stops that from
// also crippling Souin: a shared cache prefers s-maxage, so it still serves for
// 6h without touching the origin. Emitting only max-age=0, must-revalidate would
// make Souin revalidate on every read and re-run the SPARQL query behind it —
// the trap that a previous qualified no-cache attempt fell into.
func cacheControlFor(cacheable bool) string {
	if cacheable {
		return "public, max-age=0, must-revalidate, s-maxage=" + sharedMaxAge
	}
	return "public, max-age=0, must-revalidate"
}

// varyFor returns the header this response varies on: the normalized
// X-Site-Lang when a cache in front of us produced one, otherwise the raw
// Accept-Language we had to negotiate ourselves.
func varyFor(c *gin.Context) string {
	if langViaHeader(c) {
		return lang.SiteLangHeader
	}
	return "Accept-Language"
}

// etagMiddleware buffers the response body, stamps it with an ETag derived from
// the body and the language, and answers a matching If-None-Match with 304.
//
// Buffering is affordable here because every response on these routes is already
// fully materialised before the write: c.JSON marshals up front, and the HTML
// and fragment renderers build the whole document in memory. It adds a copy, not
// a new memory class. Streaming and static routes are skipped entirely.
func etagMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isCacheableMethod(c.Request.Method) || skipETagPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		bw := &bufferingWriter{ResponseWriter: c.Writer}
		c.Writer = bw
		c.Next()
		c.Writer = bw.ResponseWriter

		body := bw.buf.Bytes()

		// Only successful, fully-buffered responses are ETagged. A handler that
		// hijacked or streamed the connection has already written its own bytes,
		// and an error response must never be replayed from a cache.
		if bw.hijacked || bw.status != http.StatusOK || bw.noStore() {
			bw.flush(body)
			return
		}

		h := bw.ResponseWriter.Header()

		// API tier: nothing about the response depends on a request header, so
		// it gets neither an ETag nor a Vary — just a TTL keyed by the URL.
		// Note this sits *below* the no-store check above, so the handlers that
		// opt a transient failure out of caching still win.
		if isURLPure(c) {
			h.Set("Cache-Control", apiCacheControl(isCacheable(c)))
			bw.flush(body)
			return
		}

		etag := etagFor(activeLang(c), body)
		h.Set("ETag", etag)
		h.Set("Vary", varyFor(c))
		// A handler that already chose no-store took the early return above, so
		// anything reaching here gets the revalidate-friendly policy.
		h.Set("Cache-Control", cacheControlFor(isCacheable(c)))

		if etagMatches(c.GetHeader("If-None-Match"), etag) {
			// 304 must not carry a body or a Content-Length for one.
			h.Del("Content-Length")
			bw.ResponseWriter.WriteHeader(http.StatusNotModified)
			return
		}

		bw.flush(body)
	}
}

func isCacheableMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// skipETagPath excludes routes that must not be buffered: static files (served
// straight from disk, and already covered by their own Cache-Control), the
// health/ping probes, and the MCP endpoint, which streams.
func skipETagPath(path string) bool {
	switch path {
	case "/ping", "/health", "/mcp", "/favicon.ico", "/robots.txt":
		return true
	}
	return strings.HasPrefix(path, "/static/")
}

// etagFor derives a strong ETag from the language and a hash of the body.
//
// The language prefix is not what makes the tag unique — the body hash already
// differs between languages — but it makes a captured request self-describing
// (`"fr-v8a1b2c"` says at a glance which variant a client holds), which is worth
// a lot when debugging a caching layer you cannot step through.
func etagFor(code string, body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + code + "-v" + hex.EncodeToString(sum[:])[:7] + `"`
}

// etagMatches implements the If-None-Match comparison: a comma-separated list of
// tags, "*" matching anything, and weak tags comparing equal to strong ones
// (weak comparison is what RFC 9110 mandates for If-None-Match).
func etagMatches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == want {
			return true
		}
	}
	return false
}

// bufferingWriter captures the response so its body can be hashed before any of
// it reaches the client. It reports status 200 by default, matching gin, so a
// handler that writes without calling WriteHeader is still handled.
type bufferingWriter struct {
	gin.ResponseWriter
	buf      bytes.Buffer
	status   int
	hijacked bool
	flushed  bool
}

func (w *bufferingWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

// WriteHeaderNow must not reach the wrapped writer: gin calls it to commit the
// status line, which would send headers before the body has been hashed and make
// the ETag unsettable. The real header is written in flush.
func (w *bufferingWriter) WriteHeaderNow() {}

// Hijack hands the connection over and records that fact, so the middleware
// leaves an upgraded connection alone instead of trying to ETag it.
func (w *bufferingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	return w.ResponseWriter.Hijack()
}

func (w *bufferingWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.buf.Write(b)
}

func (w *bufferingWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *bufferingWriter) Written() bool { return w.buf.Len() > 0 || w.status != 0 }

func (w *bufferingWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *bufferingWriter) Size() int { return w.buf.Len() }

// Flush is a no-op while buffering: a handler asking to flush mid-response would
// otherwise defeat the whole point. Streaming routes are excluded up front.
func (w *bufferingWriter) Flush() {}

// noStore reports whether the handler opted this response out of caching. Those
// handlers set the header directly (transient SPARQL failures, validation
// errors), and their choice wins over anything this middleware would add.
func (w *bufferingWriter) noStore() bool {
	return strings.Contains(w.ResponseWriter.Header().Get("Cache-Control"), "no-store")
}

// flush writes the buffered response through to the real writer exactly once.
func (w *bufferingWriter) flush(body []byte) {
	if w.flushed {
		return
	}
	w.flushed = true
	if w.status != 0 {
		w.ResponseWriter.WriteHeader(w.status)
	}
	if len(body) > 0 {
		w.ResponseWriter.Write(body)
	}
}
