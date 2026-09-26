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
*/
(function () {
  'use strict';

  // boot() runs at (or after) DOMContentLoaded, so the DOM is ready by the time
  // initSparqlGraph is called and the readyState guards the inline block needed
  // are no longer necessary.
  function initSparqlGraph(root) {

    var ID = root.getAttribute('data-sparql-graph-id');

    // Set once the Graph Explorer workspace mounts; used by the fullscreen toggle and
    // the resize handler to re-fit the canvas after its size changes.
    var currentWorkspace = null;

    // When lazy, init is deferred until the container is first made visible (the caller
    // dispatches a 'graph:init' event at the -root element). This avoids Graph Explorer
    // measuring a zero-size box if the container starts hidden (e.g. behind a view toggle).
    var LAZY = root.getAttribute('data-sparql-graph-lazy') === 'true';

    function readIsland(suffix) {
      var el = document.getElementById(ID + suffix);
      if (!el) return null;
      try { return JSON.parse(el.innerHTML.trim()); } catch (e) { return null; }
    }

    // Apply the container height (kept out of the style="" attribute to avoid Go's
    // html/template CSS-context sanitizer; see the note by the -root div).
    (function() {
      var root = document.getElementById(ID + '-root');
      var h = readIsland('-height');
      if (root && h) root.style.height = h;
    })();

    // --- Guarded Graph Explorer CDN loader --------------------------------------------
    // The graph-explorer bundle is NOT part of the global base layout (only /ontodia
    // loads it inline). Inject it once here and initialize once it's ready, so multiple
    // partial instances share a single script load.
    var GE_SRC = 'https://cdn.jsdelivr.net/npm/graph-explorer@2.1.0/dist/graph-explorer-full.min.js';

    function whenGraphExplorerReady(cb) {
      // Ensure the single shared loader script exists (inject once, deduped by marker).
      if (!window.GraphExplorer && !document.querySelector('script[data-graph-explorer-loader]')) {
        var loader = document.createElement('script');
        loader.src = GE_SRC;
        loader.setAttribute('data-graph-explorer-loader', '');
        loader.addEventListener('error', function() {
          var container = document.getElementById(ID + '-root');
          if (container) {
            container.innerHTML = '<div class="alert alert-danger m-3">Failed to load Graph Explorer library</div>';
          }
        });
        document.head.appendChild(loader);
      }
      // Poll for readiness rather than relying on the script's load event: the shared
      // script may already be loaded (cached, or loaded by another instance) before we
      // could attach a listener, so a load handler alone would race and be missed.
      if (window.GraphExplorer) { cb(); return; }
      var waited = 0;
      var poll = setInterval(function() {
        if (window.GraphExplorer) {
          clearInterval(poll);
          cb();
        } else if ((waited += 50) >= 15000) {
          clearInterval(poll);
          var container = document.getElementById(ID + '-root');
          if (container && !container.querySelector('svg')) {
            container.innerHTML = '<div class="alert alert-danger m-3">Failed to load Graph Explorer library</div>';
          }
        }
      }, 50);
    }

    function init() {
      var GE = window.GraphExplorer;
      var container = document.getElementById(ID + '-root');
      if (!container) return;

      if (!GE || !GE.Workspace || !GE.SparqlDataProvider) {
        container.innerHTML = '<div class="alert alert-danger m-3">Failed to load Graph Explorer library</div>';
        return;
      }

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
      var constructedStore = null;

      function onWorkspaceMounted(workspace) {
        if (!workspace) return;
        // Stash the workspace so the fullscreen toggle / resize handler can re-fit later.
        currentWorkspace = workspace;

        if (constructedStore) {
          mountConstructed(workspace, constructedStore);
          return;
        }

        // OWLStatsSettings with LINDAS-specific label properties and prefixes.
        var settings = Object.assign({}, GE.OWLStatsSettings, {
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
        });

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
        var dataProvider = new GE.SparqlDataProvider(
          {
            endpointUrl: ENDPOINT_URL,
            acceptBlankNodes: false,
            queryMethod: GE.SparqlQueryMethod.POST,
          },
          settings
        );

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

        model.importLayout({
          dataProvider: dataProvider,
          preloadedElements: {},
          layoutData: undefined,
          diagram: linkTypeOptions ? { linkTypeOptions: linkTypeOptions } : undefined,
        });

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
        function relayout() {
          if (didLayout) return;
          didLayout = true;
          workspace.forceLayout();
          workspace.zoomToFit();
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
      }

      // --- Construct mode ---------------------------------------------------------------
      // The page IRI substituted for `??`, as in server-side queries. Validated
      // because it is spliced into query text between angle brackets.
      function validIri(iri) {
        return /^https?:\/\/[^<>"{}|\\^`\s]+$/.test(iri);
      }

      function showError(message) {
        var alert = document.createElement('div');
        alert.className = 'alert alert-danger m-3';
        alert.textContent = message;
        container.replaceChildren(alert);
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
        model.importLayout({
          dataProvider: MEM.makeProvider(store),
          preloadedElements: {},
          layoutData: undefined,
        });

        var iris = Object.keys(store.elements);
        var cols = Math.ceil(Math.sqrt(iris.length));
        iris.forEach(function(iri, i) {
          var el = model.createElement(iri);
          if (el) el.setPosition({ x: (i % cols) * 300, y: Math.floor(i / cols) * 200 });
        });

        Promise.resolve(model.requestElementData(iris))
          .then(function() { return model.requestLinksOfType(); })
          .then(function() {
            model.elements.forEach(function(el) {
              var data = store.elements[el.iri];
              if (data && Object.keys(data.properties).length > 0 && el.setExpanded) el.setExpanded(true);
            });
            setTimeout(function() {
              workspace.forceLayout();
              workspace.zoomToFit();
            }, 300);
          });
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

      var props = {
        ref: onWorkspaceMounted,
        typeStyleResolver: typeStyleResolver,
        linkTemplateResolver: linkTemplateResolver,
        languages: [
          { code: 'en', label: 'English' },
          { code: 'de', label: 'German' },
          { code: 'fr', label: 'French' },
          { code: 'it', label: 'Italian' },
        ],
        language: 'en',
        viewOptions: {
          onIriClick: function(iriEvent) {
            var iri = iriEvent.iri || iriEvent;
            window.open(visotoResourceHref(iri), '_blank');
          },
        },
      };

      if (!CONSTRUCT) {
        GE.renderTo(GE.Workspace, container, props);
        return;
      }
      // Constructed labels come in every language the ontology has; show the
      // page's (html lang, from the site-lang cookie) when GE offers it.
      var pageLang = (document.documentElement.lang || '').slice(0, 2);
      if (props.languages.some(function(l) { return l.code === pageLang; })) props.language = pageLang;
      fetchConstructed()
        .then(function(store) {
          if (Object.keys(store.elements).length === 0) throw new Error('The graph query returned no triples');
          constructedStore = store;
          GE.renderTo(GE.Workspace, container, props);
        })
        .catch(function(err) { showError('Graph query failed: ' + err.message); });
    }

    // Run init at most once, whether triggered on load (eager) or on first show (lazy).
    var initialized = false;
    function initOnce() {
      if (initialized) return;
      initialized = true;
      whenGraphExplorerReady(init);
    }

    // --- Fullscreen ("maximize") toggle -----------------------------------------------
    // Toggling the .graph-maximized class on the card pins the canvas to the viewport
    // (see .graph-maximized rules in ontodia_overrides.css). We deliberately DON'T use
    // the JS Fullscreen API — a fixed overlay lets native F11 stack on top for true
    // edge-to-edge. Graph Explorer only auto-fits once at mount, so re-fit after the box
    // resizes (both on toggle and on window resize, e.g. F11).
    function setupFullscreen() {
      var card = document.getElementById(ID + '-card');
      var maximizeBtn = document.getElementById(ID + '-maximize');
      var exitBtn = document.getElementById(ID + '-exit');
      if (!card || !maximizeBtn || !exitBtn) return;

      function refit() {
        // zoomToFit re-centers within the resized viewport; guard until the workspace mounts.
        setTimeout(function() {
          if (currentWorkspace && currentWorkspace.zoomToFit) currentWorkspace.zoomToFit();
        }, 100);
      }

      maximizeBtn.addEventListener('click', function() {
        // Fullscreen on a collapsed card would pin a canvas that is inside a
        // display:none wrapper — nothing visible but the exit button. Expand first;
        // shown.bs.collapse then refits, and the refit() below covers the already-
        // expanded case. Bootstrap's bundle is exposed as window.tabler here (the
        // Tabler build), NOT window.bootstrap — that global does not exist.
        var collapsed = document.getElementById('collapse-' + ID);
        var Collapse = window.tabler && window.tabler.Collapse;
        if (collapsed && !collapsed.classList.contains('show') && Collapse) {
          Collapse.getOrCreateInstance(collapsed).show();
        }
        card.classList.add('graph-maximized');
        refit();
      });
      exitBtn.addEventListener('click', function() {
        card.classList.remove('graph-maximized');
        refit();
      });

      // Re-fit on viewport resize while maximized (covers native F11 growing the viewport).
      var resizeTimer;
      window.addEventListener('resize', function() {
        if (!card.classList.contains('graph-maximized')) return;
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(function() {
          if (currentWorkspace && currentWorkspace.zoomToFit) currentWorkspace.zoomToFit();
        }, 150);
      });

      // Re-fit after the collapse animation finishes. While collapsed the canvas has
      // no height, so the workspace's idea of its viewport is stale on the way back —
      // without this the graph returns off-centre or clipped. shown.bs.collapse fires
      // at the END of the transition, so the box is already at full height here.
      var collapseEl = document.getElementById('collapse-' + ID);
      if (collapseEl) collapseEl.addEventListener('shown.bs.collapse', refit);
    }
    // Buttons exist in the DOM regardless of GE readiness; wire them up as soon as possible.
    setupFullscreen();

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
