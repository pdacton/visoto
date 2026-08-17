// Package column models a <sparql-column> declaration: the template vocabulary
// describing ONE column of a SPARQL result table — what it is called, what it
// explains, how it is rendered, and whether it can be filtered.
//
// It generalizes <sparql-facet>, which described only the filter. That element was
// already column-scoped in everything but its name: its var had to be a projected
// column and its control hung off that column's header. What it could not do was
// name the column — headers rendered the raw SPARQL variable — so the label an
// author wrote reached only the funnel button's tooltip.
//
// Declarations are author-supplied (trusted template input), read once at startup
// and never mutated afterwards. The store still only ever sees what internal/facet
// builds; Facet() is the projection onto that package's spec.
//
// Two attributes are deliberately optional, because the frontend can answer them
// better than the author can: an omitted filter kind and an omitted type are
// resolved from the loaded rows (see static/js/sparql-table.js). The resolved pair
// travels back as request input and is whitelisted here, so it can only ever select
// among known controls and coercions.
package column

import (
	"fmt"
	"regexp"
	"strings"

	"hutzli.org/visoto/internal/facet"
)

// Filter kinds — the "filter" attribute. Absent means the column carries no control
// at all (a declaration that only names or explains the column). Present but empty
// means "infer from the data", which only the frontend can do.
const (
	FilterNone   = ""
	FilterAuto   = "auto"
	FilterSelect = facet.ControlSelect
	FilterText   = facet.ControlText
	FilterRange  = facet.ControlRange
)

// Spec is one declared column.
type Spec struct {
	Var    string // SPARQL variable bound to the cell value; the Tabulator field
	Label  string // column header title; the var name when empty
	Tip    string // header tooltip
	Filter string // FilterNone | FilterAuto | select | text | range
	Type   string // facet.Type*; empty means the frontend resolves it from the data
	Root   string // facet anchor: "" | "?var" | facet.InstanceRoot
	Path   string // property path from the root to the filtered value
	Icon   bool   // render this column's IRI as a resource icon
	Badge  bool   // render this column's value as a badge
	Group  bool   // group the table by this column initially
	Hidden bool   // keep the variable (grouping, exports) but don't show the column
	Width  string // fixed column width: "180", "180px", "20%"
}

// FromAttributes reads one <sparql-column> element's attributes. It normalizes
// only; coherence is Validate's job, so a malformed declaration is reported with
// the file that holds it rather than silently degraded.
func FromAttributes(attrs map[string]string) Spec {
	s := Spec{
		Var:    strings.TrimPrefix(strings.TrimSpace(attrs["var"]), "?"),
		Label:  attrs["label"],
		Tip:    attrs["tip"],
		Type:   strings.TrimSpace(attrs["type"]),
		Root:   strings.TrimSpace(attrs["root"]),
		Path:   strings.TrimSpace(attrs["path"]),
		Icon:   flagAttr(attrs, "icon"),
		Badge:  flagAttr(attrs, "badge"),
		Group:  flagAttr(attrs, "group"),
		Hidden: flagAttr(attrs, "hidden"),
		Width:  strings.TrimSpace(attrs["width"]),
	}
	// Presence is the signal: a bare filter= asks for inference, so it must be
	// distinguishable from no filter at all.
	if v, ok := attrs["filter"]; ok {
		s.Filter = strings.ToLower(strings.TrimSpace(v))
		if s.Filter == "" {
			s.Filter = FilterAuto
		}
	}
	return s
}

// flagAttr reads a valueless boolean attribute. Presence means on, unless it
// carries an explicit falsy value — so a template can switch one off from a
// condition without restructuring the element.
func flagAttr(attrs map[string]string, name string) bool {
	v, ok := attrs[name]
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

// Validate reports whether the declared attribute combination is coherent. Called
// where declarations are indexed, so a typo fails startup instead of producing a
// column that quietly does nothing.
func (s Spec) Validate() error {
	if strings.TrimSpace(s.Var) == "" {
		return fmt.Errorf("column: var is required")
	}
	switch s.Filter {
	case FilterNone, FilterAuto, FilterSelect, FilterText, FilterRange:
	default:
		return fmt.Errorf("column %q: filter=%q is not one of %q, %q, %q — write a bare filter to infer it from the data",
			s.Var, s.Filter, FilterSelect, FilterText, FilterRange)
	}
	if s.Type != "" && !knownType(s.Type) {
		return fmt.Errorf("column %q: type=%q is not one of %q, %q, %q, %q",
			s.Var, s.Type, facet.TypeIRI, facet.TypeString, facet.TypeNumber, facet.TypeDate)
	}
	// A control hangs off the column header, and a hidden column has none — the
	// filter would be declared, wired and unreachable.
	if s.Hidden && s.Filterable() {
		return fmt.Errorf("column %q: hidden and filter= are contradictory — a hidden column has no header to hang the control off; "+
			"drop one, or keep the column and give it a narrow width=", s.Var)
	}
	if s.Width != "" && !widthRe.MatchString(s.Width) {
		return fmt.Errorf("column %q: width=%q is not a number of pixels (%q, %q) or a percentage (%q)",
			s.Var, s.Width, "180", "180px", "20%")
	}
	// Root/path coherence belongs to the facet builder — ask it rather than restate
	// the rule here, so the two can never drift.
	return s.Facet("", "").Validate()
}

// Filterable reports whether this column carries a filter control.
func (s Spec) Filterable() bool { return s.Filter != FilterNone }

// Facet projects the declaration onto the spec internal/facet builds queries from.
//
// resolvedControl and resolvedType are what the frontend worked out from the loaded
// rows. They fill ONLY what the declaration left implicit, and only when they name a
// known control/type: request input therefore selects among fixed coercions and can
// never reach query text. A declaration always wins over them.
func (s Spec) Facet(resolvedControl, resolvedType string) facet.FacetSpec {
	control := s.Filter
	if control == FilterNone || control == FilterAuto {
		control = ""
		switch resolvedControl {
		case FilterSelect, FilterText, FilterRange:
			control = resolvedControl
		}
	}
	if control == "" {
		control = facet.ControlSelect
	}

	typ := s.Type
	if typ == "" && knownType(resolvedType) {
		typ = resolvedType
	}
	if typ == "" {
		typ = facet.TypeString
	}

	return facet.FacetSpec{
		Var:     s.Var,
		Root:    s.Root,
		Path:    s.Path,
		Type:    typ,
		Control: control,
		Label:   s.Label,
	}
}

// widthRe accepts what Tabulator's column width option understands, and nothing
// else: a bare pixel count, an explicit unit, or a percentage. Validated here so a
// typo fails at startup rather than producing a column of no width at all.
var widthRe = regexp.MustCompile(`^\d+(px|%|em|rem)?$`)

func knownType(t string) bool {
	switch t {
	case facet.TypeIRI, facet.TypeString, facet.TypeNumber, facet.TypeDate:
		return true
	}
	return false
}

// Table is a table's declared columns, in document order.
type Table []Spec

// IconVars returns the variables rendered with a resource icon as a comma-separated
// list, or "". Like badge (and unlike grouping), icon is not a single-column role:
// a table listing municipalities with their district and canton wants the icon on
// all three, and returning only the first silently dropped the rest. The list is
// joined into one string because that is the shape the template island and the
// Tabulator config both read.
func (t Table) IconVars() string { return t.flaggedVars(func(s Spec) bool { return s.Icon }) }

// BadgeVars returns the variables rendered as badges as a comma-separated list,
// or "". Not a single-column role either: a table may well want a badge on both
// a status and a version column. See IconVars for the wire format.
func (t Table) BadgeVars() string { return t.flaggedVars(func(s Spec) bool { return s.Badge }) }

// flaggedVars returns the vars of every column matching pick, comma-separated.
// Used by the roles a table may fill more than once; contrast firstFlagged.
func (t Table) flaggedVars(pick func(Spec) bool) string {
	var vars []string
	for _, s := range t {
		if pick(s) && s.Var != "" {
			vars = append(vars, s.Var)
		}
	}
	return strings.Join(vars, ",")
}

// GroupVar returns the variable the table is initially grouped by, or "".
func (t Table) GroupVar() string { return t.firstFlagged(func(s Spec) bool { return s.Group }) }

// Filterable reports whether any declared column carries a filter control — which
// is what makes a table faceted. Authors no longer say so a second time.
func (t Table) Filterable() bool {
	for _, s := range t {
		if s.Filterable() {
			return true
		}
	}
	return false
}

// Find returns the column declared for varName, if any.
func (t Table) Find(varName string) (Spec, bool) {
	for _, s := range t {
		if s.Var == varName {
			return s, true
		}
	}
	return Spec{}, false
}

// firstFlagged returns the var of the first column matching pick. These flags name
// a single column each, so a second one is ignored rather than fought over.
func (t Table) firstFlagged(pick func(Spec) bool) string {
	for _, s := range t {
		if pick(s) {
			return s.Var
		}
	}
	return ""
}
