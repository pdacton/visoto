/* eslint-disable */
/*
  Behaviour for the "sparqlLazyTree" partial (templates/partials/sparql-lazy-tree.html).

  The eager sibling (sparql-tree.js) reads a JSON island holding the whole
  hierarchy and builds the tree from it. This one starts empty and asks
  /api/lazy-tree/<id>/<role> for one level at a time, so the page carries no node
  payload and the cost of a hierarchy is what the user actually opens.

  The render callback, the treegrid column builder and the expand/collapse
  handlers are lifted from sparql-tree.js: the server shapes each node the same
  way (title, key, extra vars as top-level {value,label,type} objects), so the two
  trees present identically.

  Two behaviours exist because the tree RE-MOUNTS on every navigation — a node
  title is a real link, so clicking one loads a page:
    - focus is restored in ONE request (/focus returns the ancestor chain and every
      level along it), not a recursive parents walk plus a fetch per level;
    - expanded branches are restored from sessionStorage, since otherwise every
      navigation collapses the tree back to the focused path.

  Attributes read from the root element:
    data-sparql-lazy-tree      marker; presence means "initialize me"
    data-sparql-lazy-tree-id   DOM id prefix for this tree's elements and islands
*/
(function () {
  'use strict';

  var STORAGE_PREFIX = 'vs-lazytree:';
  var MAX_STORED_EXPANDED = 300;   // cap: an aggressive expander must not blow the quota
  var SEARCH_DEBOUNCE_MS = 300;
  var MAX_REVEALED = 8;            // hierarchical hits revealed in place; see revealHits
  var SPINNER_DELAY_MS = 150;      // don't flash a spinner for a fast level

  // vsT/vsTf come from static/js/i18n.js, which base.html loads first. Called
  // directly rather than through a local wrapper so the i18n key-consistency test
  // (which scans for literal vsT( calls) can see every key this file uses.

  function readConfig(id) {
    var el = document.getElementById(id + '-config');
    if (!el) return null;
    try {
      return JSON.parse(el.innerHTML.trim());
    } catch (e) {
      console.error('sparqlLazyTree: bad config island for ' + id, e);
      return null;
    }
  }

  function initLazyTree(root) {
    var ID = root.getAttribute('data-sparql-lazy-tree-id');
    if (!ID) return;
    var cfg = readConfig(ID);
    if (!cfg || !cfg.treeId) return;

    var treeElem = document.getElementById(ID + '-tree');
    var statusElem = document.getElementById('status-' + ID);
    var searchInput = document.getElementById('search-' + ID);
    var breadcrumbBox = document.getElementById('breadcrumb-' + ID);
    var flatToggleBox = document.getElementById('flat-toggle-' + ID);
    var flatToggle = document.getElementById('flat-' + ID);
    if (!treeElem) return;

    var minSearch = cfg.minSearchLength > 0 ? cfg.minSearchLength : 3;
    var restoreState = cfg.restoreState !== false;   // default on

    // --- Request plumbing ---------------------------------------------------
    // The template set scopes the id, exactly as the async-table routes require;
    // the endpoint slug and language ride along so the response is a pure
    // function of its URL and the shared cache can hold it.
    function templateSet() {
      var meta = document.querySelector('meta[name="vs-template-set"]');
      return meta ? meta.getAttribute('content') : '';
    }

    function roleURL(role, params) {
      var u = new URL('/api/lazy-tree/' + encodeURIComponent(cfg.treeId) + '/' + role, window.location.origin);
      u.searchParams.set('src', templateSet());
      if (cfg.iri) u.searchParams.set('iri', cfg.iri);
      var slug = (typeof activeEndpointSlug === 'function') ? activeEndpointSlug() : '';
      if (slug) u.searchParams.set('endpoint', slug);
      var lang = document.documentElement.getAttribute('lang');
      if (lang) u.searchParams.set('lang', lang);
      if (cfg.limit) u.searchParams.set('limit', cfg.limit);
      for (var k in (params || {})) {
        var v = params[k];
        if (Array.isArray(v)) v.forEach(function (item) { u.searchParams.append(k, item); });
        else if (v !== undefined && v !== null && v !== '') u.searchParams.set(k, v);
      }
      return u.toString();
    }

    function fetchRole(role, params) {
      return fetch(roleURL(role, params), { headers: { 'Accept': 'application/json' } })
        .then(function (r) { return r.json(); });
    }

    // --- Status line --------------------------------------------------------
    function setStatus(html, kind) {
      if (!statusElem) return;
      if (!html) { statusElem.hidden = true; statusElem.innerHTML = ''; return; }
      statusElem.hidden = false;
      statusElem.className = 'px-3 py-2 small border-bottom ' +
        (kind === 'error' ? 'text-danger' : 'text-muted');
      statusElem.innerHTML = html;
    }

    // --- Expansion-state persistence ---------------------------------------
    // Every navigation re-mounts the tree, so without this the user loses every
    // open branch but the focused path on each click.
    function storageKey() {
      var slug = (typeof activeEndpointSlug === 'function') ? activeEndpointSlug() : '';
      return STORAGE_PREFIX + cfg.treeId + ':' + slug + ':' + (cfg.iri || '');
    }
    function readExpanded() {
      if (!restoreState) return [];
      try {
        var raw = sessionStorage.getItem(storageKey());
        return raw ? (JSON.parse(raw) || []) : [];
      } catch (e) { return []; }   // private mode, blocked storage, corrupt value
    }
    function writeExpanded(keys) {
      if (!restoreState) return;
      try {
        sessionStorage.setItem(storageKey(), JSON.stringify(keys.slice(0, MAX_STORED_EXPANDED)));
      } catch (e) { /* quota or blocked storage: state restore is a convenience */ }
    }
    // Programmatic expansions — restoring state, revealing a search hit, walking to
    // the focus target — must NOT be recorded. Only what the user opened is worth
    // restoring: otherwise one search that reveals 8 hits writes their whole
    // ancestry into storage, and the next page load replays it as ~45 level
    // requests, which is precisely the round-trip storm this file exists to avoid.
    var suppressRemember = 0;
    function withoutRemembering(fn) {
      suppressRemember++;
      return Promise.resolve()
        .then(fn)
        .finally(function () { suppressRemember--; });
    }
    function rememberExpansion() {
      if (!restoreState || !tree || suppressRemember > 0) return;
      var keys = [];
      tree.visit(function (n) { if (n.isExpanded && n.isExpanded()) keys.push(n.key); });
      writeExpanded(keys);
    }

    // --- Node conversion ----------------------------------------------------
    // The server already emits Wunderbaum's source shape; this only marks a node
    // that reported no children so the expander is dropped.
    function toSource(nodes) {
      return (nodes || []).map(function (n) { return n; });
    }

    // --- Level loading ------------------------------------------------------
    var levelCache = {};   // parent key ("" = roots) → envelope, seeded by /focus

    function loadLevel(parentKey) {
      if (levelCache[parentKey] !== undefined) {
        var cached = levelCache[parentKey];
        delete levelCache[parentKey];       // one-shot: a later expand refetches
        return Promise.resolve(cached);
      }
      var role = parentKey === '' ? 'roots' : 'children';
      var params = parentKey === '' ? {} : { parent: parentKey };
      return fetchRole(role, params);
    }

    function applyEnvelope(node, env) {
      if (env.error) {
        // Per-node error: the level failed, the rest of the tree is unaffected.
        if (node) {
          node.setStatus('error', env.error);
        } else {
          setStatus(escapeHTML(env.error), 'error');
        }
        return [];
      }
      if (!env.complete) {
        var msg = env.limitMode === 'page'
          ? vsTf('js.lazyTree.loadMore', 'Showing the first {n} — expand further to load more.', { n: env.nodes.length })
          : vsTf('js.lazyTree.showingNofM', 'Showing {n} of many.', { n: env.nodes.length });
        setStatus(escapeHTML(msg));
      }
      return toSource(env.nodes);
    }

    function escapeHTML(s) {
      var d = document.createElement('div');
      d.textContent = String(s == null ? '' : s);
      return d.innerHTML;
    }

    // --- Columns ------------------------------------------------------------
    // Built from the first level's extra variables, the same way sparql-tree.js
    // derives them from the query's vars.
    function columnsFor(nodes) {
      var reserved = { key: 1, title: 1, lazy: 1, icon: 1, children: 1, expanded: 1 };
      var names = [];
      (nodes || []).forEach(function (n) {
        for (var k in n) {
          if (reserved[k] || names.indexOf(k) >= 0) continue;
          if (n[k] && typeof n[k] === 'object' && 'value' in n[k]) names.push(k);
        }
      });
      if (!names.length) return null;
      var cols = [{ id: '*', title: vsT('js.lazyTree.nameColumn', 'NAME'), width: '*' }];
      names.forEach(function (name) {
        cols.push({ id: name, title: name.toUpperCase(), width: '150px' });
      });
      return cols;
    }

    // --- Boot ---------------------------------------------------------------
    var tree = null;
    var spinnerTimer = setTimeout(function () {
      treeElem.classList.add('wb-skeleton');
    }, SPINNER_DELAY_MS);

    // Focus (with the ancestor chain) and a plain roots load are the same startup
    // in one request: /focus returns the roots too, so a page that lands on a
    // node never pays a separate round trip for them.
    var startup = cfg.resourceIRI
      ? fetchRole('focus', { node: cfg.resourceIRI }).catch(function () { return null; })
      : Promise.resolve(null);

    startup.then(function (focus) {
      var rootsEnv = null;
      var path = [];

      if (focus && !focus.error && focus.levels) {
        path = focus.path || [];
        indexLabels(focus.levels);
        for (var key in focus.levels) levelCache[key] = focus.levels[key];
        rootsEnv = levelCache[''];
        delete levelCache[''];
      }
      if (!rootsEnv) return fetchRole('roots', {}).then(function (env) { return { env: env, path: path }; });
      return { env: rootsEnv, path: path };
    }).then(function (res) {
      clearTimeout(spinnerTimer);
      treeElem.classList.remove('wb-skeleton', 'wb-initializing');
      buildTree(res.env, res.path);
    }).catch(function (err) {
      clearTimeout(spinnerTimer);
      treeElem.classList.remove('wb-skeleton', 'wb-initializing');
      setStatus(escapeHTML(vsT('js.lazyTree.loadFailed', 'The hierarchy could not be loaded.')), 'error');
      console.error('sparqlLazyTree: roots failed', err);
    });

    function buildTree(rootsEnv, focusPath) {
      var source = applyEnvelope(null, rootsEnv || { nodes: [] });
      var columns = columnsFor(source);

      tree = new mar10.Wunderbaum({
        element: treeElem,
        source: source,
        columns: columns,
        rowHeightPx: 44,   // must match --wb-header-height in the CSS overrides
        // Lucide icons instead of Wunderbaum's Bootstrap Icons default, which this
        // project does not load — see static/js/wunderbaum-icons.js.
        iconMap: window.visotoWunderbaumIcons,
        filter: { autoApply: true, mode: 'hide' },

        // One level per expansion. Wunderbaum keeps what it loaded, so collapsing
        // and re-expanding does not refetch.
        lazyLoad: function (e) {
          return loadLevel(e.node.key).then(function (env) {
            var kids = applyEnvelope(e.node, env);
            if (!kids.length) {
              // Optimistic expander that found nothing: drop it, so the node reads
              // as the leaf it turned out to be.
              e.node.lazy = false;
            }
            return kids;
          });
        },

        render: function (e) {
          var titleSpan = e.nodeElem.querySelector('.wb-title');
          if (titleSpan) {
            var displayTitle = e.node.titleWithHighlight || e.node.title;
            var href = (typeof visotoResourceHref === 'function')
              ? visotoResourceHref(e.node.key)
              : '/resource?iri=' + encodeURIComponent(e.node.key);
            titleSpan.innerHTML = "<a href='" + href + "'>" + displayTitle + "</a>";
          }
          if (e.renderColInfosById) {
            for (var id in e.renderColInfosById) {
              var col = e.renderColInfosById[id];
              if (col.id === '*') continue;
              var colData = e.node.data ? e.node.data[col.id] : null;
              col.elem.textContent = colData ? (colData.label || colData.value || '') : '';
            }
          }
        },

        // Enter/Space follows the focused node's link rather than selecting: in a
        // navigator the node IS the link.
        keydown: function (e) {
          if ((e.event.key === 'Enter' || e.event.key === ' ') && e.node) {
            var a = e.node.getNodeElem && e.node.getNodeElem();
            var link = a && a.querySelector('a');
            if (link) { e.event.preventDefault(); link.click(); }
          }
        },

        expand: function () { rememberExpansion(); },
        collapse: function () { rememberExpansion(); },
      });

      restoreExpansion(focusPath);
      renderBreadcrumb(focusPath);
    }

    // Re-expand what the user had open before the last navigation, then reveal the
    // focused node. The focus path is expanded first so the target is visible even
    // if the stored set is stale.
    function restoreExpansion(focusPath) {
      var chain = (focusPath || []).slice();
      var stored = readExpanded().filter(function (k) { return chain.indexOf(k) < 0; });
      var toExpand = chain.concat(stored);

      var step = function (i) {
        if (i >= toExpand.length) return Promise.resolve();
        var node = tree.findKey ? tree.findKey(toExpand[i]) : null;
        if (!node || !node.setExpanded) return step(i + 1);
        return node.setExpanded(true).catch(function () {}).then(function () { return step(i + 1); });
      };

      withoutRemembering(function () {
        return step(0).then(function () {
          if (!cfg.resourceIRI) return;
          var target = tree.findKey ? tree.findKey(cfg.resourceIRI) : null;
          if (!target) return;
          return target.makeVisible().then(function () {
            target.setActive();
            if (target.scrollIntoView) target.scrollIntoView();
          });
        });
      }).catch(function (e) { console.warn('sparqlLazyTree: restore failed', e); });
    }

    // Labels for nodes not yet in the tree.
    //
    // The breadcrumb names every ancestor, but at the moment it is drawn the middle
    // levels have only just arrived from /focus and are not yet mounted — so
    // tree.findKey misses them and the crumb would show a raw IRI. Those levels DO
    // carry each node's title, so harvest them as they pass through.
    var labelIndex = {};
    function indexLabels(levels) {
      for (var parentKey in (levels || {})) {
        ((levels[parentKey] || {}).nodes || []).forEach(function (n) {
          if (n && n.key && n.title) labelIndex[n.key] = n.title;
        });
      }
    }
    function labelFor(iri) {
      var node = tree && tree.findKey ? tree.findKey(iri) : null;
      if (node && node.title) return node.title;
      return labelIndex[iri] || iri;
    }

    function renderBreadcrumb(path) {
      if (!breadcrumbBox || !path || path.length < 2) return;
      var ol = breadcrumbBox.querySelector('ol');
      if (!ol) return;
      ol.innerHTML = '';
      path.forEach(function (iri, i) {
        var li = document.createElement('li');
        li.className = 'breadcrumb-item' + (i === path.length - 1 ? ' active' : '');
        var label = labelFor(iri);
        if (i === path.length - 1) {
          li.textContent = label;
        } else {
          var a = document.createElement('a');
          a.href = (typeof visotoResourceHref === 'function')
            ? visotoResourceHref(iri) : '/resource?iri=' + encodeURIComponent(iri);
          a.textContent = label;
          li.appendChild(a);
        }
        ol.appendChild(li);
      });
      breadcrumbBox.hidden = false;
    }

    // --- Search -------------------------------------------------------------
    // Wunderbaum's own filter only sees LOADED nodes, which on a lazy tree is
    // misleading — so below the threshold the box says it is filtering locally,
    // and at or above it the search goes to the endpoint.
    var searchTimer = null;
    if (searchInput) {
      searchInput.addEventListener('input', function (e) {
        clearTimeout(searchTimer);
        var term = e.target.value.trim();
        searchTimer = setTimeout(function () { runSearch(term); }, SEARCH_DEBOUNCE_MS);
      });
    }
    if (flatToggle) {
      flatToggle.addEventListener('change', function () {
        if (searchInput && searchInput.value.trim()) runSearch(searchInput.value.trim());
      });
    }

    function runSearch(term) {
      if (!tree) return;
      if (!term) {
        tree.clearFilter();
        setStatus('');
        if (flatToggleBox) flatToggleBox.hidden = true;
        return;
      }
      if (term.length < minSearch) {
        // Local filter over what is loaded, and say so — otherwise "no results"
        // reads as "not in the hierarchy" rather than "not loaded yet".
        tree.filterNodes(term);
        setStatus(escapeHTML(vsTf('js.lazyTree.localFilter',
          'Filtering loaded nodes only. Type {n} characters to search the whole hierarchy.',
          { n: minSearch })));
        return;
      }
      if (flatToggleBox) flatToggleBox.hidden = false;
      setStatus(escapeHTML(vsT('js.lazyTree.searching', 'Searching…')));

      fetchRole('search', { q: term }).then(function (env) {
        if (env.error) {
          // No search role declared, or the query failed: fall back to the local
          // filter rather than leaving the box inert.
          tree.filterNodes(term);
          setStatus(escapeHTML(vsT('js.lazyTree.localFilterOnly',
            'Searching loaded nodes only.')));
          return;
        }
        var hits = env.nodes || [];
        if (!hits.length) {
          setStatus(escapeHTML(vsT('js.lazyTree.noResults', 'No matches.')));
          tree.filterNodes(' __no_match__');   // hide everything
          return;
        }
        var flat = flatToggle ? flatToggle.checked : !!cfg.flatSearch;
        if (flat) {
          showFlatHits(hits, env);
        } else {
          revealHits(hits, env);
        }
      }).catch(function (err) {
        console.error('sparqlLazyTree: search failed', err);
        setStatus(escapeHTML(vsT('js.lazyTree.searchFailed', 'The search failed.')), 'error');
      });
    }

    // Flat presentation: replace the tree with the hits. Cheap — no ancestor walk.
    function showFlatHits(hits, env) {
      tree.clearFilter();
      var flatSource = hits.map(function (n) {
        var copy = Object.assign({}, n);
        copy.lazy = false;      // a flat hit is a leaf in this presentation
        return copy;
      });
      tree.root.removeChildren();
      tree.root.addChildren(flatSource);
      tree.update(mar10.ChangeType.structure);
      setStatus(escapeHTML(vsTf('js.lazyTree.flatHits', '{n} matches, shown as a flat list.',
        { n: hits.length }) + (env.complete ? '' : ' ' + vsT('js.lazyTree.moreHits', 'More matches exist.'))));
    }

    // Hierarchical presentation: reveal hits in place.
    //
    // Revealing a hit means knowing its ancestors, and that is one /focus request
    // per hit — there is no batch form, because each hit has its own chain. So only
    // the first MAX_REVEALED are revealed, and the status line says so rather than
    // letting the tree look as though it found fewer matches than it did. Users who
    // want to see all of them switch to the flat list, which costs nothing extra.
    function revealHits(hits, env) {
      var keys = hits.map(function (h) { return h.key; });
      var revealing = Math.min(keys.length, MAX_REVEALED);
      setStatus(escapeHTML(
        revealing < hits.length
          ? vsTf('js.lazyTree.partialReveal',
              '{n} matches. Showing the first {shown} in the hierarchy — switch to the flat list to see them all.',
              { n: hits.length, shown: revealing })
          : vsTf('js.lazyTree.hierarchicalHits', '{n} matches.', { n: hits.length })));

      // Each request returns a whole chain AND the levels along it, so the reveal
      // needs no further round trips per hit.
      var walks = keys.slice(0, revealing).map(function (key) {
        return fetchRole('focus', { node: key }).catch(function () { return null; });
      });
      Promise.all(walks).then(function (results) {
        var seeded = false;
        results.forEach(function (focus) {
          if (!focus || focus.error || !focus.levels) return;
          for (var parentKey in focus.levels) {
            if (levelCache[parentKey] === undefined) { levelCache[parentKey] = focus.levels[parentKey]; seeded = true; }
          }
        });
        if (!seeded) return;
        var chains = results.filter(Boolean).map(function (f) { return f.path || []; });
        var expandAll = chains.reduce(function (acc, chain) {
          return acc.concat(chain.slice(0, -1));
        }, []);
        var uniq = expandAll.filter(function (k, i) { return expandAll.indexOf(k) === i; });
        var step = function (i) {
          if (i >= uniq.length) return Promise.resolve();
          var node = tree.findKey ? tree.findKey(uniq[i]) : null;
          if (!node || !node.setExpanded) return step(i + 1);
          return node.setExpanded(true).catch(function () {}).then(function () { return step(i + 1); });
        };
        withoutRemembering(function () {
          return step(0).then(function () {
            tree.filterNodes(searchInput ? searchInput.value.trim() : '');
          });
        });
      });
    }

    // --- Collapse all -------------------------------------------------------
    var collapseBtn = document.getElementById('collapse-tree-' + ID);
    if (collapseBtn) {
      collapseBtn.addEventListener('click', function (e) {
        e.preventDefault();
        if (!tree) return;
        tree.visit(function (node) { node.setExpanded(false); });
        rememberExpansion();
      });
    }

    // --- External focus -----------------------------------------------------
    // A documented way for other page code to move the tree without a navigation:
    //   el.dispatchEvent(new CustomEvent('visoto:focus-node', {detail:{iri: '…'}}))
    root.addEventListener('visoto:focus-node', function (e) {
      var iri = e.detail && e.detail.iri;
      if (!iri || !tree) return;
      fetchRole('focus', { node: iri }).then(function (focus) {
        if (!focus || focus.error || !focus.levels) return;
        indexLabels(focus.levels);
        for (var key in focus.levels) {
          if (key !== '') levelCache[key] = focus.levels[key];
        }
        restoreExpansion(focus.path || []);
        renderBreadcrumb(focus.path || []);
      });
    });
  }

  // Re-entrant on purpose: a duplicate <script src> tag, or this file being
  // re-executed inside an HTMX-swapped fragment, must still pick up elements that
  // were not in the DOM the first time. No module-level "already loaded" latch —
  // only the per-element guard below.
  function boot() {
    document.querySelectorAll('[data-sparql-lazy-tree]').forEach(function (root) {
      if (root.__visotoLazyTreeInit) return;
      root.__visotoLazyTreeInit = true;
      initLazyTree(root);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
