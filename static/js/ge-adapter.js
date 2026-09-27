/* eslint-disable */
/*
  The one file that talks to Graph Explorer (window.GraphExplorer, GE 2.1.0 from
  the CDN). Everything else — graph-kit.js, sparql-graph.js, schema-graph.js —
  goes through window.VisotoGE, so a GE upgrade or a switch of library touches
  this file only.

  Workarounds that an upstream GE change would make unnecessary are tagged
  `GE-UPSTREAM: B<n>` (see .project/todo/graph-layout-plan.md, Part B); grep for
  the tag when bumping GE.
*/
(function () {
  'use strict';

  if (window.VisotoGE) return; // loaded by several partials on one page

  // Keep src and integrity in step: a bump without a new hash fails closed
  // (the browser refuses the script and every graph shows the load error).
  var GE_SRC = 'https://cdn.jsdelivr.net/npm/graph-explorer@2.1.0/dist/graph-explorer-full.min.js';
  var GE_SRI = 'sha384-Ynq7Fk7Xsafr7ssxX9KDGbSK7Ld3Dpx7L/VJfWW/Y3tqav33x0LhuVXTSgDuwTRo';
  var LOAD_TIMEOUT = 15000;

  // --- Loader ------------------------------------------------------------------
  // One shared <script> for every graph on the page. Readiness is polled rather
  // than taken from the load event: the script may already have loaded (another
  // graph injected it) before a listener could be attached.
  var loading = null;
  function load() {
    if (window.GraphExplorer) return Promise.resolve(window.GraphExplorer);
    if (loading) return loading;
    loading = new Promise(function (resolve, reject) {
      if (!document.querySelector('script[data-graph-explorer-loader]')) {
        var script = document.createElement('script');
        script.src = GE_SRC;
        script.integrity = GE_SRI;
        script.crossOrigin = 'anonymous';
        script.setAttribute('data-graph-explorer-loader', '');
        script.addEventListener('error', function () { reject(new Error('load')); });
        document.head.appendChild(script);
      }
      var waited = 0;
      var poll = setInterval(function () {
        if (window.GraphExplorer) {
          clearInterval(poll);
          resolve(window.GraphExplorer);
        } else if ((waited += 50) >= LOAD_TIMEOUT) {
          clearInterval(poll);
          reject(new Error('timeout'));
        }
      }, 50);
    });
    // A failed load may be retried (Retry button): forget the rejected promise
    // and the dead script tag so the next load() starts over.
    loading.catch(function () {
      loading = null;
      var dead = document.querySelector('script[data-graph-explorer-loader]');
      if (dead && !window.GraphExplorer) dead.remove();
    });
    return loading;
  }

  function GE() { return window.GraphExplorer; }

  // --- Undo history ------------------------------------------------------------
  // GE records its own actions (drag, remove, connections-menu add, drag-drop,
  // link-type visibility, force layout) into `model.history`, but 2.1.0 ships
  // only NonRememberingHistory, which throws on undo(). This is the remembering
  // implementation the interface was designed for, passed as the Workspace's
  // `history` prop.
  //
  // Batches nest: GE opens one on every pointer-down on the paper and, inside
  // some batches, calls `model.history.execute` directly rather than
  // `batch.history.execute`. So batch.history IS this history, and every
  // command goes into the innermost open batch. Stored batches become one
  // compound undo step; an empty batch leaves no step.
  //
  // GE-UPSTREAM: B1 (a remembering history ships with GE's undo buttons).
  function createHistory() {
    var listeners = [];
    var undoStack = [];
    var redoStack = [];
    var batches = []; // open batches, innermost last

    function changed() {
      listeners.forEach(function (fn) { fn({ hasChanges: undoStack.length > 0 }); });
    }

    function compound(title, commands) {
      return {
        title: title,
        invoke: function () {
          // `commands` are undo commands in the order they were recorded; run
          // them newest first. Their inverses, in that order, redo the batch —
          // and reversing again is correct for the next undo.
          var inverses = [];
          for (var i = commands.length - 1; i >= 0; i--) inverses.push(commands[i].invoke());
          return compound(title, inverses);
        },
      };
    }

    function titled(command, title) {
      if (!title) return command;
      return {
        title: title,
        invoke: function () { return titled(command.invoke(), title); },
      };
    }

    var history = {
      events: {
        on: function (name, fn) { if (name === 'historyChanged') listeners.push(fn); },
        off: function (name, fn) {
          var i = listeners.indexOf(fn);
          if (i >= 0) listeners.splice(i, 1);
        },
      },
      undoStack: undoStack,
      redoStack: redoStack,
      reset: function () {
        undoStack.length = 0;
        redoStack.length = 0;
        changed();
      },
      undo: function () {
        var command = undoStack.pop();
        if (!command) return;
        redoStack.push(command.invoke());
        changed();
      },
      redo: function () {
        var command = redoStack.pop();
        if (!command) return;
        undoStack.push(command.invoke());
        changed();
      },
      // A GE command returns its inverse, titled after the inverse action (undoing
      // "Remove element" is an "Add element"). Keep the title of what the user
      // did, on the step and on every inverse of it.
      execute: function (command) {
        history.registerToUndo(titled(command.invoke(), command.title));
      },
      registerToUndo: function (command) {
        var top = batches[batches.length - 1];
        if (top) {
          top.commands.push(command);
          return;
        }
        undoStack.push(command);
        redoStack.length = 0;
        changed();
      },
      startBatch: function (title) {
        var batch = {
          title: title,
          commands: [],
          history: history,
          store: function () {
            if (!close(batch)) return;
            if (!batch.commands.length) return;
            // An untitled batch (GE's removeItems) takes its first command's title.
            var t = batch.title || batch.commands[0].title;
            history.registerToUndo(batch.commands.length === 1 && !batch.title
              ? batch.commands[0]
              : compound(t, batch.commands));
          },
          discard: function () { close(batch); },
        };
        batches.push(batch);
        return batch;
      },
    };
    function close(batch) {
      var i = batches.indexOf(batch);
      if (i < 0) return false;
      batches.splice(i, 1);
      return true;
    }
    return history;
  }

  // --- Workspace ---------------------------------------------------------------
  // Graph Explorer's own toolbar is replaced by templates/partials/graph-toolbar.html
  // (GE-UPSTREAM: B4, an extensible toolbar).
  function render(container, props) {
    var ge = GE();
    ge.renderTo(ge.Workspace, container, Object.assign({ hideToolbar: true }, props));
  }

  // A SparqlDataProvider over OWLStatsSettings, with `settings` merged on top.
  // POST, never GET: see the comment at the call site in sparql-graph.js.
  function sparqlProvider(endpointUrl, settings) {
    var ge = GE();
    var provider = new ge.SparqlDataProvider(
      { endpointUrl: endpointUrl, acceptBlankNodes: false, queryMethod: ge.SparqlQueryMethod.POST },
      Object.assign({}, ge.OWLStatsSettings, settings)
    );
    // acceptBlankNodes:false keeps blank nodes off the canvas but not out of
    // rdf:type values: GE then asks classInfo for ids like `b0_genid-…` inside
    // <…>, which is no IRI, and the endpoint rejects the whole batch (400), so
    // every class in it loses its label. Ask only for absolute IRIs.
    var classInfo = provider.classInfo.bind(provider);
    provider.classInfo = function (params) {
      var ids = params.classIds.filter(function (id) { return /^[a-z][a-z0-9+.-]*:/i.test(id); });
      if (!ids.length) return Promise.resolve([]);
      return classInfo(Object.assign({}, params, { classIds: ids }));
    };
    return provider;
  }

  // Everything the toolbar and graph-kit do to a mounted workspace.
  function commands(workspace) {
    var model = workspace.getModel();
    return {
      model: model,
      history: model.history,
      undo: function () { workspace.undo(); },
      redo: function () { workspace.redo(); },
      zoomIn: function () { workspace.zoomIn(); },
      zoomOut: function () { workspace.zoomOut(); },
      zoomToFit: function () { workspace.zoomToFit(); },
      // The diagram as graph-layout.js's plain graph. Sizes come from the
      // rendered boxes, so render first. model.links holds only visible links:
      // GE removes a hidden link type's links (GL-21 "hidden = left out").
      layoutGraph: function () {
        workspace.getDiagram().performSyncUpdate();
        var nodes = model.elements.map(function (el) {
          return {
            id: el.id, iri: el.iri,
            x: el.position.x, y: el.position.y,
            width: el.size.width, height: el.size.height,
            fixed: false, // pinning (GL-26) arrives in A2
          };
        });
        var edges = [];
        model.links.forEach(function (link) {
          var s = model.sourceOf(link), t = model.targetOf(link);
          if (s && t) edges.push({ source: s.id, target: t.id, type: link.typeId });
        });
        return { nodes: nodes, edges: edges };
      },
      // Moves elements; call inside a history batch. The captured geometry is
      // the undo step; link vertices are cleared so edges are straight (GL-16).
      applyPositions: function (positions) {
        model.history.registerToUndo(GE().RestoreGeometry.capture(model));
        Object.keys(positions).forEach(function (id) {
          var el = model.getElement(id);
          if (el) el.setPosition(positions[id]);
        });
        model.links.forEach(function (link) {
          if (link.vertices && link.vertices.length) link.setVertices([]);
        });
        workspace.getDiagram().performSyncUpdate();
      },
      // The WebCola step of GE's own force layout, on plain nodes (mutated in
      // place). Same recipe as GE's forceLayout(), with our link length.
      // GE-UPSTREAM: B2 (an async layout hook would take graph-layout.js whole).
      force: function (nodes, links, linkLength) {
        var I = GE().InternalApi;
        var anyFixed = nodes.some(function (n) { return n.fixed; });
        if (anyFixed) {
          I.biasFreePadded(nodes, { x: 50, y: 50 }, function () {
            I.groupForceLayout({ nodes: nodes, links: links, preferredLinkLength: linkLength, avoidOvelaps: true });
          });
        } else {
          I.groupForceLayout({ nodes: nodes, links: links, preferredLinkLength: linkLength });
          I.biasFreePadded(nodes, { x: 50, y: 50 }, function () { I.groupRemoveOverlaps(nodes); });
        }
      },
      selectedIds: function () {
        var Element = GE().Element;
        return workspace.getEditor().selection
          .filter(function (item) { return item instanceof Element; })
          .map(function (el) { return el.id; });
      },
      elementIdByIri: function (iri) {
        var found = iri && model.elements.find(function (el) { return el.iri === iri; });
        return found ? found.id : null;
      },
      clearAll: function () { workspace.clearAll(); },
      exportSvg: function (name) { workspace.exportSvg(name); },
      exportPng: function (name) { workspace.exportPng(name); },
      print: function () { workspace.print(); },
      language: function () { return workspace.getDiagram().getLanguage(); },
      setLanguage: function (code) { workspace.changeLanguage(code); },
      elementCount: function () { return model.elements.length; },
    };
  }

  window.VisotoGE = {
    load: load,
    createHistory: createHistory,
    render: render,
    sparqlProvider: sparqlProvider,
    commands: commands,
  };
})();
