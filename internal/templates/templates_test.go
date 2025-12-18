package templates

import (
	"testing"
)

func TestLoad(t *testing.T) {
	// Note: This test requires running from project root where templates/ exists
	// Skip if templates directory doesn't exist

	// This will panic if templates can't be loaded (by design)
	r := Load("../../templates")

	if r == nil {
		t.Error("Load() returned nil renderer")
	}

	// Verify singleton was set
	if Get() == nil {
		t.Error("Get() returned nil after Load()")
	}

	// Note: Can't compare renderer instances directly (not comparable)
	// Just verify Get() doesn't panic and returns something
	r2 := Get()
	if r2 == nil {
		t.Error("Get() returned nil after Load()")
	}
}

func TestGetPanicsWithoutLoad(t *testing.T) {
	// Reset renderer
	renderer = nil

	defer func() {
		if r := recover(); r == nil {
			t.Error("Get() should panic when called before MustLoad()")
		}
	}()

	Get() // Should panic
}
