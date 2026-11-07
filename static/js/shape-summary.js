/* eslint-disable */
/*
  Behaviour for the shape summary card (templates/partials/shape-summary.html):
  "Copy as text" copies the summary as the reader sees it.

  Re-entrancy: like bnode-tree.js, the card is swapped in by HTMX and carries
  its own <script> tag, so boot() guards per card.
*/
(function () {
  'use strict';

  function initCard(card) {
    // The copy button is a Tabler Clipboard component (data-bs-toggle /
    // data-bs-target in the markup), picked up after the swap by page-init.js.
    if (window.lucide) lucide.createIcons({ nodes: [card] });
  }

  function boot() {
    document.querySelectorAll('[data-shape-summary]').forEach(function (card) {
      if (card.__visotoShapeSummaryInit) return;
      card.__visotoShapeSummaryInit = true;
      initCard(card);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
