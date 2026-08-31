package sparql

import (
	"fmt"
	"regexp"
	"strings"
)

// This file holds the pure query-string helpers behind working-set instance
// tables (served by the async-table-data HTTP handler). They compose two cheap
// queries from a declared instance query rather than wrapping it as a subquery
// (which would force the store to materialize the entire joined result set before
// capping — measured 10-50x slower):
//
//   - count: SELECT (COUNT(*) AS ?count) WHERE { ?key a <class> }   (class only)
//   - working set: the declared query with the "?key a <class>" membership triple
//     replaced by a capped IRI subquery (optionally search-filtered); OPTIONALs
//     join over only those keys. The subquery is deliberately UNORDERED: an
//     ORDER BY ?key forces the store to sort the whole class before LIMIT
//     (measured ~80s on cube:Observation's 12.8M instances vs ~1s unordered,
//     past the 60s request timeout), and nothing needs the order — there is no
//     OFFSET and the frontend sorts/pages the set locally.
//
// Everything here is a pure function over strings / QueryResult: no HTTP, config,
// or endpoint access, so it is unit-testable in isolation.

// deriveKeyVarRe matches an unfinalized class-membership triple "?var a ??" or
// "?var rdf:type ??" in a declared instance query — the ?? placeholder is still
// present (the query has not yet had the class IRI substituted). The captured
// group is the key variable used as the pagination/ordering anchor.
var deriveKeyVarRe = regexp.MustCompile(`(?s)\?([A-Za-z_][A-Za-z0-9_]*)\s+(?:a|rdf:type|<http://www\.w3\.org/1999/02/22-rdf-syntax-ns#type>)\s+\?\?`)

// DeriveKeyVar extracts the class-membership key variable from a declared
// instance query — the ?var in "?var a ??" / "?var rdf:type ??". It returns ""
// when no such triple exists, which signals the query is NOT a class-instance
// table (e.g. the BIND(?? AS ?s) attribute/relationship queries) and must never
// be paginated.
//
// This is the ONLY source of the key var. It was author-supplied once (as sortVar),
// then derived here but still echoed to the client and accepted back as a query
// param; both of those are gone. Every caller derives it from the declared query,
// so no request input can name a variable that gets spliced into SPARQL.
func DeriveKeyVar(declared string) string {
	m := deriveKeyVarRe.FindStringSubmatch(declared)
	if m == nil {
		return ""
	}
	return m[1]
}

// MembershipTriplePattern matches the class-membership triple in a declared
// query — "?key a <iri>" or "?key rdf:type <iri>" — so it can be swapped for the
// capped working-set IRI subquery. The IRI is the finalized <...> form (?? already
// substituted by the caller).
func MembershipTriplePattern(keyVar string) *regexp.Regexp {
	// e.g.  ?taxonName rdf:type <http://...> .
	v := regexp.QuoteMeta(keyVar)
	return regexp.MustCompile(`(?s)\?` + v + `\s+(?:a|rdf:type|<http://www\.w3\.org/1999/02/22-rdf-syntax-ns#type>)\s+<[^>]+>\s*\.`)
}

// MembershipBody builds the inner WHERE body that selects the key IRIs of the
// class, optionally restricted to those whose name property CONTAINS the search
// term. Shared by the working-set query (wrapped with LIMIT) and the count
// query (wrapped with COUNT), so both scope to the same instance set.
//
// classIRI, keyVar and searchProp all originate from request params, so all are
// validated here rather than at each caller: this is the one chokepoint both the
// count and working-set paths funnel through.
func MembershipBody(classIRI, keyVar, term, searchProp string) (string, error) {
	if err := ValidateVarName(keyVar); err != nil {
		return "", err
	}
	classTerm, err := IRITerm(classIRI)
	if err != nil {
		return "", fmt.Errorf("class IRI: %w", err)
	}
	body := fmt.Sprintf("?%s a %s", keyVar, classTerm)
	if term == "" {
		return body, nil
	}
	lit := StringLiteral(strings.ToLower(term))
	if searchProp != "" {
		propTerm, err := IRITerm(searchProp)
		if err != nil {
			return "", fmt.Errorf("search property: %w", err)
		}
		// Match the pinned name property when the class has one, OR the key IRI
		// string (many datasets — e.g. Plazi TaxonName — carry the name only in
		// the IRI). Left-joined so instances without the name property still match
		// on their IRI. Measured ~2-6s on ~918k instances.
		return fmt.Sprintf(
			"%s . OPTIONAL { ?%s %s ?__match . } FILTER(CONTAINS(LCASE(STR(?__match)), %s) || CONTAINS(LCASE(STR(?%s)), %s))",
			body, keyVar, propTerm, lit, keyVar, lit,
		), nil
	}
	// No name property configured: match the key IRI string alone.
	return fmt.Sprintf("%s . FILTER(CONTAINS(LCASE(STR(?%s)), %s))", body, keyVar, lit), nil
}

// StringLiteral renders a Go string as a safely quoted SPARQL string literal.
func StringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return `"` + s + `"`
}

// BuildWorkingSetQuery rewrites the declared instance query to load a single
// bounded "working set" (the working-set table model): the "?key a <class>"
// membership triple is replaced by an unordered IRI subquery capped at LIMIT max
// with NO OFFSET, so the client receives the whole class when it fits under max,
// or an arbitrary max keys otherwise, and the OPTIONALs join over only those keys.
// When term != "" the subquery is search-filtered so the working set is rebuilt
// from the server's matches. Any trailing LIMIT/OFFSET in the declared query is
// stripped. The ?? entity placeholder is substituted with the class IRI,
// mirroring the preprocessor's convention.
//
// Unlike the former remote pager there is never a deep OFFSET (the frontend pages
// the working set locally), so query cost stays flat across the whole session.
//
// classIRI is request input, so it is validated before substitution: an IRI that
// could close the <...> term early would let a caller inject arbitrary graph
// patterns into the declared query.
func BuildWorkingSetQuery(declared, classIRI, keyVar, term, searchProp string, max int) (string, error) {
	declared, err := SubstituteEntity(declared, classIRI)
	if err != nil {
		return "", err
	}
	q := StripTrailingLimitOffset(declared)

	body, err := MembershipBody(classIRI, keyVar, term, searchProp)
	if err != nil {
		return "", err
	}
	inner := fmt.Sprintf("{ SELECT ?%s WHERE { %s } LIMIT %d }", keyVar, body, max)

	re := MembershipTriplePattern(keyVar)
	if re.MatchString(q) {
		return re.ReplaceAllString(q, inner), nil
	}
	// Fallback: declared query didn't contain a recognizable membership triple.
	// Wrap it, still applying the cap on the key var. The logged
	// shape stays visible via the "Execute on endpoint" button.
	return fmt.Sprintf("SELECT * WHERE { %s\n%s }", inner, q), nil
}

var trailingLimitOffsetRe = regexp.MustCompile(`(?is)\s+(LIMIT|OFFSET)\s+\d+(\s+(LIMIT|OFFSET)\s+\d+)?\s*$`)

// StripTrailingLimitOffset removes a trailing LIMIT/OFFSET clause from a query so
// the working-set builder can supply its own cap without conflict.
func StripTrailingLimitOffset(q string) string {
	q = strings.TrimRight(q, " \t\r\n")
	return trailingLimitOffsetRe.ReplaceAllString(q, "")
}

// DistinctKeyCount counts the distinct key-variable IRIs present in a loaded
// working set (rows may repeat a key when OPTIONALs multiply matches).
func DistinctKeyCount(result QueryResult, keyVar string) int {
	seen := map[string]struct{}{}
	for _, b := range result.Bindings {
		if bind, ok := b[keyVar]; ok {
			seen[bind.Value] = struct{}{}
		}
	}
	return len(seen)
}
