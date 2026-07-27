/* eslint-disable */
/*
  Behaviour for the "sparqlGrid" partial (templates/partials/sparql-grid.html):
  the "open in YASGUI" button and the CSV / JSON downloads.

  Extracted from two inline <script> blocks in that partial so the code is
  cacheable, lintable and editable as JavaScript.

  The query and endpoint used to be interpolated straight into the script with
  toJSONRaw. They now travel in <template> islands written with toJSON, which
  escapes < > & as \u-sequences — necessary because a <template> re-encodes
  entities when read back through innerHTML, and SPARQL queries are full of
  angle-bracketed IRIs.

  Attributes read from the root element:
    data-sparql-grid           marker; presence means "initialize me"
    data-sparql-grid-id        DOM id prefix for this grid's elements/islands
    data-sparql-grid-filename  base name for downloaded files
*/
(function () {
  'use strict';

  function initSparqlGrid(root) {
    var ID = root.getAttribute('data-sparql-grid-id');
    if (!ID) return;

    function readIsland(suffix) {
      var el = document.getElementById(ID + suffix);
      if (!el) return null;
      try { return JSON.parse(el.innerHTML.trim()); } catch (e) { return null; }
    }

    // --- YASGUI "open query" button -----------------------------------------
    var sparqlEndpointBtn = document.querySelector("." + "sparql-endpoint-" + ID);
    if (sparqlEndpointBtn) {
      sparqlEndpointBtn.addEventListener("click", function(e) {
        e.preventDefault();
        var query = readIsland("-query");
        var endpoint = readIsland("-endpoint");
        var url = "https://yasgui.triply.cc/#query=" + encodeURIComponent(query) + "&endpoint=" + encodeURIComponent(endpoint);
        window.open(url, "_blank");
      });
    }

    // --- CSV / JSON downloads (no-ops when the grid has no results) ---------
    var bindingsEl = document.getElementById(ID + "-bindings");
    if (!bindingsEl) return;
    var bindings = JSON.parse(bindingsEl.innerHTML.trim());
    var filename = root.getAttribute('data-sparql-grid-filename') || ID;

    // Extract property/value pairs as plain text rows
    function getRows() {
      return bindings.map(function(b) {
        return {
          property: (b.property && (b.property.DisplayText || b.property.Value)) || '',
          value:    (b.value    && (b.value.DisplayText    || b.value.Value))    || ''
        };
      });
    }

    // CSV download
    var csvBtn = document.querySelector("." + "download-csv-" + ID);
    if (csvBtn) {
      csvBtn.addEventListener("click", function(e) {
        e.preventDefault();
        var rows = getRows();
        var lines = ["\uFEFFproperty,value"];
        rows.forEach(function(r) {
          function q(s) { return '"' + String(s).replace(/"/g, '""') + '"'; }
          lines.push(q(r.property) + "," + q(r.value));
        });
        var blob = new Blob([lines.join("\r\n")], { type: "text/csv;charset=utf-8;" });
        var a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        a.download = filename + ".csv";
        a.click();
      });
    }

    // JSON download
    var jsonBtn = document.querySelector("." + "download-json-" + ID);
    if (jsonBtn) {
      jsonBtn.addEventListener("click", function(e) {
        e.preventDefault();
        var rows = getRows();
        var blob = new Blob([JSON.stringify(rows, null, 2)], { type: "application/json" });
        var a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        a.download = filename + ".json";
        a.click();
      });
    }
  }

  // Re-entrant on purpose: a duplicate <script src> tag, or this file being
  // re-executed inside an HTMX-swapped fragment, must still pick up elements
  // that were not in the DOM the first time. There is deliberately no
  // module-level "already loaded" latch — only the per-element guard below.
  // Initialize every grid on the page. Guarded per element so a second boot
  // (duplicate script tag, or a fragment swapped in later) cannot double-init.
  function boot() {
    document.querySelectorAll('[data-sparql-grid]').forEach(function (root) {
      if (root.__visotoSparqlGridInit) return;
      root.__visotoSparqlGridInit = true;
      initSparqlGrid(root);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
