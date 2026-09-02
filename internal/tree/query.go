package tree

import (
	"fmt"
	"regexp"
	"strings"

	"hutzli.org/visoto/internal/sparql"
)

// Query building for the five roles.
//
// Every role query is parameterised by injecting a VALUES clause immediately
// after the opening brace of the declared query's own WHERE:
//
//	SELECT ?node WHERE { VALUES ?parent { <iri> }  ?node skos:broader ?parent … }
//
// It must go INSIDE that WHERE. Wrapping the declared query in an outer
// SELECT * WHERE { VALUES … { …declared… } } looks equivalent and is not: a
// declared query carries its own SELECT, so the wrapped copy becomes a SUBQUERY,
// and SPARQL evaluates subqueries bottom-up. A subquery that does not project
// ?parent is evaluated independently of the outer VALUES, which then filters
// nothing — the level comes back as the WHOLE relation (every child of every
// parent), silently, with no error. That is the failure this file exists to
// prevent, so the injection point is not a stylistic choice.
//
// Finding the right brace is therefore load-bearing: whereClauseOpen scans for the
// top-level WHERE and its "{", skipping over strings, IRIs and comments so a "{"
// inside a literal cannot be mistaken for it. A query whose brace cannot be found
// is REJECTED rather than parameterised by a wrap that would be wrong.
//
// The values themselves are IRI terms built by sparql.IRITerm, which rejects
// anything that could close the <...> early. No request input reaches the query
// as a variable name: the names are the constants in validate.go.

// LimitMode says how the level's limit was applied, so the client knows whether
// "load more" means anything.
type LimitMode string

const (
	// LimitPage: the query has its own ORDER BY, so the level is deterministically
	// ordered and a limit is a page size — asking for more is meaningful.
	LimitPage LimitMode = "page"
	// LimitCap: no ORDER BY, so row order is arbitrary and a limit is a hard cap.
	// Loading "more" would return an arbitrary different subset, so the client
	// reports "showing N of M" instead of offering it.
	LimitCap LimitMode = "cap"
)

// DefaultPageLimit and DefaultCapLimit are the per-level defaults for the two
// modes: a page size when the query orders its rows, a hard cap when it does not.
const (
	DefaultPageLimit = 200
	DefaultCapLimit  = 10000
)

// orderByPresentRe reports whether a query carries its own ORDER BY.
var orderByPresentRe = regexp.MustCompile(`(?is)\bORDER\s+BY\b`)

// ModeFor returns the limit semantics of a declared query and the limit to apply.
// A caller-supplied limit of 0 means "use the default for the mode".
func ModeFor(declared string, limit int) (LimitMode, int) {
	mode := LimitCap
	def := DefaultCapLimit
	if orderByPresentRe.MatchString(declared) {
		mode = LimitPage
		def = DefaultPageLimit
	}
	if limit <= 0 {
		return mode, def
	}
	return mode, limit
}

// buildOpts are the inputs shared by every role query.
type buildOpts struct {
	declared string   // the role's query text, as declared
	pageIRI  string   // the ?? substitution value; may be empty if unused
	varName  string   // reserved variable to bind, or "" for none
	iris     []string // IRI values to bind to varName
	literal  string   // literal value to bind to varName, for the search token
	limit    int      // 0 means no limit clause
	offset   int      // 0 means none; only meaningful in page mode
}

// build applies ?? substitution, wraps the declared query with the VALUES binding,
// and reapplies limit/offset outside the wrapper.
func build(o buildOpts) (string, error) {
	declared := o.declared

	// ?? is the page's resource IRI. Substituted only when the query uses it, so a
	// tree on a page with no resource IRI still works as long as its queries do not
	// reference ??.
	if strings.Contains(declared, "??") {
		if o.pageIRI == "" {
			return "", fmt.Errorf("query uses ?? but no page IRI was supplied")
		}
		var err error
		declared, err = sparql.SubstituteEntity(declared, o.pageIRI)
		if err != nil {
			return "", fmt.Errorf("page IRI: %w", err)
		}
	}

	// A declared LIMIT would bound the inner group rather than the result, so move
	// it out. The caller's limit wins; a declared one is a floor the author chose.
	declared = sparql.StripTrailingLimitOffset(declared)

	if o.varName == "" {
		return withLimit(declared, o.limit, o.offset), nil
	}

	values, err := valuesClause(o.varName, o.iris, o.literal)
	if err != nil {
		return "", err
	}
	injected, err := injectValues(declared, values)
	if err != nil {
		return "", err
	}
	return withLimit(injected, o.limit, o.offset), nil
}

// injectValues places the VALUES clause just inside the declared query's WHERE.
func injectValues(declared, values string) (string, error) {
	open, err := whereClauseOpen(declared)
	if err != nil {
		return "", err
	}
	return declared[:open+1] + "\n  " + values + "\n" + declared[open+1:], nil
}

// whereClauseOpen returns the index of the "{" that opens the query's top-level
// WHERE clause.
//
// The scan skips string literals, IRIs and comments, so a brace inside any of them
// cannot be mistaken for the real one — a label containing "{" would otherwise
// place the binding inside a literal and produce a syntax error at the endpoint.
//
// SPARQL allows the WHERE keyword to be omitted (SELECT ?s { ?s ?p ?o }), so the
// fallback is the first top-level "{" after the projection.
func whereClauseOpen(q string) (int, error) {
	inString := byte(0)
	inIRI := false
	inComment := false
	seenWhere := false
	whereEnd := -1

	upper := strings.ToUpper(q)
	for i := 0; i < len(q); i++ {
		ch := q[i]

		if inComment {
			if ch == '\n' {
				inComment = false
			}
			continue
		}
		if inString != 0 {
			if ch == '\\' {
				i++ // escaped character: never a delimiter
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		if inIRI {
			if ch == '>' {
				inIRI = false
			}
			continue
		}

		switch ch {
		case '#':
			inComment = true
			continue
		case '\'', '"':
			inString = ch
			continue
		case '<':
			// "<" is an IRI opener only where a term may start; as a comparison
			// operator it is followed by a space or a digit. Treating a comparison
			// as an IRI would swallow the rest of the line, so require a plausible
			// scheme character.
			if i+1 < len(q) && (isSchemeChar(q[i+1])) {
				inIRI = true
			}
			continue
		}

		if !seenWhere && ch == 'W' && strings.HasPrefix(upper[i:], "WHERE") {
			seenWhere = true
			whereEnd = i + len("WHERE")
			continue
		}
		if ch == '{' {
			// With WHERE present, take the first brace after it; without, the first
			// top-level brace at all.
			if !seenWhere || i > whereEnd {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("cannot locate the WHERE clause of the declared query — " +
		"a role query must be a SELECT with a braced WHERE body")
}

// isSchemeChar reports whether c can begin an IRI scheme, distinguishing "<http…"
// from the "<" comparison operator.
func isSchemeChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// valuesClause renders "VALUES ?name { <a> <b> }" for IRI inputs, or
// "VALUES ?name { \"term\" }" for the search token.
func valuesClause(name string, iris []string, literal string) (string, error) {
	if err := sparql.ValidateVarName(name); err != nil {
		return "", err
	}
	if literal != "" {
		return fmt.Sprintf("VALUES ?%s { %s }", name, sparql.StringLiteral(literal)), nil
	}
	if len(iris) == 0 {
		return "", fmt.Errorf("no values to bind to ?%s", name)
	}
	terms := make([]string, 0, len(iris))
	for _, iri := range iris {
		term, err := sparql.IRITerm(iri)
		if err != nil {
			return "", fmt.Errorf("value for ?%s: %w", name, err)
		}
		terms = append(terms, term)
	}
	return fmt.Sprintf("VALUES ?%s { %s }", name, strings.Join(terms, " ")), nil
}

func withLimit(q string, limit, offset int) string {
	if limit > 0 {
		q += fmt.Sprintf("\nLIMIT %d", limit)
	}
	if offset > 0 {
		q += fmt.Sprintf("\nOFFSET %d", offset)
	}
	return q
}

// RootsQuery builds the top-level query. It binds nothing: roots are whatever the
// declared query says they are, scoped by ?? if it uses it.
func RootsQuery(declared, pageIRI string, limit, offset int) (string, error) {
	return build(buildOpts{declared: declared, pageIRI: pageIRI, limit: limit, offset: offset})
}

// ChildrenQuery builds one level below parent.
func ChildrenQuery(declared, pageIRI, parent string, limit, offset int) (string, error) {
	return build(buildOpts{
		declared: declared, pageIRI: pageIRI,
		varName: VarParent, iris: []string{parent},
		limit: limit, offset: offset,
	})
}

// ParentsQuery builds one upward step for a batch of nodes.
//
// Single-level by design: the declared query names the direct parent relation, and
// the ancestor walk applies it repeatedly. A transitive path (skos:broader*) in
// one query would be cheaper in round trips and far more expensive on endpoints
// that handle property paths badly, which includes some of ours. Batching the
// nodes is what keeps the walk affordable: one query per LEVEL, not per node.
func ParentsQuery(declared, pageIRI string, nodes []string) (string, error) {
	return build(buildOpts{
		declared: declared, pageIRI: pageIRI,
		varName: VarNode, iris: nodes,
	})
}

// NodeDataQuery batch-fetches extra bindings for a set of visible nodes.
func NodeDataQuery(declared, pageIRI string, nodes []string) (string, error) {
	return build(buildOpts{
		declared: declared, pageIRI: pageIRI,
		varName: VarNode, iris: nodes,
	})
}

// SearchQuery binds the user's term as a string literal.
//
// The term is the one free-text input in the whole surface. It is bound through
// VALUES as a quoted literal rather than substituted into the query body, so a
// term containing quotes or backslashes is contained rather than escaping into
// query syntax.
func SearchQuery(declared, pageIRI, term string, limit int) (string, error) {
	if strings.TrimSpace(term) == "" {
		return "", fmt.Errorf("empty search term")
	}
	return build(buildOpts{
		declared: declared, pageIRI: pageIRI,
		varName: VarToken, literal: term,
		limit: limit,
	})
}
