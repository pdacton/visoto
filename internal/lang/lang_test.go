package lang

import (
	"net/http"
	"testing"
)

// testLangs mirrors the shipped set; labels are irrelevant to resolution but
// carried so the fixture matches what config actually hands over.
func testLangs() []Language {
	return []Language{
		{Code: "de", Label: "Deutsch"}, {Code: "fr", Label: "Français"},
		{Code: "it", Label: "Italiano"}, {Code: "en", Label: "English"},
		{Code: "rm", Label: "Rumantsch"}, {Code: "", Label: "No language"},
	}
}

func testSet() *Set {
	return New(testLangs(), "en")
}

func TestClean(t *testing.T) {
	s := testSet()
	tests := []struct {
		name   string
		raw    string
		want   string
		wantOk bool
	}{
		{"exact code", "de", "de", true},
		{"region subtag dropped", "de-CH", "de", true},
		{"underscore variant", "fr_CH", "fr", true},
		{"uppercase", "IT", "it", true},
		{"padded", "  rm  ", "rm", true},
		{"empty is a configured member", "", "", true},
		{"unconfigured", "es", "", false},
		{"garbage", "!!", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := s.Clean(tt.raw)
			if got != tt.want || ok != tt.wantOk {
				t.Errorf("Clean(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.want, tt.wantOk)
			}
		})
	}
}

func TestCleanEmptyNotConfigured(t *testing.T) {
	// A config that omits "" must not silently accept it.
	s := New([]Language{{Code: "de", Label: "Deutsch"}, {Code: "en", Label: "English"}}, "en")
	if _, ok := s.Clean(""); ok {
		t.Error("Clean(\"\") = ok, want not ok when \"\" is not configured")
	}
}

func TestFromRequest(t *testing.T) {
	s := testSet()
	tests := []struct {
		name        string
		header      map[string]string // nil value entries are set as present-but-empty
		emptyHeader bool              // set X-Site-Lang to "" (present, no value)
		cookie      string
		hasCookie   bool
		want        string
		wantVia     bool
	}{
		{
			name: "no signals at all falls back to default",
			want: "en", wantVia: false,
		},
		{
			name:   "accept-language only",
			header: map[string]string{"Accept-Language": "fr-CH,fr;q=0.9,en;q=0.8"},
			want:   "fr", wantVia: false,
		},
		{
			name:   "accept-language with no configured match",
			header: map[string]string{"Accept-Language": "es-ES,es;q=0.9"},
			want:   "en", wantVia: false,
		},
		{
			name:      "cookie beats accept-language",
			header:    map[string]string{"Accept-Language": "fr"},
			cookie:    "it",
			hasCookie: true,
			want:      "it", wantVia: false,
		},
		{
			name:      "unusable cookie falls through to accept-language",
			header:    map[string]string{"Accept-Language": "fr"},
			cookie:    "es",
			hasCookie: true,
			want:      "fr", wantVia: false,
		},
		{
			name:      "site-lang header beats cookie and accept-language",
			header:    map[string]string{"X-Site-Lang": "de", "Accept-Language": "fr"},
			cookie:    "it",
			hasCookie: true,
			want:      "de", wantVia: true,
		},
		{
			name:        "present-but-empty header selects the empty language",
			emptyHeader: true,
			header:      map[string]string{"Accept-Language": "fr"},
			want:        "", wantVia: true,
		},
		{
			name:   "header present but unusable falls through, still viaHeader",
			header: map[string]string{"X-Site-Lang": "es", "Accept-Language": "fr"},
			want:   "fr", wantVia: true,
		},
		{
			name:   "region subtag in header is normalized",
			header: map[string]string{"X-Site-Lang": "de-CH"},
			want:   "de", wantVia: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.header {
				r.Header.Set(k, v)
			}
			if tt.emptyHeader {
				r.Header[http.CanonicalHeaderKey(SiteLangHeader)] = []string{""}
			}
			if tt.hasCookie {
				r.AddCookie(&http.Cookie{Name: CookieName, Value: tt.cookie})
			}
			got, via := s.FromRequest(r)
			if got != tt.want || via != tt.wantVia {
				t.Errorf("FromRequest() = (%q, %v), want (%q, %v)", got, via, tt.want, tt.wantVia)
			}
		})
	}
}

func TestKey(t *testing.T) {
	if Key("") != "_" {
		t.Errorf("Key(\"\") = %q, want \"_\"", Key(""))
	}
	if Key("de") != "de" {
		t.Errorf("Key(\"de\") = %q, want \"de\"", Key("de"))
	}
}

func TestCodesIsACopy(t *testing.T) {
	s := testSet()
	got := s.Codes()
	got[0] = "zz"
	if s.Codes()[0] != "de" {
		t.Error("Codes() leaked the internal slice")
	}
}

// TestOptions checks the picker entries the topbar renders: configured order,
// labels straight from config, and exactly one Selected entry.
func TestOptions(t *testing.T) {
	s := testSet()
	opts := s.Options("fr")

	if len(opts) != len(testLangs()) {
		t.Fatalf("Options() returned %d entries, want %d", len(opts), len(testLangs()))
	}
	if opts[0].Code != "de" || opts[0].Label != "Deutsch" {
		t.Errorf("first option = %+v, want the configured de/Deutsch entry", opts[0])
	}
	var selected []string
	for _, o := range opts {
		if o.Selected {
			selected = append(selected, o.Code)
		}
	}
	if len(selected) != 1 || selected[0] != "fr" {
		t.Errorf("selected codes = %v, want exactly [fr]", selected)
	}

	// The no-language entry is a real option, and must be selectable like any
	// other — it is the one whose empty code is easiest to lose.
	empty := opts[len(opts)-1]
	if empty.Code != "" || empty.Label != "No language" {
		t.Errorf("last option = %+v, want the empty-code entry", empty)
	}
	for _, o := range s.Options("") {
		if o.Code == "" && !o.Selected {
			t.Error("Options(\"\") did not mark the no-language entry selected")
		}
	}
}
