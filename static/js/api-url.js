/*
  The one place an /api/* request learns the three values that make its
  response a pure function of its URL (the routes sit in the shared Caddy/Souin
  cache and read no cookie — see resolveEndpoint / resolveQueryLang in
  app/server.go):

    endpoint   the active endpoint slug        (endpoint-switcher.js)
    lang       the active UI language          (language-switcher.js)
    src        the template set an async query
               id is scoped to                 (template-set.js)

  Two transports, one rule:

    HTMX requests    every hx-get to /api/* gets all three from the single
                     htmx:configRequest hook installed below. There is NO path
                     allowlist: a new fragment route is covered without touching
                     this file. (The three per-file allowlists this replaces
                     drifted — /api/cube-table was in none of them and silently
                     queried the default endpoint in the default language.)
    plain fetch()es  build the URL with VisotoApi.url(path, params).

  A value the caller sets always wins. In url(), `undefined` means "not set, use
  the default"; `null` means "send no such param" (so a route whose answer does
  not depend on the template set passes src: null rather than splitting its
  cache entry per page); '' is a real value and is sent — it is the configured
  "no language" code, which is different from an absent lang.

  The same goes for links to a resource page: visotoResourceHref(iri) is the one
  JS builder of /resource?iri=… (sparql.ResourceHref is the Go one), so the URL
  scheme and its escaping live in one place per language.

  This is the only file that may listen for htmx:configRequest, and the only
  one that may assemble a /resource URL (app/conventions_test.go).

  Loaded in <head>, before any script that builds a URL. The three readers are
  resolved at call time, not load time, so it does not matter that the files
  defining them load later.
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
    return;
  }
  var page = api.create(root);
  page.create = api.create;
  root.VisotoApi = page;
  root.visotoResourceHref = page.resourceHref;
  if (root.document) page.install(root.document);
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  // env supplies the three readers (the window, in a browser). A missing reader
  // simply contributes no default.
  function create(env) {
    env = env || {};

    function defaults() {
      var out = {};
      var slug = typeof env.activeEndpointSlug === 'function' ? env.activeEndpointSlug() : '';
      if (slug) out.endpoint = slug;
      // `!= null`, not truthiness: '' is the real "no language" code.
      var code = typeof env.activeSiteLang === 'function' ? env.activeSiteLang() : null;
      if (code !== null && code !== undefined) out.lang = code;
      var set = typeof env.activeTemplateSet === 'function' ? env.activeTemplateSet() : '';
      if (set) out.src = set;
      return out;
    }

    function isApiPath(path) {
      return typeof path === 'string' && path.indexOf('/api/') === 0;
    }

    // The param names already present in path's own query string.
    function namesIn(path) {
      var names = {};
      var q = path.indexOf('?');
      if (q === -1) return names;
      path.slice(q + 1).split('#')[0].split('&').forEach(function (pair) {
        if (!pair) return;
        var name = pair.split('=')[0];
        try { name = decodeURIComponent(name.replace(/\+/g, ' ')); } catch (e) { /* keep raw */ }
        names[name] = true;
      });
      return names;
    }

    // apply fills the defaults into an htmx parameter bag (or any plain object)
    // for a request to path, leaving alone what the element or the path's own
    // query string already says.
    function apply(path, params) {
      var d = defaults();
      var taken = namesIn(path);
      Object.keys(d).forEach(function (name) {
        if (taken[name] || params[name] !== undefined) return;
        params[name] = d[name];
      });
      return params;
    }

    // url builds an /api URL: params plus whichever defaults they leave unset.
    // Arrays become repeated params. path may already carry a query string.
    function url(path, params) {
      var all = {};
      Object.keys(params || {}).forEach(function (name) { all[name] = params[name]; });
      apply(path, all);
      var parts = [];
      Object.keys(all).forEach(function (name) {
        var value = all[name];
        if (value === undefined || value === null) return;
        (Array.isArray(value) ? value : [value]).forEach(function (item) {
          parts.push(encodeURIComponent(name) + '=' + encodeURIComponent(item));
        });
      });
      if (!parts.length) return path;
      var last = path.charAt(path.length - 1);
      var sep = path.indexOf('?') === -1 ? '?' : (last === '?' || last === '&' ? '' : '&');
      return path + sep + parts.join('&');
    }

    // resourceHref builds the resource page URL for iri, carrying the endpoint
    // slug: the active one, or `slug` when the link must point at a specific
    // store. Use it for every JS-built link and for JS-side navigation;
    // server-rendered anchors are bare by design (the page is cached per URL,
    // not per visitor) and get the slug from endpoint-switcher.js's click
    // rewriter.
    function resourceHref(iri, slug) {
      var href = '/resource?iri=' + encodeURIComponent(iri);
      if (slug === undefined) {
        slug = typeof env.activeEndpointSlug === 'function' ? env.activeEndpointSlug() : '';
      }
      if (slug) href += '&endpoint=' + encodeURIComponent(slug);
      return href;
    }

    // install registers the hook on document, so it also covers the first
    // load-triggered hx-get on the page. GET only: the fragment routes are all
    // GETs, and a body-carrying request is not a function of its URL anyway.
    function install(doc) {
      doc.addEventListener('htmx:configRequest', function (evt) {
        var detail = evt.detail || {};
        if (!isApiPath(detail.path)) return;
        if (detail.verb && String(detail.verb).toLowerCase() !== 'get') return;
        apply(detail.path, detail.parameters);
      });
    }

    return { defaults: defaults, isApiPath: isApiPath, apply: apply, url: url, resourceHref: resourceHref, install: install };
  }

  return { create: create };
});
