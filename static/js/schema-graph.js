/* eslint-disable */
/*
  Behaviour for the "schemaGraph" partial (templates/partials/schema-graph.html).

  Extracted from an inline <script> in that partial so the code is cacheable,
  lintable and editable as JavaScript rather than as Go template output. The
  template now emits only markup plus data islands; everything below is plain JS.

  Instances are discovered declaratively: the partial marks its root element with
  data-schema-graph, and this file initializes every such element it finds. Per
  instance configuration comes from attributes and <template> data islands keyed
  on the instance id, so nothing here is generated server-side.

  Attributes read from the root element:
    data-schema-graph        marker; presence means "initialize me"
    data-schema-graph-id     DOM id prefix for this instance's islands/elements
    data-schema-graph-lazy   "true" to defer init until a 'schema:init' event

  See the partial's header comment for what the view actually does. Loading,
  the toolbar, undo, fullscreen and error messages are shared with
  sparql-graph.js through static/js/graph-kit.js; GE itself is reached through
  static/js/ge-adapter.js.
*/
(function () {
  'use strict';

  function initSchemaGraph(root) {
    var ID = root.getAttribute('data-schema-graph-id');
    if (!ID) return;
    var LAZY = root.getAttribute('data-schema-graph-lazy') === 'true';

    var kit = window.VisotoGraph.create(ID);
    var readIsland = kit.readIsland;

    var AVAILABLE_ICONS = readIsland('-available-icons') || {};
    var ENDPOINT_URL = readIsland('-endpoint-url') || '/api/sparql';
    var urlParams = new URLSearchParams(window.location.search);
    var RESOURCE_IRI = readIsland('-iri') || urlParams.get('iri');
    var CLASS_OVERRIDE = urlParams.get('class');

    // setStatus takes MARKUP (callers wrap names in <code>); interpolated values
    // must therefore be passed through escapeHtml first.
    function setStatus(html) {
      var el = document.getElementById(ID + '-status');
      if (el) el.innerHTML = html;
    }

    // localName() decodeURIComponent's the IRI, but validIri() tested the RAW
    // string — so "%3Cimg onerror=...%3E" passes validation and decodes back
    // into live markup. Escape anything IRI-derived before it reaches innerHTML.
    function escapeHtml(text) {
      var d = document.createElement('div');
      d.textContent = text;
      return d.innerHTML;
    }
    // message is TEXT, never markup: it carries endpoint-supplied strings
    // (err.message from the SPARQL fetch/parse path); kit.showMessage sets it
    // via textContent, so a hostile endpoint's error text cannot execute script.
    function showError(message, retry) {
      kit.showMessage(message, { retry: retry });
      setStatus('');
    }

    function validIri(iri) {
      return /^https?:\/\/[^<>"{}|\\^`\s]+$/.test(iri);
    }

    function localName(iri) {
      var decoded = decodeURIComponent(iri);
      return decoded.includes('#') ? decoded.split('#').pop() : decoded.split('/').pop();
    }

    function sparql(query, accept) {
      return fetch(ENDPOINT_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/sparql-query', 'Accept': accept },
        body: query,
      }).then(function (res) {
        if (!res.ok) throw new Error('SPARQL request failed: ' + res.status);
        return res.text();
      });
    }

    // -------------------------------------------------------------------------
    // Derivation + projection query. One CONSTRUCT derives the informal schema
    // and projects it straight into "UML" triples: class boxes (rdfs:Class nodes
    // with rdfs:label), object properties as edges between boxes, datatype
    // properties as literal attribute rows on the anchor box.
    //   - instance mode: subjects = the single instance.
    //   - class mode: subjects = a sample of up to 50 instances of the class.
    // -------------------------------------------------------------------------
    function vizQuery(mode, iri, cls) {
      var subjects = mode === 'class'
        ? '{ SELECT ?instance WHERE { ?instance a <' + cls + '> } LIMIT 50 }'
        : 'VALUES ?instance { <' + iri + '> }';
      return [
        'PREFIX xsd:  <http://www.w3.org/2001/XMLSchema#>',
        'PREFIX rdf:  <http://www.w3.org/1999/02/22-rdf-syntax-ns#>',
        'PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>',
        'CONSTRUCT {',
        '  ?class a rdfs:Class ; rdfs:label ?classLbl .',
        '  ?nbr   a rdfs:Class ; rdfs:label ?nbrLbl .',
        '  ?src ?p ?tgt .',
        '  ?class ?p ?attr .',
        '} WHERE {',
        '  VALUES ?class { <' + cls + '> }',
        '  {',
        '    SELECT ?dir ?p ?dt ?oc ?kind (COUNT(*) AS ?count) WHERE {',
        '      ' + subjects,
        '      {',
        '        BIND("out" AS ?dir)',
        '        ?instance ?p ?o .',
        '        FILTER(?p != rdf:type)',
        '        OPTIONAL { ?o a ?ocx }',
        '        BIND(IF(isLiteral(?o), DATATYPE(?o), ?ub) AS ?dt)',
        '        BIND(IF(isIRI(?o), COALESCE(?ocx, ?ub), ?ub) AS ?oc)',
        '        BIND(IF(isIRI(?o) && !BOUND(?ocx), true, ?ub) AS ?kind)',
        '      } UNION {',
        '        BIND("in" AS ?dir)',
        '        ?s ?p ?instance .',
        '        FILTER(?p NOT IN (rdf:type, rdf:first, rdf:rest))',
        '        OPTIONAL { ?s a ?ocx }',
        '        BIND(COALESCE(?ocx, ?ub) AS ?oc)',
        '        BIND(IF(!BOUND(?ocx), true, ?ub) AS ?kind)',
        '      }',
        '    } GROUP BY ?dir ?p ?dt ?oc ?kind',
        '  }',
        '  BIND(IF(BOUND(?dt), REPLACE(REPLACE(STR(?dt), STR(xsd:), "xsd:"), STR(rdf:), "rdf:"), ?ub) AS ?attr)',
        '  BIND(IF(!BOUND(?dt), COALESCE(?oc, rdfs:Resource), ?ub) AS ?nbr)',
        '  BIND(IF(?dir = "out" && BOUND(?nbr), ?class, IF(?dir = "in", ?nbr, ?ub)) AS ?src)',
        '  BIND(IF(?dir = "out" && BOUND(?nbr), ?nbr,   IF(?dir = "in", ?class, ?ub)) AS ?tgt)',
        '  BIND(REPLACE(STR(?class), "^.*[/#]", "") AS ?classLbl)',
        '  BIND(IF(BOUND(?nbr), REPLACE(STR(?nbr), "^.*[/#]", ""), ?ub) AS ?nbrLbl)',
        '}',
      ].join('\n');
    }

    // N-Triples parsing, the store and the in-memory DataProvider are shared
    // with sparql-graph.js's construct mode (static/js/graph-memory-store.js).
    var MEM = window.VisotoMemoryGraph;

    // -------------------------------------------------------------------------
    // Rendering: same Tabler-grey link styling as sparql-graph; class boxes
    // placed on a grid, only boxes with attribute rows expanded (expanding the
    // rest just renders a noisy "no properties" placeholder), then force layout
    // + fit.
    // -------------------------------------------------------------------------
    var LINK_LINE = '#e6e7e9';
    var LINK_LABEL = '#1d273b';
    var LINK_DEFAULT = {
      markerTarget: { fill: LINK_LINE, stroke: LINK_LINE },
      renderLink: function () {
        return {
          connection: { stroke: LINK_LINE, 'stroke-width': 1.5 },
          label: {
            attrs: {
              rect: { fill: '#ffffff', stroke: 'none', rx: 3, ry: 3 },
              text: { fill: LINK_LABEL, 'font-size': 12, 'font-weight': 500 },
            },
          },
        };
      },
    };

    // Icon resolution is shared with sparql-graph.js and mirrors internal/icon
    // (see static/js/visoto-icons.js).
    function typeStyleResolver(types) {
      var url = window.VisotoIcons.resolve('', types, AVAILABLE_ICONS);
      return { icon: url || '/static/img/resource/defaultClass.svg' };
    }

    function renderSchema(store, mode, cls) {
      var container = kit.container;
      // GL-2: the schema is a class model, read as a tree; Radial centres on
      // the anchor class (GL-19).
      kit.defaultLayout = 'tree-right';
      kit.pageIri = cls;
      var summary = Object.keys(store.elements).length + ' classes, ' + store.links.length + ' relations';
      if (mode === 'class') {
        setStatus('derived from up to 50 sampled instances of <code>' + escapeHtml(localName(cls)) + '</code> &mdash; ' + summary);
      } else {
        setStatus('derived from <code>' + escapeHtml(localName(RESOURCE_IRI)) + '</code> (anchored on <code>' + escapeHtml(localName(cls)) + '</code>) &mdash; ' + summary);
      }

      function onWorkspaceMounted(workspace) {
        if (!workspace) return;
        kit.attach(workspace);
        var model = workspace.getModel();
        // Seed after importLayout settles: it registers link types
        // asynchronously (see seed() in sparql-graph.js).
        Promise.resolve(model.importLayout({
          dataProvider: MEM.makeProvider(store),
          preloadedElements: {},
          layoutData: undefined,
        })).then(seed);
      }

      function seed() {
        var workspace = kit.workspace;
        var model = workspace.getModel();

        var iris = Object.keys(store.elements);
        var cols = Math.ceil(Math.sqrt(iris.length));
        iris.forEach(function (iri, i) {
          var el = model.createElement(iri);
          if (el) {
            el.setPosition({ x: (i % cols) * 300, y: Math.floor(i / cols) * 200 });
          }
        });

        Promise.resolve(model.requestElementData(iris))
          .then(function () { return model.requestLinksOfType(); })
          .then(function () {
            model.elements.forEach(function (el) {
              var data = store.elements[el.iri];
              var hasAttrs = data && Object.keys(data.properties).length > 0;
              if (hasAttrs && el.setExpanded) el.setExpanded(true);
            });
            setTimeout(function () {
              kit.layout(null, { initial: true }).then(kit.ready);
            }, 300);
          });
      }

      window.VisotoGE.render(container, kit.workspaceProps({
        ref: onWorkspaceMounted,
        typeStyleResolver: typeStyleResolver,
        // No element template override: the icon rides on element.data.image,
        // which StandardTemplate.renderThumbnail() renders directly.
        linkTemplateResolver: function () { return LINK_DEFAULT; },
        viewOptions: {
          onIriClick: function (iriEvent) {
            var iri = iriEvent.iri || iriEvent;
            window.open(visotoResourceHref(iri), '_blank');
          },
        },
      }));
    }

    // -------------------------------------------------------------------------
    // Mode detection: a resource that has instances is treated as a class
    // (sample mode); anything else is treated as an instance (anchor class
    // auto-detected, preferring LINDAS domain classes over generic role
    // classes).
    // -------------------------------------------------------------------------
    function detectMode() {
      var q = 'ASK { ?i a <' + RESOURCE_IRI + '> }';
      return sparql(q, 'application/sparql-results+json').then(function (text) {
        return JSON.parse(text).boolean === true ? 'class' : 'instance';
      });
    }

    function detectAnchorClass() {
      if (CLASS_OVERRIDE && validIri(CLASS_OVERRIDE)) return Promise.resolve(CLASS_OVERRIDE);
      var q = 'SELECT ?c WHERE { <' + RESOURCE_IRI + '> a ?c }';
      return sparql(q, 'application/sparql-results+json').then(function (text) {
        var bindings = JSON.parse(text).results.bindings;
        if (bindings.length === 0) {
          var err = new Error('Resource has no rdf:type and no instances — cannot derive a schema');
          err.noRetry = true;
          throw err;
        }
        var types = bindings.map(function (b) { return b.c.value; });
        var preferred = types.filter(function (t) { return t.indexOf('schema.ld.admin.ch') >= 0; });
        return preferred[preferred.length - 1] || types[0];
      });
    }

    function derive() {
      if (!RESOURCE_IRI || !validIri(RESOURCE_IRI)) {
        showError('No valid resource IRI to derive a schema for');
        return;
      }
      setStatus('deriving&hellip;');
      var done = kit.busy(vsT('js.graph.loading', 'Loading graph…'));
      detectMode()
        .then(function (mode) {
          var clsPromise = mode === 'class' ? Promise.resolve(RESOURCE_IRI) : detectAnchorClass();
          return clsPromise.then(function (cls) {
            return sparql(vizQuery(mode, RESOURCE_IRI, cls), 'application/n-triples').then(function (nt) {
              var store = MEM.buildStore(MEM.parseNTriples(nt), AVAILABLE_ICONS);
              done();
              if (Object.keys(store.elements).length === 0) {
                kit.showMessage(vsT('js.graph.empty', 'The graph query found nothing to draw.'), { level: 'info' });
                setStatus('');
                return;
              }
              renderSchema(store, mode, cls);
            });
          });
        })
        .catch(function (err) {
          done();
          // A resource without type or instances is a property of the data,
          // not a failure: nothing to retry.
          showError('Schema derivation failed: ' + err.message, err.noRetry ? undefined : derive);
        });
    }

    // Run at most once, whether triggered on load (eager) or on first show (lazy).
    var initialized = false;
    function initOnce() {
      if (initialized) return;
      initialized = true;
      kit.load().then(derive);
    }

    if (LAZY) {
      // Defer until the caller signals the container is visible (see resource-view-toggle.js).
      root.addEventListener('schema:init', initOnce, { once: true });
    } else {
      initOnce();
    }
  }

  // Re-entrant on purpose: a duplicate <script src> tag, or this file being
  // re-executed inside an HTMX-swapped fragment, must still pick up elements
  // that were not in the DOM the first time. There is deliberately no
  // module-level "already loaded" latch — only the per-element guard below.
  // Initialize every instance on the page. Guarded per element so a second boot
  // (duplicate script tag, or a fragment swapped in later) cannot double-init.
  function boot() {
    document.querySelectorAll('[data-schema-graph]').forEach(function (root) {
      if (root.__visotoSchemaGraphInit) return;
      root.__visotoSchemaGraphInit = true;
      initSchemaGraph(root);
    });
  }

  // The partial loads this file with `defer`, so the DOM is parsed by the time
  // this runs — but check readyState anyway so the file stays safe to load from
  // a fragment, where DOMContentLoaded has already fired and will never fire again.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
