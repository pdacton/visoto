package app

import (
	"strings"
	"testing"

	"github.com/pdacton/visoto/internal/config"
	"github.com/pdacton/visoto/internal/icon"
)

// TestExampleConfigCoversIconFolders: every namespace folder under
// static/img/resource must name a prefix in visoto.config.example, because
// icon.Init fails startup on an undeclared one — a fresh install from the
// example would otherwise exit before serving anything.
func TestExampleConfigCoversIconFolders(t *testing.T) {
	cfg, err := config.Load("../visoto.config.example")
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}
	namespaces := make(map[string]string, len(cfg.RDF.ParsedPrefixes))
	for _, p := range cfg.RDF.ParsedPrefixes {
		namespaces[p.Name] = strings.Trim(p.URI, "<>")
	}
	if err := icon.Init("../static/img/resource", namespaces, nil); err != nil {
		t.Fatalf("visoto.config.example is missing an icon-folder prefix: %v", err)
	}
}
