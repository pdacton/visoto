package mcp

import (
	"testing"

	"hutzli.org/visoto/internal/sparql"
)

func classRows(pairs ...string) []map[string]sparql.Binding {
	var rows []map[string]sparql.Binding
	for i := 0; i+1 < len(pairs); i += 2 {
		rows = append(rows, map[string]sparql.Binding{
			"type":  {Value: pairs[i], Type: "uri", DisplayText: pairs[i]},
			"count": {Value: pairs[i+1], Type: "literal", DisplayText: pairs[i+1]},
		})
	}
	return rows
}

// TestMergeClassCounts adds counts across graphs into one row per class,
// sorts by count, skips non-numeric counts and reports failed graphs.
func TestMergeClassCounts(t *testing.T) {
	results := map[string]sparql.QueryResult{
		"g1": {Bindings: classRows("ex:Obs", "10", "ex:Term", "4")},
		"g2": {Bindings: classRows("ex:Obs", "5", "ex:Org", "7", "ex:Bad", "n/a")},
		"g3": {Error: "Query timeout exceeded"},
	}

	classes, failed := mergeClassCounts(results)

	want := []classCount{
		{iri: "ex:Obs", display: "ex:Obs", count: 15},
		{iri: "ex:Org", display: "ex:Org", count: 7},
		{iri: "ex:Term", display: "ex:Term", count: 4},
	}
	if len(classes) != len(want) {
		t.Fatalf("got %d classes %+v, want %d", len(classes), classes, len(want))
	}
	for i := range want {
		if classes[i] != want[i] {
			t.Errorf("classes[%d] = %+v, want %+v", i, classes[i], want[i])
		}
	}
	if len(failed) != 1 || failed[0] != "g3" {
		t.Errorf("failed = %v, want [g3]", failed)
	}
}

// TestMergeClassCountsTieOrder keeps equal counts in a stable IRI order, so
// the limit cut is deterministic.
func TestMergeClassCountsTieOrder(t *testing.T) {
	results := map[string]sparql.QueryResult{
		"g1": {Bindings: classRows("ex:B", "3")},
		"g2": {Bindings: classRows("ex:A", "3")},
	}
	classes, _ := mergeClassCounts(results)
	if len(classes) != 2 || classes[0].iri != "ex:A" || classes[1].iri != "ex:B" {
		t.Errorf("classes = %+v, want ex:A then ex:B", classes)
	}
}
