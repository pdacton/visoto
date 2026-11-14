package search

import (
	"strings"
	"testing"
)

// ?class= and ?property= are request input that every provider writes into its
// query as an IRI term. All five used to splice them raw, so a class such as the
// one below closed the <…> early and appended graph patterns of the caller's
// choosing. Every provider must refuse to build a query from it.
func TestProvidersRefuseHostileFilters(t *testing.T) {
	const hostile = "http://schema.org/Person> . ?s ?p ?secret . FILTER(?p != <http://x/"

	providers := map[string]Provider{
		"stardog":      &StardogProvider{},
		"qlever":       &QLeverProvider{},
		"fuseki":       &FusekiProvider{},
		"graphdb":      &GraphDBProvider{},
		"sparql-query": &SparqlQueryProvider{},
	}
	for name, p := range providers {
		for field, params := range map[string]SearchParams{
			"class":    {Query: "bern", Class: hostile},
			"property": {Query: "bern", Property: hostile},
		} {
			query, err := p.BuildQuery(params)
			if err == nil {
				t.Errorf("%s: hostile %s accepted, query:\n%s", name, field, query)
			}
		}

		// The same filters as ordinary IRIs still build, each as one closed term.
		query, err := p.BuildQuery(SearchParams{
			Query: "bern", Class: "http://schema.org/Person", Property: "http://schema.org/name",
		})
		if err != nil {
			t.Errorf("%s: legitimate filters refused: %v", name, err)
			continue
		}
		for _, term := range []string{"<http://schema.org/Person>", "<http://schema.org/name>"} {
			if !strings.Contains(query, term) {
				t.Errorf("%s: query lacks %s:\n%s", name, term, query)
			}
		}
	}
}
