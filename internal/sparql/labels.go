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

	"hutzli.org/visoto/internal/cache"
	"hutzli.org/visoto/internal/logger"
)

// ── Cache ────────────────────────────────────────────────────────────────────

// labelCache holds resolved labels. Keyed by IRI *and* language preferences,
// because the label a visitor should see depends on both (see labelCacheKey).
// The implementation is the shared expiring map in internal/cache.
var labelCache = cache.New[string](1 * time.Hour)

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
	return labelCache.Get(labelCacheKey(iri, languages))
}

// setCachedLabel stores label in cache with TTL
func setCachedLabel(iri, label string, languages []string) {
	labelCache.Set(labelCacheKey(iri, languages), label)
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
PREFIX schema2: <https://schema.org/>
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
        (schema2:name "4")
        (dct:title "5")
        (dc:title "6")
        (rico:title "7")
      }

      ?iri ?prop ?val .
      # Some sources publish a labelling triple with an empty literal (LINDAS ships blank
      # English schema:name values on popular-vote titles). Without this guard MIN() below
      # actively prefers them, since "" sorts ahead of any real text at the same priority.
      FILTER (STR(?val) != "")
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

const labelQueryBatchSize = 1000

// fetchLabelsBatch queries the endpoint for a single batch of IRIs (≤ labelQueryBatchSize)
// and merges results into labelMap. Falls back to last-segment for any IRI without a label.
func fetchLabelsBatch(p *Preprocessor, endpointURL string, iris []string, languages []string, labelMap map[string]string) {
	query := buildLabelQuery(iris, languages)
	finalizedQuery := p.finalizeQuery(query, "")
	response, err := p.querySparqlEndpoint(context.Background(), endpointURL, finalizedQuery)
	if err != nil {
		log := logger.Get()
		log.Warn("label query failed, using fallback",
			slog.String("error", err.Error()),
			slog.Int("iri_count", len(iris)))
		for _, iri := range iris {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback, languages)
		}
		return
	}

	var sparqlResp sparqlResponse
	if err := json.Unmarshal(response, &sparqlResp); err != nil {
		log := logger.Get()
		log.Warn("label response parse failed",
			slog.String("error", err.Error()))
		for _, iri := range iris {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback, languages)
		}
		return
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
	for _, iri := range iris {
		if _, found := labelMap[iri]; !found {
			fallback := extractLastSegment(iri)
			labelMap[iri] = fallback
			setCachedLabel(iri, fallback, languages)
		}
	}
}

// fetchLabels queries endpoint for labels and returns IRI→label mapping.
// Splits the request into batches of labelQueryBatchSize to avoid endpoint limits.
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

	// Query in batches to stay within endpoint limits (parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < len(uncachedIRIs); i += labelQueryBatchSize {
		end := i + labelQueryBatchSize
		if end > len(uncachedIRIs) {
			end = len(uncachedIRIs)
		}
		batch := uncachedIRIs[i:end]
		wg.Add(1)
		go func(b []string) {
			defer wg.Done()
			batchResult := make(map[string]string)
			fetchLabelsBatch(p, endpointURL, b, languages, batchResult)
			mu.Lock()
			for k, v := range batchResult {
				labelMap[k] = v
			}
			mu.Unlock()
		}(batch)
	}
	wg.Wait()

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
