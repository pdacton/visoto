/*
  Mermaid + ELK setup, extracted from an inline <script type="module"> in
  templates/layout/base.html. It stays a module because it imports Mermaid and
  the ELK layout package straight from a CDN as ES modules.

  Besides initialising Mermaid it watches data-bs-theme and re-renders every
  diagram registered in window.mermaidDiagramRenderers when the theme flips —
  that registry is populated by static/js/sparql-mermaid-flow.js.
*/
// Import Mermaid and ELK layout as ES modules.
// Pinned exactly rather than @11 / @0: a range leaves the deployed version as
// whatever the CDN served that day. SRI is not available here — integrity
// cannot be expressed on a bare ES module import — so the pin is the only
// thing fixing what actually runs.
import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@12.0.0/dist/mermaid.esm.min.mjs';
import elkLayouts from 'https://cdn.jsdelivr.net/npm/@mermaid-js/layout-elk@1.0.0/dist/mermaid-layout-elk.esm.min.mjs';

// Register ELK layout loaders.
// Mermaid 12 bundles ELK, so this call is no longer strictly required — it is
// kept because it still works and keeps the layout package explicit.
mermaid.registerLayoutLoaders(elkLayouts);

// Make mermaid available globally for other scripts
window.mermaid = mermaid;

// Initialize Mermaid with dark mode support and ELK as default renderer
(function() {
  var isDark = document.documentElement.getAttribute('data-bs-theme') === 'dark';
  mermaid.initialize({
    startOnLoad: false,
    theme: isDark ? 'dark' : 'default',
    look: 'classic',
    layout: 'elk',
    securityLevel: 'loose',
    flowchart: {
      curve: 'linear',
      htmlLabels: true
    },
    elk: {
      mergeEdges: false,
      nodePlacementStrategy: 'LINEAR_SEGMENTS'
    }
  });

  // Re-initialize on theme change and re-render existing diagrams
  var observer = new MutationObserver(function() {
    var isDark = document.documentElement.getAttribute('data-bs-theme') === 'dark';

    // Update Mermaid configuration with new theme
    mermaid.initialize({
      startOnLoad: false,
      theme: isDark ? 'dark' : 'default',
      look: 'classic',
      layout: 'elk',
      securityLevel: 'loose',
      flowchart: {
        curve: 'linear',
        htmlLabels: true,
      },
      elk: {
        mergeEdges: false,
        nodePlacementStrategy: 'LINEAR_SEGMENTS'
      }
    });

    // Re-render all registered Mermaid diagrams with new theme
    if (window.mermaidDiagramRenderers) {
      Object.values(window.mermaidDiagramRenderers).forEach(function(renderer) {
        try {
          renderer.render();
        } catch (err) {
          console.error('Error re-rendering Mermaid diagram:', renderer.id, err);
        }
      });
    }
  });
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-bs-theme']
  });
})();
