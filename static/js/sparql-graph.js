/* eslint-disable */
/*
  Behaviour for the "sparqlGraph" partial (templates/partials/sparql-graph.html).

  Extracted from an inline <script> in that partial so the code is cacheable,
  lintable and editable as JavaScript. The template now emits only markup and
  JSON data islands; per-instance values arrive as data attributes.

  Attributes read from the root element:
    data-sparql-graph       marker; presence means "initialize me"
    data-sparql-graph-id    DOM id prefix for this instance's islands/elements
    data-sparql-graph-lazy  "true" to defer init until a 'graph:init' event

  Two modes:
    - browse (default): seed IRIs, then Graph Explorer's SparqlDataProvider
      fetches their data and the links between them from the endpoint.
    - construct (a -construct island is present): one CONSTRUCT is posted,
      and its triples ARE the diagram, served from an in-memory provider
      (static/js/graph-memory-store.js). For views whose edges do not exist
      verbatim in the data, e.g. an ontology's domain/range pairs drawn as
      one association per property.

  Loading, the toolbar, undo, fullscreen and error messages are shared with
  schema-graph.js through static/js/graph-kit.js; GE itself is reached through
  static/js/ge-adapter.js.
*/
(function () {
  'use strict';

  // boot() runs at (or after) DOMContentLoaded, so the DOM is ready by the time
  // initSparqlGraph is called and the readyState guards the inline block needed
  // are no longer necessary.
  function initSparqlGraph(root) {

    var ID = root.getAttribute('data-sparql-graph-id');
    var kit = window.VisotoGraph.create(ID);
    var readIsland = kit.readIsland;

    // When lazy, init is deferred until the container is first made visible (the caller
    // dispatches a 'graph:init' event at the -root element). This avoids Graph Explorer
    // measuring a zero-size box if the container starts hidden (e.g. behind a view toggle).
    var LAZY = root.getAttribute('data-sparql-graph-lazy') === 'true';

    function init() {
      var container = kit.container;
      if (!container) return;

      var AVAILABLE_ICONS = readIsland('-available-icons') || {};
      var ENDPOINT_URL = readIsland('-endpoint-url') || '/api/sparql';

      // Build the starting IRI list: iris[] + optional single iri + ?iri= URL param.
      var startIris = [];
      var irisIsland = readIsland('-iris');
      if (Array.isArray(irisIsland)) {
        irisIsland.forEach(function(v) { if (v) startIris.push(v); });
      }
      var singleIri = readIsland('-iri');
      if (singleIri) startIris.push(singleIri);
      var urlIri = new URLSearchParams(window.location.search).get('iri');
      if (urlIri) startIris.push(urlIri);
      // De-duplicate while preserving order.
      startIris = startIris.filter(function(v, i) { return startIris.indexOf(v) === i; });
      // Fall back to a default when nothing was provided.
      if (startIris.length === 0) startIris = ['http://www.w3.org/2000/01/rdf-schema#Class'];

      // Construct mode: the query text, and the store built from its result
      // before the workspace mounts (see render() at the end of init).
      var CONSTRUCT = readIsland('-construct');
      // GL-2: a constructed diagram is a class model, read as a tree; a browse
      // graph is a network. Radial centres on the page's resource (GL-19).
      kit.defaultLayout = CONSTRUCT ? 'tree-right' : 'network';
      kit.searchEndpoint = !CONSTRUCT; // Add resource: endpoint search (GL-47)
      // GL-10 / GL-49: "a class includes its subclasses". Asked of the endpoint
      // for the classes on the canvas only (both ends bound by VALUES), so the
      // property path stays cheap. A constructed diagram's nodes are classes
      // themselves; their rdf:type is the metaclass, with nothing to nest.
      if (!CONSTRUCT) {
        kit.superclasses = function (types) {
          var iris = types.filter(validIri);
          if (iris.length < 2) return Promise.resolve({});
          var values = iris.map(function (t) { return '<' + t + '>'; }).join(' ');
          var query = 'SELECT ?sub ?sup WHERE { VALUES ?sub { ' + values + ' } VALUES ?sup { ' + values +
            ' } ?sub <http://www.w3.org/2000/01/rdf-schema#subClassOf>+ ?sup . FILTER(?sub != ?sup) }';
          return fetch(ENDPOINT_URL, {
            method: 'POST',
            headers: { 'Content-Type': 'application/sparql-query', 'Accept': 'application/sparql-results+json' },
            body: query,
          }).then(function (res) {
            if (!res.ok) throw new Error('SPARQL request failed: ' + res.status);
            return res.json();
          }).then(function (json) {
            var map = {};
            json.results.bindings.forEach(function (b) {
              (map[b.sub.value] = map[b.sub.value] || []).push(b.sup.value);
            });
            return map;
          });
        };
      }
      kit.pageIri = singleIri || urlIri;
      var constructedStore = null;

      function onWorkspaceMounted(workspace) {
        if (!workspace) return;
        kit.attach(workspace);
        enableElementResize(container, workspace.getModel());

        if (constructedStore) {
          mountConstructed(workspace, constructedStore);
          return;
        }

        // OWLStatsSettings with LINDAS-specific label properties and prefixes.
        var settings = {
          defaultPrefix:
            'PREFIX rdf:    <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\n' +
            'PREFIX rdfs:   <http://www.w3.org/2000/01/rdf-schema#>\n' +
            'PREFIX owl:    <http://www.w3.org/2002/07/owl#>\n' +
            'PREFIX skos:   <http://www.w3.org/2004/02/skos/core#>\n' +
            'PREFIX schema: <http://schema.org/>\n' +
            'PREFIX schch:  <https://schema.ld.admin.ch/>\n' +
            'PREFIX gtfs:   <http://vocab.gtfs.org/terms#>\n' +
            'PREFIX vl:     <https://version.link/>\n' +
            'PREFIX rico:   <https://www.ica.org/standards/RiC/ontology#>\n' +
            'PREFIX regch:  <https://register.ld.admin.ch/>\n' +
            'PREFIX refch:  <https://reference.data.admin.ch/>\n' +
            'PREFIX dcterms: <http://purl.org/dc/terms/>\n',
          dataLabelProperty: 'schema:name | skos:prefLabel | dcterms:title | rdfs:label',
          schemaLabelProperty: 'schema:name | skos:prefLabel | dcterms:title | rdfs:label',
        };

        // POST, not GET: GET puts the whole query in the URL, and the edge query
        // is the one that outgrows it.
        //
        // Graph Explorer's linksInfoQuery asks "which of these nodes link to
        // which" by listing EVERY seed IRI TWICE:
        //
        //     SELECT ?source ?type ?target WHERE {
        //         VALUES (?source) {${ids}}
        //         VALUES (?target) {${ids}}
        //     }
        //
        // With 41 seeds (the Agate page) plus GE's ~700-char PREFIX header that
        // is an 8,727-character URL, and LINDAS answers
        // "431 Request Header Fields Too Large" — measured cliff on
        // cached.lindas.admin.ch: 7,506 chars still 200, 8,006 already 431
        // (past ~8.2 KB it turns into a 400). The links request fails, and the
        // graph renders every node with no edge between any of them. Small
        // graphs stayed under the limit, which is why this only showed up on the
        // most connected resources.
        //
        // The SPARQL itself is fine — the same query sent by POST returns its 42
        // rows from both cached.lindas.admin.ch and ld.admin.ch. This is a
        // transport limit, not the query-shape problem some stores have with the
        // double-VALUES cross product.
        //
        // GE posts the raw query with Content-Type: application/sparql-query
        // (not form-encoded); both configured LINDAS endpoints accept that form.
        // The trade-off is that POSTed queries are not cached by intermediaries
        // the way GETs are, so the graph loses some benefit of the "cached"
        // endpoint — worth it against edges that silently vanish above a size
        // nobody can predict from the page.
        var dataProvider = window.VisotoGE.sparqlProvider(ENDPOINT_URL, settings);

        // Set data.image from an element's own IRI, preserving the rest of its
        // data. setData replaces the model wholesale, so the existing fields are
        // carried over; GE re-renders from the model, so this survives.
        function stampIcon(element, iri) {
          if (!element) return;
          var url = ICONS.resolve(iri, [], AVAILABLE_ICONS);
          if (!url) return;
          var data = element.data || {};
          if (data.image) return; // a real image from the data wins
          element.setData(Object.assign({}, data, { id: data.id || iri, iri: iri, image: url }));
        }

        // Stamp each element's own-IRI icon onto the model as data.image.
        //
        // typeStyleResolver() below covers instances, whose rdf:type names a real
        // class. It cannot cover a CLASS node (a class resource page puts one at the
        // centre): its type is owl:Class, so every class would resolve to the same
        // generic icon. GE 1.3.0 let a StandardTemplate subclass patch iconUrl into
        // this.props before super.render(); in 2.x props are rebuilt from the model
        // on every render, so that patch is discarded (and React 19 makes props
        // read-only besides). data.image survives because it IS the model, and
        // renderThumbnail() prefers it over iconUrl.
        var innerElementInfo = dataProvider.elementInfo.bind(dataProvider);
        dataProvider.elementInfo = function (params) {
          return innerElementInfo(params).then(function (dict) {
            Object.keys(dict).forEach(function (iri) {
              if (dict[iri].image) return; // a real image from the data wins
              var url = ICONS.resolve(iri, [], AVAILABLE_ICONS);
              if (url) dict[iri].image = url;
            });
            return dict;
          });
        };
        // Endpoint errors surface as an inline message with Retry (GL-45).
        kit.guardProvider(dataProvider);

        var model = workspace.getModel();
        // data-sparql-graph-hide-type-edges hides rdf:type / rdfs:subClassOf
        // edges. GE loads those alongside the data relations, and on a diagram
        // whose point is the architecture they are pure noise: one edge per node
        // fanning into a handful of class boxes, drowning the consumes/operates
        // /contains edges the reader came for.
        //
        // Off by default — on an ordinary resource graph "what type is this"
        // is worth seeing, and linkTemplateResolver already draws it dashed to
        // set it apart. Hiding is opt-in per instance.
        //
        // linkTypeOptions is GE's own mechanism (importLayout -> setLinkSettings),
        // so the edges are never requested rather than fetched and then hidden.
        var linkTypeOptions;
        if (root.getAttribute('data-sparql-graph-hide-type-edges') === 'true') {
          linkTypeOptions = [
            { property: 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type', visible: false },
            { property: 'http://www.w3.org/2000/01/rdf-schema#subClassOf', visible: false },
          ];
        }

        // Seed only once importLayout has settled. It registers the provider's
        // link types from an async linkTypes() call; seeding meanwhile lets
        // requestLinksOfType register some of them first, and GE then throws
        // "Link type '<iri>' already exists" and draws nothing (the Dependencies
        // graph on system-map pages, whose link types load slowly).
        //
        // kit.boot restores an autosaved or opened canvas instead when there is
        // one; fresh() is also what Reset diagram runs.
        kit.boot({
          provider: function () { return dataProvider; },
          fingerprint: window.VisotoGraphStore.fingerprint(startIris),
          fresh: function () {
            return Promise.resolve(model.importLayout({
              dataProvider: dataProvider,
              preloadedElements: {},
              layoutData: undefined,
              diagram: linkTypeOptions ? { linkTypeOptions: linkTypeOptions } : undefined,
            })).catch(function () { /* seed anyway: an edgeless graph beats none */ }).then(seed);
          },
        });

        // Resolves once the first layout is applied and the history reset.
        function seed() {
          return new Promise(function (resolve) {
          // Load each starting element and fetch its data.
          //
          // The icon is stamped on at creation, BEFORE requestElementData, because
          // elementInfo cannot be relied on to return anything: a class IRI is
          // often only implied by its instances' rdf:type and carries no triples
          // of its own (https://schema.ld.admin.ch/Canton has zero), so the query
          // comes back empty and the element keeps the placeholder data GE built
          // from the IRI. The elementInfo wrapper below then has no dictionary
          // entry to enrich. Resolving from the IRI here needs no data at all --
          // which is the whole point, since the IRI is all that exists.
          // Seed positions are a RING, not a row.
          //
          // A single row (every node at the same y, x stepping right) is fine for
          // the one-node case the base layout uses, but it is a degenerate starting
          // state for a force layout: with no vertical displacement between any two
          // nodes there is almost no vertical force to separate them, so a
          // multi-node seed settles back into a flat band that zoomToFit then
          // squeezes into an unreadable strip. Distributing the seed around a
          // circle gives the simulation a 2D starting spread and it resolves into
          // an actual graph.
          //
          // The radius grows with the node count so the ring stays roughly evenly
          // spaced instead of overlapping as the seed set gets larger.
          var CENTER_X = 400;
          var CENTER_Y = 300;
          var radius = Math.max(200, startIris.length * 30);
          startIris.forEach(function(startIRI, index) {
            var element = model.createElement(startIRI);
            if (element) {
              var angle = (2 * Math.PI * index) / startIris.length;
              element.setPosition({
                x: CENTER_X + radius * Math.cos(angle),
                y: CENTER_Y + radius * Math.sin(angle),
              });
              stampIcon(element, startIRI);
            }
          });
          // requestElementData fetches each element's OWN data (labels, types,
          // properties) but NOT the edges between them — those are a separate
          // round trip via requestLinksOfType. Without it a multi-IRI seed draws as
          // unconnected boxes: every node present, no line between any two.
          //
          // The gap went unnoticed for as long as the only caller was the resource
          // page's Graph view in layout/base.html, which seeds ONE IRI and so has
          // no edges to miss. A seed set built from a query (the dependency graph
          // on the SoftwareApplication pages) is the case that needs it.
          //
          // The two calls MUST be sequenced. Both register link types as they go,
          // and the model throws "Link type '<iri>' already exists" if the second
          // registration lands while the first is still in flight — which aborts
          // link loading entirely and leaves the graph edgeless, the very symptom
          // this call is here to fix. Chaining off requestElementData's promise
          // (rather than firing both at once) keeps the registrations ordered.
          //
          // The layout runs AFTER the links land, not on a bare timer. forceLayout
          // positions nodes by their edges, so laying out while the graph is still
          // edgeless just spreads the seed row evenly — the nodes keep the flat
          // line they were created on and never regroup once the edges arrive.
          //
          // relayout() is called from both the success and the failure path (and
          // from a timer, in case neither promise ever settles) so an endpoint that
          // refuses the links query still gets a laid-out, if edgeless, graph. It
          // guards against running twice, which would otherwise re-scatter a graph
          // the reader may already have started dragging.
          var didLayout = false;
          var loaded = kit.busy(vsT('js.graph.loading', 'Loading graph…'));
          function relayout() {
            if (didLayout) return;
            didLayout = true;
            loaded();
            kit.layout(null, { initial: true }).then(kit.ready).then(resolve);
          }

          var elementsLoaded = model.requestElementData(startIris);
          if (elementsLoaded && elementsLoaded.then && model.requestLinksOfType) {
            elementsLoaded
              .then(function() { return model.requestLinksOfType(); })
              .then(relayout)
              .catch(relayout);
          }
          // Fallback: fires only if the chain above never settles (or was never
          // started, on a build whose model lacks requestLinksOfType).
          setTimeout(relayout, 4000);
          });
        }
      }

      // --- Construct mode ---------------------------------------------------------------
      // The page IRI substituted for `??`, as in server-side queries. Validated
      // because it is spliced into query text between angle brackets.
      // Drag a node box's right edge to widen it. GE has no element resize: it
      // sizes a box from its rendered DOM, re-measuring only when the element
      // redraws. So the drag sets an inline width on the template root (React
      // never touches it — the root's only prop is className) and calls
      // redraw() so links re-attach to the new bounds.
      //
      // The listener runs in the CAPTURE phase on the container GE renders
      // into: React's onMouseDown (which starts an element drag) listens on the
      // same node in the bubble phase, so stopPropagation here keeps the resize
      // from also moving the box. The ew-resize cursor is a ::after strip in
      // ontodia_overrides.css, RESIZE_EDGE px wide to match the hit test.
      var RESIZE_EDGE = 8;
      var RESIZE_MIN = 180; // GE's own min-width for a standard template
      function enableElementResize(container, model) {
        if (container.dataset.vsElementResize) return;
        container.dataset.vsElementResize = '1';
        container.addEventListener('mousedown', function (e) {
          if (e.button !== 0 || !(e.target instanceof Element)) return;
          var box = e.target.closest('.graph-explorer-standard-template');
          var host = box && box.closest('[data-element-id]');
          if (!host) return;
          var rect = box.getBoundingClientRect();
          if (e.clientX < rect.right - RESIZE_EDGE) return;
          var element = model.getElement(host.getAttribute('data-element-id'));
          if (!element) return;
          e.preventDefault();
          e.stopPropagation();

          var startX = e.clientX;
          var startWidth = box.offsetWidth;
          var scale = rect.width / startWidth || 1; // screen px per paper px (zoom)
          function onMove(ev) {
            var width = Math.max(RESIZE_MIN, startWidth + (ev.clientX - startX) / scale);
            box.style.width = width + 'px';
            box.style.maxWidth = 'none';
            element.redraw();
          }
          function onUp() {
            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onUp);
          }
          document.addEventListener('mousemove', onMove);
          document.addEventListener('mouseup', onUp);
        }, true);
      }

      function validIri(iri) {
        return /^https?:\/\/[^<>"{}|\\^`\s]+$/.test(iri);
      }

      function fetchConstructed() {
        var iri = singleIri || urlIri;
        if (CONSTRUCT.indexOf('??') >= 0 && !(iri && validIri(iri))) {
          return Promise.reject(new Error('No valid resource IRI for the graph query'));
        }
        var query = CONSTRUCT.split('??').join('<' + iri + '>');
        return fetch(ENDPOINT_URL, {
          method: 'POST',
          headers: { 'Content-Type': 'application/sparql-query', 'Accept': 'application/n-triples' },
          body: query,
        }).then(function(res) {
          if (!res.ok) throw new Error('SPARQL request failed: ' + res.status);
          return res.text();
        }).then(function(text) {
          var MEM = window.VisotoMemoryGraph;
          return MEM.buildStore(MEM.parseNTriples(text), AVAILABLE_ICONS);
        });
      }

      // Every constructed node is placed on a grid, nodes with attribute rows
      // are expanded (the rest would show GE's "no properties" placeholder),
      // then force layout once the links are in.
      function mountConstructed(workspace, store) {
        var MEM = window.VisotoMemoryGraph;
        var model = workspace.getModel();
        kit.boot({
          provider: function () { return MEM.makeProvider(store); },
          fingerprint: window.VisotoGraphStore.fingerprint(Object.keys(store.elements)),
          // Seeded after importLayout settles, as in browse mode (see seed()).
          fresh: function () {
            return Promise.resolve(model.importLayout({
              dataProvider: MEM.makeProvider(store),
              preloadedElements: {},
              layoutData: undefined,
            })).then(function () { return seedConstructed(workspace, store); });
          },
        });
      }

      function seedConstructed(workspace, store) {
        var model = workspace.getModel();
        var iris = Object.keys(store.elements);
        var cols = Math.ceil(Math.sqrt(iris.length));
        iris.forEach(function(iri, i) {
          var el = model.createElement(iri);
          if (el) el.setPosition({ x: (i % cols) * 300, y: Math.floor(i / cols) * 200 });
        });

        return Promise.resolve(model.requestElementData(iris))
          .then(function() { return model.requestLinksOfType(); })
          .then(function() {
            model.elements.forEach(function(el) {
              var data = store.elements[el.iri];
              if (data && Object.keys(data.properties).length > 0 && el.setExpanded) el.setExpanded(true);
            });
            // After the expanded boxes have rendered, so their sizes are real.
            // Small islands (a property without domain or range: two boxes, one
            // edge) are packed in rows by the layout itself (GL-17).
            return new Promise(function (resolve) { setTimeout(resolve, 300); });
          })
          .then(function() { return kit.layout(null, { initial: true }); })
          .then(kit.ready);
      }

      // Icon resolution is shared with schema-graph.js and mirrors internal/icon
      // (see static/js/visoto-icons.js) — one definition of "own name first, then
      // any exact type match, then any .fallback".
      var ICONS = window.VisotoIcons;

      // typeStyleResolver: resolves the icon from rdf:type values (instances like Bern).
      function typeStyleResolver(types) {
        return { icon: ICONS.resolve('', types, AVAILABLE_ICONS) || '/static/img/resource/defaultClass.svg' };
      }

      // No element template override: icons ride on data.image (see above), which
      // StandardTemplate.renderThumbnail() renders directly.

      // linkTemplateResolver: style all edges with Tabler-palette neutrals instead of
      // the library's default black arrowheads / per-type colors.
      // Hex values mirror Tabler CSS vars (SVG presentation attrs can't use var()):
      //   #e6e7e9  -> line + arrowhead — matches Tabler --tblr-border-color, so edges
      //               read as light structural lines rather than heavy strokes
      //   #1d273b  -> label text (medium weight, not bold)
      //
      // Why the arrowhead colours here actually take effect (an earlier attempt didn't):
      // the library merges this template into its default via defaultsDeep(), which only
      // FILLS MISSING keys. The default markerTarget is
      //   { d:"M0,0 L0,8 L9,4 z", width:9, height:8, fill:"black" }  (NO stroke key).
      // So we must set BOTH fill AND stroke on markerTarget — otherwise stroke stays
      // unset (fine) but any colour we only put on `connection.stroke` never reaches the
      // arrowhead, because the arrowhead is a separate <marker><path> whose fill is what
      // paints it. Setting markerTarget.fill is the part that recolours the arrow.

      var LINK_LINE = '#e6e7e9';
      var LINK_LABEL = '#1d273b';

      // Full IRIs the resolver dispatches on (it receives the EXPANDED link type IRI,
      // not the prefixed form). Structural "schema" edges get a dashed line to set
      // them apart from ordinary data relations while keeping the same grey palette.
      var RDF_TYPE       = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type';
      var RDFS_SUBCLASS  = 'http://www.w3.org/2000/01/rdf-schema#subClassOf';
      var SCHEMA_HASPART = 'http://schema.org/hasPart';
      var SCHEMA_ISPART  = 'http://schema.org/isPartOf';

      // Shared white-pill label — identical across all link types.
      var LINK_LABEL_ATTRS = {
        rect: { fill: '#ffffff', stroke: 'none', rx: 3, ry: 3 },
        text: { fill: LINK_LABEL, 'font-size': 12, 'font-weight': 500 },
      };

      // Build a link template. `connectionExtra` merges into the line's SVG attrs
      // (e.g. a stroke-dasharray for dashed variants). Arrowhead + label stay constant.
      function makeLinkTemplate(connectionExtra) {
        return {
          markerTarget: { fill: LINK_LINE, stroke: LINK_LINE },
          renderLink: function() {
            return {
              connection: Object.assign(
                { stroke: LINK_LINE, 'stroke-width': 1.5 },
                connectionExtra || {}
              ),
              label: { attrs: LINK_LABEL_ATTRS },
            };
          },
        };
      }

      // Solid grey for data relations; dashed grey for the two structural/"is-a" edges.
      // IMPORTANT: return a real template (never undefined) in the default branch —
      // the library does NOT fall back to its own bundle when the resolver returns
      // undefined, so undefined would drop our grey styling back to black defaults.
      var LINK_DEFAULT = makeLinkTemplate();
      var LINK_DASHED  = makeLinkTemplate({ 'stroke-dasharray': '4,4', 'stroke-width': 4 });
      var LINK_WIDE    = makeLinkTemplate({ 'stroke-width': 4 });
      // UML generalization: hollow triangle at the superclass end. Construct mode
      // only — there the diagram is a class model, and subClassOf is inheritance
      // rather than one more "is-a" edge to de-emphasize.
      var GENERALIZATION_LINE = '#9ba3af';
      var LINK_GENERALIZATION = {
        markerTarget: { d: 'M0,0 L0,12 L14,6 z', width: 14, height: 12, fill: '#ffffff', stroke: GENERALIZATION_LINE },
        renderLink: function() {
          return {
            connection: { stroke: GENERALIZATION_LINE, 'stroke-width': 1.5 },
            label: { attrs: LINK_LABEL_ATTRS },
          };
        },
      };
      function linkTemplateResolver(linkTypeId) {
        if (CONSTRUCT && linkTypeId === RDFS_SUBCLASS) {
          return LINK_GENERALIZATION;
        }
        if (linkTypeId === RDF_TYPE || linkTypeId === RDFS_SUBCLASS) {
          return LINK_DASHED;
        }
        if (linkTypeId === SCHEMA_HASPART || linkTypeId === SCHEMA_ISPART) {
          return LINK_WIDE;
        }
        return LINK_DEFAULT;
      }

      var props = kit.workspaceProps({
        ref: onWorkspaceMounted,
        typeStyleResolver: typeStyleResolver,
        linkTemplateResolver: linkTemplateResolver,
        viewOptions: {
          onIriClick: function(iriEvent) {
            var iri = iriEvent.iri || iriEvent;
            window.open(visotoResourceHref(iri), '_blank');
          },
        },
      });

      if (!CONSTRUCT) {
        window.VisotoGE.render(container, props);
        return;
      }
      // Construct mode: the diagram exists only once the CONSTRUCT has answered.
      // A failure keeps the (empty) card with a Retry; an empty answer says so.
      function loadConstructed() {
        var done = kit.busy(vsT('js.graph.loading', 'Loading graph…'));
        fetchConstructed()
          .then(function(store) {
            done();
            if (Object.keys(store.elements).length === 0) {
              kit.showMessage(vsT('js.graph.empty', 'The graph query found nothing to draw.'), { level: 'info' });
              return;
            }
            constructedStore = store;
            window.VisotoGE.render(container, props);
          })
          .catch(function(err) {
            done();
            kit.showMessage(vsTf('js.graph.queryFailed', 'Graph query failed: {error}', { error: err.message }),
              { retry: loadConstructed });
          });
      }
      loadConstructed();
    }

    // Run init at most once, whether triggered on load (eager) or on first show (lazy).
    var initialized = false;
    function initOnce() {
      if (initialized) return;
      initialized = true;
      kit.load().then(init);
    }

    if (LAZY) {
      // Defer until the caller signals the container is visible. The listener is one-shot;
      // initOnce guards against duplicate 'graph:init' events.
      root.addEventListener('graph:init', initOnce, { once: true });
    } else {
      initOnce();
    }
  }

  // Re-entrant on purpose: a duplicate <script src> tag, or this file being
  // re-executed inside an HTMX-swapped fragment, must still pick up elements
  // that were not in the DOM the first time. There is deliberately no
  // module-level "already loaded" latch — only the per-element guard below.
  // Initialize every graph on the page. Guarded per element so a second boot
  // (duplicate script tag, or a fragment swapped in later) cannot double-init.
  function boot() {
    document.querySelectorAll('[data-sparql-graph]').forEach(function (root) {
      if (root.__visotoSparqlGraphInit) return;
      root.__visotoSparqlGraphInit = true;
      initSparqlGraph(root);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
