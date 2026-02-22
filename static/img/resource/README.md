# Resource Icons

SVG icons used to represent RDF resource types in the UI.

## Naming Convention

- `ClassName.svg` — standard icon, used when this class matches a resource.
- `ClassName.fallback.svg` — fallback icon, used only if no standard icon matched first.

## Fallback Icons

Some RDF classes (e.g. `DefinedTermSet`, `Version`) are very generic and apply to
a large proportion of resources in the LINDAS triple store. Their icons are marked
as fallbacks so that more specific class icons take precedence.

To deprioritize an icon, rename it from `Name.svg` to `Name.fallback.svg`.
