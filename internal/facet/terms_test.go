package facet

import "testing"

func TestIRITerm(t *testing.T) {
	valid := "http://schema.ld.admin.ch/Municipality"
	got, err := IRITerm(valid)
	if err != nil {
		t.Fatalf("valid IRI rejected: %v", err)
	}
	if got != "<"+valid+">" {
		t.Fatalf("got %q", got)
	}

	// Injection / malformed inputs must all be rejected.
	bad := []string{
		"",
		"http://ex/a> UNION { ?s ?p ?o } #", // early close
		"http://ex/a b",                     // space
		`http://ex/"a"`,                     // quote
		"http://ex/{x}",                     // braces
		"not-absolute",                      // no scheme
		"http://ex/a\nb",                    // newline
	}
	for _, s := range bad {
		if _, err := IRITerm(s); err == nil {
			t.Errorf("expected rejection for %q", s)
		}
	}
}

func TestRangeBound(t *testing.T) {
	cases := []struct {
		val, typ string
		want     string
		wantErr  bool
	}{
		{"", TypeNumber, "", false},
		{"42", TypeNumber, "42", false},
		{"-3.14", TypeNumber, "-3.14", false},
		{"1e6", TypeNumber, "1e6", false},
		{"12; DROP", TypeNumber, "", true},
		{"2020-01-31", TypeDate, `"2020-01-31"^^<http://www.w3.org/2001/XMLSchema#date>`, false},
		{"2020-13-99x", TypeDate, "", true},
		{"anything", TypeString, "", true}, // string is not a range type
	}
	for _, c := range cases {
		got, err := RangeBound(c.val, c.typ)
		if (err != nil) != c.wantErr {
			t.Errorf("RangeBound(%q,%q) err=%v wantErr=%v", c.val, c.typ, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("RangeBound(%q,%q)=%q want %q", c.val, c.typ, got, c.want)
		}
	}
}

func TestEnumTerm(t *testing.T) {
	got, err := EnumTerm("http://ex/a", TypeIRI)
	if err != nil || got != "<http://ex/a>" {
		t.Fatalf("iri enum: got %q err %v", got, err)
	}
	got, err = EnumTerm(`a"b`, TypeString)
	if err != nil || got != `"a\"b"` {
		t.Fatalf("string enum escaping: got %q err %v", got, err)
	}
}
