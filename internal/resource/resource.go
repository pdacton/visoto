package resource

// package represents an RDF resource represented by an IRI and its associated template and data
// it takes into accout prefixes defined in the config file

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/sparql"
)

// Resource represents a RDF resource identified by an IRI

type Resource struct {
	IRI          string              // IRI of the resource
	shortIRI     string              // short IRI with prefix
	TemplateName string              // template file associated with the resource
	TemplatePath string              // full path to the template file
	Data         sparql.TemplateData // data returned by the queries for this resource (SPARQL web result format with Head, Bindings)
}

// public Functions --------------------------------

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

	// Validate the full IRI
	if _, err := url.ParseRequestURI(fullIRI); err != nil {
		return nil, fmt.Errorf("incorrect resource iri: %v", fullIRI)
	}

	log := logger.Get()
	log.Debug("created resource",
		slog.String("input", iri),
		slog.String("fullIRI", fullIRI),
		slog.String("shortIRI", shortIRI))

	return &Resource{
		IRI:      fullIRI,
		shortIRI: shortIRI,
		Data: sparql.TemplateData{
			QueryResults: make(map[string]sparql.QueryResult),
		},
	}, nil
}

func (r *Resource) GetIRI() string {
	return r.IRI
}

func (r *Resource) GetShortIRI() string {
	return r.shortIRI
}

// ResolveTemplate determines the appropriate template for this resource
// Resolution order:
// 1. Direct IRI match in templates/instances/ (tries both full IRI and short IRI)
// 2. Direct IRI match in templates/classes/ (tries both full IRI and short IRI)
// 3. RDF type match in templates/classes/ (with type priority)
// 4. Fallback to templates/pages/resource.html
func (r *Resource) ResolveTemplate(preprocessor *sparql.Preprocessor, typePriority []string, prefixes []config.Prefix) error {
	log := logger.Get()
	fullIRITemplate := normalizeToFilename(r.IRI)
	shortIRITemplate := normalizeToFilename(r.shortIRI)

	// Helper to check and set template
	tryTemplate := func(dir, template, reason string) bool {
		path := dir + template
		if templateExists(path) {
			r.TemplateName = template
			r.TemplatePath = path
			log.Debug("resolved template "+reason, slog.String("iri", r.IRI), slog.String("template", template))
			return true
		}
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
	if types, err := preprocessor.QueryTypes(r.IRI); err != nil {
		log.Warn("failed to query RDF types, using fallback template", slog.String("iri", r.IRI), slog.String("error", err.Error()))
	} else if len(types) > 0 {
		log.Debug("found RDF types for resource", slog.String("iri", r.IRI), slog.Any("types", types))
		for _, typ := range sortTypesByPriority(types, typePriority) {
			// Try shortened IRI first, then full IRI
			shortTyp := shortenIRI(typ, prefixes)
			if tryTemplate("templates/instances/", normalizeToFilename(shortTyp), "via RDF type match") ||
				tryTemplate("templates/instances/", normalizeToFilename(typ), "via RDF type match") {
				return nil
			}
		}
	}

	// 4. Fallback to default template
	r.TemplateName = "resource.html"
	r.TemplatePath = "templates/pages/resource.html"
	log.Debug("using fallback template", slog.String("iri", r.IRI), slog.String("template", r.TemplateName))
	return nil
}

// internal Helper functions ----------------------------

// normalizeToFilename converts an IRI to a URL-encoded filename
func normalizeToFilename(iri string) string {
	return url.QueryEscape(iri) + ".html"
}

// templateExists checks if a template file exists on the filesystem
func templateExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sortTypesByPriority reorders types based on priority list
// Types in priority list come first (in priority order), then remaining types
func sortTypesByPriority(types []string, priority []string) []string {
	if len(priority) == 0 {
		return types
	}

	// Create a map for quick priority lookup
	priorityMap := make(map[string]int)
	for i, p := range priority {
		priorityMap[p] = i
	}

	// Separate into prioritized and non-prioritized
	var prioritized []string
	var others []string

	for _, typ := range types {
		if _, hasPriority := priorityMap[typ]; hasPriority {
			prioritized = append(prioritized, typ)
		} else {
			others = append(others, typ)
		}
	}

	// Sort prioritized types by their priority index
	for i := 0; i < len(prioritized); i++ {
		for j := i + 1; j < len(prioritized); j++ {
			if priorityMap[prioritized[i]] > priorityMap[prioritized[j]] {
				prioritized[i], prioritized[j] = prioritized[j], prioritized[i]
			}
		}
	}

	// Combine prioritized and others
	return append(prioritized, others...)
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

// shortenIRI converts a full IRI to prefixed form if possible
func shortenIRI(fullIRI string, prefixes []config.Prefix) string {
	// Try to match against each prefix URI
	for _, p := range prefixes {
		uri := strings.Trim(p.URI, "<>")
		if localName, found := strings.CutPrefix(fullIRI, uri); found {
			return p.Name + ":" + localName
		}
	}

	// No matching prefix found, return full IRI
	return fullIRI
}
