package graphlayout

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"hutzli.org/visoto/internal/config"
)

func testStore(t *testing.T, cfg config.GraphLayoutsConfig) *Store {
	t.Helper()
	s, err := New(cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	clock := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { clock = clock.Add(time.Second); return clock }
	return s
}

// snap builds a minimal visoto-graph v1 canvas; n elements, tagged by label so
// distinct calls hash differently.
func snap(label string, n int, savedAt string) []byte {
	elems := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			elems += ","
		}
		elems += fmt.Sprintf(`{"@type":"Element","iri":"urn:%s:%d"}`, label, i)
	}
	return []byte(fmt.Sprintf(`{"format":"visoto-graph","version":1,"savedAt":%q,"layout":"force",`+
		`"diagram":{"layoutData":{"@type":"Layout","elements":[%s],"links":[]}}}`, savedAt, elems))
}

var key = Key{Endpoint: "lindas-prod", Page: "/resource?iri=urn%3Ax", Graph: "graph"}

func TestShareListGet(t *testing.T) {
	s := testStore(t, config.GraphLayoutsConfig{Mode: "open"})
	id, err := s.Share(key, snap("a", 3, "t1"))
	if err != nil {
		t.Fatal(err)
	}
	list, def, err := s.List(key)
	if err != nil {
		t.Fatal(err)
	}
	if def != id || len(list) != 1 || list[0].Nodes != 3 || list[0].Layout != "force" {
		t.Fatalf("list = %+v, default %q; want one 3-node row, default %q", list, def, id)
	}
	l, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if l.Key != key || l.Nodes != 3 {
		t.Fatalf("get = %+v", l)
	}
	if _, err := s.Get("0000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id: err = %v, want ErrNotFound", err)
	}
}

func TestShareDedupesIgnoringSavedAt(t *testing.T) {
	s := testStore(t, config.GraphLayoutsConfig{Mode: "open"})
	first, _ := s.Share(key, snap("a", 2, "t1"))
	other, _ := s.Share(key, snap("b", 2, "t2"))
	again, err := s.Share(key, snap("a", 2, "t3"))
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Fatalf("re-sharing an identical canvas made %q, want reuse of %q", again, first)
	}
	list, def, _ := s.List(key)
	if len(list) != 2 || list[0].ID != first || list[1].ID != other || def != first {
		t.Fatalf("list = %+v, default %q; want the reused row first and default", list, def)
	}
}

func TestSharePrunesPerPage(t *testing.T) {
	s := testStore(t, config.GraphLayoutsConfig{Mode: "open", KeepPerPage: 3})
	otherKey := key
	otherKey.Graph = "other"
	otherID, _ := s.Share(otherKey, snap("o", 1, ""))

	first, _ := s.Share(key, snap("0", 1, ""))
	for i := 1; i < 6; i++ {
		if _, err := s.Share(key, snap(fmt.Sprint(i), 1, "")); err != nil {
			t.Fatal(err)
		}
	}
	list, def, _ := s.List(key)
	if len(list) != 3 || def != list[0].ID {
		t.Fatalf("list = %+v, default %q; want 3 kept, newest default", list, def)
	}
	if _, err := s.Get(first); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest snapshot survived pruning")
	}
	if _, err := s.Get(otherID); err != nil {
		t.Fatalf("another graph's snapshot was pruned: %v", err)
	}

	// Reverting to an older kept snapshot moves only the pointer.
	if err := s.SetDefault(key, list[2].ID); err != nil {
		t.Fatal(err)
	}
	if after, def, _ := s.List(key); len(after) != 3 || def != list[2].ID {
		t.Fatalf("after revert: list %+v, default %q", after, def)
	}
}

func TestSetDefaultRejectsForeignID(t *testing.T) {
	s := testStore(t, config.GraphLayoutsConfig{Mode: "open"})
	otherKey := key
	otherKey.Page = "/other.html"
	foreign, _ := s.Share(otherKey, snap("f", 1, ""))
	if err := s.SetDefault(key, foreign); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound for another page's snapshot", err)
	}
}

func TestShareRowLimit(t *testing.T) {
	s := testStore(t, config.GraphLayoutsConfig{Mode: "open", MaxRows: 2})
	s.Share(key, snap("a", 1, ""))
	s.Share(key, snap("b", 1, ""))
	if _, err := s.Share(key, snap("c", 1, "")); !errors.Is(err, ErrFull) {
		t.Fatalf("err = %v, want ErrFull", err)
	}
	// An identical canvas needs no new row, so it is still accepted.
	if _, err := s.Share(key, snap("a", 1, "later")); err != nil {
		t.Fatalf("dedupe at the limit: %v", err)
	}
}

func TestShareRejectsInvalid(t *testing.T) {
	s := testStore(t, config.GraphLayoutsConfig{Mode: "open"})
	for _, body := range []string{
		`not json`,
		`{"format":"other","version":1,"diagram":{"layoutData":{}}}`,
		`{"format":"visoto-graph","version":2,"diagram":{"layoutData":{}}}`,
		`{"format":"visoto-graph","version":1,"diagram":{}}`,
	} {
		if _, err := s.Share(key, []byte(body)); !errors.Is(err, ErrInvalid) {
			t.Errorf("Share(%s) err = %v, want ErrInvalid", body, err)
		}
	}
}

func TestCanonicalPage(t *testing.T) {
	cases := map[string]string{
		"/resource?iri=urn%3Ax&endpoint=lindas-prod&layout=abc": "/resource?iri=urn%3Ax",
		"/resource?endpoint=x&iri=urn%3Ax":                      "/resource?iri=urn%3Ax",
		"/mobility.html?utm=1":                                  "/mobility.html",
		"/ontodia.html":                                         "/ontodia.html",
	}
	for in, want := range cases {
		if got, ok := CanonicalPage(in); !ok || got != want {
			t.Errorf("CanonicalPage(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "resource", "https://evil.example/x", "//evil.example/x"} {
		if _, ok := CanonicalPage(bad); ok {
			t.Errorf("CanonicalPage(%q) accepted", bad)
		}
	}
}
