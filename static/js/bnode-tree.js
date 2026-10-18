/* eslint-disable */
/*
  Behaviour for the blank-node tree card (templates/partials/bnode-tree.html):
  collapse/expand all nodes, "Copy as Turtle", and "Execute on endpoint".

  The nodes themselves fold with native <details>, so the tree works without
  this file; it only adds the card-level actions.

  Re-entrancy: the card is swapped in by HTMX and carries its own <script> tag,
  so this file can run several times. boot() guards per card instead of
  latching a module-level flag that would leave a later card dead.
*/
(function () {
  'use strict';

  function initCard(card) {
    var cfg = {};
    try {
      cfg = JSON.parse(document.getElementById(card.getAttribute('data-bnode-tree-config')).innerHTML.trim());
    } catch (e) { /* actions that need config degrade to no-ops */ }

    var toggle = card.querySelector('[data-bnode-toggle-all]');
    if (toggle) {
      toggle.addEventListener('click', function (e) {
        e.preventDefault();
        var nodes = card.querySelectorAll('details.vs-bn-node');
        // Collapse when any node is open, otherwise expand everything.
        var anyOpen = Array.prototype.some.call(nodes, function (d) { return d.open; });
        nodes.forEach(function (d) { d.open = !anyOpen; });
        var label = anyOpen ? toggle.getAttribute('data-label-expand') : toggle.getAttribute('data-label-collapse');
        toggle.setAttribute('title', label);
        toggle.innerHTML = '<i data-lucide="' + (anyOpen ? 'chevrons-up-down' : 'chevrons-down-up') + '"></i>';
        if (window.lucide) lucide.createIcons({ nodes: [toggle] });
      });
    }

    var copy = card.querySelector('[data-bnode-copy]');
    if (copy && cfg.turtle) {
      copy.addEventListener('click', function (e) {
        e.preventDefault();
        var label = copy.querySelector('span');
        var original = label ? label.textContent : '';
        fetch(cfg.turtle)
          .then(function (r) { if (!r.ok) throw new Error(r.status); return r.text(); })
          .then(function (ttl) { return navigator.clipboard.writeText(ttl); })
          .then(function () { if (label) label.textContent = vsT('js.bnode.copied', 'Copied'); })
          .catch(function () { if (label) label.textContent = vsT('js.bnode.copyFailed', 'Copy failed'); })
          .then(function () { setTimeout(function () { if (label) label.textContent = original; }, 1500); });
      });
    }

    // Same YASGUI hand-off as the table cards' "Execute on endpoint".
    var exec = card.querySelector('[data-bnode-endpoint]');
    if (exec && cfg.query) {
      exec.addEventListener('click', function (e) {
        e.preventDefault();
        var url = 'https://yasgui.triply.cc/#query=' + encodeURIComponent(cfg.query) +
          '&endpoint=' + encodeURIComponent(cfg.endpoint || '');
        window.open(url, '_blank');
      });
    }

    if (window.lucide) lucide.createIcons({ nodes: [card] });
  }

  function boot() {
    document.querySelectorAll('[data-bnode-tree]').forEach(function (card) {
      if (card.__visotoBnodeTreeInit) return;
      card.__visotoBnodeTreeInit = true;
      initCard(card);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
