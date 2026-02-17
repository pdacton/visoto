---
name: graph-explorer-customization
description: Reference guide for customizing Graph Explorer (Ontodia fork) - source files, CSS classes, API, and customization patterns
---

# Graph Explorer Customization Skill

Reference guide for customizing the Graph Explorer library (a fork of Ontodia) used for RDF graph visualization in Visoto.

## Library Information
- **Library**: [Graph Explorer](https://github.com/zazuko/graph-explorer) (fork of Ontodia)
- **CDN**: `graph-explorer@1.3.0`
- **CSS Prefix**: All classes use `graph-explorer-` prefix

## Local Project Files
- `templates/pages/ontodia.html` - Main Graph Explorer page with initialization code
- `static/css/ontodia_overrides.css` - Custom CSS overrides

## Key Source Files Reference

### Widgets (UI Components)
| File | Description |
|------|-------------|
| [_halo.scss](https://github.com/zazuko/graph-explorer/blob/master/styles/widgets/_halo.scss) | Halo selection styling - border (`#d8956d`), shadow, button positions (N/NE/E/SE/S/SW/W/NW docking) |
| [_haloLink.scss](https://github.com/zazuko/graph-explorer/blob/master/styles/widgets/_haloLink.scss) | Link halo styling |
| [halo.tsx](https://github.com/zazuko/graph-explorer/blob/master/src/graph-explorer/widgets/halo.tsx) | Halo React component - buttons: remove, expand, navigate, follow, add-to-filter |
| [_classTree.scss](https://github.com/zazuko/graph-explorer/blob/master/styles/widgets/_classTree.scss) | Left panel class tree |
| [_instancesSearch.scss](https://github.com/zazuko/graph-explorer/blob/master/styles/widgets/_instancesSearch.scss) | Instance search panel |
| [_connectionsMenu.scss](https://github.com/zazuko/graph-explorer/blob/master/styles/widgets/_connectionsMenu.scss) | Connections popup menu |

### Workspace & Toolbar
| File | Description |
|------|-------------|
| [toolbar.tsx](https://github.com/zazuko/graph-explorer/blob/master/src/graph-explorer/workspace/toolbar.tsx) | Canvas toolbar - Clear All, Layout, Zoom In/Out, Fit, PNG/SVG Export, Print, Language |
| [workspaceMarkup.tsx](https://github.com/zazuko/graph-explorer/blob/master/src/graph-explorer/workspace/workspaceMarkup.tsx) | Main workspace layout with panels |
| [styles/workspace/](https://github.com/zazuko/graph-explorer/tree/master/styles/workspace) | Workspace styling directory |

### Editor & Core
| File | Description |
|------|-------------|
| [editorController.tsx](https://github.com/zazuko/graph-explorer/blob/master/src/graph-explorer/editor/editorController.tsx) | Main editor logic, renders halo via `renderDefaultHalo()` |
| [paperArea.tsx](https://github.com/zazuko/graph-explorer/blob/master/src/graph-explorer/diagram/paperArea.tsx) | Canvas/paper area - pan, zoom, pointer events |

### Templates (Node Rendering)
| File | Description |
|------|-------------|
| [styles/templates/](https://github.com/zazuko/graph-explorer/tree/master/styles/templates) | Node template styles |

### Data Providers
| File | Description |
|------|-------------|
| [data/sparql/](https://github.com/zazuko/graph-explorer/tree/master/src/graph-explorer/data/sparql) | SPARQL data provider implementation |

## CSS Class Reference

### Halo (Selection Overlay)
```css
.graph-explorer-halo                    /* Main halo container */
.graph-explorer-halo__navigate          /* East - connections button */
.graph-explorer-halo__folow             /* West - follow/link button */
.graph-explorer-halo__remove            /* Northeast - delete button */
.graph-explorer-halo__expand            /* South - expand properties */
.graph-explorer-halo__add-to-filter     /* Southeast - filter button */
.graph-explorer-halo__revert            /* North - undo button */
.graph-explorer-halo__establish-connection  /* Southwest - new connection */
```

### Node Templates
```css
.graph-explorer-standard-template__main   /* Node main container */
.graph-explorer-standard-template__body   /* Node body (has left accent border) */
.graph-explorer-overlayed-element         /* Element overlay wrapper */
```

### Workspace
```css
.graph-explorer-toolbar                   /* Canvas toolbar */
.graph-explorer-class-tree                /* Class tree panel */
.graph-explorer-instances-search          /* Instance search panel */
```

## Common Customization Patterns

### Override Halo Styling
```css
.graph-explorer-halo {
  border: none !important;
  box-shadow: 0 0.5rem 1rem rgba(0, 0, 0, 0.15) !important;
}
```

### Hide Specific Halo Buttons
```css
.graph-explorer-halo__add-to-filter { display: none !important; }
```

### Override Halo Button Icons
```css
.graph-explorer-halo__navigate {
  background-image: url('/static/img/custom-icon.svg') !important;
}
```

### Override Node Template Styling
```css
.graph-explorer-standard-template__main {
  border-color: #dee2e6 !important;
  background-color: #f8f9fa !important;
}
```

## Workspace API

### Key Methods
```javascript
workspace.getModel()                    // Get diagram model
workspace.forceLayout()                 // Apply force-directed layout
workspace.zoomToFit()                   // Fit diagram to viewport
workspace.exportSvg()                   // Export as SVG
workspace.exportPng()                   // Export as PNG
```

### Model Methods
```javascript
model.elements                          // Array of all elements
model.links                             // Array of all links
model.createElement(iri)                // Create element from IRI (does NOT fetch data)
model.removeElement(elementId)          // Remove element by ID
model.exportLayout()                    // Export current layout
model.importLayout(layoutData)          // Import layout
model.requestElementData([iri])         // Fetch data - pass IRI STRINGS, not Element objects
model.requestLinksOfType({ elementId }) // Request connections
```

### Loading an Element Programmatically
```javascript
// createElement only creates a placeholder - must call requestElementData to fetch labels/types
var element = model.createElement(startIRI);
element.setPosition({ x: 400, y: 300 });
model.requestElementData([startIRI]);  // MUST pass string IRI, not element object
```

### Element Properties
```javascript
element.id                              // Internal element ID
element.iri                             // Resource IRI
element.position                        // { x, y } position
element.setPosition({ x, y })           // Set position
```

## Data Provider Configuration

### SPARQL Provider for LINDAS
```javascript
var settings = Object.assign({}, GE.OWLStatsSettings, {
  // LINDAS uses schema:name, not rdfs:label — without this, labels will be empty
  dataLabelProperty: '<http://schema.org/name> | rdfs:label',
  schemaLabelProperty: '<http://schema.org/name> | rdfs:label',
});

var dataProvider = new GE.SparqlDataProvider({
  endpointUrl: 'https://ld.admin.ch/query/',
  acceptBlankNodes: false,
  queryMethod: GE.SparqlQueryMethod.GET,
}, settings);
```

### Available Settings Objects
- `GE.OWLStatsSettings` - Used by LINDAS production; includes class instance counts
- `GE.OWLRDFSSettings` - For large datasets without statistics

### Known Issues with LINDAS Data
- **Labels empty**: `OWLStatsSettings.dataLabelProperty` defaults to `rdfs:label` only. LINDAS entities use `schema:name` — must override `dataLabelProperty`.
- **`startsWith` crash**: Some LINDAS entities have `schema:identifier` as bare integers (e.g. `261`). The library's `isEncodedBlank()` calls `.startsWith()` on these non-string values. Fix by overriding `elementInfoQuery` to filter: `FILTER (isLiteral(?propValue) && datatype(?propValue) IN (xsd:string, rdf:langString))`.

## Toolbar Props (for custom toolbar)
```typescript
interface ToolbarProps {
  onForceLayout?: () => void;
  onClearAll?: () => void;
  onZoomIn?: () => void;
  onZoomOut?: () => void;
  onZoomToFit?: () => void;
  onExportSVG?: (fileName?: string) => void;
  onExportPNG?: (fileName?: string) => void;
  onPrint?: () => void;
  languages?: readonly WorkspaceLanguage[];
  selectedLanguage?: string;
  onChangeLanguage?: (language: string) => void;
}
```

## Adding Custom Functionality

### Context Menu (Right-Click)
Graph Explorer doesn't have built-in context menu support. Implement by:
1. Listen for `contextmenu` event on container
2. Walk DOM to find `.graph-explorer-overlayed-element`
3. Get element ID from `data-element-id` attribute
4. Find element in model by ID to get IRI
5. Show custom menu and wire actions to model methods

### Custom Toolbar Buttons
1. Create HTML buttons outside Graph Explorer container
2. Wire `onclick` to workspace/model methods
3. Use `workspaceRef` reference captured in `onWorkspaceMounted`
