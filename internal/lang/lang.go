// Package lang resolves the site UI language for a request.
//
// The language set is configuration, not code (see [application] languages in
// visoto.config). A Set is built once at startup and answers two questions:
// whether a raw tag maps onto a configured code (Clean), and which code a given
// request should be rendered in (FromRequest).
//
// Resolution order — X-Site-Lang header, then the site-lang cookie, then
// Accept-Language, then the configured default — mirrors the production request
// path. In production Caddy folds the cookie and Accept-Language into a single
// normalized X-Site-Lang header before the shared cache, so the header branch is
// the only one that runs; the cookie and Accept-Language branches are what make
// the same code work in dev, where there is no Caddy in front.
package lang

import (
	"net/http"
	"strings"

	"golang.org/x/text/language"
)

// SiteLangHeader is the normalized language header Caddy sets in production.
// Its presence — not its value — decides which header the response Varies on,
// so callers must test for the key, never for a non-empty value.
const SiteLangHeader = "X-Site-Lang"

// CookieName is the client-managed preference cookie written by the topbar
// picker (static/js/language-switcher.js). The server never sets it.
const CookieName = "site-lang"

// Language is one configured UI language: the code carried on the wire, the
// compact label the closed picker shows, and the full name its menu shows.
// Mirrors config.Language, so this package stays free of a dependency on the
// config loader. Name is optional; Options falls back to Label when it is empty.
type Language struct {
	Code  string
	Label string
	Name  string
}

// Option is one rendered entry of the language picker.
type Option struct {
	Code     string
	Label    string // compact form, shown in the closed topbar control
	Name     string // full language name, shown in the dropdown menu
	Selected bool   // true for the language the page is being rendered in
}

// Set is the configured language set. Build it once at startup with New and
// treat it as immutable afterwards; all methods are safe for concurrent use.
type Set struct {
	langs   []Language
	def     string
	known   map[string]bool
	matcher language.Matcher
	matched []string // parallel to the tags handed to the matcher
}

// New builds a Set from the configured languages and default code. Entries are
// taken as given (config validation has already checked their shape); an
// empty-string code is legal and is carried through as the "no language" choice.
func New(langs []Language, def string) *Set {
	s := &Set{
		langs: append([]Language(nil), langs...),
		def:   def,
		known: make(map[string]bool, len(langs)),
	}

	// The matcher only handles real languages, so "" is tracked separately and
	// never offered as an Accept-Language match.
	tags := make([]language.Tag, 0, len(langs))
	for _, l := range langs {
		s.known[l.Code] = true
		if l.Code == "" {
			continue
		}
		tag, err := language.Parse(l.Code)
		if err != nil {
			continue
		}
		tags = append(tags, tag)
		s.matched = append(s.matched, l.Code)
	}
	if len(tags) > 0 {
		s.matcher = language.NewMatcher(tags)
	}
	return s
}

// Options returns the picker entries for a page rendered in `active`, with the
// matching one marked Selected. The server must render that selection rather
// than leaving it to JS — a client-side-only selection is what previously made
// shared links open on the wrong endpoint.
func (s *Set) Options(active string) []Option {
	opts := make([]Option, 0, len(s.langs))
	for _, l := range s.langs {
		name := l.Name
		if name == "" {
			name = l.Label
		}
		opts = append(opts, Option{Code: l.Code, Label: l.Label, Name: name, Selected: l.Code == active})
	}
	return opts
}

// Codes returns the configured codes in their configured order (the order the
// topbar picker lists them). The returned slice is a copy.
func (s *Set) Codes() []string {
	codes := make([]string, 0, len(s.langs))
	for _, l := range s.langs {
		codes = append(codes, l.Code)
	}
	return codes
}

// Default returns the configured fallback code.
func (s *Set) Default() string { return s.def }

// Has reports whether code is a configured member. Note that "" may be a member.
func (s *Set) Has(code string) bool { return s.known[code] }

// Clean normalizes a raw language tag onto a configured code: it lowercases,
// trims, and drops any region subtag ("de-CH" -> "de"), then checks membership.
// The empty string maps onto the empty-string code when that is configured.
// Returns ok=false for anything the config does not list, so an unconfigured
// value can never reach the renderer.
func (s *Set) Clean(raw string) (string, bool) {
	code := strings.ToLower(strings.TrimSpace(raw))
	// RDF language tags and our catalogs are keyed on base codes, so a "de-CH"
	// header must compare as "de" or it would fall through to the default.
	if i := strings.IndexAny(code, "-_"); i >= 0 {
		code = code[:i]
	}
	if s.known[code] {
		return code, true
	}
	return "", false
}

// FromRequest resolves the language for r.
//
// viaHeader reports whether the request carried an X-Site-Lang header at all —
// that is, whether a normalizing cache sits in front of us. Callers use it to
// choose the Vary header, so it reflects the header's *presence*, independent of
// whether its value was usable.
func (s *Set) FromRequest(r *http.Request) (code string, viaHeader bool) {
	// Presence, not value: http.Header.Get cannot tell an absent header from the
	// empty-valued one Caddy sets for the "no language" choice, and the two mean
	// different things here.
	if raw, ok := r.Header[http.CanonicalHeaderKey(SiteLangHeader)]; ok {
		viaHeader = true
		v := ""
		if len(raw) > 0 {
			v = raw[0]
		}
		if c, ok := s.Clean(v); ok {
			return c, true
		}
		// Header present but unusable (a drifted Caddyfile, or a code removed
		// from the config): fall through to the other signals rather than
		// rendering a language we have no catalog for.
	}

	if ck, err := r.Cookie(CookieName); err == nil {
		if c, ok := s.Clean(ck.Value); ok {
			return c, viaHeader
		}
	}

	if c, ok := s.fromAcceptLanguage(r.Header.Get("Accept-Language")); ok {
		return c, viaHeader
	}

	return s.def, viaHeader
}

// fromAcceptLanguage picks the best configured code for an Accept-Language
// header using x/text's CLDR-aware matcher, which understands quality values,
// region fallbacks, and mutual intelligibility far better than a hand-rolled
// parse. Returns ok=false when the header is absent or matches nothing.
func (s *Set) fromAcceptLanguage(header string) (string, bool) {
	if header == "" || s.matcher == nil {
		return "", false
	}
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(tags) == 0 {
		return "", false
	}
	_, idx, conf := s.matcher.Match(tags...)
	// language.No means the matcher fell back to its first tag rather than
	// finding a real match — treat that as no preference so the configured
	// default wins instead of whichever code happens to be listed first.
	if conf == language.No || idx < 0 || idx >= len(s.matched) {
		return "", false
	}
	return s.matched[idx], true
}

// Key returns a map-safe key for a code. The empty-string language needs a
// non-empty stand-in wherever a code becomes part of a composite identifier —
// template names, catalog lookups — so it stays distinguishable from "absent".
func Key(code string) string {
	if code == "" {
		return "_"
	}
	return code
}
