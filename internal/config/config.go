// load application config file from TOML file into Config struct

package config

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"hutzli.org/visoto/internal/logger"
)

// TODO: add validation funtions for config fields (e.g. port range, valid URIs, etc.)
// TODO: add constructor function for Config with default values

// OntologyEntry represents a well-known ontology that can be loaded via the upload modal.
type OntologyEntry struct {
	Name  string `toml:"name"  json:"name"`  // Short display name (e.g. "SKOS")
	URL   string `toml:"url"   json:"url"`   // Canonical ontology URL to fetch
	Graph string `toml:"graph" json:"graph"` // Target named graph URI (e.g. "urn:ontology:w3/skos")
}

// Config represents the complete application configuration
type Config struct {
	Application ApplicationConfig `toml:"application"`
	RDF         RDFConfig         `toml:"rdf"`
	Logging     LoggingConfig     `toml:"logging"`
	MCP         MCPConfig         `toml:"mcp"`
	Ontologies  []OntologyEntry   `toml:"ontologies"`
}

// MCPConfig holds MCP server settings
type MCPConfig struct {
	Port int `toml:"port"` // default 8070
}

// ApplicationConfig holds application-level settings
type ApplicationConfig struct {
	Port            int              `toml:"port"`
	SparqlEndpoint  string           `toml:"sparqlEndpoint"`  // Default SPARQL endpoint
	SparqlEndpoints []SparqlEndpoint `toml:"sparqlEndpoints"` // Named endpoints for menu
	Timeout         int              `toml:"timeout"`         // timeout in seconds
	GeminiAPIKey    string           `toml:"gemini_api_key"`  // API key for Google Gemini

	// AllowPrivateUploadURLs permits the URL-mode upload (/api/upload) to fetch
	// private, loopback, and link-local addresses. Off by default to prevent
	// SSRF; enable in dev/test configs that fetch from http://localhost.
	AllowPrivateUploadURLs bool `toml:"allow_private_upload_urls"`

	// Languages is the closed set of UI languages the site offers, in the order
	// the topbar picker lists them, each with the label shown in that picker. An
	// empty-string code is legal and meaningful: it is the "no language" choice
	// (reserved semantics for RDF literal filtering — untagged literals only),
	// and for UI strings it renders the base catalog. Every non-empty code needs
	// a locales/<code>.toml catalog.
	//
	// The codes are duplicated in the Caddyfile, which normalizes the request
	// into X-Site-Lang and cannot read this file. That duplication is safe:
	// lang.Set re-cleans the header against this list, so a Caddyfile that drifts
	// ahead can only fall back to DefaultLanguage, never inject an unknown code.
	Languages []Language `toml:"languages"`

	// DefaultLanguage is used when the request expresses no usable preference.
	// Must be the code of a member of Languages.
	DefaultLanguage string `toml:"default_language"`
}

// Language is one entry in the UI language picker.
type Language struct {
	// Code is the value carried by the site-lang cookie and the X-Site-Lang
	// header, and the name of the locales/<code>.toml catalog. "" is the
	// "no language" choice.
	Code string `toml:"code"`
	// Label is what the picker displays. Conventionally the language's own
	// endonym ("Deutsch", not "German"), so the picker reads the same whichever
	// language the page is currently rendered in — which is what a reader
	// looking for their own language needs. It is a literal, not a message key:
	// one label per code, identical in every locale.
	Label string `toml:"label"`

	// Name is the full language name shown in the picker's dropdown menu, while
	// Label stays the compact form shown in the closed topbar control. Optional:
	// when unset it falls back to Label, so a config that only lists labels
	// keeps working unchanged. Like Label it is a literal, not a message key.
	Name string `toml:"name"`
}

// DisplayName is the full name to show in the language menu: Name when the
// config supplies one, otherwise the compact Label.
func (l Language) DisplayName() string {
	if n := strings.TrimSpace(l.Name); n != "" {
		return n
	}
	return l.Label
}

// SparqlEndpoint represents a named SPARQL endpoint configuration
type SparqlEndpoint struct {
	Name    string `toml:"name"`    // Display name (e.g., "LINDAS", "Wikidata")
	URL     string `toml:"url"`     // Full endpoint URL
	Default bool   `toml:"default"` // Optional: mark as default
	Monitor bool   `toml:"monitor"` // Enable health monitoring for this endpoint
	Tag     string `toml:"tag"`     // Logical group tag for UI customization (e.g., "lindas", "stadtzuerich")
	Slug    string `toml:"slug"`    // Unique URL-safe identifier for the ?endpoint= query param (shareable links)
	// Write credentials. json:"-" is load-bearing, not cosmetic: this struct
	// reaches templates as TemplateData.SparqlEndpoints, which is serialised
	// wholesale by the footer's raw-data dump and by the chat resource-data
	// embed. Without these tags encoding/json exports the field names and
	// these secrets end up in the HTML of every page. They are only ever read
	// server-side (internal/upload, internal/export) to set an Authorization
	// header, so nothing needs them on the wire.
	Username       string `toml:"username"     json:"-"` // Optional basic auth username for write operations
	Password       string `toml:"password"     json:"-"` // Optional basic auth password for write operations
	AccessToken    string `toml:"access_token" json:"-"` // Optional Bearer token for write operations (takes precedence over username/password)
	SearchProvider string `toml:"search_provider"`       // FTS provider: "stardog" (default), "graphdb" (Simple FTS), "graphdb-lucene" (auto-discovered Lucene connectors), "fuseki", "qlever", "sparql-query"
	ExportProvider string `toml:"export_provider"`       // optional export provider override: "graphdb", "gsp", "construct"
}

// RDFConfig holds RDF-related settings
type RDFConfig struct {
	Prefixes        []string `toml:"prefixes"`
	ParsedPrefixes  []Prefix
	TypePriority    []string          `toml:"type_priority"`    // Priority order for RDF types in template resolution
	MagicProperties map[string]string `toml:"magic_properties"` // visoto:<key> tokens expanded to property paths in queries
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
			Port:            8080,
			Timeout:         30,
			Languages:       DefaultLanguages(),
			DefaultLanguage: "en",
		},
		Logging: LoggingConfig{
			Level:  "INFO",
			Format: "text",
			Output: "stdout",
		},
		MCP: MCPConfig{
			Port: 8070,
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

	// The slug is the only endpoint identifier that crosses the wire (URL params,
	// cookie, API params), so every configured endpoint must carry a usable one.
	if err := cfg.Application.validateEndpointSlugs(); err != nil {
		return cfg, fmt.Errorf("invalid endpoint config in %s: %w", configPath, err)
	}

	// A config that lists a language with no catalog, or a default outside the
	// list, would silently render half-translated pages — fail fast instead.
	if err := cfg.Application.validateLanguages(); err != nil {
		return cfg, fmt.Errorf("invalid language config in %s: %w", configPath, err)
	}

	// The PORT env var, when set, overrides the port from the config file.
	// This lets a second instance run on a different port without editing
	// visoto.config (e.g. PORT=8061 go run ./cmd/visoto/).
	if portEnv := os.Getenv("PORT"); portEnv != "" {
		port, err := strconv.Atoi(portEnv)
		if err != nil || port < 1 || port > 65535 {
			return cfg, fmt.Errorf("invalid PORT env var %q: must be an integer in 1..65535", portEnv)
		}
		cfg.Application.Port = port
		logger.Get().Info("port overridden by PORT env var", slog.Int("port", port))
	}

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

// DefaultEndpoint returns the endpoint marked default = true, else the first
// configured endpoint, or nil when no endpoints are configured (bare
// sparql_endpoint-only configs).
func (a *ApplicationConfig) DefaultEndpoint() *SparqlEndpoint {
	for i := range a.SparqlEndpoints {
		if a.SparqlEndpoints[i].Default {
			return &a.SparqlEndpoints[i]
		}
	}
	if len(a.SparqlEndpoints) > 0 {
		return &a.SparqlEndpoints[0]
	}
	return nil
}

// validateEndpointSlugs requires every configured endpoint to have a non-empty
// slug, unique case-insensitively (GetEndpointBySlug matches with EqualFold).
func (a *ApplicationConfig) validateEndpointSlugs() error {
	seen := make(map[string]string, len(a.SparqlEndpoints))
	for _, ep := range a.SparqlEndpoints {
		if ep.Slug == "" {
			return fmt.Errorf("endpoint %q has no slug; every endpoint needs a unique slug", ep.Name)
		}
		key := strings.ToLower(ep.Slug)
		if other, dup := seen[key]; dup {
			return fmt.Errorf("endpoints %q and %q share slug %q (case-insensitive)", other, ep.Name, ep.Slug)
		}
		seen[key] = ep.Name
	}
	return nil
}

// DefaultLanguages is the language set used when visoto.config declares none:
// the four Swiss national languages plus English, and the empty "no language"
// choice. Labels are compact codes, names their endonyms. Returns a fresh
// slice so callers can't mutate it.
func DefaultLanguages() []Language {
	return []Language{
		{Code: "de", Label: "DE", Name: "Deutsch"},
		{Code: "fr", Label: "FR", Name: "Français"},
		{Code: "it", Label: "IT", Name: "Italiano"},
		{Code: "en", Label: "EN", Name: "English"},
		{Code: "rm", Label: "RM", Name: "Rumantsch"},
		{Code: "", Label: "None", Name: "No language"},
	}
}

// LanguageCodes returns just the configured codes, in configured order — the
// form the language resolver and the catalog loader want.
func (a *ApplicationConfig) LanguageCodes() []string {
	codes := make([]string, 0, len(a.Languages))
	for _, l := range a.Languages {
		codes = append(codes, l.Code)
	}
	return codes
}

// validateLanguages checks the structural rules for the UI language set: at
// least one entry, no duplicate codes, well-formed non-empty codes, a label on
// every entry, and a default that is a member of the list. An empty
// `languages` in the file falls back to DefaultLanguages rather than leaving the
// site with no languages at all.
//
// Whether each code actually has a translation catalog is checked by
// internal/i18n at bundle load — this package deliberately knows nothing about
// locale files.
func (a *ApplicationConfig) validateLanguages() error {
	if len(a.Languages) == 0 {
		a.Languages = DefaultLanguages()
	}
	seen := make(map[string]bool, len(a.Languages))
	for _, l := range a.Languages {
		if l.Code != strings.ToLower(strings.TrimSpace(l.Code)) {
			return fmt.Errorf("language code %q must be lowercase and unpadded", l.Code)
		}
		if l.Code != "" && !languageCodeRe.MatchString(l.Code) {
			return fmt.Errorf("language %q is not a bare language code (e.g. \"de\", \"rm\")", l.Code)
		}
		if seen[l.Code] {
			return fmt.Errorf("language %q is listed twice", l.Code)
		}
		// Without a label the picker would render a blank, unselectable-looking
		// option — worse than refusing to start.
		if strings.TrimSpace(l.Label) == "" {
			return fmt.Errorf("language %q has no label", l.Code)
		}
		seen[l.Code] = true
	}
	// No empty-string special case here: Load seeds DefaultLanguage to "en", so a
	// literal "" reaching this point was written deliberately, and "" is a legal
	// member of the set.
	if !seen[a.DefaultLanguage] {
		return fmt.Errorf("default_language %q is not among the configured languages %v",
			a.DefaultLanguage, a.LanguageCodes())
	}
	return nil
}

// languageCodeRe matches a bare ISO 639 code. Region subtags are deliberately
// rejected: X-Site-Lang values are normalized to base codes, and a configured
// "de-CH" would never be matched by lang.Set.Clean.
var languageCodeRe = regexp.MustCompile(`^[a-z]{2,3}$`)

// GetEndpointByURL returns the SparqlEndpoint with the given URL, or nil if
// no configured endpoint uses that URL.
func (a *ApplicationConfig) GetEndpointByURL(url string) *SparqlEndpoint {
	for i := range a.SparqlEndpoints {
		if a.SparqlEndpoints[i].URL == url {
			return &a.SparqlEndpoints[i]
		}
	}
	return nil
}

// GetEndpointBySlug returns the SparqlEndpoint with the given slug (case-insensitive),
// or nil if no endpoint has that slug configured. Endpoints with an empty Slug never match.
func (a *ApplicationConfig) GetEndpointBySlug(slug string) *SparqlEndpoint {
	for i := range a.SparqlEndpoints {
		if a.SparqlEndpoints[i].Slug != "" && strings.EqualFold(a.SparqlEndpoints[i].Slug, slug) {
			return &a.SparqlEndpoints[i]
		}
	}
	return nil
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
