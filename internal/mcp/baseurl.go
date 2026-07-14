package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// baseURLCtxKey is the private context key under which the per-request public
// base URL is stored by the streamable HTTP transport (see NewServer).
type baseURLCtxKey struct{}

// BaseURLFromRequest derives the public base URL (scheme://host) of this Visoto
// instance from an incoming request. Behind a reverse proxy the X-Forwarded-*
// headers win (Caddy sets X-Forwarded-Proto and preserves Host); a direct
// request falls back to its own Host header, and fallbackPort covers requests
// without one.
func BaseURLFromRequest(r *http.Request, fallbackPort int) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return fmt.Sprintf("http://localhost:%d", fallbackPort)
	}
	return strings.TrimRight(scheme+"://"+host, "/")
}

// contextWithBaseURL stores the derived public base URL in the request context.
func contextWithBaseURL(ctx context.Context, baseURL string) context.Context {
	return context.WithValue(ctx, baseURLCtxKey{}, baseURL)
}

// baseURLFromContext returns the base URL stored by contextWithBaseURL,
// falling back to http://localhost:<port> when the context carries none
// (e.g. tool calls made outside the HTTP transport).
func baseURLFromContext(ctx context.Context, fallbackPort int) string {
	if v, ok := ctx.Value(baseURLCtxKey{}).(string); ok && v != "" {
		return v
	}
	return fmt.Sprintf("http://localhost:%d", fallbackPort)
}
