package bnodetree

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// The downloads serialize the same deduplicated triples the card shows, so a
// download always matches the tree. Blank nodes are relabelled _:b0, _:b1, … in
// first-seen order: the store's own labels are meaningless outside the result.

// labeler hands out stable short blank-node labels.
type labeler struct {
	ids map[string]string
}

func (l *labeler) id(b sparql.Binding) string {
	k := key(b)
	if id, ok := l.ids[k]; ok {
		return id
	}
	id := fmt.Sprintf("b%d", len(l.ids))
	l.ids[k] = id
	return id
}

// subjects lists every subject in a deterministic order: roots first, then
// blank nodes in the order they are reached from the roots.
func (g *Graph) subjects() []sparql.Binding {
	var out []sparql.Binding
	seen := map[string]bool{}
	var walk func(b sparql.Binding)
	walk = func(b sparql.Binding) {
		k := key(b)
		if seen[k] {
			return
		}
		if _, has := g.out[k]; !has && b.Type == "bnode" {
			return
		}
		seen[k] = true
		out = append(out, b)
		for _, e := range g.out[k] {
			if e.O.Type == "bnode" {
				walk(e.O)
			}
		}
	}
	for _, r := range g.Roots {
		walk(r)
	}
	return out
}

func (g *Graph) edges(s sparql.Binding) []edge { return g.out[key(s)] }

// --- N-Triples --------------------------------------------------------------

// NTriples returns the graph as N-Triples.
func (g *Graph) NTriples() string {
	l := &labeler{ids: map[string]string{}}
	var b strings.Builder
	for _, s := range g.subjects() {
		for _, e := range g.edges(s) {
			fmt.Fprintf(&b, "%s <%s> %s .\n", ntTerm(s, l), e.P.Value, ntTerm(e.O, l))
		}
	}
	return b.String()
}

func ntTerm(t sparql.Binding, l *labeler) string {
	switch {
	case t.Type == "bnode":
		return "_:" + l.id(t)
	case isLiteral(t):
		return literalString(t, func(dt string) string { return "<" + dt + ">" })
	default:
		return "<" + t.Value + ">"
	}
}

// literalString writes a literal with its language tag or datatype; dt formats
// the datatype IRI (full in N-Triples, possibly a CURIE in Turtle).
func literalString(t sparql.Binding, dt func(string) string) string {
	s := `"` + escapeLiteral(t.Value) + `"`
	switch {
	case t.Lang != "":
		return s + "@" + t.Lang
	case t.Datatype != "" && t.Datatype != xsdString:
		return s + "^^" + dt(t.Datatype)
	}
	return s
}

var literalEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)

func escapeLiteral(s string) string { return literalEscaper.Replace(s) }

// --- Turtle -----------------------------------------------------------------

// Turtle returns the graph as Turtle: blank nodes referenced exactly once are
// nested inline as [ … ], well-formed lists as ( … ); shared and cyclic blank
// nodes get a _:label and a block of their own.
func (g *Graph) Turtle(prefixes []config.Prefix) string {
	w := &turtleWriter{g: g, prefixes: prefixes, used: map[string]bool{}, l: &labeler{ids: map[string]string{}}}
	var body strings.Builder
	for _, s := range g.subjects() {
		if s.Type == "bnode" && w.inline(s) {
			continue
		}
		w.block(&body, s)
	}
	var head strings.Builder
	names := make([]string, 0, len(w.used))
	for n := range w.used {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, p := range prefixes {
			if p.Name == n {
				fmt.Fprintf(&head, "@prefix %s: <%s> .\n", n, strings.Trim(p.URI, "<>"))
				break
			}
		}
	}
	if head.Len() > 0 {
		head.WriteString("\n")
	}
	return head.String() + body.String()
}

type turtleWriter struct {
	g        *Graph
	prefixes []config.Prefix
	used     map[string]bool
	l        *labeler
}

// inline: referenced exactly once, so it can be written where it is used.
func (w *turtleWriter) inline(b sparql.Binding) bool {
	return b.Type == "bnode" && w.g.refs[key(b)] == 1
}

func (w *turtleWriter) block(out *strings.Builder, s sparql.Binding) {
	out.WriteString(w.term(s, 0))
	w.predicates(out, w.g.edges(s), 1)
	out.WriteString(" .\n\n")
}

func (w *turtleWriter) predicates(out *strings.Builder, edges []edge, indent int) {
	pad := strings.Repeat("    ", indent)
	byPred := map[string][]sparql.Binding{}
	var order []string
	for _, e := range edges {
		if _, ok := byPred[e.P.Value]; !ok {
			order = append(order, e.P.Value)
		}
		byPred[e.P.Value] = append(byPred[e.P.Value], e.O)
	}
	// "a" first, as Turtle is usually written.
	sort.SliceStable(order, func(i, j int) bool { return order[i] == rdfType && order[j] != rdfType })
	for i, p := range order {
		if i > 0 {
			out.WriteString(" ;")
		}
		out.WriteString("\n" + pad)
		if p == rdfType {
			out.WriteString("a")
		} else {
			out.WriteString(w.iri(p))
		}
		for j, o := range byPred[p] {
			if j > 0 {
				out.WriteString(",")
			}
			out.WriteString(" " + w.term(o, indent))
		}
	}
}

func (w *turtleWriter) term(t sparql.Binding, indent int) string {
	switch {
	case t.Type == "bnode" && w.inline(t):
		if items, ok := w.list(t); ok {
			parts := make([]string, len(items))
			for i, it := range items {
				parts[i] = w.term(it, indent)
			}
			return "( " + strings.Join(parts, " ") + " )"
		}
		edges := w.g.edges(t)
		if len(edges) == 0 {
			return "[]"
		}
		var b strings.Builder
		b.WriteString("[")
		w.predicates(&b, edges, indent+1)
		b.WriteString("\n" + strings.Repeat("    ", indent) + "]")
		return b.String()
	case t.Type == "bnode":
		return "_:" + w.l.id(t)
	case isLiteral(t):
		return literalString(t, w.iri)
	default:
		return w.iri(t.Value)
	}
}

// list returns the members of a well-formed list whose cells are all
// referenced once (so nothing else points into the middle of it).
func (w *turtleWriter) list(t sparql.Binding) ([]sparql.Binding, bool) {
	var items []sparql.Binding
	seen := map[string]bool{}
	cur := t
	for {
		if cur.Type == "uri" && cur.Value == rdfNil {
			return items, len(items) > 0
		}
		k := key(cur)
		if cur.Type != "bnode" || seen[k] || w.g.refs[k] != 1 {
			return nil, false
		}
		seen[k] = true
		edges := w.g.out[k]
		if len(edges) != 2 {
			return nil, false
		}
		var first, rest *sparql.Binding
		for i := range edges {
			switch edges[i].P.Value {
			case rdfFirst:
				first = &edges[i].O
			case rdfRest:
				rest = &edges[i].O
			}
		}
		if first == nil || rest == nil {
			return nil, false
		}
		items = append(items, *first)
		cur = *rest
	}
}

// pnLocal is a conservative PN_LOCAL: anything else is written as <IRI>.
var pnLocal = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*(\.[A-Za-z0-9_-]+)*$|^$`)

func (w *turtleWriter) iri(iri string) string {
	if prefix, local := Compact(iri, w.prefixes); prefix != "" && pnLocal.MatchString(local) {
		w.used[prefix] = true
		return prefix + ":" + local
	}
	return "<" + iri + ">"
}

// --- JSON-LD ----------------------------------------------------------------

// JSONLD returns the graph as flattened, expanded JSON-LD: one node object per
// subject, lists left as their rdf:first/rdf:rest cells.
func (g *Graph) JSONLD() ([]byte, error) {
	l := &labeler{ids: map[string]string{}}
	id := func(t sparql.Binding) string {
		if t.Type == "bnode" {
			return "_:" + l.id(t)
		}
		return t.Value
	}
	var nodes []map[string]any
	for _, s := range g.subjects() {
		n := map[string]any{"@id": id(s)}
		for _, e := range g.edges(s) {
			var obj map[string]any
			switch {
			case isLiteral(e.O):
				obj = map[string]any{"@value": e.O.Value}
				if e.O.Lang != "" {
					obj["@language"] = e.O.Lang
				} else if e.O.Datatype != "" && e.O.Datatype != xsdString {
					obj["@type"] = e.O.Datatype
				}
			default:
				obj = map[string]any{"@id": id(e.O)}
			}
			pred := e.P.Value
			if pred == rdfType && e.O.Type == "uri" {
				pred = "@type"
				types, _ := n[pred].([]any)
				n[pred] = append(types, e.O.Value)
				continue
			}
			vals, _ := n[pred].([]any)
			n[pred] = append(vals, obj)
		}
		nodes = append(nodes, n)
	}
	if nodes == nil {
		nodes = []map[string]any{}
	}
	return json.MarshalIndent(nodes, "", "  ")
}
