// Remember which cards the user collapsed, across page loads.
//
// A card is any `.card` whose body is a direct `.collapse[id]` child — the
// markup every partial (sparqlTable, sparqlGrid, trees, graphs, map, …) emits.
// The key is template set + collapse id, so closing "Outgoing" on one Canton
// page closes it on every Canton page, but not on Person pages: ids are only
// unique within a set (see template-set.js).
//
// Only the collapsed state is stored, and restoring only ever closes a card,
// never opens one. That keeps it free of side effects: a card the server
// renders closed (empty or failed results, an explicit collapsed=true) stays
// closed, and nothing lazy-loaded on first open is triggered at page load.
// Re-opening a card simply deletes its key.
(function () {
  var STORAGE_KEY = 'visoto-collapsed-cards';
  var SELECTOR = '.card > .collapse[id]';

  // Wrapped because localStorage throws outright in some privacy modes, where
  // the cards should still work — just without remembering.
  function load() {
    try {
      return JSON.parse(localStorage.getItem(STORAGE_KEY)) || {};
    } catch (e) {
      return {};
    }
  }

  function save(state) {
    try {
      if (Object.keys(state).length) localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
      else localStorage.removeItem(STORAGE_KEY);
    } catch (e) { /* per-viewer convenience only */ }
  }

  function keyFor(el) {
    return activeTemplateSet() + '|' + el.id;
  }

  // Closes remembered cards under root by setting the classes directly rather
  // than through Bootstrap's hide(): no animation, no flash, and no Collapse
  // instance exists yet — Bootstrap reads the classes when the toggle is first
  // clicked. The cards' own shown.bs.collapse handlers (table redraw, graph
  // re-fit) already cope with a body that starts hidden.
  function restore(root) {
    var state = load();
    root.querySelectorAll(SELECTOR).forEach(function (el) {
      if (!state[keyFor(el)] || !el.classList.contains('show')) return;
      el.classList.remove('show');
      document.querySelectorAll('[data-bs-toggle="collapse"][data-bs-target="#' + CSS.escape(el.id) + '"]')
        .forEach(function (toggle) {
          toggle.classList.add('collapsed');
          toggle.setAttribute('aria-expanded', 'false');
        });
    });
  }

  function record(evt, collapsed) {
    var el = evt.target;
    if (!el.matches || !el.matches(SELECTOR)) return; // nested collapses bubble here too
    var state = load();
    if (collapsed) state[keyFor(el)] = 1;
    else delete state[keyFor(el)];
    save(state);
  }

  document.addEventListener('hidden.bs.collapse', function (evt) { record(evt, true); });
  document.addEventListener('shown.bs.collapse', function (evt) { record(evt, false); });

  // Async fragments (sparqlAsyncTable, shape summary, …) arrive after load.
  // htmx:load rather than afterSwap: it carries the new content even for an
  // outerHTML swap, where afterSwap's target is the element just replaced.
  document.addEventListener('htmx:load', function (evt) { restore(evt.detail.elt); });

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { restore(document); });
  } else {
    restore(document);
  }
})();
