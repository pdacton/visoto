package search

import "hutzli.org/visoto/internal/sparql"

// SearchParams represents user input from the search form
type SearchParams struct {
	Query    string // Lucene syntax search query
	Class    string // Optional RDF class filter (IRI)
	Property string // Optional property filter (IRI)
	Limit    int    // Results limit (default 20)
}

// SearchResult wraps SPARQL query results for search
type SearchResult struct {
	Query        string             // Original search query
	Results      sparql.QueryResult // SPARQL results (Vars, Bindings, Error)
	Provider     string             // Provider used (stardog, graphdb, fuseki, sparql-query)
	FallbackUsed bool               // true when sparql-query fallback was used instead of native FTS
}

// Filter represents a class or property filter option
type Filter struct {
	Label string // Display label for UI
	IRI   string // Full IRI (empty string for "Any")
}
