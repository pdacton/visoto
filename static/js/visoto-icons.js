/* eslint-disable */
/*
  Resource icon resolution in the browser.

  internal/icon is the authority: SPARQL tables, trees and the page header get
  their icons resolved server-side. Graph Explorer cannot, because it supplies an
  element's rdf:type from its own data provider while rendering — there is no
  server round trip left to attach to — so the graph embeds resolve here.

  Two inputs, both from internal/icon:
    - the #vs-icons island in base.html (icon.Config): base path, the default per
      kind and the manifest URL. On every page, read synchronously.
    - the manifest (/api/icons.json?v=…, icon.Manifest): the icon set. Fetched
      once per page, only by the code that resolves, via load().

    VisotoIcons.load()                 Promise; resolves once the set is known
                                       (to an empty set if the fetch fails)
    VisotoIcons.resolve(subject, kind) path, or kind's default ('' for 'cell')
                                       subject = { iri, types }
    VisotoIcons.stamp(data, iri)       sets data.image to iri's own icon unless
                                       data already carries an image
    VisotoIcons.defaultFor(kind)       'page' | 'class' | 'instance' -> path
    VisotoIcons.isDefault(src)         whether src is one of those defaults

  Keep resolve() in step with Resolver.Resolve in internal/icon/icon.go:
    - a resource's own local name wins over any of its types
    - across several types, ANY exact match beats ANY .fallback match
      (a LINDAS resource routinely carries a generic class alongside a specific
      one, often listed first)
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.VisotoIcons = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var config = null;   // the #vs-icons island
  var names = {};      // the manifest's icon set
  var loading = null;  // the one manifest fetch per page

  function readConfig() {
    if (config) return config;
    config = { manifestUrl: '', base: '/static/img/resource/', defaults: {} };
    try {
      var el = typeof document !== 'undefined' && document.getElementById('vs-icons');
      if (el) config = JSON.parse(el.innerHTML.trim()); // a <template> island, like vs-prefixes
    } catch (e) {
      if (typeof console !== 'undefined') console.warn('vs-icons island unreadable', e);
    }
    return config;
  }

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

  function defaultFor(kind) {
    return (readConfig().defaults || {})[kind] || '';
  }

  function isDefault(src) {
    var d = readConfig().defaults || {};
    return Object.keys(d).some(function (k) { return d[k] && d[k] === src; });
  }

  // Mirror of Resolver.Resolve. names holds bare names for real icons and
  // "<name>.fallback" for the weaker ones.
  function resolve(subject, kind) {
    var base = readConfig().base;
    var iri = subject && subject.iri;
    var types = (subject && subject.types) || [];

    var name = localName(iri);
    if (name) {
      if (names[name]) return base + name + '.svg';
      if (names[name + '.fallback']) return base + name + '.fallback.svg';
    }

    var i, n;
    for (i = 0; i < types.length; i++) {
      n = localName(types[i]);
      if (n && names[n]) return base + n + '.svg';
    }
    for (i = 0; i < types.length; i++) {
      n = localName(types[i]);
      if (n && names[n + '.fallback']) return base + n + '.fallback.svg';
    }
    return defaultFor(kind);
  }

  // Graph Explorer draws data.image in preference to the type-resolved icon, and
  // data.image survives GE 2.x's props rebuild because it IS the model — so this
  // is how a node shows its OWN icon (a class node, whose rdf:type owl:Class
  // would otherwise give every class the same box). An image the data already
  // carries wins. Returns data, for chaining.
  function stamp(data, iri) {
    if (!data || data.image) return data;
    var url = resolve({ iri: iri }, 'cell');
    if (url) data.image = url;
    return data;
  }

  function load() {
    if (loading) return loading;
    var url = readConfig().manifestUrl;
    if (!url || typeof fetch !== 'function') {
      loading = Promise.resolve();
      return loading;
    }
    // Best effort: without the set every node gets its default, which is what a
    // missing icon looks like anyway — never a reason to fail the graph.
    loading = fetch(url, { credentials: 'same-origin' })
      .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.json(); })
      .then(function (m) { names = (m && m.names) || {}; })
      .catch(function (e) {
        if (typeof console !== 'undefined') console.warn('icon manifest unavailable', e);
      });
    return loading;
  }

  // For tests: set both inputs directly instead of reading the page.
  function use(cfg, set) {
    config = cfg;
    names = set || {};
    loading = Promise.resolve();
  }

  return {
    load: load,
    resolve: resolve,
    stamp: stamp,
    localName: localName,
    defaultFor: defaultFor,
    isDefault: isDefault,
    use: use
  };
});
