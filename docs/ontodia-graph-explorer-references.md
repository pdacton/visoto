# Graph Explorer / Ontodia - Technical Reference

Quick reference for improving Visoto's Graph Explorer implementation.

## Version Info
- **Visoto uses**: Graph Explorer v1.3.0 (Zazuko fork) via CDN
- **Repository status**: metaphacts/ontodia archived Sept 26, 2024 (read-only)
- **Active fork**: zazuko/graph-explorer

## Key Documentation Links

### Essential References
- **CHANGELOG** (most complete option listing): https://github.com/metaphacts/ontodia/blob/master/CHANGELOG.md
- **SPARQL Settings Source**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/data/sparql/sparqlDataProviderSettings.ts
- **SPARQL Provider Source**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/data/sparql/sparqlDataProvider.ts
- **Main API Exports**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/index.ts
- **WorkspaceProps**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/workspace/workspace.ts
- **Customization Props**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/customization/props.ts

### Examples & Tutorials
- **16 Working Examples**: https://github.com/metaphacts/ontodia/tree/master/examples
  - `sparqlNoStats.ts` - Large datasets without statistics (like OWLRDFSSettings)
  - `styleCustomization.ts` - Custom styling
  - `toolbarCustomization.tsx` - Custom toolbar
  - `wikidata.ts` / `dbpedia.ts` - External endpoint examples
- **Wiki Tutorials**: https://github.com/metaphacts/ontodia/wiki
- **TypeScript Example Repo**: https://github.com/ontodia-org/ontodia-typescript-example

### Production Reference
**LINDAS Graph Explorer** (Swiss Federal Government):
- URL: https://lindas.admin.ch/graph-explorer/
- Same endpoint as Visoto: `https://lindas.admin.ch/query`
- Config: `acceptBlankNodes: false`, languages: [en, de, fr, it]
- Labels: `schema:name | rdfs:label`

## Configuration Reference

### Current Visoto Implementation
```javascript
// From /templates/pages/ontodia.html
var settings = Object.assign({}, GE.OWLRDFSSettings, {
  dataLabelProperty: '<http://schema.org/name> | rdfs:label',
  schemaLabelProperty: '<http://schema.org/name> | rdfs:label'
});

var dataProvider = new GE.SparqlDataProvider({
  endpointUrl: 'https://ld.admin.ch/query/',
  queryMethod: GE.SparqlQueryMethod.POST,
}, settings);
```

### SparqlDataProvider Options
**Constructor Options:**
- `endpointUrl` - SPARQL endpoint URL (required)
- `queryMethod` - `SparqlQueryMethod.GET` or `.POST`
- `acceptBlankNodes` - Boolean (default: depends on settings)
- `imagePropertyUris` - Array of property URIs for images
- `queryFunction` - Custom SPARQL request handler
- `prepareLabels` - External label fetching function

### SparqlDataProviderSettings
**Label Configuration:**
- `dataLabelProperty` - SPARQL property path for instance labels
- `schemaLabelProperty` - SPARQL property path for class/property labels

**SPARQL Query Overrides** (all optional):
- `classTreeQuery` - Class hierarchy
- `classInfoQuery` - Class details
- `propertyInfoQuery` - Property details
- `linkTypesQuery` - Available link types
- `elementInfoQuery` - Element details
- `linksInfoQuery` - Links for elements
- `imageQuery` - Element images
- `linkTypesOfQuery` - Link types for element
- `linkTypesStatisticsQuery` - Link statistics
- `filterRefElementLinkQuery` - Reference link filter

**Filter Patterns:**
- `filterTypePattern` - Type filtering pattern
- `filterRefElementLinkPattern` - Link filtering pattern
- `filterElementInfoPattern` - Element info filtering
- `filterAdditionalRestriction` - Additional SPARQL filters

**Property Configuration:**
- `propertyConfigurations` - Datatype property patterns
- `defaultPrefix` - Default SPARQL prefixes

### Settings Presets
**OWLStatsSettings** (default):
- For RDFS/OWL datasets with class statistics
- Best for medium datasets with full-text search

**OWLRDFSSettings** (Visoto uses this):
- For RDFS/OWL datasets without class statistics
- For large datasets or without full-text search

**WikidataSettings**:
- Pre-configured for Wikidata endpoint

**DBPediaSettings**:
- Pre-configured for DBPedia endpoint

### WorkspaceProps
**Key Configuration:**
- `ref` - Callback when workspace mounts
- `viewOptions.onIriClick` - Handler for IRI clicks
- `languages` - Language preferences array
- `toolbar` - Custom toolbar configuration
- `hideScrollBars` - Boolean
- `typeStyleResolver` - Element type styling function
- `linkTemplateResolvers` - Link template resolution
- `templatesResolvers` - Custom template resolution

### Exported API (from index.ts)
**Data Models:**
- `ElementModel`, `LinkModel`, `ClassModel`, `PropertyModel`
- `DataProvider`, `SparqlDataProvider`, `RdfDataProvider`
- `MetadataApi`, `ValidationApi`

**Diagram:**
- `DiagramModel`, `Element`, `Link`, `Cell`
- `LayoutData`, `SerializedDiagram`
- `calculateLayout()`, `applyLayout()`, `forceLayout()`

**UI Components:**
- `Workspace`, `WorkspaceProps`, `DefaultToolbar`, `ToolbarProps`

**Editor/Authoring:**
- `EditorController`, `EditorOptions`, `ValidationState`

## Quick Lookup by Use Case

### How to: Configure SPARQL Endpoint
See: [sparqlDataProvider.ts](https://github.com/metaphacts/ontodia/blob/master/src/ontodia/data/sparql/sparqlDataProvider.ts)
Example: [sparql.ts](https://github.com/metaphacts/ontodia/blob/master/examples/sparql.ts)

### How to: Customize Labels
See: [sparqlDataProviderSettings.ts](https://github.com/metaphacts/ontodia/blob/master/src/ontodia/data/sparql/sparqlDataProviderSettings.ts)
- Set `dataLabelProperty` and `schemaLabelProperty`
- Use SPARQL property path syntax: `'<http://schema.org/name> | rdfs:label'`

### How to: Customize Styles
See: [styleCustomization.ts](https://github.com/metaphacts/ontodia/blob/master/examples/styleCustomization.ts)
API: [customization/props.ts](https://github.com/metaphacts/ontodia/blob/master/src/ontodia/customization/props.ts)

### How to: Customize Toolbar
See: [toolbarCustomization.tsx](https://github.com/metaphacts/ontodia/blob/master/examples/toolbarCustomization.tsx)
API: [workspace/toolbar.tsx](https://github.com/metaphacts/ontodia/blob/master/src/ontodia/workspace/toolbar.tsx)

### How to: Add Multi-language Support
Reference: LINDAS production instance (en, de, fr, it)
Set `WorkspaceProps.languages` array

### How to: Save/Load Diagrams
API: `SerializedDiagram`, `LayoutData` from index.ts
Current Visoto implementation: See `/templates/pages/ontodia.html` lines 58-173

### How to: Handle Large Datasets
Use: `OWLRDFSSettings` (no class statistics)
Set: `acceptBlankNodes: false`

## TypeScript Definitions Source
To see all available options:
```bash
npm install graph-explorer
# Check: node_modules/graph-explorer/dist/*.d.ts
```

Or generate TypeDoc:
```bash
git clone https://github.com/zazuko/graph-explorer.git
cd graph-explorer
npm install
npx typedoc --out docs src/index.ts
```

## Repository Structure
Key directories in source:
- `/src/ontodia/data/` - Data providers, models, APIs
- `/src/ontodia/diagram/` - Visualization, rendering, layout
- `/src/ontodia/workspace/` - Workspace UI components
- `/src/ontodia/customization/` - Styling and templates
- `/src/ontodia/editor/` - Editing functionality
- `/examples/` - 16 working examples

## Notes
- No official TypeDoc site published - must generate locally or read source
- CHANGELOG is surprisingly complete for option discovery
- Most wiki docs apply to both metaphacts and zazuko forks
- Production LINDAS instance is best real-world reference
