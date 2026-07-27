package sparql

import "strings"

import "testing"

// The payload that was exploitable before validation existed: it closes the
// <...> IRI term early, appends its own triple pattern, and comments out the rest
// of the declared query with a trailing '#'.
const injectionIRI = "https://schema.ld.admin.ch/ZefixOrganisation> . ?org <http://schema.org/identifier> ?name . #"

func TestValidateIRIRejectsInjection(t *testing.T) {
	bad := []struct {
		name string
		iri  string
	}{
		{"term-closing injection", injectionIRI},
		{"bare closing bracket", "http://x/>"},
		{"opening bracket", "http://x/<y"},
		{"space", "http://x/ y"},
		{"newline", "http://x/\ny"},
		{"tab", "http://x/\ty"},
		{"double quote", `http://x/"y`},
		{"brace", "http://x/{y}"},
		{"backslash", `http://x/\y`},
		{"pipe", "http://x/|y"},
		{"backtick", "http://x/`y"},
		{"caret", "http://x/^y"},
		{"nul byte", "http://x/\x00y"},
		{"empty", ""},
		{"relative", "/not/absolute"},
		{"bare word", "notaniri"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateIRI(tt.iri); err == nil {
				t.Errorf("ValidateIRI(%q) = nil, want error", tt.iri)
			}
			if _, err := IRITerm(tt.iri); err == nil {
				t.Errorf("IRITerm(%q) = nil error, want error", tt.iri)
			}
		})
	}
}

func TestValidateIRIAcceptsLegitimate(t *testing.T) {
	good := []string{
		"https://schema.ld.admin.ch/ZefixOrganisation",
		"http://www.w3.org/2000/01/rdf-schema#label",
		"http://schema.org/name",
		"urn:uuid:6e8bc430-9c3a-11d9-9669-0800200c9a66",
		"https://ld.admin.ch/ech/97/legalforms/0106",
	}
	for _, iri := range good {
		if err := ValidateIRI(iri); err != nil {
			t.Errorf("ValidateIRI(%q) = %v, want nil", iri, err)
		}
		term, err := IRITerm(iri)
		if err != nil {
			t.Fatalf("IRITerm(%q): %v", iri, err)
		}
		if term != "<"+iri+">" {
			t.Errorf("IRITerm(%q) = %q", iri, term)
		}
	}
}

func TestSubstituteEntityRejectsInjection(t *testing.T) {
	declared := "SELECT ?org ?name WHERE { ?org a ?? . OPTIONAL { ?org <http://schema.org/name> ?name } }"

	if _, err := SubstituteEntity(declared, injectionIRI); err == nil {
		t.Fatal("SubstituteEntity accepted an injection payload")
	}

	got, err := SubstituteEntity(declared, "https://x/C")
	if err != nil {
		t.Fatalf("SubstituteEntity: %v", err)
	}
	if strings.Contains(got, "??") {
		t.Errorf("placeholder not substituted: %s", got)
	}
	if !strings.Contains(got, "?org a <https://x/C> .") {
		t.Errorf("unexpected substitution result: %s", got)
	}
}

func TestValidateVarName(t *testing.T) {
	for _, ok := range []string{"org", "taxonName", "_x", "a1_b"} {
		if err := ValidateVarName(ok); err != nil {
			t.Errorf("ValidateVarName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "1abc", "a-b", "a b", "a}", "org . ?s ?p ?o", "a\n"} {
		if err := ValidateVarName(bad); err == nil {
			t.Errorf("ValidateVarName(%q) = nil, want error", bad)
		}
	}
}

// The working-set builders take request input (class IRI, key var, search
// property) and must reject anything that could inject query syntax.
func TestWorkingSetBuildersRejectInjection(t *testing.T) {
	declared := "SELECT ?org WHERE { ?org a ?? }"

	if _, err := BuildWorkingSetQuery(declared, injectionIRI, "org", "", "", 100); err == nil {
		t.Error("BuildWorkingSetQuery accepted an injected class IRI")
	}
	if _, err := BuildWorkingSetQuery(declared, "https://x/C", "org . ?s ?p ?o", "", "", 100); err == nil {
		t.Error("BuildWorkingSetQuery accepted an injected key var")
	}
	if _, err := MembershipBody("https://x/C", "org", "term", "http://p/> . ?s ?p ?o . #"); err == nil {
		t.Error("MembershipBody accepted an injected search property")
	}
	// The legitimate shape still builds.
	if _, err := MembershipBody("https://x/C", "org", "term", "http://schema.org/name"); err != nil {
		t.Errorf("MembershipBody rejected legitimate input: %v", err)
	}
}
