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
//
// This package is the one definition of the rule, of the default icons and of
// the icon set. The browser gets the defaults with every page (Config) and the
// set on demand (Manifest, /api/icons.json); static/js/visoto-icons.js applies
// the rule to them.
package icon

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/url"
	"os"
	"sort"
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

// Kind says what a miss means for the caller: which default icon, if any, stands
// in when nothing matches.
type Kind int

const (
	// Cell is a table cell or tree node: a miss renders no icon at all.
	Cell Kind = iota
	// Page is the resource page header, which always has a slot to fill.
	Page
	// Class is a class shown on its own: the sidebar menus, the graph class tree
	// and a graph box resolved from its types.
	Class
	// Instance is a resource whose class is unknown (a bookmark without a stored
	// icon).
	Instance
)

// defaults is the stand-in per Kind. Cell has none on purpose.
var defaults = map[Kind]string{
	Page:     BasePath + "default.svg",
	Class:    BasePath + "defaultClass.svg",
	Instance: BasePath + "defaultInstance.svg",
}

// Default returns the icon a miss of this kind falls back to, or "" for Cell.
func (k Kind) Default() string { return defaults[k] }

// Subject is what the resolver knows about one resource.
type Subject struct {
	IRI   string
	Types []string // rdf:type values, in endpoint order (order is only a tie-break)
	// Override is the resource's own icon from the data: the value of the "icon"
	// magic property (e.g. schema:thumbnail), "" if none. See Resolver.override.
	Override string
}

// Resolver holds the set of available icon names and applies the matching rule.
type Resolver struct {
	mu        sync.RWMutex
	icons     map[string]bool // bare name -> present (Canton.svg)
	fallbacks map[string]bool // bare name -> present (DefinedTerm.fallback.svg)
	version   string          // content hash of the name set, for cache-busting URLs
}

// std is the process-wide resolver the package-level functions use. The icon
// directory is a startup asset (like templates/), so one shared set is right.
var std = &Resolver{}

// Init scans dir into the process-wide resolver. See Resolver.Load.
func Init(dir string, log *slog.Logger) error { return std.Load(dir, log) }

// Resolve applies the process-wide resolver. See Resolver.Resolve.
func Resolve(s Subject, k Kind) string { return std.Resolve(s, k) }

// Current returns the process-wide resolver, for the manifest route.
func Current() *Resolver { return std }

// Load scans dir and replaces the icon set. Safe to call more than once: a
// re-scan does not keep the previous directory's names.
func (r *Resolver) Load(dir string, log *slog.Logger) error {
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

	r.mu.Lock()
	r.icons = icons
	r.fallbacks = fallbacks
	r.version = hashNames(icons, fallbacks)
	r.mu.Unlock()

	if log != nil {
		log.Info("icon cache initialized",
			slog.Int("count", len(icons)),
			slog.Int("fallback_count", len(fallbacks)))
	}
	return nil
}

// Resolve returns the icon path for a resource, or k's default when nothing
// matches (which for Cell is "").
//
// Order:
//
//  0. the resource's Override, when it is usable (see override)
//  1. the resource's own local name          (a class IRI: schch:Canton -> Canton.svg)
//  2. the resource's own local name, fallback
//  3. any type's local name                  (an instance: .../canton/1 typed schch:Canton)
//  4. any type's local name, fallback
//  5. k.Default()
//
// Steps 3 and 4 are two separate passes on purpose: with several types, an exact
// match on the last one must still beat a fallback match on the first.
func (r *Resolver) Resolve(s Subject, k Kind) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if path := r.override(s.Override); path != "" {
		return path
	}

	if name := LocalName(s.IRI); name != "" {
		if r.icons[name] {
			return BasePath + name + svgSuffix
		}
		if r.fallbacks[name] {
			return BasePath + name + fallbackSuffix
		}
	}

	for _, t := range s.Types {
		if name := LocalName(t); name != "" && r.icons[name] {
			return BasePath + name + svgSuffix
		}
	}
	for _, t := range s.Types {
		if name := LocalName(t); name != "" && r.fallbacks[name] {
			return BasePath + name + fallbackSuffix
		}
	}
	return k.Default()
}

// override turns an icon value from the data into a path, or "" when it is not
// usable — in which case resolution carries on as if there were none.
//
// Accepted forms:
//
//	https://img.example/x.svg     an absolute https URL
//	/static/img/resource/x.svg    a site-relative path
//	Canton | Canton.svg           the name of an icon in the set (Canton.svg, or
//	                              a fallback icon: DefinedTerm.fallback)
//
// Anything else — another scheme (javascript:, data:), a protocol-relative
// "//host" URL, a name not in the set — is refused, so a value from an endpoint
// can never put an arbitrary scheme into an <img src>. Plain http:// is refused
// too: the site CSP (Caddyfile, img-src 'self' data: https:) would block it, and
// a refused value falls back to the type's icon rather than a broken image.
// Caller must hold r.mu.
func (r *Resolver) override(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return ""
	case strings.HasPrefix(value, "https://"):
		if u, err := url.Parse(value); err == nil && u.Host != "" {
			return value
		}
		return ""
	case strings.HasPrefix(value, "//"):
		return ""
	case strings.HasPrefix(value, "/"):
		return value
	}
	name := strings.TrimSuffix(value, svgSuffix)
	if base, ok := strings.CutSuffix(name, fallbackKey); ok {
		if r.fallbacks[base] {
			return BasePath + base + fallbackSuffix
		}
		return ""
	}
	if r.icons[name] {
		return BasePath + name + svgSuffix
	}
	return ""
}

// Names returns every available icon name. Regular icons are keyed by bare
// class name ("Canton"); fallback icons carry a ".fallback" suffix
// ("DefinedTerm.fallback") so a caller can rebuild the file name by appending
// ".svg".
func (r *Resolver) Names() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]bool, len(r.icons)+len(r.fallbacks))
	for k := range r.icons {
		out[k] = true
	}
	for k := range r.fallbacks {
		out[k+fallbackKey] = true
	}
	return out
}

// Version identifies the current icon set. It changes exactly when the set does,
// so a URL carrying it can be cached as immutable.
func (r *Resolver) Version() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.version
}

// ManifestPath is the route serving Manifest.
const ManifestPath = "/api/icons.json"

// ManifestURL is ManifestPath tagged with the icon set's Version. The response is
// a pure function of the set, so the version in the URL lets it be cached as
// immutable: a deploy that adds an icon changes the URL rather than waiting out
// a stale entry.
func ManifestURL() string { return ManifestPath + "?v=" + std.Version() }

// Manifest is the icon set as browser-side resolution reads it, served once per
// set as /api/icons.json instead of being inlined into every graph embed. Only
// the Graph Explorer embeds need it; everything else resolves server-side.
type Manifest struct {
	Names map[string]bool `json:"names"` // see Names
}

// Manifest returns the browser manifest for the current icon set.
func (r *Resolver) Manifest() Manifest { return Manifest{Names: r.Names()} }

// PageConfig is the small, set-independent part of the browser contract,
// rendered into every page as the #vs-icons island (templates/layout/base.html):
// where icons live, the default per kind, and where to fetch the Manifest. It is
// what lets JS that never resolves anything (bookmarks.js) still use the right
// defaults without a fetch.
type PageConfig struct {
	ManifestURL string            `json:"manifestUrl"`
	Base        string            `json:"base"`
	Defaults    map[string]string `json:"defaults"` // "page" | "class" | "instance" -> path
	// Properties are the alternatives of the "icon" magic property, in priority
	// order and as written (CURIEs or <IRIs>), for the graph embeds that look a
	// resource's own icon up themselves. Empty when none is configured.
	Properties []string `json:"properties"`
}

// Config returns the PageConfig for the process-wide resolver. iconPath is the
// "icon" magic property ("schema:thumbnail|foaf:thumbnail"), "" if unset.
func Config(iconPath string) PageConfig {
	props := []string{}
	for _, alt := range strings.Split(iconPath, "|") {
		if alt = strings.TrimSpace(alt); alt != "" {
			props = append(props, alt)
		}
	}
	return PageConfig{
		Properties:  props,
		ManifestURL: ManifestURL(),
		Base:        BasePath,
		Defaults: map[string]string{
			"page":     Page.Default(),
			"class":    Class.Default(),
			"instance": Instance.Default(),
		},
	}
}

// hashNames is a short, order-independent content hash of the icon set.
func hashNames(icons, fallbacks map[string]bool) string {
	names := make([]string, 0, len(icons)+len(fallbacks))
	for k := range icons {
		names = append(names, k)
	}
	for k := range fallbacks {
		names = append(names, k+fallbackKey)
	}
	sort.Strings(names)
	sum := sha256.Sum256([]byte(strings.Join(names, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}

// LocalName reduces an IRI to the name an icon file would be called after.
//
// This is the single definition of that rule; static/js/visoto-icons.js is the
// JS mirror of this function and must be kept in step with it.
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
