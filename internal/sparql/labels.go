package sparql

// labels.go provides label resolution for RDF IRIs.
//
// It picks ONE best literal per IRI for a magic property — "label" for names,
// "description" for the page subtitle. The candidate properties and their
// priority are the alternatives of [rdf.magic_properties] in visoto.config, in
// written order. Ranking is language first, then property:
//
//  1. the requested (page) language,
//  2. the site languages in their configured order, where "" means an
//     untagged literal,
//  3. any other language;
//
// and within one language rung, the earlier property wins. Resolved values are
// cached in memory with a TTL to avoid redundant network requests.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/cache"
	"hutzli.org/visoto/internal/logger"
)

// ── Cache ────────────────────────────────────────────────────────────────────

// labelCache holds resolved values, keyed by what decides them (see
// bestCacheKey). The implementation is the shared expiring map in internal/cache.
var labelCache = cache.New[string](1 * time.Hour)

// bestCacheKey identifies one resolved value: the magic property, the endpoint
// (its prefixes and data differ), the IRI and the language ORDER — ranking is
// language first, so [en de] and [de en] can resolve differently.
func bestCacheKey(kind, endpointURL, iri string, languages []string) string {
	return kind + "\x00" + endpointURL + "\x00" + iri + "\x00" + strings.Join(languages, ",")
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

// DefaultLabelPath is used when visoto.config defines no "label" magic property.
const DefaultLabelPath = "rdfs:label|skos:prefLabel|schema:name|dct:title|dc:title|rico:title|rico:name"

// simpleTerm matches one predicate — a CURIE or an <IRI> — the only thing a
// VALUES row can hold. Property paths (a/b, ^a) cannot be ranked this way.
var simpleTerm = regexp.MustCompile(`^(?:[A-Za-z][\w.-]*:[\w.-]*|<[^<>"{}|^\x60\\\s]+>)$`)

// langCode matches a language code safe to splice into the query.
var langCode = regexp.MustCompile(`^[a-z]{1,8}$`)

// bestProperties returns the ranked candidate predicates of magic property
// kind: its alternatives in written order. Entries that are not a single
// predicate are skipped with a warning.
func (p *Preprocessor) bestProperties(kind string) []string {
	path, ok := p.config.MagicProperties[kind]
	if !ok && kind == "label" {
		path = DefaultLabelPath
	}
	var props []string
	for _, alt := range strings.Split(path, "|") {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		if !simpleTerm.MatchString(alt) {
			logger.Get().Warn("magic property alternative is not a single predicate, ignored for ranking",
				slog.String("property", kind), slog.String("alternative", alt))
			continue
		}
		props = append(props, alt)
	}
	return props
}

// rankLanguages is the language order a value is chosen by: the requested
// languages (reduced to base codes, "en-US" → "en"), then the configured site
// languages, without repeats. "" stands for an untagged literal, so its place in
// the site list decides where untagged values rank.
func (p *Preprocessor) rankLanguages(requested []string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(code string) {
		code = strings.ToLower(strings.SplitN(strings.TrimSpace(code), "-", 2)[0])
		if seen[code] || (code != "" && !langCode.MatchString(code)) {
			return
		}
		seen[code] = true
		out = append(out, code)
	}
	for _, l := range requested {
		add(l)
	}
	for _, l := range p.config.LabelLanguages {
		add(l)
	}
	return out
}

// buildBestQuery returns, for each IRI, its single best value among props:
// ranked by language (the order of languages, then anything else), then by
// property (the order of props). Ranks are fixed-width two-digit numbers
// concatenated ahead of the value, so MIN() picks the winner and SUBSTR strips
// the four rank characters again.
func buildBestQuery(iris, props, languages []string) string {
	if len(iris) == 0 || len(props) == 0 {
		return ""
	}

	var values strings.Builder
	values.WriteString("VALUES ?iri { ")
	for _, iri := range iris {
		fmt.Fprintf(&values, "<%s> ", iri)
	}
	values.WriteString("}")

	var propRows strings.Builder
	for i, prop := range props {
		fmt.Fprintf(&propRows, "        (%s \"%02d\")\n", prop, i)
	}

	var langRungs strings.Builder
	for i, code := range languages {
		cond := `?lang = ""`
		if code != "" {
			cond = fmt.Sprintf(`?lang = "%s" || STRSTARTS(?lang, "%s-")`, code, code)
		}
		fmt.Fprintf(&langRungs, "          IF(%s, \"%02d\", 1/0),\n", cond, i)
	}

	return fmt.Sprintf(`SELECT ?iri ?label WHERE {
  %s
  {
    SELECT ?iri (MIN(?ranked) AS ?best) WHERE {
      %s
      VALUES (?prop ?pRank) {
%s      }
      ?iri ?prop ?val .
      # Some sources publish an empty literal (LINDAS ships blank English
      # schema:name values on popular-vote titles); MIN() would prefer it.
      FILTER (STR(?val) != "")
      BIND (LCASE(lang(?val)) AS ?lang)
      BIND (COALESCE(
%s          "99"
        ) AS ?lRank)
      BIND (CONCAT(?lRank, ?pRank, STR(?val)) AS ?ranked)
    }
    GROUP BY ?iri
  }
  BIND (SUBSTR(?best, 5) AS ?label)
}`, values.String(), values.String(), propRows.String(), langRungs.String())
}

// ── Label fetching & enrichment ───────────────────────────────────────────────

// unwrapQuotes strips ONE pair of double quotes enclosing the whole label.
//
// Some sources serialise the quote characters into the literal itself: every
// rico:title on the Federal Archives' 17,657 rico:Activity deliveries on LINDAS
// reads "\"1000/00893 Bundesverwaltung (Bern) (1843 (ca.)-1965)\"", and those
// deliveries are linked from 4.7M record sets, so the quotes surfaced in every
// relationship table. A label that is quoted end to end is never meant that way;
// A label that merely starts with a quote, or quotes two parts of itself
// ("Foo" und "Bar"), is left alone.
func unwrapQuotes(label string) string {
	if len(label) < 2 || !strings.HasPrefix(label, `"`) || !strings.HasSuffix(label, `"`) {
		return label
	}
	inner := label[1 : len(label)-1]
	if strings.Contains(inner, `"`) {
		return label
	}
	return inner
}

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

// fetchBestBatch resolves one batch of IRIs (≤ labelQueryBatchSize) into out.
// IRIs without a value get extractLastSegment when fallback is set, and are
// left out otherwise; either answer is cached.
func (p *Preprocessor) fetchBestBatch(kind, endpointURL string, iris, languages []string, fallback bool, out map[string]string) {
	remember := func(iri, value string) {
		if value != "" {
			out[iri] = value
		}
		labelCache.Set(bestCacheKey(kind, endpointURL, iri, languages), value)
	}
	miss := func(iri string) {
		if fallback {
			remember(iri, extractLastSegment(iri))
		} else {
			remember(iri, "")
		}
	}

	query := buildBestQuery(iris, p.bestProperties(kind), languages)
	if query == "" {
		for _, iri := range iris {
			miss(iri)
		}
		return
	}
	finalizedQuery := p.finalizeQuery(query, "", endpointURL)
	response, err := p.querySparqlEndpoint(context.Background(), endpointURL, finalizedQuery)
	var sparqlResp sparqlResponse
	if err == nil {
		err = json.Unmarshal(response, &sparqlResp)
	}
	if err != nil {
		logger.Get().Warn("best-value query failed, using fallback",
			slog.String("property", kind),
			slog.String("error", err.Error()),
			slog.Int("iri_count", len(iris)))
		for _, iri := range iris {
			if fallback {
				out[iri] = extractLastSegment(iri) // not cached: the next request retries
			}
		}
		return
	}

	// One row per IRI, guaranteed by the GROUP BY in the subquery.
	found := make(map[string]bool, len(iris))
	for _, binding := range sparqlResp.Results.Bindings {
		value := unwrapQuotes(binding["label"].Value)
		if value == "" {
			continue
		}
		iri := binding["iri"].Value
		found[iri] = true
		remember(iri, value)
	}
	for _, iri := range iris {
		if !found[iri] {
			miss(iri)
		}
	}
}

// fetchBest resolves the best value of magic property kind for each IRI, from
// cache where possible, querying the rest in parallel batches. requested are
// the caller's languages (page language first); see rankLanguages.
func (p *Preprocessor) fetchBest(kind, endpointURL string, iris, requested []string, fallback bool) map[string]string {
	languages := p.rankLanguages(requested)
	out := make(map[string]string)
	var uncached []string
	for _, iri := range iris {
		if v, ok := labelCache.Get(bestCacheKey(kind, endpointURL, iri, languages)); ok {
			if v != "" {
				out[iri] = v
			}
			continue
		}
		uncached = append(uncached, iri)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < len(uncached); i += labelQueryBatchSize {
		batch := uncached[i:min(i+labelQueryBatchSize, len(uncached))]
		wg.Add(1)
		go func(b []string) {
			defer wg.Done()
			part := make(map[string]string)
			p.fetchBestBatch(kind, endpointURL, b, languages, fallback, part)
			mu.Lock()
			for k, v := range part {
				out[k] = v
			}
			mu.Unlock()
		}(batch)
	}
	wg.Wait()
	return out
}

// fetchLabels returns IRI → best label; an IRI without one gets its last segment.
func fetchLabels(p *Preprocessor, endpointURL string, iris []string, languages []string) map[string]string {
	return p.fetchBest("label", endpointURL, iris, languages, true)
}

// ResolveBest returns the single best value of magic property kind ("label",
// "description") for iri on the preprocessor's endpoint, in the language of
// acceptLanguage. A label falls back to the IRI's last segment; any other kind
// returns "" when the resource has no value.
func (p *Preprocessor) ResolveBest(kind, iri, acceptLanguage string) string {
	return p.fetchBest(kind, p.config.EndpointURL, []string{iri}, parseAcceptLanguage(acceptLanguage), kind == "label")[iri]
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
