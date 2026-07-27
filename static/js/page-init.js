/* eslint-disable */
/*
  Small page-wide initialisations that used to sit in three separate inline
  <script> blocks in templates/layout/base.html, each placed directly after the
  CDN library it depends on.

  They are safe to run together from one file as long as it is loaded after all
  of those libraries: classic (non-defer) scripts execute in document order, so
  by the time this runs tabler, highlight.js and lucide are all present. Each
  block is still guarded, so a missing library degrades to a no-op instead of
  throwing and taking the rest of the file down with it.
*/
(function () {
  'use strict';

  // --- Bootstrap tooltips --------------------------------------------------
  if (window.tabler && tabler.bootstrap) {
    // Initialize Bootstrap tooltips - required for Bootstrap 5
    // Tooltips must be explicitly initialized as they are opt-in for performance reasons
    // See: https://getbootstrap.com/docs/5.0/components/tooltips/#example-enable-tooltips-everywhere
    // Note: Tabler exposes Bootstrap under tabler.bootstrap
    var tooltipTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="tooltip"]'))
    var tooltipList = tooltipTriggerList.map(function (tooltipTriggerEl) {
      return new tabler.bootstrap.Tooltip(tooltipTriggerEl)
    })
  }

  // --- Syntax highlighting -------------------------------------------------
  if (window.hljs) {
    hljs.highlightAll();
    document.querySelectorAll('code.language-json .hljs-attr').forEach(function(el) {
      var text = el.textContent.replace(/"/g, '');
      if (text === 'Vars' || text === 'Bindings') {
        el.style.backgroundColor = 'yellow';
      }
    });
  }

  // --- Lucide icons --------------------------------------------------------
  if (window.lucide) {
    lucide.createIcons();
  }
})();
