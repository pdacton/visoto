/* Resource page Table <-> Graph <-> Data <-> Schema view toggle.
 *
 * Wiring (see templates/layout/header.html and templates/layout/base.html):
 *   - Header renders a Tabler btn-check radio group with inputs
 *     [data-view="table"|"graph"|"data"|"schema"]. Bootstrap styles the checked
 *     radio's label as active for free.
 *   - Body wraps the table content in #resource-table-view, the graph in
 *     #resource-graph-view, the data table in #resource-data-view, and the
 *     derived schema diagram in #resource-schema-view (all but the table view
 *     start .d-none).
 *   - The graph partial (templates/partials/sparql-graph.html) is rendered with
 *     lazy=true and initialized on a 'graph:init' event at its -root element.
 *   - The data view (templates/partials/sparql-async-table.html) is an HTMX
 *     element with hx-trigger="showData"; the query runs only when we fire that
 *     event on first reveal.
 *   - The schema partial (templates/partials/schema-graph.html) is rendered with
 *     lazy=true and initialized on a 'schema:init' event at its -root element.
 *
 * Switching only shows/hides the wrappers — neither the graph instances nor the
 * loaded data table is destroyed, so state (dragged nodes, zoom, table grouping)
 * survives repeated toggling. Modeled on bindRangeButtons in monitoring.js.
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
        var root = document.getElementById('schema-graph-root');
        if (root) root.dispatchEvent(new Event('schema:init'));
      }
    }

    // btn-check radios manage their own active styling; we only react to selection.
    inputs.forEach(function (input) {
      input.addEventListener('change', function () {
        if (!input.checked) return;
        if (input.dataset.view === 'graph') {
          showGraph();
        } else if (input.dataset.view === 'data') {
          showData();
        } else if (input.dataset.view === 'schema') {
          showSchema();
        } else {
          showTable();
        }
      });
    });

    // The async table fragment arrives via HTMX; re-run Lucide so the card icon
    // (and any icon cells) render after the swap.
    if (dataView) {
      dataView.addEventListener('htmx:afterSwap', function () {
        if (window.lucide) window.lucide.createIcons();
      });
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
