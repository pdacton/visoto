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

// anchorVar resolves the variable a constraint filters through, or "" when the
// constraint filters its projected column directly.
//
// CLASS mode returns the anchor variable: an explicit Root, or the sniffed key var
// when Root is empty (so declarations written before Root existed keep working).
// COLUMN mode (no path) and INSTANCE mode (a fixed root IRI) both return "": a
// pattern anchored on a constant is identical for every row and so cannot filter
// one, which leaves the projected column as the only thing to constrain.
func anchorVar(spec FacetSpec, keyVar string) string {
	if strings.TrimSpace(spec.Path) == "" {
		return "" // column mode
	}
	root := strings.TrimSpace(spec.Root)
	switch root {
	case "":
		return keyVar // backwards compatible: today's sniffed key var
	case InstanceRoot:
		return "" // instance mode filters the column; only enumeration differs
	default:
		return strings.TrimPrefix(root, "?")
	}
}

// BuildColumnValuesQuery builds the COLUMN-mode enumeration: the declared query
// (?? already substituted) wrapped as a subquery, grouped by the facet variable.
//
// This is the expensive shape — it runs the whole base query — and it is the only
// correct one when nothing links the value to a cheap anchor: a variable produced
// by a BIND inside a UNION branch has no property path to enumerate from. Callers
// gate it on the loaded set being incomplete; a complete set enumerates client-side
// from the rows it already holds, without a request.
//
// Keeping the inner query's own LIMIT is deliberate: it bounds the cost AND keeps
// the offered values consistent with the rows the table actually shows.
func BuildColumnValuesQuery(fullQuery string, spec FacetSpec, limit int) (string, error) {
	if err := sparql.ValidateVarName(spec.Var); err != nil {
		return "", fmt.Errorf("facet %q: var: %w", spec.Var, err)
	}
	// The outer aggregate alias would collide with an inner projection of the same
	// name ("alias already used"), so reject that rather than emit a broken query.
	if countProjectionRe.MatchString(fullQuery) {
		return "", fmt.Errorf("facet %q: the query already projects ?count, which collides with the facet count alias — rename it", spec.Var)
	}
	if limit <= 0 {
		limit = DefaultEnumerateLimit
	}
	v := "?" + spec.Var
	return fmt.Sprintf(
		"SELECT %s (COUNT(*) AS ?count) WHERE {\n{\n%s\n}\n} GROUP BY %s ORDER BY DESC(?count) LIMIT %d",
		v, fullQuery, v, limit,
	), nil
}

// countProjectionRe matches a ?count variable bound in the declared query, whether
// projected directly or produced by an aggregate alias.
var countProjectionRe = regexp.MustCompile(`(?i)(\?count\b|\bAS\s+\?count\b)`)

// aggregateAliasRe reports whether varName is produced by an aggregate alias, e.g.
// "(GROUP_CONCAT(?postalCode; separator=\", \") AS ?postalCodes)".
//
// The argument list tolerates ONE level of nesting. A flat [^)]* stops at the
// first ")", so a nested call like "SUM(IF(?a, 1, 0))" never matched and the alias
// slipped past the guard into the silent-empty-result failure below. Plain forms
// ("COUNT(DISTINCT ?x)", a GROUP_CONCAT separator) carry no inner parens and
// matched either way.
func aggregateAliasRe(varName string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b(COUNT|SUM|AVG|MIN|MAX|SAMPLE|GROUP_CONCAT)\s*\((?:[^()]|\([^()]*\))*\)\s+AS\s+\?` +
		regexp.QuoteMeta(varName) + `\b`)
}

// outerProjectionAggregate reports whether varName is bound by an aggregate alias
// in the OUTERMOST select's projection — the only position where the alias exists
// solely in the projection and a WHERE-level FILTER on it misfires.
//
// An aggregate nested inside a subquery is a different story: the subquery
// computes it and projects it outward, so by the time the outer WHERE runs, the
// variable is an ordinary bound solution binding and an ordinary FILTER on it is
// correct. templates/classes/schch%3ACanton.html is exactly that shape — its
// ?districts/?municipalities counts come from "OPTIONAL { SELECT ... GROUP BY }"
// subqueries — and there is no property path from ?canton to a computed count, so
// rejecting it would leave the author no way forward at all.
//
// Depth is tracked by brace counting, skipping quoted literals so a separator like
// "}" cannot shift it. ok=false means the query could not be read confidently
// (unbalanced braces); the caller then keeps the conservative rejection rather
// than guessing.
func outerProjectionAggregate(fullQuery, varName string) (isOuter, ok bool) {
	re := aggregateAliasRe(varName)
	depth := 0
	for i := 0; i < len(fullQuery); i++ {
		switch c := fullQuery[i]; c {
		case '"', '\'':
			// Skip the literal. An unterminated one means we cannot trust the
			// depths that follow, so fail closed rather than mis-scoping.
			j := i + 1
			for j < len(fullQuery) && fullQuery[j] != c {
				if fullQuery[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(fullQuery) {
				return false, false
			}
			i = j
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false, false
			}
		}
	}
	if depth != 0 {
		return false, false
	}
	// Re-scan for the alias, reporting the brace depth at each match.
	for _, loc := range re.FindAllStringIndex(fullQuery, -1) {
		if braceDepthAt(fullQuery, loc[0]) == 0 {
			return true, true
		}
	}
	return false, true
}

// braceDepthAt returns the brace nesting depth at byte offset pos, skipping
// quoted literals. Callers must have already validated that the query's braces
// balance (see outerProjectionAggregate).
func braceDepthAt(q string, pos int) int {
	depth := 0
	for i := 0; i < pos && i < len(q); i++ {
		switch c := q[i]; c {
		case '"', '\'':
			j := i + 1
			for j < len(q) && q[j] != c {
				if q[j] == '\\' {
					j++
				}
				j++
			}
			i = j
		case '{':
			depth++
		case '}':
			depth--
		}
	}
	return depth
}

// checkFilterable rejects a COLUMN-mode constraint on an aggregate alias that the
// outermost select produces.
//
// This is the one failure in faceted search that is otherwise SILENT: such an
// alias exists only in the projection, so a WHERE-level FILTER naming it raises a
// per-row evaluation error, every row is excluded, and the store answers 200 with
// an empty result. Nothing distinguishes that from "no matches" (verified against
// LINDAS). Restricting an aggregate needs HAVING, which the injector does not emit.
//
// The fix an author wants is almost always root+path: filter the underlying
// property existentially instead of the concatenated value, which is both correct
// and better semantics on a multi-valued path. So the error says that.
//
// Scope is what separates the two cases, and it has to be measured rather than
// pattern-matched: an aggregate computed in a SUBQUERY reaches the outer query as
// an ordinary variable, filters correctly, and must be let through — see
// outerProjectionAggregate.
func checkFilterable(fullQuery string, spec FacetSpec) error {
	if strings.TrimSpace(spec.Path) != "" {
		return nil // routed through a property path — the alias is never named
	}
	if !aggregateAliasRe(spec.Var).MatchString(fullQuery) {
		return nil // not an aggregate at all
	}
	// Unreadable query shape (ok=false) falls through to the rejection: the
	// silent-empty-result failure is worse than an over-strict error.
	if isOuter, ok := outerProjectionAggregate(fullQuery, spec.Var); ok && !isOuter {
		return nil // computed in a subquery, projected outward — a plain FILTER works
	}
	return fmt.Errorf("facet %q: cannot filter an aggregate alias — it exists only in the projection, "+
		"so a FILTER on it silently matches nothing; add root=\"?key\" path=\"<property>\" to filter the "+
		"underlying property instead", spec.Var)
}

// BuildInstanceValuesQuery builds the INSTANCE-mode enumeration: the values
// reachable from one fixed resource over the facet path. Cheap, because it skips
// the base query entirely.
//
// Caveat for callers: it is NOT bounded by the declared query's LIMIT, so on a
// resource with far more triples than the table loads it can offer values that
// match no visible row.
func BuildInstanceValuesQuery(resourceIRI string, spec FacetSpec, limit int) (string, error) {
	resTerm, err := IRITerm(resourceIRI)
	if err != nil {
		return "", fmt.Errorf("resource IRI: %w", err)
	}
	if err := sparql.ValidateVarName(spec.Var); err != nil {
		return "", fmt.Errorf("facet %q: var: %w", spec.Var, err)
	}
	if limit <= 0 {
		limit = DefaultEnumerateLimit
	}
	v := "?" + spec.Var
	return fmt.Sprintf(
		"SELECT %s (COUNT(*) AS ?count) WHERE {\n  %s %s %s .\n} GROUP BY %s ORDER BY DESC(?count) LIMIT %d",
		v, resTerm, spec.Path, v, v, limit,
	), nil
}

// BuildFacetedQuery rewrites the declared query (with the IRI already substituted
// for ??) by injecting one block per active constraint.
//
// Class-mode constraints inject a FILTER EXISTS immediately after the membership
// triple, preserving the "don't wrap the base query as a subquery" performance rule
// from internal/sparql/paging.go: the store still drives from the membership triple
// and prunes with the existential filters before the OPTIONALs run.
//
// Column- and instance-mode constraints inject a plain FILTER on the projected
// variable before the closing brace, which works whatever bound the variable — a
// BIND inside a UNION branch, a nested OPTIONAL — because a group-level FILTER is
// evaluated over the group's solutions after they are built.
//
// The provider supplies only the text leg (TextMatchClause); enum and range legs
// are portable and built here. Constraints with no usable value are skipped.
func BuildFacetedQuery(fullQuery, keyVar string, constraints []FacetConstraint, p FacetProvider) (string, error) {
	anchored := map[string][]string{} // anchor var → its existential blocks
	var anchorOrder []string          // stable injection order
	var columnBlocks []string

	for _, con := range constraints {
		if err := con.Spec.Validate(); err != nil {
			return "", err
		}
		if err := checkFilterable(fullQuery, con.Spec); err != nil {
			return "", err
		}
		root := anchorVar(con.Spec, keyVar)
		block, err := facetBlock(root, con, p)
		if err != nil {
			return "", err
		}
		if block == "" {
			continue
		}
		if root == "" {
			columnBlocks = append(columnBlocks, block)
			continue
		}
		if _, seen := anchored[root]; !seen {
			anchorOrder = append(anchorOrder, root)
		}
		anchored[root] = append(anchored[root], block)
	}

	out := fullQuery

	// Class mode: anchor on the membership triple "?root a <iri>" and append the
	// blocks right after it, so the store prunes before the OPTIONALs run. Falls
	// back to the closing brace when no recognizable membership triple is present.
	for _, root := range anchorOrder {
		injection := "\n  " + strings.Join(anchored[root], "\n  ")
		if loc := sparql.MembershipTriplePattern(root).FindStringIndex(out); loc != nil {
			out = out[:loc[1]] + injection + out[loc[1]:]
			continue
		}
		out = insertBeforeLastBrace(out, injection)
	}

	// Column/instance mode: a group-level FILTER, so it must sit in the outermost
	// group where every projected variable is in scope.
	if len(columnBlocks) > 0 {
		out = insertBeforeLastBrace(out, "\n  "+strings.Join(columnBlocks, "\n  ")+"\n")
	}
	return out, nil
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
//
// root is the anchor variable, or "" to filter the projected column directly.
func facetBlock(root string, con FacetConstraint, p FacetProvider) (string, error) {
	if root == "" {
		return columnBlock(con, p)
	}
	if err := sparql.ValidateVarName(root); err != nil {
		return "", fmt.Errorf("facet %q: root: %w", con.Spec.Var, err)
	}
	keyVar := root
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

// columnBlock builds the block for a COLUMN/INSTANCE-mode constraint: a plain
// group-level FILTER on the projected variable itself. Returns "" when the
// constraint carries no usable value.
//
// The three shapes mirror facetBlock's existential ones, but compose as a boolean
// expression rather than as graph patterns — "(no value)" is `!BOUND(?var)` rather
// than FILTER NOT EXISTS, and OR-ing it with a concrete match is `||` rather than
// UNION. (A UNION of two filter-only groups would not work here: its branches are
// evaluated against the empty solution, where the projected variable is unbound.)
//
// NOTE the deliberate asymmetry with existsBlock: the text leg uses the portable
// CONTAINS expression rather than the provider's TextMatchClause. A provider may
// return a graph pattern (a native FTS SERVICE), and there is no pattern context
// here to host one — column mode has only an expression slot.
func columnBlock(con FacetConstraint, p FacetProvider) (string, error) {
	if err := sparql.ValidateVarName(con.Spec.Var); err != nil {
		return "", fmt.Errorf("facet %q: var: %w", con.Spec.Var, err)
	}
	v := "?" + con.Spec.Var
	expr, err := valueExpr(v, con)
	if err != nil {
		return "", err
	}
	notBound := "!BOUND(" + v + ")"
	switch {
	case con.HasNoValue() && expr != "":
		return fmt.Sprintf("FILTER((%s) || %s)", expr, notBound), nil
	case con.HasNoValue():
		return fmt.Sprintf("FILTER(%s)", notBound), nil
	case expr != "":
		return fmt.Sprintf("FILTER(%s)", expr), nil
	default:
		return "", nil
	}
}

// valueExpr builds the bare boolean expression restricting variable v, for
// composition inside a single FILTER. The clause-returning counterpart used by
// class mode is valueClause.
func valueExpr(v string, con FacetConstraint) (string, error) {
	switch con.Spec.Control {
	case ControlSelect:
		return enumExpr(v, con)
	case ControlText:
		return textExpr(v, con), nil
	case ControlRange:
		return rangeExpr(v, con)
	default:
		return "", fmt.Errorf("unknown facet control %q", con.Spec.Control)
	}
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
	expr, err := enumExpr(fv, con)
	if err != nil || expr == "" {
		return "", err
	}
	return fmt.Sprintf("FILTER(%s)", expr), nil
}

// enumExpr is the bare "fv IN (t1, t2, …)" expression behind enumClause.
func enumExpr(fv string, con FacetConstraint) (string, error) {
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
	return fmt.Sprintf("%s IN (%s)", fv, strings.Join(terms, ", ")), nil
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

// LabelPath reaches an IRI's human-readable label. It mirrors the top of the
// priority list that produces DisplayText (internal/sparql/labels.go), so a text
// filter tests what the cell actually shows.
//
// Written as CURIEs: the preprocessor declares the prefixes a query uses, and these
// three are in every shipped prefix list. An author who needs a different vocabulary
// — or one property, for speed on a large class — declares path= instead, which
// takes the class-mode route and never reaches this constant.
const LabelPath = "(rdfs:label|skos:prefLabel|schema:name)"

// textExpr is the portable CONTAINS expression for column mode (see columnBlock on
// why the provider seam is bypassed here). Returns "" when there is no term.
//
// An IRI-valued column is matched through its LABEL rather than its lexical form:
// STR() of an IRI is the IRI itself, so a user typing "Zürich" would be tested
// against .../municipality/261 and never match — the filter would look broken while
// the local preview (which compares display text) found the row. EXISTS is a SPARQL
// 1.1 expression, so the label hop still composes with the "(no value)" leg that
// columnBlock ORs around this.
func textExpr(v string, con FacetConstraint) string {
	term := ""
	if len(con.Values) > 0 {
		term = strings.TrimSpace(con.Values[0])
	}
	if term == "" {
		return ""
	}
	if con.Spec.Type == TypeIRI {
		lv := facetValueVar(con.Spec)
		return fmt.Sprintf("EXISTS { %s %s %s . %s }", v, LabelPath, lv, PortableTextMatch(lv, term))
	}
	return PortableTextExpr(v, term)
}

// rangeClause builds a typed numeric/date range FILTER from [min, max]. Either
// bound may be empty for a one-sided range.
func rangeClause(fv string, con FacetConstraint) (string, error) {
	expr, err := rangeExpr(fv, con)
	if err != nil || expr == "" {
		return "", err
	}
	return fmt.Sprintf("FILTER(%s)", expr), nil
}

// rangeExpr is the bare coerced comparison behind rangeClause.
func rangeExpr(fv string, con FacetConstraint) (string, error) {
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
	return strings.Join(parts, " && "), nil
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
	return fmt.Sprintf("FILTER(%s)", PortableTextExpr(fv, term))
}

// PortableTextExpr is PortableTextMatch's bare expression, for composition inside a
// larger FILTER (column mode ORs it with !BOUND).
func PortableTextExpr(fv, term string) string {
	return fmt.Sprintf("CONTAINS(LCASE(STR(%s)), %s)", fv, StringLiteralTerm(strings.ToLower(term)))
}
