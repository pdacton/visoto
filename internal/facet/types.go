// Package facet implements backend faceted search over a declared class-instance
// SPARQL query. It mirrors the store-search abstraction in internal/search: a
// registry of FacetProvider implementations selected once per deployment, with a
// portable BaseFacetProvider that works on any SPARQL 1.1 store. Store-specific
// index optimizations (native FTS text legs, fast facet counts) slot in behind
// the same interface later.
//
// Two phases:
//
//   - Enumerate (BuildFacetValuesQuery): DISTINCT facet values + counts, to
//     populate the UI controls.
//   - Filter (BuildFacetedQuery): rewrite the declared instance query, injecting
//     one FILTER EXISTS block per active facet selection, then hand the result to
//     the existing async-table render path.
//
// Everything here is a pure function over strings / QueryResult (no HTTP, config,
// or endpoint access), so it is unit-testable in isolation.
package facet

import (
	"fmt"
	"strings"
)

// Control types (the "filter" attribute on a <sparql-column> element, or what the
// frontend resolved from the data when that attribute was left bare).
const (
	ControlSelect = "select" // dropdown of enumerated values → VALUES / IN
	ControlText   = "text"   // free-text box → CONTAINS (store FTS override point)
	ControlRange  = "range"  // min/max inputs → typed range FILTER
)

// Term types (the "type" attribute). Decides how a selected value is turned into
// a safe RDF term.
const (
	TypeIRI    = "iri"    // enum values are IRIs
	TypeString = "string" // enum/text values are plain string literals
	TypeNumber = "number" // range bounds are numeric literals
	TypeDate   = "date"   // range bounds are xsd:date literals
)

// InstanceRoot is the Root value meaning "the page resource itself" — the rows
// hang off one fixed IRI rather than one entity per row. Written as "??" in the
// template, mirroring the placeholder the declared query uses for the same IRI.
const InstanceRoot = "??"

// FacetSpec is a declared facet: what it hangs off, how to reach a value, and how
// to expose/constrain it. Projected from a <sparql-column> declaration by
// column.Spec.Facet. Var, Root and Path are author-supplied (trusted template
// input) and inserted verbatim; Control and Type may instead come from what the
// frontend resolved, which that projection whitelists first.
//
// Root selects one of three modes:
//
//   - Root == "" and Path == "" — COLUMN mode. The facet filters the projected
//     variable directly, so it works on any query regardless of shape. Values are
//     enumerated by re-running the declared query (BuildColumnValuesQuery).
//   - Path != "" — CLASS mode. The facet anchors on one entity per row (Root, or
//     the sniffed key var when Root is empty, for backwards compatibility) and
//     filters existentially, so a multi-valued path keeps the whole entity.
//     Values enumerate cheaply from the membership triple (BuildFacetValuesQuery).
//   - Root == InstanceRoot — INSTANCE mode. Rows hang off one fixed resource.
//     Filtering is identical to COLUMN mode (a constant pattern cannot discriminate
//     between rows); only enumeration differs (BuildInstanceValuesQuery).
//
// Root without Path is rejected: there is nothing to walk from the root to a value.
type FacetSpec struct {
	Var     string // SPARQL variable name bound to the facet value (e.g. "rank")
	Root    string // what the facet hangs off: "" | "?var" | InstanceRoot
	Path    string // property path from the root to the value (e.g. "dwc:taxonRank")
	Type    string // term type: iri | string | number | date
	Control string // UI control: select | text | range
	Label   string // display label
}

// Validate reports whether the declared attribute combination is coherent. Called
// where specs are read so a malformed declaration fails loudly instead of silently
// degrading to a mode the author did not ask for.
func (s FacetSpec) Validate() error {
	if strings.TrimSpace(s.Var) == "" {
		return fmt.Errorf("facet: var is required")
	}
	if strings.TrimSpace(s.Root) != "" && strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("facet %q: root=%q needs a path — there is nothing to walk from the root to a value", s.Var, s.Root)
	}
	return nil
}

// NoValueSentinel is the reserved value a select facet sends to mean "members that
// lack any value for this facet" (→ FILTER NOT EXISTS). It is deliberately not a
// legal IRI or a value the store could ever enumerate, so it can never collide with
// a real selection. It is intercepted before term rendering and never reaches
// EnumTerm/IRITerm.
const NoValueSentinel = "__vs_no_value__"

// FacetConstraint is an active user selection against a FacetSpec. Values carries
// the (untrusted, revalidated) end-user input:
//
//   - select: one or more chosen enum values (IRIs or string literals); may also
//     include NoValueSentinel to match members lacking a value (OR-combined)
//   - text:   a single search term
//   - range:  exactly two entries [min, max]; either may be "" for a one-sided range
//
// NoValue asks (for ANY control) to also match members lacking any value on the
// facet path (→ FILTER NOT EXISTS, OR-combined with the concrete match). For select
// facets this may instead arrive as NoValueSentinel inside Values, because the
// multi-select checkbox list sends "(no value)" as just another repeated f.<var>
// value; range/text carry it out-of-band in this flag so their positional Values
// stay clean. HasNoValue() unifies the two.
type FacetConstraint struct {
	Spec    FacetSpec
	Values  []string
	NoValue bool
}

// HasNoValue reports whether the selection asks to match members lacking any value
// on the facet path — either via the explicit NoValue flag (range/text) or the
// "(no value)" sentinel mixed into Values (select).
func (c FacetConstraint) HasNoValue() bool {
	if c.NoValue {
		return true
	}
	for _, v := range c.Values {
		if v == NoValueSentinel {
			return true
		}
	}
	return false
}

// ConcreteValues returns the selected values with the "(no value)" sentinel removed
// — the values that map to actual RDF terms.
func (c FacetConstraint) ConcreteValues() []string {
	out := make([]string, 0, len(c.Values))
	for _, v := range c.Values {
		if v != NoValueSentinel {
			out = append(out, v)
		}
	}
	return out
}
