/*
  Behaviour for the "sparqlFlow" partial (templates/partials/sparql-flow.html).

  Turns ?from ?predicate ?to bindings into Mermaid flowchart text and draws it
  with Pele (https://github.com/obsidianmd/pele). Pele lays the diagram out
  itself — there is no external layout engine — and supplies pan/zoom and its
  zoom controls. Theme colours come from --pele-* CSS variables
  (static/css/flow_overrides.css), so a light/dark switch needs no re-render.

  Attributes read from the root element:
    data-sparql-flow            marker; presence means "initialize me"
    data-sparql-flow-id         id prefix for this diagram's elements/islands
    data-sparql-flow-direction  initial direction ("LR", "TD", "RL" or "BT")
    data-sparql-flow-zoom-*     translated zoom-control labels (in, out, reset)
    data-sparql-flow-error      translated message for a diagram Pele cannot draw
*/
(function () {
  'use strict';

  // Keep src and integrity in step: a bump without a new hash fails closed.
  var PELE_JS = 'https://cdn.jsdelivr.net/npm/pele@1.12.0/dist/pele.global.js';
  var PELE_JS_SRI = 'sha384-c+QpxCAcAD72seekPP6lAUNHjqe/wUv+S2pk2yBo4ECNKMxym7WFtKKoR5xMtyhz';
  var LOAD_TIMEOUT = 15000;

  var DIRECTIONS = ['LR', 'TD', 'RL', 'BT'];

  // Mermaid shape syntax; LABEL is replaced with the quoted node label.
  var SHAPE_TEMPLATES = {
    'rectangle': '["LABEL"]',
    'rounded': '("LABEL")',
    'stadium': '(["LABEL"])',
    'subroutine': '[["LABEL"]]',
    'cylindrical': '[("LABEL")]',
    'circle': '(("LABEL"))',
    'asymmetric': '>"LABEL"]',
    'rhombus': '{"LABEL"}',
    'hexagon': '{{"LABEL"}}',
    'parallelogram': '[/"LABEL"/]',
    'parallelogram_alt': '[\\"LABEL"\\]',
    'trapezoid': '[/"LABEL"\\]',
    'trapezoid_alt': '[\\"LABEL"/]',
    'double_circle': '((("LABEL")))'
  };

  // --- Pele loader -----------------------------------------------------------------
  // One shared <script> for every diagram on the page. Readiness is polled, as in
  // sparql-map.js: another diagram may have injected the script already.
  var peleLoading = null;
  function loadPele() {
    if (window.pele && window.pele.mount) return Promise.resolve(window.pele);
    if (peleLoading) return peleLoading;
    peleLoading = new Promise(function (resolve, reject) {
      if (!document.querySelector('script[data-pele-loader]')) {
        var script = document.createElement('script');
        script.src = PELE_JS;
        script.integrity = PELE_JS_SRI;
        script.crossOrigin = 'anonymous';
        script.setAttribute('data-pele-loader', '');
        script.addEventListener('error', function () { reject(new Error('load')); });
        document.head.appendChild(script);
      }
      var waited = 0;
      var poll = setInterval(function () {
        if (window.pele && window.pele.mount) {
          clearInterval(poll);
          resolve(window.pele);
        } else if ((waited += 50) >= LOAD_TIMEOUT) {
          clearInterval(poll);
          reject(new Error('timeout'));
        }
      }, 50);
    });
    peleLoading.catch(function () { peleLoading = null; });
    return peleLoading;
  }

  function storageGet(key) {
    try { return localStorage.getItem(key); } catch (err) { return null; }
  }

  function storageSet(key, value) {
    try { localStorage.setItem(key, value); } catch (err) { /* storage unavailable */ }
  }

  function readIsland(id) {
    var el = document.getElementById(id);
    return el ? el.innerHTML.trim() : '';
  }

  // Escape text for a quoted Mermaid label. Pele reads tags such as <br> and <i>
  // in labels, so data text has its markup characters turned into entities.
  function escapeLabel(text) {
    if (!text) return '';
    return String(text)
      .replace(/&/g, '&amp;')
      .replace(/"/g, '&quot;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/\n/g, '<br>');
  }

  // Fit a resource icon into Pele's 24×24 icon box. The files carry a pixel
  // width/height but no viewBox, and their ids (clip0, …) repeat across files,
  // so each icon gets a viewBox and ids prefixed with its own name. Pele's
  // wrapper sets fill="none" and a stroke; the root resets both.
  function iconMarkup(text, name) {
    var doc = new DOMParser().parseFromString(text, 'image/svg+xml');
    var svg = doc.documentElement;
    if (!svg || svg.nodeName !== 'svg') return '';
    if (!svg.getAttribute('viewBox')) {
      svg.setAttribute('viewBox', '0 0 ' + parseFloat(svg.getAttribute('width') || 24) + ' ' +
        parseFloat(svg.getAttribute('height') || 24));
    }
    svg.setAttribute('width', '24');
    svg.setAttribute('height', '24');
    svg.setAttribute('fill', 'currentColor');
    svg.setAttribute('stroke', 'none');
    var prefix = name.replace(/[^\w-]/g, '-') + '-';
    var markup = new XMLSerializer().serializeToString(svg);
    return markup
      .replace(/\sid="([^"]+)"/g, ' id="' + prefix + '$1"')
      .replace(/url\(#([^)]+)\)/g, 'url(#' + prefix + '$1)')
      .replace(/href="#([^"]+)"/g, 'href="#' + prefix + '$1"');
  }

  function defaultConfig() {
    return { typeMap: {}, classDefs: [], reversePredicates: [], defaultShape: 'rounded' };
  }

  function FlowDiagram(root) {
    this.root = root;
    this.id = root.getAttribute('data-sparql-flow-id');
    this.bindings = JSON.parse(readIsland(this.id + '-bindings') || '[]');
    var configText = readIsland(this.id + '-config');
    this.config = configText ? JSON.parse(configText) : defaultConfig();
    this.extraField = readIsland(this.id + '-extraField') || null;
    this.select = document.getElementById('direction-' + this.id);
    this.cardBody = document.getElementById(this.id + '-card-body');
    this.mounted = null;
    this.mapping = this.buildNodeMapping();
    this.iconMarkup = {};   // Pele icon name → SVG markup, filled by loadIcons()
    this.idToIcon = {};     // node id → Pele icon name
  }

  // First type entry in config.typeMap whose name occurs in the type IRI/label.
  FlowDiagram.prototype.typeEntry = function (typeUri) {
    var typeMap = this.config.typeMap;
    if (!typeUri || !typeMap) return null;
    for (var typeName in typeMap) {
      if (typeUri.indexOf(typeName) !== -1) return typeMap[typeName];
    }
    return null;
  };

  FlowDiagram.prototype.nodeShape = function (typeUri, label) {
    var entry = this.typeEntry(typeUri);
    var shapeName = (entry && entry.shape) || this.config.defaultShape || 'rounded';
    var template = SHAPE_TEMPLATES[shapeName] || SHAPE_TEMPLATES.rounded;
    return template.replace('LABEL', label);
  };

  FlowDiagram.prototype.nodeClass = function (typeUri) {
    var entry = this.typeEntry(typeUri);
    return entry && entry.cssClass ? entry.cssClass : null;
  };

  // IRI → node id (n1, n2, …), plus each node's type and extra-field text.
  FlowDiagram.prototype.buildNodeMapping = function () {
    var uriToId = {};
    var idToUri = {};
    var idToType = {};
    var idToTypeIri = {};
    var idToExtra = {};
    var index = 1;
    var extraField = this.extraField;

    function typeText(term) {
      return term ? (term.DisplayText || term.Value) : null;
    }

    function addNode(term, typeTerm) {
      if (uriToId[term.Value]) return uriToId[term.Value];
      var id = 'n' + index++;
      uriToId[term.Value] = id;
      idToUri[id] = term.Value;
      var type = typeText(typeTerm);
      if (type) idToType[id] = type;
      if (typeTerm && typeTerm.Value) idToTypeIri[id] = typeTerm.Value;
      return id;
    }

    function setExtra(id, term) {
      var value = term && (term.DisplayText || term.Value);
      if (value && id && !idToExtra[id]) idToExtra[id] = value;
    }

    this.bindings.forEach(function (b) {
      if (!b.from || !b.to) return;
      addNode(b.from, b.fromType);
      addNode(b.to, b.toType);
    });

    // Extra field: from-/to-specific (fromValidFrom, toValidFrom) wins over a
    // single shared variable (validFrom).
    if (extraField) {
      var cap = extraField.charAt(0).toUpperCase() + extraField.slice(1);
      this.bindings.forEach(function (b) {
        if (!b.from || !b.to) return;
        var fromId = uriToId[b.from.Value];
        var toId = uriToId[b.to.Value];
        setExtra(fromId, b['from' + cap]);
        setExtra(toId, b['to' + cap]);
        setExtra(fromId, b[extraField]);
        setExtra(toId, b[extraField]);
      });
    }

    return { uriToId: uriToId, idToUri: idToUri, idToType: idToType, idToTypeIri: idToTypeIri, idToExtra: idToExtra };
  };

  FlowDiagram.prototype.nodeLabel = function (id, term) {
    var m = this.mapping;
    var text = escapeLabel(term.DisplayText || term.Value);
    var type = m.idToType[id] ? '<i>' + escapeLabel(m.idToType[id]) + '</i>' : '';
    var label;
    if (this.idToIcon[id]) {
      // Card layout, after Graph Explorer: type line on top, then icon + bold label.
      label = (type ? type + '<br>' : '') + this.idToIcon[id] + ' <b>' + text + '</b>';
    } else {
      label = text + (type ? '<br>' + type : '');
    }
    if (m.idToExtra[id]) label += '<br>' + escapeLabel(m.idToExtra[id]);
    return label;
  };

  // Resource icons for nodes whose typeMap entry sets "icon": true, resolved like
  // Graph Explorer's (static/js/visoto-icons.js) and fetched before the first
  // render, since Pele's icon resolver is synchronous.
  FlowDiagram.prototype.loadIcons = function () {
    var self = this;
    var available = JSON.parse(readIsland(this.id + '-available-icons') || 'null');
    if (!available || !window.VisotoIcons) return Promise.resolve();

    var urlToName = {};
    var count = 0;
    Object.keys(this.mapping.idToUri).forEach(function (id) {
      var entry = self.typeEntry(self.mapping.idToType[id]);
      if (!entry || !entry.icon) return;
      var url = window.VisotoIcons.resolve(self.mapping.idToUri[id], [self.mapping.idToTypeIri[id]], available) ||
        window.VisotoIcons.basePath + 'defaultInstance.svg';
      // Pele only reads Font Awesome icon syntax; fak: is FA's custom-kit prefix.
      if (!urlToName[url]) urlToName[url] = 'fak:fa-' + self.id + '-' + (count++);
      self.idToIcon[id] = urlToName[url];
    });

    return Promise.all(Object.keys(urlToName).map(function (url) {
      return fetch(url).then(function (res) { return res.ok ? res.text() : ''; }).then(function (text) {
        var markup = iconMarkup(text, urlToName[url]);
        if (markup) self.iconMarkup[urlToName[url]] = markup;
      }, function () { /* no icon; the label stays */ });
    })).then(function () {
      // Drop names whose file did not load, so Pele does not print them as text.
      Object.keys(self.idToIcon).forEach(function (id) {
        if (!self.iconMarkup[self.idToIcon[id]]) delete self.idToIcon[id];
      });
    });
  };

  // Mermaid flowchart text: header, class definitions, nodes, classes, edges, links.
  FlowDiagram.prototype.source = function (direction) {
    var self = this;
    var m = this.mapping;
    var nodeLines = [];
    var classLines = [];
    var edgeLines = [];
    var clickLines = [];
    var defined = {};
    var reverse = this.config.reversePredicates || [];

    function defineNode(id, term) {
      if (defined[id]) return;
      defined[id] = true;
      nodeLines.push(id + self.nodeShape(m.idToType[id], self.nodeLabel(id, term)));
      var cssClass = self.nodeClass(m.idToType[id]);
      if (cssClass) classLines.push(id + ':::' + cssClass);
      clickLines.push('click ' + id + ' href "' + visotoResourceHref(m.idToUri[id]) + '"');
    }

    this.bindings.forEach(function (b) {
      if (!b.from || !b.to || !b.predicate) return;
      var fromId = m.uriToId[b.from.Value];
      var toId = m.uriToId[b.to.Value];
      defineNode(fromId, b.from);
      defineNode(toId, b.to);

      var edgeLabel = escapeLabel(b.predicate.DisplayText || b.predicate.Value);
      var predicate = b.predicate.Value || '';
      var reversed = reverse.some(function (p) { return predicate.indexOf(p) !== -1; });
      // A reversed predicate is laid out backwards (to → from) so it runs with
      // the main flow, and dotted to set it apart. Mermaid has no start-only
      // arrowhead, so it is drawn two-headed and dropTrailingArrows() removes
      // the head at `from`, leaving the arrow pointing at `to` as in the data.
      edgeLines.push(reversed
        ? toId + ' <-.->|"' + edgeLabel + '"| ' + fromId
        : fromId + ' -->|"' + edgeLabel + '"| ' + toId);
    });

    return ['flowchart ' + direction]
      .concat(this.config.classDefs || [], nodeLines, classLines, edgeLines, clickLines)
      .join('\n');
  };

  // Two-headed edges are only ever reversed predicates (see source()): remove the
  // trailing head and run the line on to where its tip was, so it meets the node.
  FlowDiagram.prototype.dropTrailingArrows = function () {
    this.root.querySelectorAll('.pele-edge').forEach(function (edge) {
      var markers = edge.querySelectorAll('.pele-marker');
      if (markers.length !== 2) return;
      var tail = markers[1];
      var tip = /^M\s*([-\d.]+)[ ,]([-\d.]+)/.exec(tail.getAttribute('d') || '');
      var line = edge.querySelector('path:not(.pele-marker)');
      if (tip && line) line.setAttribute('d', line.getAttribute('d') + 'L' + tip[1] + ',' + tip[2]);
      tail.remove();
    });
  };

  FlowDiagram.prototype.direction = function () {
    var saved = storageGet('flow-direction-' + this.id);
    if (DIRECTIONS.indexOf(saved) !== -1) return saved;
    // Phones (below Bootstrap's sm) default to top-down: a wide left-to-right
    // diagram shrinks to unreadable text there. A chosen direction still wins.
    if (window.matchMedia('(max-width: 575.98px)').matches) return 'TD';
    var initial = this.root.getAttribute('data-sparql-flow-direction');
    return DIRECTIONS.indexOf(initial) !== -1 ? initial : 'LR';
  };

  FlowDiagram.prototype.showError = function (err) {
    console.error('[sparql-flow]', err);
    var alert = document.createElement('div');
    alert.className = 'alert alert-danger m-0';
    alert.textContent = this.root.getAttribute('data-sparql-flow-error');
    this.root.replaceChildren(alert);
  };

  FlowDiagram.prototype.render = function (pele) {
    var self = this;
    var direction = this.direction();
    if (this.select) this.select.value = direction;

    this.mounted = pele.mount(this.root, this.source(direction), {
      idPrefix: this.id + '-',
      autoDirection: false,
      icons: function (name) { return self.iconMarkup[name]; },
      config: { flowchart: { curve: this.config.curve || 'step' } },
      zoom: {
        always: true,
        labels: {
          zoomIn: this.root.getAttribute('data-sparql-flow-zoom-in'),
          zoomOut: this.root.getAttribute('data-sparql-flow-zoom-out'),
          reset: this.root.getAttribute('data-sparql-flow-zoom-reset')
        }
      },
      onRender: function () { self.dropTrailingArrows(); },
      onError: function (err) { self.showError(err); }
    });

    if (this.select) {
      this.select.addEventListener('change', function () {
        var dir = self.select.value;
        storageSet('flow-direction-' + self.id, dir);
        self.mounted.update(self.source(dir));
      });
    }
  };

  // Drag the corner handle to change the card height; remembered per diagram.
  FlowDiagram.prototype.wireResize = function () {
    var self = this;
    var handle = document.getElementById(this.id + '-resize-handle');
    var body = this.cardBody;
    if (!handle || !body) return;

    var saved = storageGet('flow-height-' + this.id);
    if (saved) body.style.height = saved;

    var resizing = false;
    var startY = 0;
    var startHeight = 0;

    handle.addEventListener('mousedown', function (e) {
      resizing = true;
      startY = e.clientY;
      startHeight = body.offsetHeight;
      e.preventDefault();
    });
    document.addEventListener('mousemove', function (e) {
      if (!resizing) return;
      body.style.height = Math.max(200, startHeight + e.clientY - startY) + 'px';
    });
    document.addEventListener('mouseup', function () {
      if (!resizing) return;
      resizing = false;
      storageSet('flow-height-' + self.id, body.style.height);
      if (self.mounted && self.mounted.zoom) self.mounted.zoom.refresh();
    });
  };

  FlowDiagram.prototype.init = function () {
    var self = this;
    this.wireResize();
    Promise.all([loadPele(), this.loadIcons()]).then(function (loaded) {
      var pele = loaded[0];
      self.render(pele);
    }, function (err) {
      self.showError(err);
    });
  };

  // Re-entrant on purpose: a duplicate <script src> tag, or this file being
  // re-executed inside an HTMX-swapped fragment, must still pick up elements
  // that were not in the DOM the first time. There is deliberately no
  // module-level "already loaded" latch — only the per-element guard below.
  function boot() {
    document.querySelectorAll('[data-sparql-flow]').forEach(function (root) {
      if (root.__visotoSparqlFlowInit) return;
      root.__visotoSparqlFlowInit = true;
      new FlowDiagram(root).init();
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
