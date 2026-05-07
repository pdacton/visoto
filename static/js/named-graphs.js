// Named Graphs modal — load, display, and delete named graphs via Graph Store Protocol.

function ngSelectedEndpoint() {
  const sel = document.getElementById('endpoint-selector');
  return sel ? sel.value : '';
}

function ngShow(id) { document.getElementById(id).classList.remove('d-none'); }
function ngHide(id) { document.getElementById(id).classList.add('d-none'); }

function ngResetModal() {
  ngShow('ng-loading');
  ngHide('ng-error');
  ngHide('ng-table-wrapper');
  ngHide('ng-delete-result');
  document.getElementById('ng-tbody').innerHTML = '';
  document.getElementById('ng-select-all').checked = false;
  document.getElementById('ng-delete-btn').disabled = true;
  document.getElementById('ng-export-btn').disabled = true;
}

function ngUpdateDeleteButton() {
  const anyChecked = document.querySelectorAll('#ng-tbody .ng-row-check:checked').length > 0;
  document.getElementById('ng-delete-btn').disabled = !anyChecked;
}

function ngUpdateExportButton() {
  const anyChecked = document.querySelectorAll('#ng-tbody .ng-row-check:checked').length > 0;
  document.getElementById('ng-export-btn').disabled = !anyChecked;
}

function ngExportSelected() {
  const checkboxes = document.querySelectorAll('#ng-tbody .ng-row-check:checked');
  if (checkboxes.length === 0) return;
  const iris = Array.from(checkboxes).map(cb => cb.value);
  const endpoint = ngSelectedEndpoint();
  const format = document.getElementById('ng-export-format').value;
  const params = new URLSearchParams();
  params.set('endpoint', endpoint);
  params.set('format', format);
  iris.forEach(iri => params.append('graph', iri));
  const a = document.createElement('a');
  a.href = '/api/export-graphs?' + params.toString();
  a.download = '';
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}

function ngRenderTable(graphs) {
  const tbody = document.getElementById('ng-tbody');
  tbody.innerHTML = '';

  if (graphs.length === 0) {
    ngShow('ng-empty');
    return;
  }
  ngHide('ng-empty');

  graphs.forEach(g => {
    const tr = document.createElement('tr');
    const iri = g.iri || '';
    const label = g.label || '';
    const count = typeof g.tripleCount === 'number' ? g.tripleCount.toLocaleString() : '';

    tr.innerHTML = `
      <td>
        <input type="checkbox" class="form-check-input ng-row-check" value="${escapeHtml(iri)}" aria-label="Select graph">
      </td>
      <td style="max-width: 420px;">
        <span class="text-truncate d-block" title="${escapeHtml(iri)}" style="max-width: 420px;">${escapeHtml(iri)}</span>
      </td>
      <td>${escapeHtml(label)}</td>
      <td class="text-end text-muted">${escapeHtml(count)}</td>
    `;
    tbody.appendChild(tr);
  });

  // Wire individual checkboxes to update delete button and sync select-all
  tbody.querySelectorAll('.ng-row-check').forEach(cb => {
    cb.addEventListener('change', () => {
      ngUpdateDeleteButton();
      ngUpdateExportButton();
      const all = tbody.querySelectorAll('.ng-row-check');
      const checked = tbody.querySelectorAll('.ng-row-check:checked');
      document.getElementById('ng-select-all').checked = all.length === checked.length;
    });
  });
}

function ngLoadGraphs() {
  const endpoint = ngSelectedEndpoint();
  fetch(`/api/named-graphs?endpoint=${encodeURIComponent(endpoint)}`)
    .then(r => r.json())
    .then(data => {
      ngHide('ng-loading');
      ngShow('ng-table-wrapper');
      ngRenderTable(data.graphs || []);
    })
    .catch(err => {
      ngHide('ng-loading');
      const el = document.getElementById('ng-error');
      el.textContent = `Failed to load named graphs: ${err.message}`;
      ngShow('ng-error');
    });
}

async function ngDeleteSelected() {
  const checkboxes = document.querySelectorAll('#ng-tbody .ng-row-check:checked');
  if (checkboxes.length === 0) return;

  const iris = Array.from(checkboxes).map(cb => cb.value);
  const endpoint = ngSelectedEndpoint();

  const btn = document.getElementById('ng-delete-btn');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner-border spinner-border-sm me-1" role="status"></span>Deleting…';

  ngHide('ng-delete-result');

  let deleted = 0;
  let failed = 0;
  const errors = [];

  for (const iri of iris) {
    try {
      const resp = await fetch(
        `/api/named-graphs?graph=${encodeURIComponent(iri)}&endpoint=${encodeURIComponent(endpoint)}`,
        { method: 'DELETE' }
      );
      const data = await resp.json();
      if (data.success) {
        deleted++;
      } else {
        failed++;
        errors.push(`${iri}: ${data.error || 'unknown error'}`);
      }
    } catch (err) {
      failed++;
      errors.push(`${iri}: ${err.message}`);
    }
  }

  // Restore button
  btn.innerHTML = '<i data-lucide="trash-2" style="width: 1rem; height: 1rem;" class="me-1"></i>Delete named graph';
  if (window.lucide) lucide.createIcons();

  // Show result
  const resultEl = document.getElementById('ng-delete-result');
  if (failed === 0) {
    resultEl.className = 'alert alert-success m-3';
    resultEl.textContent = `${deleted} graph${deleted !== 1 ? 's' : ''} deleted successfully.`;
  } else {
    resultEl.className = 'alert alert-warning m-3';
    resultEl.innerHTML = `${deleted} deleted, ${failed} failed.<br><small>${errors.map(escapeHtml).join('<br>')}</small>`;
  }
  ngShow('ng-delete-result');

  // Reload table
  ngHide('ng-table-wrapper');
  ngShow('ng-loading');
  ngHide('ng-error');
  document.getElementById('ng-select-all').checked = false;
  ngLoadGraphs();
}

async function ngDeleteDefaultGraph() {
  const endpoint = ngSelectedEndpoint();
  const btn = document.getElementById('ng-delete-default-btn');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner-border spinner-border-sm me-1" role="status"></span>Deleting…';

  ngHide('ng-delete-result');

  try {
    const resp = await fetch(
      `/api/named-graphs?graph=default&endpoint=${encodeURIComponent(endpoint)}`,
      { method: 'DELETE' }
    );
    const data = await resp.json();
    const resultEl = document.getElementById('ng-delete-result');
    if (data.success) {
      resultEl.className = 'alert alert-success m-3';
      resultEl.textContent = 'Default graph deleted successfully.';
    } else {
      resultEl.className = 'alert alert-danger m-3';
      resultEl.textContent = `Failed to delete default graph: ${data.error || 'unknown error'}`;
    }
    ngShow('ng-delete-result');
  } catch (err) {
    const resultEl = document.getElementById('ng-delete-result');
    resultEl.className = 'alert alert-danger m-3';
    resultEl.textContent = `Network error: ${err.message}`;
    ngShow('ng-delete-result');
  }

  btn.disabled = false;
  btn.innerHTML = '<i data-lucide="trash-2" style="width: 1rem; height: 1rem;" class="me-1"></i>Delete default graph';
  if (window.lucide) lucide.createIcons();
}

function ngToggleAll(e) {
  const checked = e.target.checked;
  document.querySelectorAll('#ng-tbody .ng-row-check').forEach(cb => { cb.checked = checked; });
  ngUpdateDeleteButton();
  ngUpdateExportButton();
}

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

document.addEventListener('DOMContentLoaded', () => {
  const modal = document.getElementById('namedGraphsModal');
  if (!modal) return;

  modal.addEventListener('show.bs.modal', () => {
    ngResetModal();
    ngLoadGraphs();
  });

  document.getElementById('ng-delete-btn').addEventListener('click', ngDeleteSelected);
  document.getElementById('ng-delete-default-btn').addEventListener('click', ngDeleteDefaultGraph);
  document.getElementById('ng-select-all').addEventListener('change', ngToggleAll);
  document.getElementById('ng-export-btn').addEventListener('click', ngExportSelected);
});
