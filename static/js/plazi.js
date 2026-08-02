/* eslint-disable */
/*
  Share bars for the kingdom cards on /plazi.html.

  Each card's count arrives independently over HTMX, so the total is not known
  until the last one lands. This watches the cards, and once every count has a
  number, sizes each bar as that kingdom's share of the total.

  Why a share bar at all: the counts span three orders of magnitude (Animalia
  ~781k vs Protozoa ~2k). Six numbers alone do not convey that; a bar does it at
  a glance, which is the point of showing the kingdoms side by side.

  The bar is rendered hidden and only revealed once sized, so a failed or slow
  count leaves no empty track behind — the cards simply look like they did
  before the bars existed.

  Counts are formatted server-side per locale (781'462 in de, 781,462 in en), so
  the digits are recovered here by stripping every non-digit rather than by
  parseInt, which would stop at the first separator and read 781'462 as 781.
*/
(function () {
  'use strict';

  var MARKER = '[data-kingdom-figure]';

  /** Reads one card's count, or null while it is still a spinner or an error. */
  function countOf(card) {
    var el = card.querySelector('[data-kingdom-count]');
    if (!el) return null;
    var digits = (el.textContent || '').replace(/\D/g, '');
    if (digits === '') return null;
    return parseInt(digits, 10);
  }

  function paint() {
    var figures = document.querySelectorAll(MARKER);
    if (!figures.length) return;

    var counts = [];
    var total = 0;
    for (var i = 0; i < figures.length; i++) {
      var n = countOf(figures[i]);
      if (n === null) return; // still loading; wait for the next swap
      counts.push(n);
      total += n;
    }
    if (total <= 0) return;

    for (var j = 0; j < figures.length; j++) {
      var body = figures[j].closest('.card-body');
      var bar = body && body.querySelector('[data-kingdom-bar]');
      if (!bar) continue;
      var pct = (counts[j] / total) * 100;
      var fill = bar.querySelector('.progress-bar');
      if (fill) {
        // Floor at a hairline so the smallest kingdoms remain visible rather
        // than collapsing to nothing — the bar is comparative, not metric.
        fill.style.width = Math.max(pct, 0.75) + '%';
        fill.setAttribute('aria-valuenow', pct.toFixed(1));
      }
      bar.hidden = false;
    }
  }

  // Every count lands as its own HTMX swap, so re-check after each one; paint()
  // is a no-op until the full set has arrived.
  document.body.addEventListener('htmx:afterSwap', paint);

  // No module-level latch: HTMX swaps can re-run page scripts, and this tag sits
  // above the CDN bundles, so both readiness paths are covered.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', paint);
  } else {
    paint();
  }
})();
