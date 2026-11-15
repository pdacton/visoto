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

// IRI is an absolute IRI that is safe to splice into a query as a <…> term.
//
// Its field is unexported, so the only ways to hold a non-zero IRI are ParseIRI
// and MustIRI: a function that takes an IRI cannot be handed unvalidated request
// input, and forgetting to validate is a compile error rather than something a
// review has to catch. Term is the one place in the codebase that writes the
// angle brackets (TestNoRawIRISplices holds every other file to that).
//
// The zero IRI means "none" — an optional filter that was not given. Check it
// with IsZero; its Term is "<>", which is syntactically harmless and matches
// nothing.
type IRI struct{ s string }

// ParseIRI validates s (see ValidateIRI) and returns it as an IRI.
func ParseIRI(s string) (IRI, error) {
	if err := ValidateIRI(s); err != nil {
		return IRI{}, err
	}
	return IRI{s}, nil
}

// ParseOptionalIRI is ParseIRI for an optional value: the empty string is the
// zero IRI rather than an error.
func ParseOptionalIRI(s string) (IRI, error) {
	if s == "" {
		return IRI{}, nil
	}
	return ParseIRI(s)
}

// MustIRI is ParseIRI for IRIs written in the source — vocabulary constants,
// test fixtures. It panics on an invalid one; never pass it request input.
func MustIRI(s string) IRI {
	iri, err := ParseIRI(s)
	if err != nil {
		panic("sparql.MustIRI: " + err.Error())
	}
	return iri
}

// String returns the bare IRI, without brackets.
func (i IRI) String() string { return i.s }

// IsZero reports whether i is the zero IRI, i.e. no IRI at all.
func (i IRI) IsZero() bool { return i.s == "" }

// Term renders the IRI as a bracketed SPARQL term, "<…>".
func (i IRI) Term() string { return "<" + i.s + ">" }

// IRITerm renders s as a bracketed SPARQL IRI term "<s>" after validating it
// with ValidateIRI. It is ParseIRI and Term in one step, for a builder that
// receives a string and splices it once.
func IRITerm(s string) (string, error) {
	iri, err := ParseIRI(s)
	if err != nil {
		return "", err
	}
	return iri.Term(), nil
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
