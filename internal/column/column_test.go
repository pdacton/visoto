package column

import (
	"strings"
	"testing"

	"hutzli.org/visoto/internal/facet"
)

func TestFromAttributesNormalizes(t *testing.T) {
	got := FromAttributes(map[string]string{
		"var":   "?canton",
		"label": "Canton",
		"tip":   "The canton it belongs to",
		"path":  " schema:containedInPlace ",
		"root":  "?municipality",
	})
	if got.Var != "canton" {
		t.Errorf("Var = %q, want %q — the ? prefix is written by habit and must not reach SPARQL twice", got.Var, "canton")
	}
	if got.Path != "schema:containedInPlace" {
		t.Errorf("Path = %q, want it trimmed", got.Path)
	}
	if got.Label != "Canton" || got.Tip != "The canton it belongs to" {
		t.Errorf("label/tip lost: %+v", got)
	}
}

// A bare filter= is the compact form: it asks the frontend to pick the control
// from the data. It must be distinguishable from no filter at all, which means the
// column carries no control.
func TestFromAttributesFilterPresence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs map[string]string
		want  string
	}{
		{"absent", map[string]string{"var": "x"}, FilterNone},
		{"bare", map[string]string{"var": "x", "filter": ""}, FilterAuto},
		{"explicit", map[string]string{"var": "x", "filter": "text"}, FilterText},
		{"cased", map[string]string{"var": "x", "filter": "SELECT"}, FilterSelect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := FromAttributes(tc.attrs)
			if got.Filter != tc.want {
				t.Errorf("Filter = %q, want %q", got.Filter, tc.want)
			}
			if got.Filterable() != (tc.want != FilterNone) {
				t.Errorf("Filterable() = %v for filter %q", got.Filterable(), got.Filter)
			}
		})
	}
}

func TestFromAttributesFlags(t *testing.T) {
	on := FromAttributes(map[string]string{"var": "x", "icon": "", "badge": "true", "group": "1"})
	if !on.Icon || !on.Badge || !on.Group {
		t.Errorf("valueless/true flags did not read as on: %+v", on)
	}
	off := FromAttributes(map[string]string{"var": "x", "icon": "false", "badge": "0", "group": "off"})
	if off.Icon || off.Badge || off.Group {
		t.Errorf("explicit falsy values did not read as off: %+v", off)
	}
	none := FromAttributes(map[string]string{"var": "x"})
	if none.Icon || none.Badge || none.Group {
		t.Errorf("absent flags read as on: %+v", none)
	}
}

func TestFromAttributesLayout(t *testing.T) {
	got := FromAttributes(map[string]string{"var": "x", "hidden": "", "width": " 180px "})
	if !got.Hidden {
		t.Error("hidden did not read as on")
	}
	if got.Width != "180px" {
		t.Errorf("Width = %q, want it trimmed to %q", got.Width, "180px")
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spec    Spec
		wantErr string
	}{
		{"ok bare", Spec{Var: "x"}, ""},
		{"ok filtered", Spec{Var: "x", Filter: FilterAuto}, ""},
		{"ok typed", Spec{Var: "x", Filter: FilterRange, Type: facet.TypeDate}, ""},
		{"ok hidden", Spec{Var: "x", Hidden: true}, ""},
		{"ok width px", Spec{Var: "x", Width: "180px"}, ""},
		{"ok width bare", Spec{Var: "x", Width: "180"}, ""},
		{"ok width percent", Spec{Var: "x", Width: "20%"}, ""},
		{"no var", Spec{}, "var is required"},
		{"bad filter", Spec{Var: "x", Filter: "dropdown"}, "filter="},
		{"bad type", Spec{Var: "x", Type: "uri"}, "type="},
		{"root without path", Spec{Var: "x", Root: "?key"}, "needs a path"},
		// A control hangs off the header, so hiding the column would wire up a
		// filter nobody can reach.
		{"hidden and filtered", Spec{Var: "x", Hidden: true, Filter: FilterAuto}, "contradictory"},
		{"bad width", Spec{Var: "x", Width: "wide"}, "width="},
		{"bad width unit", Spec{Var: "x", Width: "12ch"}, "width="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// The frontend resolves what the declaration left implicit and sends it back. The
// declaration always wins, and anything outside the known sets is ignored: this is
// the boundary where request input meets query construction.
func TestFacetResolution(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		spec                  Spec
		gotControl, gotType   string
		wantControl, wantType string
	}{
		{"declaration wins", Spec{Var: "x", Filter: FilterSelect, Type: facet.TypeIRI},
			"range", "number", facet.ControlSelect, facet.TypeIRI},
		{"resolution fills the gap", Spec{Var: "x", Filter: FilterAuto},
			"range", "date", facet.ControlRange, facet.TypeDate},
		{"unknown control ignored", Spec{Var: "x", Filter: FilterAuto},
			"dropdown", "iri", facet.ControlSelect, facet.TypeIRI},
		{"unknown type ignored", Spec{Var: "x", Filter: FilterAuto},
			"text", "uri", facet.ControlText, facet.TypeString},
		{"nothing resolved", Spec{Var: "x", Filter: FilterAuto},
			"", "", facet.ControlSelect, facet.TypeString},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.spec.Facet(tc.gotControl, tc.gotType)
			if got.Control != tc.wantControl {
				t.Errorf("Control = %q, want %q", got.Control, tc.wantControl)
			}
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tc.wantType)
			}
		})
	}
}

func TestFacetCarriesTheFilterPlumbing(t *testing.T) {
	spec := Spec{Var: "postalCodes", Root: "?municipality", Path: "schema:postalCode", Filter: FilterText, Label: "Postal code"}
	got := spec.Facet("", "")
	if got.Var != "postalCodes" || got.Root != "?municipality" || got.Path != "schema:postalCode" {
		t.Errorf("Facet() dropped the anchor/path: %+v", got)
	}
	if got.Label != "Postal code" {
		t.Errorf("Label = %q, want it carried through for the control's tooltip", got.Label)
	}
}

func TestTablePresentation(t *testing.T) {
	tbl := Table{
		{Var: "type", Icon: true},
		{Var: "kind", Badge: true, Filter: FilterAuto},
		{Var: "canton", Group: true},
		{Var: "other", Icon: true}, // icon is multi-column: both are listed
	}
	// A table listing an entity beside its parents wants the icon on every IRI
	// column, so IconVar collects them all rather than letting the first win.
	if got := tbl.IconVars(); got != "type,other" {
		t.Errorf("IconVars() = %q, want %q", got, "type,other")
	}
	if got := tbl.BadgeVars(); got != "kind" {
		t.Errorf("BadgeVars() = %q, want %q", got, "kind")
	}
	if got := tbl.GroupVar(); got != "canton" {
		t.Errorf("GroupVar() = %q, want %q", got, "canton")
	}
	if !tbl.Filterable() {
		t.Error("Filterable() = false, but one column declares a filter — this is what makes a table faceted")
	}
	if _, ok := tbl.Find("canton"); !ok {
		t.Error(`Find("canton") missed a declared column`)
	}
	if _, ok := tbl.Find("nope"); ok {
		t.Error(`Find("nope") found an undeclared column`)
	}
}

// Badge is the one presentation flag that is NOT a single-column role: a catalog
// may badge both a status and a version. Returning only the first (as IconVar and
// GroupVar do) rendered every later badge column as bare text, with no error —
// the reader's only clue was that the pill was missing.
func TestBadgeVarsListsEveryBadgeColumn(t *testing.T) {
	tbl := Table{
		{Var: "cube"},
		{Var: "status", Badge: true},
		{Var: "modified"},
		{Var: "superseded", Badge: true},
	}
	if got, want := tbl.BadgeVars(), "status,superseded"; got != want {
		t.Errorf("BadgeVars() = %q, want %q — a second badge column must not be dropped", got, want)
	}
}

// The empty result must stay empty rather than becoming ",": the reader splits on
// commas, and a stray empty entry would match a column whose variable is unset.
func TestBadgeVarsEmptyWithoutBadges(t *testing.T) {
	tbl := Table{{Var: "cube", Icon: true}, {Var: "modified"}}
	if got := tbl.BadgeVars(); got != "" {
		t.Errorf("BadgeVars() = %q, want \"\" when no column declares a badge", got)
	}
	// A badge flag on a declaration with no var has no column to attach to.
	if got := (Table{{Badge: true}}).BadgeVars(); got != "" {
		t.Errorf("BadgeVars() = %q, want \"\" for a badge declaration with no var", got)
	}
}

func TestTableWithoutFiltersIsNotFaceted(t *testing.T) {
	// Columns declared only to name and explain themselves must not turn the table
	// into a faceted one — that would wire up controls nobody asked for.
	tbl := Table{{Var: "name", Label: "Name"}, {Var: "note", Tip: "why"}}
	if tbl.Filterable() {
		t.Error("Filterable() = true for declarations that carry no filter")
	}
}
