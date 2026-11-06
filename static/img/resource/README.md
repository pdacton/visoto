# Resource Icons

SVG icons used to represent RDF resource types in the UI.

## Naming Convention

- `ClassName.svg` — standard icon, used when this class matches a resource.
- `ClassName.fallback.svg` — fallback icon, used only if no standard icon matched first.

## Namespace Folders

Root icons match on the IRI's local name alone, so `Property.svg` serves every
ontology's `Property`. To give one namespace its own icon, put it in a subfolder
named after the prefix:

- `rdf/Property.svg` — only `http://www.w3.org/1999/02/22-rdf-syntax-ns#Property`
- `Property.svg` — every other `…Property`

The folder name must be a prefix declared in the global `[rdf] prefixes` of
`visoto.config` (endpoint-level overrides are ignored); an undeclared one fails
startup. A namespaced icon beats a root icon, but a `.fallback` one still loses
to any exact icon. Folders starting with `_` (e.g. `_sheet/`) are ignored.

## Fallback Icons

Some RDF classes (e.g. `DefinedTermSet`, `Version`) are very generic and apply to
a large proportion of resources in the LINDAS triple store. Their icons are marked
as fallbacks so that more specific class icons take precedence.

To deprioritize an icon, rename it from `Name.svg` to `Name.fallback.svg`.
