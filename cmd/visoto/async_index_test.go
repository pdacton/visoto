package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/lang"
	"hutzli.org/visoto/internal/templates"
)

const testTemplatesDir = "../../templates"

// mustIndex builds the index over the shipped templates. Failing here is the
// same failure that aborts startup, so every test below doubles as a boot check.
func mustIndex(t *testing.T) {
	t.Helper()
	if err := initAsyncIndex(testTemplatesDir); err != nil {
		t.Fatalf("initAsyncIndex(): %v", err)
	}
}

// TestInitAsyncIndex runs the startup gate in CI: duplicate <sparql-async> ids
// within a set, <sparql-facet for=…> naming no base query, and {{ template }}
// includes a set does not parse all abort here rather than on the server's next
// boot.
func TestInitAsyncIndex(t *testing.T) {
	mustIndex(t)

	if len(asyncIdx.queries) == 0 {
		t.Fatal("no template sets indexed")
	}
	var total int
	for _, ids := range asyncIdx.queries {
		total += len(ids)
	}
	if total == 0 {
		t.Fatal("no <sparql-async> declarations indexed")
	}
}

// TestEveryAsyncIDResolvesFromItsOwnSet is the invariant every /api fragment
// route depends on: the id a page renders is findable with that page's set name.
func TestEveryAsyncIDResolvesFromItsOwnSet(t *testing.T) {
	mustIndex(t)

	for set, ids := range asyncIdx.queries {
		for id, want := range ids {
			got, found := findAsyncQuery(set, id)
			if !found {
				t.Errorf("findAsyncQuery(%q, %q) not found", set, id)
				continue
			}
			if got != want {
				t.Errorf("findAsyncQuery(%q, %q) returned a different query", set, id)
			}
		}
	}
}

// TestAsyncIDsAreScopedToTheirSet is the point of the whole change: an id
// declared by one page must not resolve for another. Before this, ids lived in
// one global namespace and the first match across five directories won.
func TestAsyncIDsAreScopedToTheirSet(t *testing.T) {
	mustIndex(t)

	// A page-declared id, and a set that does not declare it.
	const (
		pageSet   = "pages/plazi.html"
		pageID    = "AnimaliaCount"
		otherSet  = "pages/health.html"
		layoutID  = "resourceTriples" // declared in layout/base.html
		unknownID = "nope-does-not-exist"
	)

	if _, found := findAsyncQuery(pageSet, pageID); !found {
		t.Fatalf("%q should declare %q — update this test if the template changed", pageSet, pageID)
	}
	if _, found := findAsyncQuery(otherSet, pageID); found {
		t.Errorf("%q resolved from %q; ids must not leak across sets", pageID, otherSet)
	}
	if _, found := findAsyncQuery("", pageID); found {
		t.Errorf("%q resolved with an empty src; a request without ?src= must miss", pageID)
	}
	if _, found := findAsyncQuery(pageSet, unknownID); found {
		t.Errorf("unknown id %q resolved from %q", unknownID, pageSet)
	}

	// A layout-declared query is a member of every set, so it must resolve from
	// all of them — this is what lets the resource Data view work on any page.
	for set := range asyncIdx.queries {
		if _, found := findAsyncQuery(set, layoutID); !found {
			t.Errorf("layout query %q not visible from set %q", layoutID, set)
		}
	}
}

// TestFacetSpecsResolveFromTheirSet checks the facet half of the index, and that
// every declared facet still finds its base query.
func TestFacetSpecsResolveFromTheirSet(t *testing.T) {
	mustIndex(t)

	var seen int
	for set, byBase := range asyncIdx.facets {
		for baseID, specs := range byBase {
			if len(specs) == 0 {
				t.Errorf("set %q base %q indexed with no specs", set, baseID)
			}
			if got := findFacetSpecs(set, baseID); len(got) != len(specs) {
				t.Errorf("findFacetSpecs(%q, %q) = %d specs, want %d", set, baseID, len(got), len(specs))
			}
			for _, s := range specs {
				if _, ok := findFacetSpec(set, baseID, s.Var); !ok {
					t.Errorf("findFacetSpec(%q, %q, %q) not found", set, baseID, s.Var)
				}
				seen++
			}
		}
	}
	if seen == 0 {
		t.Skip("no <sparql-facet> declarations in the shipped templates")
	}
}

// TestMetricHandlerRequiresSrc covers the wire contract end to end. Only the
// not-found paths are exercised: a resolved id would run a real SPARQL query.
func TestMetricHandlerRequiresSrc(t *testing.T) {
	mustIndex(t)

	// metricHandler resolves the data language before it looks the query up, so
	// the language set has to exist even on the paths that never reach a query.
	siteLangs = lang.New(configLanguages(config.DefaultLanguages()), "en")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/metric/:id", metricHandler)

	for _, tc := range []struct {
		name string
		url  string
	}{
		{"no src", "/api/metric/AnimaliaCount"},
		{"wrong src", "/api/metric/AnimaliaCount?src=pages/health.html"},
		{"unknown src", "/api/metric/AnimaliaCount?src=pages/nope.html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if w.Code != http.StatusNotFound {
				t.Errorf("GET %s = %d, want %d", tc.url, w.Code, http.StatusNotFound)
			}
		})
	}
}

// TestIndexedSetsMatchRegisteredTemplates ties the index to the renderer: the
// set names the frontend sends as ?src= are produced by {{ templateSet }}, which
// is bound from the same mapping Load registers under. If these two ever drift,
// every async request on the affected page 404s.
func TestIndexedSetsMatchRegisteredTemplates(t *testing.T) {
	mustIndex(t)

	sets, err := templates.SetFiles(testTemplatesDir)
	if err != nil {
		t.Fatalf("SetFiles(): %v", err)
	}
	if len(sets) != len(asyncIdx.queries) {
		t.Errorf("indexed %d sets, SetFiles reports %d", len(asyncIdx.queries), len(sets))
	}
	for set := range sets {
		if _, ok := asyncIdx.queries[set]; !ok {
			t.Errorf("set %q missing from the async index", set)
		}
	}
}
