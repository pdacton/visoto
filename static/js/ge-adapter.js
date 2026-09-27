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
    useClassTreeRoute(provider, endpointUrl);
    return provider;
  }

  // The Classes panel. GE's OWLStats class-tree query counts the instances of
  // every class over the whole store, which never finishes on LINDAS (60 s
  // proxy timeout, endless spinner). Endpoints with daily class statistics
  // (class_stats in visoto.config) serve the tree from /api/class-tree instead:
  // the declared classes that have instances, counts from the latest snapshot,
  // in GE's own result shape. The route answers 404 for other endpoints, and
  // GE's own query runs as before.
  //
  // GE's classTree() is kept for parsing (cycle breaking, labels, badges): for
  // one call its query is swapped for a marker, and executeSparqlQuery answers
  // the marker with the fetched result instead of asking the endpoint.
  var CLASS_TREE_MARKER = '# visoto:class-tree';
  function useClassTreeRoute(provider, endpointUrl) {
    var m = /^\/api\/sparql\?endpoint=([^&]+)$/.exec(endpointUrl || '');
    if (!m || !provider.settings) return;
    var treeUrl = '/api/class-tree?endpoint=' + m[1];
    var ownTree = provider.classTree.bind(provider);
    var exec = provider.executeSparqlQuery.bind(provider);
    var pending = null;
    provider.executeSparqlQuery = function (query) {
      if (pending && query.indexOf(CLASS_TREE_MARKER) >= 0) {
        var result = pending;
        pending = null;
        return Promise.resolve(result);
      }
      return exec(query);
    };
    provider.classTree = function () {
      return fetch(treeUrl, { headers: { Accept: 'application/sparql-results+json' } })
        .then(function (res) {
          if (res.status === 404) return null;
          if (!res.ok) throw new Error('class tree: HTTP ' + res.status);
          return res.json();
        })
        .then(function (json) {
          if (!json) return ownTree();
          var settings = provider.settings;
          var own = settings.classTreeQuery;
          pending = json;
          settings.classTreeQuery = CLASS_TREE_MARKER;
          try {
            return ownTree(); // reads the query synchronously, before its first await
          } finally {
            settings.classTreeQuery = own;
          }
        })
        .catch(function (err) {
          // An empty panel, not an endless spinner.
          console.error(err);
          return [];
        });
    };
  }

  // Everything the toolbar and graph-kit do to a mounted workspace.
  function commands(workspace) {
    var model = workspace.getModel();

    // GE 2.1 bug: ElementLayer.requestRedraw(element, RedrawFlags.None) — what
    // the model's changeCells event sends for an added or removed element —
    // returns early (`forAll | 0 === forAll`), so the layer never re-renders
    // for it. GE's own flows get away with it because a selection change
    // follows and redraws everything; an undo, redo or Visoto action does
    // not, and the canvas keeps showing removed nodes (and omits added ones).
    // Any element's redraw() recomputes the whole layer, so nudge one after
    // every history change. GE-UPSTREAM: B1 (report with the undo PR).
    // With no element left (redo of Clear all), a language round trip is the
    // one public path to the layer's redraw-all.
    function refresh() {
      var any = model.elements[0];
      if (any) {
        any.redraw();
        return;
      }
      var view = workspace.getDiagram();
      var lang = view.getLanguage();
      view.setLanguage(lang === 'en' ? 'de' : 'en');
      view.setLanguage(lang);
    }
    model.history.events.on('historyChanged', refresh);

    return {
      refresh: refresh,
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
      // --- Selection and element access (A2) -------------------------------
      // GE-UPSTREAM: B3 — multi-selection, box selection and group drag are
      // Visoto's (graph-selection.js); GE itself selects one cell per click.
      setSelection: function (ids) {
        var els = ids.map(function (id) { return model.getElement(id); }).filter(Boolean);
        workspace.getEditor().setSelection(els);
      },
      onSelectionChange: function (fn) {
        workspace.getEditor().events.on('changeSelection', fn);
      },
      elementIds: function () {
        return model.elements.map(function (el) { return el.id; });
      },
      exists: function (id) { return !!model.getElement(id); },
      box: function (id) {
        var el = model.getElement(id);
        return el && { id: id, x: el.position.x, y: el.position.y, width: el.size.width, height: el.size.height };
      },
      // Ids of the elements on the canvas linked to any of `ids`.
      neighbourIds: function (ids) {
        var out = {};
        ids.forEach(function (id) {
          var el = model.getElement(id);
          if (!el) return;
          el.links.forEach(function (link) {
            var s = model.sourceOf(link), t = model.targetOf(link);
            [s, t].forEach(function (n) { if (n && n.id !== id) out[n.id] = true; });
          });
        });
        return Object.keys(out);
      },
      pageToPaper: function (pageX, pageY) {
        return workspace._getPaperArea().pageToPaperCoords(pageX, pageY);
      },
      fitRect: function (rect) { workspace.zoomToFitRect(rect); },
      // Opens a history batch whose undo step restores every position as it
      // is NOW; the caller moves elements (movePositions) and stores it.
      startGeometryBatch: function (title) {
        var batch = model.history.startBatch(title);
        model.history.registerToUndo(GE().RestoreGeometry.capture(model));
        return batch;
      },
      // Positions without history: inside a geometry batch, or for elements
      // an enclosing batch already records (new elements being placed).
      movePositions: function (positions) {
        Object.keys(positions).forEach(function (id) {
          var el = model.getElement(id);
          if (el) el.setPosition(positions[id]);
        });
        workspace.getDiagram().performSyncUpdate();
      },
      removeElements: function (ids) {
        var els = ids.map(function (id) { return model.getElement(id); }).filter(Boolean);
        if (els.length) workspace.getEditor().removeItems(els);
      },
      // Neighbour IRIs of `ids` in the data (not only on the canvas), with the
      // canvas element each was found from: [{ iri, from }].
      neighbourIris: function (ids, limit) {
        var provider = model.dataProvider;
        var onCanvas = {};
        model.elements.forEach(function (el) { onCanvas[el.iri] = true; });
        return Promise.all(ids.map(function (id) {
          var el = model.getElement(id);
          if (!el) return [];
          return provider.linkElements({ elementId: el.iri, linkId: null, offset: 0, limit: limit })
            .then(function (dict) {
              return Object.keys(dict).map(function (iri) { return { iri: iri, from: id }; });
            });
        })).then(function (lists) {
          var seen = {};
          return [].concat.apply([], lists).filter(function (item) {
            if (onCanvas[item.iri] || seen[item.iri]) return false;
            seen[item.iri] = true;
            return true;
          });
        });
      },
      // Adds elements (recorded in the open batch) at the given positions and
      // loads their data and links. Resolves to the new element ids.
      addElements: function (items) {
        var ids = [];
        items.forEach(function (item) {
          var el = model.createElement(item.iri);
          if (!el) return;
          el.setPosition({ x: item.x, y: item.y });
          ids.push(el.id);
        });
        var iris = items.map(function (item) { return item.iri; });
        var loading = Promise.resolve(model.requestElementData(iris))
          .then(function () { return model.requestLinksOfType(); })
          .then(function () { workspace.getDiagram().performSyncUpdate(); return ids; });
        return { ids: ids, loaded: loading };
      },
      // --- A4: link types, search, details, menu additions ------------------
      // Link types drawn on the canvas with their edge counts and labels (GL-20).
      linkTypeCounts: function () {
        var view = workspace.getDiagram();
        var counts = {};
        model.links.forEach(function (link) { counts[link.typeId] = (counts[link.typeId] || 0) + 1; });
        return Object.keys(counts).map(function (typeId) {
          var type = model.getLinkType(typeId);
          return {
            id: typeId,
            label: view.formatLabel(type ? type.label : [], typeId),
            count: counts[typeId],
          };
        }).sort(function (a, b) { return a.label.localeCompare(b.label); });
      },
      // GL-47: resources whose label matches `text`, via the provider's
      // filter() — the lookup GE's Instances panel uses.
      search: function (text, limit) {
        var view = workspace.getDiagram();
        return model.dataProvider.filter({
          text: text, offset: 0, limit: limit, languageCode: view.getLanguage(),
        }).then(function (dict) {
          return Object.keys(dict).map(function (iri) {
            return { iri: iri, label: view.formatLabel(dict[iri].label.values, iri) };
          });
        });
      },
      iriOnCanvas: function (iri) {
        var el = model.elements.find(function (e) { return e.iri === iri; });
        return el ? el.id : null;
      },
      isExpanded: function (id) {
        var el = model.getElement(id);
        return !!(el && el.isExpanded);
      },
      // GL-48, inside the caller's batch: one GE command per element.
      setExpanded: function (ids, expanded) {
        var setElementExpanded = GE().setElementExpanded;
        ids.forEach(function (id) {
          var el = model.getElement(id);
          if (el && el.isExpanded !== expanded) model.history.execute(setElementExpanded(el, expanded));
        });
      },
      // GL-14 / GL-15 for additions from GE's connections menu. `before(n)`
      // may veto the addition (returns false); `after(ids)` runs once GE has
      // placed the new elements around their source. Drag-and-drop from the
      // panels also fires GE's addElements, but lands where it was dropped,
      // so only the menu path is wrapped.
      // GE-UPSTREAM: B2 (placement through the layout hook).
      onMenuAdd: function (before, after) {
        var editor = workspace.getEditor();
        var inner = editor.onAddElementsInConnectionMenu.bind(editor);
        var pending = false;
        editor.onAddElementsInConnectionMenu = function (iris, target, linkType) {
          if (before && before(iris.length) === false) return;
          pending = true;
          return inner(iris, target, linkType);
        };
        editor.events.on('addElements', function (e) {
          if (!pending) return;
          pending = false;
          if (after) after(e.elements.map(function (el) { return el.id; }));
        });
      },
      // --- A5: classes, namespaces, find -----------------------------------
      // What graph-filter.js needs to know about an element.
      describe: function (id) {
        var el = model.getElement(id);
        if (!el) return null;
        var data = el.data || {};
        return {
          id: id,
          iri: el.iri,
          types: data.types || [],
          label: workspace.getDiagram().formatLabel(data.label ? data.label.values : [], el.iri),
        };
      },
      classLabel: function (classIri) {
        var cls = model.getClass(classIri) || model.createClass(classIri);
        return workspace.getDiagram().formatLabel(cls ? cls.label : [], classIri);
      },
      // GL-32: blur every element for which `match(id)` is false; null clears.
      highlight: function (match) {
        workspace.getDiagram().setHighlighter(match ? function (item) {
          return item instanceof GE().Element ? match(item.id) : false;
        } : undefined);
      },
      // GL-49 / GL-52, inside the caller's batch. hide() removes elements and
      // returns what show() needs to bring them back; show() re-creates them
      // where they were and reloads their data and links.
      hide: function (ids) {
        var records = [];
        var els = [];
        ids.forEach(function (id) {
          var el = model.getElement(id);
          if (!el) return;
          els.push(el);
          records.push({ iri: el.iri, x: el.position.x, y: el.position.y, expanded: !!el.isExpanded, types: (el.data && el.data.types) || [] });
        });
        if (els.length) workspace.getEditor().removeItems(els);
        return records;
      },
      show: function (records) {
        var ids = [];
        records.forEach(function (r) {
          var el = model.createElement(r.iri);
          if (!el) return;
          el.setPosition({ x: r.x, y: r.y });
          if (r.expanded) el.setExpanded(true);
          ids.push(el.id);
        });
        var iris = records.map(function (r) { return r.iri; });
        return {
          ids: ids,
          loaded: Promise.resolve(model.requestElementData(iris))
            .then(function () { return model.requestLinksOfType(); })
            .then(function () { workspace.getDiagram().performSyncUpdate(); return ids; }),
        };
      },
      // --- A6: what is drawn --------------------------------------------------
      // For Turtle export (GL-38) and the list view (GL-39): every node with
      // its types and labels (property rows only for expanded nodes — only
      // those are drawn), and every visible edge.
      drawnGraph: function () {
        var view = workspace.getDiagram();
        var elements = model.elements.map(function (el) {
          var data = el.data || {};
          return {
            id: el.id,
            iri: el.iri,
            label: view.formatLabel(data.label ? data.label.values : [], el.iri),
            labels: data.label ? data.label.values : [],
            types: data.types || [],
            typeLabels: (data.types || []).map(function (t) {
              var cls = model.getClass(t);
              return view.formatLabel(cls ? cls.label : [], t);
            }),
            properties: el.isExpanded ? (data.properties || {}) : {},
          };
        });
        var links = [];
        model.links.forEach(function (link) {
          var s = model.sourceOf(link), t = model.targetOf(link);
          if (!s || !t) return;
          var type = model.getLinkType(link.typeId);
          links.push({
            source: s.iri, target: t.iri, type: link.typeId,
            sourceId: s.id, targetId: t.id,
            typeLabel: view.formatLabel(type ? type.label : [], link.typeId),
          });
        });
        return { elements: elements, links: links };
      },
      // --- Saving (A3) -----------------------------------------------------
      // GE's SerializedDiagram: element ids, IRIs, positions, expanded state,
      // links with vertices, link-type visibility. Labels and data are not in
      // it; importDiagram re-fetches them, so a restored canvas is never stale.
      exportDiagram: function () {
        return JSON.parse(JSON.stringify(model.exportLayout()));
      },
      // Replaces the canvas with `diagram`, loading element data and the links
      // between the elements from `provider`. An IRI that no longer resolves
      // stays on the canvas as a bare IRI.
      importDiagram: function (provider, diagram) {
        return Promise.resolve(model.importLayout({
          dataProvider: provider,
          diagram: diagram,
          preloadedElements: {},
          validateLinks: true,
        })).then(function () { workspace.getDiagram().performSyncUpdate(); });
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

  // GE's paper pointer events, reduced to what graph-selection.js needs:
  // { elementId, sourceEvent, click } (elementId null on empty paper or a link).
  function pointerHandler(fn) {
    return function (e) {
      var Element = GE().Element;
      fn({
        elementId: e.target instanceof Element ? e.target.id : null,
        onLink: !!e.target && !(e.target instanceof Element),
        sourceEvent: e.sourceEvent,
        click: !!e.triggerAsClick,
      });
    };
  }

  window.VisotoGE = {
    pointerHandler: pointerHandler,
    load: load,
    createHistory: createHistory,
    render: render,
    sparqlProvider: sparqlProvider,
    commands: commands,
  };
})();
