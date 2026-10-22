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
                                       subject = { iri, types, override }
    VisotoIcons.stamp(data, iri)       sets data.image to iri's own icon unless
                                       data already carries an image
    VisotoIcons.properties()           the "icon" magic property as full IRIs,
                                       in priority order ([] when unset)
    VisotoIcons.pick(candidates)       [{ property, value }] -> path or ''
    VisotoIcons.fetchOverrides(iris, select)
                                       Promise of { iri: path } for the
                                       resources that name their own icon
    VisotoIcons.defaultFor(kind)       'page' | 'class' | 'instance' -> path
    VisotoIcons.isDefault(src)         whether src is one of those defaults

  Keep resolve() and override() in step with Resolver.Resolve / override in
  internal/icon/icon.go:
    - a usable own icon from the data (the "icon" magic property) wins outright
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

  // Mirror of Resolver.override: an icon value from the data, as a path, or ''
  // when it is not usable. https URLs, site-relative paths and names in the set
  // only — never another scheme, a protocol-relative URL or plain http (the
  // site CSP would block it).
  function override(value) {
    var base = readConfig().base;
    value = String(value == null ? '' : value).trim();
    if (!value) return '';
    if (value.indexOf('https://') === 0) {
      try { return new URL(value).host ? value : ''; } catch (e) { return ''; }
    }
    if (value.indexOf('//') === 0) return '';
    if (value.charAt(0) === '/') return value;
    // names keys fallback icons as "<name>.fallback", so one lookup serves both.
    var name = value.replace(/\.svg$/, '');
    return names[name] ? base + name + '.svg' : '';
  }

  // Mirror of Resolver.Resolve. names holds bare names for real icons and
  // "<name>.fallback" for the weaker ones.
  function resolve(subject, kind) {
    var base = readConfig().base;
    var iri = subject && subject.iri;
    var types = (subject && subject.types) || [];

    var own = override(subject && subject.override);
    if (own) return own;

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

  // The "icon" magic property's alternatives as full IRIs, in priority order.
  // CURIEs expand through the active endpoint's prefixes (prefixes.js). Like the
  // server (simpleTerm in internal/sparql/labels.go), only a single predicate is
  // usable — a path such as a/b is skipped — and so is a CURIE whose prefix is
  // not declared.
  var SIMPLE_TERM = /^(?:[A-Za-z][\w.-]*:[\w.-]*|<[^<>"{}|^`\\\s]+>)$/;
  function properties() {
    var list = readConfig().properties || [];
    var prefixes = typeof window !== 'undefined' && window.VisotoPrefixes ? window.VisotoPrefixes.fromPage() : null;
    var out = [];
    list.forEach(function (p) {
      var iri = '';
      if (SIMPLE_TERM.test(p)) iri = p.charAt(0) === '<' ? p.slice(1, -1) : (prefixes ? prefixes.expand(p) : p);
      if (/^[a-z][a-z0-9+.-]*:\/\//i.test(iri)) out.push(iri);
      else if (typeof console !== 'undefined') console.warn('icon property ' + p + ' is not a single predicate with a declared prefix; ignored');
    });
    return out;
  }

  // Mirror of the server's best-value pick (internal/sparql/labels.go) for the
  // icon: the earliest property wins, the smallest value breaks a tie. The pick
  // is made BEFORE validation, as on the server, so both agree on which value a
  // resource names — an unusable one then falls back to the usual rule.
  // (The server ranks language before property; icon values are IRIs or plain
  // literals, where that makes no difference.)
  function pick(candidates) {
    var props = properties();
    var best = null, bestRank = Infinity;
    (candidates || []).forEach(function (c) {
      var rank = props.indexOf(c.property);
      if (rank < 0 || c.value === '' || c.value == null) return;
      if (rank < bestRank || (rank === bestRank && String(c.value) < String(best))) {
        best = String(c.value); bestRank = rank;
      }
    });
    return best === null ? '' : override(best);
  }

  // One SELECT for the own icons of iris. select(query) must resolve to the
  // SPARQL JSON bindings array. Best effort: a failure means no overrides.
  function fetchOverrides(iris, select) {
    var props = properties();
    iris = (iris || []).filter(function (iri) { return /^[a-z][a-z0-9+.-]*:\/\//i.test(iri) && !/[\s<>"]/.test(iri); });
    if (!props.length || !iris.length) return Promise.resolve({});
    var query =
      'SELECT ?inst ?prop ?icon WHERE {\n' +
      '  VALUES ?inst { ' + iris.map(function (i) { return '<' + i + '>'; }).join(' ') + ' }\n' +
      '  VALUES ?prop { ' + props.map(function (p) { return '<' + p + '>'; }).join(' ') + ' }\n' +
      '  ?inst ?prop ?icon .\n' +
      '}';
    return Promise.resolve()
      .then(function () { return select(query); })
      .then(function (bindings) {
        var byInst = {};
        (bindings || []).forEach(function (b) {
          if (!b.inst || !b.prop || !b.icon || b.icon.type === 'bnode') return;
          (byInst[b.inst.value] = byInst[b.inst.value] || []).push({ property: b.prop.value, value: b.icon.value });
        });
        var out = {};
        Object.keys(byInst).forEach(function (iri) {
          var path = pick(byInst[iri]);
          if (path) out[iri] = path;
        });
        return out;
      })
      .catch(function (e) {
        if (typeof console !== 'undefined') console.warn('icon override lookup failed', e);
        return {};
      });
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
    override: override,
    stamp: stamp,
    properties: properties,
    pick: pick,
    fetchOverrides: fetchOverrides,
    localName: localName,
    defaultFor: defaultFor,
    isDefault: isDefault,
    use: use
  };
});
