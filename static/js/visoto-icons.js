/* eslint-disable */
/*
  Resource icon resolution for the clients that must do it themselves.

  The Go mirror of this file is internal/icon (Resolve + LocalName), and it is
  the authority: SPARQL tables get their icons resolved server-side and only look
  them up in a map. The two Graph Explorer partials cannot, because Graph Explorer
  supplies an element's rdf:type from its own data provider while rendering —
  there is no server round trip left to attach to. They resolve here instead,
  against the full name map the -available-icons island carries.

  Keep the rules in step with internal/icon/icon.go:
    - a resource's own IRI wins over any of its types
    - across several types, ANY exact match beats ANY .fallback match
      (a LINDAS resource routinely carries a generic class alongside a specific
      one, often listed first)
    - within each step, a namespaced icon (rdf/Property.svg, keyed by full IRI
      and CURIE) beats a root icon (Property.svg, keyed by local name)
*/
(function () {
  'use strict';

  var BASE = '/static/img/resource/';

  // Mirror of icon.LocalName. Order matters: decode first, because a prefixed
  // form arriving from a URL ("schema%3APerson") hides its colon until then.
  function localName(iri) {
    if (!iri) return '';
    try { iri = decodeURIComponent(iri); } catch (e) { /* use as-is */ }
    iri = iri.replace(/\/+$/, '');
    if (iri.indexOf('#') !== -1) {
      iri = iri.split('#').pop();
    } else if (iri.indexOf('/') !== -1) {
      iri = iri.split('/').pop();
    }
    if (iri.indexOf(':') !== -1) iri = iri.split(':').pop();
    return iri;
  }

  function decode(iri) {
    try { return decodeURIComponent(iri); } catch (e) { return iri; }
  }

  // Mirror of icon.lookup: the namespaced key (IRI or CURIE) first, then the
  // bare local name. suffix is '' or '.fallback'. Returns a path or ''.
  function lookup(iri, suffix, available) {
    if (!iri) return '';
    var rel = available[decode(iri) + suffix];
    if (!rel) {
      var name = localName(iri);
      if (name) rel = available[name + suffix];
    }
    return rel ? BASE + rel : '';
  }

  // available is the -available-icons island (icon.Names): lookup key -> file
  // path relative to BASE; fallback keys carry a ".fallback" suffix. Returns ''
  // when nothing matches, so each caller applies its own default.
  function resolve(iri, types, available) {
    if (!available) return '';
    types = types || [];

    var url = lookup(iri, '', available) || lookup(iri, '.fallback', available);
    if (url) return url;

    var i;
    for (i = 0; i < types.length; i++) {
      url = lookup(types[i], '', available);
      if (url) return url;
    }
    for (i = 0; i < types.length; i++) {
      url = lookup(types[i], '.fallback', available);
      if (url) return url;
    }
    return '';
  }

  window.VisotoIcons = {
    basePath: BASE,
    localName: localName,
    resolve: resolve
  };
})();
