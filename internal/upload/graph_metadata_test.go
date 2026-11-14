package upload

import (
	"strings"
	"testing"
	"time"

	"github.com/pdacton/visoto/internal/config"
	"github.com/pdacton/visoto/internal/sparql"
)

func TestGraphMetadataUpdates(t *testing.T) {
	at := time.Date(2026, 10, 21, 11, 48, 4, 0, time.UTC)
	got := graphMetadataUpdates(sparql.MustIRI("urn:visoto:graph:x"), `DCAT "AP"`, at)
	if len(got) != 3 {
		t.Fatalf("want 3 updates, got %d: %v", len(got), got)
	}
	insert := got[2]
	for _, want := range []string{
		`INSERT DATA { GRAPH <urn:visoto:graph:x> {`,
		`"2026-10-21T11:48:04Z"^^<http://www.w3.org/2001/XMLSchema#dateTime>`,
		`<http://www.w3.org/2000/01/rdf-schema#label> "DCAT \"AP\""`,
	} {
		if !strings.Contains(insert, want) {
			t.Errorf("insert %q missing %q", insert, want)
		}
	}

	if n := len(graphMetadataUpdates(sparql.MustIRI("urn:visoto:graph:x"), "", at)); n != 2 {
		t.Errorf("empty label: want 2 updates (no label delete), got %d", n)
	}
}

// The graph URI of an upload is form input that ends up in UPDATE statements.
// writeGraphMetadata refuses one that is not a safe IRI before any request is
// built: the endpoint is never contacted (its URL here would fail the test).
func TestWriteGraphMetadataRefusesUnsafeGraphURI(t *testing.T) {
	ep := &config.SparqlEndpoint{URL: "http://127.0.0.1:1/unreachable"}
	for _, graph := range []string{"", "urn:x> } ; DROP ALL ; {", "urn:has space", "no-scheme"} {
		err := writeGraphMetadata(ep, graph, "label")
		if err == nil || !strings.Contains(err.Error(), "cannot be described") {
			t.Errorf("writeGraphMetadata(%q) = %v, want a refusal", graph, err)
		}
	}
}
