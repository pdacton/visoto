package sparql

import (
	"fmt"
	"regexp"
	"strings"
)

// This file holds the pure query-string helpers behind remote-paginated instance
// tables (served by the async-table-data HTTP handler). They compose three cheap
// queries from a declared instance query rather than wrapping it as a subquery
// (which would force the store to materialize the entire joined result set before
// paging — measured 10-50x slower):
//
//   - count: SELECT (COUNT(*) AS ?count) WHERE { ?key a <class> }   (class only)
//   - page:  the declared query with the "?key a <class>" membership triple
//            replaced by a paginated IRI subquery; OPTIONALs run over one page.
//   - search: the same, restricted to instances whose name property or key IRI
//            CONTAINS the term.
//
// Everything here is a pure function over strings / QueryResult: no HTTP, config,
// or endpoint access, so it is unit-testable in isolation.

// MembershipTriplePattern matches the class-membership triple in a declared
// query — "?key a <iri>" or "?key rdf:type <iri>" — so it can be swapped for a
// paginated IRI subquery. The IRI is the finalized <...> form (?? already
// substituted by the caller).
func MembershipTriplePattern(sortVar string) *regexp.Regexp {
	// e.g.  ?taxonName rdf:type <http://...> .
	v := regexp.QuoteMeta(sortVar)
	return regexp.MustCompile(`(?s)\?` + v + `\s+(?:a|rdf:type|<http://www\.w3\.org/1999/02/22-rdf-syntax-ns#type>)\s+<[^>]+>\s*\.`)
}

// MembershipBody builds the inner WHERE body that selects the key IRIs of the
// class, optionally restricted to those whose name property CONTAINS the search
// term. Shared by the page query (wrapped with ORDER BY/LIMIT/OFFSET) and the
// count query (wrapped with COUNT), so both scope to the same instance set.
func MembershipBody(classIRI, sortVar, term, searchProp string) string {
	body := fmt.Sprintf("?%s a <%s>", sortVar, classIRI)
	if term == "" {
		return body
	}
	lit := StringLiteral(strings.ToLower(term))
	if searchProp != "" {
		// Match the pinned name property when the class has one, OR the key IRI
		// string (many datasets — e.g. Plazi TaxonName — carry the name only in
		// the IRI). Left-joined so instances without the name property still match
		// on their IRI. Measured ~2-6s on ~918k instances.
		return fmt.Sprintf(
			"%s . OPTIONAL { ?%s <%s> ?__match . } FILTER(CONTAINS(LCASE(STR(?__match)), %s) || CONTAINS(LCASE(STR(?%s)), %s))",
			body, sortVar, searchProp, lit, sortVar, lit,
		)
	}
	// No name property configured: match the key IRI string alone.
	return fmt.Sprintf("%s . FILTER(CONTAINS(LCASE(STR(?%s)), %s))", body, sortVar, lit)
}

// StringLiteral renders a Go string as a safely quoted SPARQL string literal.
func StringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return `"` + s + `"`
}

// BuildPagedQuery rewrites the declared instance query for remote paging: the
// "?key a <class>" membership triple is replaced by a paginated, ordered IRI
// subquery (optionally search-filtered) so only one page of keys is joined
// against the OPTIONALs. Any trailing LIMIT/OFFSET in the declared query is
// stripped (paging supplies its own). The ?? entity placeholder is substituted
// with the class IRI, mirroring the preprocessor's convention.
func BuildPagedQuery(declared, classIRI, sortVar, term, searchProp string, size, offset int) string {
	declared = strings.ReplaceAll(declared, "??", "<"+classIRI+">")
	q := StripTrailingLimitOffset(declared)

	inner := fmt.Sprintf(
		"{ SELECT ?%s WHERE { %s } ORDER BY ?%s LIMIT %d OFFSET %d }",
		sortVar, MembershipBody(classIRI, sortVar, term, searchProp), sortVar, size, offset,
	)

	re := MembershipTriplePattern(sortVar)
	if re.MatchString(q) {
		return re.ReplaceAllString(q, inner)
	}
	// Fallback: declared query didn't contain a recognizable membership triple.
	// Wrap it, still applying deterministic paging on the key var. Slower, but
	// correct; logged shape stays visible via the "Execute on endpoint" button.
	return fmt.Sprintf("SELECT * WHERE { %s\n%s }", inner, q)
}

var trailingLimitOffsetRe = regexp.MustCompile(`(?is)\s+(LIMIT|OFFSET)\s+\d+(\s+(LIMIT|OFFSET)\s+\d+)?\s*$`)

// StripTrailingLimitOffset removes a trailing LIMIT/OFFSET clause from a query so
// the remote pager can supply its own without conflict.
func StripTrailingLimitOffset(q string) string {
	q = strings.TrimRight(q, " \t\r\n")
	return trailingLimitOffsetRe.ReplaceAllString(q, "")
}

// DistinctKeyCount counts the distinct key-variable IRIs present in a result page.
func DistinctKeyCount(result QueryResult, sortVar string) int {
	seen := map[string]struct{}{}
	for _, b := range result.Bindings {
		if bind, ok := b[sortVar]; ok {
			seen[bind.Value] = struct{}{}
		}
	}
	return len(seen)
}
