package bnodetree

import (
	"sort"
	"strconv"
	"strings"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// Well-known IRIs the renderer treats specially. Hard-coded on purpose: the
// tree is generic, and these few vocabulary terms only steer captions, titles
// and ordering — there is nothing to configure per deployment.
const (
	shNS       = "http://www.w3.org/ns/shacl#"
	owlNS      = "http://www.w3.org/2002/07/owl#"
	shOrder    = shNS + "order"
	shPath     = shNS + "path"
	cubeUndef  = "https://cube.link/Undefined"
	xsdString  = "http://www.w3.org/2001/XMLSchema#string"
	rdfLangStr = rdfNS + "langString"
)

// captions names the predicates whose values read better with a caption. The
// key selects the caption text in the template (bnode.caption.<key>).
var captions = map[string]string{
	shNS + "or":              "anyOf",
	shNS + "and":             "allOf",
	shNS + "xone":            "exactlyOneOf",
	shNS + "not":             "not",
	shNS + "in":              "oneOf",
	owlNS + "oneOf":          "oneOf",
	owlNS + "unionOf":        "unionOf",
	owlNS + "intersectionOf": "intersectionOf",
}

// enumPreds hold closed value sets: shown as chips when every member is an IRI
// or a literal, otherwise as a captioned list.
var enumPreds = map[string]bool{
	shNS + "in":              true,
	owlNS + "oneOf":          true,
	owlNS + "unionOf":        true,
	owlNS + "intersectionOf": true,
}

// titlePreds, in order of preference, give a blank node its header label.
var titlePreds = []string{
	"http://schema.org/name",
	"https://schema.org/name",
	"http://www.w3.org/2000/01/rdf-schema#label",
	"http://www.w3.org/2004/02/skos/core#prefLabel",
	shNS + "name",
	"http://purl.org/dc/terms/title",
	"http://www.w3.org/2006/vcard/ns#fn",
	"http://xmlns.com/foaf/0.1/name",
}

// leadPreds are listed before all other predicates of a node, in this order.
var leadPreds = []string{shPath}

// Row is one result row of Request.Query.
type Row struct {
	Depth   int
	S, P, O sparql.Binding
}

// RowsFrom converts a query result into rows, skipping malformed ones.
func RowsFrom(result sparql.QueryResult) []Row {
	rows := make([]Row, 0, len(result.Bindings))
	for _, b := range result.Bindings {
		d, err := strconv.Atoi(b["d"].Value)
		if err != nil || b["s"].Value == "" || b["p"].Value == "" {
			continue
		}
		rows = append(rows, Row{Depth: d, S: b["s"], P: b["p"], O: b["o"]})
	}
	return rows
}

// Graph is the deduplicated triple set behind a tree, shared by the renderer
// and the serializers.
type Graph struct {
	Roots     []sparql.Binding // IRI subjects at depth 0, in first-seen order
	Truncated bool             // the row cap was hit; the structure is incomplete
	out       map[string][]edge
	refs      map[string]int // blank node → number of triples pointing at it
}

type edge struct {
	P, O sparql.Binding
}

// key identifies a term across rows: blank nodes by label, others by value.
func key(b sparql.Binding) string {
	if b.Type == "bnode" {
		return "_:" + b.Value
	}
	return b.Value
}

func isLiteral(b sparql.Binding) bool { return b.Type == "literal" || b.Type == "typed-literal" }

// NewGraph deduplicates rows into a graph. More than RowCap rows means the
// query hit its LIMIT.
func NewGraph(rows []Row) *Graph {
	g := &Graph{out: map[string][]edge{}, refs: map[string]int{}}
	if len(rows) > RowCap {
		g.Truncated = true
		rows = rows[:RowCap]
	}
	seen := map[string]bool{}
	rootSeen := map[string]bool{}
	for _, r := range rows {
		t := key(r.S) + "\x00" + r.P.Value + "\x00" + key(r.O) + "\x00" + r.O.Datatype + "\x00" + r.O.Lang
		if seen[t] {
			continue
		}
		seen[t] = true
		if r.Depth == 0 && r.S.Type == "uri" && !rootSeen[r.S.Value] {
			rootSeen[r.S.Value] = true
			g.Roots = append(g.Roots, r.S)
		}
		g.out[key(r.S)] = append(g.out[key(r.S)], edge{P: r.P, O: r.O})
		if r.O.Type == "bnode" {
			g.refs[key(r.O)]++
		}
	}
	return g
}

// Empty reports whether no root has any blank-node structure to show.
func (g *Graph) Empty() bool { return len(g.Roots) == 0 }

// --- render model ---------------------------------------------------------

// Term is an IRI prepared for display.
type Term struct {
	IRI    string
	Label  string // resolved label, or the last IRI segment
	Prefix string // CURIE prefix, "" when the IRI has no declared namespace
	Local  string // CURIE local part, or the last IRI segment
	Href   string // Visoto resource link
}

// CURIE is the compact form, or the full IRI when no prefix matches.
func (t Term) CURIE() string {
	if t.Prefix == "" {
		return t.IRI
	}
	return t.Prefix + ":" + t.Local
}

// Prop is one predicate of a node with all its values.
type Prop struct {
	Pred    Term
	Caption string // caption key ("anyOf", …), "" for none
	Values  []*Value
}

// Value kinds.
const (
	KindIRI     = "iri"
	KindLiteral = "literal"
	KindNode    = "node"
	KindRef     = "ref"
	KindList    = "list"
	KindEnum    = "enum"
	KindMore    = "more"
	KindNil     = "nil"
)

// Value is one object of a Prop.
type Value struct {
	Kind string

	Term Term // KindIRI

	Text      string // KindLiteral
	Datatype  string // CURIE of a non-default datatype
	Lang      string
	Undefined bool // cube:Undefined
	External  bool // literal holding an http(s) URL

	Node *Node // KindNode

	Ref      int    // KindRef: anchor of the node referenced
	RefTitle string // its title, for the "same node as" line

	Items []*Value // KindList, KindEnum (the shown part)
	Rest  []*Value // KindEnum beyond EnumShown
	Count int      // KindEnum total

	MoreNodes   int // KindMore: blank nodes past MaxDepth
	MoreTriples int // and their triples
}

// Node is a rendered blank node.
type Node struct {
	Anchor    int // >0 when referenced elsewhere; shown as a numbered marker
	Level     int
	Title     string
	Types     []Term
	Order     string // sh:order, shown as a badge
	Props     []*Prop
	Malformed bool  // looked like a list but is not a well-formed one
	Summary   *Prop // the single simple property, shown in the header when closed
	Open      bool  // rendered expanded
}

// Flat reports a node with nothing to put in a header — no title, type, order
// or marker — and a single simple property, like [ sh:datatype xsd:integer ].
// It is rendered as that one row, without a foldable header.
func (n *Node) Flat() bool {
	return n.Summary != nil && n.Title == "" && len(n.Types) == 0 && n.Order == "" && n.Anchor == 0 && !n.Malformed
}

// Root is one IRI root with its blank-node-valued predicates.
type Root struct {
	Term  Term
	Props []*Prop
}

// Tree is what the template renders.
type Tree struct {
	Roots      []*Root
	BlankNodes int  // distinct blank nodes (list cells excluded), hidden ones included
	Truncated  bool // row cap hit
	Collapsed  bool // nodes start closed
}

// Options steer the rendering.
type Options struct {
	Lang      string          // preferred language for blank-node titles
	Prefixes  []config.Prefix // for CURIEs
	Collapsed bool
}

// Build renders g into a tree.
func Build(g *Graph, opts Options) *Tree {
	b := &builder{g: g, opts: opts, anchors: map[string]*Node{}, onStack: map[string]*Node{}, counted: map[string]bool{}}
	t := &Tree{Truncated: g.Truncated, Collapsed: opts.Collapsed}
	for _, r := range g.Roots {
		root := &Root{Term: b.term(r)}
		root.Props = b.props(g.out[key(r)], 0, true)
		if len(root.Props) > 0 {
			t.Roots = append(t.Roots, root)
		}
	}
	t.BlankNodes = len(b.counted)
	return t
}

type builder struct {
	g       *Graph
	opts    Options
	nextID  int
	anchors map[string]*Node // blank node → its first rendering
	onStack map[string]*Node // cycle guard
	counted map[string]bool
}

// props groups edges by predicate. level is the level of the subject; at the
// root (rootOnly) only blank-node-valued predicates are kept.
func (b *builder) props(edges []edge, level int, rootOnly bool) []*Prop {
	byPred := map[string]*Prop{}
	var order []string
	for _, e := range edges {
		if rootOnly && e.O.Type != "bnode" {
			continue
		}
		p, ok := byPred[e.P.Value]
		if !ok {
			p = &Prop{Pred: b.term(e.P), Caption: captions[e.P.Value]}
			byPred[e.P.Value] = p
			order = append(order, e.P.Value)
		}
		p.Values = append(p.Values, b.value(e.P.Value, e.O, level+1))
	}
	props := make([]*Prop, 0, len(order))
	for _, k := range order {
		p := byPred[k]
		p.Values = mergeMore(p.Values)
		sortValues(p.Values)
		props = append(props, p)
	}
	sort.SliceStable(props, func(i, j int) bool {
		ri, rj := leadRank(props[i].Pred.IRI), leadRank(props[j].Pred.IRI)
		if ri != rj {
			return ri < rj
		}
		// By the name shown in the key column, so predicates from different
		// namespaces interleave alphabetically rather than by namespace IRI.
		if props[i].Pred.Local != props[j].Pred.Local {
			return props[i].Pred.Local < props[j].Pred.Local
		}
		return props[i].Pred.IRI < props[j].Pred.IRI
	})
	return props
}

func leadRank(iri string) int {
	for i, p := range leadPreds {
		if p == iri {
			return i
		}
	}
	return len(leadPreds)
}

// value renders one object; level is the level a blank node there would have.
func (b *builder) value(pred string, o sparql.Binding, level int) *Value {
	switch {
	case o.Type == "uri" && o.Value == rdfNil:
		return &Value{Kind: KindNil}
	case o.Type == "uri":
		return &Value{Kind: KindIRI, Term: b.term(o)}
	case isLiteral(o):
		return b.literal(o)
	case o.Type != "bnode":
		return &Value{Kind: KindLiteral, Text: o.Value}
	}
	if items, ok := b.list(o); ok {
		vals := make([]*Value, len(items))
		allPlain := true
		for i, it := range items {
			vals[i] = b.value("", it, level)
			if vals[i].Kind != KindIRI && vals[i].Kind != KindLiteral {
				allPlain = false
			}
		}
		if enumPreds[pred] && allPlain {
			v := &Value{Kind: KindEnum, Count: len(vals), Items: vals}
			if len(vals) > EnumShown {
				v.Items, v.Rest = vals[:EnumShown], vals[EnumShown:]
			}
			return v
		}
		return &Value{Kind: KindList, Items: vals}
	}
	return b.node(o, level)
}

// node renders a blank node, or a reference to it when it was rendered before
// or is an ancestor, or a depth-limit marker past MaxDepth.
func (b *builder) node(o sparql.Binding, level int) *Value {
	k := key(o)
	if !b.isListCell(k) {
		b.counted[k] = true
	}
	if n, ok := b.onStack[k]; ok {
		return b.ref(n)
	}
	if n, ok := b.anchors[k]; ok {
		return b.ref(n)
	}
	if level > MaxDepth {
		return &Value{Kind: KindMore, MoreNodes: 1, MoreTriples: len(b.g.out[k])}
	}
	n := &Node{Level: level, Open: !b.opts.Collapsed}
	if b.g.refs[k] > 1 {
		b.anchors[k] = n
	}
	b.onStack[k] = n
	edges := b.g.out[k]
	var rest []edge
	for _, e := range edges {
		switch {
		case e.P.Value == rdfType && e.O.Type == "uri":
			n.Types = append(n.Types, b.term(e.O))
		case e.P.Value == shOrder && isLiteral(e.O) && n.Order == "":
			n.Order = e.O.Value
		default:
			rest = append(rest, e)
		}
	}
	n.Title = b.title(edges)
	if n.Title == "" {
		// An unnamed property shape is best known by its path.
		for _, e := range edges {
			if e.P.Value == shPath && e.O.Type == "uri" {
				n.Title = b.term(e.O).Label
				break
			}
		}
	}
	n.Malformed = b.looksLikeList(k)
	n.Props = b.props(rest, level, false)
	if len(n.Props) == 1 && len(n.Props[0].Values) == 1 {
		switch n.Props[0].Values[0].Kind {
		case KindLiteral, KindIRI:
			n.Summary = n.Props[0]
		}
	}
	delete(b.onStack, k)
	return &Value{Kind: KindNode, Node: n}
}

func (b *builder) ref(n *Node) *Value {
	if n.Anchor == 0 {
		b.nextID++
		n.Anchor = b.nextID
	}
	return &Value{Kind: KindRef, Ref: n.Anchor, RefTitle: n.Title}
}

// list returns the members of a well-formed RDF list starting at o.
func (b *builder) list(o sparql.Binding) ([]sparql.Binding, bool) {
	var items []sparql.Binding
	seen := map[string]bool{}
	cur := o
	for {
		if cur.Type == "uri" && cur.Value == rdfNil {
			return items, len(items) > 0
		}
		k := key(cur)
		if cur.Type != "bnode" || seen[k] {
			return nil, false
		}
		seen[k] = true
		var first, rest []sparql.Binding
		for _, e := range b.g.out[k] {
			switch e.P.Value {
			case rdfFirst:
				first = append(first, e.O)
			case rdfRest:
				rest = append(rest, e.O)
			case rdfType:
			default:
				return nil, false
			}
		}
		if len(first) != 1 || len(rest) != 1 {
			return nil, false
		}
		items = append(items, first[0])
		cur = rest[0]
	}
}

func (b *builder) isListCell(k string) bool {
	for _, e := range b.g.out[k] {
		if e.P.Value == rdfFirst || e.P.Value == rdfRest {
			return true
		}
	}
	return false
}

// looksLikeList: a node carrying rdf:first/rdf:rest that list() rejected.
func (b *builder) looksLikeList(k string) bool { return b.isListCell(k) }

// title picks a blank node's header label: the first title predicate with a
// value, in the preferred language, else untagged, else any.
func (b *builder) title(edges []edge) string {
	for _, tp := range titlePreds {
		var untagged, other string
		for _, e := range edges {
			if e.P.Value != tp || !isLiteral(e.O) || strings.TrimSpace(e.O.Value) == "" {
				continue
			}
			switch {
			case b.opts.Lang != "" && langMatches(e.O.Lang, b.opts.Lang):
				return e.O.Value
			case e.O.Lang == "" && untagged == "":
				untagged = e.O.Value
			case other == "" || e.O.Lang == "en":
				other = e.O.Value
			}
		}
		if untagged != "" {
			return untagged
		}
		if other != "" {
			return other
		}
	}
	return ""
}

func langMatches(tag, want string) bool {
	tag, want = strings.ToLower(tag), strings.ToLower(want)
	return tag == want || strings.HasPrefix(tag, want+"-")
}

func (b *builder) literal(o sparql.Binding) *Value {
	v := &Value{Kind: KindLiteral, Text: o.Value, Lang: o.Lang}
	switch o.Datatype {
	case "", xsdString, rdfLangStr:
	case cubeUndef:
		v.Undefined = true
	default:
		v.Datatype = b.term(sparql.Binding{Type: "uri", Value: o.Datatype}).CURIE()
	}
	if strings.HasPrefix(o.Value, "http://") || strings.HasPrefix(o.Value, "https://") {
		v.External = !strings.ContainsAny(o.Value, " \t\n")
	}
	return v
}

func (b *builder) term(t sparql.Binding) Term {
	out := Term{IRI: t.Value, Href: sparql.ResourceHref(t.Value)}
	out.Prefix, out.Local = Compact(t.Value, b.opts.Prefixes)
	out.Label = t.DisplayText
	if out.Label == "" || out.Label == t.Value || strings.Contains(out.Label, "?") {
		out.Label = lastSegment(t.Value)
		// A URL with a query string is only recognisable whole: the segment
		// after the last slash would be a meaningless "browse?dataset=…".
		if strings.Contains(out.Label, "?") {
			out.Label = t.Value
		}
	}
	return out
}

// Compact splits iri into a declared prefix and local part, using the longest
// matching namespace. Without a match it returns "" and the last segment.
func Compact(iri string, prefixes []config.Prefix) (prefix, local string) {
	best := -1
	for _, p := range prefixes {
		ns := strings.Trim(p.URI, "<>")
		if ns == "" || !strings.HasPrefix(iri, ns) || len(ns) <= best {
			continue
		}
		rest := iri[len(ns):]
		if strings.ContainsAny(rest, "/#?") {
			continue
		}
		best = len(ns)
		prefix, local = p.Name, rest
	}
	if best < 0 {
		return "", lastSegment(iri)
	}
	return prefix, local
}

func lastSegment(iri string) string {
	s := strings.TrimRight(iri, "/#")
	if i := strings.LastIndexAny(s, "/#"); i >= 0 && i < len(s)-1 {
		return s[i+1:]
	}
	return iri
}

// mergeMore folds all depth-limit markers of one predicate into a single one.
func mergeMore(vals []*Value) []*Value {
	var more *Value
	out := vals[:0]
	for _, v := range vals {
		if v.Kind != KindMore {
			out = append(out, v)
			continue
		}
		if more == nil {
			more = v
			continue
		}
		more.MoreNodes += v.MoreNodes
		more.MoreTriples += v.MoreTriples
	}
	if more != nil {
		out = append(out, more)
	}
	return out
}

// sortValues orders sibling values: nodes by sh:order then title, literals by
// language then text; markers stay last. The sort is stable, so equal values
// keep query order.
func sortValues(vals []*Value) {
	sort.SliceStable(vals, func(i, j int) bool {
		a, b := vals[i], vals[j]
		if (a.Kind == KindMore) != (b.Kind == KindMore) {
			return b.Kind == KindMore
		}
		if a.Kind == KindNode && b.Kind == KindNode {
			oa, ea := strconv.ParseFloat(a.Node.Order, 64)
			ob, eb := strconv.ParseFloat(b.Node.Order, 64)
			switch {
			case ea == nil && eb == nil && oa != ob:
				return oa < ob
			case (ea == nil) != (eb == nil):
				return ea == nil
			}
			return a.Node.Title < b.Node.Title
		}
		if a.Kind == KindLiteral && b.Kind == KindLiteral {
			if a.Lang != b.Lang {
				return a.Lang < b.Lang
			}
			return a.Text < b.Text
		}
		if a.Kind == KindIRI && b.Kind == KindIRI {
			return a.Term.Label < b.Term.Label
		}
		return false
	})
}
