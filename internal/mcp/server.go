package mcp

import (
	"net/http"

	goMcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/sparql"
)

// NewServer creates and returns a configured MCP HTTP handler.
// The returned mux handles POST /mcp (MCP streamable HTTP) and GET /health.
func NewServer(cfg *config.Config, preprocessor *sparql.Preprocessor) http.Handler {
	tc := &toolContext{
		preprocessor: preprocessor,
		cfg:          cfg,
	}

	mcpServer := server.NewMCPServer(
		"Visoto SPARQL MCP",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// list_endpoints — no parameters
	mcpServer.AddTool(
		goMcp.NewTool("list_endpoints",
			goMcp.WithDescription("List all configured SPARQL triple store endpoints with their names and URLs."),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleListEndpoints,
	)

	// list_prefixes — no parameters
	mcpServer.AddTool(
		goMcp.NewTool("list_prefixes",
			goMcp.WithDescription("List all configured RDF prefix declarations (e.g. schema:, rdf:, dct:)."),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleListPrefixes,
	)

	// check_endpoint
	mcpServer.AddTool(
		goMcp.NewTool("check_endpoint",
			goMcp.WithDescription("Check if a SPARQL endpoint is reachable by running ASK {}. Returns status and latency."),
			goMcp.WithString("endpoint",
				goMcp.Description("Endpoint name (e.g. 'LINDAS prod') or full URL. Uses the default endpoint if omitted."),
			),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleCheckEndpoint,
	)

	// run_sparql_query
	mcpServer.AddTool(
		goMcp.NewTool("run_sparql_query",
			goMcp.WithDescription(
				"Execute a SPARQL SELECT query against a configured endpoint. "+
					"Configured RDF prefixes (rdf:, schema:, dct:, etc.) are injected automatically — no need to declare them. "+
					"Returns results as JSON rows. On failure, returns helpful hints.",
			),
			goMcp.WithString("query",
				goMcp.Description("SPARQL SELECT query text. Prefixes are injected automatically."),
				goMcp.Required(),
			),
			goMcp.WithString("endpoint",
				goMcp.Description("Endpoint name (e.g. 'LINDAS prod') or full URL. Uses the default endpoint if omitted."),
			),
			goMcp.WithBoolean("resolve_labels",
				goMcp.Description("If true, resolve IRIs to human-readable labels. Slower but more readable. Default: false."),
			),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleRunSPARQLQuery,
	)

	// discover_classes
	mcpServer.AddTool(
		goMcp.NewTool("discover_classes",
			goMcp.WithDescription("Discover the most common RDF types/classes in an endpoint, ordered by instance count."),
			goMcp.WithString("endpoint",
				goMcp.Description("Endpoint name or URL. Uses the default endpoint if omitted."),
			),
			goMcp.WithNumber("limit",
				goMcp.Description("Maximum number of classes to return. Default: 100."),
				goMcp.DefaultNumber(100),
				goMcp.Min(1),
				goMcp.Max(1000),
			),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleDiscoverClasses,
	)

	// discover_properties
	mcpServer.AddTool(
		goMcp.NewTool("discover_properties",
			goMcp.WithDescription("Discover the most frequently used RDF predicates/properties in an endpoint."),
			goMcp.WithString("endpoint",
				goMcp.Description("Endpoint name or URL. Uses the default endpoint if omitted."),
			),
			goMcp.WithNumber("limit",
				goMcp.Description("Maximum number of properties to return. Default: 100."),
				goMcp.DefaultNumber(100),
				goMcp.Min(1),
				goMcp.Max(1000),
			),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleDiscoverProperties,
	)

	// get_resource
	mcpServer.AddTool(
		goMcp.NewTool("get_resource",
			goMcp.WithDescription("Retrieve all RDF triples (predicate–object pairs) for a given resource IRI."),
			goMcp.WithString("iri",
				goMcp.Description("The full IRI of the resource to retrieve, e.g. https://ld.admin.ch/municipality/351."),
				goMcp.Required(),
			),
			goMcp.WithString("endpoint",
				goMcp.Description("Endpoint name or URL. Uses the default endpoint if omitted."),
			),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleGetResource,
	)

	// search_by_label
	mcpServer.AddTool(
		goMcp.NewTool("search_by_label",
			goMcp.WithDescription(
				"Search for RDF resources by label text using rdfs:label and skos:prefLabel. "+
					"Case-insensitive substring match.",
			),
			goMcp.WithString("text",
				goMcp.Description("Search string to find in resource labels."),
				goMcp.Required(),
			),
			goMcp.WithString("type_filter",
				goMcp.Description("Optional RDF type IRI to filter results, e.g. https://schema.org/Person."),
			),
			goMcp.WithString("endpoint",
				goMcp.Description("Endpoint name or URL. Uses the default endpoint if omitted."),
			),
			goMcp.WithNumber("limit",
				goMcp.Description("Maximum number of results. Default: 50."),
				goMcp.DefaultNumber(50),
				goMcp.Min(1),
				goMcp.Max(500),
			),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleSearchByLabel,
	)

	// count_instances
	mcpServer.AddTool(
		goMcp.NewTool("count_instances",
			goMcp.WithDescription(
				"Count RDF instances per class in the endpoint. "+
					"Provide class_iri to count only instances of that specific class.",
			),
			goMcp.WithString("class_iri",
				goMcp.Description("Optional RDF class IRI to count instances for, e.g. https://schema.org/Person. "+
					"If omitted, counts instances for all classes."),
			),
			goMcp.WithString("endpoint",
				goMcp.Description("Endpoint name or URL. Uses the default endpoint if omitted."),
			),
			goMcp.WithReadOnlyHintAnnotation(true),
			goMcp.WithDestructiveHintAnnotation(false),
		),
		tc.handleCountInstances,
	)

	// Build the streamable HTTP transport (POST /mcp)
	httpServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithStateLess(true),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", httpServer)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}
