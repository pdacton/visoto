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
  // The default reversed types, expanded with the active endpoint's prefixes.
  var defaults = null;
  function defaultReversed() {
    if (!defaults) defaults = window.VisotoLayout.defaultReversed(window.VisotoPrefixes.fromPage());
    return defaults;
  }

  function isReversed(typeId, overrides) {
    if (Object.prototype.hasOwnProperty.call(overrides, typeId)) return !!overrides[typeId];
    return !!defaultReversed()[typeId];
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
      Object.keys(defaultReversed()).forEach(function (t) { if (isReversed(t, overrides)) out[t] = true; });
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
          if (box.checked === !!defaultReversed()[t.id]) delete v[t.id];
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
      // Lands in the middle of the view, selected (GL-14, GH #42).
      var r = kit.container.getBoundingClientRect();
      var p = c.pageToPaper(r.left + r.width / 2 + window.scrollX, r.top + r.height / 2 + window.scrollY);
      var added;
      kit.batch(vsT('js.graph.cmd.addResource', 'Add resource'), function () {
        added = c.addElements([{ iri: iri, x: p.x - 90, y: p.y - 30 }]);
      });
      added.loaded.then(function (ids) {
        return kit.placeNew(ids).then(function () {
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

    // --- Connections menu: direction subheaders --------------------------------
    // ontodia_overrides.css groups the menu's rows into incoming and outgoing
    // and draws a subheader above each group from these two variables; CSS
    // cannot translate, so the words come from here. Set once on the canvas
    // container, which GE's dialogs live in and never replace.
    kit.container.style.setProperty('--vs-conn-incoming', JSON.stringify(vsT('js.graph.incoming', 'Incoming')));
    kit.container.style.setProperty('--vs-conn-outgoing', JSON.stringify(vsT('js.graph.outgoing', 'Outgoing')));
    kit.container.style.setProperty('--vs-conn-model', JSON.stringify(vsT('js.graph.modelLinks', 'Model')));

    // --- Connections menu + panel: model predicates in their own section ------
    // Ontology and shape predicates (type, subClassOf, domain, targetClass, …)
    // are split off from the business connections: each row whose predicate
    // (GE's data-linktypeid) lies in one of these namespaces gets
    // data-vs-model, and ontodia_overrides.css moves it below the others under
    // a "Model" subheader. In the menu that subheader hangs off the first model
    // row in visual order (see menuRank), marked data-vs-model-head;
    // the panel draws it from its list's ::after. Attributes, not DOM moves:
    // React keeps owning the rows, and a re-rendered row is tagged again.
    // Whole namespaces, plus single terms from vocabularies that are mostly
    // business data (schema.org's own domain/range declarations).
    function expandAll(curies) {
      return curies.map(function (c) { return window.VisotoPrefixes.fromPage().expand(c); })
        .filter(function (iri) { return iri.indexOf('//') > 0; }); // undeclared prefixes stay CURIEs
    }
    var MODEL_NS = expandAll(['rdf:', 'rdfs:', 'owl:', 'sh:']);
    var MODEL_TERMS = expandAll(['schema:domainIncludes', 'schema:rangeIncludes']);
    // Typing and the class hierarchy open the section: data-vs-model="top".
    var MODEL_TOP = expandAll(['rdf:type', 'rdfs:subClassOf']);
    var CONN_ROW = '.link-in-popup-menu[data-linktypeid], .linkInToolBox[data-linktypeid]';
    function modelKind(row) {
      var id = row.getAttribute('data-linktypeid');
      if (MODEL_TOP.indexOf(id) >= 0) return 'top';
      return MODEL_TERMS.indexOf(id) >= 0 ||
        MODEL_NS.some(function (ns) { return id.indexOf(ns) === 0; }) ? '' : null;
    }
    function setAttr(el, name, value) { // null removes
      if (value === null) { if (el.hasAttribute(name)) el.removeAttribute(name); }
      else if (el.getAttribute(name) !== value) el.setAttribute(name, value);
    }
    // A menu row's place in the model section, as ontodia_overrides.css orders
    // it: top rows first, incoming before outgoing within each part.
    function menuRank(r) {
      return (r.getAttribute('data-vs-model') === 'top' ? 0 : 2) +
        (r.querySelector('.link-in-popup-menu_direction__in-direction') ? 0 : 1);
    }
    var modelPending = false;
    function tagModelRows() {
      modelPending = false;
      kit.container.querySelectorAll(CONN_ROW).forEach(function (row) {
        setAttr(row, 'data-vs-model', modelKind(row));
      });
      kit.container.querySelectorAll('.graph-explorer-connections-menu_links-list').forEach(function (list) {
        var rows = list.querySelectorAll('.link-in-popup-menu[data-vs-model]');
        var head = null;
        rows.forEach(function (r) { if (!head || menuRank(r) < menuRank(head)) head = r; });
        rows.forEach(function (r) { setAttr(r, 'data-vs-model-head', r === head ? '' : null); });
      });
      tagPanelDirections();
    }
    // Connections panel: data-vs-in / data-vs-out on each row whose type the
    // selected element has in that direction, from kit.linkCounts (kit.js
    // recordLinkCounts). The panel follows GE's single selection; with none,
    // or several, or counts not in yet, rows stay untagged and show no arrows.
    function tagPanelDirections() {
      var ids = c.selectedIds();
      var counts = ids.length === 1 ? kit.linkCounts.get(c.iriOf(ids[0])) : null;
      kit.container.querySelectorAll('.link-types-toolbox .linkInToolBox[data-linktypeid]').forEach(function (row) {
        var n = counts && counts[row.getAttribute('data-linktypeid')];
        setAttr(row, 'data-vs-in', n && n.in ? '' : null);
        setAttr(row, 'data-vs-out', n && n.out ? '' : null);
      });
    }
    new MutationObserver(function () {
      if (!modelPending) { modelPending = true; requestAnimationFrame(tagModelRows); }
    }).observe(kit.container,
      { subtree: true, childList: true, attributes: true, attributeFilter: ['data-linktypeid'] });

    // --- Connections panel: one cycling visibility button per row ---------------
    // GE renders three buttons per row (allVisible, withoutLabels, invisible);
    // ontodia_overrides.css shows only the active one. A click on it presses
    // the next GE button instead — labels → lines only → hidden → labels — so
    // GE keeps owning the state. The tooltip names the state and the next one
    // (GE's own titles describe a button, not a state, and are English only).
    var LINK_STATES = {
      allVisible: vsT('js.graph.linksWithLabels', 'Links with labels'),
      withoutLabels: vsT('js.graph.linksOnly', 'Links only'),
      invisible: vsT('js.graph.linksHidden', 'Links hidden'),
    };
    var LINK_BTN = '.link-types-toolbox .connected-links .graph-explorer-btn';
    function nextLinkBtn(btn) { return btn.previousElementSibling || btn.parentElement.lastElementChild; }
    kit.container.addEventListener('click', function (e) {
      var btn = e.target instanceof Element && e.target.closest(LINK_BTN + '.active');
      if (!btn) return;
      e.preventDefault();
      e.stopPropagation();
      nextLinkBtn(btn).click();
    }, true);
    var titlePending = false;
    function titleLinkBtns() {
      titlePending = false;
      kit.container.querySelectorAll(LINK_BTN + '.active').forEach(function (btn) {
        var next = nextLinkBtn(btn);
        var title = vsTf('js.graph.linksCycle', '{current} · click: {next}',
          { current: LINK_STATES[btn.id] || '', next: LINK_STATES[next.id] || '' });
        if (btn.title !== title) btn.title = title;
      });
    }
    new MutationObserver(function () {
      if (!titlePending) { titlePending = true; requestAnimationFrame(titleLinkBtns); }
    }).observe(kit.container,
      { subtree: true, childList: true, attributes: true, attributeFilter: ['class'] });

    // --- Node menu: GE's halo buttons as a Tabler toolbar -----------------------
    // Each halo GE renders gets a .graph-halo-bar child with the canvas
    // toolbar's buttons (btn-sm btn-action btn-icon, .vr dividers);
    // a click presses GE's own halo button, hidden in ontodia_overrides.css,
    // so GE keeps the behaviour. Groups: what changes the canvas (connections,
    // expand properties) | what opens elsewhere (find connected, resource
    // page) | remove. React leaves a child it does not own alone; a new halo
    // (each selection) gets a new bar from the observer.
    var HALO_ITEMS = [
      ['navigate', 'share-2'], ['expand', 'panel-top-open', 'panel-top-close'], null,
      ['add-to-filter', 'funnel-plus'], ['folow', 'external-link'], null,
      ['remove', 'x'],
    ];
    function haloBar(halo) {
      var bar = document.createElement('div');
      bar.className = 'graph-halo-bar d-flex align-items-center gap-1 px-2 py-1';
      bar.setAttribute('role', 'toolbar');
      HALO_ITEMS.forEach(function (item) {
        if (!item) { bar.insertAdjacentHTML('beforeend', '<div class="vr"></div>'); return; }
        var ge = halo.querySelector('.graph-explorer-halo__' + item[0]);
        if (!ge) return;
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn btn-sm btn-action btn-icon';
        btn.dataset.halo = item[0];
        btn.title = ge.title;
        btn.setAttribute('aria-label', ge.title);
        btn.innerHTML = item[2]
          ? '<i data-lucide="' + item[1] + '" class="icon graph-halo-expand-open"></i>' +
            '<i data-lucide="' + item[2] + '" class="icon graph-halo-expand-close"></i>'
          : '<i data-lucide="' + item[1] + '" class="icon"></i>';
        bar.appendChild(btn);
      });
      bar.addEventListener('click', function (e) {
        var btn = e.target.closest('[data-halo]');
        var ge = btn && halo.querySelector('.graph-explorer-halo__' + btn.dataset.halo);
        if (ge) ge.click();
      });
      halo.appendChild(bar);
      if (window.lucide) window.lucide.createIcons();
    }
    new MutationObserver(function () {
      kit.container.querySelectorAll('.graph-explorer-halo').forEach(function (halo) {
        if (!halo.querySelector(':scope > .graph-halo-bar')) haloBar(halo);
      });
    }).observe(kit.container, { subtree: true, childList: true });

    // --- Icons in result rows (Instances panel, connections-menu Objects) ------
    // Each row GE renders gets its element's icon (kit.listIcons, filled by
    // kit.recordListIcons) as --vs-list-icon on its style; the CSS draws it
    // before the label. Rows are painted as they appear, so a long result
    // list costs one map lookup per rendered row. A custom property rather
    // than an <img>: GE re-renders these rows, and an element of ours inside
    // its markup could be dropped or upset that; React only rewrites the
    // style properties it owns. The IRI is read from GE's row tooltip,
    // "<label> <<iri>>\nClasses: …" — the only place the row names it.
    var ROW = 'graph-explorer-list-element-view';
    function paintRow(li) {
      var title = (li.getAttribute('title') || '').split('\nClasses:')[0];
      var open = title.lastIndexOf('<');
      var close = title.lastIndexOf('>');
      var url = open >= 0 && close > open ? kit.listIcons.get(title.slice(open + 1, close)) : null;
      if (url) li.style.setProperty('--vs-list-icon', 'url("' + url.replace(/["\\\n]/g, encodeURIComponent) + '")');
      else li.style.removeProperty('--vs-list-icon');
    }
    function paintRows(node) {
      if (!(node instanceof Element)) return;
      if (node.classList.contains(ROW)) paintRow(node);
      else node.querySelectorAll('.' + ROW).forEach(paintRow);
    }
    new MutationObserver(function (records) {
      records.forEach(function (r) {
        if (r.type === 'attributes') { if (r.target.classList.contains(ROW)) paintRow(r.target); return; }
        r.addedNodes.forEach(paintRows);
      });
    }).observe(kit.container, { subtree: true, childList: true, attributes: true, attributeFilter: ['title'] });

    // --- GE's side panels (Classes / Instances left, Connections right) ---------
    // GE collapses them only through a small round chevron on their inner edge.
    // Each panel gets one panel-left/right toggle in its place (the chevrons are
    // hidden in ontodia_overrides.css, the toggle clicks them), in the chevron's
    // place on the panel's inner edge, open or closed. The
    // panels' widths are published as --graph-inset-left/right on the stage, so
    // the floating toolbar, selection bar and docked panels sit between them
    // instead of over their headers (ontodia_overrides.css). Our buttons live in
    // the stage, not in GE's React DOM, and follow it through observers.
    var stage = kit.container.parentElement;
    var toggles = {};
    function toggle(side) {
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn btn-sm btn-icon graph-sidebar-toggle';
      btn.addEventListener('click', function () { if (btn._handle) btn._handle.click(); });
      stage.appendChild(btn);
      return (toggles[side] = btn);
    }
    function setToggle(btn, side, open) {
      if (btn._open === open) return;
      btn._open = open;
      var label = side === 'left'
        ? (open ? vsT('js.graph.closeLeftPanel', 'Close the classes and instances panel')
                : vsT('js.graph.openLeftPanel', 'Open the classes and instances panel'))
        : (open ? vsT('js.graph.closeRightPanel', 'Close the connections panel')
                : vsT('js.graph.openRightPanel', 'Open the connections panel'));
      btn.title = label;
      btn.setAttribute('aria-label', label);
      btn.innerHTML = '<i data-lucide="panel-' + side + '-' + (open ? 'close' : 'open') + '" class="icon"></i>';
      if (window.lucide) window.lucide.createIcons();
    }
    function syncSidebars() {
      var insets = { left: 0, right: 0 };
      var s = stage.getBoundingClientRect();
      kit.container.querySelectorAll('.graph-explorer-accordion-item__handle-btn').forEach(function (handle) {
        var side = handle.classList.contains('graph-explorer-accordion-item__handle-btn-left') ? 'left' : 'right';
        var item = handle.closest('.graph-explorer-accordion-item');
        if (!item) return;
        var r = item.getBoundingClientRect();
        insets[side] = Math.round(r.width);
        var open = item.classList.contains('graph-explorer-accordion-item--expanded');
        var btn = toggles[side] || toggle(side);
        btn._handle = handle;
        setToggle(btn, side, open);
        // Where GE's own chevron sat: straddling the panel's inner edge, at
        // half its height, so it never collides with the panel or canvas
        // corner buttons (GH #47).
        btn.style.top = (r.top - s.top + r.height / 2 - 16) + 'px';
        btn.style.left = ((side === 'left' ? r.right : r.left) - s.left - 16) + 'px';
      });
      stage.style.setProperty('--graph-inset-left', insets.left + 'px');
      stage.style.setProperty('--graph-inset-right', insets.right + 'px');
    }
    var syncQueued = false;
    function queueSync() {
      if (syncQueued) return;
      syncQueued = true;
      requestAnimationFrame(function () { syncQueued = false; syncSidebars(); });
    }
    // Collapse toggles flip a class on the panel; drags and resizes change its
    // size. Both are watched on the canvas container only.
    new MutationObserver(queueSync).observe(kit.container, { subtree: true, attributes: true, attributeFilter: ['class'] });
    if (window.ResizeObserver) new ResizeObserver(queueSync).observe(kit.container);
    kit.container.addEventListener('mouseup', queueSync);
    if (kit.card) new MutationObserver(queueSync).observe(kit.card, { attributes: true, attributeFilter: ['class'] });
    queueSync();

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
