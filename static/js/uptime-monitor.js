/* eslint-disable */
/*
  Uptime bars on the monitoring page (templates/pages/monitoring.html, the
  [data-uptime] card): one Tabler .tracking row per monitored endpoint, one
  .tracking-block per UTC day, from GET /api/monitoring/uptime.

  A day is green when every probe succeeded, yellow from 95 %, red below, and
  stays the neutral block colour when there were no probes (monitoring off, or
  before the 30-day retention).
*/
(function () {
  'use strict';

  var WARN_FROM = 0.95;

  function lang() {
    return document.documentElement.lang || undefined;
  }

  function pct(ok, probes) {
    return (100 * ok / probes).toLocaleString(lang(), { maximumFractionDigits: 2 }) + ' %';
  }

  function dayLabel(iso) {
    return new Date(iso + 'T00:00:00Z').toLocaleDateString(lang(), {
      timeZone: 'UTC', day: 'numeric', month: 'short', year: 'numeric'
    });
  }

  function blockClass(d) {
    if (!d.probes) return '';
    if (d.ok === d.probes) return 'bg-success';
    return d.ok / d.probes >= WARN_FROM ? 'bg-warning' : 'bg-danger';
  }

  function blockTitle(d) {
    var day = dayLabel(d.date);
    if (!d.probes) return vsTf('js.monitoring.uptimeNoData', '{day}: no probes', { day: day });
    return vsTf('js.monitoring.uptimeDay', '{day}: {ok} of {probes} probes ok ({pct})',
      { day: day, ok: d.ok, probes: d.probes, pct: pct(d.ok, d.probes) });
  }

  function renderEndpoint(ep) {
    var probes = 0, ok = 0;
    ep.days.forEach(function (d) { probes += d.probes; ok += d.ok; });

    var row = document.createElement('div');
    row.className = 'mb-3';

    var head = document.createElement('div');
    head.className = 'd-flex justify-content-between align-items-baseline mb-1';
    var name = document.createElement('span');
    name.className = 'fw-medium';
    name.textContent = ep.name;
    var total = document.createElement('span');
    total.className = 'text-secondary small';
    total.textContent = probes ? pct(ok, probes) : vsT('js.monitoring.uptimeNoProbes', 'no probes');
    head.appendChild(name);
    head.appendChild(total);

    var track = document.createElement('div');
    track.className = 'tracking';
    ep.days.forEach(function (d) {
      var b = document.createElement('div');
      b.className = ('tracking-block ' + blockClass(d)).trim();
      b.title = blockTitle(d);
      track.appendChild(b);
    });

    row.appendChild(head);
    row.appendChild(track);
    return row;
  }

  function boot() {
    var card = document.querySelector('[data-uptime]');
    if (!card) return;
    var body = card.querySelector('[data-uptime-rows]');

    fetch('/api/monitoring/uptime')
      .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
      .then(function (data) {
        body.textContent = '';
        var eps = data.endpoints || [];
        if (!eps.length) {
          var p = document.createElement('p');
          p.className = 'text-secondary mb-0';
          p.textContent = vsT('js.monitoring.uptimeNone', 'No monitored endpoints.');
          body.appendChild(p);
          return;
        }
        eps.forEach(function (ep) { body.appendChild(renderEndpoint(ep)); });
      })
      .catch(function (err) {
        console.error('monitoring uptime error', err);
        body.textContent = vsT('js.monitoring.uptimeFailed', 'Could not load uptime.');
      });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
