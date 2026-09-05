package signature

import (
	"strings"
	"unicode"

	"hutzli.org/visoto/internal/pipeline/rdf"
)

// Descriptor is everything that identifies a field's content, as observed by the
// profiler. It is the input to both levels of identity described below.
type Descriptor struct {
	// Name as it appears in the data: a CSV header, an XML element name, the
	// local part of a predicate IRI.
	Name string

	// Datatype is the inferred XSD datatype IRI.
	Datatype string

	// PatternClass is the dominant shape of the values, e.g. "9999" for a
	// four-digit number.
	PatternClass string

	// DistinctCount and RowCount give the cardinality class.
	DistinctCount int64
	RowCount      int64

	// Sketch is the MinHash over the field's values.
	Sketch *Sketch
}

// Key is the exact signature key: fields agreeing on all of it are the same
// signature without any similarity computation.
//
// This is the cheap half of identity. It buckets `bfs_nr` with `BFS-Nr.` but not
// with `gemeinde_id`, which is why the similarity link (§6.4) is the other half:
// exact keys give precision, sketches give recall.
func (d Descriptor) Key() string {
	return strings.Join([]string{
		NormalizeName(d.Name),
		shortDatatype(d.Datatype),
		d.PatternClass,
		d.CardinalityClass(),
	}, "|")
}

// CardinalityClass buckets the distinct-to-row ratio. Buckets rather than the
// raw ratio, so that the same column sampled at different sizes still lands in
// one signature.
func (d Descriptor) CardinalityClass() string {
	switch {
	case d.RowCount <= 0:
		return "unknown"
	case d.DistinctCount <= 1:
		return "constant"
	case d.DistinctCount == d.RowCount:
		return "unique"
	case d.DistinctCount <= 12:
		return "enum"
	}
	switch ratio := float64(d.DistinctCount) / float64(d.RowCount); {
	case ratio < 0.01:
		return "code"
	case ratio < 0.5:
		return "categorical"
	default:
		return "high"
	}
}

// NormalizeName reduces a field name to the form used for signature identity.
//
// Case, separators and the language suffixes that multilingual catalogues append
// carry no information about what a column holds: `Gemeinde-Nr._de` and
// `gemeinde_nr` are the same column. Everything else is left alone — over-
// normalizing would merge fields that genuinely differ.
func NormalizeName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	for _, suffix := range langSuffixes {
		if len(out) > len(suffix)+2 && strings.HasSuffix(out, suffix) {
			return strings.TrimSuffix(out, suffix)
		}
	}
	return out
}

// langSuffixes are the language markers catalogues append to column names.
var langSuffixes = []string{"de", "fr", "it", "en", "rm"}

// PatternClass reduces a value to its shape: digit runs become 9s, letter runs
// become a's, everything else is kept literally.
//
// The shape is what distinguishes a four-digit municipality number from a
// four-digit year in a column whose name says neither.
func PatternClass(value string) string {
	const maxRun = 40
	var b strings.Builder
	var lastKind rune
	var runLen int

	flush := func() {
		for i := 0; i < runLen && i < maxRun; i++ {
			b.WriteRune(lastKind)
		}
		runLen = 0
	}

	for _, r := range value {
		var kind rune
		switch {
		case unicode.IsDigit(r):
			kind = '9'
		case unicode.IsLetter(r):
			kind = 'a'
		default:
			kind = r
		}
		if kind == lastKind && (kind == '9' || kind == 'a') {
			runLen++
			continue
		}
		flush()
		lastKind, runLen = kind, 1
	}
	flush()

	out := b.String()
	if len(out) > maxRun*2 {
		return out[:maxRun*2]
	}
	return out
}

// shortDatatype trims a datatype IRI to its local name, so xsd:integer and a
// bare "integer" produce the same key.
func shortDatatype(datatype string) string {
	d := strings.TrimSpace(datatype)
	if d == "" {
		return "unknown"
	}
	if i := strings.LastIndexAny(d, "#/"); i >= 0 {
		return d[i+1:]
	}
	return d
}

// DatatypeIRI expands a bare datatype name back to an XSD IRI, for emission.
func DatatypeIRI(name string) string {
	if strings.Contains(name, ":") || strings.Contains(name, "/") {
		return name
	}
	return rdf.Xsd(name)
}
