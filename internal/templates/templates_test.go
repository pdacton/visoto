package templates

import (
	"path/filepath"
	"testing"
	"time"
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

// TestDebugMode covers the flag behind the footer's raw template-data dump.
// The dump leaks every page's internal data, so the default must be off and
// SetDebugMode must be the only thing that turns it on.
func TestDebugMode(t *testing.T) {
	t.Cleanup(func() { SetDebugMode(false) })

	if DebugMode() {
		t.Error("DebugMode() = true by default, want false")
	}

	SetDebugMode(true)
	if !DebugMode() {
		t.Error("DebugMode() = false after SetDebugMode(true), want true")
	}

	SetDebugMode(false)
	if DebugMode() {
		t.Error("DebugMode() = true after SetDebugMode(false), want false")
	}
}

// TestCurrentYear guards the footer's copyright against going stale again by
// checking it tracks the clock rather than returning a baked-in year.
func TestCurrentYear(t *testing.T) {
	if got, want := currentYear(), time.Now().Year(); got != want {
		t.Errorf("currentYear() = %d, want %d", got, want)
	}
}
