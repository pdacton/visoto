package export

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"hutzli.org/visoto/internal/config"
)

// ConstructProvider exports named graphs using SPARQL CONSTRUCT queries.
// Query: CONSTRUCT { ?s ?p ?o } WHERE { GRAPH <iri> { ?s ?p ?o } }
// This is the final fallback and never returns ErrNotApplicable.
type ConstructProvider struct{}

func (c *ConstructProvider) Name() string { return "construct" }

func (c *ConstructProvider) Export(params ExportParams) (io.ReadCloser, error) {
	if len(params.GraphIRIs) == 0 {
		return nil, fmt.Errorf("at least one graph IRI required")
	}

	readers := make([]io.Reader, 0, len(params.GraphIRIs))
	closers := make([]io.Closer, 0, len(params.GraphIRIs))

	for _, iri := range params.GraphIRIs {
		rc, err := constructFetch(params.Endpoint, iri, params.Format)
		if err != nil {
			for _, cl := range closers {
				cl.Close()
			}
			return nil, fmt.Errorf("CONSTRUCT export failed for %s: %w", iri, err)
		}
		readers = append(readers, rc)
		closers = append(closers, rc)
	}

	return &multiReadCloser{r: io.MultiReader(readers...), closers: closers}, nil
}

// constructFetch issues a SPARQL CONSTRUCT query for a single named graph.
func constructFetch(ep *config.SparqlEndpoint, iri, format string) (io.ReadCloser, error) {
	query := fmt.Sprintf("CONSTRUCT { ?s ?p ?o } WHERE { GRAPH <%s> { ?s ?p ?o } }", iri)
	form := url.Values{}
	form.Set("query", query)

	req, err := http.NewRequest(http.MethodPost, ep.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", format)
	applyAuth(req, ep)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("SPARQL endpoint returned HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func init() { RegisterProvider(&ConstructProvider{}) }
