package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/lang"
	"hutzli.org/visoto/internal/parser"
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
// within a set, a <sparql-column> naming no base query, a malformed column
// declaration, a leftover <sparql-facet>, and {{ template }} includes a set does
// not parse all abort here rather than on the server's next boot.
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

// TestColumnsResolveFromTheirSet checks the column half of the index, and that
// every declared column still finds its base query.
func TestColumnsResolveFromTheirSet(t *testing.T) {
	mustIndex(t)

	var seen int
	for set, byBase := range asyncIdx.columns {
		for baseID, specs := range byBase {
			if len(specs) == 0 {
				t.Errorf("set %q base %q indexed with no columns", set, baseID)
			}
			if got := findColumns(set, baseID); len(got) != len(specs) {
				t.Errorf("findColumns(%q, %q) = %d columns, want %d", set, baseID, len(got), len(specs))
			}
			for _, s := range specs {
				if _, ok := findColumn(set, baseID, s.Var); !ok {
					t.Errorf("findColumn(%q, %q, %q) not found", set, baseID, s.Var)
				}
				seen++
			}
		}
	}
	if seen == 0 {
		t.Skip("no <sparql-column> declarations in the shipped templates")
	}
}

// TestColumnsInheritTheirContainer covers the compact form: a <sparql-columns
// for="…"> writes the base query id once and its children inherit it. The
// Municipality class template is the widest user of it.
func TestColumnsInheritTheirContainer(t *testing.T) {
	mustIndex(t)

	const (
		set  = "classes/schch%3AMunicipality.html"
		base = "municipalityInstances"
	)
	cols := findColumns(set, base)
	if len(cols) < 2 {
		t.Fatalf("findColumns(%q, %q) = %d columns; the template should declare a container full of them",
			set, base, len(cols))
	}
	// None of them carries for= itself — inheritance is the only reason they resolve.
	if _, ok := findColumn(set, base, "canton"); !ok {
		t.Error(`the "canton" column did not inherit its container's for=`)
	}
	if cols.IconVar() == "" {
		t.Error("no column flagged icon; the icon var is meant to come from the declaration")
	}
	if !cols.Filterable() {
		t.Error("no column carries a filter; this table is supposed to be faceted")
	}
}

// TestNoLegacyFacetDeclarations is the rename guard. Nothing reads <sparql-facet>
// any more, so a missed one would silently cost a table its filters; initAsyncIndex
// fails startup instead, and mustIndex would already have caught it. This asserts
// the guard is wired the way the error message promises.
func TestNoLegacyFacetDeclarations(t *testing.T) {
	err := rejectLegacyFacets("templates/example.html", []parser.ExtractedElement{
		{TagName: "sparql-facet", Attributes: map[string]string{"for": "x", "var": "y", "control": "select"}},
	})
	if err == nil {
		t.Fatal("rejectLegacyFacets accepted a configured <sparql-facet>")
	}
	if !strings.Contains(err.Error(), "sparql-column") {
		t.Errorf("error should name the replacement element, got: %v", err)
	}
	// Prose that merely names the tag carries no attributes and must be left alone.
	if err := rejectLegacyFacets("templates/example.html", []parser.ExtractedElement{
		{TagName: "sparql-facet", Attributes: map[string]string{}},
	}); err != nil {
		t.Errorf("rejectLegacyFacets rejected a bare mention: %v", err)
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
