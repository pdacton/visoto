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
    var copy = card.querySelector('[data-shape-copy]');
    var body = card.querySelector('[data-shape-text]');
    if (copy && body) {
      copy.addEventListener('click', function (e) {
        e.preventDefault();
        var title = copy.getAttribute('title');
        navigator.clipboard.writeText(body.innerText.trim())
          .then(function () { copy.setAttribute('title', copy.getAttribute('data-label-done')); })
          .catch(function () { copy.setAttribute('title', copy.getAttribute('data-label-failed')); })
          .then(function () { setTimeout(function () { copy.setAttribute('title', title); }, 1500); });
      });
    }
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
