// Package access defines what a request does with a store — the unit an
// Authorizer decides on — and the open default under which everything is
// allowed. The public app package re-exports these types for plugins.
package access

import "context"

// Action is what a route does with the store it addresses.
type Action string

const (
	// Public routes touch no store data: health checks, static assets,
	// redirects, the API description. They are never refused.
	Public Action = "public"
	// Browse reads a store through Visoto's own queries: pages, /api fragments,
	// search, store listings.
	Browse Action = "browse"
	// SPARQL runs caller-supplied or bulk queries: /api/sparql (Graph Explorer,
	// schema diagram), graph export, chat, MCP run_sparql_query.
	SPARQL Action = "sparql"
	// Upload writes to a store: loading RDF, dropping a graph.
	Upload Action = "upload"
	// Admin is an operational route: cache purge, monitoring on/off.
	Admin Action = "admin"
	// SignedIn needs some account but no store right: sharing a graph layout.
	SignedIn Action = "signed-in"
)

// Request is one access decision: Action on Store (an endpoint slug; "" for an
// endpoint that is not configured), optionally narrowed to one resource IRI.
type Request struct {
	Action Action
	Store  string
	IRI    string
}

// Authorizer decides requests. ctx is the HTTP request's context, so a plugin
// that stored the caller's identity there in its middleware finds it again; a
// context without one is an anonymous caller. context.Background() is therefore
// how the core asks "may an anonymous visitor see this?" — the test for whether
// a response may enter the shared cache.
type Authorizer interface {
	Authorize(ctx context.Context, r Request) bool
}

// AllowAll is the open-source default: no accounts, everything allowed.
type AllowAll struct{}

func (AllowAll) Authorize(context.Context, Request) bool { return true }
