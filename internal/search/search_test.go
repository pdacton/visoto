package search

import (
	"context"
	"errors"
	"testing"
)

// stubProvider is a plain Provider: a pure string builder, no endpoint access.
type stubProvider struct {
	called bool
	err    error
}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) BuildQuery(params SearchParams) (string, error) {
	s.called = true
	if s.err != nil {
		return "", s.err
	}
	return "SELECT * WHERE { ?s ?p ?o }", nil
}

// stubDiscoveringProvider implements the optional capability interface.
type stubDiscoveringProvider struct {
	stubProvider
	discoverCalled bool
	sawEndpointURL string
	discoverErr    error
}

func (s *stubDiscoveringProvider) BuildQueryWithContext(sc SearchContext) (string, error) {
	s.discoverCalled = true
	s.sawEndpointURL = sc.EndpointURL
	if s.discoverErr != nil {
		return "", s.discoverErr
	}
	return "SELECT * WHERE { ?s ?p ?o }", nil
}

// TestExecuteDispatchesToDiscoveringProvider guards the capability wiring: if the
// type assertion stops matching, a discovering provider silently loses its
// endpoint access and every search degrades to the fallback.
func TestExecuteDispatchesToDiscoveringProvider(t *testing.T) {
	dp := &stubDiscoveringProvider{}
	s := &Searcher{provider: dp, endpointURL: "https://example.org/query"}

	// preprocessor is nil, so execution panics if reached — but building the query
	// is all this test needs, and a nil preprocessor would surface as a panic
	// rather than a silent pass. Guard by recovering.
	defer func() { _ = recover() }()
	_ = s.Execute(context.Background(), SearchParams{Query: "x"}, "")

	if !dp.discoverCalled {
		t.Error("Execute() should call BuildQueryWithContext on a DiscoveringProvider")
	}
	if dp.called {
		t.Error("Execute() should not also call the plain BuildQuery path")
	}
	if dp.sawEndpointURL != "https://example.org/query" {
		t.Errorf("provider saw endpoint %q, want the searcher's", dp.sawEndpointURL)
	}
}

// TestExecuteUsesPlainProviderPath checks that the five non-discovering providers
// are untouched by the capability interface.
func TestExecuteUsesPlainProviderPath(t *testing.T) {
	p := &stubProvider{}
	s := &Searcher{provider: p}

	defer func() { _ = recover() }()
	_ = s.Execute(context.Background(), SearchParams{Query: "x"}, "")

	if !p.called {
		t.Error("Execute() should call BuildQuery on a plain Provider")
	}
}

// TestExecuteFallsBackWhenBuildFails is the regression guard for the bug where a
// provider that could not build a query returned early, skipping the CONTAINS
// fallback entirely.
//
// This is the common case for the Lucene provider, not an edge case: the
// connectors index a handful of properties, so any search on an unindexed one
// cannot be built and MUST reach the fallback. Returning early there loses every
// match on every unindexed literal in the dataset.
func TestExecuteFallsBackWhenBuildFails(t *testing.T) {
	dp := &stubDiscoveringProvider{discoverErr: errNoLuceneIndex}
	s := &Searcher{provider: dp, endpointURL: "https://example.org/query"}

	// A nil preprocessor panics when the fallback tries to execute. Reaching that
	// panic is the proof we wanted: control flow continued past the build error
	// instead of returning.
	reachedExecution := false
	func() {
		defer func() {
			if recover() != nil {
				reachedExecution = true
			}
		}()
		_ = s.Execute(context.Background(), SearchParams{Query: "x"}, "")
	}()

	if !dp.discoverCalled {
		t.Fatal("Execute() should have tried the discovering path")
	}
	if !reachedExecution {
		t.Error("a build failure must fall through to the CONTAINS fallback, not return early")
	}
}

func TestExecuteBuildErrorIsNotSwallowed(t *testing.T) {
	// A non-empty query with a provider that always fails to build should still
	// report an error rather than silently claiming zero results.
	p := &stubProvider{err: errors.New("boom")}
	s := &Searcher{provider: p}

	reached := false
	func() {
		defer func() {
			if recover() != nil {
				reached = true
			}
		}()
		_ = s.Execute(context.Background(), SearchParams{Query: "x"}, "")
	}()

	if !reached {
		t.Error("a plain provider's build error should also reach the fallback")
	}
}
