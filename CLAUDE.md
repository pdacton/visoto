# Agent Instructions for this Project

## Project Overview
- This project is written in Go and uses Go's `html/template` package for server-side rendering.
- The frontend uses the [Tabler](https://tabler.io/) CSS and JavaScript framework for UI components and layout, which is based on Bootstrap 5.
- For interactive data tables, the project uses the [Tabulator](https://tabulator.info/docs/6.3/quickstart) library.

## Key Guidelines

### Skills
Available skills in `.claude/skills/`:
- **graph-explorer** - Reference guide for customizing Graph Explorer (Ontodia fork) - source files, CSS classes, API, and customization patterns
- **branding** - Apply Visoto brand guidelines for RDF visualization tool including visual identity, voice, technical conventions, and code standards
- **instanceTemplate** - Generate Visoto template files for RDF instances showing attributes, relationships, and connections
- **classTemplate** - Generate Visoto template files for RDF classes showing instances and class hierarchy

Use these skills when working on related tasks.

### Go Backend
- Use idiomatic Go for all backend logic.
- Templates are rendered using Go's `template.Template`. Shared templates like base, header, footer are located in the `templates/layout/` directory. Templates for specific resources are located in the `templates/pages/` subfolder).
- Static assets (CSS, JS, images) are served from the `/static` directory.
- Prefer member function (method) coding style over standalone functions when working with structs. Define methods with receiver types to encapsulate behavior and improve code organization. Example: func (u *User) SendEmail(message string) error { }

### Templates
- All HTML templates are based on the Bootstrap 5 framework and should use Bootstrap 5 classes and components for layout and styling.
- Reuse Bootstrap 5 CSS and JavaScript as much as possible for consistency and maintainability.
- Extend or include the base layout, and use Tabler classes for additional styling.
- When adding new pages, create a new template in the appropriate folder and ensure it is loaded by the Go server.
- Use Go template syntax (`{{ ... }}`) for dynamic content.

### Tabler Integration
- Use Tabler's CSS classes for layout, navigation, and UI components.
- Include Tabler's JS via CDN or local assets as needed.
- For dark/light mode, ensure the correct `data-bs-theme` attribute is set and toggled.

### Tabulator Integration
- When creating data tables, use the Tabulator library as described in the [Tabulator Quickstart Guide](https://tabulator.info/docs/6.3/quickstart).
- Include Tabulator's CSS and JS in your templates where tables are used.
- Initialize Tabulator tables in a `<script>` block or external JS file after the DOM is ready.

### General
- Keep code modular and organized.
- Follow best practices for accessibility (ARIA attributes, semantic HTML).
- Document any custom logic or non-obvious code in comments.

### Data Layer
- The project uses a **SPARQL endpoint** for data access, configured in the `visoto.config` file.
- All RDF data queries go through the SPARQL endpoint (currently https://ld.admin.ch/query/).
- No traditional database layer - all data is retrieved via SPARQL queries.

### Testing
- **Go backend**: Unit tests using Go's standard testing package (`*_test.go` files).
- **Templates**: Template testing approach is currently undefined.

### Deployment
- Deployment is managed via the `deploy.sh` script in the project root.
- **Environment variables**: Currently none in use.

### Graph Explorer Integration
- The project uses [Graph Explorer](https://github.com/zazuko/graph-explorer) (a fork of Ontodia) for RDF graph visualization.
- Graph Explorer is loaded from CDN: `graph-explorer@1.3.0`
- Local files:
  - `templates/pages/ontodia.html` - Main Graph Explorer page
  - `static/css/ontodia_overrides.css` - Custom CSS overrides
- **For API reference and configuration options**, see `/docs/ontodia-graph-explorer-references.md`
- **For customization guidance**, see the `graph-explorer` skill in `.claude/skills/graph-explorer/`

## References

### External Documentation
- [Tabler Documentation](https://tabler.io/docs/)
- [Tabler SCSS Variables](https://github.com/tabler/tabler/blob/dev/core/scss/_variables.scss) - Box shadows, colors, spacing
- [Tabulator Documentation](https://tabulator.info/docs/6.3/quickstart)
- [Go Templates Documentation](https://pkg.go.dev/html/template)
- [Bootstrap 5 Documentation](https://getbootstrap.com/docs/5.0/getting-started/introduction/)
- [Graph Explorer Repository](https://github.com/zazuko/graph-explorer)

### Internal Documentation
- `/docs/ontodia-graph-explorer-references.md` - Complete Graph Explorer/Ontodia API reference and configuration options

---
For any new features, follow the established structure and use Bootstrap 5, Tabler, and Tabulator for UI consistency.
