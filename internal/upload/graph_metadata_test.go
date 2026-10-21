package upload

import (
	"strings"
	"testing"
	"time"
)

func TestGraphMetadataUpdates(t *testing.T) {
	at := time.Date(2026, 10, 21, 11, 48, 4, 0, time.UTC)
	got := graphMetadataUpdates("urn:visoto:graph:x", `DCAT "AP"`, at)
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

	if n := len(graphMetadataUpdates("urn:visoto:graph:x", "", at)); n != 2 {
		t.Errorf("empty label: want 2 updates (no label delete), got %d", n)
	}
}

func TestValidIRI(t *testing.T) {
	for iri, want := range map[string]bool{
		"urn:visoto:graph:20261021T114804-dcat": true,
		"https://example.org/g":                 true,
		"":                                      false,
		"urn:x> } ; DROP ALL ; {":               false,
		"urn:has space":                         false,
	} {
		if validIRI(iri) != want {
			t.Errorf("validIRI(%q) = %v, want %v", iri, !want, want)
		}
	}
}
