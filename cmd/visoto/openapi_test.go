package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"hutzli.org/visoto/internal/sparqlproxy"
)

// openAPISpec is the hand-written API description served at /apiDocs.html.
const openAPISpec = "../../static/openapi.yaml"

// documentedRoute reports whether a registered path belongs in the OpenAPI
// spec: everything under /api plus the operational routes. HTML pages do not.
func documentedRoute(path string) bool {
	switch path {
	case "/metrics", "/mcp", "/health", "/ping":
		return true
	}
	return strings.HasPrefix(path, "/api/")
}

var (
	routeCall   = regexp.MustCompile(`router\.(GET|POST|PUT|PATCH|DELETE|Any)\(("[^"]+"|sparqlproxy\.Path)`)
	ginParam    = regexp.MustCompile(`:([A-Za-z_]+)`)
	specPath    = regexp.MustCompile(`(?m)^  (/\S*):\s*$`)
	specMethod  = regexp.MustCompile(`(?m)^    (get|post|put|patch|delete):\s*$`)
	anyMethodOK = map[string]bool{"get": true, "post": true}
)

// registeredRoutes reads the route table from main.go: "METHOD /path" in
// OpenAPI's path syntax. Any expands to the methods MCP clients use.
func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, m := range routeCall.FindAllStringSubmatch(string(src), -1) {
		path := strings.Trim(m[2], `"`)
		if m[2] == "sparqlproxy.Path" {
			path = sparqlproxy.Path
		}
		if !documentedRoute(path) {
			continue
		}
		path = ginParam.ReplaceAllString(path, "{$1}")
		if m[1] == "Any" {
			for method := range anyMethodOK {
				out[method+" "+path] = true
			}
			continue
		}
		out[strings.ToLower(m[1])+" "+path] = true
	}
	return out
}

// specRoutes reads "method /path" for every operation in the spec. The spec is
// formatted with paths at two spaces and methods at four, which this relies on.
func specRoutes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(openAPISpec)
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "\npaths:\n")
	end := strings.Index(body, "\ncomponents:\n")
	if start < 0 || end < start {
		t.Fatal("spec has no paths: … components: section")
	}
	paths := body[start:end]

	out := map[string]bool{}
	locs := specPath.FindAllStringSubmatchIndex(paths, -1)
	for i, loc := range locs {
		blockEnd := len(paths)
		if i+1 < len(locs) {
			blockEnd = locs[i+1][0]
		}
		path := paths[loc[2]:loc[3]]
		for _, m := range specMethod.FindAllStringSubmatch(paths[loc[1]:blockEnd], -1) {
			out[m[1]+" "+path] = true
		}
	}
	return out
}

func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// TestOpenAPICoversRoutes keeps static/openapi.yaml and the route table in
// step: every API route is documented, and every documented operation exists.
func TestOpenAPICoversRoutes(t *testing.T) {
	routes := registeredRoutes(t)
	spec := specRoutes(t)
	if len(routes) == 0 || len(spec) == 0 {
		t.Fatalf("parsed %d routes and %d spec operations; the parsers are broken", len(routes), len(spec))
	}
	for _, r := range diff(routes, spec) {
		t.Errorf("route %s is not documented in static/openapi.yaml", r)
	}
	for _, r := range diff(spec, routes) {
		t.Errorf("static/openapi.yaml documents %s, which main.go does not register", r)
	}
}
