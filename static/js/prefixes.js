/*
  Prefix declarations of the active endpoint, for browser JS.

  The server renders the endpoint's merged prefix list (global [rdf] prefixes
  plus the endpoint's own overrides, see docs/configuration.md "Per-endpoint
  prefixes") into the #vs-prefixes island in base.html. This module is the only
  way browser code should reach a vocabulary namespace: write the term as a
  CURIE and expand() it. Then an endpoint that redeclares a prefix — schema: as
  https on one endpoint, http on another, in either direction — is followed
  without any namespace hard-coded in JS.

    VisotoPrefixes.fromPage()        instance for this page (cached)
    VisotoPrefixes.create(list)      instance from [{name, iri}]
    VisotoPrefixes.labelPath()       the "label" magic property path

    p.expand('schema:name')          'http://schema.org/name' (or https …)
                                     unknown names and full IRIs pass through
    p.compact(iri)                   { prefix, local } or null — for display and
                                     export only: two names may share an IRI
    p.sparqlBlock()                  'PREFIX a: <…>\n' for every entry

  compact() takes the longest matching namespace and, on a tie, the first name
  in the list — the same rule as shortenIRI in internal/resource/resource.go.
  Matching code must use expand(), which is unambiguous.
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.VisotoPrefixes = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  function create(list) {
    var entries = (Array.isArray(list) ? list : []).filter(function (p) {
      return p && typeof p.name === 'string' && typeof p.iri === 'string' && p.iri !== '';
    });
    var byName = {};
    entries.forEach(function (p) {
      if (!Object.prototype.hasOwnProperty.call(byName, p.name)) byName[p.name] = p.iri;
    });

    function expand(curie) {
      var s = String(curie);
      var i = s.indexOf(':');
      if (i <= 0) return s;
      var local = s.slice(i + 1);
      if (local.slice(0, 2) === '//') return s; // a full IRI, not a CURIE
      var ns = byName[s.slice(0, i)];
      return ns === undefined ? s : ns + local;
    }

    function compact(iri) {
      var s = String(iri);
      var best = null;
      entries.forEach(function (p) {
        if (s.indexOf(p.iri) === 0 && (!best || p.iri.length > best.iri.length)) best = p;
      });
      return best ? { prefix: best.name, local: s.slice(best.iri.length) } : null;
    }

    function sparqlBlock() {
      return entries.map(function (p) { return 'PREFIX ' + p.name + ': <' + p.iri + '>\n'; }).join('');
    }

    return { list: entries, expand: expand, compact: compact, sparqlBlock: sparqlBlock };
  }

  var page = null;

  // The instance for this page, from the #vs-prefixes island. A missing island or
  // bad JSON yields an empty list: nothing expands or compacts, nothing throws.
  function fromPage() {
    if (page) return page;
    var list = [];
    try {
      var el = typeof document !== 'undefined' && document.getElementById('vs-prefixes');
      if (el) list = JSON.parse(el.innerHTML.trim()); // a <template> island, read like graph-kit.js readIsland
    } catch (e) {
      if (typeof console !== 'undefined') console.warn('vs-prefixes island unreadable', e);
    }
    page = create(list);
    return page;
  }

  // The "label" magic property from visoto.config (#vs-label-path island): the
  // label properties as a SPARQL alternative path, in priority order. Falls
  // back to rdfs:label when the island is absent.
  function labelPath() {
    try {
      var el = typeof document !== 'undefined' && document.getElementById('vs-label-path');
      var path = el && JSON.parse(el.innerHTML.trim());
      if (typeof path === 'string' && path) return path;
    } catch (e) { /* fall through */ }
    return 'rdfs:label';
  }

  return { create: create, fromPage: fromPage, labelPath: labelPath };
});
