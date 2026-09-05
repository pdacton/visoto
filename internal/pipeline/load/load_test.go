package load

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/rdf"
)

func sampleQuads() []rdf.Quad {
	s := rdf.IRI("https://example.org/dataset/1")
	return []rdf.Quad{
		rdf.NewQuad(s, rdf.A, rdf.DcatDataset, ""),
		rdf.NewQuad(s, rdf.DctTitle, rdf.LangLiteral("Bevölkerung", "de"), ""),
		rdf.NewQuad(s, rdf.DctTitle, rdf.Term{}, ""), // malformed: must be skipped
	}
}

func TestBulkFileStagesNQuads(t *testing.T) {
	dir := t.TempDir()
	loader, err := New(config.LoaderBulkFile, Options{WorkDir: dir, RunID: "r7"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer loader.Close()

	ctx := context.Background()
	graph := "https://example.org/id/graph/catalog/src/r7"
	if err := loader.BeginGraph(ctx, graph); err != nil {
		t.Fatalf("BeginGraph: %v", err)
	}
	if err := loader.Append(ctx, graph, sampleQuads()); err != nil {
		t.Fatalf("Append: %v", err)
	}
	summary, err := loader.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if summary.QuadsWritten != 2 || summary.QuadsSkipped != 1 {
		t.Errorf("written=%d skipped=%d, want 2/1", summary.QuadsWritten, summary.QuadsSkipped)
	}
	if summary.NextStep == "" {
		t.Error("a staged load is not yet queryable, so it must report a next step")
	}
	if len(summary.Artifacts) != 2 {
		t.Fatalf("artifacts = %v", summary.Artifacts)
	}

	data, err := os.ReadFile(summary.Artifacts[0])
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("staged %d lines, want 2:\n%s", len(lines), data)
	}
	for _, line := range lines {
		if !strings.HasSuffix(line, "<"+graph+"> .") {
			t.Errorf("line is not routed to the graph: %s", line)
		}
	}

	var manifest struct {
		RunID  string           `json:"run_id"`
		Quads  int64            `json:"quads"`
		Graphs map[string]int64 `json:"graphs"`
	}
	raw, err := os.ReadFile(summary.Artifacts[1])
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.RunID != "r7" || manifest.Quads != 2 || manifest.Graphs[graph] != 2 {
		t.Errorf("manifest = %+v", manifest)
	}
}

func TestBulkFileRejectsDefaultGraph(t *testing.T) {
	loader, err := New(config.LoaderBulkFile, Options{WorkDir: t.TempDir(), RunID: "r"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer loader.Close()
	if err := loader.Append(context.Background(), "", sampleQuads()); err == nil {
		t.Error("writing to the default graph should be refused")
	}
}

func TestBulkFileCommitIsOnce(t *testing.T) {
	loader, err := New(config.LoaderBulkFile, Options{WorkDir: t.TempDir(), RunID: "r"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer loader.Close()
	if _, err := loader.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := loader.Commit(context.Background()); err == nil {
		t.Error("a second commit should fail rather than silently rewrite the manifest")
	}
}

func TestSparqlUpdateDropsThenInserts(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, string(body))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	loader, err := New(config.LoaderSparqlUpdate, Options{Endpoint: srv.URL, BatchQuads: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer loader.Close()

	ctx := context.Background()
	graph := "https://example.org/id/graph/catalog/src/r7"
	if err := ReplaceGraph(ctx, loader, graph, sampleQuads()); err != nil {
		t.Fatalf("ReplaceGraph: %v", err)
	}
	summary, err := loader.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// One DROP plus one INSERT per quad, because BatchQuads is 1.
	if len(requests) != 3 {
		t.Fatalf("sent %d requests, want 3:\n%s", len(requests), strings.Join(requests, "\n---\n"))
	}
	if !strings.HasPrefix(requests[0], "DROP SILENT GRAPH <") {
		t.Errorf("first request = %s", requests[0])
	}
	for _, req := range requests[1:] {
		if !strings.Contains(req, "INSERT DATA { GRAPH <"+graph+">") {
			t.Errorf("insert not scoped to the graph: %s", req)
		}
	}
	if summary.QuadsWritten != 2 || summary.QuadsSkipped != 1 {
		t.Errorf("written=%d skipped=%d, want 2/1", summary.QuadsWritten, summary.QuadsSkipped)
	}
	// The data is live, so there is nothing for an operator to do next.
	if summary.NextStep != "" {
		t.Errorf("NextStep = %q, want empty", summary.NextStep)
	}
}

func TestSparqlUpdateRefusesMalformedGraphIRI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should reach the endpoint")
	}))
	defer srv.Close()

	loader, err := New(config.LoaderSparqlUpdate, Options{Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer loader.Close()

	bad := "https://example.org/g> } ; DROP ALL ; INSERT DATA { <a> <b> <c"
	if err := loader.BeginGraph(context.Background(), bad); err == nil {
		t.Error("a graph IRI that escapes its brackets must be refused")
	}
}

func TestSparqlUpdateNeedsEndpoint(t *testing.T) {
	if _, err := New(config.LoaderSparqlUpdate, Options{}); err == nil {
		t.Error("sparql-update without an endpoint should fail at construction")
	}
}

func TestUnknownLoader(t *testing.T) {
	if _, err := New("nope", Options{}); err == nil {
		t.Error("unknown loader should fail")
	}
	names := Names()
	if len(names) != 2 || names[0] != config.LoaderBulkFile {
		t.Errorf("Names() = %v", names)
	}
}
