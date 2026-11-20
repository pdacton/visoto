# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

**Primary: linked-data practitioners.** People who publish, model or maintain RDF, mostly around Swiss government data (LINDAS, I14Y): data stewards, ontology modellers, semantic-web developers, researchers. They open Visoto to inspect a resource, check how a class is used, see what a graph contains, derive a schema, or confirm that their published data looks right. They know RDF; they don't want to write SPARQL for every look.

**Secondary: the curious public and analysts.** People who want to find and read Swiss open data (mobility, politics, energy, culture…) without knowing SPARQL. Theme pages and the home page serve them.

**Also: AI assistants and their users**, through the MCP server (`/mcp`, `/mcp/signin`) and the `/connect` page.

## Product Purpose

Visoto turns any RDF resource behind a SPARQL endpoint into a readable web page. Every page is built live from SPARQL queries embedded in its template; there is no copy of the RDF data. Success: a practitioner pastes an IRI and understands the resource, its class, and its neighbourhood within seconds, and a non-expert can follow links through Swiss linked data without hitting raw triples.

## Positioning

Class-aware readable pages. The resource's `rdf:type` picks a template laid out for that kind of thing (a cube, a vocabulary term, a person, a class), filled from the live endpoint. Generic RDF browsers show a property/value dump; Visoto shows a page designed for the type, with a generic fallback for everything else.

## Operating Context

- Multiple configurable SPARQL endpoints (LINDAS prod/int/test, cached LINDAS by default, a private QLever); the endpoint slug is part of every link.
- Shared links are the unit of collaboration: `/resource?iri=<IRI>&endpoint=<slug>`. Server-rendered pages are cached and identical for every visitor.
- Practitioners move between Visoto, the endpoint's own tools, I14Y and their modelling files; Graph Explorer, schema derivation, faceted tables and the upload/named-graph tools keep them in Visoto longer.
- UI languages: de, en, fr, it, rm. Light and dark mode.

## Capabilities and Constraints

- Resource pages, class pages, instance tables (Tabulator, lazy/async, working-set model for huge classes), faceted search, full-text search, Graph Explorer, schema derivation (SHACL + UML-style diagram), cube catalog, upload and named graphs, endpoint monitoring, theme pages, bookmarks, AI chat, MCP server.
- Endpoint data drifts: counts change between calls, so numbers come from live queries, never hard-coded prose.
- Endpoints are slow and sometimes fail; loading, empty and error states are normal, not edge cases.
- Every user-facing string is translated; text must survive German and French lengths.
- Built on Tabler (Bootstrap 5) and Tabulator; no inline JS in templates.
- **Open core + accounts.** The public core is free and open source (Apache 2.0). Accounts (sign-in, MCP OAuth) live in a separate commercial edition. The core does not upsell; personal UI is client-side only.

## Brand Commitments

- Name: Visoto. Logos: `static/img/visoto_light.svg`, `static/img/visoto_dark.svg`.
- Tagline in use: "Linked data you can browse."
- Voice: technical but approachable, clear and concise, professional yet friendly; error messages guide toward a fix.
- Resource icons: tinted squircle + Lucide glyph per namespace (`static/img/resource/<prefix>/`).

## Evidence on Hand

- Live demo: https://visoto.hutzli.org.
- Screenshots: `docs/screenshot1.png` – `docs/screenshot3.png`.
- Real data from LINDAS endpoints on every page.
- No testimonials, customer names, usage numbers or pricing exist; don't invent them.

## Product Principles

1. **The data is the content.** Pages exist to make one resource understandable; chrome stays out of its way.
2. **Read before query.** A page should answer what a practitioner would otherwise write SPARQL for.
3. **Honest about live data.** Show loading, failure and drift plainly; never fake a number.
4. **Every link is shareable.** A page is a pure function of its URL, the same for everyone.
5. **Experts first, newcomers welcome.** Keep RDF truth visible (IRIs, types, prefixes) without making it a barrier.

## Accessibility & Inclusion

Keyboard and screen-reader support for all features; light and dark themes; five UI languages including Romansh.
