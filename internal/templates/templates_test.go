package templates

import (
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	const templatesDir = "../../templates"

	r := Load(templatesDir)

	if r == nil {
		t.Fatal("Load() returned nil renderer")
	}

	// Verify expected template groups are non-empty
	for _, group := range []struct {
		name    string
		pattern string
	}{
		{"layouts", templatesDir + "/layout/*.html"},
		{"partials", templatesDir + "/partials/*.html"},
		{"pages", templatesDir + "/pages/*.html"},
		{"classes", templatesDir + "/classes/*.html"},
		{"instances", templatesDir + "/instances/*.html"},
	} {
		files, err := filepath.Glob(group.pattern)
		if err != nil {
			t.Fatalf("glob error for %s: %v", group.name, err)
		}
		if len(files) == 0 {
			t.Errorf("expected at least one %s template, got none", group.name)
		}
	}
}
