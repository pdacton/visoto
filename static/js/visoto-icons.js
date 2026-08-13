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
    - a resource's own local name wins over any of its types
    - across several types, ANY exact match beats ANY .fallback match
      (a LINDAS resource routinely carries a generic class alongside a specific
      one, often listed first)
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

  // available is the -available-icons island: bare names for real icons,
  // "<name>.fallback" for the weaker ones. Returns '' when nothing matches, so
  // each caller applies its own default.
  function resolve(iri, types, available) {
    if (!available) return '';
    types = types || [];

    var name = localName(iri);
    if (name) {
      if (available[name]) return BASE + name + '.svg';
      if (available[name + '.fallback']) return BASE + name + '.fallback.svg';
    }

    var i, n;
    for (i = 0; i < types.length; i++) {
      n = localName(types[i]);
      if (n && available[n]) return BASE + n + '.svg';
    }
    for (i = 0; i < types.length; i++) {
      n = localName(types[i]);
      if (n && available[n + '.fallback']) return BASE + n + '.fallback.svg';
    }
    return '';
  }

  window.VisotoIcons = {
    basePath: BASE,
    localName: localName,
    resolve: resolve
  };
})();
