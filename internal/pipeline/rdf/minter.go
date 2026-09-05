package rdf

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Minter builds the pipeline's IRIs. Every IRI it returns is a deterministic
// function of its inputs, so a re-run over unchanged input mints the identical
// IRI (R-CAT-3, R-SIG-2) and produces an empty diff.
//
// The base IRI is frozen once and configured, never derived from the deployment
// host: rewriting minted IRIs after the first load is expensive (D2).
type Minter struct {
	base string
}

// NewMinter returns a minter rooted at base. A missing trailing slash is added
// so callers need not care.
func NewMinter(base string) *Minter {
	if base == "" {
		base = DefaultBaseIRI
	}
	if !strings.HasSuffix(base, "/") && !strings.HasSuffix(base, "#") {
		base += "/"
	}
	return &Minter{base: base}
}

// DefaultBaseIRI is used when the config names none.
const DefaultBaseIRI = "https://visoto.hutzli.org/id/"

// Base returns the configured base IRI, for logging and provenance.
func (m *Minter) Base() string { return m.base }

// RunIRI names one execution of the pipeline over one source.
func (m *Minter) RunIRI(runID string) Term { return IRI(m.base + "run/" + runID) }

// CatalogGraph is the graph holding one source's verbatim DCAT-AP for one run.
func (m *Minter) CatalogGraph(source, runID string) string {
	return m.base + "graph/catalog/" + slug(source) + "/" + runID
}

// StructureGraph is the graph holding one distribution's derived structure for
// one run. The distribution IRI is hashed rather than embedded: source IRIs are
// arbitrarily long and would make the graph name unreadable.
func (m *Minter) StructureGraph(distIRI, runID string) string {
	return m.base + "graph/structure/" + Hash(distIRI) + "/" + runID
}

// RunGraph holds run provenance for every source. Cumulative.
func (m *Minter) RunGraph() string { return m.base + "graph/run" }

// SignatureGraph holds signature nodes and their annotations. Cumulative.
func (m *Minter) SignatureGraph() string { return m.base + "graph/signature" }

// CurrentGraph holds one source's pointers to the graphs a reader should use,
// so readers need no date arithmetic over run timestamps (R-CAT-4).
//
// It is scoped per source rather than shared, because the run that rewrites it
// knows only about its own source: a single shared pointer graph could not be
// replaced without erasing every other source's pointers.
func (m *Minter) CurrentGraph(source string) string {
	return m.base + "graph/current/" + slug(source)
}

// SourceIRI names a configured source as a resource, so run provenance and the
// current-graph pointers have something to hang off.
func (m *Minter) SourceIRI(source string) Term {
	return IRI(m.base + "source/" + slug(source))
}

// StructureIRI names the structure derived from one distribution in one run.
func (m *Minter) StructureIRI(distIRI, runID string) Term {
	return IRI(m.base + "structure/" + Hash(distIRI) + "/" + runID)
}

// FieldIRI names one field within a structure. The locator, not the field name,
// is the identity: names are absent in many formats and duplicated in others.
func (m *Minter) FieldIRI(distIRI, runID, locator string) Term {
	return IRI(m.base + "field/" + Hash(distIRI) + "/" + runID + "/" + Hash(locator))
}

// SignatureIRI names a field signature. The key is the content-derived digest
// computed by the profiler, so the same field content mints the same IRI in
// every run and across every source (§6.4).
func (m *Minter) SignatureIRI(key string) Term {
	return IRI(m.base + "sig/" + Hash(key))
}

// Skolem gives a stable IRI to something the source expressed as a blank node.
// Identity is the parent IRI plus a kind and a local key, so re-harvesting the
// same catalogue reproduces the same IRI instead of accumulating fresh ones.
func (m *Minter) Skolem(parentIRI, kind, key string) Term {
	return IRI(m.base + "skolem/" + slug(kind) + "/" + Hash(parentIRI+"\x00"+key))
}

// Hash returns the short digest used throughout the minted IRI space. 16 hex
// characters is 64 bits — ample against accidental collision at the design
// scale of ~10^6 fields (R-NFR-1) and short enough to keep IRIs readable.
func Hash(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// slug reduces a name to the characters that are safe and readable in an IRI
// path segment. Source names come from config, so this is about legibility
// rather than safety, but it also keeps a careless config from minting an IRI
// that needs escaping.
func slug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '.' || r == '_' || r == '-' || r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "unnamed"
	}
	return out
}
