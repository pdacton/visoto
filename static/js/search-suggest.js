/*
  Topbar search: open a pasted IRI or CURIE directly, and suggest labels while
  typing.

    Enter on an http(s)/urn IRI       → its resource page, no search
    Enter on a CURIE with a declared  → expanded through VisotoPrefixes, then
    prefix (schema:Person)              its resource page
    anything else                     → /search, as before

  CURIE search-ahead: while the prefix is being typed ("dc"), the declared
  prefixes that start with it are offered and complete the field; once the
  colon is there ("dcat:", "dcat:Data"), the endpoint's classes in that
  namespace are listed, most instances first. The classes come from
  /api/class-tree (cached server-side), fetched once per page and filtered
  locally, so CURIE typing never waits on the endpoint. Classes only:
  properties and instances have no cheap list to match IRIs against.

  Suggestions come from GET /api/search (VisotoApi.url adds endpoint and lang).
  Search is slow on endpoints without a full-text index (a CONTAINS scan), so
  nothing here ever makes Enter wait: Enter with no highlighted row goes
  straight on, a reply that arrives after the input changed is dropped, and a
  failed request just closes the list.

  Hits sharing label and type collapse into one row (I14Y publishes every
  version of a concept as its own resource: "Bern" came back as three rows of
  the same thing). The row opens the shortest IRI — the unversioned one, where
  there is one — and says how many it stands for.

  The pure parts (classify, collapse, shortIri) are exported for
  tests/js/search-suggest.test.js; attach() wires a form in the page. Strings
  arrive as data-str-* attributes on the form, translated server-side.
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.VisotoSearchSuggest = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var MIN_CHARS = 2;
  var DEBOUNCE_MS = 300;
  var SLOW_MS = 1500;
  var LIMIT = 8;

  // classify says what the typed text is. prefixes is a VisotoPrefixes
  // instance (or anything with expand()); a CURIE counts only when its prefix
  // is declared, so "Note: x" or "10:30" stay text.
  function classify(text, prefixes) {
    var s = String(text || '').trim();
    if (s.charAt(0) === '<' && s.charAt(s.length - 1) === '>') s = s.slice(1, -1).trim();
    if (!s || /\s/.test(s)) return { kind: 'text' };
    if (/^https?:\/\/[^/?#]+/i.test(s) || /^urn:[a-z0-9][a-z0-9-]*:\S+$/i.test(s)) {
      return { kind: 'iri', iri: s };
    }
    var m = /^([A-Za-z][\w.-]*):(\S+)$/.exec(s);
    if (m && prefixes && typeof prefixes.expand === 'function') {
      var iri = prefixes.expand(s);
      if (iri !== s) return { kind: 'curie', iri: iri };
    }
    return { kind: 'text' };
  }

  // collapse merges versions of one resource: hits with the same label and
  // type whose IRIs extend a shorter one by a path (…/351 and
  // …/351/version/2020.0.0). Same label and type alone is not enough — two
  // different concepts can both be called "Bern". A row opens its shortest
  // IRI; extra counts the versions folded into it. Rows keep the order in which
  // their first hit arrived.
  function collapse(hits) {
    var groups = {};
    var order = [];
    (Array.isArray(hits) ? hits : []).forEach(function (h, i) {
      if (!h || typeof h.iri !== 'string' || !h.iri) return;
      var label = h.label || h.iri;
      var type = h.type || '';
      var key = label + '\u0000' + type;
      if (!groups[key]) { groups[key] = []; order.push(key); }
      groups[key].push({ iri: h.iri, label: label, type: type, at: i });
    });
    var rows = [];
    order.forEach(function (key) {
      var members = groups[key].slice().sort(function (a, b) { return a.iri.length - b.iri.length; });
      var stems = [];
      members.forEach(function (m) {
        var stem = null;
        for (var j = 0; j < stems.length; j++) {
          var s = stems[j].iri.replace(/\/+$/, '');
          if (m.iri.indexOf(s + '/') === 0 || m.iri.indexOf(s + '#') === 0) { stem = stems[j]; break; }
        }
        if (stem) {
          stem.extra++;
          stem.at = Math.min(stem.at, m.at);
        } else {
          stems.push({ iri: m.iri, label: m.label, type: m.type, extra: 0, at: m.at });
        }
      });
      rows = rows.concat(stems);
    });
    rows.sort(function (a, b) { return a.at - b.at; });
    return rows.map(function (r) { return { iri: r.iri, label: r.label, type: r.type, extra: r.extra }; });
  }

  // shortIri is the IRI as a CURIE when a prefix covers it, else without its
  // scheme. Display only.
  function shortIri(iri, prefixes) {
    var c = prefixes && typeof prefixes.compact === 'function' ? prefixes.compact(iri) : null;
    if (c && c.local) return c.prefix + ':' + c.local;
    return String(iri).replace(/^https?:\/\//i, '');
  }

  // curieParts splits "dcat:" or "dcat:Data" when dcat is a declared prefix:
  // { name, ns, local }. null otherwise (undeclared prefix, spaces, full IRI).
  function curieParts(text, prefixes) {
    var m = /^([A-Za-z][\w.-]*):([^\s/][^\s]*|)$/.exec(String(text || '').trim());
    if (!m || !prefixes || typeof prefixes.expand !== 'function') return null;
    var ns = prefixes.expand(m[1] + ':');
    if (ns === m[1] + ':') return null;
    return { name: m[1], ns: ns, local: m[2] };
  }

  // prefixMatches lists declared prefixes whose name starts with text (no
  // colon typed yet), shortest first: "dc" offers dc:, dcat:, dcterms:.
  function prefixMatches(text, prefixes, limit) {
    var s = String(text || '').trim().toLowerCase();
    if (!s || !/^[a-z][\w.-]*$/.test(s) || !prefixes || !Array.isArray(prefixes.list)) return [];
    var seen = {};
    return prefixes.list
      .filter(function (p) {
        if (seen[p.name] || p.name.toLowerCase().indexOf(s) !== 0) return false;
        seen[p.name] = true;
        return true;
      })
      .sort(function (a, b) { return a.name.length - b.name.length || (a.name < b.name ? -1 : 1); })
      .slice(0, limit || 4)
      .map(function (p) { return { name: p.name, iri: p.iri }; });
  }

  // classesFromTree reads /api/class-tree (SPARQL JSON: ?class ?label ?parent
  // and, once a daily snapshot exists, ?instcount) into one entry per class.
  function classesFromTree(json) {
    var rows = json && json.results && Array.isArray(json.results.bindings) ? json.results.bindings : [];
    var byIri = {};
    var out = [];
    rows.forEach(function (b) {
      var iri = b && b['class'] && b['class'].value;
      if (!iri) return;
      var label = b.label ? b.label.value : '';
      var n = b.instcount ? parseInt(b.instcount.value, 10) : NaN;
      var k = byIri[iri];
      if (!k) {
        k = byIri[iri] = { iri: iri, label: label, count: isNaN(n) ? null : n };
        out.push(k);
        return;
      }
      if (!k.label && label) k.label = label;
      if (!isNaN(n) && (k.count === null || n > k.count)) k.count = n;
    });
    return out;
  }

  // matchClasses keeps the classes in namespace ns whose local name starts
  // with local (case-insensitive), most instances first, then by name.
  function matchClasses(classes, ns, local, limit) {
    var want = String(local || '').toLowerCase();
    return (classes || [])
      .filter(function (k) {
        return k.iri.indexOf(ns) === 0 && k.iri.length > ns.length &&
          k.iri.slice(ns.length).toLowerCase().indexOf(want) === 0;
      })
      .sort(function (a, b) {
        var ca = a.count === null ? -1 : a.count;
        var cb = b.count === null ? -1 : b.count;
        if (ca !== cb) return cb - ca;
        return a.iri < b.iri ? -1 : a.iri > b.iri ? 1 : 0;
      })
      .slice(0, limit || LIMIT);
  }

  function fill(template, value) {
    return String(template || '').replace('%s', value);
  }

  // attach wires the topbar form. env: { prefixes, apiUrl(path, params),
  // resourceHref(iri), fetch, navigate(href) } — the browser globals by default.
  function attach(form, input, env) {
    env = env || {};
    var doc = form.ownerDocument;
    var win = doc.defaultView;
    var prefixes = env.prefixes || (win.VisotoPrefixes ? win.VisotoPrefixes.fromPage() : null);
    var apiUrl = env.apiUrl || function (p, q) { return win.VisotoApi.url(p, q); };
    var hrefFor = env.resourceHref || function (iri) { return win.visotoResourceHref(iri); };
    var doFetch = env.fetch || win.fetch.bind(win);
    var navigate = env.navigate || function (href) { win.location.href = href; };
    var str = function (name) { return form.getAttribute('data-str-' + name) || ''; };

    var listId = input.id + '-suggestions';
    var list = doc.createElement('div');
    list.id = listId;
    list.className = 'dropdown-menu vs-search-suggest';
    list.setAttribute('role', 'listbox');
    list.setAttribute('aria-label', str('list'));
    input.insertAdjacentElement('afterend', list);

    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-autocomplete', 'list');
    input.setAttribute('aria-controls', listId);
    input.setAttribute('aria-expanded', 'false');
    input.setAttribute('autocomplete', 'off');
    input.setAttribute('spellcheck', 'false'); // IRIs and CURIEs are not words

    var options = [];   // the selectable <a> rows, in order
    var active = -1;
    var seq = 0;        // bumps on every input change; stale replies compare against it
    var debounce = null;
    var slow = null;
    var controller = null;

    function cancelPending() {
      clearTimeout(debounce);
      clearTimeout(slow);
      if (controller) controller.abort();
      controller = null;
    }

    function close() {
      list.classList.remove('show');
      list.replaceChildren();
      options = [];
      setActive(-1);
      input.setAttribute('aria-expanded', 'false');
    }

    function open() {
      list.classList.add('show');
      input.setAttribute('aria-expanded', 'true');
    }

    function setActive(i) {
      options.forEach(function (o, j) {
        o.classList.toggle('active', j === i);
        o.setAttribute('aria-selected', j === i ? 'true' : 'false');
      });
      active = i;
      if (i >= 0 && options[i]) {
        input.setAttribute('aria-activedescendant', options[i].id);
        options[i].scrollIntoView({ block: 'nearest' });
      } else {
        input.removeAttribute('aria-activedescendant');
      }
    }

    function span(cls, text) {
      var el = doc.createElement('span');
      el.className = cls;
      el.textContent = text;
      return el;
    }

    function option(href, label, detail, aside) {
      var a = doc.createElement('a');
      a.className = 'dropdown-item vs-suggest-item';
      a.href = href;
      a.id = listId + '-' + options.length;
      a.setAttribute('role', 'option');
      a.setAttribute('aria-selected', 'false');
      var main = span('vs-suggest-main', '');
      main.appendChild(span('vs-suggest-label', label));
      if (detail) main.appendChild(span('vs-suggest-iri', detail));
      a.appendChild(main);
      if (aside) a.appendChild(span('vs-suggest-type', aside));
      options.push(a);
      return a;
    }

    function note(text) {
      var el = span('dropdown-item-text vs-suggest-note', text);
      el.setAttribute('role', 'status');
      return el;
    }

    function searchHref(q) {
      var params = new URLSearchParams(new FormData(form));
      params.set(input.name, q);
      return form.getAttribute('action') + '?' + params.toString();
    }

    function render(nodes) {
      list.replaceChildren.apply(list, nodes);
      setActive(-1);
      if (nodes.length) open(); else close();
    }

    function showResource(c) {
      options = [];
      render([option(hrefFor(c.iri), str('open'), c.iri, '↵')]);
      setActive(0);
    }

    // The endpoint's classes, fetched once per page on the first CURIE and
    // filtered locally after that. A 404 (no class statistics for this
    // endpoint) or any failure leaves an empty list: CURIEs still open.
    var classes = null;
    var classesPromise = null;
    function loadClasses() {
      if (!classesPromise) {
        classesPromise = doFetch(apiUrl('/api/class-tree', { src: null }))
          .then(function (r) { return r.ok ? r.json() : null; })
          .catch(function () { return null; })
          .then(function (json) { classes = classesFromTree(json); return classes; });
      }
      return classesPromise;
    }

    // A prefix row completes the field instead of navigating.
    function prefixOption(p) {
      var a = option('#', p.name + ':', p.iri, '');
      a.setAttribute('data-fill', p.name + ':');
      return a;
    }

    function count(n) {
      if (n === null || n === undefined) return '';
      var lang = doc.documentElement.getAttribute('lang') || undefined;
      var s;
      try { s = n.toLocaleString(lang); } catch (e) { s = String(n); }
      return fill(str('instances'), s);
    }

    // "dcat:" lists dcat's classes; "dcat:Data" narrows them and also offers
    // to open dcat:Data itself.
    // The "open it as typed" row comes last when classes match: "schema:Pers"
    // is far more likely heading for schema:Person than to be a term itself.
    function showCurie(cp) {
      options = [];
      var nodes = [];
      var exact = cp.local ? cp.ns + cp.local : '';
      matchClasses(classes, cp.ns, cp.local, LIMIT).forEach(function (k) {
        var local = k.iri.slice(cp.ns.length);
        var label = k.label && k.label.toLowerCase() !== local.toLowerCase() ? k.label : '';
        nodes.push(option(hrefFor(k.iri), cp.name + ':' + local, label, count(k.count)));
        if (k.iri === exact) exact = '';
      });
      if (exact) nodes.push(option(hrefFor(exact), str('open'), cp.name + ':' + cp.local, '↵'));
      render(nodes);
    }

    function useFill(el) {
      input.value = el.getAttribute('data-fill');
      input.focus();
      input.dispatchEvent(new Event('input', { bubbles: true }));
    }

    var prefixRows = [];

    function versions(n) {
      if (!n) return '';
      return n === 1 ? str('version-one') : fill(str('version-other'), String(n));
    }

    function showPrefixes() {
      options = [];
      render(prefixRows.map(prefixOption));
    }

    function showHits(q, hits) {
      options = [];
      var head = prefixRows.map(prefixOption);
      var rows = collapse(hits);
      if (!rows.length) {
        render(head.concat([note(str('empty'))]));
        return;
      }
      var nodes = head.concat(rows.map(function (r) {
        var aside = [r.type, versions(r.extra)].filter(Boolean).join(' · ');
        return option(hrefFor(r.iri), r.label, shortIri(r.iri, prefixes), aside);
      }));
      var all = option(searchHref(q), fill(str('all'), q), '', '');
      all.classList.add('vs-suggest-all');
      var divider = doc.createElement('div');
      divider.className = 'dropdown-divider';
      nodes.push(divider, all);
      render(nodes);
    }

    function suggest(q, mySeq) {
      controller = typeof AbortController === 'function' ? new AbortController() : null;
      slow = setTimeout(function () {
        if (mySeq === seq) { options = []; render(prefixRows.map(prefixOption).concat([note(str('searching'))])); }
      }, SLOW_MS);
      var url = apiUrl('/api/search', { q: q, limit: LIMIT, src: null });
      doFetch(url, controller ? { signal: controller.signal } : undefined)
        .then(function (r) { if (!r.ok) throw new Error('HTTP ' + r.status); return r.json(); })
        .then(function (hits) {
          clearTimeout(slow);
          if (mySeq === seq) showHits(q, hits);
        })
        .catch(function () {
          clearTimeout(slow);
          if (mySeq !== seq) return;
          if (prefixRows.length) showPrefixes(); else close();
        });
    }

    input.addEventListener('input', function () {
      seq++;
      cancelPending();
      prefixRows = [];
      var q = input.value.trim();
      var mySeq = seq;
      var cp = curieParts(q, prefixes);
      if (cp) {
        showCurie(cp);
        if (!classes) loadClasses().then(function () { if (mySeq === seq) showCurie(cp); });
        return;
      }
      var c = classify(q, prefixes);
      if (c.kind !== 'text') { showResource(c); return; }
      if (q.length < MIN_CHARS) { close(); return; }
      prefixRows = prefixMatches(q, prefixes, 4);
      if (prefixRows.length) showPrefixes();
      debounce = setTimeout(function () { suggest(q, mySeq); }, DEBOUNCE_MS);
    });

    input.addEventListener('keydown', function (e) {
      var isOpen = list.classList.contains('show');
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        if (!options.length) return;
        e.preventDefault();
        // -1 is "the field itself": stepping past either end lands back there.
        var next = active + (e.key === 'ArrowDown' ? 1 : -1);
        if (next >= options.length) next = -1;
        else if (next < -1) next = options.length - 1;
        setActive(next);
        return;
      }
      if (e.key === 'Escape') {
        if (isOpen) { e.preventDefault(); seq++; cancelPending(); close(); }
        else input.blur();
        return;
      }
      if (e.key === 'Tab') { close(); return; }
      if (e.key !== 'Enter' || e.isComposing) return;
      e.preventDefault();
      if (active >= 0 && options[active]) {
        if (options[active].hasAttribute('data-fill')) useFill(options[active]);
        else navigate(options[active].getAttribute('href'));
        return;
      }
      var q = input.value.trim();
      if (!q) return;
      var c = classify(q, prefixes);
      seq++;
      cancelPending();
      if (c.kind !== 'text') navigate(hrefFor(c.iri));
      else form.submit();
    });

    // Keep focus in the field while a row is clicked; the link itself navigates.
    list.addEventListener('mousedown', function (e) { e.preventDefault(); });
    list.addEventListener('click', function (e) {
      var row = e.target.closest('[data-fill]');
      if (!row) return;
      e.preventDefault();
      useFill(row);
    });
    input.addEventListener('blur', function () {
      setTimeout(function () {
        if (doc.activeElement !== input) { seq++; cancelPending(); close(); }
      }, 100);
    });
    return { close: close };
  }

  return {
    classify: classify, collapse: collapse, shortIri: shortIri,
    curieParts: curieParts, prefixMatches: prefixMatches,
    classesFromTree: classesFromTree, matchClasses: matchClasses,
    attach: attach,
  };
});
