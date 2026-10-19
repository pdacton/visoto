// Package shapeprose explains a SHACL shape in plain language: one short line
// per property, built from fixed phrases so it translates safely.
//
// The Go side never produces finished text. It reduces a shape to Parts — a
// phrase kind plus the values the phrase is about — and the shapeSummary
// partial picks the translated phrase for each kind and renders the values
// after it as links or chips. No markup ever goes into a translated string.
//
// The input is a bnodetree.Graph fetched with RootTriples and WithProperties,
// so a shape, its property shapes (blank or IRI) and their nested structures
// come from one result set.
package shapeprose

import (
	"hutzli.org/visoto/internal/bnodetree"
	"hutzli.org/visoto/internal/config"
)

// Options steer the explanation.
type Options struct {
	Lang     string          // preferred language for names and descriptions; also the language names' language
	Prefixes []config.Prefix // for CURIEs
}

// Item is one value inside a Part: a code with its own phrase (a datatype, a
// node kind, a time unit), an IRI, or a literal text. Sep tells the template
// what to put before it: "" (first), "comma", "or" or "and".
type Item struct {
	Sep  string
	Code string
	Term *bnodetree.Term
	Text string
}

// Part is one phrase of a line. Kind picks the phrase, Code its variant, N and
// M are the numbers it mentions, Items the values rendered after it.
type Part struct {
	Kind  string
	Code  string
	N, M  int
	Items []Item
	More  int    // values not listed (sh:in beyond the first ones)
	Inner []Part // qualified and negated constraints: rendered after the phrase
}

// Fallback is a predicate no rule explains, shown as "Also: {CURIE} {values}"
// so that nothing is dropped silently.
type Fallback struct {
	Pred  bnodetree.Term
	Items []Item
}

// Line explains one property (all property shapes with the same path, merged)
// or one alternative of a logical constraint.
type Line struct {
	Label        string
	Path         string          // the path in CURIE notation ("^dcat:inSeries", "a/b")
	PathTerm     *bnodetree.Term // set for a plain predicate path, linked in the template
	Parts        []Part
	Descriptions []string
	Message      string // sh:message, shown as a tooltip
	Severity     string // "warning" or "info"; "" for the default (violation)
	Deactivated  bool
	Group        string // sh:group label

	Groups []Group // logical alternatives and nested property lines

	Fallbacks []Fallback

	order    float64
	hasOrder bool
}

// Group is a set of child lines under a caption: the alternatives of sh:or
// ("anyOf"), sh:xone ("exactlyOneOf"), sh:and ("allOf"), a negated shape
// ("not"), or the property lines of a nested shape ("nested"). "deeper" marks
// structure below the depth explained here, with no lines.
type Group struct {
	Kind  string
	Lines []*Line
}

// Summary is what the shapeSummary card renders.
type Summary struct {
	Targets       []Part // whom the shape applies to
	Self          *Line  // constraints on the focus node itself, nil when none
	Lines         []*Line
	Closed        bool
	Ignored       []Item // sh:ignoredProperties of a closed shape
	PropertyShape bool   // the shape is a property shape: one line, no targets
	Cut           bool   // the structure is deeper than was fetched
}

// Empty reports a resource that is no shape at all.
func (s *Summary) Empty() bool {
	return len(s.Targets) == 0 && s.Self == nil && len(s.Lines) == 0 && !s.Closed
}
