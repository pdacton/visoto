package sparql

// labels.go provides label resolution for RDF IRIs.
//
// It fetches human-readable labels from a SPARQL endpoint by querying common
// labelling properties (rdfs:label, skos:prefLabel, schema:name, etc.) and
// ranking results by property type and browser language preference. Resolved
// labels are cached in memory with a TTL to avoid redundant network requests.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/logger"
)

// ── Cache types & variables ──────────────────────────────────────────────────

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

// ── Cache lifecycle ──────────────────────────────────────────────────────────

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
		labelCache.Range(func(key, value any) bool {
			if entry, ok := value.(labelCacheEntry); ok {
				if now.After(entry.expiration) {
					labelCache.Delete(key)
				}
			}
			return true
		})
	}
}

// ── Cache operations ─────────────────────────────────────────────────────────

// labelCacheKey returns a cache key incorporating the IRI and language preferences.
// Languages are sorted so that equivalent preference sets produce the same key.
func labelCacheKey(iri string, languages []string) string {
	langs := make([]string, len(languages))
	copy(langs, languages)
	sort.Strings(langs)
	return iri + "\x00" + strings.Join(langs, ",")
}

// getCachedLabel retrieves label from cache if not expired
func getCachedLabel(iri string, languages []string) (string, bool) {
	key := labelCacheKey(iri, languages)
	if val, ok := labelCache.Load(key); ok {
		if entry, ok := val.(labelCacheEntry); ok {
			if time.Now().Before(entry.expiration) {
				return entry.label, true
			}
			labelCache.Delete(key) // Expired, remove it
		}
	}
	return "", false
}

// setCachedLabel stores label in cache with TTL
func setCachedLabel(iri, label string, languages []string) {
	labelCache.Store(labelCacheKey(iri, languages), labelCacheEntry{
		label:      label,
		expiration: time.Now().Add(cacheTTL),
	})
}

// ── Query building ───────────────────────────────────────────────────────────

// extractIRIs collects all unique IRI values from a QueryResult, sorted for determinism
func extractIRIs(result QueryResult) []string {
	iriSet := make(map[string]bool)

	for _, binding := range result.Bindings {
		for _, value := range binding {
			if value.Type == "uri" {
				iriSet[value.Value] = true
			}
		}
	}

	iris := make([]string, 0, len(iriSet))
	for iri := range iriSet {
		iris = append(iris, iri)
	}
	sort.Strings(iris)

	return iris
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

	// Extract language codes, inserting base language after each variant (en-US → en)
	seen := make(map[string]bool)
	result := make([]string, 0, len(langs))
	for _, lw := range langs {
		if !seen[lw.lang] {
			result = append(result, lw.lang)
			seen[lw.lang] = true
		}
		if strings.Contains(lw.lang, "-") {
			base := strings.SplitN(lw.lang, "-", 2)[0]
			if !seen[base] {
				result = append(result, base)
				seen[base] = true
			}
		}
	}

	return result
}

// buildLabelQuery constructs SPARQL query to fetch labels for given IRIs
// Checks rdfs:label, skos:prefLabel, schema:name, dct:title, dc:title, rico:title (in priority order)
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

	// Build dynamic BIND/COALESCE clause for languages
	var userLangPrio string
	if len(languages) > 0 {
		langParts := make([]string, len(languages))
		for i, lang := range languages {
			langParts[i] = fmt.Sprintf("IF(?lang = '%s', '%d', 1/0),", strings.ToLower(lang), i+1)
		}
		userLangPrio = strings.Join(langParts, "\n          ")
	}

	query := fmt.Sprintf(`
PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
PREFIX schema: <http://schema.org/>
PREFIX dct: <http://purl.org/dc/terms/>
PREFIX dc: <http://purl.org/dc/elements/1.1/>
PREFIX rico: <https://www.ica.org/standards/RiC/ontology#>

SELECT ?iri ?label WHERE {

  # 1. VALUES clause with list of IRIs
  %s

  # 2. Subquery to rank labels by property and language priority
  {
    SELECT ?iri (MIN(?rankedLabel) AS ?bestEntry) WHERE {
      # same VALUES clause with same IRIs in subquery for join optimization (reduce result size of subquery)
	  %s

      # A. Define Property Priority (10-40)
	  # more properties can be added here...
      VALUES (?prop ?pPrio) {
        (rdfs:label "1")
        (skos:prefLabel "2")
        (schema:name "3")
        (dct:title "4")
        (dc:title "5")
        (rico:title "6")
      }

      ?iri ?prop ?val .
      BIND(lang(?val) AS ?lang)

	  # B. Define Language Priority (1-9)
      BIND(
        COALESCE(
          # dynamic language priority based on browser language settings
          %s
          # hardcoded language priority if no language preferences are provided
          IF(?lang = "de", "4", 1/0),
          IF(?lang = "fr", "5", 1/0),
          IF(?lang = "it", "6", 1/0),
          IF(?lang = "en", "7", 1/0),
          IF(?lang = "rm", "8", 1/0),
          "9" # Default/Fallback
        )
	  AS ?lPrio)

      # C. Create the sortable rank: "14LabelText" (PropPrio=1, LangPrio=4)
      BIND(CONCAT(?pPrio, ?lPrio, STR(?val)) AS ?rankedLabel)
    }
    GROUP BY ?iri
  }

  # 3. Remove the two-digit prefix from the subquery (PropPrio + LangPrio) to get the clean label
  BIND(SUBSTR(?bestEntry, 3) AS ?label)

}`, valuesClause.String(), valuesClause.String(), userLangPrio)

	return query
}

// ── Label fetching & enrichment ───────────────────────────────────────────────

// extractLastSegment extracts the last path segment from an IRI as fallback label
// Examples:
//
//	http://example.com/resource/Person -> "Person"
//	http://example.com/ns#property -> "property"
//	http://example.com/path/to/resource/ -> "resource"
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
// TODO: split up queries if more than 5k IRIs, LINDAS returns 403 error (too many entries) for 10k labels
func fetchLabels(p *Preprocessor, endpointURL string, iris []string, languages []string) map[string]string {
	// Check cache first
	uncachedIRIs := make([]string, 0, len(iris))
	labelMap := make(map[string]string)

	for _, iri := range iris {
		if label, found := getCachedLabel(iri, languages); found {
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

	finalizedQuery := p.finalizeQuery(query)
	response, err := p.querySparqlEndpoint(context.Background(), endpointURL, finalizedQuery)
	if err != nil {
		log := logger.Get()
		log.Warn("label query failed, using fallback",
			slog.String("error", err.Error()),
			slog.Int("iri_count", len(uncachedIRIs)))
		for _, iri := range uncachedIRIs {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback, languages)
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
			setCachedLabel(iri, fallback, languages)
		}
		return labelMap
	}

	// Build mapping from results — one row per IRI guaranteed by GROUP BY in subquery
	for _, binding := range sparqlResp.Results.Bindings {
		iriValue := binding["iri"].Value
		labelValue := binding["label"].Value

		if labelValue == "" {
			continue
		}

		labelMap[iriValue] = labelValue
		setCachedLabel(iriValue, labelValue, languages)
	}

	// Apply fallback for IRIs without labels
	for _, iri := range uncachedIRIs {
		if _, found := labelMap[iri]; !found {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback, languages)
		}
	}

	return labelMap
}

// enrichWithLabels populates the DisplayText field for URI bindings
func enrichWithLabels(result *QueryResult, labelMap map[string]string) {
	for i := range result.Bindings {
		for varName, value := range result.Bindings[i] {
			if value.Type == "uri" {
				if label, found := labelMap[value.Value]; found {
					result.Bindings[i][varName] = Binding{
						Type:        value.Type,
						Value:       value.Value,
						DisplayText: label,
					}
				}
			}
			// Literals keep Value as DisplayText (already set in simplifyBindings)
		}
	}
}
