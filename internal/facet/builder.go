package facet

import (
	"fmt"
	"regexp"
	"strings"

	"hutzli.org/visoto/internal/sparql"
)

// DefaultEnumerateLimit caps how many distinct values a select facet enumerates.
// A dropdown longer than this is unusable anyway, and the cap keeps the DISTINCT
// bounded on huge classes.
const DefaultEnumerateLimit = 200

// facetValueVar returns the fresh, block-local variable used inside a facet's
// FILTER EXISTS. It is deliberately distinct from the facet's declared Var so the
// existential test never correlates with (or multiplies rows of) the outer query.
func facetValueVar(spec FacetSpec) string {
	return "?__facet_" + spec.Var
}

// BuildFacetValuesQuery builds the Phase-A enumeration query for one facet: the
// distinct values reachable from the class's members via the facet path, ranked
// by how many members carry each value. classIRI is validated (it originates from
// a request param); Var and Path are trusted author input inserted verbatim.
func BuildFacetValuesQuery(classIRI, keyVar string, spec FacetSpec, limit int) (string, error) {
	classTerm, err := IRITerm(classIRI)
	if err != nil {
		return "", fmt.Errorf("class IRI: %w", err)
	}
	if limit <= 0 {
		limit = DefaultEnumerateLimit
	}
	v := "?" + spec.Var
	k := "?" + keyVar
	return fmt.Sprintf(
		"SELECT %s (COUNT(DISTINCT %s) AS ?count) WHERE {\n"+
			"  %s a %s .\n"+
			"  %s %s %s .\n"+
			"} GROUP BY %s ORDER BY DESC(?count) LIMIT %d",
		v, k, k, classTerm, k, spec.Path, v, v, limit,
	), nil
}

// BuildFacetedQuery rewrites the declared instance query (with the class IRI
// already substituted for ??) by injecting one FILTER EXISTS block per active
// constraint immediately after the class-membership triple. This preserves the
// "don't wrap the base query as a subquery" performance rule from
// internal/sparql/paging.go: the store still drives the query from the membership
// triple and prunes members with the existential filters before the OPTIONALs run.
//
// The provider supplies only the text leg (TextMatchClause); enum and range legs
// are portable and built here. Constraints with no usable value are skipped.
func BuildFacetedQuery(fullQuery, keyVar string, constraints []FacetConstraint, p FacetProvider) (string, error) {
	var blocks []string
	for _, con := range constraints {
		block, err := facetBlock(keyVar, con, p)
		if err != nil {
			return "", err
		}
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return fullQuery, nil
	}
	injection := "\n  " + strings.Join(blocks, "\n  ")

	// Anchor on the membership triple "?key a <iri>" and append the blocks right
	// after it. Falls back to inserting before the final closing brace when no
	// recognizable membership triple is present.
	re := sparql.MembershipTriplePattern(keyVar)
	if loc := re.FindStringIndex(fullQuery); loc != nil {
		return fullQuery[:loc[1]] + injection + fullQuery[loc[1]:], nil
	}
	return insertBeforeLastBrace(fullQuery, injection), nil
}

var lastBraceRe = regexp.MustCompile(`}[^}]*$`)

// insertBeforeLastBrace inserts injection immediately before the final '}' of the
// query — the fallback anchor when there is no membership triple to append after.
func insertBeforeLastBrace(query, injection string) string {
	loc := lastBraceRe.FindStringIndex(query)
	if loc == nil {
		return query + injection
	}
	return query[:loc[0]] + injection + query[loc[0]:]
}

// facetBlock builds the whole graph-pattern block for one active constraint, or ""
// when the constraint carries no usable value. The facet value is bound to a fresh
// block-local variable so the test is purely existential.
//
// Any facet control (select, text, range) may ask to match members that LACK any
// value on the facet path — via the "(no value)" checkbox. This is not a value
// clause but a different block shape (FILTER NOT EXISTS), so it is dispatched here
// rather than in valueClause. For select it may arrive as NoValueSentinel inside
// Values; for text/range it arrives via the NoValue flag; HasNoValue() unifies both:
//
//   - concrete values only → FILTER EXISTS { ?key <path> ?fv . <value clause> }
//   - "(no value)" only     → FILTER NOT EXISTS { ?key <path> ?fv }
//   - both (OR within facet) → { <exists> } UNION { <not-exists> }
func facetBlock(keyVar string, con FacetConstraint, p FacetProvider) (string, error) {
	fv := facetValueVar(con.Spec)

	if con.HasNoValue() {
		notExists := fmt.Sprintf("FILTER NOT EXISTS { ?%s %s %s }", keyVar, con.Spec.Path, fv)
		existsPart, err := existsBlock(keyVar, con, fv, p)
		if err != nil {
			return "", err
		}
		if existsPart == "" {
			return notExists, nil // "(no value)" only
		}
		// Both: members that either match the concrete constraint OR have no value.
		return fmt.Sprintf("{ %s } UNION { %s }", existsPart, notExists), nil
	}

	return existsBlock(keyVar, con, fv, p)
}

// existsBlock builds the FILTER EXISTS { ?key <path> ?fv . <clause> } for the
// concrete (non-sentinel) part of a constraint, or "" when there is nothing to
// constrain.
func existsBlock(keyVar string, con FacetConstraint, fv string, p FacetProvider) (string, error) {
	value, err := valueClause(fv, con, p)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", nil
	}
	return fmt.Sprintf("FILTER EXISTS { ?%s %s %s . %s }", keyVar, con.Spec.Path, fv, value), nil
}

// valueClause builds the inner FILTER that restricts the facet value variable fv,
// dispatching on the facet's control type. Returns "" when there is nothing to
// constrain (e.g. an empty selection or an all-empty range).
func valueClause(fv string, con FacetConstraint, p FacetProvider) (string, error) {
	switch con.Spec.Control {
	case ControlSelect:
		return enumClause(fv, con)
	case ControlText:
		return textClause(fv, con, p)
	case ControlRange:
		return rangeClause(fv, con)
	default:
		return "", fmt.Errorf("unknown facet control %q", con.Spec.Control)
	}
}

// enumClause builds FILTER(fv IN (t1, t2, ...)) from the selected enum values
// (OR within one facet). Each value is turned into a safe RDF term. The "(no value)"
// sentinel is excluded here (ConcreteValues); it is handled as a NOT EXISTS block in
// facetBlock and must never reach EnumTerm.
func enumClause(fv string, con FacetConstraint) (string, error) {
	var terms []string
	for _, val := range con.ConcreteValues() {
		if strings.TrimSpace(val) == "" {
			continue
		}
		term, err := EnumTerm(val, con.Spec.Type)
		if err != nil {
			return "", err
		}
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		return "", nil
	}
	return fmt.Sprintf("FILTER(%s IN (%s))", fv, strings.Join(terms, ", ")), nil
}

// textClause delegates to the provider's TextMatchClause (portable CONTAINS in
// the base provider; a native FTS SERVICE in a store-specific override).
func textClause(fv string, con FacetConstraint, p FacetProvider) (string, error) {
	term := ""
	if len(con.Values) > 0 {
		term = strings.TrimSpace(con.Values[0])
	}
	if term == "" {
		return "", nil
	}
	return p.TextMatchClause(fv, term)
}

// rangeClause builds a typed numeric/date range FILTER from [min, max]. Either
// bound may be empty for a one-sided range.
func rangeClause(fv string, con FacetConstraint) (string, error) {
	var lo, hi string
	if len(con.Values) > 0 {
		lo = con.Values[0]
	}
	if len(con.Values) > 1 {
		hi = con.Values[1]
	}
	loTerm, err := RangeBound(lo, con.Spec.Type)
	if err != nil {
		return "", err
	}
	hiTerm, err := RangeBound(hi, con.Spec.Type)
	if err != nil {
		return "", err
	}
	// Coerce the stored value to its comparison type. RDF datasets routinely carry
	// numbers and dates as xsd:string (e.g. LINDAS schema:postalCode), where a
	// direct comparison against a typed bound raises a type error and silently
	// drops every row. Casting the value makes the range work whether the store
	// typed it or not; non-castable values error inside the FILTER and are simply
	// excluded, which is the desired behaviour.
	cv := coerce(fv, con.Spec.Type)
	var parts []string
	if loTerm != "" {
		parts = append(parts, fmt.Sprintf("%s >= %s", cv, loTerm))
	}
	if hiTerm != "" {
		parts = append(parts, fmt.Sprintf("%s <= %s", cv, hiTerm))
	}
	if len(parts) == 0 {
		return "", nil
	}
	return fmt.Sprintf("FILTER(%s)", strings.Join(parts, " && ")), nil
}

// coerce wraps a facet value variable in the xsd constructor for its range type,
// so comparisons work regardless of how the store typed the literal.
//
// The constructor is written as a full IRI rather than the "xsd:" CURIE on
// purpose: nothing declares a PREFIX xsd: — not the query, not visoto.config's
// prefix list, and not the preprocessor (which only auto-declares prefixes it
// finds in that list). LINDAS/GraphDB happens to pre-declare xsd: implicitly, but
// that is not SPARQL 1.1 behaviour, so a CURIE here would make every range facet
// fail with "undefined prefix" on a conformant store. The full-IRI form needs no
// declaration and matches how xsdDate is already written in terms.go.
func coerce(fv, facetType string) string {
	switch facetType {
	case TypeDate:
		return "<" + xsdDate + ">(" + fv + ")"
	default: // number
		return "<" + xsdDecimal + ">(" + fv + ")"
	}
}

// PortableTextMatch is the store-agnostic text leg: a case-insensitive substring
// match on the facet value's lexical form. Exposed so BaseFacetProvider (and
// tests) share one definition.
func PortableTextMatch(fv, term string) string {
	return fmt.Sprintf("FILTER(CONTAINS(LCASE(STR(%s)), %s))", fv, StringLiteralTerm(strings.ToLower(term)))
}
