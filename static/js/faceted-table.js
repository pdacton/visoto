/*
 * Faceted search — shared control builders. Pairs with the <sparql-column>
 * declarations on the page: a table is faceted exactly when one of its columns
 * carries a filter, which the server reports back as the fragment's facetFor.
 *
 * Filters are attached to COLUMN HEADERS of the one working-set Tabulator table (see
 * templates/partials/sparql-table.html), not a separate panel. This module is the
 * reusable toolkit that table init calls to:
 *
 *   - read the <sparql-column> declarations from the page DOM (readColumns) and
 *     resolve the ones that left their filter kind or type to the data (resolveColumn)
 *   - build one facet control (select checkbox dropdown / range / text), each with a
 *     "(no value)" option that matches members lacking any value on the facet path
 *   - build the header-attached funnel UI for a facetable column (headerFormatter):
 *     column title + funnel button + a body-portaled dropdown hosting the control
 *   - lazy-load a select facet's value list from /api/facet-values on first open
 *   - serialize the active selections into /api/faceted-table query params
 *
 * Every control calls an onChange callback the table supplies; the table decides what
 * to do (instant local filter + debounced backend authority). This module owns no
 * table state and never calls into Tabulator — headerFormatter only returns a DOM
 * node through Tabulator's titleFormatter contract.
 */
(function () {
  'use strict';

  // Mirrors facet.NoValueSentinel (internal/facet/types.go): the value a SELECT facet
  // sends to mean "members lacking any value for this facet" (→ NOT EXISTS). Range and
  // text facets send it out-of-band as f.<var>.novalue=1 instead (their positional
  // params stay clean), but the UI affordance is the same "(no value)" checkbox.
  var NO_VALUE = '__vs_no_value__';

  // The active endpoint slug, so cacheable routes stay pure functions of the URL
  // (shared resolver from endpoint-switcher.js). Returns a bare "endpoint=<slug>"
  // pair — callers join it with the rest, so it never depends on whether other
  // params happen to precede it.
  function endpointParam() {
    var slug = (typeof activeEndpointSlug === 'function') ? activeEndpointSlug() : '';
    return slug ? 'endpoint=' + encodeURIComponent(slug) : '';
  }

  // The active site language, so these routes return labels in the picked
  // language (they read only the URL — shared resolver from
  // language-switcher.js).
  //
  // Note the deliberate asymmetry with endpointParam: an empty slug means "none
  // selected", so dropping the pair and letting the server default is right. An
  // empty language is a *choice* ("no language"), so this returns a bare "lang="
  // rather than dropping it — an absent param would mean the server default,
  // which is a different language.
  function langParam() {
    var code = (typeof activeSiteLang === 'function') ? activeSiteLang() : null;
    return code === null ? '' : 'lang=' + encodeURIComponent(code);
  }

  // The template set the base query id is scoped to. Read live from the page
  // rather than from the fragment config: unlike the endpoint and the language,
  // it describes the page these fetches are running on, not how this fragment
  // was rendered, and it is always present on a rendered page.
  function srcParam() {
    var set = (typeof activeTemplateSet === 'function') ? activeTemplateSet() : '';
    return set ? 'src=' + encodeURIComponent(set) : '';
  }

  // Join non-empty query-param pairs into a query string.
  function joinParams(parts) {
    return parts.filter(function (p) { return !!p; }).join('&');
  }

  // ---- reload-aware fetch options -------------------------------------------
  // The table's data (working set, faceted results, facet values) is fetched by
  // JS, not by the browser as a document subresource. A hard reload
  // (Ctrl-Shift-R) only bypasses the HTTP cache for browser-initiated requests —
  // by the time these fetch()es run the navigation is finished and that flag no
  // longer applies. With Cache-Control: public, max-age=21600 on those routes,
  // the effect is that Ctrl-Shift-R refreshes the JS/CSS but serves table data
  // that can be up to 6h old (and, if a query's SELECT vars changed, of the wrong
  // shape entirely).
  //
  // So we make these requests inherit the page's reload intent: normal
  // navigations use "default" (full caching, no extra round-trips), reloads use
  // "reload" (bypass the cache), matching what the browser does for scripts.
  //
  // NOTE: the Navigation Timing API reports plain F5 and Ctrl-Shift-R both as
  // type "reload" — it cannot distinguish them — so F5 also refetches. That is
  // the intended reading of "reload this page". Only this browser's cache is
  // affected; the shared Souin/CDN copy is untouched.
  var isReloadNavigation = (function () {
    try {
      var nav = performance.getEntriesByType('navigation')[0];
      if (nav && nav.type) return nav.type === 'reload';
      // Fallback for older browsers still exposing the legacy API.
      return !!(performance.navigation && performance.navigation.type === 1);
    } catch (e) {
      return false;
    }
  })();

  // fetchOptions builds the init object for a cacheable data fetch, adding the
  // cache mode only when this navigation was a reload.
  function fetchOptions(extra) {
    var init = extra || {};
    if (isReloadNavigation) init.cache = 'reload';
    return init;
  }

  // Collect the <sparql-column> declarations for one base query from the page DOM,
  // in document order. A column names its query with for=, or inherits it from an
  // enclosing <sparql-columns for="…"> so a table writes the id once.
  //
  // What comes back is the DECLARATION, not the finished column: filter and type may
  // be empty, meaning "resolve me from the data" — see resolveColumn, which the table
  // calls once it holds rows.
  function readColumns(id) {
    var specs = [];
    document.querySelectorAll('sparql-column').forEach(function (el) {
      var box = el.closest('sparql-columns');
      var owner = el.getAttribute('for') || (box ? box.getAttribute('for') : '');
      if (owner !== id) return;
      // order is declared once, on the container: "these columns are in the order
      // I wrote them". Reordering every table by declaration order would be wrong —
      // most declare only the few columns they have something to say about.
      var ordered = flagAttr(el, 'order') || (box ? flagAttr(box, 'order') : false);
      var name = (el.getAttribute('var') || '').replace(/^\?/, '');
      if (!name) return;
      specs.push({
        name: name,
        label: el.getAttribute('label') || '',
        tip: el.getAttribute('tip') || '',
        // Presence is the signal: a bare filter asks for inference, no filter at all
        // means the column is declared only to name/explain/render itself.
        filter: el.hasAttribute('filter') ? ((el.getAttribute('filter') || '').trim().toLowerCase() || 'auto') : '',
        type: (el.getAttribute('type') || '').trim(),
        icon: flagAttr(el, 'icon'),
        badge: flagAttr(el, 'badge'),
        group: flagAttr(el, 'group'),
        hidden: flagAttr(el, 'hidden'),
        width: (el.getAttribute('width') || '').trim(),
        ordered: ordered
      });
    });
    return specs;
  }

  // Mirrors column.flagAttr (internal/column/column.go): presence means on, unless
  // the attribute carries an explicit falsy value.
  function flagAttr(el, name) {
    if (!el.hasAttribute(name)) return false;
    switch ((el.getAttribute(name) || '').trim().toLowerCase()) {
      case 'false': case '0': case 'no': case 'off': return false;
    }
    return true;
  }

  // When a checkbox list is the better control.
  //
  // An IRI column enumerates ENTITIES — cantons, legal forms, properties — and
  // picking from a list of them is what a user wants, up to the point where the
  // server's own enumeration would truncate: DefaultEnumerateLimit
  // (internal/facet/builder.go) is 200, and past it a working-set table would offer
  // a silently partial list. So that is the line.
  var SELECT_MAX_IRI = 200;

  // A string column is different: its distinct count usually just tracks the row
  // count. A dropdown only makes sense for a genuinely small CONTROLLED VOCABULARY
  // ("outgoing"/"incoming", a status), which shows up as few values REPEATING across
  // the rows that carry them. Canton names are 26 values over 26 rows — small, but
  // one per row, so a text search is right; ?kind is 2 values over thousands.
  //
  // The ratio is measured against the rows that actually have a value, not all rows:
  // a column bound on one row in twenty-six (a Romansh name) is unique, not a
  // vocabulary of one.
  var SELECT_MAX_STRING = 25;
  var SELECT_MIN_REPEAT = 4;

  var ISO_DATE_RE = /^\d{4}-\d{2}-\d{2}/;

  // Resolve one declaration against the data: fill in the type and filter kind the
  // author left implicit, and return the finished spec the controls are built from.
  //
  // probe is what the table read off the loaded rows for this column:
  //   { sample, distinct, bound } — the first BOUND binding (not row 0's, which is
  // often unbound on an OPTIONAL column), how many distinct values there are, and
  // how many rows carry one.
  function resolveColumn(spec, probe) {
    probe = probe || {};
    var type = spec.type || inferType(probe.sample);
    // An absent filter stays absent — the column is declared for its name or tip.
    var control = spec.filter;
    if (control === 'auto') control = inferControl(type, probe.distinct || 0, probe.bound || 0);
    return {
      name: spec.name,
      label: spec.label || spec.name,
      tip: spec.tip,
      filter: spec.filter,
      control: control,
      type: type,
      icon: spec.icon,
      badge: spec.badge,
      group: spec.group,
      hidden: spec.hidden,
      width: spec.width,
      ordered: spec.ordered
    };
  }

  function inferType(sample) {
    if (!sample) return 'string';
    if (sample.Type === 'uri') return 'iri';
    var text = String(sample.DisplayText || sample.Value || '').trim();
    if (ISO_DATE_RE.test(text)) return 'date';
    // Number() over parseFloat: "8001 Zürich" must NOT read as a number, and
    // parseFloat would happily return 8001 and offer a min/max control for it.
    if (text !== '' && !isNaN(Number(text))) return 'number';
    return 'string';
  }

  function inferControl(type, distinct, bound) {
    if (type === 'number' || type === 'date') return 'range';
    if (distinct <= 0) return 'text';
    if (type === 'iri') return distinct <= SELECT_MAX_IRI ? 'select' : 'text';
    var repeats = distinct * SELECT_MIN_REPEAT <= bound;
    return (distinct <= SELECT_MAX_STRING && repeats) ? 'select' : 'text';
  }

  function el(tag, cls, attrs) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (attrs) Object.keys(attrs).forEach(function (k) { n.setAttribute(k, attrs[k]); });
    return n;
  }

  // One .form-check checkbox row. muted=true dims the "(no value)" pseudo-option so it
  // reads as distinct from real values. onToggle fires after the box changes.
  function checkRow(value, label, count, muted, onToggle) {
    var row = el('div', 'form-check');
    var input = el('input', 'form-check-input vs-check', { type: 'checkbox' });
    input.value = value;
    var lab = el('label', 'form-check-label' + (muted ? ' text-secondary fst-italic' : ''));
    lab.textContent = count ? (label + ' (' + count + ')') : label;
    lab.style.cursor = 'pointer';
    lab.addEventListener('click', function () { input.click(); });
    input.addEventListener('change', onToggle);
    row.appendChild(input);
    row.appendChild(lab);
    return row;
  }

  // A "(no value)" toggle for range/text controls, where there's no value list to sit
  // in. Returns the wrapper; the checkbox is .vs-novalue.
  function noValueToggle(onToggle) {
    var row = el('div', 'form-check mt-1');
    var input = el('input', 'form-check-input vs-novalue', { type: 'checkbox' });
    var lab = el('label', 'form-check-label text-secondary fst-italic');
    lab.textContent = vsT('js.facet.noValue', '(no value)');
    lab.style.cursor = 'pointer';
    lab.addEventListener('click', function () { input.click(); });
    input.addEventListener('change', onToggle);
    row.appendChild(input);
    row.appendChild(lab);
    return row;
  }

  // Build one facet control for a spec. `ctx` = { id, iri, onChange }. Returns a
  // .vs-facet element carrying the current selection; onChange is called on any change.
  function buildControl(spec, ctx) {
    var wrap = el('div', 'vs-facet p-2');
    wrap.dataset.var = spec.name;
    wrap.dataset.control = spec.control;
    wrap.dataset.type = spec.type;

    var apply = ctx.onChange || function () {};

    if (spec.control === 'range') {
      var inputType = spec.type === 'date' ? 'date' : 'number';
      var line = el('div', 'd-flex align-items-center gap-1');
      var min = el('input', 'form-control form-control-sm vs-min', { type: inputType, placeholder: vsT('js.facet.min', 'min') });
      var max = el('input', 'form-control form-control-sm vs-max', { type: inputType, placeholder: vsT('js.facet.max', 'max') });
      var dash = el('span', 'text-secondary'); dash.textContent = '–';
      line.appendChild(min); line.appendChild(dash); line.appendChild(max);
      wrap.appendChild(line);
      min.addEventListener('change', apply);
      max.addEventListener('change', apply);
      wrap.appendChild(noValueToggle(apply));

    } else if (spec.control === 'text') {
      var input = el('input', 'form-control form-control-sm vs-text', { type: 'text', placeholder: vsT('js.facet.contains', 'contains…') });
      wrap.appendChild(input);
      // Apply on blur/Enter only — never per keystroke (bounds the URL/cache key space).
      input.addEventListener('change', apply);
      input.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); apply(); } });
      wrap.appendChild(noValueToggle(apply));

    } else { // select — a scrollable checkbox list
      // Scroll cap and width live in CSS (.vs-facet-menu .vs-select-menu).
      var menu = el('div', 'vs-select-menu');
      menu.dataset.loaded = '0';
      // Fixed "(no value)" option — available even before the value list loads.
      menu.appendChild(checkRow(NO_VALUE, vsT('js.facet.noValue', '(no value)'), 0, true, apply));
      menu.appendChild(el('div', 'dropdown-divider'));
      var listHost = el('div', 'vs-select-list');
      var loading = el('div', 'text-secondary small px-1'); loading.textContent = vsT('js.facet.loading', 'Loading…');
      listHost.appendChild(loading);
      menu.appendChild(listHost);
      wrap.appendChild(menu);
      // Lazy-load the enumerated values the first time this control is opened; the
      // table calls loadFacetValues(wrap, ctx) from its header-menu open handler.
      wrap._loadValues = function () { loadFacetValues(wrap, spec.name, ctx, apply); };
    }
    return wrap;
  }

  // Render an enumerated value list ({value,label,count}) into a .vs-select-list.
  function renderValues(list, values, onToggle) {
    if (!list) return;
    list.innerHTML = '';
    if (!values.length) {
      var none = el('div', 'text-secondary small px-1'); none.textContent = vsT('js.facet.noValues', 'No values');
      list.appendChild(none);
      return;
    }
    values.forEach(function (v) {
      list.appendChild(checkRow(v.value, v.label, v.count, false, onToggle));
    });
  }

  // Lazy-load the enumerated values for a select facet into its .vs-select-list.
  //
  // ctx.localValues, when supplied, offers the options the loaded rows already
  // imply, so a table holding the complete population never calls the backend —
  // the point of the completeness gate, since column-mode enumeration costs a full
  // re-run of the base query. Its three answers:
  //
  //   array  — use these
  //   null   — rows have not landed yet (this also runs eagerly at header render);
  //            leave the menu unloaded so the next open retries, and do NOT fetch
  //   false  — only the backend can answer (an incomplete working set); fall through
  function loadFacetValues(wrap, name, ctx, onToggle) {
    var menu = wrap.querySelector('.vs-select-menu');
    if (!menu || menu.dataset.loaded === '1' || menu.dataset.loaded === 'loading') return;
    var list = wrap.querySelector('.vs-select-list');

    if (typeof ctx.localValues === 'function') {
      var local = ctx.localValues(name);
      if (local) {
        renderValues(list, local, onToggle);
        menu.dataset.loaded = '1';
        return;
      }
      if (local === null) return;
    }

    menu.dataset.loaded = 'loading';
    var url = '/api/facet-values/' + encodeURIComponent(ctx.id) + '/' + encodeURIComponent(name) +
      '?' + joinParams(['iri=' + encodeURIComponent(ctx.iri || ''), endpointParam(), langParam(), srcParam()]);
    fetch(url, fetchOptions({ headers: { 'Accept': 'application/json' } }))
      .then(function (r) { return r.json(); })
      .then(function (data) {
        renderValues(list, data.values || [], onToggle);
        menu.dataset.loaded = '1';
      })
      .catch(function () {
        menu.dataset.loaded = '0'; // allow a retry on next open
        if (list) {
          list.innerHTML = '';
          var errRow = el('div', 'text-danger small px-1'); errRow.textContent = vsT('js.facet.failedToLoad', 'Failed to load');
          list.appendChild(errRow);
        }
      });
  }

  // Tabulator titleFormatter factory for a facetable column: renders the column
  // title plus a funnel button that opens a dropdown hosting the facet control.
  // ctx = { tableId, facetFor, iri, onChange }; Tabulator calls the returned
  // function per header render (titleFormatter may return a DOM node).
  //
  // The facet menu is PORTALED to <body> while open: Tabulator clips its header
  // with overflow:hidden, so a menu positioned inside the header cell would be cut
  // off; fixed positioning under the button escapes that clip. The menu stays
  // permanently attached to <body> (hidden) rather than added/removed, so its
  // selection state stays queryable by the table's liveFacetControls() even while
  // closed. It is tagged data-facet-menu="<tableId>:<var>" and any prior menu for
  // the same table+var is dropped, so header re-renders don't leak menus (or
  // duplicate a control, which would split the selection).
  function headerFormatter(spec, ctx) {
    return function (cell) {
      // Layout (block, with the label ellipsising and the funnel absolutely
      // right-aligned) lives in tabulator_overrides.css — see the .vs-facet-header
      // rules there, which explain why the funnel cannot simply sit inline here.
      var wrap = el('span', 'vs-facet-header');
      var title = el('span', 'vs-facet-label');
      title.textContent = cell.getValue();
      // A declared tip explains the column on hover. Set on the header node we build
      // rather than through Tabulator's own tooltip option, so it works identically
      // on columns that carry no filter and never reach the funnel branch below.
      // The native title attribute is the whole affordance: tipped headers are
      // deliberately not marked visually (no underline, no help cursor).
      if (spec.tip) title.title = spec.tip;
      wrap.appendChild(title);

      // A column may be declared purely to name and explain itself.
      if (spec.control === 'none' || !spec.control) return wrap;

      var dd = el('span', 'dropdown vs-facet-dd');
      var btn = el('button', 'btn btn-sm btn-ghost-secondary p-0 px-1 vs-facet-btn',
        { type: 'button', 'aria-expanded': 'false' });
      btn.title = vsTf('js.facet.filterBy', 'Filter by {label}', { label: spec.label });
      // Icon size is set in CSS (.vs-facet-btn svg): lucide.createIcons() below
      // REPLACES this <i> with an <svg>, so styling the placeholder is pointless.
      btn.innerHTML = '<i data-lucide="list-filter"></i>';
      dd.appendChild(btn);

      // position:fixed and the menu width live in CSS (.vs-facet-menu); this code
      // only sets top/left, in positionMenu() below.
      var menu = el('div', 'dropdown-menu p-0 vs-facet-menu');
      var control = buildControl(spec, {
        id: ctx.facetFor,
        iri: ctx.iri,
        localValues: ctx.localValues,
        onChange: ctx.onChange
      });
      // Tag the control so the table's liveFacetControls() finds it even while the
      // menu is portaled to <body> (out of the table's DOM subtree).
      control.dataset.facetTable = ctx.tableId;
      menu.appendChild(control);

      var menuKey = ctx.tableId + ':' + spec.name;
      menu.dataset.facetMenu = menuKey;
      document.querySelectorAll('.vs-facet-menu[data-facet-menu="' + menuKey + '"]')
        .forEach(function (m) { if (m.parentNode) m.parentNode.removeChild(m); });
      document.body.appendChild(menu);

      // Proactively load the enumerated values now, rather than waiting for the
      // first funnel-open, so the list is ready the instant the menu is opened (no
      // "Loading…" flash). Only select facets have _loadValues; it is idempotent
      // (guarded by menu.dataset.loaded), so calling it here AND on open — and
      // across header re-renders — never double-fetches.
      if (control._loadValues) control._loadValues();

      var open = false;
      function positionMenu() {
        var r = btn.getBoundingClientRect();
        menu.style.top = (r.bottom + 2) + 'px';
        // Right-align the menu to the button, clamped to the viewport.
        // The fallback mirrors .vs-facet-menu's 15rem floor at a 16px root. It is
        // now near-dead: that rule sizes the menu with width:max-content, which
        // resolves to a real width even while hidden (a bare min-width measured
        // 0, which is why the fallback was load-bearing before). A range facet's
        // menu is wider than the floor, so trust offsetWidth, not this number.
        var width = menu.offsetWidth || 240;
        var left = Math.min(r.right - width, window.innerWidth - width - 8);
        menu.style.left = Math.max(8, left) + 'px';
      }
      function openMenu() {
        if (open) return;
        open = true;
        menu.classList.add('show');
        btn.setAttribute('aria-expanded', 'true');
        positionMenu();
        if (control._loadValues) control._loadValues(); // lazy-load enum values once
        document.addEventListener('mousedown', onDocDown, true);
        document.addEventListener('keydown', onDocKey, true);
        window.addEventListener('resize', positionMenu);
        window.addEventListener('scroll', positionMenu, true);
      }
      function closeMenu() {
        if (!open) return;
        open = false;
        menu.classList.remove('show');
        btn.setAttribute('aria-expanded', 'false');
        document.removeEventListener('mousedown', onDocDown, true);
        document.removeEventListener('keydown', onDocKey, true);
        window.removeEventListener('resize', positionMenu);
        window.removeEventListener('scroll', positionMenu, true);
      }
      function onDocDown(e) {
        // Close when clicking outside both the button and the (body-level) menu.
        if (menu.contains(e.target) || btn.contains(e.target)) return;
        closeMenu();
      }
      function onDocKey(e) {
        // Escape closes the menu and returns focus to the funnel button, so
        // keyboard users aren't stranded in a detached (body-portaled) subtree.
        if (e.key === 'Escape') { closeMenu(); btn.focus(); }
      }
      btn.addEventListener('click', function (e) {
        e.preventDefault();
        e.stopPropagation(); // don't trigger the column's header sort
        open ? closeMenu() : openMenu();
      });

      wrap.appendChild(dd);
      if (window.lucide) lucide.createIcons({ nodes: [btn] });
      return wrap;
    };
  }

  // Read the active selection for one .vs-facet control into a normalized object:
  //   { name, control, type, values:[...], min, max, term, noValue }
  // Only the fields relevant to the control are populated.
  function readSelection(wrap) {
    var sel = { name: wrap.dataset.var, control: wrap.dataset.control, type: wrap.dataset.type };
    if (sel.control === 'range') {
      sel.min = (wrap.querySelector('.vs-min').value || '').trim();
      sel.max = (wrap.querySelector('.vs-max').value || '').trim();
      sel.noValue = !!(wrap.querySelector('.vs-novalue') || {}).checked;
    } else if (sel.control === 'text') {
      sel.term = (wrap.querySelector('.vs-text').value || '').trim();
      sel.noValue = !!(wrap.querySelector('.vs-novalue') || {}).checked;
    } else { // select
      var values = [];
      wrap.querySelectorAll('.vs-check:checked').forEach(function (chk) { values.push(chk.value); });
      sel.values = values; // may include NO_VALUE
      sel.noValue = values.indexOf(NO_VALUE) !== -1;
    }
    return sel;
  }

  // Reset one .vs-facet control to its empty state (no selection). The inverse of
  // readSelection: unchecks every box, empties min/max/text and the (no value)
  // toggle, so a subsequent readSelection reports the control as inactive. Does NOT
  // fire onChange — the caller applies the combined reset once (see the reset flow
  // in sparql-table.html) to avoid one backend fetch per control.
  function clearControl(wrap) {
    if (!wrap) return;
    wrap.querySelectorAll('input[type="checkbox"]').forEach(function (chk) { chk.checked = false; });
    wrap.querySelectorAll('.vs-min, .vs-max, .vs-text').forEach(function (inp) { inp.value = ''; });
  }

  // Whether a selection actually constrains anything.
  function isActive(sel) {
    if (sel.control === 'range') return !!(sel.min || sel.max || sel.noValue);
    if (sel.control === 'text') return !!(sel.term || sel.noValue);
    return sel.values && sel.values.length > 0;
  }

  // Serialize a list of .vs-facet controls into /api/faceted-table query params.
  // passthrough = { title, icon, max } presentation params. The per-column roles
  // (icon, badge, grouping) are not among them: the backend reads those off the
  // <sparql-column> declarations directly.
  function buildFacetURL(id, iri, wraps, passthrough) {
    var parts = ['iri=' + encodeURIComponent(iri || '')];
    Object.keys(passthrough || {}).forEach(function (k) {
      var v = passthrough[k];
      if (v) parts.push(k + '=' + encodeURIComponent(v));
    });
    wraps.forEach(function (wrap) {
      var sel = readSelection(wrap);
      if (!isActive(sel)) return;
      var key = 'f.' + sel.name;
      // Tell the server which control and value type this facet resolved to. A
      // declaration that named them still wins server-side, and anything not in the
      // known sets is ignored (column.Spec.Facet), so these only ever fill the gap a
      // bare filter= left open.
      parts.push(key + '.as=' + encodeURIComponent(sel.control));
      parts.push(key + '.type=' + encodeURIComponent(sel.type));
      if (sel.control === 'range') {
        if (sel.min) parts.push(key + '.min=' + encodeURIComponent(sel.min));
        if (sel.max) parts.push(key + '.max=' + encodeURIComponent(sel.max));
        if (sel.noValue) parts.push(key + '.novalue=1');
      } else if (sel.control === 'text') {
        if (sel.term) parts.push(key + '=' + encodeURIComponent(sel.term));
        if (sel.noValue) parts.push(key + '.novalue=1');
      } else {
        // Multi-select: one repeated f.<name>= per checked box (→ QueryArray); the
        // NO_VALUE sentinel rides along as just another value (select convention).
        (sel.values || []).forEach(function (val) {
          parts.push(key + '=' + encodeURIComponent(val));
        });
      }
    });
    parts.push(endpointParam());
    parts.push(langParam());
    parts.push(srcParam());
    return '/api/faceted-table/' + encodeURIComponent(id) + '?' + joinParams(parts);
  }

  // Local predicate for one selection against a row's binding for the facet var.
  // `binding` is the SPARQL binding object ({Value, DisplayText, Type}) or null.
  // Enum facets select by IRI, so an iri-typed select must compare the binding's raw
  // Value (the IRI); other facets compare the human-readable display text — matching
  // how each is stored server-side and shown to the user.
  function matchesLocally(sel, binding) {
    var present = !!(binding && (binding.Value || binding.DisplayText));
    var display = present ? String(binding.DisplayText || binding.Value || '') : '';
    var raw = present ? String(binding.Value || binding.DisplayText || '') : '';
    var concrete;
    if (sel.control === 'range') {
      if (!sel.min && !sel.max) {
        concrete = false; // no bounds → only the no-value leg (if any) constrains
      } else if (sel.type === 'date') {
        // Dispatch on the DECLARED type, never on whether parseFloat succeeds:
        // parseFloat("2015-01-01") is 2015, not NaN, so a numeric-first branch
        // silently reduces every date comparison to its year (2015-01-01 would
        // match min=2015-06-30). ISO-8601 dates are lexicographically ordered,
        // so a plain string compare is both correct and granularity-preserving.
        concrete = present &&
          (!sel.min || display >= sel.min) &&
          (!sel.max || display <= sel.max);
      } else {
        var n = parseFloat(display);
        concrete = present && !isNaN(n) &&
          (!sel.min || n >= parseFloat(sel.min)) &&
          (!sel.max || n <= parseFloat(sel.max));
      }
    } else if (sel.control === 'text') {
      // Always the display text, even on an IRI column: a text box is a search for
      // what the cell SHOWS. (The backend leg agrees — internal/facet's textExpr
      // routes an iri-typed column through its label rather than its IRI string.)
      concrete = sel.term
        ? (present && display.toLowerCase().indexOf(sel.term.toLowerCase()) !== -1)
        : false;
    } else { // select
      var concreteVals = (sel.values || []).filter(function (v) { return v !== NO_VALUE; });
      // iri-typed enums selected the IRI → compare raw Value; string enums → display.
      var cmp = (sel.type === 'iri') ? raw : display;
      concrete = concreteVals.length ? (present && concreteVals.indexOf(cmp) !== -1) : false;
    }
    // "(no value)" (any control) OR's with the concrete match — mirrors the backend UNION.
    if (sel.noValue && !present) return true;
    return concrete;
  }

  // Public API for the table init in sparql-table.html.
  window.VisotoFacets = {
    NO_VALUE: NO_VALUE,
    fetchOptions: fetchOptions,
    isReloadNavigation: isReloadNavigation,
    readColumns: readColumns,
    resolveColumn: resolveColumn,
    buildControl: buildControl,
    headerFormatter: headerFormatter,
    readSelection: readSelection,
    clearControl: clearControl,
    isActive: isActive,
    buildFacetURL: buildFacetURL,
    matchesLocally: matchesLocally
  };
})();
