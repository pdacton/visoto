package sparql

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// This file holds the RDF-term builders that are the security boundary between
// request input and query text. Any value that originates from an HTTP request
// and ends up inside a query string must pass through one of these, which either
// produce a syntactically-contained term or return an error.
//
// The class IRI behind the `??` entity placeholder is the widest such input: it
// arrives as a query param on the resource, async-table and faceted-table routes
// and is substituted into the declared query as "<iri>". Without validation, an
// IRI carrying a ">" closes the term early and the remainder is parsed as query
// syntax — arbitrary triple patterns can be injected and a trailing "#" comments
// out the rest of the declared query.

// iriIllegal matches any character forbidden inside a SPARQL IRIREF (<...>) per
// the grammar: <, >, ", {, }, |, ^, `, backslash, and any char <= 0x20 (space +
// controls). Rejecting these makes it impossible to close the <...> early or to
// smuggle in whitespace-delimited SPARQL syntax.
var iriIllegal = regexp.MustCompile("[<>\"{}|^`\\\\\\x00-\\x20]")

// ValidateIRI reports whether s is safe to splice into a query as a bracketed
// IRI term. It returns an error for anything that could break out of the <...>
// delimiters or is not an absolute IRI.
func ValidateIRI(s string) error {
	if s == "" {
		return fmt.Errorf("empty IRI")
	}
	if iriIllegal.MatchString(s) {
		return fmt.Errorf("illegal character in IRI %q", s)
	}
	u, err := url.Parse(s)
	if err != nil || !u.IsAbs() {
		return fmt.Errorf("not an absolute IRI: %q", s)
	}
	return nil
}

// IRITerm renders s as a bracketed SPARQL IRI term "<s>" after validating it
// with ValidateIRI.
func IRITerm(s string) (string, error) {
	if err := ValidateIRI(s); err != nil {
		return "", err
	}
	return "<" + s + ">", nil
}

// varNameRe matches a legal SPARQL variable name (the part after the '?'),
// restricted to the ASCII subset this codebase actually generates.
var varNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateVarName reports whether name is safe to splice into a query as "?name".
// Callers interpolate variable names as bare "?name" terms, so an unchecked string
// would inject query syntax. The names now come from template declarations
// (<sparql-column var=, root=) rather than from request params — the working-set
// routes used to take keyVar off the query string and no longer do, deriving it
// from the declared query instead — but they are still spliced, so they are still
// checked at the point of use.
func ValidateVarName(name string) error {
	if !varNameRe.MatchString(name) {
		return fmt.Errorf("invalid SPARQL variable name %q", name)
	}
	return nil
}

// SubstituteEntity replaces the `??` entity placeholder in a declared query with
// the validated IRI term for iri. It is the single safe spelling of what used to
// be an unchecked strings.ReplaceAll(query, "??", "<"+iri+">") at every call
// site. An invalid IRI is an error rather than a silently malformed query.
func SubstituteEntity(query, iri string) (string, error) {
	term, err := IRITerm(iri)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(query, "??", term), nil
}
