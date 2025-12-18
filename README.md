## Visoto

Visoto is a Go web application for visualizing and exploring RDF resources using SPARQL queries. It is built with the Gin framework and uses Go templates for rendering dynamic HTML pages. The project supports modern UI features, including dark mode, and integrates Tabler and Tabulator for responsive layouts and interactive data tables.

Key features:
- Query RDF resources via SPARQL endpoints (e.g., DBpedia)
- Visualize resource data in a user-friendly web interface
- Modern frontend with Tabler (Bootstrap 5-based) and Tabulator
- Dark mode toggle and accessible design
- Organized project structure for templates and static assets

To run the project:
1. Install dependencies: `go get github.com/gin-gonic/gin`
2. Start the server: `go run ./cmd/visoto`
3. Access the app at http://localhost:8080




For a connectivity test, call http://localhost:8080/ping


Todo:
- make SPARQL endpoint configureable (currently hardcoded for Lindas)
