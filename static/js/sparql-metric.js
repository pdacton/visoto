/* eslint-disable */
/*
  Behaviour for the "sparqlMetric" partial (templates/partials/sparql-metric.html):
  the "Execute on endpoint" action in each metric card's "⋮" menu.

  A sibling of sparql-table.js, using the same marker -> config-island convention:
  the card carries data-sparql-metric, and the query + endpoint arrive in a
  <template> island written by the partial.

  Two things differ from sparql-table.js:

  1. WHEN the island is read. The card's sibling <sparql-async> element is in the
     DOM at load, but it holds the query as authored: no PREFIXes, magic
     properties unexpanded, ?? unsubstituted — not what YASGUI needs. The
     FINALIZED query exists only server-side, and rides back with the HTMX count
     response, landing in the card some time after boot. It is therefore resolved
     lazily rather than read at load.

  2. HOW the link opens. sparql-table.js calls window.open() from a click handler,
     which browsers may treat as a popup and block. Here the menu item is left as
     a real <a target="_blank"> and we only fill in its href, so activating it is
     an ordinary user-initiated navigation that no popup blocker intercepts. The
     href is written when the dropdown opens (and refreshed on click), by which
     point the count response has long since delivered the island.
*/
(function () {
  'use strict';

  function initSparqlMetric(root) {
    var ID = root.getAttribute('data-sparql-metric-id');
    if (!ID) return;

    var item = root.querySelector('.sparql-endpoint-' + ID);
    if (!item) return;

    // Point the menu item at the YASGUI URL built from the config island.
    // Returns whether the href is now a real link.
    function syncHref() {
      var island = document.getElementById(ID + '-metric-config');
      if (!island) return false;

      var cfg;
      try {
        cfg = JSON.parse(island.innerHTML.trim());
      } catch (err) {
        return false;
      }
      if (!cfg.query || !cfg.endpoint) return false;

      item.href = "https://yasgui.triply.cc/#query=" + encodeURIComponent(cfg.query) +
                  "&endpoint=" + encodeURIComponent(cfg.endpoint);
      return true;
    }

    // The island arrives with the HTMX count response, so the href must be set as
    // soon as that lands — NOT on menu-open or click. Both of those are too late
    // on a page whose counts are still in flight: the user opens the menu and
    // activates the item in one interaction, and a click listener cannot rewrite
    // an href the browser has already resolved (it would follow "#" and reopen
    // the current page in a new tab).
    if (!syncHref()) {
      // htmx:afterSwap fires on the swapped element — the value div inside this
      // card — and bubbles, so listening on the card catches its own count only.
      root.addEventListener('htmx:afterSwap', syncHref);
    }

    // Last resort for any non-HTMX path that fills the card (and harmless once
    // the href is already correct).
    var dropdown = item.closest('.dropdown');
    if (dropdown) dropdown.addEventListener('show.bs.dropdown', syncHref);
  }

  function boot() {
    document.querySelectorAll('[data-sparql-metric]').forEach(function (root) {
      if (root.__visotoSparqlMetricInit) return;
      root.__visotoSparqlMetricInit = true;
      initSparqlMetric(root);
    });
  }

  // This tag sits in the middle of the body, so booting during the initial parse
  // could run before later cards are in the DOM. Wait for DOMContentLoaded then.
  //
  // When the document is already complete the wait is not just unnecessary but
  // wrong: a metric card can be swapped in as part of an HTMX fragment long after
  // DOMContentLoaded has fired, and it will never fire again.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
