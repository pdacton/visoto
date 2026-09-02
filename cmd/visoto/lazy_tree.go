package main

// The /api/lazy-tree/:id/:role tier — the JSON backend for the sparqlLazyTree
// partial.
//
// Where sparqlTree fetches a whole hierarchy in one synchronous query and embeds
// every row in the page, this loads one LEVEL per request: the page ships no node
// payload at all, and cost scales with what the user actually expands rather than
// with the size of the hierarchy. The route table is the level structure —
// roots, children of a node, parents of a batch, a search, and focus, which
// resolves an ancestor chain and every level along it in one round trip.
//
// Two invariants the whole surface rests on:
//
//   - The client sends an ID, never SPARQL. Query text is resolved server-side
//     from (template set, tree id, role), so a request can only ever ask for a
//     query some template already declared.
//   - Every IRI from the request is validated before it reaches a query, and the
//     search term is bound as a literal. internal/tree does both; this file is the
//     HTTP glue that resolves the declaration, runs the query, and shapes JSON.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/sparql"
	"hutzli.org/visoto/internal/tree"
)

// maxFocusDepth bounds the ancestor walk. A hierarchy with a cycle (A broader B
// broader A — malformed, but real data is malformed) would otherwise walk until
// the request times out. Deeper than any real thesaurus.
const maxFocusDepth = 64

// maxBatchNodes caps how many IRIs one parents/node-data request may name. The
// batch goes into a VALUES clause, so an unbounded list is an unbounded query.
const maxBatchNodes = 500

// treeNode is one node as the client consumes it — Wunderbaum's source shape, so
// the render and column logic lifted from sparql-tree.js works unchanged.
type treeNode struct {
	Key   string              `json:"key"`            // the node IRI
	Title string              `json:"title"`          // display label
	Lazy  bool                `json:"lazy"`           // has (or may have) children
	Icon  string              `json:"icon,omitempty"` // resource icon path, when resolved
	Extra map[string]treeCell `json:"-"`              // extra projected vars, flattened on marshal
}

// treeCell mirrors the {value,label,type} shape sparql-tree.js puts on a node for
// each extra variable, so treegrid columns read the same way in both trees.
type treeCell struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// MarshalJSON flattens Extra to top-level keys: Wunderbaum exposes non-reserved
// source properties as node.data[name], which is what the render callback reads.
func (n treeNode) MarshalJSON() ([]byte, error) {
	out := map[string]any{"key": n.Key, "title": n.Title, "lazy": n.Lazy}
	if n.Icon != "" {
		out["icon"] = n.Icon
	} else {
		// Suppress Wunderbaum's per-node folder/document glyph. The tree already
		// shows a label, so the icon is noise — and since this project loads Lucide
		// rather than the Bootstrap Icons the default iconMap names, it renders as
		// nothing while still holding a 20px box that pushes the title away from
		// its chevron.
		//
		// It must be the JSON literal false: the renderer skips the element only for
		// `icon === false` on the NODE ("" still yields the empty box, and a false
		// entry in the iconMap is not consulted for this).
		out["icon"] = false
	}
	for k, v := range n.Extra {
		out[k] = v
	}
	return json.Marshal(out)
}

// levelEnvelope is the response shape for any single level (LT-12).
type levelEnvelope struct {
	Nodes     []treeNode `json:"nodes"`
	Total     int        `json:"total"`
	Complete  bool       `json:"complete"`
	LimitMode string     `json:"limitMode"` // "page" (load more is meaningful) or "cap"
	Error     string     `json:"error,omitempty"`
}

// lazyTreeHandler serves GET /api/lazy-tree/:id/:role.
//
// One handler for every role rather than five near-identical ones: they differ
// only in which declaration they read and which input they bind, and sharing the
// resolve/validate/execute path is what keeps the security posture uniform.
func lazyTreeHandler(c *gin.Context) {
	id := c.Param("id")
	role := strings.TrimPrefix(c.Param("role"), "/")

	block, found := findTreeQueries(c.Query("src"), id)
	if !found {
		badTreeRequest(c, http.StatusNotFound, fmt.Sprintf("no <sparql-tree-queries for=%q> in this template set", id))
		return
	}

	switch role {
	case tree.RoleRoots:
		serveLevel(c, block, tree.RoleRoots, "")
	case tree.RoleChildren:
		parent := c.Query("parent")
		if parent == "" {
			badTreeRequest(c, http.StatusBadRequest, "children requires ?parent=<iri>")
			return
		}
		// Validated here rather than inside the query builder so a malformed IRI is
		// refused before any work is done for it.
		if err := sparql.ValidateIRI(parent); err != nil {
			badTreeRequest(c, http.StatusBadRequest, err.Error())
			return
		}
		serveLevel(c, block, tree.RoleChildren, parent)
	case tree.RoleParents:
		serveParents(c, block)
	case tree.RoleSearch:
		serveSearch(c, block)
	case "focus":
		serveFocus(c, block)
	case tree.RoleNodeData:
		serveNodeData(c, block)
	default:
		badTreeRequest(c, http.StatusNotFound, fmt.Sprintf("unknown role %q", role))
	}
}

// badTreeRequest reports a malformed or unresolvable request. Never cached: a 400
// held by the shared cache would outlive the mistake that caused it.
func badTreeRequest(c *gin.Context, status int, msg string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, levelEnvelope{Nodes: []treeNode{}, Complete: true, Error: msg})
}

// treeQueryContext bundles what every role needs to run a query.
type treeQueryContext struct {
	preprocessor *parser.Preprocessor
	ctx          context.Context
	cancel       context.CancelFunc
	pageIRI      string
	lang         string
}

// validateIRIs rejects a batch containing any IRI that could break out of a
// <...> term. Checked before the query is built so the whole request fails rather
// than one silently-dropped node.
func validateIRIs(iris []string) error {
	for _, iri := range iris {
		if err := sparql.ValidateIRI(iri); err != nil {
			return err
		}
	}
	return nil
}

func newTreeQueryContext(c *gin.Context) treeQueryContext {
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
	return treeQueryContext{
		preprocessor: prepareQueryInputs(c),
		ctx:          ctx,
		cancel:       cancel,
		pageIRI:      c.Query("iri"),
		lang:         queryLang(c),
	}
}

// serveLevel answers roots and children — the two required roles, and the only
// ones on the hot path of ordinary browsing.
func serveLevel(c *gin.Context, block parser.TreeQueries, role, parent string) {
	declared := block.Roles[role]
	mode, limit := tree.ModeFor(declared, requestedLimit(c))
	offset := requestedOffset(c)

	q := newTreeQueryContext(c)
	defer q.cancel()

	var query string
	var err error
	// One extra row is requested so completeness is known without a COUNT: if the
	// endpoint returns limit+1, there is at least one more level entry to load.
	fetch := limit + 1
	if role == tree.RoleRoots {
		query, err = tree.RootsQuery(declared, q.pageIRI, fetch, offset)
	} else {
		query, err = tree.ChildrenQuery(declared, q.pageIRI, parent, fetch, offset)
	}
	if err != nil {
		badTreeRequest(c, http.StatusBadRequest, err.Error())
		return
	}

	nodes, execErr := runLevel(c, q, block, query)
	if execErr != nil {
		// A transient endpoint failure must never be cached as if it were data.
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, levelEnvelope{Nodes: []treeNode{}, Complete: true,
			LimitMode: string(mode), Error: execErr.Error()})
		return
	}

	complete := len(nodes) <= limit
	if !complete {
		nodes = nodes[:limit]
	}
	markCacheable(c)
	c.JSON(http.StatusOK, levelEnvelope{
		Nodes: nodes, Total: offset + len(nodes), Complete: complete, LimitMode: string(mode),
	})
}

// runLevel executes one level query and shapes its rows into nodes.
func runLevel(c *gin.Context, q treeQueryContext, block parser.TreeQueries, query string) ([]treeNode, error) {
	result, err := q.preprocessor.ExecuteQueryWithContext(q.ctx, query, true, q.lang, "")
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}
	return nodesFrom(result, block), nil
}

// nodesFrom converts SPARQL rows to tree nodes.
//
// ?hasChildren, when the query projects it, decides whether a node gets an
// expander. When it does NOT, every node is optimistically lazy: the expander is
// rendered, and the client drops it if the level comes back empty. That is one
// wasted request per leaf the user opens, in exchange for the common query staying
// free of an EXISTS subquery on every row.
func nodesFrom(result sparql.QueryResult, block parser.TreeQueries) []treeNode {
	hasChildrenProjected := false
	for _, v := range result.Vars {
		if v == "hasChildren" {
			hasChildrenProjected = true
		}
	}

	seen := make(map[string]bool, len(result.Bindings))
	nodes := make([]treeNode, 0, len(result.Bindings))
	for _, row := range result.Bindings {
		nb, ok := row["node"]
		if !ok || nb.Value == "" {
			continue
		}
		// A node with two labels multiplies rows. Keep the first and move on rather
		// than rendering the same node twice.
		if seen[nb.Value] {
			continue
		}
		seen[nb.Value] = true

		n := treeNode{Key: nb.Value, Title: nb.DisplayText, Lazy: true}
		if n.Title == "" {
			n.Title = nb.Value
		}
		if lb, ok := row["label"]; ok && lb.Value != "" {
			n.Title = lb.DisplayText
			if n.Title == "" {
				n.Title = lb.Value
			}
		}
		if hasChildrenProjected {
			n.Lazy = truthyBinding(row["hasChildren"])
		}
		if icon, ok := result.Icons[nb.Value]; ok {
			n.Icon = icon
		}

		for name, b := range row {
			switch name {
			case "node", "parent", "label", "hasChildren", "score":
				continue
			}
			if n.Extra == nil {
				n.Extra = map[string]treeCell{}
			}
			label := b.DisplayText
			if label == "" {
				label = b.Value
			}
			n.Extra[name] = treeCell{Value: b.Value, Label: label, Type: b.Type}
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// truthyBinding reads a SPARQL boolean. EXISTS yields "true"/"false" literals;
// some endpoints render them as "1"/"0".
func truthyBinding(b sparql.Binding) bool {
	switch strings.ToLower(strings.TrimSpace(b.Value)) {
	case "true", "1":
		return true
	}
	return false
}

// serveParents answers one upward step for a batch of nodes. The client needs this
// only when walking to a focus target; /focus does the whole walk server-side and
// is what the partial actually calls.
func serveParents(c *gin.Context, block parser.TreeQueries) {
	declared, ok := block.Roles[tree.RoleParents]
	if !ok {
		badTreeRequest(c, http.StatusBadRequest,
			`this tree declares no <sparql-tree-query role="parents">`)
		return
	}
	nodes := c.QueryArray("node")
	if len(nodes) == 0 {
		badTreeRequest(c, http.StatusBadRequest, "parents requires at least one ?node=<iri>")
		return
	}
	if len(nodes) > maxBatchNodes {
		badTreeRequest(c, http.StatusBadRequest,
			fmt.Sprintf("too many nodes in one request (%d, max %d)", len(nodes), maxBatchNodes))
		return
	}
	if err := validateIRIs(nodes); err != nil {
		badTreeRequest(c, http.StatusBadRequest, err.Error())
		return
	}

	q := newTreeQueryContext(c)
	defer q.cancel()

	query, err := tree.ParentsQuery(declared, q.pageIRI, nodes)
	if err != nil {
		badTreeRequest(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := q.preprocessor.ExecuteQueryWithContext(q.ctx, query, true, q.lang, "")
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"pairs": []any{}, "error": err.Error()})
		return
	}
	markCacheable(c)
	c.JSON(http.StatusOK, gin.H{"pairs": parentPairs(result)})
}

// parentPairs extracts the (node, parent) edges of a parents result.
func parentPairs(result sparql.QueryResult) []map[string]string {
	pairs := make([]map[string]string, 0, len(result.Bindings))
	for _, row := range result.Bindings {
		n, okN := row["node"]
		p, okP := row["parent"]
		if !okN || !okP || n.Value == "" || p.Value == "" {
			continue
		}
		pairs = append(pairs, map[string]string{"node": n.Value, "parent": p.Value})
	}
	return pairs
}

// serveSearch answers a search across the whole hierarchy.
func serveSearch(c *gin.Context, block parser.TreeQueries) {
	declared, ok := block.Roles[tree.RoleSearch]
	if !ok {
		badTreeRequest(c, http.StatusBadRequest,
			`this tree declares no <sparql-tree-query role="search">`)
		return
	}
	term := c.Query("q")
	if strings.TrimSpace(term) == "" {
		badTreeRequest(c, http.StatusBadRequest, "search requires ?q=<term>")
		return
	}

	mode, limit := tree.ModeFor(declared, requestedLimit(c))
	q := newTreeQueryContext(c)
	defer q.cancel()

	query, err := tree.SearchQuery(declared, q.pageIRI, term, limit+1)
	if err != nil {
		badTreeRequest(c, http.StatusBadRequest, err.Error())
		return
	}
	nodes, execErr := runLevel(c, q, block, query)
	if execErr != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, levelEnvelope{Nodes: []treeNode{}, Complete: true,
			LimitMode: string(mode), Error: execErr.Error()})
		return
	}
	complete := len(nodes) <= limit
	if !complete {
		nodes = nodes[:limit]
	}
	markCacheable(c)
	c.JSON(http.StatusOK, levelEnvelope{
		Nodes: nodes, Total: len(nodes), Complete: complete, LimitMode: string(mode),
	})
}

// serveNodeData batch-fetches extra bindings for a set of visible nodes.
func serveNodeData(c *gin.Context, block parser.TreeQueries) {
	declared, ok := block.Roles[tree.RoleNodeData]
	if !ok {
		badTreeRequest(c, http.StatusBadRequest,
			`this tree declares no <sparql-tree-query role="node-data">`)
		return
	}
	nodes := c.QueryArray("node")
	if len(nodes) == 0 {
		badTreeRequest(c, http.StatusBadRequest, "node-data requires at least one ?node=<iri>")
		return
	}
	if len(nodes) > maxBatchNodes {
		badTreeRequest(c, http.StatusBadRequest,
			fmt.Sprintf("too many nodes in one request (%d, max %d)", len(nodes), maxBatchNodes))
		return
	}
	if err := validateIRIs(nodes); err != nil {
		badTreeRequest(c, http.StatusBadRequest, err.Error())
		return
	}

	q := newTreeQueryContext(c)
	defer q.cancel()

	query, err := tree.NodeDataQuery(declared, q.pageIRI, nodes)
	if err != nil {
		badTreeRequest(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := q.preprocessor.ExecuteQueryWithContext(q.ctx, query, true, q.lang, "")
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"nodes": []any{}, "error": err.Error()})
		return
	}
	markCacheable(c)
	c.JSON(http.StatusOK, gin.H{"nodes": nodesFrom(result, block)})
}

// focusResponse carries an ancestor chain and every level along it.
type focusResponse struct {
	Path   []string                 `json:"path"`   // root → … → target
	Levels map[string]levelEnvelope `json:"levels"` // parent IRI ("" = roots) → that level
	Error  string                   `json:"error,omitempty"`
}

// serveFocus resolves the path from the roots down to one node and returns every
// level along it, in a single round trip.
//
// Without this the client walks parents recursively and then fetches each level in
// turn: 4-8 sequential round trips on a deep hierarchy, paid on EVERY navigation,
// because the tree re-mounts on each page load. It is the same work either way —
// this does it in one request instead of a chain of them.
func serveFocus(c *gin.Context, block parser.TreeQueries) {
	parentsDecl, ok := block.Roles[tree.RoleParents]
	if !ok {
		badTreeRequest(c, http.StatusBadRequest,
			`focus needs a <sparql-tree-query role="parents"> to walk up from the node`)
		return
	}
	target := c.Query("node")
	if target == "" {
		badTreeRequest(c, http.StatusBadRequest, "focus requires ?node=<iri>")
		return
	}
	if err := sparql.ValidateIRI(target); err != nil {
		badTreeRequest(c, http.StatusBadRequest, err.Error())
		return
	}

	q := newTreeQueryContext(c)
	defer q.cancel()

	// Walk up one level at a time. Single-level by design (a transitive path is
	// expensive on endpoints that handle property paths badly), and batched per
	// level, so this is one query per DEPTH rather than one per node.
	path := []string{target}
	seen := map[string]bool{target: true}
	current := target
	for depth := 0; depth < maxFocusDepth; depth++ {
		query, err := tree.ParentsQuery(parentsDecl, q.pageIRI, []string{current})
		if err != nil {
			badTreeRequest(c, http.StatusBadRequest, err.Error())
			return
		}
		result, err := q.preprocessor.ExecuteQueryWithContext(q.ctx, query, false, q.lang, "")
		if err != nil {
			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusOK, focusResponse{Path: nil, Levels: nil, Error: err.Error()})
			return
		}
		pairs := parentPairs(result)
		if len(pairs) == 0 {
			break // reached a root
		}
		// A polyhierarchy has several parents; only one branch can expand, so take
		// the first and document that. Sorting would be arbitrary either way.
		parent := pairs[0]["parent"]
		if seen[parent] {
			break // cycle in the data; stop rather than looping to the timeout
		}
		seen[parent] = true
		path = append([]string{parent}, path...)
		current = parent
	}

	// Now fetch the levels along that path: the roots, then the children of every
	// ancestor. The target's own children are not loaded — expanding it is the
	// user's next action, not part of revealing it.
	levels := map[string]levelEnvelope{}
	rootsDecl := block.Roles[tree.RoleRoots]
	rootsMode, rootsLimit := tree.ModeFor(rootsDecl, requestedLimit(c))
	if env, err := focusLevel(c, q, block, rootsDecl, "", rootsLimit, rootsMode); err == nil {
		levels[""] = env
	}
	childrenDecl := block.Roles[tree.RoleChildren]
	childMode, childLimit := tree.ModeFor(childrenDecl, requestedLimit(c))
	for _, ancestor := range path[:len(path)-1] {
		env, err := focusLevel(c, q, block, childrenDecl, ancestor, childLimit, childMode)
		if err != nil {
			continue // a failed level must not blank the whole focus response
		}
		levels[ancestor] = env
	}

	markCacheable(c)
	c.JSON(http.StatusOK, focusResponse{Path: path, Levels: levels})
}

// focusLevel loads one level for the focus response.
func focusLevel(c *gin.Context, q treeQueryContext, block parser.TreeQueries,
	declared, parent string, limit int, mode tree.LimitMode) (levelEnvelope, error) {

	var query string
	var err error
	fetch := limit + 1
	if parent == "" {
		query, err = tree.RootsQuery(declared, q.pageIRI, fetch, 0)
	} else {
		query, err = tree.ChildrenQuery(declared, q.pageIRI, parent, fetch, 0)
	}
	if err != nil {
		return levelEnvelope{}, err
	}
	nodes, err := runLevel(c, q, block, query)
	if err != nil {
		return levelEnvelope{}, err
	}
	complete := len(nodes) <= limit
	if !complete {
		nodes = nodes[:limit]
	}
	return levelEnvelope{Nodes: nodes, Total: len(nodes), Complete: complete, LimitMode: string(mode)}, nil
}

// requestedLimit reads ?limit=, clamped. 0 means "use the role's default".
func requestedLimit(c *gin.Context) int {
	n, err := strconv.Atoi(c.Query("limit"))
	if err != nil || n < 1 {
		return 0
	}
	if n > tree.DefaultCapLimit {
		return tree.DefaultCapLimit
	}
	return n
}

// requestedOffset reads ?offset=, for "load more" under page-size semantics.
func requestedOffset(c *gin.Context) int {
	n, err := strconv.Atoi(c.Query("offset"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
