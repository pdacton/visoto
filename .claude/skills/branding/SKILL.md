---
name: visoto-branding
description: Apply Visoto brand guidelines for RDF visualization tool including visual identity, voice, technical conventions, and code standards
---

# Visoto Branding Skill

Apply consistent branding and guidelines for Visoto, an RDF resource visualization and SPARQL query exploration tool.

## Brand Identity
- **Product**: Visoto - SPARQL query and RDF resource visualization tool
- **Purpose**: Visualizing and exploring RDF resources using SPARQL queries
- **Target Audience**: Data scientists, semantic web developers, researchers, linked data professionals
- **Tech Stack**: Go (Gin framework), Tabler UI framework (Bootstrap 5-based), Tabulator for data tables

## Visual Identity & UI Guidelines
- **Logo Assets**: Use `visoto_dark.svg` for dark themes, `visoto_light.svg` for light themes
- **Color Scheme**: Support both light and dark mode theming
- **UI Framework**: Tabler (Bootstrap 5-based) for consistent, modern design
- **Typography**: Clean, modern, accessible fonts prioritizing readability
- **Layout**: Responsive design with organized sidebar navigation
- **Icons**: Use consistent iconography that supports accessibility

## Voice & Tone
- **Technical but Approachable**: Balance technical accuracy with user-friendly language
- **Clear and Concise**: Prefer direct, actionable language over verbose explanations
- **Professional yet Friendly**: Maintain expertise while being welcoming to newcomers
- **Documentation Style**: Focus on functionality and practical user experience
- **Error Messages**: Helpful and constructive, guiding users toward solutions

## Code Conventions & Standards
- **Language**: Go with modern idiomatic patterns
- **Web Framework**: Gin framework for HTTP routing and middleware
- **Project Structure**:
  - `/cmd/visoto/` - Application entry point
  - `/templates/` - Go HTML templates organized by function
  - `/static/` - CSS, JS, images, and other assets
  - `/internal/` - Private application code
- **Frontend**: Modern web standards with accessibility-first approach
- **Templating**: Go templates with clean separation of layout and content
- **Asset Organization**: Logical grouping of CSS, JS, and images

## Key Features to Emphasize
- **SPARQL Integration**: Seamless connection to RDF endpoints (currently Lindas, configurable)
- **Interactive Visualization**: User-friendly display of complex RDF data
- **Modern UI/UX**: Responsive design with dark mode support
- **Data Tables**: Interactive tables using Tabulator for exploring query results
- **Accessibility**: Ensure all features work with screen readers and keyboard navigation

## Development Guidelines
- **Configuration**: Make SPARQL endpoints configurable (currently hardcoded)
- **Error Handling**: Provide clear feedback for connection issues and query errors
- **Performance**: Optimize for handling large RDF datasets
- **Testing**: Include connectivity tests (e.g., `/ping` endpoint)
- **Documentation**: Maintain clear README with setup and usage instructions

## Brand Applications
When working on Visoto:
- Use brand-consistent language in UI text and documentation
- Apply visual identity guidelines to any new components
- Maintain technical accuracy while keeping explanations accessible
- Ensure all new features support both light and dark themes
- Follow established project structure and coding patterns