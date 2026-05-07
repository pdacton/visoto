package export

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"hutzli.org/visoto/internal/config"
)

// GraphDBProvider exports named graphs using the GraphDB REST API.
// Endpoint: GET {repoBase}/statements?context=<iri>&infer=false
// Returns ErrNotApplicable if the endpoint URL does not match the GraphDB path pattern.
type GraphDBProvider struct{}

func (g *GraphDBProvider) Name() string { return "graphdb" }

func (g *GraphDBProvider) Export(params ExportParams) (io.ReadCloser, error) {
	repoBase, err := graphdbRepoBase(params.Endpoint.URL)
	if err != nil {
		return nil, ErrNotApplicable
	}
	if len(params.GraphIRIs) == 0 {
		return nil, fmt.Errorf("at least one graph IRI required")
	}

	readers := make([]io.Reader, 0, len(params.GraphIRIs))
	closers := make([]io.Closer, 0, len(params.GraphIRIs))

	for _, iri := range params.GraphIRIs {
		rc, err := graphdbFetch(repoBase, iri, params.Format, params.Endpoint)
		if err != nil {
			for _, c := range closers {
				c.Close()
			}
			return nil, fmt.Errorf("GraphDB export failed for %s: %w", iri, err)
		}
		readers = append(readers, rc)
		closers = append(closers, rc)
	}

	return &multiReadCloser{r: io.MultiReader(readers...), closers: closers}, nil
}

// graphdbRepoBase derives the repository base URL from a GraphDB SPARQL endpoint URL.
// Strips the trailing /sparql suffix and verifies /repositories/ is in the path.
// Returns an error if the URL doesn't match the GraphDB pattern.
func graphdbRepoBase(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	p := strings.TrimSuffix(u.Path, "/sparql")
	if !strings.Contains(p, "/repositories/") {
		return "", errors.New("not a GraphDB endpoint URL")
	}
	u.Path = p
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// graphdbFetch fetches a single named graph from the GraphDB REST statements endpoint.
func graphdbFetch(repoBase, iri, format string, ep *config.SparqlEndpoint) (io.ReadCloser, error) {
	u, err := url.Parse(repoBase + "/statements")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("context", "<"+iri+">")
	q.Set("infer", "false")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", format)
	applyAuth(req, ep)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("GraphDB returned HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func init() { RegisterProvider(&GraphDBProvider{}) }
