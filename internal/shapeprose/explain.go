package shapeprose

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"hutzli.org/visoto/internal/bnodetree"
	"hutzli.org/visoto/internal/sparql"
)

const (
	// maxDepth bounds the nesting explained: a property line (0), the
	// alternatives or nested shape under it (1), and one more level (2).
	// Anything deeper is left to the nested-structure card.
	maxDepth = 2
	// inShown is how many sh:in members are listed before "+ n more".
	inShown = 8
)

// Explain reduces the shape shapeIRI in g to a Summary. A property shape (one
// carrying sh:path) gives a single line; a node shape gives its targets, one
// line per property path, and the constraints on the focus node itself.
func Explain(g *bnodetree.Graph, shapeIRI string, opts Options) *Summary {
	e := &explainer{g: g, opts: opts, langs: display.Tags(language.Make(opts.Lang))}
	s := &Summary{}
	edges := g.Out(iriBinding(shapeIRI))
	if len(edges) == 0 {
		return s
	}
	if hasPred(edges, sh+"path") {
		s.PropertyShape = true
		s.Lines = []*Line{e.line([][]bnodetree.Edge{edges}, 0)}
		return s
	}
	s.Targets = e.targets(edges)
	for _, ed := range edges {
		switch ed.P.Value {
		case sh + "closed":
			s.Closed = ed.O.Value == "true" || ed.O.Value == "1"
		case sh + "ignoredProperties":
			s.Ignored = e.listItems(ed.O, "and")
		}
	}
	s.Lines = e.propertyLines(edges, 0)
	if c := e.constraints([][]bnodetree.Edge{edges}, 0, true); !c.empty() {
		s.Self = &Line{Parts: c.parts, Groups: c.groups, Fallbacks: c.fallbacks}
	}
	if !s.Closed {
		s.Ignored = nil
	}
	return s
}

type explainer struct {
	g     *bnodetree.Graph
	opts  Options
	langs display.Namer
}

func iriBinding(v string) sparql.Binding { return sparql.Binding{Type: "uri", Value: v} }

func hasPred(edges []bnodetree.Edge, p string) bool {
	for _, e := range edges {
		if e.P.Value == p {
			return true
		}
	}
	return false
}

// --- targets ----------------------------------------------------------------

func (e *explainer) targets(edges []bnodetree.Edge) []Part {
	kinds := []struct{ pred, kind string }{
		{sh + "targetClass", "targetClass"},
		{sh + "targetNode", "targetNode"},
		{sh + "targetSubjectsOf", "targetSubjectsOf"},
		{sh + "targetObjectsOf", "targetObjectsOf"},
	}
	var out []Part
	for _, k := range kinds {
		var items []Item
		for _, ed := range edges {
			if ed.P.Value == k.pred {
				items = append(items, e.item(ed.O))
			}
		}
		if len(items) > 0 {
			out = append(out, Part{Kind: k.kind, Items: join(dedupe(items), "and")})
		}
	}
	// A shape that is also a class targets its own instances (SHACL §2.1.3.3).
	for _, ed := range edges {
		if ed.P.Value == rdfType && (ed.O.Value == rdfs+"Class" || ed.O.Value == owl+"Class") {
			out = append(out, Part{Kind: "implicitClass"})
			break
		}
	}
	return out
}

// --- property lines -----------------------------------------------------------

// propertyLines explains the sh:property shapes of a node, one line per path:
// a shape assembled from several files (DCAT-AP's range and cardinality files
// share shape IRIs) has two or three property shapes per path, and they are one
// property to the reader.
func (e *explainer) propertyLines(edges []bnodetree.Edge, depth int) []*Line {
	byPath := map[string][][]bnodetree.Edge{}
	var keys []string
	for _, ed := range edges {
		if ed.P.Value != sh+"property" {
			continue
		}
		pe := e.g.Out(ed.O)
		if len(pe) == 0 {
			continue
		}
		key := "?" + ed.O.Value // no path: a line of its own
		for _, x := range pe {
			if x.P.Value == sh+"path" {
				key, _, _ = e.path(x.O)
				break
			}
		}
		if _, ok := byPath[key]; !ok {
			keys = append(keys, key)
		}
		byPath[key] = append(byPath[key], pe)
	}
	lines := make([]*Line, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, e.line(byPath[k], depth))
	}
	sortLines(lines)
	return lines
}

func sortLines(lines []*Line) {
	sort.SliceStable(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		if a.hasOrder != b.hasOrder {
			return a.hasOrder
		}
		if a.hasOrder && a.order != b.order {
			return a.order < b.order
		}
		if la, lb := strings.ToLower(a.Label), strings.ToLower(b.Label); la != lb {
			return la < lb
		}
		return a.Path < b.Path
	})
}

// line explains one property from all its property shapes (sets).
func (e *explainer) line(sets [][]bnodetree.Edge, depth int) *Line {
	var merged []bnodetree.Edge
	for _, s := range sets {
		merged = append(merged, s...)
	}
	l := &Line{}
	var labelTerm *bnodetree.Term
	inverse := false
	for _, ed := range merged {
		if ed.P.Value == sh+"path" {
			var simple bool
			l.Path, labelTerm, simple = e.path(ed.O)
			if simple {
				l.PathTerm = labelTerm
			}
			inverse = strings.HasPrefix(l.Path, "^") && !strings.ContainsAny(l.Path[1:], "/|()*+?^")
			break
		}
	}
	if name := e.pickText(merged, namePreds); name != "" {
		l.Label = capitalize(name)
	} else if labelTerm != nil {
		l.Label = labelTerm.Label
	}
	for _, s := range sets {
		if d := e.pickText(s, descPreds); d != "" && !contains(l.Descriptions, d) {
			l.Descriptions = append(l.Descriptions, d)
		}
	}
	l.Message = e.pickText(merged, []string{sh + "message"})
	l.Severity = severity(sets)
	for _, ed := range merged {
		switch ed.P.Value {
		case sh + "deactivated":
			l.Deactivated = l.Deactivated || ed.O.Value == "true"
		case sh + "order":
			if f, err := strconv.ParseFloat(ed.O.Value, 64); err == nil && (!l.hasOrder || f < l.order) {
				l.order, l.hasOrder = f, true
			}
		case sh + "group":
			if l.Group == "" {
				l.Group = e.term(ed.O).Label
			}
		}
	}
	c := e.constraints(sets, depth, false)
	l.Parts, l.Groups, l.Fallbacks = c.parts, c.groups, c.fallbacks
	if inverse {
		// Right after the cardinality: "at least one · referenced from elsewhere".
		at := 0
		for i, p := range l.Parts {
			if p.Kind == "card" || p.Kind == "dimension" {
				at = i + 1
			}
		}
		l.Parts = append(l.Parts[:at], append([]Part{{Kind: "inverse"}}, l.Parts[at:]...)...)
	}
	return l
}

// severity is "warning" or "info" when every property shape of the line says so;
// the default (sh:Violation) and mixed severities show no badge.
func severity(sets [][]bnodetree.Edge) string {
	out := ""
	for i, s := range sets {
		sev := "violation"
		for _, ed := range s {
			if ed.P.Value == sh+"severity" {
				switch ed.O.Value {
				case sh + "Warning":
					sev = "warning"
				case sh + "Info":
					sev = "info"
				}
			}
		}
		if i == 0 {
			out = sev
		} else if sev != out {
			return ""
		}
	}
	if out == "violation" {
		return ""
	}
	return out
}

// --- constraints ------------------------------------------------------------

type constraintSet struct {
	parts     []Part
	groups    []Group
	fallbacks []Fallback
}

func (c constraintSet) empty() bool {
	return len(c.parts) == 0 && len(c.groups) == 0 && len(c.fallbacks) == 0
}

// partOrder fixes the order of parts on a line, whatever the predicate order.
var partOrder = []string{
	"dimension", "card", "class", "datatype", "nodeKind", "dataKind", "unit", "scale",
	"in", "hasValue", "range", "min", "minEx", "max", "maxEx", "length", "minLength", "maxLength",
	"pattern", "languageIn", "uniqueLang", "qualified", "qualifiedShape",
	"equals", "disjoint", "lessThan", "lessThanOrEquals", "node", "nodeInline", "not",
	"relation", "hierarchy", "default", "deeper",
}

func partRank(kind string) int {
	for i, k := range partOrder {
		if k == kind {
			return i
		}
	}
	return len(partOrder)
}

// constraints explains the constraint predicates of one property (or of the
// focus node when shapeLevel is set; there only sh: predicates count, the rest
// being the shape's own metadata). sets are the property shapes merged into the
// line: counts combine to the strictest, values are deduplicated.
func (e *explainer) constraints(sets [][]bnodetree.Edge, depth int, shapeLevel bool) constraintSet {
	var c constraintSet
	var merged []bnodetree.Edge
	for _, s := range sets {
		merged = append(merged, s...)
	}
	add := func(p Part) { c.parts = append(c.parts, p) }

	var (
		minC, maxC       = -1, -1
		classes, dts     []Item
		kinds, dims      []string
		units, hasValues []Item
		pairs            = map[string][]Item{}
		bounds           = map[string]string{}
		minLen, maxLen   = -1, -1
	)
	byPred := map[string][]bnodetree.Edge{}
	for _, ed := range merged {
		byPred[ed.P.Value] = append(byPred[ed.P.Value], ed)
	}
	for _, ed := range merged {
		p, o := ed.P.Value, ed.O
		switch {
		case ignored[p], contains(namePreds, p), contains(descPreds, p), strings.HasPrefix(p, cubeCreatorNS):
			continue
		case shapeLevel && !strings.HasPrefix(p, sh):
			continue
		}
		switch p {
		case rdfType:
			if code, ok := dimensionCodes[o.Value]; ok && !contains(dims, code) {
				dims = append(dims, code)
			}
		case sh + "minCount":
			if n, err := strconv.Atoi(o.Value); err == nil && n > minC {
				minC = n
			}
		case sh + "maxCount":
			if n, err := strconv.Atoi(o.Value); err == nil && (maxC < 0 || n < maxC) {
				maxC = n
			}
		case sh + "class":
			classes = append(classes, e.item(o))
		case sh + "datatype":
			dts = append(dts, e.datatypeItem(o))
		case sh + "nodeKind":
			if code, ok := nodeKindCodes[o.Value]; ok && !contains(kinds, code) {
				kinds = append(kinds, code)
			}
		case sh + "in":
			add(e.inPart(o))
		case sh + "hasValue":
			hasValues = append(hasValues, e.item(o))
		case sh + "minInclusive", sh + "minExclusive", sh + "maxInclusive", sh + "maxExclusive":
			bounds[strings.TrimPrefix(p, sh)] = o.Value
		case sh + "minLength":
			minLen, _ = strconv.Atoi(o.Value)
		case sh + "maxLength":
			maxLen, _ = strconv.Atoi(o.Value)
		case sh + "pattern":
			pat := Item{Text: o.Value}
			if fl := byPred[sh+"flags"]; len(fl) > 0 {
				pat.Code = fl[0].O.Value
			}
			add(Part{Kind: "pattern", Items: []Item{pat}})
		case sh + "languageIn":
			add(Part{Kind: "languageIn", Items: join(e.languageItems(o), "or")})
		case sh + "uniqueLang":
			if o.Value == "true" {
				add(Part{Kind: "uniqueLang"})
			}
		case sh + "equals", sh + "disjoint", sh + "lessThan", sh + "lessThanOrEquals":
			k := strings.TrimPrefix(p, sh)
			pairs[k] = append(pairs[k], e.item(o))
		case sh + "node":
			e.nodeConstraint(&c, o, depth)
		case sh + "qualifiedValueShape":
			// Read per set below: the counts belong to the shape that states them.
		case sh + "or", sh + "xone", sh + "and":
			e.logic(&c, strings.TrimPrefix(p, sh), o, depth)
		case sh + "not":
			e.negation(&c, o, depth)
		case sh + "defaultValue":
			add(Part{Kind: "default", Items: []Item{e.item(o)}})
		case qudt + "scaleType":
			if code, ok := scaleCodes[o.Value]; ok {
				add(Part{Kind: "scale", Code: code})
			} else {
				add(Part{Kind: "scale", Items: []Item{e.item(o)}})
			}
		case qudt + "unit", qudt + "hasUnit":
			units = append(units, e.item(o))
		case meta + "dataKind":
			add(e.dataKind(o))
		case meta + "dimensionRelation":
			add(e.relationPart(o))
		case meta + "inHierarchy", meta + "hasHierarchy", cube + "inHierarchy":
			add(e.hierarchyPart(o))
		default:
			c.fallbacks = appendFallback(c.fallbacks, e.term(ed.P), e.item(o))
		}
	}

	// Qualified value shapes, per property shape.
	for _, s := range sets {
		for _, ed := range s {
			if ed.P.Value == sh+"qualifiedValueShape" {
				add(e.qualified(s, ed.O, depth))
			}
		}
	}

	for _, d := range dims {
		add(Part{Kind: "dimension", Code: d})
	}
	if !shapeLevel {
		add(cardinality(minC, maxC))
	}
	if items := dedupe(classes); len(items) > 0 {
		add(Part{Kind: "class", Items: join(items, "and")})
	}
	if items := dedupe(dts); len(items) > 0 {
		add(Part{Kind: "datatype", Items: join(items, "and")})
	}
	// A node kind says nothing a class or datatype does not already say:
	// "of type Agent" implies a resource, "a date" a value.
	if len(classes) == 0 && len(dts) == 0 {
		for _, k := range kinds {
			add(Part{Kind: "nodeKind", Items: []Item{{Code: k}}})
		}
	}
	if items := dedupe(units); len(items) > 0 {
		add(Part{Kind: "unit", Items: join(items, "or")})
	}
	if items := dedupe(hasValues); len(items) > 0 {
		add(Part{Kind: "hasValue", Items: join(items, "and")})
	}
	if lo, hi := bounds["minInclusive"], bounds["maxInclusive"]; lo != "" && lo == hi {
		add(Part{Kind: "hasValue", Items: []Item{{Text: lo}}})
		delete(bounds, "minInclusive")
		delete(bounds, "maxInclusive")
	} else if lo != "" && hi != "" {
		add(Part{Kind: "range", Items: []Item{{Text: lo}, {Sep: "to", Text: hi}}})
		delete(bounds, "minInclusive")
		delete(bounds, "maxInclusive")
	}
	for _, b := range []struct{ key, kind string }{
		{"minInclusive", "min"}, {"minExclusive", "minEx"}, {"maxInclusive", "max"}, {"maxExclusive", "maxEx"},
	} {
		if v := bounds[b.key]; v != "" {
			add(Part{Kind: b.kind, Items: []Item{{Text: v}}})
		}
	}
	switch {
	case minLen >= 0 && maxLen >= 0:
		add(Part{Kind: "length", N: minLen, M: maxLen})
	case minLen >= 0:
		add(Part{Kind: "minLength", N: minLen})
	case maxLen >= 0:
		add(Part{Kind: "maxLength", M: maxLen})
	}
	for _, k := range []string{"equals", "disjoint", "lessThan", "lessThanOrEquals"} {
		if items := dedupe(pairs[k]); len(items) > 0 {
			add(Part{Kind: k, Items: join(items, "and")})
		}
	}
	sort.SliceStable(c.parts, func(i, j int) bool { return partRank(c.parts[i].Kind) < partRank(c.parts[j].Kind) })
	return c
}

// cardinality combines sh:minCount and sh:maxCount into one phrase.
func cardinality(minC, maxC int) Part {
	required := minC >= 1
	switch {
	case maxC == 0:
		return Part{Kind: "card", Code: "none"}
	case required && maxC == minC && minC == 1:
		return Part{Kind: "card", Code: "exactlyOne"}
	case required && maxC == minC:
		return Part{Kind: "card", Code: "exactly", N: minC}
	case required && maxC > 0:
		return Part{Kind: "card", Code: "range", N: minC, M: maxC}
	case required && minC == 1:
		return Part{Kind: "card", Code: "oneOrMore"}
	case required:
		return Part{Kind: "card", Code: "atLeast", N: minC}
	case maxC == 1:
		return Part{Kind: "card", Code: "atMostOne"}
	case maxC > 1:
		return Part{Kind: "card", Code: "atMost", M: maxC}
	}
	return Part{Kind: "card", Code: "any"}
}

// --- logical constraints ------------------------------------------------------

// flatPreds are the single constraints an alternative may consist of for an
// sh:or to read as one phrase: "a date or a date and time", "each a Catalog,
// Dataset or Data service".
var flatPreds = map[string]string{
	sh + "class":    "class",
	sh + "datatype": "datatype",
	sh + "node":     "node",
	sh + "nodeKind": "nodeKind",
}

// logic explains sh:or / sh:xone / sh:and, whose object is a list of shapes.
func (e *explainer) logic(c *constraintSet, op string, list sparql.Binding, depth int) {
	all, ok := e.g.List(list)
	// Alternatives without a single triple say nothing and are skipped. The
	// DCAT-AP shapes loaded into the local QLever (2026-10-19) have such
	// members, plus stray sh:or values that are no list.
	var members []sparql.Binding
	for _, m := range all {
		if !bnodetree.IsNode(m) || len(e.g.Out(m)) > 0 {
			members = append(members, m)
		}
	}
	if !ok || len(members) == 0 {
		c.fallbacks = appendFallback(c.fallbacks, e.term(iriBinding(sh+op)), Item{Code: "structure"})
		return
	}
	if op != "and" {
		if p, ok := e.flatten(members); ok {
			c.parts = append(c.parts, p)
			return
		}
	}
	kind := map[string]string{"or": "anyOf", "xone": "exactlyOneOf", "and": "allOf"}[op]
	if depth >= maxDepth {
		c.groups = append(c.groups, Group{Kind: "deeper"})
		return
	}
	g := Group{Kind: kind}
	for _, m := range members {
		g.Lines = append(g.Lines, e.alternative(m, depth+1))
	}
	c.groups = append(c.groups, g)
}

// flatten turns alternatives that are each a single constraint of the same kind
// into one part with "or" between the values.
func (e *explainer) flatten(members []sparql.Binding) (Part, bool) {
	kind := ""
	var items []Item
	for _, m := range members {
		var edges []bnodetree.Edge
		for _, ed := range e.g.Out(m) {
			if ed.P.Value == rdfType || ignored[ed.P.Value] || contains(namePreds, ed.P.Value) || contains(descPreds, ed.P.Value) {
				continue
			}
			edges = append(edges, ed)
		}
		if len(edges) != 1 {
			return Part{}, false
		}
		k, ok := flatPreds[edges[0].P.Value]
		if !ok || (kind != "" && k != kind) {
			return Part{}, false
		}
		kind = k
		switch k {
		case "datatype":
			items = append(items, e.datatypeItem(edges[0].O))
		case "nodeKind":
			items = append(items, Item{Code: nodeKindCodes[edges[0].O.Value]})
		case "node":
			if bnodetree.IsNode(edges[0].O) {
				return Part{}, false
			}
			items = append(items, e.item(edges[0].O))
		default:
			items = append(items, e.item(edges[0].O))
		}
	}
	if kind == "" {
		return Part{}, false
	}
	return Part{Kind: kind, Items: join(dedupe(items), "or")}, true
}

// alternative explains one member of a logical constraint: a property shape
// (it has a path) or a set of constraints on the same value.
func (e *explainer) alternative(m sparql.Binding, depth int) *Line {
	edges := e.g.Out(m)
	if hasPred(edges, sh+"path") {
		return e.line([][]bnodetree.Edge{edges}, depth)
	}
	l := &Line{}
	if !bnodetree.IsNode(m) {
		t := e.term(m)
		l.Label, l.PathTerm = t.Label, &t
	}
	c := e.constraints([][]bnodetree.Edge{edges}, depth, false)
	// An alternative restates no cardinality of its own: drop "any number".
	for _, p := range c.parts {
		if p.Kind == "card" && p.Code == "any" {
			continue
		}
		l.Parts = append(l.Parts, p)
	}
	l.Groups, l.Fallbacks = c.groups, c.fallbacks
	if nested := e.propertyLines(edges, depth); len(nested) > 0 {
		l.Groups = append(l.Groups, Group{Kind: "nested", Lines: nested})
	}
	return l
}

// negation explains sh:not: inline when the negated shape is a few simple
// constraints ("not: each a Draft"), as a group otherwise.
func (e *explainer) negation(c *constraintSet, o sparql.Binding, depth int) {
	if !bnodetree.IsNode(o) || len(e.g.Out(o)) == 0 {
		c.parts = append(c.parts, Part{Kind: "not", Items: []Item{e.item(o)}})
		return
	}
	alt := e.alternative(o, depth+1)
	if len(alt.Groups) == 0 && len(alt.Fallbacks) == 0 {
		c.parts = append(c.parts, Part{Kind: "not", Inner: alt.Parts})
		return
	}
	if depth >= maxDepth {
		c.groups = append(c.groups, Group{Kind: "deeper"})
		return
	}
	c.groups = append(c.groups, Group{Kind: "not", Lines: []*Line{alt}})
}

// nodeConstraint explains sh:node: a named shape is a link; an inline shape is
// explained in place, its own property shapes as a nested group.
func (e *explainer) nodeConstraint(c *constraintSet, o sparql.Binding, depth int) {
	edges := e.g.Out(o)
	if !bnodetree.IsNode(o) || len(edges) == 0 {
		c.parts = append(c.parts, Part{Kind: "node", Items: []Item{e.item(o)}})
		return
	}
	if depth >= maxDepth {
		c.groups = append(c.groups, Group{Kind: "deeper"})
		return
	}
	inner := e.constraints([][]bnodetree.Edge{edges}, depth+1, true)
	if len(inner.parts) > 0 {
		c.parts = append(c.parts, Part{Kind: "nodeInline", Inner: inner.parts})
	}
	c.groups = append(c.groups, inner.groups...)
	c.fallbacks = append(c.fallbacks, inner.fallbacks...)
	if nested := e.propertyLines(edges, depth+1); len(nested) > 0 {
		c.groups = append(c.groups, Group{Kind: "nested", Lines: nested})
	}
}

// qualified explains sh:qualifiedValueShape with the counts stated next to it:
// "at least one of them: in German, French, Italian or English".
func (e *explainer) qualified(set []bnodetree.Edge, q sparql.Binding, depth int) Part {
	qmin, qmax := -1, -1
	for _, ed := range set {
		switch ed.P.Value {
		case sh + "qualifiedMinCount":
			qmin, _ = strconv.Atoi(ed.O.Value)
		case sh + "qualifiedMaxCount":
			qmax, _ = strconv.Atoi(ed.O.Value)
		}
	}
	p := Part{Kind: "qualified", N: qmin, M: qmax}
	switch {
	case qmin >= 0 && qmax >= 0 && qmin == qmax:
		p.Code = "exactly"
	case qmin >= 0 && qmax >= 0:
		p.Code = "range"
	case qmax >= 0:
		p.Code = "atMost"
	default:
		p.Code = "atLeast"
	}
	if qmin == 1 && p.Code == "atLeast" {
		p.Code = "atLeastOne"
	}
	if !bnodetree.IsNode(q) || len(e.g.Out(q)) == 0 {
		p.Kind = "qualifiedShape"
		p.Items = []Item{e.item(q)}
		return p
	}
	inner := e.constraints([][]bnodetree.Edge{e.g.Out(q)}, depth+1, true)
	p.Inner = inner.parts
	return p
}

// --- cube.link ----------------------------------------------------------------

// dataKind explains meta:dataKind: [ a time:GeneralDateTimeDescription ;
// time:unitType time:unitYear ] → "a point in time (year)"; [ a schema:GeoShape ].
func (e *explainer) dataKind(o sparql.Binding) Part {
	p := Part{Kind: "dataKind"}
	for _, ed := range e.g.Out(o) {
		switch ed.P.Value {
		case rdfType:
			if code, ok := dataKindCodes[ed.O.Value]; ok {
				p.Code = code
			} else if p.Code == "" {
				p.Items = append(p.Items, e.item(ed.O))
			}
		case timeNS + "unitType":
			if code, ok := timeUnitCodes[ed.O.Value]; ok {
				p.Items = append(p.Items, Item{Code: code})
			}
		}
	}
	if p.Code != "" {
		// Keep only the time unit next to a known kind.
		var keep []Item
		for _, it := range p.Items {
			if it.Code != "" {
				keep = append(keep, it)
			}
		}
		p.Items = keep
	}
	return p
}

// relationPart explains meta:dimensionRelation [ a relation:StandardError ;
// meta:relatesTo <dimension> ] → "standard error of {dimension}".
func (e *explainer) relationPart(o sparql.Binding) Part {
	p := Part{Kind: "relation"}
	for _, ed := range e.g.Out(o) {
		switch ed.P.Value {
		case rdfType:
			if code, ok := relationCodes[ed.O.Value]; ok {
				p.Code = code
			}
		case meta + "relatesTo":
			p.Items = append(p.Items, e.item(ed.O))
		}
	}
	return p
}

// hierarchyPart explains meta:inHierarchy → "organised in hierarchy “Cantons”".
func (e *explainer) hierarchyPart(o sparql.Binding) Part {
	p := Part{Kind: "hierarchy"}
	if !bnodetree.IsNode(o) {
		p.Items = []Item{e.item(o)}
		return p
	}
	if name := e.pickText(e.g.Out(o), namePreds); name != "" {
		p.Items = []Item{{Text: name}}
	}
	return p
}

// --- values -------------------------------------------------------------------

// inPart lists the first members of an sh:in list and counts the rest.
func (e *explainer) inPart(list sparql.Binding) Part {
	members, ok := e.g.List(list)
	if !ok {
		return Part{Kind: "in", Items: []Item{{Code: "structure"}}}
	}
	items := make([]Item, 0, len(members))
	for _, m := range members {
		items = append(items, e.item(m))
	}
	p := Part{Kind: "in", N: len(items)}
	if len(items) > inShown {
		p.More = len(items) - inShown
		items = items[:inShown]
		p.Items = join(items, "comma")
		return p
	}
	p.Items = join(items, "or")
	return p
}

// listItems turns an RDF list into items (sh:ignoredProperties).
func (e *explainer) listItems(list sparql.Binding, last string) []Item {
	members, ok := e.g.List(list)
	if !ok {
		return nil
	}
	items := make([]Item, 0, len(members))
	for _, m := range members {
		items = append(items, e.item(m))
	}
	return join(items, last)
}

// languageItems names the language tags of an sh:languageIn list in the page
// language: ("de" "fr") → German, French.
func (e *explainer) languageItems(list sparql.Binding) []Item {
	members, _ := e.g.List(list)
	items := make([]Item, 0, len(members))
	for _, m := range members {
		name := m.Value
		if tag, err := language.Parse(m.Value); err == nil {
			if n := e.langs.Name(tag); n != "" {
				name = n
			}
		}
		items = append(items, Item{Text: name})
	}
	return items
}

// item turns one RDF term into an Item: an IRI as a term, a literal as text, a
// blank node as "a nested structure".
func (e *explainer) item(o sparql.Binding) Item {
	switch {
	case o.Type == "uri" && !bnodetree.IsNode(o):
		t := e.term(o)
		return Item{Term: &t}
	case bnodetree.IsNode(o):
		return Item{Code: "structure"}
	default:
		return Item{Text: o.Value}
	}
}

// datatypeItem names a datatype in plain words when it has a code.
func (e *explainer) datatypeItem(o sparql.Binding) Item {
	if code, ok := datatypeCodes[o.Value]; ok {
		return Item{Code: code}
	}
	return e.item(o)
}

// term prepares an IRI for display. Without a resolved label the local name is
// all there is; it reads better split: "accrualPeriodicity" → "Accrual periodicity".
func (e *explainer) term(b sparql.Binding) bnodetree.Term {
	t := bnodetree.NewTerm(b, e.opts.Prefixes)
	if t.Label == t.Local {
		t.Label = humanize(t.Label)
	}
	return t
}

// path renders an sh:path in CURIE notation and returns the term the line is
// named after (the first predicate) and whether it is a plain predicate.
func (e *explainer) path(b sparql.Binding) (text string, first *bnodetree.Term, simple bool) {
	if !bnodetree.IsNode(b) {
		t := e.term(b)
		return t.CURIE(), &t, true
	}
	if seq, ok := e.g.List(b); ok {
		parts := make([]string, 0, len(seq))
		for _, s := range seq {
			p, f, _ := e.path(s)
			if first == nil {
				first = f
			}
			parts = append(parts, p)
		}
		return strings.Join(parts, "/"), first, false
	}
	for _, ed := range e.g.Out(b) {
		switch ed.P.Value {
		case sh + "inversePath":
			p, f, _ := e.path(ed.O)
			return "^" + p, f, false
		case sh + "zeroOrMorePath", sh + "oneOrMorePath", sh + "zeroOrOnePath":
			p, f, _ := e.path(ed.O)
			mod := map[string]string{sh + "zeroOrMorePath": "*", sh + "oneOrMorePath": "+", sh + "zeroOrOnePath": "?"}[ed.P.Value]
			return "(" + p + ")" + mod, f, false
		case sh + "alternativePath":
			alts, _ := e.g.List(ed.O)
			parts := make([]string, 0, len(alts))
			for _, a := range alts {
				p, f, _ := e.path(a)
				if first == nil {
					first = f
				}
				parts = append(parts, p)
			}
			return "(" + strings.Join(parts, "|") + ")", first, false
		}
	}
	return "?", nil, false
}

// pickText returns the first of preds with a value, in the preferred language,
// else untagged, else English, else any.
func (e *explainer) pickText(edges []bnodetree.Edge, preds []string) string {
	for _, p := range preds {
		var untagged, english, other string
		for _, ed := range edges {
			if ed.P.Value != p || ed.O.Type == "uri" || bnodetree.IsNode(ed.O) || strings.TrimSpace(ed.O.Value) == "" {
				continue
			}
			lang := strings.ToLower(ed.O.Lang)
			switch {
			case e.opts.Lang != "" && (lang == e.opts.Lang || strings.HasPrefix(lang, e.opts.Lang+"-")):
				return ed.O.Value
			case lang == "" && untagged == "":
				untagged = ed.O.Value
			case lang == "en" && english == "":
				english = ed.O.Value
			case other == "":
				other = ed.O.Value
			}
		}
		for _, v := range []string{untagged, english, other} {
			if v != "" {
				return v
			}
		}
	}
	return ""
}

// --- helpers --------------------------------------------------------------------

// join sets the separators of a value list: commas, and last before the final one.
func join(items []Item, last string) []Item {
	for i := range items {
		switch {
		case i == 0:
			items[i].Sep = ""
		case i == len(items)-1:
			items[i].Sep = last
		default:
			items[i].Sep = "comma"
		}
	}
	return items
}

func dedupe(items []Item) []Item {
	seen := map[string]bool{}
	out := items[:0]
	for _, it := range items {
		k := it.Code + "\x00" + it.Text
		if it.Term != nil {
			k = it.Term.IRI
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, it)
	}
	return out
}

func appendFallback(fs []Fallback, pred bnodetree.Term, it Item) []Fallback {
	for i := range fs {
		if fs[i].Pred.IRI == pred.IRI {
			fs[i].Items = join(dedupe(append(fs[i].Items, it)), "comma")
			return fs
		}
	}
	return append(fs, Fallback{Pred: pred, Items: []Item{it}})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func capitalize(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

// humanize splits a camelCase or snake_case local name into words:
// "accrualPeriodicity" → "Accrual periodicity", "ktr_10" → "Ktr 10". Acronyms
// stay as they are ("hasURL" → "Has URL").
func humanize(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if r == '_' || r == '-' {
			b.WriteRune(' ')
			continue
		}
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1])) {
			b.WriteRune(' ')
			if i+1 < len(rs) && unicode.IsLower(rs[i+1]) {
				r = unicode.ToLower(r)
			}
		}
		b.WriteRune(r)
	}
	return capitalize(strings.Join(strings.Fields(b.String()), " "))
}
