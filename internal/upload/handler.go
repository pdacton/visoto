package upload

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/export"
	"hutzli.org/visoto/internal/logger"
)

// mimeForExtension returns the RDF MIME type for a file extension.
// Returns empty string if the extension is unknown.
func mimeForExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".ttl":
		return "text/turtle"
	case ".nq":
		return "application/n-quads"
	case ".nt":
		return "application/n-triples"
	case ".rdf", ".owl", ".xml":
		return "application/rdf+xml"
	case ".jsonld":
		return "application/ld+json"
	case ".trig":
		return "application/trig"
	default:
		return ""
	}
}

// mimeForURL detects an RDF MIME type from the file extension in a URL path.
// Returns empty string if undetectable.
func mimeForURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := path.Ext(u.Path)
	return mimeForExtension(ext)
}

// sendToEndpoint POSTs rdfBody to the Graph Store HTTP Protocol endpoint.
// graphURI is the target named graph; contentType must be a valid RDF MIME type.
// Auth priority: Bearer access_token > Basic username/password > none.
func sendToEndpoint(ep *config.SparqlEndpoint, graphURI, contentType string, rdfBody io.Reader) error {
	log := logger.Get()

	targetURL := ep.URL
	if graphURI != "" {
		u, err := url.Parse(ep.URL)
		if err != nil {
			return fmt.Errorf("invalid endpoint URL: %w", err)
		}
		q := u.Query()
		q.Set("graph", graphURI)
		u.RawQuery = q.Encode()
		targetURL = u.String()
	}

	req, err := http.NewRequest(http.MethodPost, targetURL, rdfBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	switch {
	case ep.AccessToken != "":
		req.Header.Set("Authorization", "Bearer "+ep.AccessToken)
	case ep.Username != "":
		req.SetBasicAuth(ep.Username, ep.Password)
	}

	log.Debug("uploading RDF to endpoint",
		slog.String("url", targetURL),
		slog.String("contentType", contentType))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// UploadHandler handles POST /api/upload.
// Accepts multipart/form-data with:
//   - file   (mutually exclusive with url) — the RDF file to upload
//   - url    (mutually exclusive with file) — a remote RDF URL the server will fetch
//   - graphURI — target named graph URI
//   - endpoint — name of the SPARQL endpoint (matches visoto.config entry)
func UploadHandler(cfg *config.ApplicationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()

		endpointName := c.PostForm("endpoint")
		graphURI := strings.TrimSpace(c.PostForm("graphURI"))
		remoteURL := strings.TrimSpace(c.PostForm("url"))

		ep := cfg.GetEndpointByName(endpointName)
		if ep == nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "no SPARQL endpoint configured"})
			return
		}

		var (
			rdfBody     io.Reader
			contentType string
		)

		if remoteURL != "" {
			// --- URL mode: fetch remote RDF server-side ---
			log.Debug("fetching remote RDF URL", slog.String("url", remoteURL))
			resp, err := http.Get(remoteURL) //nolint:gosec // URL is user-supplied but only fetched server-side
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("failed to fetch URL: %v", err)})
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("remote URL returned %s", resp.Status)})
				return
			}

			// Prefer Content-Type from remote; fall back to extension detection
			contentType = resp.Header.Get("Content-Type")
			if idx := strings.Index(contentType, ";"); idx != -1 {
				contentType = strings.TrimSpace(contentType[:idx])
			}
			if contentType == "" || contentType == "application/octet-stream" || contentType == "text/plain" {
				if detected := mimeForURL(remoteURL); detected != "" {
					contentType = detected
				}
			}
			if contentType == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "could not determine RDF content type from URL"})
				return
			}
			rdfBody = resp.Body

		} else {
			// --- File mode: read uploaded file ---
			fh, err := c.FormFile("file")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "no file or URL provided"})
				return
			}
			ext := path.Ext(fh.Filename)
			contentType = mimeForExtension(ext)
			if contentType == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fmt.Sprintf("unsupported file extension: %s", ext)})
				return
			}
			f, err := fh.Open()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to read uploaded file"})
				return
			}
			defer f.Close()
			rdfBody = f
		}

		if err := sendToEndpoint(ep, graphURI, contentType, rdfBody); err != nil {
			log.Error("RDF upload failed", slog.String("error", err.Error()), slog.String("endpoint", ep.URL))
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
			return
		}

		log.Info("RDF upload succeeded",
			slog.String("endpoint", ep.URL),
			slog.String("graph", graphURI))
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// OntologiesHandler handles GET /api/ontologies.
// Returns the list of well-known ontologies configured in visoto.config.
func OntologiesHandler(cfg *config.ApplicationConfig, ontologies []config.OntologyEntry) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = cfg // reserved for future per-endpoint filtering
		if ontologies == nil {
			ontologies = []config.OntologyEntry{}
		}
		c.JSON(http.StatusOK, gin.H{"ontologies": ontologies})
	}
}

// NamedGraph holds an IRI, optional label, and triple count for a named graph.
type NamedGraph struct {
	IRI         string `json:"iri"`
	Label       string `json:"label"`
	TripleCount int    `json:"tripleCount"`
}

// sparqlQuery sends a SPARQL query to the endpoint and returns the parsed bindings.
func sparqlQuery(ep *config.SparqlEndpoint, query string) ([]map[string]struct {
	Value string `json:"value"`
}, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("format", "application/sparql-results+json")

	req, err := http.NewRequest(http.MethodPost, ep.URL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	switch {
	case ep.AccessToken != "":
		req.Header.Set("Authorization", "Bearer "+ep.AccessToken)
	case ep.Username != "":
		req.SetBasicAuth(ep.Username, ep.Password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Results.Bindings, nil
}

// NamedGraphsHandler handles GET /api/named-graphs?endpoint=<name>.
// Runs a SPARQL query for all named graphs with optional rdfs:label and returns the list.
func NamedGraphsHandler(cfg *config.ApplicationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		endpointName := c.Query("endpoint")
		ep := cfg.GetEndpointByName(endpointName)
		if ep == nil {
			c.JSON(http.StatusOK, gin.H{"graphs": []NamedGraph{}})
			return
		}

		query := `SELECT ?g (SAMPLE(?lbl) AS ?label) (COUNT(*) AS ?count) WHERE {
  GRAPH ?g { ?s ?p ?o }
  OPTIONAL { ?g <http://www.w3.org/2000/01/rdf-schema#label> ?lbl }
} GROUP BY ?g ORDER BY ?g LIMIT 1000`

		bindings, err := sparqlQuery(ep, query)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"graphs": []NamedGraph{}})
			return
		}

		graphs := make([]NamedGraph, 0, len(bindings))
		for _, b := range bindings {
			g, ok := b["g"]
			if !ok {
				continue
			}
			label := ""
			if lbl, ok := b["label"]; ok {
				label = lbl.Value
			}
			count := 0
			if cnt, ok := b["count"]; ok {
				fmt.Sscanf(cnt.Value, "%d", &count)
			}
			graphs = append(graphs, NamedGraph{IRI: g.Value, Label: label, TripleCount: count})
		}
		c.JSON(http.StatusOK, gin.H{"graphs": graphs})
	}
}

// DeleteNamedGraphHandler handles DELETE /api/named-graphs?graph=<uri>&endpoint=<name>.
// Pass graph=default to delete the default graph.
// Pass any other graph URI to delete that named graph.
// Uses SPARQL Update (DROP SILENT) for broad endpoint compatibility.
func DeleteNamedGraphHandler(cfg *config.ApplicationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()

		endpointName := c.Query("endpoint")
		graphURI := strings.TrimSpace(c.Query("graph"))

		ep := cfg.GetEndpointByName(endpointName)
		if ep == nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "no SPARQL endpoint configured"})
			return
		}
		if graphURI == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "graph parameter required"})
			return
		}

		var updateQuery string
		if graphURI == "default" {
			updateQuery = "DROP SILENT DEFAULT"
		} else {
			updateQuery = fmt.Sprintf("DROP SILENT GRAPH <%s>", graphURI)
		}

		params := url.Values{}
		params.Set("update", updateQuery)

		req, err := http.NewRequest(http.MethodPost, ep.URL, strings.NewReader(params.Encode()))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to create request"})
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		switch {
		case ep.AccessToken != "":
			req.Header.Set("Authorization", "Bearer "+ep.AccessToken)
		case ep.Username != "":
			req.SetBasicAuth(ep.Username, ep.Password)
		}

		log.Debug("deleting named graph via SPARQL Update", slog.String("query", updateQuery), slog.String("endpoint", ep.URL))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": fmt.Sprintf("endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(body)))})
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)

		log.Info("named graph deleted", slog.String("graph", graphURI), slog.String("endpoint", ep.URL))
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// isQuadFormat reports whether the MIME type natively encodes named graph membership.
// Quad formats (N-Quads, TriG) can represent multiple named graphs in a single file.
func isQuadFormat(mime string) bool {
	return mime == "application/n-quads" || mime == "application/trig"
}

// iriToFilename converts a graph IRI to a safe filename with the given extension.
// e.g. "https://opendata.swiss/id/catalogue/vocabularies" + ".ttl"
//
//	→ "opendata.swiss-id-catalogue-vocabularies.ttl"
func iriToFilename(iri, ext string) string {
	s := iri
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	var b strings.Builder
	prev := '-'
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' {
			b.WriteRune(r)
			prev = r
		} else if prev != '-' {
			b.WriteRune('-')
			prev = '-'
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		result = "graph"
	}
	return result + ext
}

// ExportNamedGraphsHandler handles GET /api/export-graphs.
// Query params:
//   - endpoint: SPARQL endpoint name (matches visoto.config entry)
//   - graph:    one or more named graph IRIs (repeatable: ?graph=iri1&graph=iri2)
//   - format:   RDF MIME type (default: text/turtle)
//
// Single graph or quad format → streams RDF file directly.
// Multiple graphs + non-quad format → streams a ZIP archive, one file per graph.
// Tries export providers in order: graphdb → gsp → construct.
func ExportNamedGraphsHandler(cfg *config.ApplicationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()

		endpointName := c.Query("endpoint")
		graphIRIs := c.QueryArray("graph")
		format := c.DefaultQuery("format", "text/turtle")

		ep := cfg.GetEndpointByName(endpointName)
		if ep == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no SPARQL endpoint configured"})
			return
		}
		if len(graphIRIs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one graph parameter required"})
			return
		}

		ext := export.ExtForMIME(format)
		useZip := len(graphIRIs) > 1 && !isQuadFormat(format)

		if useZip {
			c.Header("Content-Type", "application/zip")
			c.Header("Content-Disposition", `attachment; filename="export.zip"`)
			c.Status(http.StatusOK)

			zw := zip.NewWriter(c.Writer)
			defer zw.Close()

			for _, iri := range graphIRIs {
				params := export.ExportParams{GraphIRIs: []string{iri}, Format: format, Endpoint: ep}
				rc, providerName, err := export.ExportWithFallback(params)
				if err != nil {
					log.Error("export failed for graph", slog.String("graph", iri), slog.String("error", err.Error()))
					continue // skip failed graphs; ZIP may be partial
				}
				entryName := iriToFilename(iri, ext)
				w, err := zw.Create(entryName)
				if err != nil {
					rc.Close()
					log.Error("zip entry creation failed", slog.String("entry", entryName), slog.String("error", err.Error()))
					continue
				}
				_, _ = io.Copy(w, rc)
				rc.Close()
				log.Info("export graph to zip", slog.String("provider", providerName), slog.String("graph", iri))
			}
		} else {
			params := export.ExportParams{GraphIRIs: graphIRIs, Format: format, Endpoint: ep}
			rc, providerName, err := export.ExportWithFallback(params)
			if err != nil {
				log.Error("export failed", slog.String("endpoint", ep.URL), slog.String("error", err.Error()))
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
				return
			}
			defer rc.Close()

			log.Info("export succeeded",
				slog.String("provider", providerName),
				slog.String("endpoint", ep.URL),
				slog.Int("graphs", len(graphIRIs)))

			filename := "export" + ext
			c.Header("Content-Type", format)
			c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
			c.Status(http.StatusOK)
			_, _ = io.Copy(c.Writer, rc)
		}
	}
}
