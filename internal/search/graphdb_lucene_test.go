package search

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"hutzli.org/visoto/internal/cache"
	"hutzli.org/visoto/internal/sparql"
)

// lindasLabelOptions is the real options JSON returned by LINDAS for the
// lindas_label connector. Kept verbatim so the parser is pinned against ground
// truth rather than against an idealised shape.
const lindasLabelOptions = `{"languages":["en","fr","de","it","rm",""],"fields":[{"propertyChain":["http://www.w3.org/2000/01/rdf-schema#label"],"fieldName":"label$rdfslabel","facet":false},{"propertyChain":["http://www.w3.org/2004/02/skos/core#prefLabel"],"fieldName":"label$skosprefLabel","facet":false},{"propertyChain":["http://www.w3.org/2004/02/skos/core#altLabel"],"fieldName":"label$skosaltLabel","facet":false},{"propertyChain":["http://www.w3.org/2004/02/skos/core#hiddenLabel"],"fieldName":"label$skoshiddenLabel","facet":false},{"propertyChain":["http://purl.org/dc/terms/title"],"fieldName":"label$dctermstitle","facet":false},{"propertyChain":["http://purl.org/dc/terms/alternative"],"fieldName":"label$dctermsalternative","facet":false},{"propertyChain":["http://schema.org/name"],"fieldName":"label$schemaname","facet":false},{"propertyChain":["https://www.ica.org/standards/RiC/ontology#name"],"fieldName":"label$riconame","facet":false},{"propertyChain":["https://www.ica.org/standards/RiC/ontology#title"],"fieldName":"label$ricotitle","facet":false}],"types":["$untyped"]}`

const lindasIdentifierOptions = `{"fields":[{"propertyChain":["http://www.w3.org/2004/02/skos/core#notation"],"fieldName":"identifier$skosnotation","facet":false},{"propertyChain":["http://schema.org/identifier"],"fieldName":"identifier$schemaidentifier","facet":false},{"propertyChain":["http://purl.org/dc/terms/identifier"],"fieldName":"identifier$dctermsidentifier","facet":false},{"propertyChain":["https://www.ica.org/standards/RiC/ontology#identifier"],"fieldName":"identifier$ricoidentifier","facet":false}],"types":["$untyped"]}`

const (
	labelIRI      = "http://www.ontotext.com/connectors/lucene/instance#lindas_label"
	identifierIRI = "http://www.ontotext.com/connectors/lucene/instance#lindas_identifier"

	rdfsLabel    = "http://www.w3.org/2000/01/rdf-schema#label"
	schemaName   = "http://schema.org/name"
	skosNotation = "http://www.w3.org/2004/02/skos/core#notation"
)

// ── Fake executor ────────────────────────────────────────────────────────────

// fakeExecutor scripts a discovery conversation without a network. Responses are
// keyed by a substring of the query so a test can answer "the connector list"
// and "the options for X" distinctly.
type fakeExecutor struct {
	mu        sync.Mutex
	responses map[string]sparql.QueryResult
	errs      map[string]error
	calls     []string
	block     bool // when set, wait for ctx cancellation instead of answering
}

func (f *fakeExecutor) exec(ctx context.Context, query string) (sparql.QueryResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, query)
	f.mu.Unlock()

	if f.block {
		<-ctx.Done()
		return sparql.QueryResult{}, ctx.Err()
	}

	for substr, err := range f.errs {
		if strings.Contains(query, substr) {
			return sparql.QueryResult{}, err
		}
	}
	for substr, res := range f.responses {
		if strings.Contains(query, substr) {
			return res, nil
		}
	}
	return sparql.QueryResult{}, nil
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func connectorList(iris ...string) sparql.QueryResult {
	res := sparql.QueryResult{Vars: []string{"inst"}}
	for _, iri := range iris {
		res.Bindings = append(res.Bindings, map[string]sparql.Binding{
			"inst": {Type: "uri", Value: iri},
		})
	}
	return res
}

func optionsResult(json string) sparql.QueryResult {
	return sparql.QueryResult{
		Vars: []string{"options"},
		Bindings: []map[string]sparql.Binding{
			{"options": {Type: "literal", Value: json}},
		},
	}
}

// lindasFake returns an executor scripted with both real LINDAS connectors.
func lindasFake() *fakeExecutor {
	return &fakeExecutor{
		responses: map[string]sparql.QueryResult{
			"listConnectors":              connectorList(identifierIRI, labelIRI),
			"#lindas_label> conn:listOpt": optionsResult(lindasLabelOptions),
			"#lindas_identifier> conn:li": optionsResult(lindasIdentifierOptions),
		},
	}
}

// testDiscoverer returns a discoverer with caches isolated from the package
// default, so cache-behaviour tests do not contaminate each other.
func testDiscoverer() *discoverer {
	return &discoverer{
		positive: cache.New[luceneDiscovery](time.Hour),
		negative: cache.New[struct{}](time.Hour),
	}
}

func labelIndex() luceneIndex {
	groups, _ := parseConnectorOptions(lindasLabelOptions)
	return luceneIndex{IRI: labelIRI, Name: "lindas_label", Groups: groups}
}

func identifierIndex() luceneIndex {
	groups, _ := parseConnectorOptions(lindasIdentifierOptions)
	return luceneIndex{IRI: identifierIRI, Name: "lindas_identifier", Groups: groups}
}

// ── Pure functions ───────────────────────────────────────────────────────────

func TestFieldGroup(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  string
	}{
		{"label field", "label$rdfslabel", "label"},
		{"identifier field", "identifier$skosnotation", "identifier"},
		{"no separator", "flat", "flat"},
		{"empty", "", ""},
		{"multiple separators", "a$b$c", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fieldGroup(tt.field); got != tt.want {
				t.Errorf("fieldGroup(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

func TestParseConnectorOptions(t *testing.T) {
	groups, err := parseConnectorOptions(lindasLabelOptions)
	if err != nil {
		t.Fatalf("parseConnectorOptions() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1 (all fields share the 'label' prefix)", len(groups))
	}
	if len(groups["label"]) != 9 {
		t.Errorf("group 'label' has %d properties, want 9", len(groups["label"]))
	}

	var foundSchemaName bool
	for _, p := range groups["label"] {
		if p == schemaName {
			foundSchemaName = true
		}
	}
	if !foundSchemaName {
		t.Error("group 'label' should index schema:name")
	}
}

func TestParseConnectorOptionsMalformed(t *testing.T) {
	if _, err := parseConnectorOptions("{not json"); err == nil {
		t.Error("parseConnectorOptions() should error on malformed JSON")
	}
}

func TestParseConnectorOptionsUnknownKeys(t *testing.T) {
	raw := `{"analyzer":"custom","entityFilter":"x","fields":[{"propertyChain":["http://example.org/p"],"fieldName":"g$p"}]}`
	groups, err := parseConnectorOptions(raw)
	if err != nil {
		t.Fatalf("unknown keys should be ignored, got error = %v", err)
	}
	if len(groups["g"]) != 1 {
		t.Errorf("got %d properties in group 'g', want 1", len(groups["g"]))
	}
}

func TestParseConnectorOptionsMultiElementChain(t *testing.T) {
	raw := `{"fields":[{"propertyChain":["http://example.org/address","http://example.org/street"],"fieldName":"addr$street"}]}`
	groups, _ := parseConnectorOptions(raw)
	if len(groups["addr"]) != 2 {
		t.Errorf("every chain element should be indexed, got %d want 2", len(groups["addr"]))
	}
}

func TestSelectIndex(t *testing.T) {
	indexes := []luceneIndex{identifierIndex(), labelIndex()}

	tests := []struct {
		name      string
		property  string
		wantName  string
		wantGroup string
		wantOK    bool
	}{
		{"rdfs:label picks the label index", rdfsLabel, "lindas_label", "label", true},
		{"schema:name picks the label index", schemaName, "lindas_label", "label", true},
		{"skos:notation picks the identifier index", skosNotation, "lindas_identifier", "identifier", true},
		{"unindexed property matches nothing", "http://xmlns.com/foaf/0.1/name", "", "", false},
		{"any property picks the broadest group", "", "lindas_label", "label", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, group, ok := selectIndex(indexes, tt.property)
			if ok != tt.wantOK {
				t.Fatalf("selectIndex() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if idx.Name != tt.wantName {
				t.Errorf("selectIndex() index = %q, want %q", idx.Name, tt.wantName)
			}
			if group != tt.wantGroup {
				t.Errorf("selectIndex() group = %q, want %q", group, tt.wantGroup)
			}
		})
	}
}

// TestSelectIndexDeterministic guards the property that makes selection safe:
// SPARQL result order is not guaranteed, so the same inputs in a different order
// must still pick the same index.
func TestSelectIndexDeterministic(t *testing.T) {
	forward := []luceneIndex{identifierIndex(), labelIndex()}
	reversed := []luceneIndex{labelIndex(), identifierIndex()}

	for _, property := range []string{"", rdfsLabel, skosNotation} {
		a, ga, oka := selectIndex(forward, property)
		b, gb, okb := selectIndex(reversed, property)
		if oka != okb || a.IRI != b.IRI || ga != gb {
			t.Errorf("selectIndex(property=%q) not order-independent: %q/%q vs %q/%q",
				property, a.Name, ga, b.Name, gb)
		}
	}
}

// TestSelectIndexTiebreak checks the most-specific-wins rule when two indexes
// both cover the searched property.
func TestSelectIndexTiebreak(t *testing.T) {
	shared := "http://example.org/shared"
	broad := luceneIndex{
		IRI: "http://x/instance#broad", Name: "broad",
		Groups: map[string][]string{"g": {shared, "http://example.org/a", "http://example.org/b"}},
	}
	narrow := luceneIndex{
		IRI: "http://x/instance#narrow", Name: "narrow",
		Groups: map[string][]string{"g": {shared}},
	}

	idx, _, ok := selectIndex([]luceneIndex{broad, narrow}, shared)
	if !ok {
		t.Fatal("selectIndex() should match")
	}
	if idx.Name != "narrow" {
		t.Errorf("selectIndex() = %q, want the more specific %q", idx.Name, "narrow")
	}
}

func TestLuceneQueryText(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"bare term gets the group prefix", "bern", "label:bern"},
		{"explicit field selector is left alone", "label:bern", "label:bern"},
		{"wildcard keeps the prefix", "bern*", "label:bern*"},
		{"a URL is treated as raw syntax", "http://x/y", "http://x/y"},
		{"boolean operators survive", "bern OR zurich", "label:bern OR zurich"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := luceneQueryText(tt.query, "label"); got != tt.want {
				t.Errorf("luceneQueryText(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestBuildLuceneQueryNoClass(t *testing.T) {
	q, err := buildLuceneQuery(labelIndex(), "label", SearchParams{Query: "bern", Limit: 50})
	if err != nil {
		t.Fatalf("buildLuceneQuery() error = %v", err)
	}

	for _, want := range []string{
		"luc:entities ?subject",
		"?subject luc:score ?score",
		"OPTIONAL { ?subject rdf:type ?type . }",
		"SELECT ?type ?subject ?score",
		`luc:query "label:bern"`,
		"ORDER BY DESC(?score)",
		"LIMIT 50",
		"<" + labelIRI + ">",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}

	// A nested SELECT is the capped-subquery shape, only used with a class filter.
	if strings.Count(q, "SELECT") != 1 {
		t.Errorf("unfiltered query should have exactly one SELECT:\n%s", q)
	}
}

func TestBuildLuceneQueryWithClass(t *testing.T) {
	params := SearchParams{
		Query: "bern",
		Class: "https://schema.ld.admin.ch/Municipality",
		Limit: 50,
	}
	q, err := buildLuceneQuery(labelIndex(), "label", params)
	if err != nil {
		t.Fatalf("buildLuceneQuery() error = %v", err)
	}

	if strings.Count(q, "SELECT") != 2 {
		t.Errorf("class-filtered query should nest a capped subquery:\n%s", q)
	}
	for _, want := range []string{
		"<https://schema.ld.admin.ch/Municipality>",
		"BIND(",
		"LIMIT 500", // inner cap: max(50*10, 500)
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q:\n%s", want, q)
		}
	}
	// The type is known when the class is pinned, so no OPTIONAL lookup.
	if strings.Contains(q, "OPTIONAL") {
		t.Errorf("class-filtered query should not use OPTIONAL:\n%s", q)
	}
}

func TestBuildLuceneQueryEscaping(t *testing.T) {
	q, err := buildLuceneQuery(labelIndex(), "label", SearchParams{Query: `say "hi"`, Limit: 10})
	if err != nil {
		t.Fatalf("buildLuceneQuery() error = %v", err)
	}
	if !strings.Contains(q, `\"hi\"`) {
		t.Errorf("quotes should be escaped for the SPARQL literal:\n%s", q)
	}

	// Lucene metacharacters must survive untouched.
	q2, _ := buildLuceneQuery(labelIndex(), "label", SearchParams{Query: "+foo -bar", Limit: 10})
	if !strings.Contains(q2, "+foo -bar") {
		t.Errorf("Lucene operators should pass through unmodified:\n%s", q2)
	}
}

func TestBuildLuceneQueryRejectsBadClass(t *testing.T) {
	params := SearchParams{Query: "bern", Class: "not an iri", Limit: 10}
	if _, err := buildLuceneQuery(labelIndex(), "label", params); err == nil {
		t.Error("buildLuceneQuery() should reject an invalid class IRI")
	}
}

// ── Discovery ────────────────────────────────────────────────────────────────

func TestDiscoverySuccess(t *testing.T) {
	d, fake := testDiscoverer(), lindasFake()

	got, err := d.discover(context.Background(), "https://ld.admin.ch/query/", fake.exec)
	if err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	if len(got.Indexes) != 2 {
		t.Fatalf("discover() found %d indexes, want 2", len(got.Indexes))
	}
	// Sorted by IRI: lindas_identifier precedes lindas_label.
	if got.Indexes[0].Name != "lindas_identifier" {
		t.Errorf("indexes should be sorted by IRI, got %q first", got.Indexes[0].Name)
	}
	if n := fake.callCount(); n != 3 {
		t.Errorf("discover() made %d queries, want 3 (1 list + 2 options)", n)
	}
}

func TestDiscoveryCached(t *testing.T) {
	d, fake := testDiscoverer(), lindasFake()
	const ep = "https://ld.admin.ch/query/"

	if _, err := d.discover(context.Background(), ep, fake.exec); err != nil {
		t.Fatalf("first discover() error = %v", err)
	}
	first := fake.callCount()

	if _, err := d.discover(context.Background(), ep, fake.exec); err != nil {
		t.Fatalf("second discover() error = %v", err)
	}
	if fake.callCount() != first {
		t.Errorf("second discover() should hit the cache, made %d extra queries",
			fake.callCount()-first)
	}
}

func TestDiscoveryNoConnectors(t *testing.T) {
	d := testDiscoverer()
	// A non-GraphDB endpoint answers with an empty result, not an error.
	fake := &fakeExecutor{responses: map[string]sparql.QueryResult{
		"listConnectors": {Vars: []string{"inst"}},
	}}

	if _, err := d.discover(context.Background(), "https://qlever.dev/api/wikidata", fake.exec); err != errNoLuceneIndex {
		t.Fatalf("discover() error = %v, want errNoLuceneIndex", err)
	}
	first := fake.callCount()

	// The negative must be cached so the ~10 non-GraphDB endpoints do not re-probe.
	if _, err := d.discover(context.Background(), "https://qlever.dev/api/wikidata", fake.exec); err != errNoLuceneIndex {
		t.Fatalf("second discover() error = %v, want errNoLuceneIndex", err)
	}
	if fake.callCount() != first {
		t.Error("a zero-connector result should be cached negatively")
	}
}

func TestDiscoveryEndpointErrorNotCached(t *testing.T) {
	d := testDiscoverer()
	fake := &fakeExecutor{errs: map[string]error{
		"listConnectors": context.DeadlineExceeded,
	}}

	if _, err := d.discover(context.Background(), "https://down.example/query", fake.exec); err != errNoLuceneIndex {
		t.Fatalf("discover() error = %v, want errNoLuceneIndex", err)
	}
	first := fake.callCount()

	// An endpoint being down says nothing about its connectors, so it re-probes.
	_, _ = d.discover(context.Background(), "https://down.example/query", fake.exec)
	if fake.callCount() <= first {
		t.Error("an endpoint error should not be cached; the next search must re-probe")
	}
}

func TestDiscoveryEmptyEndpointURLNeverCaches(t *testing.T) {
	d, fake := testDiscoverer(), lindasFake()

	if _, err := d.discover(context.Background(), "", fake.exec); err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	first := fake.callCount()

	if _, err := d.discover(context.Background(), "", fake.exec); err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	if fake.callCount() == first {
		t.Error("an unidentified endpoint must probe fresh, never cache under \"\"")
	}
}

func TestDiscoveryCancelledContext(t *testing.T) {
	d := testDiscoverer()
	fake := &fakeExecutor{block: true}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := d.discover(ctx, "https://slow.example/query", fake.exec); err != errNoLuceneIndex {
			t.Errorf("discover() error = %v, want errNoLuceneIndex", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("discover() did not return promptly on a cancelled context")
	}
}

func TestDiscoveryPartialOptionsFailure(t *testing.T) {
	d := testDiscoverer()
	fake := &fakeExecutor{
		responses: map[string]sparql.QueryResult{
			"listConnectors":              connectorList(identifierIRI, labelIRI),
			"#lindas_label> conn:listOpt": optionsResult(lindasLabelOptions),
		},
		errs: map[string]error{
			"#lindas_identifier> conn:li": context.DeadlineExceeded,
		},
	}

	got, err := d.discover(context.Background(), "https://partial.example/query", fake.exec)
	if err != nil {
		t.Fatalf("discover() error = %v", err)
	}
	if len(got.Indexes) != 1 || got.Indexes[0].Name != "lindas_label" {
		t.Errorf("one failing connector should not lose the others, got %d indexes", len(got.Indexes))
	}
}

// ── End-to-end query building ────────────────────────────────────────────────

func TestBuildQueryWithContextSelectsByProperty(t *testing.T) {
	tests := []struct {
		name     string
		property string
		want     string
	}{
		{"label property uses the label connector", rdfsLabel, "#lindas_label>"},
		{"notation property uses the identifier connector", skosNotation, "#lindas_identifier>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, fake := testDiscoverer(), lindasFake()
			q, err := d.buildQuery(SearchContext{
				Ctx:         context.Background(),
				Params:      SearchParams{Query: "bern", Property: tt.property, Limit: 50},
				EndpointURL: "https://ld.admin.ch/query/",
				Execute:     fake.exec,
			})
			if err != nil {
				t.Fatalf("buildQuery() error = %v", err)
			}
			if !strings.Contains(q, tt.want) {
				t.Errorf("query should target %s:\n%s", tt.want, q)
			}
		})
	}
}

func TestBuildQueryWithContextUncoveredProperty(t *testing.T) {
	d, fake := testDiscoverer(), lindasFake()

	_, err := d.buildQuery(SearchContext{
		Ctx:         context.Background(),
		Params:      SearchParams{Query: "bern", Property: "http://schema.org/description", Limit: 50},
		EndpointURL: "https://ld.admin.ch/query/",
		Execute:     fake.exec,
	})
	if err != errNoLuceneIndex {
		t.Fatalf("buildQuery() error = %v, want errNoLuceneIndex (falls back to CONTAINS)", err)
	}
}

func TestBuildQueryWithContextEmptyQuery(t *testing.T) {
	d, fake := testDiscoverer(), lindasFake()
	if _, err := d.buildQuery(SearchContext{
		Ctx:     context.Background(),
		Params:  SearchParams{Query: ""},
		Execute: fake.exec,
	}); err == nil {
		t.Error("buildQuery() should reject an empty search query")
	}
}

// TestGraphDBLuceneBuildQueryErrors pins the context-free path: without an
// endpoint to interrogate this provider cannot build a query, and erroring is
// what routes the search to the CONTAINS fallback instead of failing the page.
func TestGraphDBLuceneBuildQueryErrors(t *testing.T) {
	p := &GraphDBLuceneProvider{}
	if _, err := p.BuildQuery(SearchParams{Query: "bern"}); err == nil {
		t.Error("BuildQuery() should error; discovery needs a SearchContext")
	}
	if p.Name() != "graphdb-lucene" {
		t.Errorf("Name() = %q, want %q", p.Name(), "graphdb-lucene")
	}
}

// TestGraphDBLuceneImplementsDiscovering guards the wiring: if the capability
// interface stops being satisfied, Searcher silently uses the erroring
// BuildQuery path and every search quietly falls back to CONTAINS.
func TestGraphDBLuceneImplementsDiscovering(t *testing.T) {
	var p Provider = &GraphDBLuceneProvider{}
	if _, ok := p.(DiscoveringProvider); !ok {
		t.Error("GraphDBLuceneProvider must implement DiscoveringProvider")
	}
}
