package source

import (
	"strconv"
	"strings"
	"time"

	"hutzli.org/visoto/internal/pipeline/rdf"
)

// ExtractDistributions reads the fetch work items out of a record's DCAT-AP.
//
// It lives here rather than in an adapter because every adapter needs it and
// they must agree: once a CKAN record has been mapped to DCAT-AP, it is read
// exactly the same way as one that arrived as DCAT-AP already.
func ExtractDistributions(datasetIRI string, quads []rdf.Quad) []Distribution {
	// Preserve the order distributions appear in, so re-runs enumerate them the
	// same way and the state table's insert order is reproducible.
	var order []string
	members := make(map[string]bool)
	for _, q := range quads {
		if q.Subject.Value != datasetIRI || q.Predicate != rdf.DcatHasDist {
			continue
		}
		if q.Object.Kind == rdf.KindLiteral || members[q.Object.Value] {
			continue
		}
		members[q.Object.Value] = true
		order = append(order, q.Object.Value)
	}
	if len(order) == 0 {
		return nil
	}

	byIRI := make(map[string]*Distribution, len(order))
	for _, iri := range order {
		byIRI[iri] = &Distribution{IRI: iri, DatasetIRI: datasetIRI}
	}

	for _, q := range quads {
		d, ok := byIRI[q.Subject.Value]
		if !ok {
			continue
		}
		switch q.Predicate {
		case rdf.DcatDownloadURL:
			if q.Object.Kind == rdf.KindIRI || d.DownloadURL == "" {
				d.DownloadURL = q.Object.Value
			}
		case rdf.DcatAccessURL:
			// accessURL is the fallback: for a plain file distribution the two are
			// the same, and for an API distribution it is all there is.
			if d.DownloadURL == "" {
				d.DownloadURL = q.Object.Value
			}
		case rdf.DcatMediaType:
			if v := MediaTypeValue(q.Object); v != "" {
				d.DeclaredMedia = v
			}
		case rdf.DctFormat:
			if v := MediaTypeValue(q.Object); v != "" {
				d.DeclaredFormat = v
			}
		case rdf.DcatByteSize:
			if n, err := strconv.ParseInt(strings.TrimSpace(q.Object.Value), 10, 64); err == nil && n >= 0 {
				d.ByteSize = n
			}
		case rdf.DctLicense:
			if d.Licence == "" {
				d.Licence = q.Object.Value
			}
		case rdf.DctRights:
			if d.Licence == "" {
				d.Licence = q.Object.Value
			}
		case rdf.DctModified:
			if t, ok := ParseTime(q.Object.Value); ok {
				d.Modified = t
			}
		case rdf.DctIssued:
			if d.Modified.IsZero() {
				if t, ok := ParseTime(q.Object.Value); ok {
					d.Modified = t
				}
			}
		}
	}

	out := make([]Distribution, 0, len(order))
	for _, iri := range order {
		out = append(out, *byIRI[iri])
	}
	return out
}

// MediaTypeValue normalizes the many shapes a media type arrives in.
//
// Catalogues express it as a bare "text/csv", as an IANA IRI, as an EU
// file-type authority IRI, or as a bare "CSV". All of them are kept as the
// portal's *declared* type — advisory input to detection, never a conclusion
// (R-SNF-1).
func MediaTypeValue(t rdf.Term) string {
	v := strings.TrimSpace(t.Value)
	if v == "" {
		return ""
	}
	if t.Kind != rdf.KindIRI {
		return v
	}
	switch {
	case strings.Contains(v, "iana.org/assignments/media-types/"):
		// .../media-types/text/csv → text/csv
		_, rest, _ := strings.Cut(v, "media-types/")
		return rest
	case strings.Contains(v, "/file-type/"):
		// EU authority list: .../file-type/CSV → CSV
		idx := strings.LastIndex(v, "/")
		return v[idx+1:]
	default:
		return v
	}
}

// timeLayouts are the date shapes seen in catalogue metadata, most specific
// first. Portals are inconsistent even within one catalogue.
var timeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05.999999Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"2006-01",
	"2006",
}

// ParseTime parses a catalogue timestamp, reporting whether it was understood.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
