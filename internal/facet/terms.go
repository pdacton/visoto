package facet

import (
	"fmt"
	"regexp"
	"strings"

	"hutzli.org/visoto/internal/sparql"
)

// This file is the security boundary of faceted search. Facet selection values
// arrive from the client (even enum IRIs, which we enumerated ourselves, are
// echoed back and must be treated as untrusted) and are folded into SPARQL query
// text. Every value that reaches a query passes through one of these builders,
// which either produce a syntactically-contained term or return an error. No
// facet value is ever string-concatenated into a query un-validated.

// numberLexical matches an xsd numeric lexical form (integer or decimal, with an
// optional exponent and sign).
var numberLexical = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// dateLexical matches an xsd:date lexical form (YYYY-MM-DD, optional leading '-'
// for BCE and an optional timezone).
var dateLexical = regexp.MustCompile(`^-?\d{4}-\d{2}-\d{2}(Z|[+-]\d{2}:\d{2})?$`)

const (
	xsdDate    = "http://www.w3.org/2001/XMLSchema#date"
	xsdDecimal = "http://www.w3.org/2001/XMLSchema#decimal"
)

// IRITerm renders s as a bracketed SPARQL IRI term "<s>" after validating it is a
// safe absolute IRI. Returns an error for anything that could break out of the
// <...> delimiters or is not an absolute IRI.
//
// Delegates to sparql.IRITerm so there is exactly ONE definition of what a safe
// IRI term is: the same check guards the class IRI behind the `??` placeholder on
// every route, and a divergence between the two would be a silent hole.
func IRITerm(s string) (string, error) {
	return sparql.IRITerm(s)
}

// StringLiteralTerm renders s as a safely quoted SPARQL string literal.
// Delegates to sparql.StringLiteral: the escaping was previously duplicated here
// to avoid an import cycle, but this package now imports internal/sparql for the
// shared IRI validation, so one definition serves both.
func StringLiteralTerm(s string) string {
	return sparql.StringLiteral(s)
}

// RangeBound renders value as a typed numeric or date literal for a range-filter
// bound, after validating value against the lexical space of facetType. An empty
// value yields ("", nil) so callers can build one-sided ranges. Only number and
// date types are valid range bounds.
func RangeBound(value, facetType string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	switch facetType {
	case TypeNumber:
		if !numberLexical.MatchString(value) {
			return "", fmt.Errorf("invalid number %q", value)
		}
		// A bare numeric literal is a valid, correctly-typed SPARQL term
		// (xsd:integer/xsd:decimal); no quoting needed and nothing to inject.
		return value, nil
	case TypeDate:
		if !dateLexical.MatchString(value) {
			return "", fmt.Errorf("invalid date %q", value)
		}
		return fmt.Sprintf(`"%s"^^<%s>`, value, xsdDate), nil
	default:
		return "", fmt.Errorf("range not supported for facet type %q", facetType)
	}
}

// EnumTerm renders one selected enum value as an RDF term appropriate to the
// facet's type: an IRI term for iri facets, a string literal otherwise.
func EnumTerm(value, facetType string) (string, error) {
	if facetType == TypeIRI {
		return IRITerm(value)
	}
	return StringLiteralTerm(value), nil
}
