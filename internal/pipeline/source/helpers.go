package source

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"hutzli.org/visoto/internal/pipeline/rdf"
)

// Skolemizer replaces blank nodes with deterministic IRIs (R-CAT-3).
//
// Blank node labels are scoped to a single response, so re-harvesting the same
// catalogue would otherwise mint fresh identifiers for the same contact point or
// distribution on every run, and every run would look like a change. Identity is
// therefore derived from content: a node's own predicate-object pairs, plus the
// parent record it hangs off.
//
// Nested blank nodes are handled one level deep — an inner node's digest simply
// omits objects that are themselves blank. Deeper nesting stays deterministic
// but loses discriminating power, which is acceptable: DCAT-AP does not nest
// blank nodes deeply, and the alternative (a fixpoint over the whole graph) is
// not worth its cost here.
type Skolemizer struct {
	minter *rdf.Minter
	parent string
}

// NewSkolemizer returns a skolemizer whose IRIs are rooted at parentIRI, which
// should be the dataset the blank nodes were reached from.
func NewSkolemizer(m *rdf.Minter, parentIRI string) *Skolemizer {
	return &Skolemizer{minter: m, parent: parentIRI}
}

// Apply rewrites every blank node in quads to a minted IRI, returning a new
// slice. Quads without blank nodes pass through unchanged.
func (s *Skolemizer) Apply(quads []rdf.Quad) []rdf.Quad {
	labels := s.collectLabels(quads)
	if len(labels) == 0 {
		return quads
	}
	mapping := s.mint(labels, quads)

	out := make([]rdf.Quad, 0, len(quads))
	for _, q := range quads {
		if q.Subject.Kind == rdf.KindBlank {
			if iri, ok := mapping[q.Subject.Value]; ok {
				q.Subject = iri
			}
		}
		if q.Object.Kind == rdf.KindBlank {
			if iri, ok := mapping[q.Object.Value]; ok {
				q.Object = iri
			}
		}
		out = append(out, q)
	}
	return out
}

func (s *Skolemizer) collectLabels(quads []rdf.Quad) map[string]bool {
	labels := make(map[string]bool)
	for _, q := range quads {
		if q.Subject.Kind == rdf.KindBlank {
			labels[q.Subject.Value] = true
		}
		if q.Object.Kind == rdf.KindBlank {
			labels[q.Object.Value] = true
		}
	}
	return labels
}

// mint derives one IRI per blank node label from that node's own statements.
func (s *Skolemizer) mint(labels map[string]bool, quads []rdf.Quad) map[string]rdf.Term {
	content := make(map[string][]string, len(labels))
	for _, q := range quads {
		if q.Subject.Kind != rdf.KindBlank {
			continue
		}
		if q.Object.Kind == rdf.KindBlank {
			continue // excluded so the digest cannot depend on another label
		}
		content[q.Subject.Value] = append(content[q.Subject.Value],
			q.Predicate.String()+" "+q.Object.String())
	}

	mapping := make(map[string]rdf.Term, len(labels))
	for label := range labels {
		pairs := content[label]
		sort.Strings(pairs) // statement order from an endpoint is not stable
		key := strings.Join(pairs, "\n")
		if key == "" {
			// A node with no statements of its own carries no content to key on;
			// fall back to its label so the run at least completes.
			key = "label:" + label
		}
		mapping[label] = s.minter.Skolem(s.parent, "node", key)
	}
	return mapping
}

// Limiter paces requests to one host. Politeness is a hard requirement: a naive
// worker pool gets the harvester blocked by the portal (R-FET-5).
type Limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// NewLimiter returns a limiter enforcing at least interval between requests.
// A non-positive interval disables pacing.
func NewLimiter(interval time.Duration) *Limiter {
	return &Limiter{interval: interval}
}

// Wait blocks until the next request may be sent, or until ctx is done.
func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil || l.interval <= 0 {
		return ctx.Err()
	}

	l.mu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if now.Before(l.next) {
		wait = l.next.Sub(now)
		l.next = l.next.Add(l.interval)
	} else {
		l.next = now.Add(l.interval)
	}
	l.mu.Unlock()

	if wait <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
