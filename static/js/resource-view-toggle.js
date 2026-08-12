/* Resource page Table <-> Graph <-> Data <-> Schema view toggle.
 *
 * Wiring (see templates/layout/header.html and templates/layout/base.html):
 *   - Header renders a Tabler btn-check radio group with inputs
 *     [data-view="table"|"graph"|"data"|"schema"]. Bootstrap styles the checked
 *     radio's label as active for free.
 *   - Body wraps the table content in #resource-table-view, the graph in
 *     #resource-graph-view, the data table in #resource-data-view, and the
 *     schema diagram and tables in #resource-schema-view (all but the table view
 *     start .d-none).
 *   - The graph partial (templates/partials/sparql-graph.html) is rendered with
 *     lazy=true and initialized on a 'graph:init' event at its -root element.
 *   - The data view (templates/partials/sparql-async-table.html) is an HTMX
 *     element with hx-trigger="showData"; the query runs only when we fire that
 *     event on first reveal.
 *   - The schema view holds both kinds: the diagram partial
 *     (templates/partials/schema-graph.html) is rendered with lazy=true and
 *     initialized on a 'schema:init' event at its -root element, and FOUR async
 *     tables below it wait on hx-trigger="showSchema".
 *
 * Switching only shows/hides the wrappers — neither the graph instances nor the
 * loaded data table is destroyed, so state (dragged nodes, zoom, table grouping)
 * survives repeated toggling. Modeled on bindRangeButtons in monitoring.js.
 *
 * The active view is mirrored in the URL hash (#Graph, #Data, #Schema; no hash
 * for the default Table view) so links can address a specific view. The hash is
 * read case-insensitively and written via history.replaceState so toggling
 * doesn't pile up history entries or trigger an anchor scroll.
 */
(function () {
  'use strict';

  function init() {
    var inputs = document.querySelectorAll('.btn-check[data-view]');
    var tableView = document.getElementById('resource-table-view');
    var graphView = document.getElementById('resource-graph-view');
    var dataView = document.getElementById('resource-data-view');
    var schemaView = document.getElementById('resource-schema-view');
    if (!inputs.length || !tableView) return; // not a resource page

    var graphInitialized = false;
    var dataInitialized = false;
    var schemaInitialized = false;

    function hideAll() {
      tableView.classList.add('d-none');
      if (graphView) graphView.classList.add('d-none');
      if (dataView) dataView.classList.add('d-none');
      if (schemaView) schemaView.classList.add('d-none');
    }

    function showTable() {
      hideAll();
      tableView.classList.remove('d-none');
    }

    function showGraph() {
      if (!graphView) return;
      hideAll();
      // Reveal before initializing so Graph Explorer measures a non-zero size.
      graphView.classList.remove('d-none');
      if (!graphInitialized) {
        graphInitialized = true;
        // The graph partial defaults its id to "sparql-graph" (base.html passes none).
        var root = document.getElementById('sparql-graph-root');
        if (root) root.dispatchEvent(new Event('graph:init'));
      }
    }

    function showData() {
      if (!dataView) return;
      hideAll();
      dataView.classList.remove('d-none');
      if (!dataInitialized) {
        dataInitialized = true;
        // Fire the HTMX trigger on the placeholder to run the query once.
        var hxEl = dataView.querySelector('[hx-get]');
        if (hxEl && window.htmx) window.htmx.trigger(hxEl, 'showData');
      }
    }

    function showSchema() {
      if (!schemaView) return;
      hideAll();
      // Reveal before initializing so Graph Explorer measures a non-zero size.
      schemaView.classList.remove('d-none');
      if (!schemaInitialized) {
        schemaInitialized = true;
        // The schema partial defaults its id to "schema-graph" (base.html passes none).
        // Dispatched before the tables so the expensive Graph Explorer boot starts
        // ahead of four fragment requests.
        var root = document.getElementById('schema-graph-root');
        if (root) root.dispatchEvent(new Event('schema:init'));
        // Unlike the Data view's single table, this view holds FOUR async tables
        // (sub/superclasses, ontology, SHACL) — querySelectorAll, not the singular
        // querySelector used above, or only the first would ever leave its skeleton.
        if (window.htmx) {
          schemaView.querySelectorAll('[hx-get]').forEach(function (el) {
            window.htmx.trigger(el, 'showSchema');
          });
        }
      }
    }

    function selectView(view) {
      if (view === 'graph') {
        showGraph();
      } else if (view === 'data') {
        showData();
      } else if (view === 'schema') {
        showSchema();
      } else {
        showTable();
      }
    }

    // Capitalized to match the button labels; table (the default) gets no hash.
    var HASH_BY_VIEW = { graph: '#Graph', data: '#Data', schema: '#Schema' };

    function viewFromHash() {
      var h = location.hash.slice(1).toLowerCase();
      return (h === 'graph' || h === 'data' || h === 'schema' || h === 'table') ? h : null;
    }

    function syncHash(view) {
      var url = HASH_BY_VIEW[view] || location.pathname + location.search;
      history.replaceState(null, '', url);
    }

    function applyView(view) {
      var input = document.querySelector('.btn-check[data-view="' + view + '"]');
      if (input) input.checked = true;
      selectView(view);
    }

    // btn-check radios manage their own active styling; we only react to selection.
    inputs.forEach(function (input) {
      input.addEventListener('change', function () {
        if (!input.checked) return;
        selectView(input.dataset.view);
        syncHash(input.dataset.view);
      });
    });

    // Honor a view named in the URL on load (e.g. a shared link ending #Schema).
    var initialView = viewFromHash();
    if (initialView && initialView !== 'table') applyView(initialView);

    // Manual hash edits / same-page anchor links; our own replaceState calls
    // don't fire hashchange, so this only reacts to external changes.
    window.addEventListener('hashchange', function () {
      var view = viewFromHash();
      if (view) applyView(view);
    });

    // Async table fragments arrive via HTMX; re-run Lucide so the card icon (and
    // any icon cells) render after the swap. htmx:afterSwap bubbles, so one
    // listener per container covers all of that container's tables — the schema
    // view swaps four times. createIcons() only rewrites [data-lucide] nodes, so
    // being called once per swap is harmless.
    [dataView, schemaView].forEach(function (view) {
      if (!view) return;
      view.addEventListener('htmx:afterSwap', function () {
        if (window.lucide) window.lucide.createIcons();
      });
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
