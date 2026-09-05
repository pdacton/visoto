package ckan

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/source"
)

// searchResult is the payload of a package_search response.
type searchResult struct {
	Count   int           `json:"count"`
	Results []ckanPackage `json:"results"`
}

// ckanPackage is one CKAN dataset. Only the fields that map to DCAT-AP are
// declared; the rest is deliberately ignored rather than guessed at.
type ckanPackage struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Title              multiString    `json:"title"`
	Notes              multiString    `json:"notes"`
	URL                string         `json:"url"`
	Identifier         string         `json:"identifier"`
	LicenseID          string         `json:"license_id"`
	LicenseURL         string         `json:"license_url"`
	MetadataCreated    string         `json:"metadata_created"`
	MetadataModified   string         `json:"metadata_modified"`
	Issued             string         `json:"issued"`
	Modified           string         `json:"modified"`
	AccrualPeriodicity string         `json:"accrual_periodicity"`
	Organization       *ckanOrg       `json:"organization"`
	Tags               []ckanTag      `json:"tags"`
	Groups             []ckanGroup    `json:"groups"`
	Resources          []ckanResource `json:"resources"`
}

type ckanOrg struct {
	Name  string      `json:"name"`
	Title multiString `json:"title"`
	URL   string      `json:"url"`
}

type ckanTag struct {
	Name string `json:"name"`
}

type ckanGroup struct {
	Name  string      `json:"name"`
	Title multiString `json:"title"`
}

type ckanResource struct {
	ID           string      `json:"id"`
	URI          string      `json:"uri"`
	Name         multiString `json:"name"`
	Description  multiString `json:"description"`
	URL          string      `json:"url"`
	DownloadURL  string      `json:"download_url"`
	Format       string      `json:"format"`
	MimeType     string      `json:"mimetype"`
	MediaType    string      `json:"media_type"`
	Size         flexInt     `json:"size"`
	ByteSize     flexInt     `json:"byte_size"`
	Created      string      `json:"created"`
	LastModified string      `json:"last_modified"`
	Issued       string      `json:"issued"`
	Modified     string      `json:"modified"`
	License      string      `json:"license"`
	Rights       string      `json:"rights"`
}

// multiString holds a CKAN text field, which is a plain string in stock CKAN and
// a language map in the multilingual deployments opendata.swiss runs. Both shapes
// appear in the same catalogue, so both have to decode.
type multiString struct {
	Plain  string
	ByLang map[string]string
}

// UnmarshalJSON accepts a string, a language map, or null.
func (m *multiString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		return json.Unmarshal(b, &m.Plain)
	}
	if b[0] == '{' {
		raw := map[string]any{}
		if err := json.Unmarshal(b, &raw); err != nil {
			return err
		}
		m.ByLang = make(map[string]string, len(raw))
		for lang, v := range raw {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				m.ByLang[lang] = s
			}
		}
		return nil
	}
	// Numbers and arrays in a text field are malformed input, not a reason to
	// abandon the whole page.
	return nil
}

// Empty reports whether the field carries no usable text.
func (m multiString) Empty() bool { return m.Plain == "" && len(m.ByLang) == 0 }

// Terms renders the field as RDF literals: one language-tagged literal per
// language, or a single plain literal.
func (m multiString) Terms() []rdf.Term {
	if m.Plain != "" {
		return []rdf.Term{rdf.Literal(m.Plain)}
	}
	out := make([]rdf.Term, 0, len(m.ByLang))
	for _, lang := range sortedKeys(m.ByLang) {
		out = append(out, rdf.LangLiteral(m.ByLang[lang], lang))
	}
	return out
}

// Any returns some text from the field, for logging and fallbacks.
func (m multiString) Any() string {
	if m.Plain != "" {
		return m.Plain
	}
	for _, lang := range sortedKeys(m.ByLang) {
		return m.ByLang[lang]
	}
	return ""
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Sorted so repeated harvests emit statements in the same order.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// flexInt decodes a size that CKAN may express as a number, a numeric string, or
// null.
type flexInt struct {
	Value int64
	Set   bool
}

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil // a non-integer size is dropped, not fatal
	}
	f.Value, f.Set = n, true
	return nil
}

// toRecord maps one CKAN package to DCAT-AP. It reports false for a package with
// no usable identity, which cannot be addressed in RDF at all.
func (s *Source) toRecord(pkg ckanPackage) (source.Record, bool) {
	datasetIRI := s.datasetIRI(pkg)
	if datasetIRI == "" {
		s.log.Warn("skipping ckan package with no name or id",
			slog.String("title", pkg.Title.Any()))
		return source.Record{}, false
	}
	ds := rdf.IRI(datasetIRI)
	b := &quadBuilder{}

	b.add(ds, rdf.A, rdf.DcatDataset)
	b.addText(ds, rdf.DctTitle, pkg.Title)
	b.addText(ds, rdf.DctDescription, pkg.Notes)

	if id := firstNonEmpty(pkg.Identifier, pkg.ID); id != "" {
		b.add(ds, rdf.DctIdentifier, rdf.Literal(id))
	}
	if page := firstNonEmpty(pkg.URL, datasetIRI); isHTTPURL(page) {
		b.add(ds, rdf.DcatLandingPage, rdf.IRI(page))
	}
	b.addTime(ds, rdf.DctIssued, pkg.Issued, pkg.MetadataCreated)
	b.addTime(ds, rdf.DctModified, pkg.Modified, pkg.MetadataModified)
	b.addLicence(ds, pkg.LicenseURL, pkg.LicenseID)
	if isHTTPURL(pkg.AccrualPeriodicity) {
		b.add(ds, rdf.DctAccrualPeriodicity, rdf.IRI(pkg.AccrualPeriodicity))
	}

	for _, tag := range pkg.Tags {
		if name := strings.TrimSpace(tag.Name); name != "" {
			b.add(ds, rdf.DcatKeyword, rdf.Literal(name))
		}
	}

	// Groups become themes addressed by the portal's own group page. That is the
	// portal's identifier for the group, not one we invented.
	for _, g := range pkg.Groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}
		theme := rdf.IRI(s.portalIRI("group", name))
		b.add(ds, rdf.DcatTheme, theme)
		b.addText(theme, rdf.RDFSLabel, g.Title)
	}

	if org := pkg.Organization; org != nil && strings.TrimSpace(org.Name) != "" {
		publisher := rdf.IRI(s.portalIRI("organization", strings.TrimSpace(org.Name)))
		b.add(ds, rdf.DctPublisher, publisher)
		b.add(publisher, rdf.A, rdf.FoafAgent)
		b.addText(publisher, rdf.FoafName, org.Title)
	}

	for i, res := range pkg.Resources {
		s.addResource(b, ds, datasetIRI, i, res)
	}

	quads := b.quads
	return source.Record{
		DatasetIRI:    datasetIRI,
		Quads:         quads,
		Distributions: source.ExtractDistributions(datasetIRI, quads),
	}, true
}

// addResource maps one CKAN resource to a dcat:Distribution.
func (s *Source) addResource(b *quadBuilder, ds rdf.Term, datasetIRI string, index int, res ckanResource) {
	distIRI := s.distributionIRI(datasetIRI, index, res)
	dist := rdf.IRI(distIRI)

	b.add(ds, rdf.DcatHasDist, dist)
	b.add(dist, rdf.A, rdf.DcatDistribution)
	b.addText(dist, rdf.DctTitle, res.Name)
	b.addText(dist, rdf.DctDescription, res.Description)

	download := firstNonEmpty(res.DownloadURL, res.URL)
	if isHTTPURL(download) {
		b.add(dist, rdf.DcatDownloadURL, rdf.IRI(download))
		b.add(dist, rdf.DcatAccessURL, rdf.IRI(download))
	}
	if mt := firstNonEmpty(res.MediaType, res.MimeType); mt != "" {
		b.add(dist, rdf.DcatMediaType, rdf.Literal(strings.TrimSpace(mt)))
	}
	if f := strings.TrimSpace(res.Format); f != "" {
		b.add(dist, rdf.DctFormat, rdf.Literal(f))
	}
	if size := firstSet(res.ByteSize, res.Size); size.Set {
		b.add(dist, rdf.DcatByteSize, rdf.TypedLiteral(strconv.FormatInt(size.Value, 10), rdf.Xsd("decimal")))
	}
	b.addTime(dist, rdf.DctIssued, res.Issued, res.Created)
	b.addTime(dist, rdf.DctModified, res.Modified, res.LastModified)
	b.addLicence(dist, res.License, res.Rights)
}

// datasetIRI is the portal's page for the dataset, which is what the portal
// publishes as its identifier.
func (s *Source) datasetIRI(pkg ckanPackage) string {
	slug := firstNonEmpty(strings.TrimSpace(pkg.Name), strings.TrimSpace(pkg.ID))
	if slug == "" {
		return ""
	}
	return s.datasetsAt + "/" + url.PathEscape(slug)
}

// distributionIRI names a resource. CKAN resource IDs are stable, so they are
// preferred; a resource carrying neither a URI nor an ID is skolemized from its
// position and content so that re-runs reproduce the same IRI (R-CAT-3).
func (s *Source) distributionIRI(datasetIRI string, index int, res ckanResource) string {
	if isHTTPURL(res.URI) {
		return res.URI
	}
	if id := strings.TrimSpace(res.ID); id != "" {
		return datasetIRI + "/resource/" + url.PathEscape(id)
	}
	key := strconv.Itoa(index) + "\x00" + res.URL + "\x00" + res.Format
	return s.minter.Skolem(datasetIRI, "distribution", key).Value
}

// portalIRI builds a page IRI on the portal itself, e.g. .../organization/bfs.
func (s *Source) portalIRI(kind, name string) string {
	root := strings.TrimSuffix(s.datasetsAt, "/dataset")
	return root + "/" + kind + "/" + url.PathEscape(name)
}

// quadBuilder accumulates statements without a graph; the runner assigns one.
type quadBuilder struct {
	quads []rdf.Quad
}

func (b *quadBuilder) add(s, p, o rdf.Term) {
	q := rdf.NewQuad(s, p, o, "")
	if q.Valid() {
		b.quads = append(b.quads, q)
	}
}

func (b *quadBuilder) addText(s, p rdf.Term, v multiString) {
	if v.Empty() {
		return
	}
	for _, t := range v.Terms() {
		b.add(s, p, t)
	}
}

// addTime writes the first candidate that parses, normalized to xsd:dateTime.
func (b *quadBuilder) addTime(s, p rdf.Term, candidates ...string) {
	for _, c := range candidates {
		if t, ok := source.ParseTime(c); ok {
			b.add(s, p, rdf.TypedLiteral(t.Format(time.RFC3339), rdf.Xsd("dateTime")))
			return
		}
	}
}

// addLicence writes a licence as an IRI when it is one and a literal otherwise,
// preferring the first candidate that carries anything.
func (b *quadBuilder) addLicence(s rdf.Term, candidates ...string) {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if isHTTPURL(c) {
			b.add(s, rdf.DctLicense, rdf.IRI(c))
		} else {
			b.add(s, rdf.DctLicense, rdf.Literal(c))
		}
		return
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstSet(vals ...flexInt) flexInt {
	for _, v := range vals {
		if v.Set {
			return v
		}
	}
	return flexInt{}
}

// isHTTPURL reports whether s is an absolute http(s) URL. CKAN URL fields hold
// relative paths and free text often enough that this has to be checked before
// anything becomes an IRI.
func isHTTPURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
