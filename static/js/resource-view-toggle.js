/* Resource page Table <-> Graph view toggle.
 *
 * Wiring (see templates/layout/header.html and templates/layout/base.html):
 *   - Header renders a Tabler btn-check radio group with inputs [data-view="table"|"graph"].
 *     Bootstrap styles the checked radio's label as active for free.
 *   - Body wraps the table content in #resource-table-view and the graph in
 *     #resource-graph-view (starts .d-none).
 *   - The graph partial (templates/partials/sparql-graph.html) is rendered with lazy=true,
 *     so it defers init until it receives a 'graph:init' event on its -root element.
 *
 * Switching only shows/hides the wrappers — the graph instance is never destroyed, so
 * dragged node positions, expansions and zoom/pan survive repeated toggling. Modeled on
 * bindRangeButtons in static/js/monitoring.js.
 */
(function () {
  'use strict';

  function init() {
    var inputs = document.querySelectorAll('.btn-check[data-view]');
    var tableView = document.getElementById('resource-table-view');
    var graphView = document.getElementById('resource-graph-view');
    if (!inputs.length || !tableView || !graphView) return; // not a resource page

    var graphInitialized = false;

    function showGraph() {
      // Reveal the container before initializing so Graph Explorer measures a non-zero size.
      graphView.classList.remove('d-none');
      tableView.classList.add('d-none');
      if (!graphInitialized) {
        graphInitialized = true;
        // The graph partial defaults its id to "sparql-graph" (base.html passes none).
        var root = document.getElementById('sparql-graph-root');
        if (root) root.dispatchEvent(new Event('graph:init'));
      }
    }

    function showTable() {
      // Leave the graph instance alive (hidden) so its state is preserved on return.
      tableView.classList.remove('d-none');
      graphView.classList.add('d-none');
    }

    // btn-check radios manage their own active styling; we only react to selection.
    inputs.forEach(function (input) {
      input.addEventListener('change', function () {
        if (!input.checked) return;
        if (input.dataset.view === 'graph') {
          showGraph();
        } else {
          showTable();
        }
      });
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
