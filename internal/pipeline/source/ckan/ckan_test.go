package ckan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/source"
)

// multilingualPackage is shaped like opendata.swiss: language maps where stock
// CKAN has plain strings, and resources carrying media_type and byte_size.
const multilingualPackage = `{
  "success": true,
  "result": {
    "count": 1,
    "results": [{
      "id": "8f2a-uuid",
      "name": "bevoelkerung-2024",
      "title": {"de": "Bevölkerung 2024", "fr": "Population 2024", "en": ""},
      "notes": {"de": "Ständige Wohnbevölkerung."},
      "url": "https://opendata.swiss/de/dataset/bevoelkerung-2024",
      "license_id": "cc-by",
      "license_url": "https://creativecommons.org/licenses/by/4.0/",
      "metadata_created": "2024-01-04T08:00:00.123456",
      "metadata_modified": "2024-03-01T09:12:33.123456",
      "organization": {"name": "bundesamt-fuer-statistik", "title": {"de": "Bundesamt für Statistik"}},
      "tags": [{"name": "bevoelkerung"}, {"name": "gemeinde"}],
      "groups": [{"name": "population", "title": {"de": "Bevölkerung"}}],
      "resources": [
        {
          "id": "res-1",
          "name": {"de": "CSV Datei"},
          "url": "https://example.org/data/bev.csv",
          "format": "CSV",
          "media_type": "text/csv",
          "byte_size": "24110",
          "last_modified": "2024-03-01T09:00:00",
          "rights": "NonCommercialAllowed-CommercialAllowed-ReferenceRequired"
        },
        {
          "id": "res-2",
          "url": "not a url",
          "format": "XLSX",
          "size": 993
        }
      ]
    }]
  }
}`

func newSource(t *testing.T, srv *httptest.Server) source.Source {
	t.Helper()
	// dataset_iri_base is set explicitly: minted IRIs must be a function of the
	// catalogue, never of the host the API happened to be reached on (which here
	// is an ephemeral test port, and in production could be an internal name).
	s, err := New(config.PipelineSource{
		Name:           "opendata-swiss",
		Type:           TypeName,
		URL:            srv.URL + "/api/3/action",
		DatasetIRIBase: "https://opendata.swiss/dataset",
		PageSize:       100,
	}, source.Options{
		UserAgent: "visoto-harvest/test",
		Minter:    rdf.NewMinter("https://example.org/id/"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func harvestOne(t *testing.T, body string) source.Record {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/3/action/package_search") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("start") != "0" {
			// second page: report exhaustion
			w.Write([]byte(`{"success":true,"result":{"count":1,"results":[]}}`))
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	var got []source.Record
	err := newSource(t, srv).Harvest(context.Background(), time.Time{}, func(r source.Record) error {
		got = append(got, r)
		return nil
	})
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	return got[0]
}

// index makes the emitted quads searchable by subject and predicate.
func index(quads []rdf.Quad) map[string][]rdf.Term {
	out := make(map[string][]rdf.Term)
	for _, q := range quads {
		key := q.Subject.Value + " " + q.Predicate.Value
		out[key] = append(out[key], q.Object)
	}
	return out
}

func TestCKANMapsToDCAT(t *testing.T) {
	rec := harvestOne(t, multilingualPackage)

	wantDataset := "https://opendata.swiss/dataset/bevoelkerung-2024"
	if rec.DatasetIRI != wantDataset {
		t.Fatalf("DatasetIRI = %q, want %q", rec.DatasetIRI, wantDataset)
	}
	idx := index(rec.Quads)

	titles := idx[wantDataset+" "+rdf.DctTitle.Value]
	if len(titles) != 2 {
		t.Fatalf("got %d titles, want 2 (an empty language must be dropped): %v", len(titles), titles)
	}
	// Sorted by language so repeated harvests emit identical output.
	if titles[0].Lang != "de" || titles[0].Value != "Bevölkerung 2024" {
		t.Errorf("first title = %+v", titles[0])
	}
	if titles[1].Lang != "fr" {
		t.Errorf("second title = %+v", titles[1])
	}

	if got := idx[wantDataset+" "+rdf.A.Value]; len(got) != 1 || got[0] != rdf.DcatDataset {
		t.Errorf("dataset type = %v", got)
	}

	// CKAN's dotted timestamps must normalize to xsd:dateTime.
	mod := idx[wantDataset+" "+rdf.DctModified.Value]
	if len(mod) != 1 || mod[0].Datatype != rdf.Xsd("dateTime") || mod[0].Value != "2024-03-01T09:12:33Z" {
		t.Errorf("dct:modified = %+v", mod)
	}

	// A licence URL becomes an IRI; a licence id would have stayed a literal.
	lic := idx[wantDataset+" "+rdf.DctLicense.Value]
	if len(lic) != 1 || lic[0].Kind != rdf.KindIRI {
		t.Errorf("dct:license = %+v", lic)
	}

	if got := idx[wantDataset+" "+rdf.DcatKeyword.Value]; len(got) != 2 {
		t.Errorf("keywords = %v", got)
	}

	publisher := idx[wantDataset+" "+rdf.DctPublisher.Value]
	if len(publisher) != 1 || publisher[0].Kind != rdf.KindIRI {
		t.Fatalf("dct:publisher = %+v", publisher)
	}
	if got := idx[publisher[0].Value+" "+rdf.FoafName.Value]; len(got) != 1 || got[0].Lang != "de" {
		t.Errorf("publisher name = %v", got)
	}
}

func TestCKANExtractsDistributions(t *testing.T) {
	rec := harvestOne(t, multilingualPackage)
	if len(rec.Distributions) != 2 {
		t.Fatalf("got %d distributions, want 2", len(rec.Distributions))
	}

	csv := rec.Distributions[0]
	if csv.IRI != "https://opendata.swiss/dataset/bevoelkerung-2024/resource/res-1" {
		t.Errorf("distribution IRI = %q", csv.IRI)
	}
	if csv.DownloadURL != "https://example.org/data/bev.csv" {
		t.Errorf("DownloadURL = %q", csv.DownloadURL)
	}
	if csv.DeclaredMedia != "text/csv" || csv.DeclaredFormat != "CSV" {
		t.Errorf("media=%q format=%q", csv.DeclaredMedia, csv.DeclaredFormat)
	}
	if csv.ByteSize != 24110 {
		t.Errorf("ByteSize = %d, want 24110 (a numeric string must decode)", csv.ByteSize)
	}
	if csv.Licence == "" {
		t.Error("rights should have become the licence when license is absent")
	}
	if csv.Modified.IsZero() {
		t.Error("last_modified should have parsed")
	}

	// "not a url" must not become an IRI: a wrong triple is worse than a missing
	// one (R-SRC-4).
	xlsx := rec.Distributions[1]
	if xlsx.DownloadURL != "" {
		t.Errorf("DownloadURL = %q, want empty for a non-URL", xlsx.DownloadURL)
	}
	if xlsx.ByteSize != 993 {
		t.Errorf("ByteSize = %d, want 993 from the `size` fallback", xlsx.ByteSize)
	}
}

func TestCKANIsDeterministic(t *testing.T) {
	first := harvestOne(t, multilingualPackage)
	second := harvestOne(t, multilingualPackage)

	if len(first.Quads) != len(second.Quads) {
		t.Fatalf("quad count differs between runs: %d vs %d", len(first.Quads), len(second.Quads))
	}
	for i := range first.Quads {
		if first.Quads[i].String() != second.Quads[i].String() {
			t.Fatalf("quad %d differs between runs:\n  %s\n  %s",
				i, first.Quads[i], second.Quads[i])
		}
	}
}

func TestCKANSkolemizesResourceWithoutID(t *testing.T) {
	body := `{"success":true,"result":{"count":1,"results":[{
		"name":"x","resources":[{"url":"https://example.org/a.csv","format":"CSV"}]}]}}`
	rec := harvestOne(t, body)
	if len(rec.Distributions) != 1 {
		t.Fatalf("got %d distributions", len(rec.Distributions))
	}
	iri := rec.Distributions[0].IRI
	if !strings.Contains(iri, "/skolem/distribution/") {
		t.Errorf("IRI = %q, want a skolemized IRI", iri)
	}
	if harvestOne(t, body).Distributions[0].IRI != iri {
		t.Error("skolemized IRI is not stable across runs")
	}
}

func TestCKANPagesAndFiltersBySince(t *testing.T) {
	var queries []string
	page := func(start, count int, results string) string {
		return fmt.Sprintf(`{"success":true,"result":{"count":%d,"results":[%s]}}`, count, results)
	}
	pkg := func(name string) string {
		b, _ := json.Marshal(map[string]any{"name": name, "resources": []any{}})
		return string(b)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		switch r.URL.Query().Get("start") {
		case "0":
			w.Write([]byte(page(0, 3, pkg("a")+","+pkg("b"))))
		default:
			w.Write([]byte(page(0, 3, pkg("c"))))
		}
	}))
	t.Cleanup(srv.Close)

	since := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	var names []string
	err := newSource(t, srv).Harvest(context.Background(), since, func(rec source.Record) error {
		names = append(names, rec.DatasetIRI)
		return nil
	})
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("got %d datasets across pages, want 3: %v", len(names), names)
	}
	if len(queries) != 2 {
		t.Fatalf("made %d requests, want 2", len(queries))
	}
	if !strings.Contains(queries[0], "metadata_modified") {
		t.Errorf("incremental harvest did not filter: %s", queries[0])
	}
}

func TestCKANEmitStopsHarvest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(multilingualPackage))
	}))
	t.Cleanup(srv.Close)

	sentinel := fmt.Errorf("stop")
	err := newSource(t, srv).Harvest(context.Background(), time.Time{}, func(source.Record) error {
		return sentinel
	})
	if err != sentinel {
		t.Errorf("Harvest error = %v, want the emit error propagated", err)
	}
}

func TestCKANRetriesServerErrors(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.Write([]byte(`{"success":true,"result":{"count":0,"results":[]}}`))
	}))
	t.Cleanup(srv.Close)

	if err := newSource(t, srv).Harvest(context.Background(), time.Time{}, func(source.Record) error {
		return nil
	}); err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 (one retry after 502)", attempts)
	}
}

func TestCKANDoesNotRetryClientErrors(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	if err := newSource(t, srv).Harvest(context.Background(), time.Time{}, func(source.Record) error {
		return nil
	}); err == nil {
		t.Fatal("expected an error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (a 400 is a bug, not a blip)", attempts)
	}
}
