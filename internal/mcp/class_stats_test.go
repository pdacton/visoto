package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goMcp "github.com/mark3labs/mcp-go/mcp"

	"hutzli.org/visoto/internal/classstats"
	"hutzli.org/visoto/internal/config"
)

func classStatsToolContext(t *testing.T) *toolContext {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		q := string(body)
		switch {
		case strings.Contains(q, "SELECT ?class ?label ?parent"):
			fmt.Fprint(w, `{"head":{"vars":["class"]},"results":{"bindings":[
				{"class":{"type":"uri","value":"https://example.org/Big"}},
				{"class":{"type":"uri","value":"https://example.org/Small"}}]}}`)
		case strings.Contains(q, "Big"):
			fmt.Fprint(w, `{"head":{"vars":["n"]},"results":{"bindings":[{"n":{"type":"literal","value":"900"}}]}}`)
		default:
			fmt.Fprint(w, `{"head":{"vars":["n"]},"results":{"bindings":[{"n":{"type":"literal","value":"4"}}]}}`)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{Application: config.ApplicationConfig{SparqlEndpoints: []config.SparqlEndpoint{
		{Name: "LINDAS prod", URL: srv.URL + "/prod", Slug: "lindas-prod", ClassStats: true},
		{Name: "LINDAS cached", URL: srv.URL + "/cached", Slug: "lindas-cached", ClassStatsFrom: "lindas-prod", Default: true},
		{Name: "Other", URL: srv.URL + "/other", Slug: "other"},
	}}}
	cs, err := classstats.New(&cfg.Application, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.Run(context.Background(), "lindas-prod"); err != nil {
		t.Fatal(err)
	}
	return &toolContext{cfg: cfg, classStats: cs}
}

func TestCountInstancesFromSnapshot(t *testing.T) {
	tc := classStatsToolContext(t)
	ctx := context.Background()

	// Default endpoint (the cached twin) is served from lindas-prod's snapshot.
	for _, ep := range []string{"", "LINDAS cached", "lindas-prod", tc.cfg.Application.SparqlEndpoints[1].URL} {
		r, ok := tc.countInstancesFromSnapshot(ctx, "", ep)
		if !ok || r.RowCount != 2 || r.Results[0]["type"] != "https://example.org/Big" || r.Results[0]["count"] != "900" {
			t.Errorf("endpoint %q: ok=%v %+v", ep, ok, r)
		}
		if len(r.Hints) == 0 || !strings.Contains(r.Hints[0], "snapshot of") {
			t.Errorf("endpoint %q: hints %v lack the snapshot date", ep, r.Hints)
		}
	}
	if r, ok := tc.countInstancesFromSnapshot(ctx, "https://example.org/Small", ""); !ok || r.Results[0]["count"] != "4" {
		t.Errorf("single class: ok=%v %+v", ok, r)
	}
	// Not in the snapshot, or an endpoint without statistics: live query.
	if _, ok := tc.countInstancesFromSnapshot(ctx, "https://example.org/Unknown", ""); ok {
		t.Error("unknown class must fall through to the live count")
	}
	if _, ok := tc.countInstancesFromSnapshot(ctx, "", "other"); ok {
		t.Error("endpoint without class statistics must fall through to the live count")
	}
	if _, ok := (&toolContext{cfg: tc.cfg}).countInstancesFromSnapshot(ctx, "", ""); ok {
		t.Error("no collector must fall through to the live count")
	}
}

func TestClassTrendsTool(t *testing.T) {
	tc := classStatsToolContext(t)
	call := func(args map[string]any) toolResult {
		t.Helper()
		req := goMcp.CallToolRequest{}
		req.Params.Arguments = args
		res, err := tc.handleClassTrends(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		var r toolResult
		if err := json.Unmarshal([]byte(res.Content[0].(goMcp.TextContent).Text), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	// One snapshot only: nothing to compare yet, said so.
	r := call(map[string]any{"endpoint": "lindas-cached", "days": 7})
	if r.Error != "" || r.RowCount != 0 || !strings.Contains(strings.Join(r.Hints, " "), "nothing to compare") {
		t.Errorf("trends = %+v", r)
	}
	// A class series.
	r = call(map[string]any{"class_iri": "https://example.org/Big"})
	if r.RowCount != 1 || r.Results[0]["count"] != "900" {
		t.Errorf("series = %+v", r)
	}
	// No statistics: an error that says where they come from.
	r = call(map[string]any{"endpoint": "other"})
	if !strings.Contains(r.Error, "class_stats") {
		t.Errorf("error = %q", r.Error)
	}
}
