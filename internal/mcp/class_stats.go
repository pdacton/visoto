package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	goMcp "github.com/mark3labs/mcp-go/mcp"

	"hutzli.org/visoto/internal/classstats"
	"hutzli.org/visoto/internal/config"
)

// snapshotHint tells the model what kind of numbers it got.
func snapshotHint(s *classstats.Snapshot) string {
	h := "Counts come from Visoto's daily class statistics (snapshot of " + s.Day + "), not a live query."
	if s.Engine == "graphdb" {
		h += " They are read from GraphDB index statistics and are approximate."
	}
	if s.Failures > 0 {
		h += fmt.Sprintf(" %d classes could not be counted in that run and are missing.", s.Failures)
	}
	return h
}

// endpointConfig resolves the tools' endpoint parameter (slug, name or URL;
// empty = default) to its configuration, nil when none matches.
func (tc *toolContext) endpointConfig(endpoint string) *config.SparqlEndpoint {
	app := &tc.cfg.Application
	if endpoint == "" {
		return app.DefaultEndpoint()
	}
	if ep := app.GetEndpointBySlug(endpoint); ep != nil {
		return ep
	}
	for i := range app.SparqlEndpoints {
		ep := &app.SparqlEndpoints[i]
		if strings.EqualFold(ep.Name, endpoint) || ep.URL == endpoint {
			return ep
		}
	}
	return nil
}

// snapshot is the class statistics snapshot serving `endpoint` (nil if none),
// with the source slug and the endpoint's own URL (for visoto_link).
func (tc *toolContext) snapshot(endpoint string) (*classstats.Snapshot, string, string) {
	ep := tc.endpointConfig(endpoint)
	if ep == nil || tc.classStats == nil {
		return nil, "", ""
	}
	snap, src := tc.classStats.For(ep.Slug)
	return snap, src, ep.URL
}

// countInstancesFromSnapshot answers count_instances from the daily snapshot.
// Without class_iri the live query (a GROUP BY over every rdf:type) times out on
// large stores, so the snapshot is the better answer whenever there is one.
// With class_iri, a class the snapshot has is answered from it; others fall
// through to the live count (ok = false).
func (tc *toolContext) countInstancesFromSnapshot(ctx context.Context, classIRI, endpoint string) (toolResult, bool) {
	snap, _, epURL := tc.snapshot(endpoint)
	if snap == nil {
		return toolResult{}, false
	}
	r := toolResult{EndpointUsed: epURL, Hints: []string{snapshotHint(snap)}}
	if classIRI != "" {
		n, ok := snap.Counts[classIRI]
		if !ok {
			return toolResult{}, false
		}
		r.RowCount = 1
		r.Results = []map[string]any{{"count": strconv.FormatInt(n, 10)}}
		return r, true
	}
	top := snap.Top(100)
	r.RowCount = len(top)
	for _, c := range top {
		r.Results = append(r.Results, map[string]any{
			"type":             c.Class,
			"type_visoto_link": tc.visotoLink(ctx, c.Class, epURL),
			"count":            strconv.FormatInt(c.Count, 10),
		})
	}
	return r, true
}

// handleClassTrends serves class_trends: how the instance counts changed over
// the last `days` days (without class_iri), or one class's daily series.
func (tc *toolContext) handleClassTrends(ctx context.Context, request goMcp.CallToolRequest) (*goMcp.CallToolResult, error) {
	endpoint := getStringParam(request, "endpoint")
	classIRI := getStringParam(request, "class_iri")
	days := getIntParam(request, "days", 7)

	snap, src, epURL := tc.snapshot(endpoint)
	if snap == nil {
		return toMCPResult(toolResult{Error: "No daily class statistics for this endpoint. They are collected for endpoints with class_stats = true in visoto.config."})
	}
	r := toolResult{EndpointUsed: epURL, Hints: []string{snapshotHint(snap)}}

	if classIRI != "" {
		pts, err := tc.classStats.Series(src, classIRI, days)
		if err != nil {
			return toMCPResult(toolResult{Error: err.Error()})
		}
		r.VisotoLink = tc.visotoLink(ctx, classIRI, epURL)
		for _, p := range pts {
			row := map[string]any{"day": p.Day}
			if p.Count != nil {
				row["count"] = strconv.FormatInt(*p.Count, 10)
			} else {
				row["count"] = "not counted"
			}
			r.Results = append(r.Results, row)
		}
		r.RowCount = len(r.Results)
		return toMCPResult(r)
	}

	ch, err := tc.classStats.Changes(src, days)
	if err != nil {
		return toMCPResult(toolResult{Error: err.Error()})
	}
	if ch.From == "" {
		r.Hints = append(r.Hints, fmt.Sprintf("There is no snapshot %d or more days before %s yet, so nothing to compare.", days, ch.To))
		return toMCPResult(r)
	}
	r.Hints = append(r.Hints, fmt.Sprintf("Compares %s with %s. Store size: %s → %s triples.", ch.From, ch.To, fmtCount(ch.TriplesBefore), fmtCount(ch.TriplesAfter)))
	limit := getIntParam(request, "limit", 50)
	for i, c := range ch.Changes {
		if i >= limit {
			r.Hints = append(r.Hints, fmt.Sprintf("%d more changed classes not shown — raise limit.", len(ch.Changes)-limit))
			break
		}
		row := map[string]any{
			"class":             c.Class,
			"class_visoto_link": tc.visotoLink(ctx, c.Class, epURL),
			"before":            fmtCount(c.Before),
			"after":             fmtCount(c.After),
			"delta":             strconv.FormatInt(c.Delta, 10),
		}
		switch {
		case c.Before == nil:
			row["change"] = "new"
		case c.After == nil:
			row["change"] = "vanished"
		case c.Drop:
			row["change"] = "drop"
		}
		if c.Pct != nil {
			row["percent"] = fmt.Sprintf("%+.1f%%", *c.Pct*100)
		}
		r.Results = append(r.Results, row)
	}
	r.RowCount = len(r.Results)
	return toMCPResult(r)
}

func fmtCount(n *int64) string {
	if n == nil {
		return "-"
	}
	return strconv.FormatInt(*n, 10)
}
