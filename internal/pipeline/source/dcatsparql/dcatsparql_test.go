package dcatsparql

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/source"
)

// fakeEndpoint answers the adapter's two query shapes from a fixed dataset.
type fakeEndpoint struct {
	listQueries     []string
	describeQueries []string
	datasets        []string
	triples         map[string][]map[string]any // dataset IRI → solution bindings
}

// row builds one solution binding for the describe response.
func row(dataset, s, sType, p, o, oType, lang, datatype string) map[string]any {
	binding := map[string]any{
		"dataset": map[string]any{"type": "uri", "value": dataset},
		"s":       map[string]any{"type": sType, "value": s},
		"p":       map[string]any{"type": "uri", "value": p},
	}
	obj := map[string]any{"type": oType, "value": o}
	if lang != "" {
		obj["xml:lang"] = lang
	}
	if datatype != "" {
		obj["datatype"] = datatype
	}
	binding["o"] = obj
	return binding
}

func (f *fakeEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	query := string(body)

	w.Header().Set("Content-Type", "application/sparql-results+json")
	if strings.Contains(query, "SELECT DISTINCT ?dataset") {
		f.listQueries = append(f.listQueries, query)
		bindings := []map[string]any{}
		// Only the first page has content; the adapter stops on a short page.
		if !strings.Contains(query, "OFFSET 0") {
			f.writeResults(w, []string{"dataset"}, bindings)
			return
		}
		for _, d := range f.datasets {
			bindings = append(bindings, map[string]any{
				"dataset": map[string]any{"type": "uri", "value": d},
			})
		}
		f.writeResults(w, []string{"dataset"}, bindings)
		return
	}

	f.describeQueries = append(f.describeQueries, query)
	bindings := []map[string]any{}
	for _, ds := range f.datasets {
		if !strings.Contains(query, ds) {
			continue
		}
		bindings = append(bindings, f.triples[ds]...)
	}
	f.writeResults(w, []string{"dataset", "s", "p", "o"}, bindings)
}

func (f *fakeEndpoint) writeResults(w http.ResponseWriter, vars []string, bindings []map[string]any) {
	payload := map[string]any{
		"head":    map[string]any{"vars": vars},
		"results": map[string]any{"bindings": bindings},
	}
	json.NewEncoder(w).Encode(payload)
}

const (
	dsIRI   = "https://data.europa.eu/dataset/abc"
	distIRI = "https://data.europa.eu/distribution/abc-1"
)

func fixture() *fakeEndpoint {
	return &fakeEndpoint{
		datasets: []string{dsIRI},
		triples: map[string][]map[string]any{
			dsIRI: {
				row(dsIRI, dsIRI, "uri", rdf.A.Value, rdf.DcatDataset.Value, "uri", "", ""),
				row(dsIRI, dsIRI, "uri", rdf.DctTitle.Value, "Population", "literal", "en", ""),
				row(dsIRI, dsIRI, "uri", rdf.DctModified.Value, "2024-06-01T00:00:00Z", "literal", "", rdf.Xsd("dateTime")),
				row(dsIRI, dsIRI, "uri", rdf.DcatHasDist.Value, distIRI, "uri", "", ""),
				row(dsIRI, distIRI, "uri", rdf.A.Value, rdf.DcatDistribution.Value, "uri", "", ""),
				row(dsIRI, distIRI, "uri", rdf.DcatDownloadURL.Value, "https://example.org/pop.csv", "uri", "", ""),
				row(dsIRI, distIRI, "uri", rdf.DcatMediaType.Value,
					"https://www.iana.org/assignments/media-types/text/csv", "uri", "", ""),
				row(dsIRI, distIRI, "uri", rdf.DcatByteSize.Value, "51200", "literal", "", rdf.Xsd("decimal")),
				// A blank-node contact point, as DCAT-AP catalogues commonly publish.
				row(dsIRI, dsIRI, "uri", rdf.DcatContactPoint.Value, "bn1", "bnode", "", ""),
				row(dsIRI, "bn1", "bnode", rdf.FoafName.Value, "Statistics Desk", "literal", "", ""),
			},
		},
	}
}

func newSource(t *testing.T, url string) source.Source {
	t.Helper()
	s, err := New(config.PipelineSource{
		Name:     "data-europa",
		Type:     TypeName,
		URL:      url,
		PageSize: 100,
	}, source.Options{
		UserAgent: "visoto-harvest/test",
		Minter:    rdf.NewMinter("https://example.org/id/"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func harvest(t *testing.T, fake *fakeEndpoint, since time.Time) []source.Record {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	var got []source.Record
	if err := newSource(t, srv.URL).Harvest(context.Background(), since, func(r source.Record) error {
		got = append(got, r)
		return nil
	}); err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	return got
}

func TestDCATSparqlHarvest(t *testing.T) {
	records := harvest(t, fixture(), time.Time{})
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	rec := records[0]
	if rec.DatasetIRI != dsIRI {
		t.Errorf("DatasetIRI = %q", rec.DatasetIRI)
	}
	if len(rec.Distributions) != 1 {
		t.Fatalf("got %d distributions, want 1", len(rec.Distributions))
	}

	d := rec.Distributions[0]
	if d.IRI != distIRI {
		t.Errorf("distribution IRI = %q", d.IRI)
	}
	if d.DownloadURL != "https://example.org/pop.csv" {
		t.Errorf("DownloadURL = %q", d.DownloadURL)
	}
	// An IANA media-type IRI must reduce to the bare media type.
	if d.DeclaredMedia != "text/csv" {
		t.Errorf("DeclaredMedia = %q, want text/csv", d.DeclaredMedia)
	}
	if d.ByteSize != 51200 {
		t.Errorf("ByteSize = %d", d.ByteSize)
	}
}

func TestDCATSparqlSkolemizesBlankNodes(t *testing.T) {
	records := harvest(t, fixture(), time.Time{})
	for _, q := range records[0].Quads {
		if q.Subject.Kind == rdf.KindBlank || q.Object.Kind == rdf.KindBlank {
			t.Fatalf("blank node survived skolemization: %s", q)
		}
	}

	var contact string
	for _, q := range records[0].Quads {
		if q.Predicate == rdf.DcatContactPoint {
			contact = q.Object.Value
		}
	}
	if !strings.Contains(contact, "/skolem/node/") {
		t.Fatalf("contact point = %q, want a skolemized IRI", contact)
	}

	// Re-harvesting must reproduce the same IRI, or every run would look like a
	// change (R-CAT-3).
	second := harvest(t, fixture(), time.Time{})
	for _, q := range second[0].Quads {
		if q.Predicate == rdf.DcatContactPoint && q.Object.Value != contact {
			t.Errorf("skolem IRI changed between runs: %q vs %q", contact, q.Object.Value)
		}
	}
}

func TestDCATSparqlIncrementalFilter(t *testing.T) {
	fake := fixture()
	harvest(t, fake, time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC))
	if len(fake.listQueries) == 0 {
		t.Fatal("no list query issued")
	}
	q := fake.listQueries[0]
	if !strings.Contains(q, "dct:modified") || !strings.Contains(q, "2024-05-01T00:00:00Z") {
		t.Errorf("incremental query lacks the watermark filter:\n%s", q)
	}

	fake2 := fixture()
	harvest(t, fake2, time.Time{})
	if strings.Contains(fake2.listQueries[0], "FILTER") {
		t.Errorf("full harvest must not filter:\n%s", fake2.listQueries[0])
	}
}

func TestDCATSparqlSkipsMalformedDatasetIRIs(t *testing.T) {
	fake := fixture()
	// An IRI carrying a closing bracket would break out of the VALUES clause.
	fake.datasets = append(fake.datasets, "https://evil.example/> } INSERT DATA { <a> <b> <c>")
	records := harvest(t, fake, time.Time{})

	for _, q := range fake.describeQueries {
		if strings.Contains(q, "INSERT DATA") {
			t.Fatalf("malformed IRI was interpolated into a query:\n%s", q)
		}
	}
	if len(records) != 1 {
		t.Errorf("got %d records, want the well-formed one only", len(records))
	}
}

func TestDCATSparqlRetriesRetryableStatus(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"head":{"vars":["dataset"]},"results":{"bindings":[]}}`))
	}))
	t.Cleanup(srv.Close)

	if err := newSource(t, srv.URL).Harvest(context.Background(), time.Time{}, func(source.Record) error {
		return nil
	}); err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}
