/* eslint-disable */
/*
  Turtle for "Export as Turtle" (GL-38): the triples drawn on a canvas. Pure —
  no GE, no DOM — so `node --test tests/js/` covers it.

    toTurtle({
      elements: [{ iri, types: [iri], labels: [LocalizedString],
                   properties: { <iri>: { type: 'uri' | 'string', values } } }],
      links:    [{ source: iri, type: iri, target: iri }],
    }) -> string

  LocalizedString is GE's { value, language, datatype?: { value } }. What is
  drawn: each node's types and label(s), the property rows of expanded nodes
  (the caller passes properties only for those), and every visible edge.
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.VisotoTurtle = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var PREFIXES = [
    ['rdf', 'http://www.w3.org/1999/02/22-rdf-syntax-ns#'],
    ['rdfs', 'http://www.w3.org/2000/01/rdf-schema#'],
    ['owl', 'http://www.w3.org/2002/07/owl#'],
    ['xsd', 'http://www.w3.org/2001/XMLSchema#'],
    ['skos', 'http://www.w3.org/2004/02/skos/core#'],
    ['schema', 'http://schema.org/'],
    ['dcterms', 'http://purl.org/dc/terms/'],
    ['prov', 'http://www.w3.org/ns/prov#'],
    ['foaf', 'http://xmlns.com/foaf/0.1/'],
    ['dcat', 'http://www.w3.org/ns/dcat#'],
    ['schch', 'https://schema.ld.admin.ch/'],
    ['cube', 'https://cube.link/'],
  ];
  var RDF_TYPE = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type';
  var RDFS_LABEL = 'http://www.w3.org/2000/01/rdf-schema#label';
  var XSD_STRING = 'http://www.w3.org/2001/XMLSchema#string';
  var LOCAL = /^[A-Za-z_][A-Za-z0-9_-]*$/;

  function iriRef(iri) {
    // Turtle IRIREF may not contain these; escape them as UCHAR.
    return '<' + String(iri).replace(/[\u0000- <>"{}|^`\\]/g, function (ch) {
      return '\\u' + ('000' + ch.charCodeAt(0).toString(16).toUpperCase()).slice(-4);
    }) + '>';
  }

  function literal(s) {
    var text = '"' + String(s.value)
      .replace(/\\/g, '\\\\').replace(/"/g, '\\"')
      .replace(/\n/g, '\\n').replace(/\r/g, '\\r').replace(/\t/g, '\\t') + '"';
    if (s.language) return text + '@' + s.language;
    var dt = s.datatype && s.datatype.value;
    return dt && dt !== XSD_STRING ? text + '^^' + dt : text; // dt resolved by term() below
  }

  function toTurtle(graph) {
    var used = {};
    function term(iri) {
      if (iri === RDF_TYPE) return 'a';
      for (var i = 0; i < PREFIXES.length; i++) {
        var ns = PREFIXES[i][1];
        if (iri.indexOf(ns) === 0 && LOCAL.test(iri.slice(ns.length))) {
          used[PREFIXES[i][0]] = true;
          return PREFIXES[i][0] + ':' + iri.slice(ns.length);
        }
      }
      return iriRef(iri);
    }
    function lit(s) {
      var out = literal(s);
      var at = out.lastIndexOf('^^');
      if (at > 0 && !s.language) out = out.slice(0, at + 2) + term(out.slice(at + 2));
      return out;
    }

    // subject IRI -> predicate IRI -> [object text]
    var subjects = {};
    var order = [];
    function add(s, p, o) {
      if (!subjects[s]) { subjects[s] = {}; order.push(s); }
      var preds = subjects[s];
      (preds[p] = preds[p] || []);
      if (preds[p].indexOf(o) < 0) preds[p].push(o);
    }

    (graph.elements || []).forEach(function (el) {
      (el.types || []).forEach(function (t) { add(el.iri, RDF_TYPE, term(t)); });
      (el.labels || []).forEach(function (l) { add(el.iri, RDFS_LABEL, lit(l)); });
      var props = el.properties || {};
      Object.keys(props).forEach(function (p) {
        var prop = props[p];
        (prop.values || []).forEach(function (v) {
          add(el.iri, p, prop.type === 'uri' ? term(v.value) : lit(v));
        });
      });
    });
    (graph.links || []).forEach(function (l) { add(l.source, l.type, term(l.target)); });

    var body = order.map(function (s) {
      var preds = subjects[s];
      var keys = Object.keys(preds).sort(function (a, b) {
        return (a === RDF_TYPE ? -1 : 0) - (b === RDF_TYPE ? -1 : 0) || a.localeCompare(b);
      });
      return term(s) + '\n' + keys.map(function (p) {
        return '    ' + term(p) + ' ' + preds[p].join(', ');
      }).join(' ;\n') + ' .';
    }).join('\n\n');

    var head = PREFIXES.filter(function (p) { return used[p[0]]; })
      .map(function (p) { return '@prefix ' + p[0] + ': <' + p[1] + '> .'; }).join('\n');
    return (head ? head + '\n\n' : '') + body + '\n';
  }

  return { toTurtle: toTurtle };
});
