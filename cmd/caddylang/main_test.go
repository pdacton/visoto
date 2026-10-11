package main

import (
	"strings"
	"testing"

	"hutzli.org/visoto/internal/config"
)

// The generated maps must reproduce what the Caddyfile used to list by hand for
// the default language set.
func TestRenderDefaultLanguages(t *testing.T) {
	got := render(config.DefaultLanguages(), "en")
	for _, want := range []string{
		"map {http.request.header.Accept-Language} {al_lang} {\n" +
			"    ~^\\s*de  de\n    ~^\\s*fr  fr\n    ~^\\s*it  it\n    ~^\\s*rm  rm\n    default  en\n}",
		"    ~(?:^|;\\s*)site-lang=de(?:;|$)  de\n",
		"    ~(?:^|;\\s*)site-lang=en(?:;|$)  en\n",
		"    ~(?:^|;\\s*)site-lang=(?:;|$)  \"\"\n",
		"    default  {al_lang}\n}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render() missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "~^\\s*en ") {
		t.Errorf("the default language needs no Accept-Language line:\n%s", got)
	}
}

// A new configured language appears in both maps with no other change.
func TestRenderAddedLanguage(t *testing.T) {
	langs := append(config.DefaultLanguages(), config.Language{Code: "es", Label: "ES"})
	got := render(langs, "en")
	if !strings.Contains(got, "~^\\s*es  es\n") || !strings.Contains(got, "site-lang=es(?:;|$)  es\n") {
		t.Errorf("render() did not add es:\n%s", got)
	}
}
