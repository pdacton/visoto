package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"hutzli.org/visoto/internal/parser"
	"hutzli.org/visoto/internal/sparql"
)

// Bounds for the generic cube observation table. Both are deliberate:
//
//   - Columns: a cube declares its dimensions in its SHACL constraint, and the
//     count varies enormously — the petitions cube has 5, an NFI cube 74, the
//     politics committee-type cube 288. The generated query needs one OPTIONAL
//     per dimension, so an uncapped table would emit a 288-OPTIONAL query that is
//     both unreadable and slow. Dimensions are ordered by sh:order (the order the
//     publisher intended) and the rest are reachable on each observation's page.
//   - Rows: loaded once and paged locally by Tabulator. The largest observation
//     set in LINDAS holds 748,720 observations, so "all rows" is not an option.
const (
	maxCubeTableColumns = 15
	maxCubeTableRows    = 1000
)

// cubeDimension is one column of the generated table: the predicate to read from
// each observation, and the variable name it is bound to.
type cubeDimension struct {
	IRI   string
	Var   string
	Label string
}

// cubeTableHandler serves GET /api/cube-table/:id — the backing fragment for the
// sparqlCube partial.
//
// Cube observations carry per-cube dimension predicates: the petitions cube uses
// .../petition/id, /titel, /datum, while an electricity cube uses /period,
// /category, /total. There is therefore no fixed column set, and SPARQL cannot
// project a variable whose name is computed at query time. So this runs two
// queries: one reads the dimension list from the cube's observation constraint,
// then a second is GENERATED from that list with one OPTIONAL per dimension.
// The result is an ordinary wide table that the existing sparqlTable partial
// renders without knowing anything about cubes.
func cubeTableHandler(c *gin.Context) {
	id := c.Param("id")
	dataLang := queryLang(c)

	iri := c.Query("iri")
	if iri == "" {
		c.String(http.StatusBadRequest, "missing iri")
		return
	}
	// The IRI is request input and is interpolated into <...> terms below, so it
	// is validated before use — an unvalidated one could close the term early and
	// inject arbitrary graph patterns.
	if _, err := sparql.IRITerm(iri); err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusBadRequest, "invalid iri")
		return
	}

	preprocessor := prepareQueryInputs(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), cfg.GetTimeout())
	defer cancel()

	dims, err := cubeDimensions(ctx, preprocessor, iri, dataLang)
	if err != nil || len(dims) == 0 {
		// No constraint, or it declares no dimensions: render an empty card rather
		// than a broken one. Not cacheable — a transient query failure must not be
		// served as "this cube has no data" for hours.
		params := map[string]any{
			"id": id, "title": c.Query("title"), "icon": c.Query("icon"), "lang": dataLang,
			"result": sparql.QueryResult{},
		}
		if err != nil {
			params["result"] = sparql.QueryResult{Error: err.Error()}
		}
		renderTableFragment(c, params, err == nil)
		return
	}

	query := buildCubeObservationQuery(iri, dims)
	result, execErr := preprocessor.ExecuteQuery(query, true, dataLang, "")
	if execErr != nil {
		result.Error = execErr.Error()
	}

	params := map[string]any{
		"id":     id,
		"title":  c.Query("title"),
		"icon":   c.Query("icon"),
		"lang":   dataLang,
		"result": result,
	}
	renderTableFragment(c, params, execErr == nil)
}

// cubeDimensions reads the cube's dimension list from its SHACL observation
// constraint, ordered by sh:order where the publisher declared one.
//
// rdf:type and cube:observedBy are excluded: every observation carries them, and
// neither is a dimension of the data — they would just be two constant columns.
func cubeDimensions(ctx context.Context, preprocessor *parser.Preprocessor, iri, dataLang string) ([]cubeDimension, error) {
	iriTerm, err := sparql.IRITerm(iri)
	if err != nil {
		return nil, err
	}

	// SAMPLE + GROUP BY: a dimension's shape may carry several names (one per
	// language) and sh:in lists, either of which would multiply rows.
	//
	// The name is filtered to the display language first. Without the filter
	// SAMPLE picks arbitrarily, and since LINDAS cubes are labelled in de/fr/it/en
	// the headers come back as a mix of languages within one table. The second
	// OPTIONAL is the fallback for dimensions labelled in one language only, or
	// with an untagged literal.
	q := fmt.Sprintf(`
SELECT ?dimension (SAMPLE(?name_) AS ?name) (SAMPLE(?order_) AS ?order) WHERE {
  %s <https://cube.link/observationConstraint>/<http://www.w3.org/ns/shacl#property> ?shape .
  ?shape <http://www.w3.org/ns/shacl#path> ?dimension .
  FILTER(?dimension != <http://www.w3.org/1999/02/22-rdf-syntax-ns#type>)
  FILTER(?dimension != <https://cube.link/observedBy>)
  FILTER(isIRI(?dimension))
  OPTIONAL {
    ?shape <http://schema.org/name> ?preferred_ .
    FILTER(lang(?preferred_) = visoto:dispLang || lang(?preferred_) = "")
  }
  OPTIONAL { ?shape <http://schema.org/name> ?any_ }
  BIND(COALESCE(?preferred_, ?any_) AS ?name_)
  OPTIONAL { ?shape <http://www.w3.org/ns/shacl#order> ?order_ }
} GROUP BY ?dimension`, iriTerm)

	result, err := preprocessor.ExecuteQuery(q, false, dataLang, "")
	if err != nil {
		return nil, err
	}

	type row struct {
		iri, name string
		order     float64
		hasOrder  bool
	}
	rows := make([]row, 0, len(result.Bindings))
	for _, b := range result.Bindings {
		d, ok := b["dimension"]
		if !ok || d.Value == "" {
			continue
		}
		r := row{iri: d.Value}
		if n, ok := b["name"]; ok {
			r.name = n.Value
		}
		if o, ok := b["order"]; ok && o.Value != "" {
			if _, scanErr := fmt.Sscanf(o.Value, "%f", &r.order); scanErr == nil {
				r.hasOrder = true
			}
		}
		rows = append(rows, r)
	}

	// sh:order first (the order the publisher intended), then unordered
	// dimensions by IRI so the column order is at least stable across requests.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].hasOrder != rows[j].hasOrder {
			return rows[i].hasOrder
		}
		if rows[i].hasOrder && rows[i].order != rows[j].order {
			return rows[i].order < rows[j].order
		}
		return rows[i].iri < rows[j].iri
	})

	if len(rows) > maxCubeTableColumns {
		rows = rows[:maxCubeTableColumns]
	}

	dims := make([]cubeDimension, 0, len(rows))
	used := map[string]bool{"observation": true}
	for _, r := range rows {
		// The variable name IS the column header — a generated table has no
		// <sparql-column> declarations to carry a separate label. So name the
		// column after the dimension's declared schema:name, falling back to the
		// IRI's last segment. Without this, dimensions whose IRI ends in digits
		// (the NFI cubes use .../116, .../18r) render as "dim_116", "dim_18r",
		// because a SPARQL variable cannot start with a digit.
		source := r.name
		if strings.TrimSpace(source) == "" {
			source = r.iri
		}
		dims = append(dims, cubeDimension{IRI: r.iri, Var: cubeVarName(source, used), Label: r.name})
	}
	return dims, nil
}

// cubeVarName derives a safe, unique SPARQL variable name from a dimension's
// declared name or, failing that, its IRI. The column header the user sees is
// this name, so it is kept readable rather than opaque (?dim1, ?dim2 …).
//
// Takes the last path segment only when the input actually looks like an IRI: a
// declared name may legitimately contain "/" ("Fläche / Anzahl"), and splitting
// on it there would throw away the informative half.
func cubeVarName(source string, used map[string]bool) string {
	local := source
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		if i := strings.LastIndexAny(local, "/#"); i >= 0 && i+1 < len(local) {
			local = local[i+1:]
		}
	}
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	// Declared dimension names can be full sentences — the NFI cubes carry labels
	// like "Einfacher Standardfehler für Biomasse der lebenden Bäume pro
	// Waldfläche". As the variable name doubles as the column header, cap it so
	// one dimension cannot blow out the table width. Cut on an underscore
	// boundary where possible so the truncation lands between words.
	const maxVarLen = 32
	if len(name) > maxVarLen {
		cut := name[:maxVarLen]
		if i := strings.LastIndex(cut, "_"); i > maxVarLen/2 {
			cut = cut[:i]
		}
		name = strings.Trim(cut, "_")
	}
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "dim_" + name
	}
	base := name
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s_%d", base, i)
	}
	used[name] = true
	return name
}

// buildCubeObservationQuery generates the wide observation query: a bounded inner
// SELECT of observation IRIs, then one OPTIONAL per dimension.
//
// The inner SELECT is what keeps this affordable. Bounding the observations
// first means the OPTIONALs join over at most maxCubeTableRows subjects instead
// of the whole set — which matters when the set holds 748,720 of them.
func buildCubeObservationQuery(iri string, dims []cubeDimension) string {
	// Validated by the caller; IRITerm again here keeps this function safe on its
	// own terms rather than relying on call order.
	iriTerm, err := sparql.IRITerm(iri)
	if err != nil {
		return ""
	}

	var sel strings.Builder
	sel.WriteString("SELECT ?observation")
	for _, d := range dims {
		sel.WriteString(" ?")
		sel.WriteString(d.Var)
	}

	var body strings.Builder
	fmt.Fprintf(&body, " WHERE {\n  { SELECT ?observation WHERE { %s <https://cube.link/observationSet>/<https://cube.link/observation> ?observation } LIMIT %d }\n",
		iriTerm, maxCubeTableRows)
	for _, d := range dims {
		dimTerm, err := sparql.IRITerm(d.IRI)
		if err != nil {
			continue // skip a dimension IRI that cannot be safely interpolated
		}
		fmt.Fprintf(&body, "  OPTIONAL { ?observation %s ?%s }\n", dimTerm, d.Var)
	}
	body.WriteString("}")

	return sel.String() + body.String()
}
