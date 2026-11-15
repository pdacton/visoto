package mcp

import (
	"context"
	"strings"
	"testing"

	goMcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/pdacton/visoto/internal/access"
	"github.com/pdacton/visoto/internal/config"
)

// denyPrivate refuses every action on the "private" store and SPARQL anywhere.
type denyPrivate struct{}

func (denyPrivate) Authorize(_ context.Context, r access.Request) bool {
	return r.Store != "private" && r.Action != access.SPARQL
}

func accessToolContext() *toolContext {
	cfg := &config.Config{Application: config.ApplicationConfig{
		SparqlEndpoint: "http://127.0.0.1:1/public",
		SparqlEndpoints: []config.SparqlEndpoint{
			{Name: "Public", Slug: "public", URL: "http://127.0.0.1:1/public", Default: true},
			{Name: "Private", Slug: "private", URL: "http://127.0.0.1:1/private"},
		},
	}}
	return &toolContext{cfg: cfg, authz: func() access.Authorizer { return denyPrivate{} }}
}

func toolRequest(args map[string]any) goMcp.CallToolRequest {
	var req goMcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func TestGuardedToolsAskTheAuthorizer(t *testing.T) {
	tc := accessToolContext()
	ran := false
	tool := func(context.Context, goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
		ran = true
		return goMcp.NewToolResultText("ok"), nil
	}
	for _, tcase := range []struct {
		action   access.Action
		args     map[string]any
		wantDeny bool
	}{
		{access.Browse, map[string]any{"endpoint": "private"}, true},
		{access.Browse, map[string]any{"endpoint": "Private"}, true}, // by name
		{access.Browse, map[string]any{"endpoint": "http://127.0.0.1:1/private"}, true},
		{access.SPARQL, map[string]any{}, true},
		{access.Browse, map[string]any{}, false},
		{access.Browse, map[string]any{"endpoint": "public"}, false},
	} {
		ran = false
		res, _ := tc.guarded(tcase.action, tool)(context.Background(), toolRequest(tcase.args))
		if tcase.wantDeny && (ran || !res.IsError) {
			t.Errorf("%s %v: tool ran or no error, want refusal", tcase.action, tcase.args)
		}
		if !tcase.wantDeny && !ran {
			t.Errorf("%s %v: refused, want the tool to run", tcase.action, tcase.args)
		}
	}
}

func TestListEndpointsHidesUnbrowsableStores(t *testing.T) {
	res, err := accessToolContext().handleListEndpoints(context.Background(), toolRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(goMcp.TextContent).Text
	if strings.Contains(text, "Private") || !strings.Contains(text, "Public") {
		t.Errorf("list_endpoints = %s, want only Public", text)
	}
}

// A tool argument is whatever the calling model was persuaded to send. One that
// is spliced into a query as an IRI is refused unless it is a safe IRI — before
// anything runs: this context has no preprocessor, so reaching the endpoint
// would panic rather than pass.
func TestToolsRefuseHostileIRIArguments(t *testing.T) {
	const hostile = "http://x/a> ?p ?o } UNION { ?s ?p ?o . FILTER(?s != <http://x/"
	tc := accessToolContext()
	type handler func(context.Context, goMcp.CallToolRequest) (*goMcp.CallToolResult, error)
	for name, call := range map[string]struct {
		h    handler
		args map[string]any
	}{
		"get_resource":         {tc.handleGetResource, map[string]any{"iri": hostile}},
		"search_by_label type": {tc.handleSearchByLabel, map[string]any{"text": "x", "type_filter": hostile}},
		"count_instances":      {tc.handleCountInstances, map[string]any{"class_iri": hostile}},
		"discover_classes":     {tc.handleDiscoverClasses, map[string]any{"graph": hostile}},
	} {
		res, err := call.h(context.Background(), toolRequest(call.args))
		if err != nil || res == nil || !res.IsError {
			t.Errorf("%s: hostile IRI argument was not refused (result %v, err %v)", name, res, err)
		}
	}
}
