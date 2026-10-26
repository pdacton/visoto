package sparql

import (
	"reflect"
	"testing"
)

func TestConcatSeparators(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  map[string]string
	}{
		{"dcat:Dataset themes", `SELECT ?dataset ?themes WHERE {
			OPTIONAL {
			  SELECT ?dataset (GROUP_CONCAT(DISTINCT ?themeName; separator="; ") AS ?themes) WHERE {
			    ?dataset dcat:theme/(schema:name|skos:prefLabel) ?themeName .
			  } GROUP BY ?dataset
			}
		}`, map[string]string{"themes": "; "}},
		{"no distinct", `(GROUP_CONCAT(?x; SEPARATOR=", ") AS ?xs)`, map[string]string{"xs": ", "}},
		{"single quotes", `(GROUP_CONCAT(?x;separator='|') AS ?xs)`, map[string]string{"xs": "|"}},
		{"lowercase, newlines", "(group_concat(\n  ?x ;\n  separator = \" / \"\n) as ?xs)", map[string]string{"xs": " / "}},
		{"nested call in expr", `(GROUP_CONCAT(DISTINCT STR(?x); separator=", ") AS ?xs)`, map[string]string{"xs": ", "}},
		{"escaped separator", `(GROUP_CONCAT(?x; separator="\n") AS ?xs)`, map[string]string{"xs": "\n"}},
		{"dollar variable", `(GROUP_CONCAT(?x; separator=", ") AS $xs)`, map[string]string{"xs": ", "}},
		{"default separator excluded", `(GROUP_CONCAT(?x) AS ?xs)`, map[string]string{}},
		{"empty separator excluded", `(GROUP_CONCAT(?x; separator="") AS ?xs)`, map[string]string{}},
		{"two aggregates, first without separator", `SELECT (GROUP_CONCAT(?a) AS ?as) (GROUP_CONCAT(?b; separator="; ") AS ?bs) WHERE {}`,
			map[string]string{"bs": "; "}},
		{"two aggregates", `SELECT (GROUP_CONCAT(?a; separator=", ") AS ?as) (GROUP_CONCAT(?b; separator="; ") AS ?bs) WHERE {}`,
			map[string]string{"as": ", ", "bs": "; "}},
		{"no aggregate", `SELECT ?s WHERE { ?s ?p ?o ; ?q ?r }`, map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConcatSeparators(tt.query); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConcatSeparators() = %q, want %q", got, tt.want)
			}
		})
	}
}
