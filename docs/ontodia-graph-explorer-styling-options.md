# Graph Explorer / Ontodia - Styling & Customization Options

Comprehensive guide to styling and customization capabilities in Graph Explorer (zazuko/graph-explorer fork of Ontodia).

## 1. JavaScript/TypeScript Customization API

### 1.1 WorkspaceProps - Top-Level Customization

Located in: `src/graph-explorer/workspace/workspace.ts`

```typescript
interface WorkspaceProps {
  // Style Resolvers
  typeStyleResolver?: TypeStyleResolver;           // Element styling by type
  linkTemplateResolver?: LinkTemplateResolver;     // Link styling/templates
  elementTemplateResolver?: TemplateResolver;      // Custom element templates
  selectLabelLanguage?: LabelLanguageSelector;     // Label language selection

  // View Options
  viewOptions?: DiagramViewOptions;
  zoomOptions?: ZoomOptions;

  // UI Configuration
  hideScrollBars?: boolean;        // @default false
  hidePanels?: boolean;            // @default false
  hideToolbar?: boolean;           // @default false
  hideHalo?: boolean;              // @default false
  hideNavigator?: boolean;         // @default false
  hideTutorial?: boolean;          // @default true
  collapseNavigator?: boolean;     // @default false
  leftPanelInitiallyOpen?: boolean;   // @default true
  rightPanelInitiallyOpen?: boolean;  // @default false

  // Custom Components
  toolbar?: ReactElement<any>;     // Custom toolbar

  // Language Support
  languages?: readonly WorkspaceLanguage[];
  language?: string;
  onLanguageChange?: (language: string) => void;
}
```

### 1.2 TypeStyleResolver - Element Appearance by Type

Located in: `src/graph-explorer/customization/props.ts`

```typescript
type TypeStyleResolver = (types: string[]) => CustomTypeStyle | undefined;

interface CustomTypeStyle {
  color?: string;    // Element color (hex, rgb, etc.)
  icon?: string;     // Icon URL or data URI
}
```

**Example:**
```javascript
function myTypeStyleResolver(types) {
  // Detect RDF/OWL class types
  if (types.includes('http://www.w3.org/2000/01/rdf-schema#Class') ||
      types.includes('http://www.w3.org/2002/07/owl#Class')) {
    return {
      color: '#ff6b6b',
      icon: 'path/to/class-icon.svg'
    };
  }

  // Datatype properties
  if (types.includes('http://www.w3.org/2002/07/owl#DatatypeProperty')) {
    return {
      color: '#046380'
    };
  }

  return undefined; // Use default
}
```

### 1.3 LinkTemplateResolver - Link Appearance

Located in: `src/graph-explorer/customization/props.ts`

```typescript
type LinkTemplateResolver = (linkType: string) => LinkTemplate | undefined;

interface LinkTemplate {
  markerSource?: LinkMarkerStyle;
  markerTarget?: LinkMarkerStyle;
  renderLink?: (link: Link) => LinkStyle;
  setLinkLabel?: (link: Link, label: string) => void;
}

interface LinkMarkerStyle {
  fill?: string;           // Arrow fill color
  stroke?: string;         // Arrow stroke color
  strokeWidth?: string;
  d?: string;              // SVG path data for custom arrow shape
  width?: number;
  height?: number;
}

interface LinkStyle {
  connection?: {
    fill?: string;
    stroke?: string;
    'stroke-width'?: number;
    'stroke-dasharray'?: string;
  };
  label?: LinkLabel;
  properties?: LinkLabel[];
  connector?: { name?: string; args?: {} };
}

interface LinkLabel {
  position?: number;       // 0.0 to 1.0 along the link
  title?: string;
  attrs?: {
    rect?: {
      fill?: string;
      stroke?: string;
      'stroke-width'?: number;
    };
    text?: {
      fill?: string;
      stroke?: string;
      'stroke-width'?: number;
      'font-family'?: string;
      'font-size'?: string | number;
      'font-weight'?: 'normal' | 'bold' | 'lighter' | 'bolder' | number;
      text?: LocalizedString[];
    };
  };
}
```

**Example:**
```javascript
function myLinkTemplateResolver(linkType) {
  if (linkType === 'http://www.w3.org/2000/01/rdf-schema#subClassOf') {
    return {
      markerTarget: {
        fill: 'white',
        stroke: '#333',
        strokeWidth: '2',
        d: 'M0,0 L0,8 L9,4 z',  // Custom arrow shape
        width: 9,
        height: 8
      },
      renderLink: (link) => ({
        connection: {
          stroke: '#333',
          'stroke-width': 2,
          'stroke-dasharray': '5,5'  // Dashed line
        },
        connector: { name: 'rounded' }
      })
    };
  }
  return undefined;
}
```

### 1.4 TemplateResolver - Custom Element Templates

Located in: `src/graph-explorer/customization/props.ts`

```typescript
type TemplateResolver = (types: string[]) => ElementTemplate | undefined;
type ElementTemplate = ComponentClass<TemplateProps>;

interface TemplateProps {
  elementId: string;
  data: ElementModel;
  iri: ElementIri;
  types: string;
  label: string;
  color: any;
  iconUrl: string;
  imgUrl?: string;
  isExpanded?: boolean;
  propsAsList?: PropArray;
  props?: Dictionary<Property>;
}
```

You can create a React component that renders elements completely custom:

```javascript
class MyCustomTemplate extends React.Component {
  render() {
    const { label, color, iconUrl, isExpanded } = this.props;
    return (
      <div style={{ border: `2px solid ${color}` }}>
        <img src={iconUrl} />
        <h3>{label}</h3>
        {isExpanded && <div>{/* expanded content */}</div>}
      </div>
    );
  }
}

function myTemplateResolver(types) {
  if (types.includes('http://example.org/CustomType')) {
    return MyCustomTemplate;
  }
  return undefined;
}
```

### 1.5 DiagramViewOptions

```typescript
interface DiagramViewOptions {
  linkRouter?: LinkRouter;                      // Custom link routing
  onIriClick?: IriClickHandler;                 // Handle IRI clicks
  groupBy?: GroupBy[];                          // Element grouping
  disableDefaultHalo?: boolean;                 // Disable selection halo
  suggestProperties?: PropertySuggestionHandler; // Property suggestions
}
```

### 1.6 Selection Halo Configuration

The **halo** is the interactive overlay that appears when you select an element, showing action buttons positioned around it.

**How to disable the halo:**
```javascript
// Option 1: Via WorkspaceProps
const workspace = React.createElement(GE.Workspace, {
  hideHalo: true
});

// Option 2: Via DiagramViewOptions
const workspace = React.createElement(GE.Workspace, {
  viewOptions: {
    disableDefaultHalo: true
  }
});
```

**Halo buttons and what controls them:**

The halo displays different buttons based on props passed internally. While you cannot customize individual buttons via public API, the buttons that appear depend on the context:

| Button | Position | Function | When Shown |
|--------|----------|----------|------------|
| **Remove/Delete** | North-East | Remove from diagram / Delete new element | Always (when halo enabled) |
| **Navigate** | East | Open connections menu | Always |
| **Follow Link** | West | Jump to resource | When `onIriClick` handler provided |
| **Expand/Collapse** | South | Show/hide properties | Always |
| **Add to Filter** | South-East | Search for connected elements | Always |
| **Establish Connection** | South-West | Create new link | Authoring mode only |
| **Revert** | North | Undo changes | Authoring mode only |

**Customization approach:**
- **Cannot** customize which buttons appear via props (hardcoded based on mode/handlers)
- **Can** hide the entire halo with `hideHalo: true`
- **Can** style buttons via CSS (see section 2.4 below)
- **Can** disable individual buttons visually via CSS `display: none`

### 1.7 Element State & Pinned Properties

Elements can store custom state that persists in saved diagrams:

```typescript
// Get/set element state
element.setElementState({
  'http://graph-explorer.org/schema#pinnedProperties': {
    'http://schema.org/name': true,
    'http://schema.org/description': true
  }
});

// Properties with true value will show even when element is collapsed
```

## 2. CSS/SCSS Customization

Graph Explorer uses BEM-style SCSS with the prefix `graph-explorer-`.

### 2.1 Standard Element Template CSS Classes

Base class: `graph-explorer-standard-template`

Located in: `styles/templates/_standard.scss`

```scss
.graph-explorer-standard-template {
  min-width: 180px;
  max-width: 400px;

  &__main { }                    // Main container
  &__body { }                    // Body container
  &__body-horizontal { }         // Horizontal layout
  &__body-content { }            // Content area

  &__thumbnail { }               // Icon/image area (50x50px)
  &__thumbnail-image { }         // Full thumbnail image
  &__thumbnail-icon { }          // Icon (max 26x26px)

  &__label { }                   // Element label (19px font)
  &__type { }                    // Type label (11px italic gray)
  &__type-value { }              // Type text

  &__iri { }                     // IRI display area
  &__iri-key { }                 // "IRI:" label
  &__iri-value { }               // IRI link (12px gray)

  &__photo { }                   // Large photo area (when expanded)
  &__photo-image { }             // Full-width photo

  &__properties { }              // Properties list (max-height: 200px)
  &__properties-row { }          // Property row
  &__properties-key { }          // Property name (50% width)
  &__properties-values { }       // Property values (50% width)
  &__properties-value { }        // Individual value

  &__pinned-props { }            // Pinned properties (shown when collapsed)

  &__dropdown { }                // Expanded content area
  &__dropdown-content { }        // Expanded content padding

  &__hr { }                      // Horizontal rule
  &__actions { }                 // Edit/Delete buttons
}
```

**Customization via CSS Override:**
```css
/* Larger element labels */
.graph-explorer-standard-template__label {
  font-size: 22px;
  font-weight: bold;
}

/* Different thumbnail size */
.graph-explorer-standard-template__thumbnail {
  width: 70px;
  height: 70px;
  font-size: 32px;
}

/* Custom colors for type labels */
.graph-explorer-standard-template__type {
  color: #046380;
  font-weight: 600;
}
```

### 2.2 Element Layer CSS Classes

Located in: `styles/diagram/_elementLayer.scss`

```scss
.graph-explorer-overlayed-element {
  cursor: move;
  // Default inherited properties:
  font-family: "Helvetica Neue", Helvetica, Arial, sans-serif;
  font-size: 14px;
  line-height: 1.42857143;
  color: black;
}

.graph-explorer-overlayed-element--blurred {
  filter: grayscale(100%);
  opacity: 0.5;
}

.graph-explorer-exported-element {
  // Used for SVG/PNG export
}
```

### 2.3 Link Layer CSS Classes

Located in: `styles/diagram/_linkLayer.scss`

```scss
.graph-explorer-link {
  &__wrap { }           // Invisible hit area (stroke-width: 12px)
  &__vertex { }         // Link vertex points
  &__vertex-tools { }   // Vertex manipulation tools

  &--blurred { }        // Blurred/inactive link
}

/* Hover effects */
.graph-explorer-link:hover .graph-explorer-link__wrap {
  stroke: rgba(140, 140, 140, 0.44);
}
```

**Customization examples:**
```css
/* Thicker link hover area */
.graph-explorer-link__wrap {
  stroke-width: 20px;
}

/* Different hover color */
.graph-explorer-link:hover .graph-explorer-link__wrap {
  stroke: rgba(52, 152, 219, 0.6);
}

/* Always show vertices */
.graph-explorer-link__vertex {
  fill: #3498db;
}
```

### 2.4 Selection Halo CSS Classes

Located in: `styles/widgets/_halo.scss`

The halo is the interactive overlay shown when an element is selected, with buttons positioned around the element.

```scss
.graph-explorer-halo {
  position: absolute;
  pointer-events: none;
  border: 1.5px dashed #d8956d;      // Orange dashed border
  border-radius: 2px;
  box-shadow: 0 0 5px 0 #d8956d inset;

  // Action buttons (20x20px each):
  &__navigate { }        // East - Open connections menu
  &__follow { }          // West - Follow link
  &__remove { }          // North-East - Delete element
  &__expand { }          // South - Expand/collapse properties
  &__add-to-filter { }   // South-East - Add to filter
  &__revert { }          // North - Revert changes (authoring mode)
  &__establish-connection { } // South-West - Create new link (authoring mode)
}
```

**Button positioning mixins:**
- `@include n-docked` - Top center
- `@include ne-docked` - Top right
- `@include e-docked` - Right center
- `@include se-docked` - Bottom right
- `@include s-docked` - Bottom center
- `@include sw-docked` - Bottom left
- `@include w-docked` - Left center
- `@include nw-docked` - Top left

**Customization examples:**
```css
/* Change halo border color and style */
.graph-explorer-halo {
  border: 2px solid #3498db;
  box-shadow: 0 0 8px 0 rgba(52, 152, 219, 0.5) inset;
}

/* Hide specific buttons */
.graph-explorer-halo__add-to-filter {
  display: none;
}

/* Change button size */
.graph-explorer-halo__remove {
  width: 24px;
  height: 24px;
}

/* Custom button opacity */
.graph-explorer-halo__navigate {
  opacity: 1 !important;
}
```

### 2.5 Link Halo CSS Classes

Located in: `styles/widgets/_haloLink.scss`

Interactive overlay for selected links in authoring mode.

```scss
.graph-explorer-halo-link {
  &__button { }           // Base button style
  &__edit { }             // Edit link button (20x20px circle)
  &__delete { }           // Delete link button (20x20px circle)
  &__edit-label-button { } // Edit label button
  &__spinner { }          // Loading spinner
}
```

**Customization examples:**
```css
/* Change link halo button colors */
.graph-explorer-halo-link__edit,
.graph-explorer-halo-link__delete {
  background-color: #3498db;
}

/* Larger buttons */
.graph-explorer-halo-link__button {
  border-radius: 12px;
  height: 24px;
  width: 24px;
}
```

### 2.6 Other UI Components

**Workspace:**
- `styles/workspace/_workspace.scss`
- `styles/workspace/_toolbar.scss`
- `styles/workspace/_accordion.scss`
- `styles/workspace/_resizableSidebar.scss`

**Widgets:**
- `styles/widgets/_classTree.scss` - Class hierarchy tree
- `styles/widgets/_instancesSearch.scss` - Search panel
- `styles/widgets/_connectionsMenu.scss` - Connections menu
- `styles/widgets/_halo.scss` - Selection halo
- `styles/widgets/_haloLink.scss` - Link selection halo
- `styles/widgets/_navigator.scss` - Mini-map navigator
- `styles/widgets/_searchResults.scss` - Search results

**Editor (Authoring Mode):**
- `styles/editor/_loadingWidget.scss`
- `styles/editor/_authoringState.scss`
- `styles/widgets/_authoringTools.scss`
- `styles/widgets/_editForm.scss`

### 2.5 Main SCSS Entry Point

Located in: `styles/main.scss`

Imports all component styles. To create custom build:
```scss
@use "viewUtils/spinner";
@use "diagram/elementLayer";
@use "diagram/linkLayer";
@use "templates/standard";
// ... etc
```

## 3. Complete Styling Workflow

### Option A: CSS Overrides (Simplest)

Create a CSS file loaded after Graph Explorer:

```html
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/graph-explorer@1.3.0/dist/graph-explorer.css">
<link rel="stylesheet" href="/static/css/ontodia_overrides.css">
```

```css
/* /static/css/ontodia_overrides.css */
.graph-explorer-standard-template__label {
  font-size: 20px;
  color: #2c3e50;
}
```

### Option B: JavaScript Resolvers (Most Flexible)

```javascript
// Type-based styling
const typeStyleResolver = (types) => {
  if (types.includes('http://schema.org/Person')) {
    return { color: '#3498db', icon: 'icons/person.svg' };
  }
  return undefined;
};

// Link styling
const linkTemplateResolver = (linkType) => {
  if (linkType.includes('subClassOf')) {
    return {
      markerTarget: { fill: 'white', stroke: '#333' },
      renderLink: () => ({
        connection: { 'stroke-dasharray': '5,5' }
      })
    };
  }
  return undefined;
};

// Apply to workspace
const workspace = React.createElement(GE.Workspace, {
  typeStyleResolver: typeStyleResolver,
  linkTemplateResolver: linkTemplateResolver,
  // ... other props
});
```

### Option C: Custom Templates (Most Control)

Create custom React components for specific types:

```javascript
class PersonTemplate extends React.Component {
  render() {
    const { label, color, props } = this.props;
    const email = getProperty(props, 'http://schema.org/email');

    return (
      <div className="custom-person-template" style={{ borderColor: color }}>
        <div className="person-avatar">
          <img src={this.props.imgUrl || 'default-avatar.png'} />
        </div>
        <h3>{label}</h3>
        {email && <a href={`mailto:${email}`}>{email}</a>}
      </div>
    );
  }
}

const elementTemplateResolver = (types) => {
  if (types.includes('http://schema.org/Person')) {
    return PersonTemplate;
  }
  return undefined;
};
```

## 4. Examples & References

### Official Examples
- **Repository**: https://github.com/zazuko/graph-explorer
- **Examples Directory**: https://github.com/metaphacts/ontodia/tree/master/examples
  - `wikidata.ts` - External endpoint configuration
  - `dbpedia.ts` - Another endpoint example
  - `edit.ts` - Authoring mode example

### Production Reference
**LINDAS Graph Explorer** (Swiss Federal Government):
- URL: https://lindas.admin.ch/graph-explorer/
- Same SPARQL endpoint as used by Visoto
- Good example of clean, professional styling

### Source Code References
- **Main API**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/index.ts
- **Workspace Props**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/workspace/workspace.ts
- **Customization Props**: https://github.com/metaphacts/ontodia/blob/master/src/ontodia/customization/props.ts
- **Standard Template**: Source in `src/graph-explorer/customization/templates/standard.tsx`
- **CHANGELOG**: https://github.com/metaphacts/ontodia/blob/master/CHANGELOG.md

## 5. Key Customization Features by Version

From CHANGELOG analysis:

**v0.10.0:**
- Pinned properties (show properties even when collapsed)
- Custom element state in serialized diagrams
- Enhanced label language selection

**v0.9.8:**
- Custom link state in serialized layouts
- Link renaming using custom templates
- Element highlighting via `DiagramView.setHighlighter()`

**v0.9.6:**
- Unified style resolver props (breaking change from multiple resolvers)

## 6. CSS Variables & Theming

Graph Explorer doesn't use CSS custom properties by default, but you can layer them on top:

```css
:root {
  --ge-primary-color: #3498db;
  --ge-text-color: #2c3e50;
  --ge-font-size-base: 14px;
  --ge-element-border-radius: 4px;
}

.graph-explorer-standard-template__label {
  color: var(--ge-text-color);
  font-size: calc(var(--ge-font-size-base) * 1.35);
}

.graph-explorer-standard-template__main {
  border-radius: var(--ge-element-border-radius);
}
```

## 7. Best Practices

1. **Prefer JavaScript resolvers over CSS** for type-specific styling (more maintainable)
2. **Use CSS overrides** for global visual changes (fonts, spacing, colors)
3. **Custom templates** only when standard template can't accommodate your needs
4. **Test with SVG export** - styles need to export cleanly
5. **Keep element templates performant** - they're rendered for every element
6. **Use BEM naming** when adding custom CSS to avoid conflicts
7. **Leverage element state** for persistent UI state across saves

## 8. Common Customization Tasks

### Change font globally
```css
.graph-explorer-overlayed-element {
  font-family: 'Inter', sans-serif;
}
```

### Color elements by namespace
```javascript
const typeStyleResolver = (types) => {
  const firstType = types[0] || '';
  if (firstType.startsWith('http://schema.org/')) return { color: '#3498db' };
  if (firstType.startsWith('http://xmlns.com/foaf/')) return { color: '#e74c3c' };
  return undefined;
};
```

### Hide specific UI elements
```javascript
const workspace = React.createElement(GE.Workspace, {
  hideNavigator: true,
  hideToolbar: false,
  leftPanelInitiallyOpen: false
});
```

### Custom link arrows
```javascript
const linkTemplateResolver = (linkType) => ({
  markerTarget: {
    d: 'M0,0 L0,10 L10,5 z',
    fill: '#333',
    width: 10,
    height: 10
  }
});
```
