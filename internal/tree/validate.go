// Package tree builds and validates the queries behind the lazy tree partial.
//
// The lazy tree fetches one level at a time, so a page declares several related
// queries — one per role — rather than a single query returning a whole
// hierarchy. This package owns what those roles mean: which are required, which
// variables the server binds, and how a declared role query becomes a query the
// endpoint can answer.
//
// Validation runs at startup, against the declarations, so a malformed tree fails
// the boot rather than returning an empty level at request time.
package tree

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The five roles. RoleRoots and RoleChildren are required; the rest are opt-in.
const (
	RoleRoots    = "roots"
	RoleChildren = "children"
	RoleParents  = "parents"
	RoleSearch   = "search"
	RoleNodeData = "node-data"
)

// Reserved variable names. The server binds these with a prepended VALUES clause,
// so a declaration must leave them free rather than binding them in a pattern.
const (
	VarNode   = "node"
	VarParent = "parent"
	VarToken  = "__token__"
)

// knownRoles is the closed set, with the variable each one has bound for it.
// A role with no reserved variable takes none ("").
var knownRoles = map[string]string{
	RoleRoots:    "",
	RoleChildren: VarParent,
	RoleParents:  VarNode,
	RoleSearch:   VarToken,
	RoleNodeData: VarNode,
}

// ValidateRoles checks the role set of one declaration: the two required roles are
// present and every declared role is one this package knows.
//
// Duplicate roles cannot reach here — a map has no duplicates, and the parser
// rejects them at extraction, where both spellings are still visible.
func ValidateRoles(id string, roles map[string]string) error {
	for _, required := range []string{RoleRoots, RoleChildren} {
		if strings.TrimSpace(roles[required]) == "" {
			return fmt.Errorf(`tree %q: no <sparql-tree-query role=%q> — roots and children are both required`, id, required)
		}
	}
	for role := range roles {
		if _, known := knownRoles[role]; !known {
			return fmt.Errorf(`tree %q: unknown role %q — expected one of %s`, id, role, knownRoleList())
		}
	}
	return nil
}

func knownRoleList() string {
	names := make([]string, 0, len(knownRoles))
	for r := range knownRoles {
		names = append(names, r)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// ValidateReservedVars checks that a role query leaves its reserved variable free
// for the server to bind.
//
// The server parameterises a level query by prepending a VALUES clause: the
// children query is written with ?parent free, and VALUES ?parent { <iri> } binds
// it to the node being expanded. If the query ALSO binds ?parent in a triple
// pattern, the VALUES does not parameterise it — it intersects with it, silently
// narrowing the result to rows that happen to satisfy both. The level then comes
// back empty or wrong, with no error anywhere.
//
// A binding occurrence is one in a triple pattern or a BIND/VALUES of its own. A
// FILTER referencing the variable is fine, and so is projecting it in the SELECT
// clause: both read the bound value rather than constraining it independently.
func ValidateReservedVars(id, role, query string) error {
	reserved, known := knownRoles[role]
	if !known || reserved == "" {
		return nil
	}

	body := stripSelectClause(query)
	if reserved == VarToken {
		// The search token is the input: a query that never mentions it ignores what
		// the user typed and returns the same rows for every term.
		if !mentionsVar(query, VarToken) {
			return fmt.Errorf("tree %q: the search query does not use ?%s — it would return the same rows for every search term", id, VarToken)
		}
		return nil
	}

	if bindsVar(body, reserved) {
		return fmt.Errorf("tree %q: the %s query binds ?%s in a BIND or VALUES clause, but the server binds it — "+
			"leave ?%s free so it can be parameterised", id, role, reserved, reserved)
	}
	if !mentionsVar(body, reserved) {
		return fmt.Errorf("tree %q: the %s query never mentions ?%s — it cannot be scoped to the node being expanded",
			id, role, reserved)
	}
	return nil
}

// varRe matches one SPARQL variable occurrence. \b would not do: "?parent" and
// "?parentLabel" both start the same way, and only the first is the reserved name.
func varRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`\?` + regexp.QuoteMeta(name) + `(?:[^A-Za-z0-9_]|$)`)
}

func mentionsVar(q, name string) bool { return varRe(name).MatchString(q) }

// bindsVar reports whether the query assigns the variable itself, rather than
// reading it. BIND(… AS ?x) and VALUES ?x { … } both make the server's own VALUES
// redundant at best and contradictory at worst.
func bindsVar(q, name string) bool {
	bindAs := regexp.MustCompile(`(?is)\bAS\s+\?` + regexp.QuoteMeta(name) + `(?:[^A-Za-z0-9_]|$)`)
	if bindAs.MatchString(q) {
		return true
	}
	// VALUES ?x { … } and VALUES (?x ?y) { … }
	values := regexp.MustCompile(`(?is)\bVALUES\s*\(?[^{]*\?` + regexp.QuoteMeta(name) + `(?:[^A-Za-z0-9_][^{]*)?\{`)
	return values.MatchString(q)
}

// selectClauseRe matches the projection of the outermost SELECT, up to WHERE.
var selectClauseRe = regexp.MustCompile(`(?is)^\s*(?:PREFIX\s+\S+\s+<[^>]*>\s*)*\s*SELECT\b[^{]*?\bWHERE\b`)

// stripSelectClause removes the leading projection so a variable that is merely
// SELECTed is not mistaken for one the query binds.
func stripSelectClause(q string) string {
	if loc := selectClauseRe.FindStringIndex(q); loc != nil {
		return q[loc[1]:]
	}
	return q
}

// projectionRe captures the projection list of the outermost SELECT.
var projectionRe = regexp.MustCompile(`(?is)\bSELECT\s+(?:DISTINCT\s+|REDUCED\s+)?(.*?)\bWHERE\b`)

// orderByRe captures an ORDER BY clause up to the next clause keyword.
var orderByRe = regexp.MustCompile(`(?is)\bORDER\s+BY\s+(.*?)(?:\bLIMIT\b|\bOFFSET\b|\bGROUP\b|\bHAVING\b|$)`)

// WarnOrderByUnprojected reports variables a query orders by but does not project.
//
// This is the ordering trap (LT-16). Labels are resolved in a SECOND batch query
// after the level query returns, so a level query that sorts by ?label without
// projecting it sorts by whatever the endpoint has — usually the IRI — takes the
// first page of THAT order, and only then relabels the rows. The page is silently
// the wrong set of nodes, in an order that looks arbitrary.
//
// A warning rather than an error: templates predating this rule are merely
// imprecise, and failing the boot over an ORDER BY would be a poor trade. It is
// only misleading when combined with a limit, but the limit can come from the
// partial rather than the query text, so warn whenever the projection is missing.
func WarnOrderByUnprojected(query string) []string {
	order := orderByRe.FindStringSubmatch(query)
	if order == nil {
		return nil
	}
	proj := projectionRe.FindStringSubmatch(query)
	if proj == nil {
		return nil
	}
	projected := proj[1]
	// SELECT * projects everything, so nothing can be missing.
	if strings.Contains(projected, "*") {
		return nil
	}

	var missing []string
	seen := make(map[string]bool)
	for _, m := range regexp.MustCompile(`\?([A-Za-z_][A-Za-z0-9_]*)`).FindAllStringSubmatch(order[1], -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if !varRe(name).MatchString(projected + " ") {
			missing = append(missing, name)
		}
	}
	return missing
}

// Validate runs every check for one declaration. Returns the fatal error, if any,
// and the non-fatal warnings for the caller to log.
func Validate(id string, roles map[string]string) (warnings []string, err error) {
	if err := ValidateRoles(id, roles); err != nil {
		return nil, err
	}
	// Sorted so a template with two bad roles fails the same way every boot.
	names := make([]string, 0, len(roles))
	for role := range roles {
		names = append(names, role)
	}
	sort.Strings(names)

	for _, role := range names {
		query := roles[role]
		if err := ValidateReservedVars(id, role, query); err != nil {
			return nil, err
		}
		for _, v := range WarnOrderByUnprojected(query) {
			warnings = append(warnings, fmt.Sprintf(
				"tree %q, role %q: ORDER BY ?%s but ?%s is not projected — with a limit this pages over the wrong rows, "+
					"because labels are resolved after the level query returns; add ?%s to the SELECT",
				id, role, v, v, v))
		}
	}
	return warnings, nil
}

// RequiresParents reports whether a feature needs the parents role, which is what
// lets the tree walk up from a node to the roots. Focus and hierarchical search
// hits both do; flat search does not.
func RequiresParents(hasFocus, hasSearch, flatSearchOnly bool) bool {
	if hasFocus {
		return true
	}
	return hasSearch && !flatSearchOnly
}
