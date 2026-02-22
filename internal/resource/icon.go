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
	icons         map[string]bool
	fallbackIcons map[string]bool
	mu            sync.RWMutex
}

var (
	globalIconCache *iconCache
	once            sync.Once
)

// InitIconCache scans the icon directory and builds the cache
func InitIconCache(iconDir string) error {
	once.Do(func() {
		globalIconCache = &iconCache{
			icons:         make(map[string]bool),
			fallbackIcons: make(map[string]bool),
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
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if baseName, ok := strings.CutSuffix(name, ".fallback.svg"); ok {
			// Fallback icons: stored by bare class name (e.g. "DefinedTermSet")
			globalIconCache.fallbackIcons[baseName] = true
		} else if baseName, ok := strings.CutSuffix(name, ".svg"); ok {
			// Regular icons: stored without .svg extension
			globalIconCache.icons[baseName] = true
		}
	}

	log := logger.Get()
	log.Info("icon cache initialized",
		slog.Int("count", len(globalIconCache.icons)),
		slog.Int("fallback_count", len(globalIconCache.fallbackIcons)))

	return nil
}

// GetIconNames returns all available icon names for use in JS templates.
// Regular icons are keyed by bare class name (e.g. "Canton").
// Fallback icons are keyed with the ".fallback" suffix (e.g. "DefinedTermSet.fallback")
// so that callers can construct the correct file path by appending ".svg".
func GetIconNames() map[string]bool {
	if globalIconCache == nil {
		return map[string]bool{}
	}
	globalIconCache.mu.RLock()
	defer globalIconCache.mu.RUnlock()
	result := make(map[string]bool, len(globalIconCache.icons)+len(globalIconCache.fallbackIcons))
	for k, v := range globalIconCache.icons {
		result[k] = v
	}
	for k := range globalIconCache.fallbackIcons {
		result[k+".fallback"] = true
	}
	return result
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

// hasFallbackIcon checks if a fallback icon with the given name exists
func hasFallbackIcon(name string) bool {
	if globalIconCache == nil {
		return false
	}
	globalIconCache.mu.RLock()
	defer globalIconCache.mu.RUnlock()
	return globalIconCache.fallbackIcons[name]
}

// extractResourceNameFromIRI extracts the class name from a full URI
// Examples:
//
//	https://schema.ld.admin.ch/Canton -> Canton
//	http://www.w3.org/2004/02/skos/core#ConceptScheme -> ConceptScheme
func extractResourceNameFromIRI(uri string) string {
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

	// Priority 1: exact match
	// e.g. schema:Municipality gets Municipalty.svg
	resourceName := extractResourceNameFromIRI(td.ResourceIRI)
	if hasIcon(resourceName) {
		return basePath + resourceName + ".svg"
	}
	if hasFallbackIcon(resourceName) {
		return basePath + resourceName + ".fallback.svg"
	}

	// Priority 2: class match
	// e.g. the municipality "Bern" with class schema:Municipality gets Municipality.svg
	if pageClasses, ok := td.QueryResults["pageClasses"]; ok {
		for _, binding := range pageClasses.Bindings {
			if classBinding, ok := binding["class"]; ok {
				className := extractResourceNameFromIRI(classBinding.Value)
				if hasIcon(className) {
					return basePath + className + ".svg"
				}
			}
		}
		// lower prio icons:
		// If no class match, check for fallback icons like Version.fallback.svg or DefinedTerm.fallback.svg
		// this is necessary because in LINDAS some resource have many different classes
		// and we want to show the more specific icon
		for _, binding := range pageClasses.Bindings {
			if classBinding, ok := binding["class"]; ok {
				className := extractResourceNameFromIRI(classBinding.Value)
				if hasFallbackIcon(className) {
					return basePath + className + ".fallback.svg"
				}
			}
		}
	}

	// Priority 3: Default to default.svg
	return basePath + "default.svg"
}
