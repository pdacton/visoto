/* eslint-disable */
/*
  In-memory Graph Explorer data source over a set of CONSTRUCTed triples.

  The graph-explorer@2.1.0 CDN bundle has no RDFDataProvider, so a diagram whose
  nodes and edges do not exist verbatim in the endpoint (a derived schema, an
  ontology projected into UML) cannot go through SparqlDataProvider. Callers run
  their own CONSTRUCT, then hand the N-Triples here:

    var G = window.VisotoMemoryGraph;
    var store = G.buildStore(G.parseNTriples(text), availableIcons);
    model.importLayout({ dataProvider: G.makeProvider(store), ... });

  Shared by static/js/schema-graph.js and the construct mode of
  static/js/sparql-graph.js. Load it after visoto-icons.js and before either.

  How triples map onto the diagram:
    - an IRI is a NODE when it has an rdf:type, is the subject of a literal
      (attribute row), or is either end of an IRI-valued triple (an edge);
    - `<node> rdfs:label "x"` labels the node;
    - `<node> <p> "literal"` is an attribute row on the node;
    - `<a> <p> <b>` (p != rdf:type) is an edge of link type <p>;
    - `<p> rdfs:label "x"` for any IRI that is NOT a node labels it as a link
      type / attribute property — this is how an edge takes its label from the
      property it was projected from, rather than from the IRI's local name.
      Labels of marker types (below) work the same way and become the box's
      type line.
    - `<node> a <urn:visoto:Name>` is a presentation marker (see MARKER_PREFIX).
*/
(function () {
  'use strict';

  var RDF_TYPE = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type';
  var RDFS_LABEL = 'http://www.w3.org/2000/01/rdf-schema#label';
  var RDFS_CLASS = 'http://www.w3.org/2000/01/rdf-schema#Class';
  var OWL_CLASS = 'http://www.w3.org/2002/07/owl#Class';
  var RDFS_SUBCLASS = 'http://www.w3.org/2000/01/rdf-schema#subClassOf';

  // Presentation markers: a CONSTRUCT may type a node urn:visoto:<Name> to ask
  // for a distinct look (e.g. urn:visoto:ExternalClass on the ontology diagram).
  // GE's StandardTemplate exposes nothing per-node that CSS can select on, and the
  // CDN bundle keeps its React private, so a template subclass cannot add a class.
  // The one per-node value that reaches the DOM verbatim is the thumbnail's
  // <img src> (data.image), so the marker rides there as a URL fragment
  // ("…svg#vs-ExternalClass") and ontodia_overrides.css selects on it with :has().
  // The fragment does not change which image loads.
  var MARKER_PREFIX = 'urn:visoto:';
  var MARKER_FALLBACK_IMAGE = '/static/img/resource/defaultClass.svg';

  // StandardTemplate titles a box with its foaf:name property value in preference
  // to the model label, so an attribute row "foaf:name: rdf:langString" (the
  // ontology diagram's foaf:Agent) would rename the box "rdf:langString". Such a
  // row is keyed by an alias IRI instead; the alias carries foaf:name's label.
  var FOAF_NAME = 'http://xmlns.com/foaf/0.1/name';
  var FOAF_NAME_ALIAS = 'urn:visoto:attribute:' + FOAF_NAME;

  function localName(iri) {
    var decoded;
    try { decoded = decodeURIComponent(iri); } catch (e) { decoded = iri; }
    return decoded.includes('#') ? decoded.split('#').pop() : decoded.split('/').pop();
  }

  // ---------------------------------------------------------------------------
  // Minimal N-Triples parser — IRIs and plain/typed/language literals. Blank-node
  // lines are skipped: every caller's CONSTRUCT filters to IRIs.
  // ---------------------------------------------------------------------------
  function unescapeNt(s) {
    return s.replace(/\\u([0-9A-Fa-f]{4})/g, function (_, h) { return String.fromCharCode(parseInt(h, 16)); })
      .replace(/\\(.)/g, function (_, c) {
        return c === 'n' ? '\n' : c === 't' ? '\t' : c === 'r' ? '\r' : c;
      });
  }
  function parseNTriples(text) {
    var triples = [];
    var lineRe = /^<([^>]*)>\s+<([^>]*)>\s+(.+?)\s*\.\s*$/;
    var litRe = /^"((?:[^"\\]|\\.)*)"(?:@([A-Za-z][A-Za-z0-9-]*)|\^\^<([^>]*)>)?$/;
    text.split('\n').forEach(function (line) {
      line = line.trim();
      if (!line || line.charAt(0) === '#') return;
      var m = line.match(lineRe);
      if (!m) return;
      var s = m[1], p = m[2], oRaw = m[3], o;
      if (oRaw.charAt(0) === '<') {
        o = { type: 'iri', value: oRaw.slice(1, -1) };
      } else {
        var lm = oRaw.match(litRe);
        if (!lm) return;
        o = { type: 'literal', value: unescapeNt(lm[1]), language: lm[2] || '' };
      }
      triples.push({ s: s, p: p, o: o });
    });
    return triples;
  }

  // ---------------------------------------------------------------------------
  // Store: elements (ElementModel by IRI), links (LinkModel[]) and labels for
  // the IRIs that are only ever predicates.
  // ---------------------------------------------------------------------------
  function buildStore(triples, availableIcons) {
    // Pass 1: which IRIs are nodes. Needed up front because a label triple can
    // arrive before the triple that makes its subject a node.
    var isNode = {};
    triples.forEach(function (t) {
      if (t.p === RDF_TYPE) { isNode[t.s] = true; return; }
      if (t.p === RDFS_LABEL) return;
      isNode[t.s] = true;
      if (t.o.type === 'iri') isNode[t.o.value] = true;
    });

    var elements = {};   // iri -> ElementModel
    var links = [];      // LinkModel[]
    var labels = {};     // iri -> [{value, language}] for non-node IRIs
    var seenLink = {};

    function element(iri) {
      if (!elements[iri]) {
        elements[iri] = { id: iri, types: [], label: { values: [] }, properties: {} };
      }
      return elements[iri];
    }

    triples.forEach(function (t) {
      if (t.p === RDFS_LABEL && t.o.type === 'literal') {
        var lv = { value: t.o.value, language: t.o.language };
        if (isNode[t.s]) element(t.s).label.values.push(lv);
        else (labels[t.s] = labels[t.s] || []).push(lv);
        return;
      }
      var el = element(t.s);
      if (t.p === RDF_TYPE && t.o.type === 'iri') {
        if (el.types.indexOf(t.o.value) < 0) el.types.push(t.o.value);
      } else if (t.o.type === 'literal') {
        var propKey = t.p === FOAF_NAME ? FOAF_NAME_ALIAS : t.p;
        if (!el.properties[propKey]) el.properties[propKey] = { type: 'string', values: [] };
        var vals = el.properties[propKey].values;
        if (!vals.some(function (v) { return v.value === t.o.value; })) {
          vals.push({ value: t.o.value, language: t.o.language });
        }
      } else {
        var key = t.s + ' ' + t.p + ' ' + t.o.value;
        if (seenLink[key]) return;
        seenLink[key] = true;
        element(t.o.value);
        links.push({ linkTypeId: t.p, sourceId: t.s, targetId: t.o.value });
      }
    });

    Object.keys(elements).forEach(function (iri) {
      var el = elements[iri];
      if (el.label.values.length === 0) {
        el.label.values.push({ value: localName(iri), language: '' });
      }
      // A class node's rdf:type is generic (rdfs:Class / owl:Class), so the
      // typeStyleResolver -- which GE hands nothing but the type array -- can only
      // return the generic class icon. Put the node's own-IRI icon on the model
      // instead: renderThumbnail() prefers data.image, and it survives GE 2.x's
      // props rebuild because it IS the model.
      var icon = window.VisotoIcons.resolve(iri, [], availableIcons || {});
      if (icon) el.image = icon;

      var marker = el.types.filter(function (t) { return t.indexOf(MARKER_PREFIX) === 0; })[0];
      if (marker) {
        el.image = (el.image || MARKER_FALLBACK_IMAGE).split('#')[0] + '#vs-' + marker.slice(MARKER_PREFIX.length);
      }
    });

    if (labels[FOAF_NAME]) labels[FOAF_NAME_ALIAS] = labels[FOAF_NAME];

    return { elements: elements, links: links, labels: labels };
  }

  // ---------------------------------------------------------------------------
  // DataProvider over a store (implements the interface the CDN bundle expects,
  // so search / class tree / connections menus work locally).
  // ---------------------------------------------------------------------------
  // A node the CONSTRUCT itself types owl:Class / rdfs:Class.
  function isDeclaredClass(store, iri) {
    var el = store.elements[iri];
    return !!el && (el.types.indexOf(OWL_CLASS) >= 0 || el.types.indexOf(RDFS_CLASS) >= 0);
  }

  function makeProvider(store) {
    function label(iri) {
      var vals = store.labels[iri];
      return { values: vals && vals.length ? vals : [{ value: localName(iri), language: '' }] };
    }
    function linkTypeList() {
      var counts = {};
      store.links.forEach(function (l) { counts[l.linkTypeId] = (counts[l.linkTypeId] || 0) + 1; });
      return Object.keys(counts).map(function (id) {
        return { id: id, label: label(id), count: counts[id] };
      });
    }
    function elementDict(iris) {
      var dict = {};
      iris.forEach(function (iri) {
        if (store.elements[iri]) dict[iri] = store.elements[iri];
      });
      return dict;
    }
    function matchesText(el, text) {
      if (!text) return true;
      var t = text.toLowerCase();
      return el.id.toLowerCase().indexOf(t) >= 0 ||
        el.label.values.some(function (v) { return v.value.toLowerCase().indexOf(t) >= 0; });
    }
    function classNodeIds() {
      return Object.keys(store.elements).filter(function (id) {
        return isDeclaredClass(store, id);
      });
    }
    function sortByLabel(nodes) {
      function text(n) { return (n.label.values[0] || { value: n.id }).value.toLowerCase(); }
      return nodes.sort(function (a, b) { return text(a) < text(b) ? -1 : text(a) > text(b) ? 1 : 0; });
    }
    // The class and everything below it via rdfs:subClassOf edges.
    function subclassClosure(id) {
      var seen = {};
      var queue = [id];
      seen[id] = true;
      while (queue.length) {
        var cur = queue.shift();
        store.links.forEach(function (l) {
          if (l.linkTypeId === RDFS_SUBCLASS && l.targetId === cur && !seen[l.sourceId]) {
            seen[l.sourceId] = true;
            queue.push(l.sourceId);
          }
        });
      }
      return seen;
    }
    function neighbours(elementId, linkId, direction) {
      var iris = [];
      store.links.forEach(function (l) {
        if (linkId && l.linkTypeId !== linkId) return;
        if (l.sourceId === elementId && direction !== 'in') iris.push(l.targetId);
        if (l.targetId === elementId && direction !== 'out') iris.push(l.sourceId);
      });
      return iris;
    }

    return {
      // The diagram's own class nodes (typed owl:Class / rdfs:Class), nested
      // by the rdfs:subClassOf edges between them. Marker-typed nodes (external
      // classes, placeholders) are not classes of the diagram and stay out. GE
      // adds each id once and breaks cycles itself, so a class with two
      // superclasses may safely appear under both. count 0 hides the badge: the
      // nodes are classes, not instance counts.
      classTree: function () {
        var ids = classNodeIds();
        if (ids.length === 0) {
          return Promise.resolve([{
            id: RDFS_CLASS,
            label: { values: [{ value: 'Class', language: '' }] },
            count: Object.keys(store.elements).length,
            children: [],
          }]);
        }
        var isClass = {};
        ids.forEach(function (id) { isClass[id] = true; });
        var children = {};
        var hasParent = {};
        store.links.forEach(function (l) {
          if (l.linkTypeId !== RDFS_SUBCLASS || !isClass[l.sourceId] || !isClass[l.targetId]) return;
          (children[l.targetId] = children[l.targetId] || []).push(l.sourceId);
          hasParent[l.sourceId] = true;
        });
        var visiting = {};
        function node(id) {
          visiting[id] = true;
          var kids = (children[id] || [])
            .filter(function (c) { return !visiting[c]; })
            .map(node);
          delete visiting[id];
          return { id: id, label: store.elements[id].label, count: 0, children: sortByLabel(kids) };
        }
        return Promise.resolve(sortByLabel(ids.filter(function (id) { return !hasParent[id]; }).map(node)));
      },
      classInfo: function (params) {
        return Promise.resolve(params.classIds.map(function (id) {
          var el = store.elements[id];
          return { id: id, label: el ? el.label : label(id), count: 0, children: [] };
        }));
      },
      propertyInfo: function (params) {
        var dict = {};
        params.propertyIds.forEach(function (id) { dict[id] = { id: id, label: label(id) }; });
        return Promise.resolve(dict);
      },
      linkTypes: function () { return Promise.resolve(linkTypeList()); },
      linkTypesInfo: function (params) {
        return Promise.resolve(params.linkTypeIds.map(function (id) {
          return { id: id, label: label(id) };
        }));
      },
      elementInfo: function (params) {
        return Promise.resolve(elementDict(params.elementIds));
      },
      linksInfo: function (params) {
        var inSet = {};
        params.elementIds.forEach(function (id) { inSet[id] = true; });
        return Promise.resolve(store.links.filter(function (l) {
          return inSet[l.sourceId] && inSet[l.targetId];
        }));
      },
      linkTypesOf: function (params) {
        var counts = {};
        store.links.forEach(function (l) {
          if (l.sourceId === params.elementId) {
            counts[l.linkTypeId] = counts[l.linkTypeId] || { id: l.linkTypeId, inCount: 0, outCount: 0 };
            counts[l.linkTypeId].outCount++;
          }
          // A self-loop (Dictionary -derivedByInsertionFrom-> Dictionary) is one
          // link; counting it as both out and in made the Connections badge read 2.
          if (l.targetId === params.elementId && l.sourceId !== params.elementId) {
            counts[l.linkTypeId] = counts[l.linkTypeId] || { id: l.linkTypeId, inCount: 0, outCount: 0 };
            counts[l.linkTypeId].inCount++;
          }
        });
        return Promise.resolve(Object.keys(counts).map(function (k) { return counts[k]; }));
      },
      linkElements: function (params) {
        return Promise.resolve(elementDict(neighbours(params.elementId, params.linkId, params.direction)));
      },
      filter: function (params) {
        var iris = params.refElementId
          ? neighbours(params.refElementId, params.refElementLinkId, params.linkDirection)
          : Object.keys(store.elements);
        // Picking a class in the tree lists that class node and its subclasses
        // (the tree's classes ARE diagram nodes), so they can be dragged onto
        // the canvas; any other type id filters by rdf:type as usual.
        var subtree = params.elementTypeId && store.elements[params.elementTypeId]
          ? subclassClosure(params.elementTypeId) : null;
        var dict = {};
        iris.forEach(function (iri) {
          var el = store.elements[iri];
          if (!el) return;
          if (params.elementTypeId && !(subtree && subtree[iri]) && el.types.indexOf(params.elementTypeId) < 0) return;
          if (!matchesText(el, params.text)) return;
          dict[iri] = el;
        });
        return Promise.resolve(dict);
      },
    };
  }

  // ---------------------------------------------------------------------------
  // The diagram as a facade over a live endpoint (construct mode of
  // sparql-graph.js). The first picture is exactly the store; past it, GE
  // behaves as if the triple store were behind it: a class click lists its
  // instances, Connections lists the endpoint's links too, and whatever is
  // pulled in renders like in browse mode. Plan: .project/todo/ontology-live-provider.md
  //
  // GE's own CompositeDataProvider does not fit: fetchAll injects a "data
  // provider" row into every box, duplicates properties and sums connection
  // counts; sequentialFetching asks the second source only when the first
  // answered nothing, so Connections on a class node never reaches the endpoint.
  //
  // `live` is a SparqlDataProvider. A failed live call leaves the diagram's
  // answer standing: a dead endpoint must not break the facade.
  // ---------------------------------------------------------------------------
  function makeHybridProvider(store, live) {
    var base = makeProvider(store);

    function isNode(id) { return !!store.elements[id]; }
    // Placeholders and attribute aliases (urn:visoto:*) exist only here.
    function forEndpoint(id) { return /^https?:\/\//i.test(id); }
    function soft(promise) { return promise.catch(function () { return null; }); }
    // A diagram node keeps the diagram's model whoever reported it, so its
    // attribute rows and marker image never mix with endpoint data.
    function withOwnModels(dict) {
      var out = {};
      Object.keys(dict || {}).forEach(function (id) { out[id] = store.elements[id] || dict[id]; });
      return out;
    }
    function mergeDicts(a, b) { return Object.assign({}, withOwnModels(b), a); }

    // Schema lookups: the diagram answers what it knows (its own labels), the
    // endpoint the ids only it brought in.
    function schemaLookup(method, key, known) {
      return function (params) {
        var ids = params[key];
        var missing = ids.filter(function (id) { return !known(id) && forEndpoint(id); });
        var own = base[method](params);
        if (!missing.length) return own;
        var q = {}; q[key] = missing;
        return Promise.all([own, soft(live[method](q))]).then(function (r) {
          if (!r[1]) return r[0];
          if (Array.isArray(r[0])) {
            var fromLive = {};
            r[1].forEach(function (m) { fromLive[m.id] = m; });
            return r[0].map(function (m) { return fromLive[m.id] || m; });
          }
          return Object.assign({}, r[0], r[1]);
        });
      };
    }
    function hasLabel(id) { return isNode(id) || !!store.labels[id]; }

    return {
      classTree: base.classTree,
      linkTypes: base.linkTypes,
      classInfo: schemaLookup('classInfo', 'classIds', hasLabel),
      propertyInfo: schemaLookup('propertyInfo', 'propertyIds', hasLabel),
      linkTypesInfo: schemaLookup('linkTypesInfo', 'linkTypeIds', hasLabel),

      elementInfo: function (params) {
        var rest = params.elementIds.filter(function (id) { return !isNode(id) && forEndpoint(id); });
        var own = base.elementInfo(params);
        if (!rest.length) return own;
        return Promise.all([own, soft(live.elementInfo({ elementIds: rest }))]).then(function (r) {
          return Object.assign({}, r[1] || {}, r[0]);
        });
      },

      // Endpoint links only when one end is not a diagram node: two class
      // boxes never gain an edge the CONSTRUCT did not draw. Nothing but
      // diagram nodes on the canvas (the first picture) asks the endpoint nothing.
      linksInfo: function (params) {
        var own = base.linksInfo(params);
        var ids = params.elementIds.filter(forEndpoint);
        if (!ids.some(function (id) { return !isNode(id); })) return own;
        return Promise.all([own, soft(live.linksInfo(Object.assign({}, params, { elementIds: ids })))]).then(function (r) {
          var links = r[0].slice();
          var seen = {};
          links.forEach(function (l) { seen[l.sourceId + ' ' + l.linkTypeId + ' ' + l.targetId] = true; });
          (r[1] || []).forEach(function (l) {
            if (isNode(l.sourceId) && isNode(l.targetId)) return;
            var key = l.sourceId + ' ' + l.linkTypeId + ' ' + l.targetId;
            if (seen[key]) return;
            seen[key] = true;
            links.push(l);
          });
          return links;
        });
      },

      // Merged by link type. An edge both sources know (a generalization is the
      // endpoint's rdfs:subClassOf) must not count twice, so each direction
      // takes the larger count rather than the sum.
      linkTypesOf: function (params) {
        var own = base.linkTypesOf(params);
        if (!forEndpoint(params.elementId)) return own;
        return Promise.all([own, soft(live.linkTypesOf(params))]).then(function (r) {
          var byId = {};
          r[0].concat(r[1] || []).forEach(function (c) {
            var m = byId[c.id];
            byId[c.id] = m ? { id: c.id, inCount: Math.max(m.inCount, c.inCount), outCount: Math.max(m.outCount, c.outCount) } : c;
          });
          return Object.keys(byId).map(function (k) { return byId[k]; });
        });
      },

      linkElements: function (params) {
        var own = base.linkElements(params);
        if (!forEndpoint(params.elementId)) return own;
        return Promise.all([own, soft(live.linkElements(params))]).then(function (r) {
          return mergeDicts(r[0], r[1]);
        });
      },

      // - a class picked in the tree: its instances, from the endpoint (the
      //   diagram holds classes, not instances);
      // - Connections "show elements": both sources;
      // - text search over the diagram: the diagram.
      filter: function (params) {
        if (params.refElementId) {
          var own = base.filter(params);
          if (!forEndpoint(params.refElementId)) return own;
          return Promise.all([own, soft(live.filter(params))]).then(function (r) {
            return mergeDicts(r[0], r[1]);
          });
        }
        if (params.elementTypeId && forEndpoint(params.elementTypeId)) {
          return live.filter(params).then(withOwnModels);
        }
        return base.filter(params);
      },
    };
  }

  var api = {
    localName: localName,
    parseNTriples: parseNTriples,
    buildStore: buildStore,
    makeProvider: makeProvider,
    makeHybridProvider: makeHybridProvider,
    isDeclaredClass: isDeclaredClass,
  };
  if (typeof module === 'object' && module.exports) module.exports = api;
  else window.VisotoMemoryGraph = api;
})();
