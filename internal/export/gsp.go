package export

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"hutzli.org/visoto/internal/config"
)

// GSPProvider exports named graphs using the Graph Store Protocol (GSP).
// Endpoint: GET {endpoint.URL}?graph={iri} with Accept: {format}
// Returns ErrNotApplicable on non-2xx so the fallback chain advances to CONSTRUCT.
type GSPProvider struct{}

func (g *GSPProvider) Name() string { return "gsp" }

func (g *GSPProvider) Export(params ExportParams) (io.ReadCloser, error) {
	if len(params.GraphIRIs) == 0 {
		return nil, fmt.Errorf("at least one graph IRI required")
	}

	readers := make([]io.Reader, 0, len(params.GraphIRIs))
	closers := make([]io.Closer, 0, len(params.GraphIRIs))

	for _, iri := range params.GraphIRIs {
		rc, err := gspFetch(params.Endpoint, iri, params.Format)
		if err != nil {
			for _, c := range closers {
				c.Close()
			}
			return nil, err // may be ErrNotApplicable — caller checks
		}
		readers = append(readers, rc)
		closers = append(closers, rc)
	}

	return &multiReadCloser{r: io.MultiReader(readers...), closers: closers}, nil
}

// gspFetch fetches a single named graph via GSP GET.
// Returns ErrNotApplicable on network errors or non-2xx responses so the
// fallback chain can advance to the CONSTRUCT provider.
func gspFetch(ep *config.SparqlEndpoint, iri, format string) (io.ReadCloser, error) {
	u, err := url.Parse(ep.URL)
	if err != nil {
		return nil, ErrNotApplicable
	}
	q := u.Query()
	q.Set("graph", iri)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrNotApplicable
	}
	req.Header.Set("Accept", format)
	applyAuth(req, ep)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, ErrNotApplicable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, ErrNotApplicable // let CONSTRUCT try
	}
	return resp.Body, nil
}

func init() { RegisterProvider(&GSPProvider{}) }
