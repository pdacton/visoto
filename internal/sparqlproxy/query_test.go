package sparqlproxy

import "testing"

func TestIsReadOnlyQuery(t *testing.T) {
	tests := []struct {
		name string
		q    string
		want bool
	}{
		// --- plain read forms ---
		{"select", "SELECT * WHERE { ?s ?p ?o }", true},
		{"lowercase", "select * where { ?s ?p ?o }", true},
		{"mixed case", "SeLeCt * WHERE { ?s ?p ?o }", true},
		{"ask", "ASK { ?s ?p ?o }", true},
		{"construct", "CONSTRUCT { ?s ?p ?o } WHERE { ?s ?p ?o }", true},
		{"describe", "DESCRIBE <http://example.org/a>", true},
		{"leading whitespace", "\n\n\t   SELECT * WHERE { ?s ?p ?o }", true},

		// --- declarations before the keyword ---
		{"prefix then select", "PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\nSELECT * WHERE { ?s ?p ?o }", true},
		{"base prefix construct", "BASE <http://example.org/>\nPREFIX a: <http://a.example/>\nCONSTRUCT { ?s ?p ?o } WHERE { ?s ?p ?o }", true},
		{"many prefixes", "PREFIX a: <http://a#>\nPREFIX b: <http://b#>\nPREFIX c: <http://c#>\nASK { ?s ?p ?o }", true},

		// --- the '#'-is-not-a-comment cases (the whole point of stripComments) ---
		{"hash inside IRI", "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nSELECT * WHERE { ?s ?p ?o }", true},
		{"hash in string literal", "SELECT * WHERE { ?s ?p \"a # b\" }", true},
		{"triple quoted with hash and INSERT", "SELECT * WHERE { ?s ?p \"\"\"# INSERT DATA { }\"\"\" }", true},
		{"escaped quote in literal", `SELECT * WHERE { ?s ?p "he said \"# INSERT\"" }`, true},

		// --- real comments ---
		{"comment then select", "# find everything\nSELECT * WHERE { ?s ?p ?o }", true},
		{"comment mentioning insert", "# this used to INSERT DATA\nSELECT * WHERE { ?s ?p ?o }", true},

		// --- updates: must all be refused ---
		{"insert data", "INSERT DATA { <a> <b> <c> }", false},
		{"delete where", "DELETE WHERE { ?s ?p ?o }", false},
		{"delete data", "DELETE DATA { <a> <b> <c> }", false},
		{"load", "LOAD <http://example.org/data.ttl>", false},
		{"clear", "CLEAR GRAPH <http://example.org/g>", false},
		{"drop", "DROP ALL", false},
		{"create", "CREATE GRAPH <http://example.org/g>", false},
		{"add", "ADD <http://a> TO <http://b>", false},
		{"move", "MOVE <http://a> TO <http://b>", false},
		{"copy", "COPY <http://a> TO <http://b>", false},
		{"with delete", "WITH <http://example.org/g> DELETE { ?s ?p ?o } WHERE { ?s ?p ?o }", false},
		{"prefix then insert", "PREFIX a: <http://a#>\nINSERT DATA { <a> <b> <c> }", false},
		{"lowercase insert", "insert data { <a> <b> <c> }", false},

		// --- the sneaky one: SELECT only inside a comment, real op is INSERT ---
		{"select in comment then insert", "# SELECT * WHERE { ?s ?p ?o }\nINSERT DATA { <a> <b> <c> }", false},

		// --- fail closed ---
		{"empty", "", false},
		{"whitespace only", "   \n\t  ", false},
		{"comment only", "# just a comment", false},
		{"prefixes only", "PREFIX a: <http://a#>\n", false},
		{"garbage", "]]>>not sparql", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsReadOnlyQuery(tc.q); got != tc.want {
				t.Errorf("IsReadOnlyQuery() = %v, want %v\nquery:\n%s", got, tc.want, tc.q)
			}
		})
	}
}
