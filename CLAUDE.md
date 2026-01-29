# Agent Instructions for this Project

## Project Overview
- This project is written in Go and uses Go's `html/template` package for server-side rendering.
- The frontend uses the [Tabler](https://tabler.io/) CSS and JavaScript framework for UI components and layout, which is based on Bootstrap 5.
- For interactive data tables, the project uses the [Tabulator](https://tabulator.info/docs/6.3/quickstart) library.

## Key Guidelines

### Skills
- find specific skills in `.claude/skills/individual skill` folders
- use the skills appropriately

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

### Graph Explorer Integration
- The project uses [Graph Explorer](https://github.com/zazuko/graph-explorer) (a fork of Ontodia) for RDF graph visualization.
- Graph Explorer is loaded from CDN: `graph-explorer@1.3.0`
- Local files:
  - `templates/pages/ontodia.html` - Main Graph Explorer page
  - `static/css/ontodia_overrides.css` - Custom CSS overrides
- **For detailed customization reference**, see the `graph-explorer-customization` skill in `.claude/skills/graph-explorer/`

## References
- [Tabler Documentation](https://tabler.io/docs/)
- [Tabler SCSS Variables](https://github.com/tabler/tabler/blob/dev/core/scss/_variables.scss) - Box shadows, colors, spacing
- [Tabulator Documentation](https://tabulator.info/docs/6.3/quickstart)
- [Go Templates Documentation](https://pkg.go.dev/html/template)
- [Bootstrap 5 Documentation](https://getbootstrap.com/docs/5.0/getting-started/introduction/)
- [Graph Explorer Repository](https://github.com/zazuko/graph-explorer)

---
For any new features, follow the established structure and use Bootstrap 5, Tabler, and Tabulator for UI consistency.
