package upload

import "testing"

func TestNormalizeGraphURI(t *testing.T) {
	cases := map[string]string{
		"  urn:ogd:catalog  ": "urn:ogd:catalog",
		"<urn:ogd:catalog>":   "urn:ogd:catalog",
		"<<urn:ogd:catalog>>": "urn:ogd:catalog",
		"< urn:ogd:catalog >": "urn:ogd:catalog",
		"":                    "",
		"<":                   "<",
		"urn:a<b>c":           "urn:a<b>c",
		"https://x.org/g":     "https://x.org/g",
	}
	for in, want := range cases {
		if got := normalizeGraphURI(in); got != want {
			t.Errorf("normalizeGraphURI(%q) = %q, want %q", in, got, want)
		}
	}
}
