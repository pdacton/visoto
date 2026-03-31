package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"hutzli.org/visoto/internal/sparql"
)

// templateIncludeRe matches {{ template "name" ... }} directives in Go template syntax.
var templateIncludeRe = regexp.MustCompile(`{{\s*template\s+"([^"]+)"`)

// extractTemplateIncludes returns the deduplicated list of template names
// referenced via {{ template "name" ... }} in the given content.
func extractTemplateIncludes(content string) []string {
	matches := templateIncludeRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool, len(matches))
	var names []string
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// loadComponentQueries scans dir for component files matching the referenced
// template names and extracts their <sparql-query> elements. It skips names
// whose file does not exist (e.g. partials like "sparqlTable") and skips query
// IDs already present in existingIDs. Returns an error if two components define
// the same query ID.
func loadComponentQueries(dir string, includes []string, existingIDs map[string]bool) ([]sparql.ExtractedQuery, error) {
	// track IDs seen across components to detect cross-component duplicates
	componentIDs := make(map[string]bool)

	var result []sparql.ExtractedQuery
	for _, name := range includes {
		path := filepath.Join(dir, name+".html")
		data, err := os.ReadFile(path)
		if err != nil {
			// File doesn't exist — this include is a layout/partial, not a component. Skip.
			continue
		}

		queries, err := extractQueriesDOM(string(data))
		if err != nil {
			return nil, fmt.Errorf("component %q: %w", name, err)
		}

		for _, q := range queries {
			if existingIDs[q.ID] {
				// The page template already defines this query ID inline — skip component's version.
				continue
			}
			if componentIDs[q.ID] {
				return nil, fmt.Errorf("component %q: duplicate query ID %q already defined by another component", name, q.ID)
			}
			componentIDs[q.ID] = true
			result = append(result, q)
		}
	}
	return result, nil
}
