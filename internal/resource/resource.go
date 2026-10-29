// this package is a light abstraction over a RDF resource.
// It's main function is to resolve the appropriate template for a given resource based on its IRI and RDF types, following a specific resolution order.
// It also handles both full IRIs and prefixed IRIs, allowing for flexible input formats.
// The package includes helper functions for normalizing IRIs to filenames, checking template existence, and sorting RDF types by priority.

package resource

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/sparql"
)

// Resource represents a RDF resource identified by an IRI

type Resource struct {
	IRI          string              // IRI of the resource
	ShortIRI     string              // short IRI with prefix
	TemplateName string              // template file associated with the resource
	TemplatePath string              // full path to the template file
	TemplateType string              // IRI of the class this page is rendered as (see ResolveTemplate)
	Data         parser.TemplateData // data returned by the queries for this resource (SPARQL web result format with Head, Bindings)
}

// --- public Functions --------------------------------

// create a new Resource instance after validating the provided IRI
// Handles both full IRIs (http://schema.org/Person) and prefixed IRIs (schema:Person)
func New(iri string, prefixes []config.Prefix) (*Resource, error) {
	var fullIRI, shortIRI string

	// Check if input is a prefixed IRI (contains colon but doesn't start with http:// or https://)
	if strings.Contains(iri, ":") && !strings.HasPrefix(iri, "http://") && !strings.HasPrefix(iri, "https://") {
		// It's a prefixed IRI - expand it
		fullIRI = expandPrefixedIRI(iri, prefixes)
		shortIRI = iri
	} else {
		// It's a full IRI - try to shorten it
		fullIRI = iri
		shortIRI = shortenIRI(iri, prefixes)
	}

	// Validate the full IRI. url.ParseRequestURI alone is too permissive for a
	// value that gets substituted into SPARQL as <iri>: it accepts '>' and other
	// characters that would close the term early and inject graph patterns. The
	// SPARQL IRIREF rules are the real constraint, so check those too.
	if _, err := url.ParseRequestURI(fullIRI); err != nil {
		return nil, fmt.Errorf("incorrect resource iri: %v", fullIRI)
	}
	if err := sparql.ValidateIRI(fullIRI); err != nil {
		return nil, fmt.Errorf("incorrect resource iri: %w", err)
	}

	log := logger.Get()
	log.Debug("created resource",
		slog.String("input", iri),
		slog.String("fullIRI", fullIRI),
		slog.String("shortIRI", shortIRI))

	return &Resource{
		IRI:      fullIRI,
		ShortIRI: shortIRI,
		Data: parser.TemplateData{
			QueryResults: make(map[string]sparql.QueryResult),
		},
	}, nil
}

// FetchNamedGraphs populates r.Data.NamedGraphs with the named graphs that
// contain r.IRI as subject, queried on the request's selected endpoint.
// Failures are logged and leave the list empty — the page renders without the section.
func (r *Resource) FetchNamedGraphs(ctx context.Context, preprocessor *parser.Preprocessor, prefixes []config.Prefix) {
	query, ok := namedGraphsQuery(r.IRI)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := preprocessor.ExecuteQueryWithContext(ctx, query, false, "", "")
	if err != nil {
		logger.Get().Warn("named graphs query failed",
			slog.String("iri", r.IRI),
			slog.String("error", err.Error()))
		return
	}

	for _, binding := range result.Bindings {
		g, exists := binding["g"]
		if !exists || g.Value == "" {
			continue
		}
		r.Data.NamedGraphs = append(r.Data.NamedGraphs, parser.NamedGraph{
			IRI:   g.Value,
			Short: shortenIRI(g.Value, prefixes),
		})
	}
}

// namedGraphsQuery builds the graph-membership query for an IRI. It returns
// ok=false when the IRI cannot be safely interpolated into <...>.
func namedGraphsQuery(iri string) (string, bool) {
	if iri == "" || strings.ContainsAny(iri, "<>\" \t\n\r") {
		return "", false
	}
	return fmt.Sprintf("SELECT DISTINCT ?g WHERE { GRAPH ?g { <%s> ?p ?o } } LIMIT 50", iri), true
}

// ResolveTemplate determines the appropriate template for this resource
// Resolution order:
//  1. Direct IRI match in templates/classes/ (tries both full IRI and short IRI)
//  2. Direct IRI match in templates/instances/ (tries both full IRI and short IRI)
//  3. rdf:type match in templates/instances/ (with type priority)
//  4. Usage-based role (see templateForRole): class default, property page,
//     named-graph page, else the instance default
//  5. Fallback to templates/pages/resource.html
//
// It also records TemplateType: the class this page is rendered *as*, which the
// breadcrumb names so the trail describes the page rather than guessing from the
// resource's types. A multi-typed resource has no single "the" class — the Zefix
// companies carry four — so the only non-arbitrary answer is the one that decided
// the template, which is what this function computes anyway.
//
// It stays empty for the direct-IRI matches (steps 1-2), where the template was
// chosen by the resource's own IRI and no class was involved, and the breadcrumb
// then simply omits the segment.
//
// prefixes are the active endpoint's; globalPrefixes are the [rdf] ones that
// type_priority is written against. They differ only when the endpoint
// redeclares a prefix, and then type_priority is moved into its namespace.
func (r *Resource) ResolveTemplate(preprocessor *parser.Preprocessor, typePriority []string, prefixes, globalPrefixes []config.Prefix) error {
	typePriority = localizeTypePriority(typePriority, globalPrefixes, prefixes)

	log := logger.Get()
	fullIRITemplate := normalizeToFilename(r.IRI)
	shortIRITemplate := normalizeToFilename(r.ShortIRI)

	// Helper to check and set template
	tryTemplate := func(dir, template, reason string) bool {
		path := dir + template
		if templateExists(path) {
			// Extract directory name (e.g., "classes" or "instances") for template name
			dirName := strings.TrimPrefix(strings.TrimSuffix(dir, "/"), "templates/")
			r.TemplateName = dirName + "/" + template
			r.TemplatePath = path
			log.Debug("resolved template "+reason, slog.String("iri", r.IRI), slog.String("template", r.TemplateName))
			return true
		}
		log.Debug("template not found "+reason, slog.String("iri", r.IRI), slog.String("path", path))
		return false
	}

	// 1. Check for direct IRI match in classes/
	if tryTemplate("templates/classes/", shortIRITemplate, "via direct short IRI match in classes/") ||
		tryTemplate("templates/classes/", fullIRITemplate, "via direct full IRI match in classes/") {
		return nil
	}

	// 2. Check for direct IRI match in instances/
	if tryTemplate("templates/instances/", shortIRITemplate, "via direct short IRI match in instances/") ||
		tryTemplate("templates/instances/", fullIRITemplate, "via direct full IRI match in instances/") {
		return nil
	}

	// 3. Query for RDF types and check for instance templates
	var types []string
	if t, err := preprocessor.QueryTypes(r.IRI); err != nil {
		log.Warn("failed to query RDF types, using fallback template", slog.String("iri", r.IRI), slog.String("error", err.Error()))
	} else {
		types = t
		if len(types) > 0 {
			log.Debug("found RDF types for resource", slog.String("iri", r.IRI), slog.Any("types", types))
			for _, typ := range sortTypesByPriority(types, typePriority) {
				// Try shortened IRI first, then full IRI
				shortTyp := shortenIRI(typ, prefixes)
				if tryTemplate("templates/instances/", normalizeToFilename(shortTyp), "via RDF type match") ||
					tryTemplate("templates/instances/", normalizeToFilename(typ), "via RDF type match") {
					// This is the class the page is rendered as: it beat the other
					// types to a template, so it is the one the breadcrumb names.
					// Suppressed for the class-of-a-class types, which name the kind
					// of page rather than its subject.
					if !uninformativeCrumbTypes[typ] {
						r.TemplateType = typ
					}
					return nil
				}
			}
		}
	}

	// 4. Detect the resource's role from its usage: class, property, named graph,
	// or plain instance. A failed ASK counts as false and is only logged.
	isClass := false
	for _, t := range types {
		if t == "http://www.w3.org/2000/01/rdf-schema#Class" || t == "http://www.w3.org/2002/07/owl#Class" {
			isClass = true
			break
		}
	}
	role, err := preprocessor.QueryRole(r.IRI, isClass)
	if err != nil {
		log.Warn("role detection incomplete", slog.String("iri", r.IRI), slog.String("error", err.Error()))
	}
	// No template matched, but the ranking still holds: types is already sorted by
	// sortTypesByPriority above, so the first entry is the best description of the
	// page even though nothing renders it specially. Taking it from here rather than
	// sampling the separate, unordered pageClasses query is what stops the
	// breadcrumb contradicting the resolver on the very same page. Resources with
	// no types at all (plenty of them) leave this empty and get no segment.
	//
	// Blank-node types are skipped because QueryTypes does not filter them (the
	// system-map applications each carry two genid- ones), and they would land in
	// the trail as a crumb linking nowhere.
	for _, typ := range sortTypesByPriority(types, typePriority) {
		if !isBlankNodeType(typ) && !uninformativeCrumbTypes[typ] {
			r.TemplateType = typ
			break
		}
	}

	if dir, file, roleType := templateForRole(role); tryTemplate(dir, file, "via "+file+" by usage") {
		if roleType != "" {
			r.TemplateType = roleType
		}
		return nil
	}
	if tryTemplate("templates/instances/", "default.html", "via default instance template") {
		return nil
	}

	// 5. Hard fallback to generic resource template
	r.TemplateName = "pages/resource.html"
	r.TemplatePath = "templates/pages/resource.html"
	log.Debug("using fallback template", slog.String("iri", r.IRI), slog.String("template", r.TemplateName))
	return nil
}

// ---internal Helper functions ----------------------------

// templateForRole picks the fallback template for a resource no type matched.
// Precedence is class, property, named graph, then the instance default. roleType
// is the class the page is rendered as, for the breadcrumb: rdf:Property links to
// the list of all predicates; a graph gets none, since no sd:NamedGraph class
// page has instances to show.
func templateForRole(role sparql.Role) (dir, file, roleType string) {
	switch {
	case role.IsClass:
		return "templates/classes/", "default.html", ""
	case role.IsProperty:
		return "templates/instances/", "rdf%3AProperty.html", "http://www.w3.org/1999/02/22-rdf-syntax-ns#Property"
	case role.IsGraph:
		return "templates/instances/", "sd%3ANamedGraph.html", ""
	}
	return "templates/instances/", "default.html", ""
}

// normalizeToFilename converts an IRI to a URL-encoded filename.
//
// The escaping here is load-bearing for path safety, not cosmetic. The IRI
// originates in the user-controlled ?iri= query parameter and the result is
// concatenated onto a fixed "templates/classes/" or "templates/instances/"
// prefix, so this function is the only thing standing between a request and a
// path traversal. url.QueryEscape percent-encodes the separator, turning
// "../../etc/passwd" into the harmless flat filename "..%2F..%2Fetc%2Fpasswd.html".
//
// Do NOT swap this for url.PathEscape, which deliberately leaves "/" unescaped
// and would make the traversal real. CodeQL flags the os.Stat in templateExists
// as go/path-injection (alert 2) because it does not recognise QueryEscape as a
// path sanitiser; the alert is dismissed on the strength of this invariant.
func normalizeToFilename(iri string) string {
	return url.QueryEscape(iri) + ".html"
}

// uninformativeCrumbTypes are classes that never earn a breadcrumb segment.
//
// rdfs:Class and owl:Class describe the *kind* of page rather than its subject:
// the title of such a page is already a class name, so "Home / Class /
// PostalAddress" spends a crumb restating that a class page is a class. Every
// other type says something the title does not.
var uninformativeCrumbTypes = map[string]bool{
	"http://www.w3.org/2000/01/rdf-schema#Class": true,
	"http://www.w3.org/2002/07/owl#Class":        true,
}

// isBlankNodeType reports whether a type from QueryTypes is a blank node rather
// than a real class IRI.
//
// QueryTypes returns the raw binding value and drops the SPARQL "type" field that
// would say so outright, so the test is structural: a blank node arrives without a
// scheme ("genid-af5f...", "_:b0"), an IRI always has one. Checking for an
// absolute IRI rather than matching "genid-" keeps this independent of the naming
// each triple store happens to use.
func isBlankNodeType(typ string) bool {
	u, err := url.Parse(typ)
	return err != nil || !u.IsAbs()
}

// templateExists checks if a template file exists on the filesystem
func templateExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sortTypesByPriority reorders types based on priority list
// Types in priority list come first (in priority order), then remaining types
//
// Unlisted rdfs:Class / owl:Class go LAST, after every other unlisted type. When
// either has an instance template (the generic class page), a class that also
// carries a more specific type with its own instance template would otherwise
// render as whichever the endpoint happened to return first. Nothing depends on
// those templates existing: without them the loop in ResolveTemplate simply
// finds no match and falls through to classes/default.html. Listing either in
// type_priority still places it by that index.
func sortTypesByPriority(types []string, priority []string) []string {
	// Create a map for quick priority lookup
	priorityMap := make(map[string]int)
	for i, p := range priority {
		priorityMap[p] = i
	}

	// Separate into prioritized, other and generic class-of-class types
	var prioritized []string
	var others []string
	var generic []string

	for _, typ := range types {
		if _, hasPriority := priorityMap[typ]; hasPriority {
			prioritized = append(prioritized, typ)
		} else if uninformativeCrumbTypes[typ] {
			generic = append(generic, typ)
		} else {
			others = append(others, typ)
		}
	}

	// Sort prioritized types by their priority index
	sort.Slice(prioritized, func(i, j int) bool {
		return priorityMap[prioritized[i]] < priorityMap[prioritized[j]]
	})

	return append(append(prioritized, others...), generic...)
}

// localizeTypePriority rewrites type_priority entries, written as full IRIs
// against the global prefixes, into the namespace an endpoint's own prefix
// declarations give them. With schema: redeclared as https://schema.org/,
// "http://schema.org/Person" becomes "https://schema.org/Person", so the
// configured order still applies there. Entries no prefix covers pass through.
func localizeTypePriority(priority []string, global, endpoint []config.Prefix) []string {
	out := make([]string, len(priority))
	for i, iri := range priority {
		out[i] = iri
		if short := shortenIRI(iri, global); short != iri {
			out[i] = expandPrefixedIRI(short, endpoint)
		}
	}
	return out
}

// expandPrefixedIRI expands a prefixed IRI like "schema:Person" to full IRI
func expandPrefixedIRI(prefixedIRI string, prefixes []config.Prefix) string {
	parts := strings.SplitN(prefixedIRI, ":", 2)
	if len(parts) != 2 {
		return prefixedIRI
	}

	prefix := parts[0]
	localName := parts[1]

	// Find matching prefix
	for _, p := range prefixes {
		if p.Name == prefix {
			// Remove angle brackets from URI if present
			uri := strings.Trim(p.URI, "<>")
			return uri + localName
		}
	}

	// No matching prefix found, return as-is
	return prefixedIRI
}

// shortenIRI converts a full IRI to prefixed form if possible.
//
// The LONGEST matching prefix wins, not the first one declared. Several
// configured namespaces nest inside another — meta: (https://cube.link/meta/)
// and relation: (https://cube.link/relation/) both sit under cube:
// (https://cube.link/). Taking the first match in declaration order made
// https://cube.link/meta/SharedDimension shorten to "cube:meta/SharedDimension",
// which is not a legal CURIE (a local name cannot contain "/") and, worse, is a
// different string than the "meta:SharedDimension" the same resource is linked
// as elsewhere. Template resolution derives a filename from this string, so the
// one resource resolved to two different templates depending on the link the
// user arrived through.
func shortenIRI(fullIRI string, prefixes []config.Prefix) string {
	bestName, bestLocal, bestLen := "", "", -1
	for _, p := range prefixes {
		uri := strings.Trim(p.URI, "<>")
		localName, found := strings.CutPrefix(fullIRI, uri)
		if !found || len(uri) <= bestLen {
			continue
		}
		bestName, bestLocal, bestLen = p.Name, localName, len(uri)
	}
	if bestLen >= 0 {
		return bestName + ":" + bestLocal
	}

	// No matching prefix found, return full IRI
	return fullIRI
}
