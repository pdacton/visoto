// Package bnodetree turns the outgoing blank nodes of a resource into a tree for
// display: nested key/value blocks, RDF lists, enumerations and shared nodes.
//
// Everything shown comes from ONE result set. Blank-node labels are only
// stable within a single result set, so the structure under a root cannot be
// fetched piecemeal — a label returned by one query means nothing to the next.
// A deeper query may replace a shallower one (ProbeDepth), never extend it.
// A SELECT over SPARQL JSON results also spares the project an RDF parser.
package bnodetree

import (
	"fmt"
	"strings"

	"hutzli.org/visoto/internal/sparql"
)

const (
	// MaxDepth is the number of blank-node levels rendered below a root.
	MaxDepth = 5
	// queryDepth fetches one level more than is rendered: those rows are only
	// counted, which is what the "beyond depth 5" indicator reports.
	queryDepth = MaxDepth + 1
	// ProbeDepth is the level the first, cheap query stops at. Most structures
	// (a contact point, a property shape with an sh:in) end above it, and only
	// those that do not pay for the full query — see NeedsDeeper.
	ProbeDepth = 2
	// RowCap bounds the result. One more row is requested so a hit is detectable.
	RowCap = 5000
	// EnumShown is how many enumeration members are shown before "+ n more".
	EnumShown = 8
)

const (
	rdfNS    = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	rdfType  = rdfNS + "type"
	rdfFirst = rdfNS + "first"
	rdfRest  = rdfNS + "rest"
	rdfNil   = rdfNS + "nil"
)

// Request selects the roots and the first hop of a tree. All IRIs must already
// be validated full IRIs (no angle brackets) — they are spliced into the query.
type Request struct {
	IRI      string // the resource the tree hangs off
	Property string // optional: only expand blank nodes under this predicate
	Via      string // optional: follow this predicate to IRI roots first
}

// Validate checks every IRI of the request before it is spliced into a query.
func (r Request) Validate() error {
	if err := sparql.ValidateIRI(r.IRI); err != nil {
		return err
	}
	for _, opt := range []string{r.Property, r.Via} {
		if opt == "" {
			continue
		}
		if err := sparql.ValidateIRI(opt); err != nil {
			return err
		}
	}
	return nil
}

// Query returns the SELECT that fetches every triple needed for the tree.
func (r Request) Query() string { return r.QueryTo(queryDepth) }

// QueryTo returns the SELECT for levels 0 to depth.
//
// Rows are (?d ?s ?p ?o): ?d is the level of ?s, 0 for a root. Level k holds the
// nodes k blank-node hops below a root. A hop is either a plain triple to a
// blank node, or a jump through an RDF list straight to a blank-node member, so
// list cells never use up a level — an sh:in with 23 members is one level, not
// 23. For each level two kinds of rows are emitted: the node's own triples, and
// the rdf:first/rdf:rest spine of the lists hanging off it (which the hop
// pattern skips over).
//
// Each level is its own UNION branch repeating every hop above it. GraphDB
// evaluates that top-down and does not mind; QLever evaluates bottom-up, so
// each hop with an unbound subject scans every blank-object triple of the
// index, and all branches run at once. A depth-6 query against a shape that is
// two levels deep exhausted 1.5 GB of QLever query memory — hence ProbeDepth.
func (r Request) QueryTo(depth int) string {
	var b strings.Builder
	b.WriteString("SELECT DISTINCT ?d ?s ?p ?o WHERE {\n")
	first := true
	union := func(branch string) {
		if !first {
			b.WriteString("  UNION\n")
		}
		first = false
		b.WriteString("  {\n")
		b.WriteString(branch)
		b.WriteString("  }\n")
	}
	for level := 0; level <= depth; level++ {
		union(r.branch(level, false))
		if level < depth {
			union(r.branch(level, true))
		}
	}
	fmt.Fprintf(&b, "}\nLIMIT %d\n", RowCap+1)
	// Written with the rdf: CURIEs above for readability, sent as full IRIs so
	// the query does not depend on an endpoint declaring the rdf: prefix.
	return strings.NewReplacer("rdf:first", "<"+rdfFirst+">", "rdf:rest", "<"+rdfRest+">").Replace(b.String())
}

// branch walks `level` hops from the root and emits the triples of the node
// reached there (spine=false) or the list spines hanging off it (spine=true).
func (r Request) branch(level int, spine bool) string {
	var b strings.Builder
	b.WriteString("    " + r.rootPattern() + "\n")
	for k := 1; k <= level; k++ {
		b.WriteString(r.hop(k))
	}
	node := fmt.Sprintf("?n%d", level)
	if !spine {
		pred := "?p"
		if level == 0 && r.Property != "" {
			pred = "<" + r.Property + ">"
			fmt.Fprintf(&b, "    %s %s ?o . FILTER(isBlank(?o)) BIND(%s AS ?p)\n", node, pred, pred)
		} else if level == 0 {
			fmt.Fprintf(&b, "    %s ?p ?o . FILTER(isBlank(?o))\n", node)
		} else {
			fmt.Fprintf(&b, "    %s ?p ?o .\n", node)
		}
		fmt.Fprintf(&b, "    BIND(%s AS ?s) BIND(%d AS ?d)\n", node, level)
		return b.String()
	}
	lp := "?lp"
	if level == 0 && r.Property != "" {
		lp = "<" + r.Property + ">"
	}
	fmt.Fprintf(&b, "    %s %s ?lh . FILTER(isBlank(?lh))\n", node, lp)
	b.WriteString("    ?lh rdf:rest* ?s . ?s ?p ?o . FILTER(?p = rdf:first || ?p = rdf:rest)\n")
	fmt.Fprintf(&b, "    BIND(%d AS ?d)\n", level)
	return b.String()
}

// rootPattern binds ?n0 to the root(s).
func (r Request) rootPattern() string {
	if r.Via != "" {
		return fmt.Sprintf("<%s> <%s> ?n0 . FILTER(isIRI(?n0))", r.IRI, r.Via)
	}
	return fmt.Sprintf("VALUES ?n0 { <%s> }", r.IRI)
}

// hop moves from ?n(k-1) to the blank node ?nk: directly, or through a list to
// one of its blank-node members. The first hop honours Property.
func (r Request) hop(k int) string {
	from, to := fmt.Sprintf("?n%d", k-1), fmt.Sprintf("?n%d", k)
	pred := fmt.Sprintf("?h%d", k)
	if k == 1 && r.Property != "" {
		pred = "<" + r.Property + ">"
	}
	// Neither form may step along a list's own rdf:first/rdf:rest: a list head
	// reached as a node would otherwise lead to its members a second time, one
	// level deeper than the through-the-list form already put them.
	notSpine := ""
	if strings.HasPrefix(pred, "?") {
		notSpine = fmt.Sprintf(" FILTER(%s != rdf:first && %s != rdf:rest)", pred, pred)
	}
	direct := fmt.Sprintf("%s %s %s . FILTER(isBlank(%s))%s", from, pred, to, to, notSpine)
	viaList := fmt.Sprintf("%s %s ?l%d . FILTER(isBlank(?l%d))%s ?l%d rdf:rest*/rdf:first %s . FILTER(isBlank(%s))",
		from, pred, k, k, notSpine, k, to, to)
	return fmt.Sprintf("    { %s }\n    UNION\n    { %s }\n", direct, viaList)
}

// NeedsDeeper reports whether rows fetched with QueryTo(depth) stop short of the
// structure: a node at the last level points at a blank node (a child or a
// list), whose triples that query did not fetch. Spine rows never sit at the
// last level, so every row there is a node's own triple.
func NeedsDeeper(rows []Row, depth int) bool {
	for _, r := range rows {
		if r.Depth == depth && r.O.Type == "bnode" {
			return true
		}
	}
	return false
}
