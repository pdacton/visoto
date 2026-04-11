// monitoring.js — fetches endpoint metrics and renders a Chart.js time series

(function () {
  'use strict';

  const COLORS = [
    '#4e9af1', '#f1a14e', '#4ef17a', '#f14e6b',
    '#a14ef1', '#4ef1e8', '#f1e84e', '#b0b0b0',
  ];

  let chart = null;
  let currentRange = '24h';
  let toggling = false;

  // ---- update the switch state ----
  function updateBadge(enabled) {
    const toggle = document.getElementById('monitoring-toggle');
    const label = document.getElementById('monitor-badge');
    if (toggle) toggle.checked = enabled;
    if (label) label.textContent = enabled ? 'Monitoring enabled' : 'Monitoring disabled';
  }

  // ---- render status table ----
  function renderStatusTable(endpoints) {
    const tbody = document.getElementById('status-table-body');
    if (!tbody) return;
    if (!endpoints || endpoints.length === 0) {
      tbody.innerHTML = '<tr><td colspan="4" class="text-muted">No endpoints configured.</td></tr>';
      return;
    }
    tbody.innerHTML = endpoints.map(function (ep) {
      const last = ep.last;
      if (!last) {
        return `<tr>
          <td>${escHtml(ep.name)}<br><small class="text-muted">${escHtml(ep.url)}</small></td>
          <td colspan="3" class="text-muted">no data yet</td>
        </tr>`;
      }
      const ok = last.status === 'ok';
      const dur = last.duration_ms < 0 ? 'error' : last.duration_ms + ' ms';
      const cls = ok ? 'text-success' : 'text-danger';
      const ts = new Date(last.measured_at).toLocaleString();
      return `<tr>
        <td>${escHtml(ep.name)}<br><small class="text-muted">${escHtml(ep.url)}</small></td>
        <td class="${cls}">${dur}</td>
        <td class="${cls}">${escHtml(last.status)}</td>
        <td>${ts}</td>
      </tr>`;
    }).join('');
  }

  // ---- load status (badge + table) ----
  function loadStatus() {
    fetch('/api/monitoring/status')
      .then(function (r) { return r.json(); })
      .then(function (data) {
        if (!toggling) updateBadge(data.enabled);
        renderStatusTable(data.endpoints);
      })
      .catch(function (err) { console.error('monitoring status error', err); });
  }

  // ---- load and render chart ----
  function loadChart(range) {
    fetch('/api/monitoring/data?range=' + range)
      .then(function (r) { return r.json(); })
      .then(function (data) {
        renderChart(data.series || []);
      })
      .catch(function (err) { console.error('monitoring data error', err); });
  }

  function renderChart(series) {
    const canvas = document.getElementById('monitoring-chart');
    const noData = document.getElementById('no-data-msg');
    if (!canvas) return;

    // Check if there's any actual data
    const hasData = series.some(function (s) { return s.data && s.data.length > 0; });
    if (!hasData) {
      canvas.style.display = 'none';
      if (noData) noData.style.display = '';
      return;
    }
    canvas.style.display = '';
    if (noData) noData.style.display = 'none';

    const datasets = series.map(function (s, i) {
      return {
        label: s.name,
        data: s.data
          .filter(function (pt) { return pt[1] >= 0; }) // skip errors
          .map(function (pt) { return { x: pt[0], y: pt[1] }; }),
        borderColor: COLORS[i % COLORS.length],
        backgroundColor: COLORS[i % COLORS.length] + '22',
        borderWidth: 1.5,
        pointRadius: 2,
        tension: 0.2,
        fill: false,
      };
    });

    if (chart) {
      chart.data.datasets = datasets;
      chart.update();
      return;
    }

    const isDark = document.documentElement.getAttribute('data-bs-theme') === 'dark';
    const gridColor = isDark ? 'rgba(255,255,255,0.1)' : 'rgba(0,0,0,0.1)';
    const textColor = isDark ? '#adb5bd' : '#495057';

    chart = new Chart(canvas, {
      type: 'line',
      data: { datasets: datasets },
      options: {
        animation: false,
        responsive: true,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: {
            labels: { color: textColor },
          },
          tooltip: {
            callbacks: {
              label: function (ctx) {
                return ctx.dataset.label + ': ' + ctx.parsed.y + ' ms';
              },
            },
          },
        },
        scales: {
          x: {
            type: 'time',
            time: { tooltipFormat: 'PPpp' },
            ticks: { color: textColor },
            grid: { color: gridColor },
          },
          y: {
            title: { display: true, text: 'Response time (ms)', color: textColor },
            ticks: { color: textColor },
            grid: { color: gridColor },
            min: 0,
          },
        },
      },
    });
  }

  // ---- time range buttons ----
  function bindRangeButtons() {
    document.querySelectorAll('[data-range]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        document.querySelectorAll('[data-range]').forEach(function (b) {
          b.classList.remove('active');
        });
        btn.classList.add('active');
        currentRange = btn.dataset.range;
        loadChart(currentRange);
      });
    });
  }

  // ---- dark mode change: rebuild chart ----
  new MutationObserver(function () {
    if (chart) {
      chart.destroy();
      chart = null;
      loadChart(currentRange);
    }
  }).observe(document.documentElement, { attributes: true, attributeFilter: ['data-bs-theme'] });

  // ---- init ----
  document.addEventListener('DOMContentLoaded', function () {
    bindRangeButtons();
    loadStatus();
    loadChart(currentRange);
    // Refresh status table every 30 seconds
    setInterval(loadStatus, 30000);

    const toggleInput = document.getElementById('monitoring-toggle');
    if (toggleInput) {
      toggleInput.addEventListener('change', function () {
        toggling = true;
        fetch('/api/monitoring/toggle', { method: 'POST' })
          .then(function (r) { return r.json(); })
          .then(function (d) { updateBadge(d.enabled); })
          .catch(function () { updateBadge(!toggleInput.checked); }) // revert on error
          .finally(function () { toggling = false; });
      });
    }
  });

  // ---- escape HTML helper ----
  function escHtml(str) {
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }
})();
