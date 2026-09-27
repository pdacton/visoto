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
  var activeKit = null;
  document.addEventListener('keydown', function (e) {
    if (!activeKit || !activeKit.commands || isTyping(e.target)) return;
    var mod = e.ctrlKey || e.metaKey;
    if (!mod || e.altKey) return;
    var key = e.key.toLowerCase();
    if (key === 'z' && !e.shiftKey) {
      e.preventDefault();
      activeKit.undo();
    } else if ((key === 'z' && e.shiftKey) || key === 'y') {
      e.preventDefault();
      activeKit.redo();
    }
  });

  function create(id) {
    var card = document.getElementById(id + '-card');
    var frame = document.getElementById(id + '-frame');
    var container = document.getElementById(id + '-root');
    var toolbar = document.getElementById(id + '-toolbar');
    var overlay = document.getElementById(id + '-overlay');

    var h = readIsland(id, '-height');
    if (container && h) container.style.height = h;

    var kit = {
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
        retry.textContent = vsT('js.graph.retry', 'Retry');
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
      }, props);
    };

    kit.attach = function (workspace) {
      if (kit.workspace === workspace) return kit;
      kit.workspace = workspace;
      kit.commands = window.VisotoGE.commands(workspace);
      kit.commands.history.events.on('historyChanged', updateHistoryButtons);
      wireToolbar();
      updateHistoryButtons();
      updateLanguage();
      return kit;
    };

    // The page's starting state is not an undo step: forget everything the
    // initial load recorded (element creation, the first layout).
    kit.ready = function () {
      if (kit.commands) kit.commands.history.reset();
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

    kit.layout = function () {
      if (!kit.commands) return Promise.resolve();
      var title = vsT('js.graph.layout.network', 'Network');
      return kit.task(vsT('js.graph.layingOut', 'Laying out…'), function () {
        kit.batch(vsTf('js.graph.cmd.layoutAs', 'Layout — {name}', { name: title }), function () {
          kit.commands.forceLayout();
        });
        kit.fit();
      });
    };

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
        var langItem = target && target.closest('[data-graph-lang]');
        if (langItem) {
          e.preventDefault();
          kit.commands.setLanguage(langItem.getAttribute('data-graph-lang'));
          updateLanguage();
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
        case 'fit': kit.fit(); break;
        case 'zoom-in': c.zoomIn(); break;
        case 'zoom-out': c.zoomOut(); break;
        case 'layout-network': kit.layout(); break;
        case 'export-svg': c.exportSvg(fileName('svg')); break;
        case 'export-png': c.exportPng(fileName('png')); break;
        case 'print': c.print(); break;
        case 'clear-all':
          if (c.elementCount() && window.confirm(vsT('js.graph.confirmClear', 'Remove every node from the diagram?'))) {
            kit.batch(vsT('js.graph.cmd.clearAll', 'Clear all'), c.clearAll);
          }
          break;
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
    create: create,
    readIsland: readIsland,
  };
})();
