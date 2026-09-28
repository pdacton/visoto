/* eslint-disable */
/*
  "Class instances" card on /monitoring: the daily class-instance counts of the
  endpoints with class_stats = true (internal/classstats) — the latest run per
  endpoint, and the classes that appeared, vanished, grew or shrank since 1, 7
  or 30 days before. Data: GET /api/class-stats/status and
  /api/class-stats/changes?endpoint=&days=. Markup: templates/pages/monitoring.html.
*/
(function () {
  'use strict';

  var MAX_ROWS = 100;

  function fmt(n) {
    return n == null ? '–' : Number(n).toLocaleString();
  }

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  function shortName(iri) {
    var m = /[#/]([^#/]+)[#/]?$/.exec(iri);
    return m ? decodeURIComponent(m[1]) : iri;
  }

  function resourceLink(iri, slug) {
    var u = new URL(window.visotoResourceHref ? window.visotoResourceHref(iri) : '/resource?iri=' + encodeURIComponent(iri), location.origin);
    u.searchParams.set('endpoint', slug);
    var a = el('a', null, shortName(iri));
    a.href = u.pathname + u.search;
    a.title = iri;
    return a;
  }

  function boot(card) {
    var select = card.querySelector('[data-class-stats-endpoint]');
    var summary = card.querySelector('[data-class-stats-summary]');
    var rows = card.querySelector('[data-class-stats-rows]');
    var dayButtons = card.querySelectorAll('[data-class-stats-days]');
    var status = {};
    var days = 1;

    function showSummary(st, ch) {
      summary.replaceChildren();
      if (!st || !st.day) {
        summary.textContent = vsT('js.monitoring.classStatsNoRun', 'No run yet. The first one starts when the server starts, then every day at 07:00.');
        return;
      }
      var engine = st.engine === 'graphdb'
        ? vsT('js.monitoring.classStatsEngineStats', 'GraphDB statistics (approximate)')
        : vsT('js.monitoring.classStatsEngineCount', 'live counts');
      summary.textContent = vsTf('js.monitoring.classStatsSummary',
        'Snapshot of {day}: {classes} classes, {triples} triples, counted with {engine} in {seconds} s.',
        { day: st.day, classes: fmt(st.classes), triples: fmt(st.triples), engine: engine, seconds: st.seconds });
      if (st.failures) {
        summary.appendChild(document.createTextNode(' '));
        summary.appendChild(el('span', 'text-warning', vsTf('js.monitoring.classStatsFailures',
          '{n} classes could not be counted.', { n: fmt(st.failures) })));
      }
      if (ch && ch.from) {
        summary.appendChild(el('br'));
        summary.appendChild(document.createTextNode(vsTf('js.monitoring.classStatsCompared',
          'Compared with {from}: {n} classes changed; store size {before} → {after} triples.',
          { from: ch.from, n: fmt(ch.changes.length), before: fmt(ch.triplesBefore), after: fmt(ch.triplesAfter) })));
      }
    }

    function showMessage(text) {
      rows.replaceChildren();
      var tr = el('tr');
      var td = el('td', 'text-secondary', text);
      td.colSpan = 4;
      tr.appendChild(td);
      rows.appendChild(tr);
    }

    function showChanges(slug, ch) {
      if (!ch || !ch.from) {
        showMessage(vsT('js.monitoring.classStatsNothingToCompare', 'No older snapshot to compare with yet.'));
        return;
      }
      if (!ch.changes.length) {
        showMessage(vsT('js.monitoring.classStatsNoChanges', 'No class changed.'));
        return;
      }
      rows.replaceChildren();
      ch.changes.slice(0, MAX_ROWS).forEach(function (c) {
        var tr = el('tr');
        var name = el('td');
        name.appendChild(resourceLink(c.class, slug));
        if (c.before == null) name.appendChild(el('span', 'badge bg-green-lt ms-2', vsT('js.monitoring.classStatsNew', 'new')));
        else if (c.after == null) name.appendChild(el('span', 'badge bg-red-lt ms-2', vsT('js.monitoring.classStatsVanished', 'vanished')));
        else if (c.drop) name.appendChild(el('span', 'badge bg-red-lt ms-2', vsT('js.monitoring.classStatsDrop', 'drop')));
        tr.appendChild(name);
        tr.appendChild(el('td', 'text-end', fmt(c.before)));
        tr.appendChild(el('td', 'text-end', fmt(c.after)));
        var delta = (c.delta > 0 ? '+' : '') + fmt(c.delta);
        if (c.pct != null) delta += ' (' + (c.pct > 0 ? '+' : '') + (c.pct * 100).toFixed(1) + ' %)';
        tr.appendChild(el('td', 'text-end ' + (c.delta < 0 ? 'text-danger' : 'text-success'), delta));
        if (c.drop) tr.classList.add('table-danger');
        rows.appendChild(tr);
      });
      if (ch.changes.length > MAX_ROWS) {
        var tr = el('tr');
        var td = el('td', 'text-secondary', vsTf('js.monitoring.classStatsMore', '{n} more changes not shown.', { n: fmt(ch.changes.length - MAX_ROWS) }));
        td.colSpan = 4;
        tr.appendChild(td);
        rows.appendChild(tr);
      }
    }

    function load() {
      var slug = select.value;
      if (!slug) return;
      var st = status[slug];
      if (!st || !st.day) {
        showSummary(st, null);
        showMessage(vsT('js.monitoring.classStatsNothingToCompare', 'No older snapshot to compare with yet.'));
        return;
      }
      fetch('/api/class-stats/changes?endpoint=' + encodeURIComponent(slug) + '&days=' + days)
        .then(function (r) { return r.ok ? r.json() : null; })
        .then(function (ch) {
          if (select.value !== slug) return; // a newer choice won
          showSummary(st, ch);
          showChanges(slug, ch);
        })
        .catch(function (err) { console.error('class stats', err); });
    }

    fetch('/api/class-stats/status')
      .then(function (r) { return r.json(); })
      .then(function (list) {
        if (!list.length) {
          card.hidden = true; // nothing is collected on this server
          return;
        }
        list.forEach(function (st) {
          status[st.slug] = st;
          var opt = el('option', null, st.name);
          opt.value = st.slug;
          select.appendChild(opt);
        });
        var active = window.activeEndpointSlug && window.activeEndpointSlug();
        if (active && status[active]) select.value = active;
        load();
      })
      .catch(function (err) {
        console.error('class stats', err);
        summary.textContent = vsT('js.monitoring.classStatsUnavailable', 'Class statistics are not available.');
      });

    select.addEventListener('change', load);
    dayButtons.forEach(function (btn) {
      btn.addEventListener('click', function () {
        dayButtons.forEach(function (b) { b.classList.toggle('active', b === btn); });
        days = Number(btn.getAttribute('data-class-stats-days'));
        load();
      });
    });
  }

  function start() {
    document.querySelectorAll('[data-class-stats]').forEach(boot);
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
