// load application config file from TOML file into Config struct

package config

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"hutzli.org/visoto/internal/logger"
)

// TODO: add validation funtions for config fields (e.g. port range, valid URIs, etc.)
// TODO: add constructor function for Config with default values

// Config represents the complete application configuration
type Config struct {
	Application ApplicationConfig `toml:"application"`
	RDF         RDFConfig         `toml:"rdf"`
	Logging     LoggingConfig     `toml:"logging"`
}

// ApplicationConfig holds application-level settings
type ApplicationConfig struct {
	Port            int               `toml:"port"`
	SparqlEndpoint  string            `toml:"sparqlEndpoint"`   // Default SPARQL endpoint
	SparqlEndpoints []SparqlEndpoint  `toml:"sparqlEndpoints"`  // Named endpoints for menu
	Timeout         int               `toml:"timeout"`          // timeout in seconds
	GeminiAPIKey    string            `toml:"gemini_api_key"`   // API key for Google Gemini
}

// SparqlEndpoint represents a named SPARQL endpoint configuration
type SparqlEndpoint struct {
	Name    string `toml:"name"`    // Display name (e.g., "LINDAS", "Wikidata")
	URL     string `toml:"url"`     // Full endpoint URL
	Default bool   `toml:"default"` // Optional: mark as default
}

// RDFConfig holds RDF-related settings
type RDFConfig struct {
	Prefixes       []string `toml:"prefixes"`
	ParsedPrefixes []Prefix
	TypePriority   []string `toml:"type_priority"` // Priority order for RDF types in template resolution
}

// Prefix represents a single RDF prefix definition
type Prefix struct {
	Name string // prefix name (e.g., "rdf", "rdfs")
	URI  string // URI enclosed in <> or without
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `toml:"level"`  // "DEBUG", "INFO", "WARN", "ERROR"
	Format string `toml:"format"` // "json" or "text"
	Output string `toml:"output"` // "stdout", "stderr", or file path
}

// Load reads and parses the TOML config file
// Returns loaded config with defaults for missing values
func Load(configPath string) (*Config, error) {
	// Set defaults
	cfg := &Config{
		Application: ApplicationConfig{
			Port:    8080,
			Timeout: 30,
		},
		Logging: LoggingConfig{
			Level:  "INFO",
			Format: "text",
			Output: "stdout",
		},
	}

	// Try to read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Parse TOML format
	if err := toml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse TOML config %s: %w", configPath, err)
	}

	// Parse prefix strings into Prefix structs
	cfg.RDF.ParsedPrefixes = cfg.RDF.ParsePrefixStrings()

	return cfg, nil
}

// GetTimeout returns the timeout as a time.Duration
func (c *Config) GetTimeout() time.Duration {
	return time.Duration(c.Application.Timeout) * time.Second
}

// GetPort returns the formatted port string ":8080" instead of int for router.Run()
func (c *Config) GetPort() string {
	return fmt.Sprintf(":%d", c.Application.Port)
}

// GetNamedEndpointsMap returns SPARQL endpoints as a map[name]url for quick lookup
func (a *ApplicationConfig) GetNamedEndpointsMap() map[string]string {
	m := make(map[string]string)
	for _, ep := range a.SparqlEndpoints {
		m[ep.Name] = ep.URL
	}
	return m
}

// ParsePrefixStrings parses an array of prefix strings into Prefix structs
// Supports both SPARQL notation (PREFIX rdf: <uri>) and Turtle notation (@prefix rdf: <uri> .)
// Also supports short notation (key: uri)
func (r *RDFConfig) ParsePrefixStrings() []Prefix {
	var prefixes []Prefix

	// Regex patterns for different prefix formats (case-insensitive for PREFIX)
	// SPARQL allows optional colon after prefix name
	sparqlPattern := regexp.MustCompile(`(?i)^\s*PREFIX\s+(\w+)\s*:?\s*<([^>]+)>\s*$`)
	turtlePattern := regexp.MustCompile(`^\s*@prefix\s+(\w+)\s*:\s*<([^>]+)>\s*\.\s*$`)
	shortPattern := regexp.MustCompile(`^\s*(\w+)\s*:\s*(.+?)\s*$`)

	for _, line := range r.Prefixes {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try SPARQL format first
		if matches := sparqlPattern.FindStringSubmatch(line); matches != nil {
			prefixes = append(prefixes, Prefix{
				Name: matches[1],
				URI:  "<" + matches[2] + ">",
			})
			continue
		}

		// Try Turtle format
		if matches := turtlePattern.FindStringSubmatch(line); matches != nil {
			prefixes = append(prefixes, Prefix{
				Name: matches[1],
				URI:  "<" + matches[2] + ">",
			})
			continue
		}

		// Try short format (key: uri)
		if matches := shortPattern.FindStringSubmatch(line); matches != nil {
			uri := strings.TrimSpace(matches[2])
			// Add angle brackets if not present
			if !strings.HasPrefix(uri, "<") {
				uri = "<" + uri + ">"
			}
			prefixes = append(prefixes, Prefix{
				Name: matches[1],
				URI:  uri,
			})
			continue
		}

		// Warn about unmatched prefix line
		log := logger.Get()
		log.Warn("invalid prefix format",
			slog.String("prefix", line),
			slog.String("expected_format", "PREFIX name: <uri>"))
	}

	return prefixes
}
