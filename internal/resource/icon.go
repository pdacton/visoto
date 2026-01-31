package resource

import (
	"log/slog"
	"os"
	"strings"
	"sync"

	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/sparql"
)

// iconCache holds the set of available icon names (without .svg extension)
type iconCache struct {
	icons map[string]bool
	mu    sync.RWMutex
}

var (
	globalIconCache *iconCache
	once            sync.Once
)

// InitIconCache scans the icon directory and builds the cache
func InitIconCache(iconDir string) error {
	once.Do(func() {
		globalIconCache = &iconCache{
			icons: make(map[string]bool),
		}
	})

	// Scan directory for .svg files
	entries, err := os.ReadDir(iconDir)
	if err != nil {
		return err
	}

	globalIconCache.mu.Lock()
	defer globalIconCache.mu.Unlock()

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".svg") {
			// Store name without .svg extension
			name := strings.TrimSuffix(entry.Name(), ".svg")
			globalIconCache.icons[name] = true
		}
	}

	log := logger.Get()
	log.Info("icon cache initialized", slog.Int("count", len(globalIconCache.icons)))

	return nil
}

// hasIcon checks if an icon with the given name exists
func hasIcon(name string) bool {
	if globalIconCache == nil {
		return false
	}
	globalIconCache.mu.RLock()
	defer globalIconCache.mu.RUnlock()
	return globalIconCache.icons[name]
}

// extractClassName extracts the class name from a full URI
// Examples:
//   https://schema.ld.admin.ch/Canton -> Canton
//   http://www.w3.org/2004/02/skos/core#ConceptScheme -> ConceptScheme
func extractClassName(uri string) string {
	// Check for fragment first (the part after #)
	if idx := strings.LastIndex(uri, "#"); idx != -1 {
		return uri[idx+1:]
	}

	// Extract last segment after final /
	if idx := strings.LastIndex(uri, "/"); idx != -1 {
		return uri[idx+1:]
	}

	return uri
}

// GetIconForResource determines the appropriate icon path for a resource
// Accepts either sparql.TemplateData (resource pages) or other types (search, home, etc.)
// Priority for resource pages:
// 1. Resource IRI match (for instance-specific icons)
// 2. First matching RDF class from pageClasses bindings
// 3. Default to "default.svg"
// Returns the full path including "/static/img/resource/", or empty string for non-resource pages
func GetIconForResource(data interface{}) string {
	const basePath = "/static/img/resource/"

	// Check if it's TemplateData (resource pages)
	td, ok := data.(sparql.TemplateData)
	if !ok {
		// For other types (search, home, etc.) return empty string (no icon)
		return ""
	}

	// Priority 1: Check for exact resource IRI match
	resourceName := extractClassName(td.ResourceIRI)
	if hasIcon(resourceName) {
		return basePath + resourceName + ".svg"
	}

	// Priority 2: Check each class in order from pageClasses bindings
	if pageClasses, ok := td.QueryResults["pageClasses"]; ok {
		for _, binding := range pageClasses.Bindings {
			if classBinding, ok := binding["class"]; ok {
				className := extractClassName(classBinding.Value)
				if hasIcon(className) {
					return basePath + className + ".svg"
				}
			}
		}
	}

	// Priority 3: Default to default.svg
	return basePath + "default.svg"
}
