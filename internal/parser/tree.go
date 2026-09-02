package parser

// Extraction for the lazy tree's query declarations.
//
//	<sparql-tree-queries for="conceptTree">
//	  <sparql-tree-query role="roots">SELECT …</sparql-tree-query>
//	  <sparql-tree-query role="children">SELECT …</sparql-tree-query>
//	</sparql-tree-queries>
//
// This deliberately does NOT go through extractElements. That function matches a
// fixed tag list and hands every match to parseElement, which flattens the whole
// subtree with extractTextContent — a container would come back as one string
// holding every role's SPARQL concatenated, with no way to tell them apart. A
// container needs its element children read individually, which is the shape
// ExtractColumnContainers already uses.

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// TreeQueries is one <sparql-tree-queries> block: the tree id it configures and
// the query text of each role declared inside it.
type TreeQueries struct {
	ID    string            // the for= value; the id a request names
	Roles map[string]string // role name → query text, verbatim
}

// treeQueriesTag and treeQueryTag are the container and child element names.
const (
	treeQueriesTag = "sparql-tree-queries"
	treeQueryTag   = "sparql-tree-query"
)

// ExtractTreeQueries returns every <sparql-tree-queries> block in a template.
//
// Structural mistakes are errors rather than silent omissions: the symptom of a
// dropped role is a tree that simply never expands, which reads as a data problem
// rather than a markup one. Prose that merely names the elements — documentation,
// or a {{/* … */}} comment, which is not an HTML comment and so is not skipped —
// declares no for= and is left alone; see parseTreeQueriesNode for why that is the
// signal used.
func ExtractTreeQueries(content string) ([]TreeQueries, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var out []TreeQueries
	// Containers already consumed, so the orphan check below does not re-report
	// the role children that legitimately sit inside one.
	claimed := make(map[*html.Node]bool)

	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == treeQueriesTag {
			block, ok, err := parseTreeQueriesNode(n, claimed)
			if err != nil {
				return err
			}
			if ok {
				out = append(out, block)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return nil, err
	}

	// A role query outside any container names no tree, so nothing would ever read
	// it. Report it rather than letting the author wonder why their role is inert.
	var orphan error
	var findOrphans func(*html.Node)
	findOrphans = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == treeQueryTag && !claimed[n] {
			if role := attrValue(n, "role"); role != "" || strings.TrimSpace(extractTextContent(n)) != "" {
				if orphan == nil {
					orphan = fmt.Errorf(`<%s role=%q> is not inside a <%s for="…"> container`,
						treeQueryTag, role, treeQueriesTag)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findOrphans(c)
		}
	}
	findOrphans(doc)
	if orphan != nil {
		return nil, orphan
	}

	return out, nil
}

// parseTreeQueriesNode reads one container. The bool reports whether the node was
// a real declaration; a mention in prose is not, and is skipped without error.
func parseTreeQueriesNode(n *html.Node, claimed map[*html.Node]bool) (TreeQueries, bool, error) {
	id := attrValue(n, "for")

	// No for= is the one unambiguous signal that this is not a declaration.
	//
	// Prose naming the tags parses as a real container holding a real child: the
	// HTML parser treats the unknown, unclosed tags as nested elements, and the
	// trailing words of the sentence become the child's text. So "role-less child
	// with a body" cannot distinguish a mention from a typo — but a real block
	// always carries for=, because that is the tree it configures. Claim the
	// children anyway so the orphan check does not then re-report them.
	if id == "" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == treeQueryTag {
				claimed[c] = true
			}
		}
		return TreeQueries{}, false, nil
	}

	roles := make(map[string]string)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != treeQueryTag {
			continue
		}
		claimed[c] = true

		role := attrValue(c, "role")
		query := strings.TrimSpace(extractTextContent(c))
		if role == "" {
			return TreeQueries{}, false, fmt.Errorf(`<%s> in <%s for=%q> has no role="…"`,
				treeQueryTag, treeQueriesTag, id)
		}
		if query == "" {
			return TreeQueries{}, false, fmt.Errorf(`<%s role=%q> in <%s for=%q> is empty`,
				treeQueryTag, role, treeQueriesTag, id)
		}
		if _, dup := roles[role]; dup {
			return TreeQueries{}, false, fmt.Errorf(`duplicate role %q in <%s for=%q>`,
				role, treeQueriesTag, id)
		}
		roles[role] = query
	}

	if len(roles) == 0 {
		return TreeQueries{}, false, fmt.Errorf(`<%s for=%q> declares no <%s role="…">`,
			treeQueriesTag, id, treeQueryTag)
	}

	return TreeQueries{ID: id, Roles: roles}, true, nil
}
