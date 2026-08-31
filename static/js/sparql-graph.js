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
      var ENDPOINT_URL = readIsland('-endpoint-url') || 'https://lindas.cz-aws.net/query/';

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

      function onWorkspaceMounted(workspace) {
        if (!workspace) return;
        // Stash the workspace so the fullscreen toggle / resize handler can re-fit later.
        currentWorkspace = workspace;

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

        var dataProvider = new GE.SparqlDataProvider(
          {
            endpointUrl: ENDPOINT_URL,
            acceptBlankNodes: false,
            queryMethod: GE.SparqlQueryMethod.GET,
          },
          settings
        );

        var model = workspace.getModel();
        model.importLayout({
          dataProvider: dataProvider,
          preloadedElements: {},
          layoutData: undefined,
        });

        // Load each starting element and fetch its data.
        var x = 400;
        startIris.forEach(function(startIRI) {
          var element = model.createElement(startIRI);
          if (element) {
            element.setPosition({ x: x, y: 300 });
            x += 200;
          }
        });
        model.requestElementData(startIris);
        setTimeout(function() {
          workspace.forceLayout();
          workspace.zoomToFit();
        }, 1000);
      }

      // Icon resolution is shared with schema-graph.js and mirrors internal/icon
      // (see static/js/visoto-icons.js) — one definition of "own name first, then
      // any exact type match, then any .fallback".
      var ICONS = window.VisotoIcons;

      // typeStyleResolver: resolves the icon from rdf:type values (instances like Bern).
      function typeStyleResolver(types) {
        return { icon: ICONS.resolve('', types, AVAILABLE_ICONS) || '/static/img/resource/defaultClass.svg' };
      }

      // StandardTemplateWithIcon: subclass of StandardTemplate that also checks the element's own IRI
      // This handles class nodes (e.g. schema:DefinedTerm) whose rdf:type is owl:Class —
      // typeStyleResolver gets types=[...] before data loads, so it can't help them.
      // By subclassing StandardTemplate we get its render() for free and only patch the props.
      class StandardTemplateWithIconUrl extends window.GraphExplorer.StandardTemplate {
        render() {
          var url = ICONS.resolve(this.props.iri || '', [], AVAILABLE_ICONS);
          if (!url) return super.render();
          // React 19 errors on a component reassigning this.props during render, so
          // swap the patched props in only for the super.render() call and restore.
          var original = this.props;
          this.props = Object.assign({}, original, { iconUrl: url });
          try {
            return super.render();
          } finally {
            this.props = original;
          }
        }
      }

      function elementTemplateResolver(_types) {
        return StandardTemplateWithIconUrl;
      }

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
      function linkTemplateResolver(linkTypeId) {
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
        elementTemplateResolver: elementTemplateResolver,
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

      GE.renderTo(GE.Workspace, container, props);
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
