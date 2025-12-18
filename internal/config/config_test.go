package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadConfig tests loading a valid TOML config file
func TestLoadConfig(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test.toml")

	configContent := `
[application]
port = 8080
sparqlEndpoint = "https://example.com/sparql"
timeout = 30

[rdf]
prefixes = [
	"PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
	"PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>"
]
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test loading
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	// Verify application config
	if cfg.Application.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Application.Port)
	}
	if cfg.Application.SparqlEndpoint != "https://example.com/sparql" {
		t.Errorf("SparqlEndpoint = %s, want https://example.com/sparql", cfg.Application.SparqlEndpoint)
	}
	if cfg.Application.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", cfg.Application.Timeout)
	}

	// Verify RDF config
	if len(cfg.RDF.Prefixes) != 2 {
		t.Errorf("Prefixes count = %d, want 2", len(cfg.RDF.Prefixes))
	}

	// Verify parsed prefixes
	if len(cfg.RDF.ParsedPrefixes) != 2 {
		t.Errorf("ParsedPrefixes count = %d, want 2", len(cfg.RDF.ParsedPrefixes))
	}

	if cfg.RDF.ParsedPrefixes[0].Name != "rdf" {
		t.Errorf("First prefix name = %s, want rdf", cfg.RDF.ParsedPrefixes[0].Name)
	}
	if cfg.RDF.ParsedPrefixes[0].URI != "<http://www.w3.org/1999/02/22-rdf-syntax-ns#>" {
		t.Errorf("First prefix URI = %s, want <http://www.w3.org/1999/02/22-rdf-syntax-ns#>", cfg.RDF.ParsedPrefixes[0].URI)
	}
}

// TestLoadConfigFileNotFound tests loading a non-existent config file
func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("Load() error = nil, want error for non-existent file")
	}
}

// TestLoadConfigInvalidTOML tests loading invalid TOML content
func TestLoadConfigInvalidTOML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.toml")

	invalidContent := `
[application
port = not a number
`

	if err := os.WriteFile(configPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() error = nil, want error for invalid TOML")
	}
}

// TestGetTimeout tests the GetTimeout method
func TestGetTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout int
		want    time.Duration
	}{
		{"30 seconds", 30, 30 * time.Second},
		{"60 seconds", 60, 60 * time.Second},
		{"0 seconds", 0, 0},
		{"negative", -5, -5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Application: ApplicationConfig{
					Timeout: tt.timeout,
				},
			}
			got := cfg.GetTimeout()
			if got != tt.want {
				t.Errorf("GetTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetPort tests the GetPort method
func TestGetPort(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{"standard port", 8080, ":8080"},
		{"port 80", 80, ":80"},
		{"port 443", 443, ":443"},
		{"high port", 65535, ":65535"},
		{"zero port", 0, ":0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Application: ApplicationConfig{
					Port: tt.port,
				},
			}
			got := cfg.GetPort()
			if got != tt.want {
				t.Errorf("GetPort() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParsePrefixStrings_SPARQL tests parsing SPARQL format prefixes
func TestParsePrefixStrings_SPARQL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantURI  string
	}{
		{
			name:     "SPARQL uppercase",
			input:    "PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
			wantName: "rdf",
			wantURI:  "<http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
		},
		{
			name:     "SPARQL lowercase",
			input:    "prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#>",
			wantName: "rdfs",
			wantURI:  "<http://www.w3.org/2000/01/rdf-schema#>",
		},
		{
			name:     "SPARQL with spaces",
			input:    "  PREFIX   owl:   <http://www.w3.org/2002/07/owl#>  ",
			wantName: "owl",
			wantURI:  "<http://www.w3.org/2002/07/owl#>",
		},
		{
			name:     "SPARQL without colon after prefix name",
			input:    "PREFIX foaf <http://xmlns.com/foaf/0.1/>",
			wantName: "foaf",
			wantURI:  "<http://xmlns.com/foaf/0.1/>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdf := RDFConfig{
				Prefixes: []string{tt.input},
			}
			result := rdf.ParsePrefixStrings()

			if len(result) != 1 {
				t.Fatalf("ParsePrefixStrings() returned %d prefixes, want 1", len(result))
			}

			if result[0].Name != tt.wantName {
				t.Errorf("Name = %s, want %s", result[0].Name, tt.wantName)
			}
			if result[0].URI != tt.wantURI {
				t.Errorf("URI = %s, want %s", result[0].URI, tt.wantURI)
			}
		})
	}
}

// TestParsePrefixStrings_Turtle tests parsing Turtle format prefixes
func TestParsePrefixStrings_Turtle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantURI  string
	}{
		{
			name:     "Turtle format",
			input:    "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .",
			wantName: "rdf",
			wantURI:  "<http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
		},
		{
			name:     "Turtle with spaces",
			input:    "  @prefix   rdfs:   <http://www.w3.org/2000/01/rdf-schema#>  .  ",
			wantName: "rdfs",
			wantURI:  "<http://www.w3.org/2000/01/rdf-schema#>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdf := RDFConfig{
				Prefixes: []string{tt.input},
			}
			result := rdf.ParsePrefixStrings()

			if len(result) != 1 {
				t.Fatalf("ParsePrefixStrings() returned %d prefixes, want 1", len(result))
			}

			if result[0].Name != tt.wantName {
				t.Errorf("Name = %s, want %s", result[0].Name, tt.wantName)
			}
			if result[0].URI != tt.wantURI {
				t.Errorf("URI = %s, want %s", result[0].URI, tt.wantURI)
			}
		})
	}
}

// TestParsePrefixStrings_ShortFormat tests parsing short format prefixes
func TestParsePrefixStrings_ShortFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantURI  string
	}{
		{
			name:     "Short format with brackets",
			input:    "rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
			wantName: "rdf",
			wantURI:  "<http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
		},
		{
			name:     "Short format without brackets",
			input:    "rdfs: http://www.w3.org/2000/01/rdf-schema#",
			wantName: "rdfs",
			wantURI:  "<http://www.w3.org/2000/01/rdf-schema#>",
		},
		{
			name:     "Short format with spaces",
			input:    "  owl:   http://www.w3.org/2002/07/owl#  ",
			wantName: "owl",
			wantURI:  "<http://www.w3.org/2002/07/owl#>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdf := RDFConfig{
				Prefixes: []string{tt.input},
			}
			result := rdf.ParsePrefixStrings()

			if len(result) != 1 {
				t.Fatalf("ParsePrefixStrings() returned %d prefixes, want 1", len(result))
			}

			if result[0].Name != tt.wantName {
				t.Errorf("Name = %s, want %s", result[0].Name, tt.wantName)
			}
			if result[0].URI != tt.wantURI {
				t.Errorf("URI = %s, want %s", result[0].URI, tt.wantURI)
			}
		})
	}
}

// TestParsePrefixStrings_EmptyAndInvalid tests handling of empty and invalid prefixes
func TestParsePrefixStrings_EmptyAndInvalid(t *testing.T) {
	rdf := RDFConfig{
		Prefixes: []string{
			"",
			"   ",
			"PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
			"invalid line without proper format",
			"",
			"PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>",
		},
	}

	result := rdf.ParsePrefixStrings()

	// Should only parse the two valid PREFIX lines
	if len(result) != 2 {
		t.Errorf("ParsePrefixStrings() returned %d prefixes, want 2 (invalid lines should be skipped)", len(result))
	}

	if len(result) >= 2 {
		if result[0].Name != "rdf" {
			t.Errorf("First prefix name = %s, want rdf", result[0].Name)
		}
		if result[1].Name != "rdfs" {
			t.Errorf("Second prefix name = %s, want rdfs", result[1].Name)
		}
	}
}

// TestParsePrefixStrings_Mixed tests parsing a mix of different formats
func TestParsePrefixStrings_Mixed(t *testing.T) {
	rdf := RDFConfig{
		Prefixes: []string{
			"PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
			"@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .",
			"owl: http://www.w3.org/2002/07/owl#",
		},
	}

	result := rdf.ParsePrefixStrings()

	if len(result) != 3 {
		t.Fatalf("ParsePrefixStrings() returned %d prefixes, want 3", len(result))
	}

	expectedNames := []string{"rdf", "rdfs", "owl"}
	for i, expected := range expectedNames {
		if result[i].Name != expected {
			t.Errorf("Prefix[%d].Name = %s, want %s", i, result[i].Name, expected)
		}
		// All should have URIs wrapped in angle brackets
		if !hasPrefix(result[i].URI, "<") || !hasSuffix(result[i].URI, ">") {
			t.Errorf("Prefix[%d].URI = %s, should be wrapped in angle brackets", i, result[i].URI)
		}
	}
}

// Helper functions for string checking
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
