// Package icon resolves an RDF resource to one of the SVG icons shipped in
// static/img/resource/.
//
// It is deliberately a LEAF package: it imports nothing from internal/, because
// both internal/sparql and internal/resource need it and
// internal/resource -> internal/parser -> internal/sparql already exists. Putting
// the resolver in internal/resource would close that cycle. (That is also why
// Init takes the prefix table as a plain map rather than []config.Prefix.)
//
// Two naming conventions live in the icon directory:
//
//	Canton.svg                 a real icon for the class Canton
//	DefinedTerm.fallback.svg   a weaker, more generic icon for DefinedTerm
//
// The ".fallback" marker exists because LINDAS resources routinely carry several
// classes, and a generic one (schema:DefinedTerm) must never win over a specific
// one (schch:Canton) just by being listed first. Resolve therefore scans ALL
// types for an exact match before it settles for any fallback.
//
// Icons in the root match on local name alone, so one Property.svg serves every
// ontology's Property. A subfolder named after a configured prefix scopes its
// icons to that namespace:
//
//	rdf/Property.svg      only http://www.w3.org/1999/02/22-rdf-syntax-ns#Property
//	Property.svg          any other ...Property
//
// A namespace match beats a root match, but never changes the exact-before-
// fallback order. Folders starting with "_" are not icon folders and are skipped.
package icon

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
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
	// fallbackKey is appended to a key in the map returned by Names, so a caller
	// holding only that map can tell the two conventions apart.
	fallbackKey = ".fallback"
)

// cache maps a lookup key to an icon file path relative to BasePath.
//
// Root icons are keyed by bare local name ("Canton"). Namespaced icons are keyed
// twice, by full IRI and by CURIE, so a caller holding either form finds them
// with one lookup. The key spaces cannot collide: a local name never contains a
// colon, while every IRI and CURIE does.
type cache struct {
	icons     map[string]string
	fallbacks map[string]string
	mu        sync.RWMutex
}

var globalCache = &cache{}

// Init scans dir and replaces the icon cache. namespaces maps prefix name to
// namespace IRI (the global [rdf] prefixes, never an endpoint's overrides, so an
// endpoint redeclaring a prefix cannot reassign icons).
//
// A subfolder whose name is not a key of namespaces is an error: an icon folder
// that silently matched nothing would look like a working configuration.
func Init(dir string, namespaces map[string]string, log *slog.Logger) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	icons := make(map[string]string)
	fallbacks := make(map[string]string)
	var rootCount, nsCount int
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() {
			if addIcon(icons, fallbacks, name, "", nil) {
				rootCount++
			}
			continue
		}
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		ns, ok := namespaces[name]
		if !ok || ns == "" {
			return fmt.Errorf("icon folder %q: no prefix %q declared in [rdf] prefixes", filepath.Join(dir, name), name)
		}
		sub, err := os.ReadDir(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		for _, f := range sub {
			if f.IsDir() {
				continue
			}
			if addIcon(icons, fallbacks, f.Name(), name, []string{ns, name + ":"}) {
				nsCount++
			}
		}
	}

	globalCache.mu.Lock()
	globalCache.icons = icons
	globalCache.fallbacks = fallbacks
	globalCache.mu.Unlock()

	if log != nil {
		log.Info("icon cache initialized",
			slog.Int("count", rootCount),
			slog.Int("namespaced_count", nsCount))
	}
	return nil
}

// addIcon registers file under its local name, or, for a namespaced folder,
// under each of keyPrefixes + local name. It reports whether file was an icon.
func addIcon(icons, fallbacks map[string]string, file, folder string, keyPrefixes []string) bool {
	target := icons
	// .fallback.svg must be tested first — it also ends in .svg, and cutting
	// only the shorter suffix would register it as an icon named "X.fallback".
	base, ok := strings.CutSuffix(file, fallbackSuffix)
	if ok {
		target = fallbacks
	} else if base, ok = strings.CutSuffix(file, svgSuffix); !ok {
		return false
	}
	rel := file
	if folder != "" {
		rel = folder + "/" + file
	}
	if keyPrefixes == nil {
		target[base] = rel
		return true
	}
	for _, p := range keyPrefixes {
		target[p+base] = rel
	}
	return true
}

// Names returns every lookup key with its icon file path relative to BasePath,
// for the JS islands that resolve icons client-side (the Graph Explorer
// partials, which get their types from Graph Explorer's own data provider and so
// cannot use Resolve).
//
// Keys are those of the cache — local name, full IRI, CURIE — with fallback
// icons carrying a ".fallback" suffix ("DefinedTerm.fallback").
func Names() map[string]string {
	globalCache.mu.RLock()
	defer globalCache.mu.RUnlock()

	out := make(map[string]string, len(globalCache.icons)+len(globalCache.fallbacks))
	for k, v := range globalCache.icons {
		out[k] = v
	}
	for k, v := range globalCache.fallbacks {
		out[k+fallbackKey] = v
	}
	return out
}

// Resolve returns the icon path for a resource, or "" when nothing matches.
//
// Order:
//
//  1. the resource's own IRI                 (a class IRI: schch:Canton -> Canton.svg)
//  2. the resource's own IRI, fallback
//  3. any type                               (an instance: .../canton/1 typed schch:Canton)
//  4. any type, fallback
//
// Within each step a namespaced icon (rdf/Property.svg) beats a root icon
// (Property.svg). Steps 3 and 4 are two separate passes on purpose: with several
// types, an exact match on the last one must still beat a fallback match on the
// first.
//
// The empty return is deliberate — callers differ on what a miss means. Table
// cells render no icon at all, the resource page header falls back to
// default.svg, and the graphs to defaultClass.svg.
func Resolve(iri string, types []string) string {
	globalCache.mu.RLock()
	defer globalCache.mu.RUnlock()
	c := globalCache

	if p := lookup(iri, c.icons); p != "" {
		return p
	}
	if p := lookup(iri, c.fallbacks); p != "" {
		return p
	}
	for _, t := range types {
		if p := lookup(t, c.icons); p != "" {
			return p
		}
	}
	for _, t := range types {
		if p := lookup(t, c.fallbacks); p != "" {
			return p
		}
	}
	return ""
}

// lookup tries the namespaced key (the IRI or CURIE itself) before the bare
// local name. The caller holds the read lock.
func lookup(iri string, m map[string]string) string {
	if iri == "" {
		return ""
	}
	if rel, ok := m[decode(iri)]; ok {
		return BasePath + rel
	}
	if name := LocalName(iri); name != "" {
		if rel, ok := m[name]; ok {
			return BasePath + rel
		}
	}
	return ""
}

// decode percent-decodes an IRI or CURIE ("schema%3APerson" -> "schema:Person"),
// using the input as-is when it does not decode.
//
// PathUnescape, not QueryUnescape: the latter also turns "+" into a space,
// which would corrupt any IRI that legitimately contains a plus.
func decode(iri string) string {
	if decoded, err := url.PathUnescape(iri); err == nil {
		return decoded
	}
	return iri
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
	iri = strings.TrimRight(decode(iri), "/")

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
