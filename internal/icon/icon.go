// Package icon resolves an RDF resource to one of the SVG icons shipped in
// static/img/resource/.
//
// It is deliberately a LEAF package: it imports nothing from internal/, because
// both internal/sparql and internal/resource need it and
// internal/resource -> internal/parser -> internal/sparql already exists. Putting
// the resolver in internal/resource would close that cycle.
//
// Two naming conventions live in the icon directory:
//
//	Canton.svg            a real icon for the class Canton
//	DefinedTerm.fallback.svg   a weaker, more generic icon for DefinedTerm
//
// The ".fallback" marker exists because LINDAS resources routinely carry several
// classes, and a generic one (schema:DefinedTerm) must never win over a specific
// one (schch:Canton) just by being listed first. Resolve therefore scans ALL
// types for an exact match before it settles for any fallback.
package icon

import (
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
)

// BasePath is the URL prefix every resolved icon path carries.
//
// It is part of the contract with the frontend rather than an implementation
// detail: static/js/bookmarks.js finds the icon of a dragged resource by looking
// for an <img src> containing "/img/resource/", so a rendered icon must remain a
// real <img> pointing here.
const BasePath = "/static/img/resource/"

const (
	svgSuffix      = ".svg"
	fallbackSuffix = ".fallback.svg"
	// fallbackKey is appended to a name in the map returned by Names, so a caller
	// holding only that map can tell the two conventions apart.
	fallbackKey = ".fallback"
)

// cache holds the set of available icon names (without their .svg extension).
type cache struct {
	icons     map[string]bool
	fallbacks map[string]bool
	mu        sync.RWMutex
}

var (
	globalCache *cache
	once        sync.Once
)

// Init scans dir and builds the icon cache. Safe to call more than once: the
// allocation happens once, the scan re-runs and replaces the contents.
func Init(dir string, log *slog.Logger) error {
	once.Do(func() {
		globalCache = &cache{
			icons:     make(map[string]bool),
			fallbacks: make(map[string]bool),
		}
	})

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	icons := make(map[string]bool)
	fallbacks := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// .fallback.svg must be tested first — it also ends in .svg, and cutting
		// only the shorter suffix would register it as an icon named "X.fallback".
		if base, ok := strings.CutSuffix(name, fallbackSuffix); ok {
			fallbacks[base] = true
		} else if base, ok := strings.CutSuffix(name, svgSuffix); ok {
			icons[base] = true
		}
	}

	globalCache.mu.Lock()
	globalCache.icons = icons
	globalCache.fallbacks = fallbacks
	globalCache.mu.Unlock()

	if log != nil {
		log.Info("icon cache initialized",
			slog.Int("count", len(icons)),
			slog.Int("fallback_count", len(fallbacks)))
	}
	return nil
}

// Names returns every available icon name, for the JS islands that resolve icons
// client-side (the Graph Explorer partials, which get their types from Graph
// Explorer's own data provider and so cannot use Resolve).
//
// Regular icons are keyed by bare class name ("Canton"); fallback icons carry a
// ".fallback" suffix ("DefinedTerm.fallback") so a caller can rebuild the file
// name by appending ".svg".
func Names() map[string]bool {
	if globalCache == nil {
		return map[string]bool{}
	}
	globalCache.mu.RLock()
	defer globalCache.mu.RUnlock()

	out := make(map[string]bool, len(globalCache.icons)+len(globalCache.fallbacks))
	for k := range globalCache.icons {
		out[k] = true
	}
	for k := range globalCache.fallbacks {
		out[k+fallbackKey] = true
	}
	return out
}

// Resolve returns the icon path for a resource, or "" when nothing matches.
//
// Order:
//
//  1. the resource's own local name          (a class IRI: schch:Canton -> Canton.svg)
//  2. the resource's own local name, fallback
//  3. any type's local name                  (an instance: .../canton/1 typed schch:Canton)
//  4. any type's local name, fallback
//
// Steps 3 and 4 are two separate passes on purpose: with several types, an exact
// match on the last one must still beat a fallback match on the first.
//
// The empty return is deliberate — callers differ on what a miss means. Table
// cells render no icon at all, the resource page header falls back to
// default.svg, and the graphs to defaultClass.svg.
func Resolve(iri string, types []string) string {
	if name := LocalName(iri); name != "" {
		if hasIcon(name) {
			return BasePath + name + svgSuffix
		}
		if hasFallback(name) {
			return BasePath + name + fallbackSuffix
		}
	}

	for _, t := range types {
		if name := LocalName(t); name != "" && hasIcon(name) {
			return BasePath + name + svgSuffix
		}
	}
	for _, t := range types {
		if name := LocalName(t); name != "" && hasFallback(name) {
			return BasePath + name + fallbackSuffix
		}
	}
	return ""
}

// LocalName reduces an IRI to the name an icon file would be called after.
//
// This is the single definition of that rule. It previously existed in four
// slightly different copies (two in Go, two in JS) which disagreed about
// percent-encoding and prefixed forms; static/js/visoto-icons.js is the JS
// mirror of this function and must be kept in step with it.
//
//	https://schema.ld.admin.ch/Canton              -> Canton
//	http://www.w3.org/2004/02/skos/core#Concept    -> Concept
//	schema%3APerson                                -> Person
//	skos:Concept                                   -> Concept
func LocalName(iri string) string {
	if iri == "" {
		return ""
	}
	// Percent-decoding first: a prefixed IRI arriving from a URL ("schema%3APerson")
	// hides its colon until it is decoded. Undecodable input is used as-is rather
	// than discarded — it may still have a usable last segment.
	//
	// PathUnescape, not QueryUnescape: the latter also turns "+" into a space,
	// which would corrupt any IRI that legitimately contains a plus.
	if decoded, err := url.PathUnescape(iri); err == nil {
		iri = decoded
	}
	iri = strings.TrimRight(iri, "/")

	if idx := strings.LastIndex(iri, "#"); idx != -1 {
		iri = iri[idx+1:]
	} else if idx := strings.LastIndex(iri, "/"); idx != -1 {
		iri = iri[idx+1:]
	}
	// A prefixed form ("skos:Concept") only reaches here when there was no slash
	// or fragment to cut, since a full IRI's scheme colon is left of both.
	if idx := strings.LastIndex(iri, ":"); idx != -1 {
		iri = iri[idx+1:]
	}
	return iri
}

func hasIcon(name string) bool {
	if globalCache == nil {
		return false
	}
	globalCache.mu.RLock()
	defer globalCache.mu.RUnlock()
	return globalCache.icons[name]
}

func hasFallback(name string) bool {
	if globalCache == nil {
		return false
	}
	globalCache.mu.RLock()
	defer globalCache.mu.RUnlock()
	return globalCache.fallbacks[name]
}
