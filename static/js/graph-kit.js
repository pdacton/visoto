/* eslint-disable */
/*
  Shared controller for every Graph Explorer embed: the sparqlGraph partial
  (browse and construct mode, static/js/sparql-graph.js) and the schemaGraph
  partial (static/js/schema-graph.js). What a graph DOES differs per embed; how
  it is loaded, driven from the toolbar, undone, and how it reports errors is
  the same, and lives here.

  Usage (from an embed file):

    var kit = VisotoGraph.create(ID);          // before GE is loaded
    kit.load().then(function () {              // GE bundle, with Retry on failure
      VisotoGE.render(container, kit.workspaceProps({ ref: onMounted, ... }));
    });
    function onMounted(workspace) {
      kit.attach(workspace);                   // toolbar, hotkeys, fullscreen refit
      ... initial load ...
      kit.ready();                             // start of the undo history
    }

  All GE calls go through window.VisotoGE (static/js/ge-adapter.js). Markup
  comes from templates/partials/graph-toolbar.html; per-instance elements are
  found by the `<id>-…` DOM ids and `data-graph-*` markers inside the card.
*/
(function () {
  'use strict';

  if (window.VisotoGraph) return;

  // Strings come from window.vsT / vsTf (static/js/i18n.js, loaded in <head>).

  // Label languages offered in the Language menu. Native names, so no catalog
  // entry per language; Romansh is absent because LINDAS labels hardly ever are.
  var LANGUAGES = [
    { code: 'de', label: 'Deutsch' },
    { code: 'en', label: 'English' },
    { code: 'fr', label: 'Français' },
    { code: 'it', label: 'Italiano' },
  ];

  // GE's own command titles are English literals; show them translated.
  var GE_TITLES = {
    'Move elements and links': function () { return vsT('js.graph.cmd.move', 'Move'); },
    'Force layout': function () { return vsT('js.graph.cmd.layout', 'Layout'); },
    'Expand element': function () { return vsT('js.graph.cmd.expand', 'Expand details'); },
    'Collapse element': function () { return vsT('js.graph.cmd.collapse', 'Collapse details'); },
    'Drag and drop onto diagram': function () { return vsT('js.graph.cmd.add', 'Add to diagram'); },
    'Add element': function () { return vsT('js.graph.cmd.add', 'Add to diagram'); },
    'Remove element': function () { return vsT('js.graph.cmd.remove', 'Remove'); },
    'Change link type visibility': function () { return vsT('js.graph.cmd.linkVisibility', 'Show / hide links'); },
  };
  function commandTitle(command) {
    var title = command && command.title;
    if (!title) return vsT('js.graph.cmd.change', 'Change');
    var known = GE_TITLES[title];
    return known ? known() : title;
  }

  // GL-46: above this many nodes a layout asks before it starts.
  var LARGE_GRAPH = 500;

  // elkjs for the Tree layouts, loaded on first use. Unmodified from the CDN
  // (EPL-2.0 OR GPL-3.0-or-later); src and integrity move together.
  var ELK_SRC = 'https://cdn.jsdelivr.net/npm/elkjs@0.12.0/lib/elk.bundled.js';
  var ELK_SRI = 'sha384-ww57TDqx4cGknIavPm0QKO+aygLUR1BLSn2Vhbnt1XdYKWcwLyWTFKX7aZMaKIi2';
  var elkLoading = null;
  function loadElk() {
    if (elkLoading) return elkLoading;
    elkLoading = new Promise(function (resolve, reject) {
      if (window.ELK) { resolve(new window.ELK()); return; }
      var script = document.createElement('script');
      script.src = ELK_SRC;
      script.integrity = ELK_SRI;
      script.crossOrigin = 'anonymous';
      script.addEventListener('load', function () {
        if (window.ELK) resolve(new window.ELK());
        else reject(new Error('elkjs did not register window.ELK'));
      });
      script.addEventListener('error', function () { reject(new Error('elkjs failed to load')); });
      document.head.appendChild(script);
    });
    elkLoading.catch(function () { elkLoading = null; }); // Retry loads again
    return elkLoading;
  }

  function readIsland(id, suffix) {
    var el = document.getElementById(id + suffix);
    if (!el) return null;
    try { return JSON.parse(el.innerHTML.trim()); } catch (e) { return null; }
  }

  function isTyping(target) {
    if (!(target instanceof Element)) return false;
    return !!target.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]');
  }

  // Hotkeys go to the graph the user last touched, so two graphs on one page do
  // not both undo on one Ctrl+Z.
  // A press anywhere else on the page makes no graph active, so Ctrl+A and Esc
  // go back to the page (the card's own capture listener runs after this one).
  var activeKit = null;
  document.addEventListener('pointerdown', function () { activeKit = null; }, true);
  document.addEventListener('keydown', function (e) {
    if (!activeKit || !activeKit.commands || isTyping(e.target)) return;
    if (e.key === 'Escape' && activeKit.clearSelection) {
      activeKit.clearSelection(); // GL-8
      return;
    }
    var mod = e.ctrlKey || e.metaKey;
    if (!mod || e.altKey) return;
    var key = e.key.toLowerCase();
    if (key === 'a' && !e.shiftKey && activeKit.selectAll) {
      e.preventDefault();
      activeKit.selectAll(); // GL-8
    } else if (key === 'z' && !e.shiftKey) {
      e.preventDefault();
      activeKit.undo();
    } else if ((key === 'z' && e.shiftKey) || key === 'y') {
      e.preventDefault();
      activeKit.redo();
    }
  });

  var kits = {};

  function create(id) {
    var card = document.getElementById(id + '-card');
    var frame = document.getElementById(id + '-frame');
    var container = document.getElementById(id + '-root');
    var toolbar = document.getElementById(id + '-toolbar');
    var overlay = document.getElementById(id + '-overlay');

    var h = readIsland(id, '-height');
    if (container && h) container.style.height = h;

    var kit = kits[id] = {
      id: id,
      card: card,
      container: container,
      workspace: null,
      commands: null,
      readIsland: function (suffix) { return readIsland(id, suffix); },
    };

    if (card) {
      var activate = function () { activeKit = kit; };
      card.addEventListener('pointerdown', activate, true);
      card.addEventListener('focusin', activate);
    }

    function action(name) {
      return toolbar ? toolbar.querySelectorAll('[data-graph-action="' + name + '"]') : [];
    }
    function setEnabled(name, enabled) {
      action(name).forEach(function (el) {
        el.disabled = !enabled;
        el.classList.toggle('disabled', !enabled);
        el.setAttribute('aria-disabled', String(!enabled));
      });
    }

    // --- Messages (GL-45) -------------------------------------------------------
    // One inline alert over the canvas; the canvas itself stays usable. A retry
    // callback adds a Retry button.
    var message = null;
    function showMessage(text, opts) {
      opts = opts || {};
      clearMessage();
      if (!overlay) return;
      var alert = document.createElement('div');
      alert.className = 'alert alert-' + (opts.level || 'danger') + ' graph-message mb-0';
      alert.setAttribute('role', opts.level === 'info' ? 'status' : 'alert');
      var body = document.createElement('div');
      body.className = 'd-flex align-items-center gap-2 flex-wrap';
      var span = document.createElement('span');
      span.className = 'flex-fill';
      span.textContent = text; // endpoint error text is data, never markup
      body.appendChild(span);
      if (opts.retry) {
        var retry = document.createElement('button');
        retry.type = 'button';
        retry.className = 'btn btn-sm';
        retry.textContent = opts.actionLabel || vsT('js.graph.retry', 'Retry');
        retry.addEventListener('click', function () { clearMessage(); opts.retry(); });
        body.appendChild(retry);
      }
      var close = document.createElement('button');
      close.type = 'button';
      close.className = 'btn-close';
      close.setAttribute('aria-label', vsT('js.graph.dismiss', 'Dismiss'));
      close.addEventListener('click', function () { clearMessage(); if (opts.dismiss) opts.dismiss(); });
      body.appendChild(close);
      alert.appendChild(body);
      overlay.appendChild(alert);
      message = alert;
    }
    function clearMessage() {
      if (message) message.remove();
      message = null;
    }
    kit.showMessage = showMessage;
    kit.clearMessage = clearMessage;

    // --- Busy indicator with Cancel (GL-4, GL-46) --------------------------------
    // busy(text, onCancel) shows a spinner; returns a function that hides it.
    var busyEl = null;
    function busy(text, onCancel) {
      idle();
      if (!overlay) return idle;
      var el = document.createElement('div');
      el.className = 'graph-busy card card-sm shadow-sm';
      el.setAttribute('role', 'status');
      var inner = document.createElement('div');
      inner.className = 'card-body d-flex align-items-center gap-2 py-2';
      var spin = document.createElement('div');
      spin.className = 'spinner-border spinner-border-sm text-secondary';
      spin.setAttribute('aria-hidden', 'true');
      var label = document.createElement('span');
      label.textContent = text;
      inner.appendChild(spin);
      inner.appendChild(label);
      if (onCancel) {
        var cancel = document.createElement('button');
        cancel.type = 'button';
        cancel.className = 'btn btn-sm btn-ghost-secondary';
        cancel.textContent = vsT('js.graph.cancel', 'Cancel');
        cancel.addEventListener('click', function () { idle(); onCancel(); });
        inner.appendChild(cancel);
      }
      el.appendChild(inner);
      overlay.appendChild(el);
      busyEl = el;
      return function () { if (busyEl === el) idle(); };
    }
    function idle() {
      if (busyEl) busyEl.remove();
      busyEl = null;
    }
    kit.busy = busy;

    // task(text, fn): runs fn(signal) behind a cancellable spinner. The spinner
    // paints before fn starts (a synchronous layout would otherwise freeze the
    // page before it appears); fn checks signal.aborted between steps.
    var running = null;
    kit.task = function (text, fn) {
      if (running) running.abort();
      var ctrl = new AbortController();
      running = ctrl;
      var done = busy(text, function () { ctrl.abort(); });
      return new Promise(function (resolve) {
        requestAnimationFrame(function () { requestAnimationFrame(resolve); });
      }).then(function () {
        if (ctrl.signal.aborted) return;
        return fn(ctrl.signal);
      }).finally(function () {
        if (running === ctrl) running = null;
        done();
      });
    };

    // --- Library and provider errors -------------------------------------------
    kit.load = function () {
      var done = busy(vsT('js.graph.loading', 'Loading graph…'));
      return window.VisotoGE.load().then(function (ge) {
        done();
        return ge;
      }, function () {
        done();
        return new Promise(function (resolve) {
          showMessage(vsT('js.graph.libraryFailed', 'Failed to load the Graph Explorer library.'), {
            retry: function () { resolve(kit.load()); },
          });
        });
      });
    };

    // Wraps a DataProvider so an endpoint error or timeout shows the inline
    // message with Retry instead of GE failing silently. The failed call stays
    // pending until the user retries (then resolves with the retried result) or
    // dismisses (then rejects with the original error, as it would have).
    // classTree is left out: it only fills the Classes sidebar, and on a large
    // endpoint (all of LINDAS) it times out on every page, so a Retry banner
    // for it would be permanent noise over graphs that load fine.
    var PROVIDER_METHODS = ['linkTypes', 'classInfo', 'propertyInfo', 'linkTypesInfo',
      'elementInfo', 'linksInfo', 'linkTypesOf', 'linkElements', 'filter'];
    var failed = [];
    kit.guardProvider = function (provider) {
      PROVIDER_METHODS.forEach(function (name) {
        var inner = provider[name];
        if (typeof inner !== 'function') return;
        provider[name] = function () {
          var args = arguments;
          return inner.apply(provider, args).catch(function (err) {
            if (err && err.name === 'AbortError') throw err;
            return new Promise(function (resolve, reject) {
              // Re-enter through the wrapper, so a second failure asks again.
              var again = function () { return provider[name].apply(provider, args); };
              failed.push({ again: again, resolve: resolve, reject: reject, error: err });
              showMessage(vsT('js.graph.endpointFailed', 'The endpoint did not answer.'), {
                retry: function () {
                  var retrying = failed; failed = [];
                  retrying.forEach(function (f) { f.again().then(f.resolve, f.reject); });
                },
                dismiss: function () {
                  var dropped = failed; failed = [];
                  dropped.forEach(function (f) { f.reject(f.error); });
                },
              });
            });
          });
        };
      });
      return provider;
    };

    // --- Workspace ---------------------------------------------------------------
    // Props every embed passes to GE: the undo history and the label language.
    kit.workspaceProps = function (props) {
      var pageLang = (document.documentElement.lang || '').slice(0, 2);
      var lang = LANGUAGES.some(function (l) { return l.code === pageLang; }) ? pageLang : 'en';
      return Object.assign({
        history: window.VisotoGE.createHistory(),
        languages: LANGUAGES,
        language: lang,
        // GE's default minimum scale (0.2) stops Fit short of a few hundred
        // nodes (the system diagram): the overview would never fit.
        zoomOptions: { min: 0.05 },
        // Selection, group drag and pinning ride on GE's paper pointer events
        // (graph-selection.js installs kit.onPointer*).
        onPointerDown: window.VisotoGE.pointerHandler(function (e) { if (kit.onPointerDown) kit.onPointerDown(e); }),
        onPointerMove: window.VisotoGE.pointerHandler(function (e) { if (kit.onPointerMove) kit.onPointerMove(e); }),
        onPointerUp: window.VisotoGE.pointerHandler(function (e) { if (kit.onPointerUp) kit.onPointerUp(e); }),
      }, props);
    };

    kit.attach = function (workspace) {
      if (kit.workspace === workspace) return kit;
      kit.workspace = workspace;
      kit.commands = window.VisotoGE.commands(workspace);
      kit.commands.history.events.on('historyChanged', updateHistoryButtons);
      kit.commands.history.events.on('historyChanged', scheduleAutosave);
      wireToolbar();
      if (window.VisotoGraphSelection) window.VisotoGraphSelection.install(kit);
      if (window.VisotoGraphTools) window.VisotoGraphTools.install(kit);
      updateHistoryButtons();
      updateLanguage();
      updateLayoutMenu();
      return kit;
    };

    // The page's starting state is not an undo step: forget everything the
    // initial load recorded (element creation, the first layout).
    kit.ready = function () {
      if (!kit.commands) return;
      suppressAutosave = true;
      kit.commands.history.reset();
      suppressAutosave = false;
      tracking = true;
    };

    kit.undo = function () { if (kit.commands) kit.commands.undo(); };
    kit.redo = function () { if (kit.commands) kit.commands.redo(); };

    // Runs fn as ONE undo step titled `title` (already translated). Everything
    // fn records in GE's history — its own batches included — nests inside.
    kit.batch = function (title, fn) {
      var batch = kit.commands.history.startBatch(title);
      try {
        var result = fn();
      } catch (e) {
        batch.store();
        throw e;
      }
      batch.store();
      return result;
    };

    kit.fit = function () {
      if (kit.commands) kit.commands.zoomToFit();
    };

    // --- Layout (GL-11, 13, 16, 17, 19; engines in graph-layout.js) -------------
    // The embed sets kit.defaultLayout (GL-2) and kit.pageIri (Radial's centre
    // fallback, GL-19). kit.algorithm is the last one applied.
    kit.defaultLayout = 'network';
    kit.pageIri = null;
    kit.algorithm = null;

    // layout(algorithm, { initial }) lays out the selection (2+ nodes) or the
    // whole graph as ONE undo step, behind a spinner whose Cancel discards the
    // result. `initial` is the page's own first layout: no size warning.
    kit.layout = function (algorithm, opts) {
      opts = opts || {};
      var c = kit.commands;
      if (!c) return Promise.resolve();
      algorithm = algorithm || kit.algorithm || kit.defaultLayout;
      var graph = c.layoutGraph();
      if (!graph.nodes.length) return Promise.resolve();
      // GL-26: pinned nodes are fixed in every layout.
      if (kit.isPinned) graph.nodes.forEach(function (n) { n.fixed = kit.isPinned(n.id); });
      if (!opts.initial && graph.nodes.length > LARGE_GRAPH &&
          !window.confirm(vsTf('js.graph.largeLayout', 'The diagram has {n} nodes; laying it out may take a while. Continue?', { n: graph.nodes.length }))) {
        return Promise.resolve();
      }
      var selected = c.selectedIds();
      var selection = selected.length >= 2 ? selected : null;
      var centre = selection ? null : (selected[0] || c.elementIdByIri(kit.pageIri));
      var title = vsTf('js.graph.cmd.layoutAs', 'Layout — {name}', { name: layoutName(algorithm) });
      return kit.task(vsT('js.graph.layingOut', 'Laying out…'), function (signal) {
        return window.VisotoLayout.layout(graph, {
          algorithm: algorithm,
          selection: selection,
          centre: centre,
          reversed: kit.reversedTypes ? kit.reversedTypes() : undefined, // GL-21, 22
        }, {
          force: c.force,
          elk: loadElk,
          signal: signal,
        }).then(function (positions) {
          if (signal.aborted) return;
          kit.batch(title, function () { c.applyPositions(positions); });
          kit.algorithm = algorithm;
          updateLayoutMenu();
          if (!selection) kit.fit();
        });
      }).catch(function (err) {
        if (err && err.name === 'AbortError') return;
        kit.showMessage(vsT('js.graph.layoutFailed', 'The layout could not be computed.'), {
          retry: function () { kit.layout(algorithm, opts); },
        });
      });
    };

    // GL-14: places newly added nodes (ids) with every other node fixed, by a
    // Network pass. Not an undo step of its own: the batch that added the
    // nodes removes them on undo, and re-adds them where they end up.
    kit.placeNew = function (ids) {
      var c = kit.commands;
      if (!c || !ids.length) return Promise.resolve();
      var fresh = {};
      ids.forEach(function (i) { fresh[i] = true; });
      var graph = c.layoutGraph();
      graph.nodes.forEach(function (n) { n.fixed = !fresh[n.id]; });
      return window.VisotoLayout.layout(graph, { algorithm: 'network' }, { force: c.force, elk: loadElk })
        .then(function (positions) {
          var moves = {};
          ids.forEach(function (i) { if (positions[i]) moves[i] = positions[i]; });
          c.movePositions(moves);
        });
    };

    function layoutName(algorithm) {
      switch (algorithm) {
        case 'tree-down': return vsT('js.graph.layout.treeDown', 'Tree ↓');
        case 'tree-right': return vsT('js.graph.layout.treeRight', 'Tree →');
        case 'radial': return vsT('js.graph.layout.radial', 'Radial');
        default: return vsT('js.graph.layout.network', 'Network');
      }
    }

    function updateLayoutMenu() {
      if (!toolbar) return;
      var current = kit.algorithm || kit.defaultLayout;
      toolbar.querySelectorAll('[data-graph-layout]').forEach(function (el) {
        var on = el.getAttribute('data-graph-layout') === current;
        el.classList.toggle('active', on);
        el.setAttribute('aria-checked', String(on));
      });
    }

    function updateHistoryButtons() {
      if (!kit.commands) return;
      var h = kit.commands.history;
      var u = h.undoStack[h.undoStack.length - 1];
      var r = h.redoStack[h.redoStack.length - 1];
      setEnabled('undo', !!u);
      setEnabled('redo', !!r);
      action('undo').forEach(function (el) {
        el.title = u ? vsTf('js.graph.undoStep', 'Undo: {step}', { step: commandTitle(u) }) : vsT('js.graph.undo', 'Undo');
      });
      action('redo').forEach(function (el) {
        el.title = r ? vsTf('js.graph.redoStep', 'Redo: {step}', { step: commandTitle(r) }) : vsT('js.graph.redo', 'Redo');
      });
    }

    function updateLanguage() {
      if (!toolbar || !kit.commands) return;
      var current = kit.commands.language();
      toolbar.querySelectorAll('[data-graph-lang]').forEach(function (el) {
        var on = el.getAttribute('data-graph-lang') === current;
        el.classList.toggle('active', on);
        el.setAttribute('aria-checked', String(on));
      });
      var label = toolbar.querySelector('[data-graph-lang-current]');
      if (label) label.textContent = current.toUpperCase();
    }

    var wired = false;
    function wireToolbar() {
      if (!toolbar || wired) return;
      wired = true;
      toolbar.addEventListener('click', function (e) {
        var target = e.target instanceof Element ? e.target : null;
        var layoutItem = target && target.closest('[data-graph-layout]');
        if (layoutItem) {
          e.preventDefault();
          kit.layout(layoutItem.getAttribute('data-graph-layout'));
          return;
        }
        var langItem = target && target.closest('[data-graph-lang]');
        if (langItem) {
          e.preventDefault();
          kit.commands.setLanguage(langItem.getAttribute('data-graph-lang'));
          updateLanguage();
          scheduleAutosave(); // the label language is saved with the canvas
          return;
        }
        var btn = target && target.closest('[data-graph-action]');
        if (!btn || btn.disabled || btn.classList.contains('disabled')) return;
        e.preventDefault();
        run(btn.getAttribute('data-graph-action'));
      });
    }

    function fileName(ext) {
      var title = card && card.querySelector('.card-title');
      var base = (title ? title.textContent : 'diagram').trim().replace(/[^\wÀ-ɏ-]+/g, '-') || 'diagram';
      return base + '.' + ext;
    }

    function run(name) {
      var c = kit.commands;
      if (!c) return;
      switch (name) {
        case 'undo': kit.undo(); break;
        case 'redo': kit.redo(); break;
        case 'fit': (kit.fitSelection || kit.fit)(); break; // GL-12: selection-aware
        case 'zoom-in': c.zoomIn(); break;
        case 'zoom-out': c.zoomOut(); break;
        case 'export-svg': c.exportSvg(fileName('svg')); break;
        case 'export-png': c.exportPng(fileName('png')); break;
        case 'print': c.print(); break;
        case 'save-as': saveAs(); break;
        case 'my-diagrams': showDiagrams(); break;
        case 'download': download(); break;
        case 'open-file': openFilePicker(); break;
        case 'reset': kit.reset(); break;
        case 'clear-all':
          if (c.elementCount() && window.confirm(vsT('js.graph.confirmClear', 'Remove every node from the diagram?'))) {
            kit.batch(vsT('js.graph.cmd.clearAll', 'Clear all'), c.clearAll);
          }
          break;
      }
    }

    // --- Saving (GL-35, 36, 50; storage in graph-store.js) ---------------------
    // The embed calls kit.boot(spec) once its workspace is mounted:
    //   spec.provider()     the DataProvider a restored diagram loads from
    //   spec.fingerprint    the page's starting resources (GL-35 change check)
    //   spec.fresh()        builds the page default (seeds, first layout) and
    //                       ends with kit.ready(); returns a promise
    // Boot restores, in this order: a canvas handed over by an endpoint switch
    // (Open file), this graph's autosave, else the page default.
    var Store = window.VisotoGraphStore;
    var tracking = false;          // autosave only after the canvas is ready
    var suppressAutosave = false;  // history.reset() itself is not a change
    var autosaveTimer = null;
    var AUTOSAVE_DELAY = 800;

    function endpointSlug() {
      if (typeof window.activeEndpointSlug === 'function') return window.activeEndpointSlug() || '';
      return new URLSearchParams(window.location.search).get('endpoint') || '';
    }

    kit.snapshot = function () {
      var c = kit.commands;
      return {
        format: Store.FORMAT,
        version: Store.VERSION,
        savedAt: new Date().toISOString(),
        page: window.location.pathname + window.location.search,
        graph: id,
        endpoint: endpointSlug(),
        language: c.language(),
        layout: kit.algorithm || kit.defaultLayout,
        pins: kit.pinnedIds ? kit.pinnedIds() : [],
        reversed: kit.reverseOverrides ? kit.reverseOverrides() : {},
        fingerprint: kit.spec ? kit.spec.fingerprint : '',
        diagram: c.exportDiagram(),
      };
    };

    kit.scheduleAutosave = function () { scheduleAutosave(); };
    function scheduleAutosave() {
      if (!tracking || suppressAutosave || !Store) return;
      clearTimeout(autosaveTimer);
      autosaveTimer = setTimeout(function () {
        if (!Store.autosave(id, kit.snapshot())) {
          kit.showMessage(vsT('js.graph.storageFull', 'The browser refused to store the diagram (storage full or blocked).'), { level: 'warning' });
        }
      }, AUTOSAVE_DELAY);
    }

    kit.boot = function (spec) {
      kit.spec = spec;
      var pending = Store && Store.takePending(id);
      if (pending) return restore(pending, false);
      var saved = Store && Store.loadAutosave(id);
      if (saved) return restore(saved, true);
      return spec.fresh();
    };

    function restore(saved, checkFingerprint) {
      var c = kit.commands;
      tracking = false;
      var done = busy(vsT('js.graph.loading', 'Loading graph…'));
      return c.importDiagram(kit.spec.provider(), saved.diagram).then(function () {
        done();
        if (saved.language) c.setLanguage(saved.language);
        if (kit.loadPins) kit.loadPins(saved.pins);
        if (kit.applyReverseOverrides) kit.applyReverseOverrides(saved.reversed);
        kit.algorithm = saved.layout || null;
        updateLanguage();
        updateLayoutMenu();
        // Sizes arrive with the first render; fit after it.
        setTimeout(kit.fit, 300);
        kit.ready();
        if (checkFingerprint && saved.fingerprint && kit.spec.fingerprint && saved.fingerprint !== kit.spec.fingerprint) {
          kit.showMessage(vsT('js.graph.pageChanged', 'Page content changed since this diagram was saved.'), {
            level: 'info',
            actionLabel: vsT('js.graph.reset', 'Reset diagram'),
            retry: function () { kit.reset(true); },
          });
        }
      }, function (err) {
        done();
        kit.showMessage(vsT('js.graph.restoreFailed', 'The saved diagram could not be loaded.'), {
          actionLabel: vsT('js.graph.reset', 'Reset diagram'),
          retry: function () { kit.reset(true); },
        });
      });
    }

    // Reset diagram (GL-35): back to the page default, fresh history.
    kit.reset = function (confirmed) {
      if (!kit.spec) return;
      if (!confirmed && !window.confirm(vsT('js.graph.confirmReset', 'Replace the diagram with the page default? Your changes are lost.'))) return;
      tracking = false;
      clearTimeout(autosaveTimer);
      if (Store) Store.clearAutosave(id);
      clearMessage();
      kit.algorithm = null;
      if (kit.loadPins) kit.loadPins([]);
      return kit.spec.fresh();
    };

    // Opens a saved canvas into this graph (GL-36, GL-50). A canvas from
    // another endpoint switches the page to it first (GL-37 merged): the
    // canvas waits in sessionStorage for the reloaded page.
    function open(saved) {
      if (!Store || !Store.valid(saved)) {
        kit.showMessage(vsT('js.graph.notADiagram', 'This is not a Visoto diagram file.'));
        return;
      }
      if (kit.commands.elementCount() &&
          !window.confirm(vsT('js.graph.confirmOpen', 'Replace the current diagram? Your changes are lost.'))) return;
      var slug = endpointSlug();
      if (saved.endpoint && slug && saved.endpoint !== slug) {
        var url = new URL(window.location.href);
        url.searchParams.set('endpoint', saved.endpoint);
        if (Store.setPending(id, saved)) {
          window.location.assign(url.toString());
          return;
        }
      }
      clearTimeout(autosaveTimer);
      restore(saved, false).then(scheduleSave);
    }
    function scheduleSave() { tracking = true; scheduleAutosave(); }

    function saveAs() {
      if (!kit.commands || !Store) return;
      var title = card && card.querySelector('.card-title');
      var name = window.prompt(vsT('js.graph.saveAsPrompt', 'Name for this diagram:'), title ? title.textContent.trim() : '');
      if (name === null) return;
      name = name.trim() || vsT('js.graph.untitled', 'Untitled diagram');
      if (Store.saveAs(name, kit.snapshot())) {
        kit.showMessage(vsTf('js.graph.savedAs', 'Saved as “{name}”.', { name: name }), { level: 'success' });
        setTimeout(clearMessage, 2500);
      } else {
        kit.showMessage(vsT('js.graph.storageFull', 'The browser refused to store the diagram (storage full or blocked).'), { level: 'warning' });
      }
    }

    function download() {
      if (!kit.commands) return;
      var blob = new Blob([JSON.stringify(kit.snapshot(), null, 2)], { type: 'application/json' });
      var a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = fileName('visoto-graph.json');
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(function () { URL.revokeObjectURL(a.href); }, 1000);
    }

    var fileInput = null;
    function openFilePicker() {
      if (!fileInput) {
        fileInput = document.createElement('input');
        fileInput.type = 'file';
        fileInput.accept = '.json,application/json';
        fileInput.hidden = true;
        fileInput.addEventListener('change', function () {
          var file = fileInput.files && fileInput.files[0];
          fileInput.value = '';
          if (!file) return;
          file.text().then(function (text) {
            var saved = null;
            try { saved = JSON.parse(text); } catch (e) { /* reported below */ }
            open(saved);
          });
        });
        document.body.appendChild(fileInput);
      }
      fileInput.click();
    }

    // My diagrams (GL-50): a modal listing the named saves of this browser.
    var modal = document.getElementById(id + '-diagrams');
    function showDiagrams() {
      if (!modal || !Store) return;
      renderDiagrams();
      var Modal = window.tabler && window.tabler.Modal;
      if (Modal) Modal.getOrCreateInstance(modal).show();
    }
    function renderDiagrams() {
      var list = modal.querySelector('[data-graph-diagrams-list]');
      var empty = modal.querySelector('[data-graph-diagrams-empty]');
      var entries = Store.list();
      list.replaceChildren();
      empty.hidden = entries.length > 0;
      var fmt = new Intl.DateTimeFormat(document.documentElement.lang || undefined, { dateStyle: 'medium', timeStyle: 'short' });
      entries.forEach(function (entry) {
        var row = document.createElement('div');
        row.className = 'list-group-item d-flex align-items-center gap-2';
        var text = document.createElement('div');
        text.className = 'flex-fill text-truncate';
        var name = document.createElement('div');
        name.className = 'fw-medium text-truncate';
        name.textContent = entry.name;
        var meta = document.createElement('div');
        meta.className = 'small text-secondary text-truncate';
        var when = entry.savedAt ? fmt.format(new Date(entry.savedAt)) : '';
        meta.textContent = [entry.endpoint, when].filter(Boolean).join(' · ');
        text.appendChild(name);
        text.appendChild(meta);
        row.appendChild(text);
        [['open', vsT('js.graph.open', 'Open'), 'btn-primary'],
         ['rename', vsT('js.graph.rename', 'Rename'), 'btn-ghost-secondary'],
         ['delete', vsT('js.graph.delete', 'Delete'), 'btn-ghost-danger']].forEach(function (b) {
          var btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'btn btn-sm ' + b[2];
          btn.textContent = b[1];
          btn.setAttribute('aria-label', b[1] + ': ' + entry.name);
          btn.addEventListener('click', function () { diagramAction(b[0], entry); });
          row.appendChild(btn);
        });
        list.appendChild(row);
      });
    }
    function diagramAction(what, entry) {
      if (what === 'open') {
        var saved = Store.get(entry.key);
        var Modal = window.tabler && window.tabler.Modal;
        if (Modal) Modal.getOrCreateInstance(modal).hide();
        open(saved);
      } else if (what === 'rename') {
        var name = window.prompt(vsT('js.graph.saveAsPrompt', 'Name for this diagram:'), entry.name);
        if (name !== null && name.trim()) Store.rename(entry.key, name.trim());
        renderDiagrams();
      } else if (what === 'delete') {
        if (window.confirm(vsTf('js.graph.confirmDelete', 'Delete “{name}”?', { name: entry.name }))) Store.remove(entry.key);
        renderDiagrams();
      }
    }

    // --- Fullscreen ------------------------------------------------------------
    // Toggling .graph-maximized on the card pins the frame (toolbar + canvas) to
    // the viewport (ontodia_overrides.css). No Fullscreen API: a fixed overlay
    // lets native F11 stack on top. GE only fits at mount, so re-fit whenever
    // the canvas box changes size.
    (function setupFullscreen() {
      var maximizeBtn = document.getElementById(id + '-maximize');
      var exitBtn = document.getElementById(id + '-exit');
      if (!card || !maximizeBtn || !exitBtn) return;
      function refit() { setTimeout(kit.fit, 100); }
      maximizeBtn.addEventListener('click', function () {
        // Expand a collapsed card first: fullscreen inside a display:none
        // wrapper shows nothing. Tabler's bundle is window.tabler, not bootstrap.
        var collapsed = document.getElementById('collapse-' + id);
        var Collapse = window.tabler && window.tabler.Collapse;
        if (collapsed && !collapsed.classList.contains('show') && Collapse) {
          Collapse.getOrCreateInstance(collapsed).show();
        }
        card.classList.add('graph-maximized');
        refit();
      });
      exitBtn.addEventListener('click', function () {
        card.classList.remove('graph-maximized');
        refit();
      });
      var resizeTimer;
      window.addEventListener('resize', function () {
        if (!card.classList.contains('graph-maximized')) return;
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(kit.fit, 150);
      });
      // While collapsed the canvas has no height; re-fit once it is back.
      var collapseEl = document.getElementById('collapse-' + id);
      if (collapseEl) collapseEl.addEventListener('shown.bs.collapse', refit);
    })();

    return kit;
  }

  window.VisotoGraph = {
    // The kit of graph `id` (for other scripts and for debugging).
    get: function (id) { return kits[id] || null; },
    create: create,
    readIsland: readIsland,
  };
})();
