// Command caddylang writes the Caddyfile language maps from visoto.config.
//
// Caddy normalizes the request language into X-Site-Lang before its cache runs,
// and that header is part of the cache key. Caddy cannot read visoto.config, so
// the closed set of codes it accepts is generated from `languages` and
// `default_language` at deploy time (deploy.sh) instead of being kept in sync by
// hand. The set stays closed on purpose: whatever the maps output becomes a
// cache variant, so a regexp capture would let any client mint new ones.
//
//	go run ./cmd/caddylang [visoto.config] > site-lang.caddy
package main

import (
	"fmt"
	"os"
	"strings"

	"hutzli.org/visoto/internal/config"
)

func main() {
	path := "visoto.config"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "caddylang:", err)
		os.Exit(1)
	}
	fmt.Print(render(cfg.Application.Languages, cfg.Application.DefaultLanguage))
}

// quote renders a map output value: "" for the no-language code.
func quote(code string) string {
	if code == "" {
		return `""`
	}
	return code
}

// render returns the two maps the Caddyfile imports. Codes come from a
// validated config (`^[a-z]{2,3}$` or ""), so they are safe inside a regexp.
func render(languages []config.Language, defaultLang string) string {
	var b strings.Builder
	b.WriteString("# GENERATED from visoto.config by `go run ./cmd/caddylang` (deploy.sh).\n")
	b.WriteString("# Do not edit: change `languages` / `default_language` in visoto.config.\n\n")

	// Accept-Language: only a browser's first preference counts, and only for a
	// configured code; everything else is the default language. "" is never
	// expressed through Accept-Language, only by the cookie.
	b.WriteString("map {http.request.header.Accept-Language} {al_lang} {\n")
	for _, l := range languages {
		if l.Code == "" || l.Code == defaultLang {
			continue
		}
		fmt.Fprintf(&b, "    ~^\\s*%s  %s\n", l.Code, l.Code)
	}
	fmt.Fprintf(&b, "    default  %s\n}\n\n", quote(defaultLang))

	// The site-lang cookie is the explicit choice and outranks Accept-Language.
	// An empty cookie is a real choice ("no language"), distinct from none.
	b.WriteString("map {http.request.header.Cookie} {site_lang} {\n")
	for _, l := range languages {
		fmt.Fprintf(&b, "    ~(?:^|;\\s*)site-lang=%s(?:;|$)  %s\n", l.Code, quote(l.Code))
	}
	b.WriteString("    default  {al_lang}\n}\n")
	return b.String()
}
