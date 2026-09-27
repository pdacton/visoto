/* eslint-disable */
/*
  Toolbar tools of a Graph Explorer embed beyond layout and selection (A4):

    - Add resource (GL-47): search the endpoint by label, or paste an IRI.
    - Details (GL-48): expand / collapse the property cards of the selection,
      or of every node, as one undo step.
    - Layout options (GL-20–23): the link types on the canvas with their edge
      counts and a per-type "reverse for layout" switch, applied by Redraw.
    - Additions from GE's connections menu: confirmation above 20 (GL-15) and
      placement without moving existing nodes (GL-14).

  Installed by graph-kit.js on kit.attach(); markup in
  templates/partials/graph-toolbar.html (graphToolbar, graphLayoutOptions).
*/
(function () {
  'use strict';

  if (window.VisotoGraphTools) return;

  var SEARCH_LIMIT = 20;
  var SEARCH_DELAY = 350;
  var CONFIRM_ADDITIONS = 20; // GL-15

  // GL-22: user changes to "reverse for layout", per link-type IRI, shared by
  // every graph in this browser. Only differences from the defaults are kept.
  var REVERSED_KEY = 'visoto-graph:reversed';
  function readOverrides() {
    try {
      var v = JSON.parse(window.localStorage.getItem(REVERSED_KEY) || '{}');
      return v && typeof v === 'object' ? v : {};
    } catch (e) {
      return {};
    }
  }
  function writeOverrides(v) {
    try { window.localStorage.setItem(REVERSED_KEY, JSON.stringify(v)); } catch (e) { /* per-viewer convenience only */ }
  }
  function isReversed(typeId, overrides) {
    if (Object.prototype.hasOwnProperty.call(overrides, typeId)) return !!overrides[typeId];
    return !!window.VisotoLayout.DEFAULT_REVERSED[typeId];
  }

  function install(kit) {
    var c = kit.commands;
    var id = kit.id;
    var toolbar = document.getElementById(id + '-toolbar');
    var panel = document.getElementById(id + '-layoutoptions');

    // --- Reverse for layout (GL-21, GL-22) ----------------------------------
    kit.reversedTypes = function () {
      var overrides = readOverrides();
      var out = {};
      Object.keys(window.VisotoLayout.DEFAULT_REVERSED).forEach(function (t) { if (isReversed(t, overrides)) out[t] = true; });
      Object.keys(overrides).forEach(function (t) { if (overrides[t]) out[t] = true; });
      return out;
    };
    // Saved canvases carry the user's reverse settings (A3 snapshot).
    kit.reverseOverrides = readOverrides;
    kit.applyReverseOverrides = function (v) {
      if (v && typeof v === 'object') writeOverrides(Object.assign(readOverrides(), v));
      renderOptions();
    };

    // --- Layout options panel (GL-20, GL-23) ----------------------------------
    var dirty = false;
    function setDirty(value) {
      dirty = value;
      if (!panel) return;
      var redraw = panel.querySelector('[data-graph-action="redraw"]');
      if (redraw) {
        redraw.classList.toggle('btn-primary', dirty);
        redraw.classList.toggle('btn-outline-secondary', !dirty);
      }
      var hint = panel.querySelector('[data-graph-options-dirty]');
      if (hint) hint.hidden = !dirty;
    }
    function renderOptions() {
      if (!panel || panel.hidden) return;
      var list = panel.querySelector('[data-graph-options-list]');
      var empty = panel.querySelector('[data-graph-options-empty]');
      var overrides = readOverrides();
      var types = c.linkTypeCounts();
      list.replaceChildren();
      empty.hidden = types.length > 0;
      types.forEach(function (t) {
        var row = document.createElement('label');
        row.className = 'list-group-item d-flex align-items-center gap-2 py-1';
        row.title = t.id;
        var name = document.createElement('span');
        name.className = 'flex-fill text-truncate small';
        name.textContent = t.label;
        var count = document.createElement('span');
        count.className = 'badge bg-secondary-lt';
        count.textContent = String(t.count);
        count.setAttribute('aria-label', vsTf('js.graph.edgeCount', '{n} edges', { n: t.count }));
        var wrap = document.createElement('span');
        wrap.className = 'form-check form-switch m-0';
        var box = document.createElement('input');
        box.type = 'checkbox';
        box.className = 'form-check-input';
        box.checked = isReversed(t.id, overrides);
        box.setAttribute('aria-label', vsTf('js.graph.reverseFor', 'Reverse “{name}” for layout', { name: t.label }));
        box.addEventListener('change', function () {
          var v = readOverrides();
          if (box.checked === !!window.VisotoLayout.DEFAULT_REVERSED[t.id]) delete v[t.id];
          else v[t.id] = box.checked;
          writeOverrides(v);
          setDirty(true); // GL-23: applied by Redraw, not on toggle
          if (kit.scheduleAutosave) kit.scheduleAutosave();
        });
        wrap.appendChild(box);
        row.appendChild(name);
        row.appendChild(count);
        row.appendChild(wrap);
        list.appendChild(row);
      });
    }
    function toggleOptions(show) {
      if (!panel) return;
      panel.hidden = show === undefined ? !panel.hidden : !show;
      renderOptions();
    }
    if (panel) {
      panel.addEventListener('click', function (e) {
        var btn = e.target instanceof Element && e.target.closest('[data-graph-action]');
        if (!btn) return;
        var name = btn.getAttribute('data-graph-action');
        if (name === 'close-options') toggleOptions(false);
        if (name === 'redraw') {
          kit.layout(kit.algorithm || kit.defaultLayout).then(function () { setDirty(false); });
        }
      });
    }
    c.history.events.on('historyChanged', renderOptions); // edge counts follow the canvas

    // --- Details (GL-48) -------------------------------------------------------
    function toggleDetails() {
      var ids = c.selectedIds();
      if (!ids.length) ids = c.elementIds();
      if (!ids.length) return;
      var expand = ids.some(function (eid) { return !c.isExpanded(eid); });
      kit.batch(expand ? vsT('js.graph.cmd.showDetails', 'Show details') : vsT('js.graph.cmd.hideDetails', 'Hide details'), function () {
        c.setExpanded(ids, expand);
      });
    }

    // --- Add resource (GL-47) --------------------------------------------------
    var addMenu = toolbar && toolbar.querySelector('[data-graph-add]');
    var addInput = addMenu && addMenu.querySelector('input');
    var addResults = addMenu && addMenu.querySelector('[data-graph-add-results]');
    var searchTimer = null;
    var searchSeq = 0;

    function resultButton(text, sub, onPick) {
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'dropdown-item d-flex flex-column align-items-start text-wrap';
      var main = document.createElement('span');
      main.textContent = text;
      btn.appendChild(main);
      if (sub) {
        var small = document.createElement('span');
        small.className = 'small text-secondary text-truncate w-100';
        small.textContent = sub;
        btn.appendChild(small);
      }
      btn.addEventListener('click', onPick);
      return btn;
    }
    function note(text) {
      var p = document.createElement('div');
      p.className = 'dropdown-item-text small text-secondary';
      p.textContent = text;
      return p;
    }
    function runSearch() {
      var text = addInput.value.trim();
      addResults.replaceChildren();
      if (/^https?:\/\/\S+$/.test(text)) {
        addResults.appendChild(resultButton(vsT('js.graph.addIri', 'Add this IRI'), text, function () { add(text); }));
        return;
      }
      if (text.length < 2) return;
      var seq = ++searchSeq;
      addResults.appendChild(note(vsT('js.graph.searching', 'Searching…')));
      search(text).then(function (items) {
        if (seq !== searchSeq) return;
        addResults.replaceChildren();
        if (!items.length) {
          addResults.appendChild(note(vsT('js.graph.noMatches', 'No matching resources.')));
          return;
        }
        items.forEach(function (item) {
          addResults.appendChild(resultButton(item.label, item.iri, function () { add(item.iri); }));
        });
      }, function () {
        if (seq === searchSeq) addResults.replaceChildren(note(vsT('js.graph.endpointFailed', 'The endpoint did not answer.')));
      });
    }
    // Browse graphs search the endpoint through /api/search (the /search page's
    // full-text lookup, with endpoint and language on the URL); a constructed
    // diagram searches its own nodes through the in-memory provider.
    function search(text) {
      if (!kit.searchEndpoint) return c.search(text, SEARCH_LIMIT);
      var params = new URLSearchParams({ q: text, limit: String(SEARCH_LIMIT) });
      var slug = typeof window.activeEndpointSlug === 'function' ? window.activeEndpointSlug() : '';
      if (slug) params.set('endpoint', slug);
      params.set('lang', (document.documentElement.lang || '').slice(0, 2));
      return fetch('/api/search?' + params.toString()).then(function (res) {
        if (!res.ok) throw new Error('search ' + res.status);
        return res.json();
      }).then(function (hits) {
        return hits.map(function (h) { return { iri: h.iri, label: h.label + (h.type ? ' · ' + h.type : '') }; });
      });
    }

    function closeAddMenu() {
      var toggle = addMenu && addMenu.querySelector('[data-bs-toggle="dropdown"]');
      var Dropdown = window.tabler && window.tabler.Dropdown;
      if (toggle && Dropdown) Dropdown.getOrCreateInstance(toggle).hide();
    }
    function add(iri) {
      closeAddMenu();
      var existing = c.iriOnCanvas(iri);
      if (existing) {
        c.setSelection([existing]);
        if (kit.fitSelection) kit.fitSelection();
        return;
      }
      // Start at the middle of the view, then place it among its neighbours
      // with every other node fixed (GL-14).
      var r = kit.container.getBoundingClientRect();
      var p = c.pageToPaper(r.left + r.width / 2 + window.scrollX, r.top + r.height / 2 + window.scrollY);
      var added;
      kit.batch(vsT('js.graph.cmd.addResource', 'Add resource'), function () {
        added = c.addElements([{ iri: iri, x: p.x - 90, y: p.y - 30 }]);
      });
      added.loaded.then(function (ids) {
        return kit.placeNew(ids).then(function () {
          c.setSelection(ids);
          if (kit.fitSelection) kit.fitSelection();
        });
      });
    }
    if (addInput) {
      addInput.addEventListener('input', function () {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(runSearch, SEARCH_DELAY);
      });
      addInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') {
          e.preventDefault();
          clearTimeout(searchTimer);
          var first = addResults.querySelector('button');
          if (first && addResults.children.length === 1 && /^https?:/.test(addInput.value.trim())) first.click();
          else runSearch();
        }
      });
      addMenu.addEventListener('shown.bs.dropdown', function () { addInput.focus(); addInput.select(); });
    }

    // --- Connections-menu additions (GL-14, GL-15) ------------------------------
    c.onMenuAdd(function (n) {
      return n <= CONFIRM_ADDITIONS ||
        window.confirm(vsTf('js.graph.confirmAdd', 'Add {n} nodes to the diagram?', { n: n }));
    }, function (ids) { kit.placeNew(ids); });

    // --- Toolbar dispatch ----------------------------------------------------------
    if (toolbar) {
      toolbar.addEventListener('click', function (e) {
        var btn = e.target instanceof Element && e.target.closest('[data-graph-action]');
        if (!btn) return;
        var name = btn.getAttribute('data-graph-action');
        if (name === 'details') { e.preventDefault(); toggleDetails(); }
        if (name === 'layout-options') { e.preventDefault(); toggleOptions(); }
      });
    }
  }

  window.VisotoGraphTools = { install: install };
})();
