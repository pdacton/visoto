package sparql

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/logger"
)

// labelCacheEntry stores IRI→label mappings with expiration
type labelCacheEntry struct {
	label      string
	expiration time.Time
}

var (
	labelCache      = sync.Map{} // map[string]labelCacheEntry
	cacheTTL        = 1 * time.Hour
	cleanupInterval = 15 * time.Minute
	cleanupOnce     sync.Once
)

// initLabelCache starts the background cleanup goroutine (called once)
func initLabelCache() {
	cleanupOnce.Do(func() {
		go labelCacheCleanup()
	})
}

// labelCacheCleanup runs periodically to remove expired entries
func labelCacheCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		labelCache.Range(func(key, value interface{}) bool {
			if entry, ok := value.(labelCacheEntry); ok {
				if now.After(entry.expiration) {
					labelCache.Delete(key)
				}
			}
			return true
		})
	}
}

// getCachedLabel retrieves label from cache if not expired
func getCachedLabel(iri string) (string, bool) {
	if val, ok := labelCache.Load(iri); ok {
		if entry, ok := val.(labelCacheEntry); ok {
			if time.Now().Before(entry.expiration) {
				return entry.label, true
			}
			labelCache.Delete(iri) // Expired, remove it
		}
	}
	return "", false
}

// setCachedLabel stores label in cache with TTL
func setCachedLabel(iri, label string) {
	labelCache.Store(iri, labelCacheEntry{
		label:      label,
		expiration: time.Now().Add(cacheTTL),
	})
}

// parseAcceptLanguage extracts language codes from Accept-Language header
// Returns ordered list of language codes (e.g., ["en-US", "en", "de"])
func parseAcceptLanguage(header string) []string {
	if header == "" {
		return []string{"en"} // Default fallback
	}

	type langWeight struct {
		lang   string
		weight float64
	}

	// Parse "en-US,en;q=0.9,de;q=0.8" format
	parts := strings.Split(header, ",")
	langs := make([]langWeight, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Split language from quality value
		langParts := strings.Split(part, ";")
		lang := strings.TrimSpace(langParts[0])
		weight := 1.0

		// Parse quality value if present
		if len(langParts) > 1 {
			qPart := strings.TrimSpace(langParts[1])
			if strings.HasPrefix(qPart, "q=") {
				fmt.Sscanf(qPart, "q=%f", &weight)
			}
		}

		langs = append(langs, langWeight{lang: lang, weight: weight})
	}

	// Sort by weight (descending)
	sort.Slice(langs, func(i, j int) bool {
		return langs[i].weight > langs[j].weight
	})

	// Extract language codes
	result := make([]string, 0, len(langs))
	for _, lw := range langs {
		result = append(result, lw.lang)

		// Also add base language if specific variant present (en-US → en)
		if strings.Contains(lw.lang, "-") {
			base := strings.Split(lw.lang, "-")[0]
			// Avoid duplicates
			found := false
			for _, existing := range result {
				if existing == base {
					found = true
					break
				}
			}
			if !found {
				result = append(result, base)
			}
		}
	}

	return result
}

// extractIRIs collects all unique IRI values from a QueryResult
func extractIRIs(result QueryResult) []string {
	iriSet := make(map[string]bool)

	for _, binding := range result.Bindings {
		for _, value := range binding {
			if value.Type == "uri" {
				iriSet[value.Value] = true
			}
		}
	}

	// Convert to slice
	iris := make([]string, 0, len(iriSet))
	for iri := range iriSet {
		iris = append(iris, iri)
	}

	return iris
}

// buildLabelQuery constructs SPARQL query to fetch labels for given IRIs
// Checks rdfs:label, skos:prefLabel, schema:name, dc:title (in priority order)
// Filters by language preferences
func buildLabelQuery(iris []string, languages []string) string {
	if len(iris) == 0 {
		return ""
	}

	// Build VALUES clause
	var valuesClause strings.Builder
	valuesClause.WriteString("VALUES ?iri { ")
	for _, iri := range iris {
		fmt.Fprintf(&valuesClause, "<%s> ", iri)
	}
	valuesClause.WriteString("}")

	// Build FILTER clause for languages
	var langFilter string
	if len(languages) > 0 {
		langParts := make([]string, len(languages))
		for i, lang := range languages {
			langParts[i] = fmt.Sprintf("lang(?labelRaw) = '%s'", strings.ToLower(lang))
		}
		langFilter = fmt.Sprintf("FILTER(%s)", strings.Join(langParts, " || "))
	}

	query := fmt.Sprintf(`
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
PREFIX schema: <http://schema.org/>
PREFIX dc: <http://purl.org/dc/terms/>

SELECT ?iri ?label WHERE {
  %s

  {
    ?iri rdfs:label ?labelRaw .
    BIND(1 AS ?priority)
  } UNION {
    ?iri skos:prefLabel ?labelRaw .
    BIND(2 AS ?priority)
  } UNION {
    ?iri schema:name ?labelRaw .
    BIND(3 AS ?priority)
  } UNION {
    ?iri dc:title ?labelRaw .
    BIND(4 AS ?priority)
  }

  %s

  BIND(STR(?labelRaw) AS ?label)
}
ORDER BY ?iri ?priority
`, valuesClause.String(), langFilter)

	return query
}

// extractLastSegment extracts the last path segment from an IRI as fallback label
// Examples:
//   http://example.com/resource/Person -> "Person"
//   http://example.com/ns#property -> "property"
//   http://example.com/path/to/resource/ -> "resource"
func extractLastSegment(iri string) string {
	// Remove trailing slash
	iri = strings.TrimRight(iri, "/")

	// Try fragment first (after #)
	if hashIdx := strings.LastIndex(iri, "#"); hashIdx != -1 && hashIdx < len(iri)-1 {
		return iri[hashIdx+1:]
	}

	// Try last path segment (after /)
	if slashIdx := strings.LastIndex(iri, "/"); slashIdx != -1 && slashIdx < len(iri)-1 {
		return iri[slashIdx+1:]
	}

	// If no / or #, return the whole IRI (edge case)
	return iri
}

// fetchLabels queries endpoint for labels and returns IRI→label mapping
func fetchLabels(p *Preprocessor, endpointURL string, iris []string, languages []string) map[string]string {
	// Check cache first
	uncachedIRIs := make([]string, 0, len(iris))
	labelMap := make(map[string]string)

	for _, iri := range iris {
		if label, found := getCachedLabel(iri); found {
			labelMap[iri] = label
		} else {
			uncachedIRIs = append(uncachedIRIs, iri)
		}
	}

	// If all cached, return immediately
	if len(uncachedIRIs) == 0 {
		return labelMap
	}

	// Build and execute label query
	query := buildLabelQuery(uncachedIRIs, languages)
	response, err := p.querySparqlEndpoint(endpointURL, query)
	if err != nil {
		log := logger.Get()
		log.Warn("label query failed, using fallback",
			slog.String("error", err.Error()),
			slog.Int("iri_count", len(uncachedIRIs)))
		// Apply fallback for all uncached IRIs
		for _, iri := range uncachedIRIs {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback)
		}
		return labelMap
	}

	// Parse response
	var sparqlResp sparqlResponse
	if err := json.Unmarshal(response, &sparqlResp); err != nil {
		log := logger.Get()
		log.Warn("label response parse failed",
			slog.String("error", err.Error()))
		for _, iri := range uncachedIRIs {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback)
		}
		return labelMap
	}

	// Build mapping from results (first label per IRI due to ORDER BY)
	seenIRIs := make(map[string]bool)
	for _, binding := range sparqlResp.Results.Bindings {
		iriValue := binding["iri"].Value
		labelValue := binding["label"].Value

		if !seenIRIs[iriValue] {
			labelMap[iriValue] = labelValue
			setCachedLabel(iriValue, labelValue)
			seenIRIs[iriValue] = true
		}
	}

	// Apply fallback for IRIs without labels
	for _, iri := range uncachedIRIs {
		if _, found := labelMap[iri]; !found {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback)
		}
	}

	return labelMap
}

// enrichWithLabels populates the Lol field for URI bindings
func enrichWithLabels(result *QueryResult, labelMap map[string]string) {
	for i := range result.Bindings {
		for varName, value := range result.Bindings[i] {
			if value.Type == "uri" {
				if label, found := labelMap[value.Value]; found {
					// Create new struct with updated Lol field
					result.Bindings[i][varName] = struct {
						Type  string
						Value string
						Lol   string
					}{
						Type:  value.Type,
						Value: value.Value,
						Lol:   label,
					}
				}
			}
			// Literals keep Value as Lol (already set in simplifyBindings)
		}
	}
}
