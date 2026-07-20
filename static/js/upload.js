// upload.js — handles the "Upload data" modal
// Wires up: file dropzone, URL input, named-graph autocomplete, and the upload submit.

(function () {
  'use strict';

  // --- Helpers ---

  // Shared resolver from endpoint-switcher.js; the endpoint param is a slug.
  function selectedEndpoint() {
    return (typeof activeEndpointSlug === 'function') ? activeEndpointSlug() : '';
  }

  function timestampPrefix() {
    // Produces e.g. "20260406T142301"
    const now = new Date();
    const pad = (n, w = 2) => String(n).padStart(w, '0');
    return (
      now.getFullYear() +
      pad(now.getMonth() + 1) +
      pad(now.getDate()) +
      'T' +
      pad(now.getHours()) +
      pad(now.getMinutes()) +
      pad(now.getSeconds())
    );
  }

  function stemFromFilename(filename) {
    return filename
      .replace(/\.[^.]+$/, '')            // strip extension
      .replace(/[^a-zA-Z0-9_-]/g, '-')   // sanitise
      .replace(/-+/g, '-')               // collapse dashes
      .replace(/^-|-$/g, '');            // trim leading/trailing dashes
  }

  function stemFromURL(rawURL) {
    try {
      const u = new URL(rawURL);
      const parts = u.pathname.replace(/\/$/, '').split('/');
      const last = parts[parts.length - 1] || u.hostname;
      return stemFromFilename(last || 'resource');
    } catch {
      return 'resource';
    }
  }

  function proposeGraphURI(stem) {
    return `https://visoto.example.org/graph/${timestampPrefix()}-${stem || 'upload'}`;
  }

  function showResult(success, message) {
    const el = document.getElementById('upload-result');
    if (!el) return;
    el.className = `alert ${success ? 'alert-success' : 'alert-danger'} mt-2`;
    el.textContent = message;
    el.classList.remove('d-none');
  }

  function clearResult() {
    const el = document.getElementById('upload-result');
    if (!el) return;
    el.classList.add('d-none');
    el.textContent = '';
  }

  function setSubmitLoading(loading) {
    const btn = document.getElementById('upload-submit-btn');
    if (!btn) return;
    btn.disabled = loading;
    btn.innerHTML = loading
      ? '<span class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>Uploading…'
      : '<i data-lucide="upload-cloud" style="width:1rem;height:1rem;" class="me-1"></i>Upload';
    if (!loading && window.lucide) lucide.createIcons();
  }

  function setOntologyTabUI(ontologiesActive) {
    const graphSection = document.getElementById('upload-graph-uri-section');
    const submitBtn = document.getElementById('upload-submit-btn');
    const ontoBtn = document.getElementById('upload-ontologies-btn');
    if (graphSection) graphSection.classList.toggle('d-none', ontologiesActive);
    if (submitBtn) submitBtn.classList.toggle('d-none', ontologiesActive);
    if (ontoBtn) ontoBtn.classList.toggle('d-none', !ontologiesActive);
  }

  function setOntologySubmitLoading(loading, label) {
    const btn = document.getElementById('upload-ontologies-btn');
    if (!btn) return;
    btn.disabled = loading;
    btn.innerHTML = loading
      ? `<span class="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>${label || 'Uploading…'}`
      : '<i data-lucide="upload-cloud" style="width:1rem;height:1rem;" class="me-1"></i>Load Selected';
    if (!loading && window.lucide) lucide.createIcons();
  }

  // --- Named graphs autocomplete ---

  function loadNamedGraphs() {
    const endpoint = selectedEndpoint();
    const datalist = document.getElementById('named-graphs-list');
    const graphInput = document.getElementById('upload-graph-uri');
    if (!datalist) return;

    datalist.innerHTML = '';
    if (graphInput) graphInput.placeholder = 'Loading existing graphs…';

    fetch(`/api/named-graphs?endpoint=${encodeURIComponent(endpoint)}`)
      .then(r => r.json())
      .then(data => {
        (data.graphs || []).map(g => g.iri || g).sort().forEach(iri => {
          const opt = document.createElement('option');
          opt.value = iri;
          datalist.appendChild(opt);
        });
        if (graphInput) graphInput.placeholder = 'Select existing or enter new graph URI';
      })
      .catch(() => {
        if (graphInput) graphInput.placeholder = 'Enter named graph URI';
      });
  }

  // --- File dropzone ---

  let selectedFile = null;

  function initDropzone() {
    const dropzone = document.getElementById('upload-dropzone');
    const fileInput = document.getElementById('upload-file-input');
    const fileNameEl = document.getElementById('upload-file-name');
    const graphInput = document.getElementById('upload-graph-uri');

    if (!dropzone || !fileInput) return;

    function handleFile(file) {
      selectedFile = file;
      if (fileNameEl) {
        fileNameEl.textContent = `Selected: ${file.name}`;
        fileNameEl.classList.remove('d-none');
      }
      if (graphInput && !graphInput.value) {
        graphInput.value = proposeGraphURI(stemFromFilename(file.name));
      } else if (graphInput) {
        graphInput.value = proposeGraphURI(stemFromFilename(file.name));
      }
      clearResult();
    }

    fileInput.addEventListener('change', () => {
      if (fileInput.files.length > 0) handleFile(fileInput.files[0]);
    });

    dropzone.addEventListener('dragover', e => {
      e.preventDefault();
      dropzone.classList.add('dropzone-dragover');
    });
    dropzone.addEventListener('dragleave', () => {
      dropzone.classList.remove('dropzone-dragover');
    });
    dropzone.addEventListener('drop', e => {
      e.preventDefault();
      dropzone.classList.remove('dropzone-dragover');
      const file = e.dataTransfer?.files?.[0];
      if (file) handleFile(file);
    });
  }

  // --- URL input ---

  function initURLInput() {
    const urlInput = document.getElementById('upload-url-input');
    const graphInput = document.getElementById('upload-graph-uri');
    if (!urlInput || !graphInput) return;

    urlInput.addEventListener('input', () => {
      const val = urlInput.value.trim();
      if (val) {
        graphInput.value = proposeGraphURI(stemFromURL(val));
      }
      clearResult();
    });
  }

  // --- Ontologies tab ---

  let ontologiesCache = null;

  function loadOntologies() {
    const container = document.getElementById('ontologies-list-container');
    if (!container) return;

    if (ontologiesCache) {
      renderOntologies(ontologiesCache);
      return;
    }

    container.innerHTML = '<div class="text-muted small">Loading ontologies…</div>';
    fetch('/api/ontologies')
      .then(r => r.json())
      .then(data => {
        ontologiesCache = data.ontologies || [];
        renderOntologies(ontologiesCache);
      })
      .catch(() => {
        container.innerHTML = '<div class="text-danger small">Failed to load ontologies.</div>';
      });
  }

  function renderOntologies(ontologies) {
    const container = document.getElementById('ontologies-list-container');
    if (!container) return;

    if (!ontologies.length) {
      container.innerHTML = '<div class="text-muted small">No ontologies configured. Add <code>[[ontologies]]</code> entries to <code>visoto.config</code>.</div>';
      return;
    }

    const rows = ontologies.map((o, i) => `
      <div class="d-flex align-items-start gap-2 py-1 border-bottom">
        <input class="form-check-input mt-1 flex-shrink-0 ontology-checkbox" type="checkbox"
               id="onto-${i}" data-url="${escapeAttr(o.url)}" data-graph="${escapeAttr(o.graph)}">
        <label class="form-check-label w-100" for="onto-${i}" style="cursor:pointer;">
          <span class="fw-semibold">${escapeHTML(o.name)}</span>
          <span class="text-muted small d-block text-truncate" title="${escapeAttr(o.url)}">${escapeHTML(o.url)}</span>
          <span class="badge bg-secondary-lt font-monospace small">${escapeHTML(o.graph)}</span>
        </label>
      </div>`).join('');

    container.innerHTML = `<div style="max-height:220px;overflow-y:auto;">${rows}</div>`;

    if (window.lucide) lucide.createIcons();
  }

  function escapeHTML(str) {
    return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function escapeAttr(str) {
    return String(str).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  async function uploadSelectedOntologies() {
    const checkboxes = document.querySelectorAll('.ontology-checkbox:checked');
    if (!checkboxes.length) {
      showResult(false, 'Please select at least one ontology.');
      return;
    }
    clearResult();

    const endpoint = selectedEndpoint();
    const total = checkboxes.length;
    let done = 0;
    const errors = [];

    for (const cb of checkboxes) {
      const url = cb.dataset.url;
      const graph = cb.dataset.graph;
      setOntologySubmitLoading(true, `${done + 1} / ${total}…`);

      const formData = new FormData();
      formData.append('endpoint', endpoint);
      formData.append('url', url);
      formData.append('graphURI', graph);

      try {
        const resp = await fetch('/api/upload', { method: 'POST', body: formData });
        const data = await resp.json();
        if (!data.success) errors.push(`${cb.dataset.url}: ${data.error || 'failed'}`);
      } catch (err) {
        errors.push(`${url}: ${err.message}`);
      }
      done++;
    }

    setOntologySubmitLoading(false);
    if (errors.length) {
      showResult(false, `${done - errors.length}/${total} loaded. Errors:\n${errors.join('\n')}`);
    } else {
      showResult(true, `${total} ontolog${total === 1 ? 'y' : 'ies'} loaded successfully.`);
    }
  }

  // --- Submit ---

  function initSubmit() {
    const btn = document.getElementById('upload-submit-btn');
    if (!btn) return;

    btn.addEventListener('click', async () => {
      clearResult();

      const graphURI = document.getElementById('upload-graph-uri')?.value?.trim();
      if (!graphURI) {
        showResult(false, 'Please enter a named graph URI.');
        return;
      }

      // Determine active tab
      const fileTabActive = document.getElementById('tab-file')?.classList.contains('active');
      const urlInput = document.getElementById('upload-url-input');
      const remoteURL = urlInput?.value?.trim();

      if (fileTabActive) {
        if (!selectedFile) {
          showResult(false, 'Please select a file to upload.');
          return;
        }
      } else {
        if (!remoteURL) {
          showResult(false, 'Please enter a URL to fetch.');
          return;
        }
      }

      const formData = new FormData();
      formData.append('endpoint', selectedEndpoint());
      formData.append('graphURI', graphURI);

      if (fileTabActive) {
        formData.append('file', selectedFile);
      } else {
        formData.append('url', remoteURL);
      }

      setSubmitLoading(true);
      try {
        const resp = await fetch('/api/upload', { method: 'POST', body: formData });
        const data = await resp.json();
        if (data.success) {
          showResult(true, `Upload successful. Data loaded into <${graphURI}>.`);
        } else {
          showResult(false, data.error || 'Upload failed.');
        }
      } catch (err) {
        showResult(false, `Network error: ${err.message}`);
      } finally {
        setSubmitLoading(false);
      }
    });
  }

  // --- Modal lifecycle ---

  function resetModal() {
    selectedFile = null;
    ontologiesCache = null;
    const fileNameEl = document.getElementById('upload-file-name');
    if (fileNameEl) { fileNameEl.classList.add('d-none'); fileNameEl.textContent = ''; }
    const fileInput = document.getElementById('upload-file-input');
    if (fileInput) fileInput.value = '';
    const urlInput = document.getElementById('upload-url-input');
    if (urlInput) urlInput.value = '';
    const graphInput = document.getElementById('upload-graph-uri');
    if (graphInput) graphInput.value = '';
    clearResult();
    setOntologyTabUI(false);
    // Switch back to File tab
    const fileTabBtn = document.getElementById('tab-file-btn');
    if (fileTabBtn) fileTabBtn.click();
  }

  document.addEventListener('DOMContentLoaded', () => {
    initDropzone();
    initURLInput();
    initSubmit();

    const modal = document.getElementById('uploadDataModal');
    if (modal) {
      modal.addEventListener('show.bs.modal', () => {
        resetModal();
        loadNamedGraphs();
      });
    }

    const ontoTabBtn = document.getElementById('tab-ontologies-btn');
    if (ontoTabBtn) {
      ontoTabBtn.addEventListener('shown.bs.tab', () => {
        loadOntologies();
        setOntologyTabUI(true);
      });
    }

    // Switch back to file/url UI when leaving ontologies tab
    document.querySelectorAll('#tab-file-btn, #tab-url-btn').forEach(btn => {
      btn.addEventListener('shown.bs.tab', () => setOntologyTabUI(false));
    });

    const ontoBtn = document.getElementById('upload-ontologies-btn');
    if (ontoBtn) ontoBtn.addEventListener('click', uploadSelectedOntologies);
  });
})();
