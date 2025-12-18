# Implementation Plan: Refactor Template Functions to Internal Package

## Overview
Move template-related code from `cmd/visoto/main.go` to a new `internal/templates` package. This follows the existing project patterns and prepares for future URL routing and rendering features.

## Goals
1. Extract template functionality from main.go into a dedicated package
2. Follow existing internal package patterns (logger, config, sparqlpreproc)
3. Maintain current functionality with no behavioral changes
4. Prepare structure for future router and renderer additions

## Analysis of Existing Patterns

### Package Structure Pattern
Existing internal packages follow this pattern:
- `internal/logger/` - Singleton pattern, `MustInit()` + `Get()`, Config struct
- `internal/config/` - `Load()` function, Config struct with methods
- `internal/sparqlpreproc/` - `New()` constructor, Config struct, Preprocessor type

### Common Patterns Observed
1. **Configuration**: Each package has a `Config` struct for initialization
2. **Constructors**: Use `New(config Config)` pattern (sparqlpreproc) or `MustInit(cfg Config)` (logger)
3. **Exported types**: Main types are exported (e.g., `Preprocessor`, `Config`)
4. **Package-level singletons**: Used in logger package via `var log *slog.Logger`
5. **Testing**: Standard Go table-driven tests in `*_test.go` files

## Current State (What to Move)

From `cmd/visoto/main.go`:

### Lines 37-40: TemplateSet Type
```go
type TemplateSet struct {
    *template.Template
}
```

### Lines 42-55: MustLoad Method
```go
func (ts *TemplateSet) MustLoad() {
    // Initialize with layout templates
    ts.Template = template.Must(template.ParseGlob("templates/layout/*.html"))

    // Add page templates to the existing set
    ts.Template = template.Must(ts.Template.ParseGlob("templates/pages/*.html"))

    log := logger.Get()
    log.Debug("templates loaded",
        slog.String("templates", ts.DefinedTemplates()))
}
```

### Line 58: Package-level Instance
```go
var tmpl = &TemplateSet{}
```

**What stays in main.go:**
- Import of `html/template` (still needed for gin.Context in handlers)
- Template execution calls in route handlers: `tmpl.ExecuteTemplate(c.Writer, ...)`
- The `tmpl` variable declaration (but will use `templates.New()`)

## Implementation Steps

### Step 1: Create New Package Structure
```
internal/templates/
├── templates.go       # Main package file
└── templates_test.go  # Unit tests (future)
```

### Step 2: Create `internal/templates/templates.go`

**Package declaration and imports:**
```go
package templates

import (
    "html/template"
    "log/slog"
    "hutzli.org/visoto/internal/logger"
)
```

**Type definition:**
```go
// TemplateSet wraps template.Template with additional functionality
// Embeds template.Template to inherit all its methods
type TemplateSet struct {
    *template.Template
}
```

**Constructor function:**
```go
// New creates a new TemplateSet instance
// Returns an empty TemplateSet that must be loaded before use
func New() *TemplateSet {
    return &TemplateSet{}
}
```

**MustLoad method:**
```go
// MustLoad loads Go HTML templates from templates/layout and templates/pages
// Exits the program if templates cannot be loaded
// Uses template.Must to panic on parsing errors (fail-fast approach)
func (ts *TemplateSet) MustLoad() {
    // Initialize with layout templates
    ts.Template = template.Must(template.ParseGlob("templates/layout/*.html"))

    // Add page templates to the existing set
    ts.Template = template.Must(ts.Template.ParseGlob("templates/pages/*.html"))

    log := logger.Get()
    log.Debug("templates loaded",
        slog.String("templates", ts.DefinedTemplates()))
}
```

### Step 3: Update `cmd/visoto/main.go`

**Update imports:**
```go
import (
    "io"
    "fmt"
    "log/slog"
    "os"
    "strings"
    "net/url"
    "net/http"
    "html/template"  // Keep this - used by gin.Context
    "encoding/json"
    "github.com/gin-gonic/gin"
    "hutzli.org/visoto/internal/config"
    "hutzli.org/visoto/internal/logger"
    "hutzli.org/visoto/internal/sparqlpreproc"
    "hutzli.org/visoto/internal/templates"  // NEW
)
```

**Remove lines 37-55:**
- Delete `TemplateSet` type definition
- Delete `MustLoad()` method

**Update line 58:**
```go
// OLD:
var tmpl = &TemplateSet{}

// NEW:
var tmpl = templates.New()
```

**All other code remains unchanged:**
- Line 358: `tmpl.MustLoad()` - no change needed
- Line 159: `tmpl.ExecuteTemplate(c.Writer, "base.html", nil)` - no change needed
- Line 192: `tmpl.ExecuteTemplate(c.Writer, "resource.html", data)` - no change needed
- Line 224: `tmpl.ExecuteTemplate(c.Writer, "embedded.html", data)` - no change needed
- All other template execution calls - no change needed

### Step 4: Verify Module and Imports

**Module name:** `hutzli.org/visoto` (from go.mod)

**Import path for new package:** `hutzli.org/visoto/internal/templates`

**No go.mod changes needed** - internal packages don't require module updates

### Step 5: Testing Strategy

**Manual testing approach:**
1. Run `go build cmd/visoto/main.go` to verify compilation
2. Run the application: `go run cmd/visoto/main.go`
3. Test routes:
   - `GET /` - home page (base.html)
   - `GET /ping` - health check
   - `GET /resource/https://register.ld.admin.ch/zefix/company/38909#` - resource page
   - `GET /embedded/https://register.ld.admin.ch/zefix/company/38909#` - embedded page
4. Verify template loading debug logs appear
5. Verify no errors in template execution

**Future unit test structure** (templates_test.go):
```go
package templates

import "testing"

func TestNew(t *testing.T) {
    ts := New()
    if ts == nil {
        t.Error("New() returned nil")
    }
}

// Additional tests can be added later
```

## File Changes Summary

### New Files (1)
- `internal/templates/templates.go` - 30 lines

### Modified Files (1)
- `cmd/visoto/main.go`:
  - Add 1 import line
  - Remove 19 lines (TemplateSet type + MustLoad method)
  - Change 1 line (tmpl initialization)
  - Net change: -17 lines

### Deleted Files (0)

## Future Extensions (Not in this PR)

### Phase 2: Router (URL-to-template matching)
```go
// internal/templates/router.go
func (ts *TemplateSet) LookupByURL(url string) (string, error)
func (ts *TemplateSet) RegisterRoute(pattern, templateName string)
```

### Phase 3: Renderer (Gin-aware helpers)
```go
// internal/templates/renderer.go
func (ts *TemplateSet) Render(c *gin.Context, templateName string, data any) error
```

### Phase 4: Configuration
```go
// Allow configurable template paths
type Config struct {
    LayoutPath string
    PagesPath  string
}
```

## Risks and Mitigations

### Risk: Breaking template loading
**Mitigation:**
- No changes to MustLoad() logic - exact copy
- Template paths remain hardcoded and unchanged
- Template.Must() ensures fail-fast behavior

### Risk: Import cycle
**Mitigation:**
- New package only imports logger (existing dependency pattern)
- main.go imports templates (one-way dependency)
- No circular dependencies possible

### Risk: Template execution failures
**Mitigation:**
- TemplateSet embeds *template.Template
- All template.Template methods available unchanged
- ExecuteTemplate() calls work identically

## Success Criteria

1. ✅ Code compiles without errors
2. ✅ Application starts successfully
3. ✅ All route handlers work as before
4. ✅ Templates load correctly (debug log appears)
5. ✅ Template execution succeeds for all routes
6. ✅ No runtime errors or panics
7. ✅ Code follows existing package patterns

## Implementation Order

1. Create `internal/templates/` directory
2. Write `internal/templates/templates.go`
3. Update imports in `cmd/visoto/main.go`
4. Update tmpl initialization in `cmd/visoto/main.go`
5. Remove TemplateSet type and MustLoad from `cmd/visoto/main.go`
6. Build and test

## Rollback Plan

If issues arise:
1. Revert changes to `cmd/visoto/main.go`
2. Delete `internal/templates/` directory
3. Code returns to working state immediately

## Time Estimate

- Implementation: 10 minutes
- Testing: 10 minutes
- Total: 20 minutes

This is a low-risk refactoring with clear patterns to follow.
