/* Resource page Table <-> Graph <-> Data view toggle.
 *
 * Wiring (see templates/layout/header.html and templates/layout/base.html):
 *   - Header renders a Tabler btn-check radio group with inputs
 *     [data-view="table"|"graph"|"data"]. Bootstrap styles the checked radio's
 *     label as active for free.
 *   - Body wraps the table content in #resource-table-view, the graph in
 *     #resource-graph-view, and the data table in #resource-data-view (both the
 *     graph and data views start .d-none).
 *   - The graph partial (templates/partials/sparql-graph.html) is rendered with
 *     lazy=true and initialized on a 'graph:init' event at its -root element.
 *   - The data view (templates/partials/sparql-async-table.html) is an HTMX
 *     element with hx-trigger="showData"; the query runs only when we fire that
 *     event on first reveal.
 *
 * Switching only shows/hides the wrappers — neither the graph instance nor the
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
    if (!inputs.length || !tableView) return; // not a resource page

    var graphInitialized = false;
    var dataInitialized = false;

    function showTable() {
      tableView.classList.remove('d-none');
      if (graphView) graphView.classList.add('d-none');
      if (dataView) dataView.classList.add('d-none');
    }

    function showGraph() {
      if (!graphView) return;
      // Reveal before initializing so Graph Explorer measures a non-zero size.
      graphView.classList.remove('d-none');
      tableView.classList.add('d-none');
      if (dataView) dataView.classList.add('d-none');
      if (!graphInitialized) {
        graphInitialized = true;
        // The graph partial defaults its id to "sparql-graph" (base.html passes none).
        var root = document.getElementById('sparql-graph-root');
        if (root) root.dispatchEvent(new Event('graph:init'));
      }
    }

    function showData() {
      if (!dataView) return;
      dataView.classList.remove('d-none');
      tableView.classList.add('d-none');
      if (graphView) graphView.classList.add('d-none');
      if (!dataInitialized) {
        dataInitialized = true;
        // Fire the HTMX trigger on the placeholder to run the query once.
        var hxEl = dataView.querySelector('[hx-get]');
        if (hxEl && window.htmx) window.htmx.trigger(hxEl, 'showData');
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
