/* eslint-disable */
/*
  Classes, namespaces and Find on canvas for a Graph Explorer embed (A5):

    - Classes (GL-10, GL-49): the classes of the nodes on the canvas, each with
      an eye toggle (hide / show its nodes) and a ⋮ menu (Select all, Remove
      all). A class includes its subclasses (rdfs:subClassOf*), resolved by
      kit.superclasses when the embed provides it.
    - Namespaces (GL-52): the namespaces of the node IRIs, each with an eye
      toggle and the same ⋮ menu — e.g. hide external vocabularies on an
      ontology diagram.
    - Each section heading has a Hide all / Show all toggle: Show all while
      any row of the section is hidden, else Hide all.
    - Find on canvas (GL-32): highlight and zoom to nodes whose label or IRI
      contains the text; Enter selects them.

  GE has no element visibility flag (GE-UPSTREAM: B5), so hiding removes the
  nodes through GE's own commands and keeps a record of each (IRI, position,
  expanded, types); showing re-creates them from the records and reloads
  their data and links. A node is hidden while any rule (class or namespace)
  matches it. Each change is one undo step: a state command for the rules
  and records, plus GE's add/remove commands, in one batch.

  Installed by graph-kit.js on kit.attach(); markup: graphFilterPanel and the
  Find dropdown in templates/partials/graph-toolbar.html.
*/
(function () {
  'use strict';

  if (window.VisotoGraphFilter) return;

  // Short name for a namespace: its prefix from the active endpoint's
  // declarations (static/js/prefixes.js), else the namespace IRI itself.
  function nsLabel(ns) {
    var c = window.VisotoPrefixes.fromPage().compact(ns);
    return c ? c.prefix + ':' + c.local : ns;
  }

  var ABSOLUTE = /^[a-z][a-z0-9+.-]*:/i;

  function namespaceOf(iri) {
    var i = Math.max(iri.lastIndexOf('#'), iri.lastIndexOf('/'));
    return i > 0 ? iri.slice(0, i + 1) : iri;
  }

  function install(kit) {
    var c = kit.commands;
    var id = kit.id;
    var panel = document.getElementById(id + '-filter');
    var toolbar = document.getElementById(id + '-toolbar');

    var rules = { classes: {}, namespaces: {} };
    var hidden = []; // records of hidden nodes (see ge-adapter.js hide())
    var supers = {}; // class IRI -> [superclass IRIs] among the classes seen

    function clone(o) { return JSON.parse(JSON.stringify(o)); }

    // Does class C (with its subclasses) cover one of `types`?
    function coversType(classIri, types) {
      return types.some(function (t) {
        return t === classIri || (supers[t] && supers[t].indexOf(classIri) >= 0);
      });
    }
    function isHiddenBy(r, state) {
      var byClass = Object.keys(state.classes).some(function (cls) { return coversType(cls, r.types); });
      return byClass || !!state.namespaces[namespaceOf(r.iri)];
    }

    // Undoable state: rules + records. invoke() applies `to`, returns the undo.
    function setState(to, title) {
      return {
        title: title,
        invoke: function () {
          var from = { rules: clone(rules), hidden: clone(hidden) };
          rules = clone(to.rules);
          hidden = clone(to.hidden);
          render();
          return setState(from, title);
        },
      };
    }

    // Applies new rules: hides what they now match, shows what they no longer
    // do, as one undo step titled `title`.
    function apply(nextRules, title) {
      var toHide = c.elementIds().map(c.describe).filter(function (d) {
        return d && isHiddenBy(d, nextRules);
      });
      var stay = [], back = [];
      hidden.forEach(function (r) { (isHiddenBy(r, nextRules) ? stay : back).push(r); });
      var shown;
      kit.batch(title, function () {
        var pinnedBack = back.filter(function (r) { return r.pinned; });
        var records = c.hide(toHide.map(function (d) { return d.id; }));
        records.forEach(function (r) { r.pinned = kit.isPinned ? toHide.some(function (d) { return d.iri === r.iri && kit.isPinned(d.id); }) : false; });
        records.forEach(function (r) {
          var d = toHide.find(function (x) { return x.iri === r.iri; });
          r.color = d && kit.colorOf ? kit.colorOf(d.id) : null;
        });
        shown = back.length ? c.show(back) : null;
        c.history.execute(setState({ rules: nextRules, hidden: stay.concat(records) }, title));
        if (shown && pinnedBack.length && kit.addPins) {
          kit.addPins(shown.ids.filter(function (nid, i) { return back[i] && back[i].pinned; }));
        }
        if (shown && kit.addColors) {
          var colorsBack = {};
          shown.ids.forEach(function (nid, i) { if (back[i] && back[i].color) colorsBack[nid] = back[i].color; });
          kit.addColors(colorsBack);
        }
      });
      if (shown) shown.loaded.then(render);
    }

    function hideShowTitle(hiding, name) {
      return hiding ? vsTf('js.graph.cmd.hideClass', 'Hide {name}', { name: name })
                    : vsTf('js.graph.cmd.showClass', 'Show {name}', { name: name });
    }
    function toggleClass(cls) {
      var next = clone(rules);
      var label = c.classLabel(cls);
      if (next.classes[cls]) delete next.classes[cls]; else next.classes[cls] = true;
      apply(next, hideShowTitle(!!next.classes[cls], label));
    }
    function toggleNamespace(ns) {
      var next = clone(rules);
      if (next.namespaces[ns]) delete next.namespaces[ns]; else next.namespaces[ns] = true;
      apply(next, hideShowTitle(!!next.namespaces[ns], nsLabel(ns)));
    }
    // Hides every listed row of a section ('classes' or 'namespaces'), or
    // shows them all again when any is hidden.
    function toggleAll(kind, keys, name) {
      var next = clone(rules);
      var hiding = !Object.keys(next[kind]).length;
      next[kind] = {};
      if (hiding) keys.forEach(function (k) { next[kind][k] = true; });
      apply(next, hideShowTitle(hiding, name));
    }
    function membersOf(cls) {
      return c.elementIds().map(c.describe).filter(function (d) { return d && coversType(cls, d.types); })
        .map(function (d) { return d.id; });
    }
    function membersOfNamespace(ns) {
      return c.elementIds().map(c.describe).filter(function (d) { return d && namespaceOf(d.iri) === ns; })
        .map(function (d) { return d.id; });
    }

    // --- Saved canvases (A3) carry the rules and the hidden records ---------
    kit.filterState = function () { return { rules: clone(rules), hidden: clone(hidden) }; };
    kit.loadFilterState = function (s) {
      rules = s && s.rules ? clone(s.rules) : { classes: {}, namespaces: {} };
      hidden = s && s.hidden ? clone(s.hidden) : [];
      render();
    };

    // --- Superclasses (for "a class includes its subclasses") ---------------
    var asked = {};
    function learnSupers(types) {
      if (!kit.superclasses) return;
      var fresh = types.filter(function (t) { return !asked[t]; });
      if (!fresh.length) return;
      fresh.forEach(function (t) { asked[t] = true; });
      kit.superclasses(Object.keys(asked)).then(function (map) {
        supers = map || {};
        render();
      }, function () { /* nesting is a refinement; flat classes still work */ });
    }

    // --- Panel -----------------------------------------------------------------
    function eye(hiddenNow, label, onToggle) {
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn btn-sm btn-ghost-secondary btn-icon';
      btn.setAttribute('aria-pressed', String(!hiddenNow));
      var text = hiddenNow ? vsTf('js.graph.showName', 'Show {name}', { name: label }) : vsTf('js.graph.hideName', 'Hide {name}', { name: label });
      btn.title = text;
      btn.setAttribute('aria-label', text);
      var i = document.createElement('i');
      i.setAttribute('data-lucide', hiddenNow ? 'eye-off' : 'eye');
      i.className = 'icon';
      btn.appendChild(i);
      btn.addEventListener('click', onToggle);
      return btn;
    }
    function row(opts) {
      var item = document.createElement('div');
      item.className = 'list-group-item d-flex align-items-center gap-1 py-1' + (opts.hidden ? ' text-secondary' : '');
      item.title = opts.iri;
      item.appendChild(eye(opts.hidden, opts.label, opts.onToggle));
      var name = document.createElement('span');
      name.className = 'flex-fill text-truncate small' + (opts.hidden ? ' text-decoration-line-through' : '');
      name.textContent = opts.label;
      item.appendChild(name);
      var count = document.createElement('span');
      count.className = 'badge bg-secondary-lt';
      count.textContent = String(opts.count);
      item.appendChild(count);
      if (opts.menu) item.appendChild(opts.menu);
      return item;
    }
    function rowMenu(label, members) {
      var wrap = document.createElement('div');
      wrap.className = 'dropdown';
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn btn-sm btn-ghost-secondary btn-icon';
      btn.setAttribute('data-bs-toggle', 'dropdown');
      btn.setAttribute('aria-expanded', 'false');
      btn.setAttribute('aria-label', vsTf('js.graph.classActions', 'Actions for {name}', { name: label }));
      var i = document.createElement('i');
      i.setAttribute('data-lucide', 'more-vertical');
      i.className = 'icon';
      btn.appendChild(i);
      var menu = document.createElement('div');
      menu.className = 'dropdown-menu dropdown-menu-end';
      [['select', vsT('js.graph.selectAllOfClass', 'Select all')],
       ['remove', vsT('js.graph.removeAllOfClass', 'Remove all')]].forEach(function (m) {
        var item = document.createElement('button');
        item.type = 'button';
        item.className = 'dropdown-item' + (m[0] === 'remove' ? ' text-danger' : '');
        item.textContent = m[1];
        item.addEventListener('click', function () {
          var ids = members();
          if (m[0] === 'select') c.setSelection(ids);
          else kit.batch(vsTf('js.graph.cmd.removeClass', 'Remove all {name}', { name: label }), function () { c.removeElements(ids); });
        });
        menu.appendChild(item);
      });
      wrap.appendChild(btn);
      wrap.appendChild(menu);
      return wrap;
    }

    function allToggle(kind, keys) {
      var btn = panel.querySelector('[data-graph-filter-all="' + kind + '"]');
      var anyHidden = Object.keys(rules[kind]).length > 0;
      btn.hidden = !keys.length;
      btn.textContent = anyHidden ? vsT('js.graph.showAll', 'Show all') : vsT('js.graph.hideAll', 'Hide all');
      var name = btn.parentElement.querySelector('[data-graph-filter-heading]').textContent;
      btn.onclick = function () { toggleAll(kind, keys, name); };
    }

    function render() {
      if (!panel || panel.hidden) return;
      var described = c.elementIds().map(c.describe).filter(Boolean);
      var all = described.concat(hidden);

      var classCount = {};
      // Blank-node types (b0_genid-…, no scheme) are not listable classes.
      all.forEach(function (d) {
        d.types.forEach(function (t) { if (ABSOLUTE.test(t)) classCount[t] = (classCount[t] || 0) + 1; });
      });
      Object.keys(rules.classes).forEach(function (t) { if (!classCount[t]) classCount[t] = 0; });
      learnSupers(Object.keys(classCount));
      var classes = Object.keys(classCount).map(function (t) { return { iri: t, label: c.classLabel(t), count: classCount[t] }; })
        .sort(function (a, b) { return a.label.localeCompare(b.label); });

      var nsCount = {};
      all.forEach(function (d) { var ns = namespaceOf(d.iri); nsCount[ns] = (nsCount[ns] || 0) + 1; });
      // Count first, then name: rows must not trade places when a toggle moves
      // nodes between the canvas and the hidden records.
      var namespaces = Object.keys(nsCount).sort(function (a, b) {
        return nsCount[b] - nsCount[a] || nsLabel(a).localeCompare(nsLabel(b));
      });

      var classList = panel.querySelector('[data-graph-filter-classes]');
      classList.replaceChildren();
      classes.forEach(function (cl) {
        classList.appendChild(row({
          iri: cl.iri, label: cl.label, count: cl.count, hidden: !!rules.classes[cl.iri],
          onToggle: function () { toggleClass(cl.iri); },
          menu: rowMenu(cl.label, function () { return membersOf(cl.iri); }),
        }));
      });
      panel.querySelector('[data-graph-filter-noclasses]').hidden = classes.length > 0;

      var nsList = panel.querySelector('[data-graph-filter-namespaces]');
      nsList.replaceChildren();
      namespaces.forEach(function (ns) {
        nsList.appendChild(row({
          iri: ns, label: nsLabel(ns), count: nsCount[ns], hidden: !!rules.namespaces[ns],
          onToggle: function () { toggleNamespace(ns); },
          menu: rowMenu(nsLabel(ns), function () { return membersOfNamespace(ns); }),
        }));
      });

      allToggle('classes', classes.map(function (cl) { return cl.iri; }));
      allToggle('namespaces', namespaces);
      if (window.lucide) window.lucide.createIcons();
    }

    function togglePanel(show) {
      if (!panel) return;
      panel.hidden = show === undefined ? !panel.hidden : !show;
      // The toolbar button stays pressed while the panel is open.
      if (toolbar) {
        toolbar.querySelectorAll('[data-graph-action="filter"]').forEach(function (btn) {
          btn.classList.toggle('active', !panel.hidden);
          btn.setAttribute('aria-pressed', String(!panel.hidden));
        });
      }
      if (!panel.hidden) {
        var other = document.getElementById(id + '-layoutoptions');
        if (other) other.hidden = true;
      }
      render();
    }
    if (panel) {
      panel.addEventListener('click', function (e) {
        var btn = e.target instanceof Element && e.target.closest('[data-graph-action="close-filter"]');
        if (btn) togglePanel(false);
      });
    }
    c.history.events.on('historyChanged', render);

    // --- Find on canvas (GL-32) ------------------------------------------------
    var find = toolbar && toolbar.querySelector('[data-graph-find]');
    var findInput = find && find.querySelector('input');
    var findCount = find && find.querySelector('[data-graph-find-count]');
    var matches = [];
    function runFind() {
      var q = findInput.value.trim().toLowerCase();
      if (!q) {
        matches = [];
        c.highlight(null);
        findCount.textContent = '';
        return;
      }
      matches = c.elementIds().map(c.describe).filter(function (d) {
        return d && (d.label.toLowerCase().indexOf(q) >= 0 || d.iri.toLowerCase().indexOf(q) >= 0);
      }).map(function (d) { return d.id; });
      var set = {};
      matches.forEach(function (m) { set[m] = true; });
      c.highlight(function (eid) { return !!set[eid]; });
      findCount.textContent = vsTf('js.graph.findCount', '{n} of {total}', { n: matches.length, total: c.elementIds().length });
      if (matches.length) {
        var boxes = matches.map(c.box);
        var l = Infinity, t = Infinity, r = -Infinity, b = -Infinity;
        boxes.forEach(function (n) { l = Math.min(l, n.x); t = Math.min(t, n.y); r = Math.max(r, n.x + n.width); b = Math.max(b, n.y + n.height); });
        c.fitRect({ x: l, y: t, width: r - l, height: b - t });
      }
    }
    if (findInput) {
      var findTimer = null;
      findInput.addEventListener('input', function () { clearTimeout(findTimer); findTimer = setTimeout(runFind, 200); });
      findInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); runFind(); if (matches.length) c.setSelection(matches); }
      });
      find.addEventListener('shown.bs.dropdown', function () { findInput.focus(); findInput.select(); runFind(); });
      find.addEventListener('hidden.bs.dropdown', function () { c.highlight(null); });
    }

    if (toolbar) {
      toolbar.addEventListener('click', function (e) {
        var btn = e.target instanceof Element && e.target.closest('[data-graph-action="filter"]');
        if (!btn) return;
        e.preventDefault();
        togglePanel();
      });
    }
  }

  window.VisotoGraphFilter = { install: install, namespaceOf: namespaceOf };
})();
