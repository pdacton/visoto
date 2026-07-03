// bookmarks.js
// Manages a personal bookmarks list in the left sidebar.
// - Resources can be added via the "Add" button on resource pages or by
//   dragging any /resource/… link into the sidebar drop zone.
// - Items can be reordered by dragging within the list.
// - Items can be removed with the × button.
// - All state persists in localStorage under 'visoto-bookmarks'.

(function () {
  var STORAGE_KEY = 'visoto-bookmarks';
  var DRAG_TYPE_IRI = 'text/visoto-iri';
  var DRAG_TYPE_ICON = 'text/visoto-icon';
  var DRAG_TYPE_IDX = 'text/visoto-bookmark-index';

  // ── Storage ────────────────────────────────────────────────────────────────

  function loadBookmarks() {
    try {
      return JSON.parse(localStorage.getItem(STORAGE_KEY)) || [];
    } catch (e) {
      return [];
    }
  }

  function saveBookmarks(items) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
    } catch (e) {}
  }

  // ── Data operations ────────────────────────────────────────────────────────

  function addBookmark(iri, label, shortIri, icon) {
    var items = loadBookmarks();
    if (items.some(function (b) { return b.iri === iri; })) return; // deduplicate
    items.unshift({ iri: iri, label: label || shortIri || iri, shortIri: shortIri || '', icon: icon || '' });
    saveBookmarks(items);
    renderBookmarks();
  }

  function removeBookmark(iri) {
    var items = loadBookmarks().filter(function (b) { return b.iri !== iri; });
    saveBookmarks(items);
    renderBookmarks();
  }

  function reorderBookmarks(fromIdx, toIdx) {
    if (fromIdx === toIdx) return;
    var items = loadBookmarks();
    var moved = items.splice(fromIdx, 1)[0];
    items.splice(toIdx, 0, moved);
    saveBookmarks(items);
    renderBookmarks();
  }

  // ── Rendering ──────────────────────────────────────────────────────────────

  // Extract the underlying RDF IRI from a Visoto resource link, independent of URL format.
  // Handles both /resource/<encoded-iri> (path form) and /resource?iri=<encoded-iri> (query form).
  function extractIri(hrefStr) {
    var u;
    try { u = new URL(hrefStr, location.origin); } catch (e) { return null; }
    var q = u.searchParams.get('iri');
    if (q) return q;                                 // URL API already decodes query values
    var m = u.pathname.match(/\/resource\/(.+)/);
    return m ? decodeURIComponent(m[1]) : null;
  }

  function lastSegment(iri) {
    iri = iri.replace(/\/$/, '');
    var hash = iri.lastIndexOf('#');
    var name = (hash !== -1 && hash < iri.length - 1) ? iri.slice(hash + 1) : null;
    if (!name) {
      var slash = iri.lastIndexOf('/');
      name = (slash !== -1 && slash < iri.length - 1) ? iri.slice(slash + 1) : iri;
    }
    // Strip CURIE prefix (e.g. "rdf:Property" → "Property")
    if (name.includes(':')) name = name.split(':').pop();
    return name;
  }

  function renderBookmarks() {
    var list = document.getElementById('bookmarks-list');
    var dropZone = document.getElementById('bookmarks-drop-zone');
    if (!list) return;

    var items = loadBookmarks();
    list.innerHTML = '';

    if (items.length === 0) {
      if (dropZone) dropZone.style.display = '';
      return;
    }
    if (dropZone) dropZone.style.display = '';

    items.forEach(function (item, idx) {
      var li = document.createElement('li');
      li.className = 'nav-item bookmark-item';
      li.draggable = true;
      li.dataset.idx = idx;
      li.dataset.iri = item.iri;

      // Bookmarks reference instances, not classes. Icons are keyed by RDF type
      // (Person.svg, Organization.svg, …), which we know only if it was captured
      // when the bookmark was created; otherwise use the generic instance icon.
      // Never derive the icon from the instance IRI's last segment — that yields
      // a bare identifier (e.g. "5359.svg") that never exists and 404s.
      var iconSrc = item.icon || '/static/img/resource/defaultInstance.svg';
      var label = item.label || item.shortIri || lastSegment(item.iri);
      var href = '/resource/' + encodeURIComponent(item.iri);

      li.innerHTML =
        '<a class="nav-link pe-1" href="' + href + '" title="' + escapeHtml(item.iri) + '">' +
          '<span class="nav-link-icon d-none d-lg-inline-block me-2 flex-shrink-0">' +
            '<img src="' + iconSrc + '" alt="" width="16" height="16" ' +
              'onerror="this.onerror=null;this.src=\'/static/img/resource/defaultInstance.svg\'">' +
          '</span>' +
          '<span class="nav-link-title text-truncate">' + escapeHtml(label) + '</span>' +
        '</a>' +
        '<button class="btn btn-sm btn-ghost-secondary bookmark-remove ms-auto px-1 py-0 flex-shrink-0" ' +
          'data-iri="' + escapeHtml(item.iri) + '" title="Remove bookmark" ' +
          'style="opacity:0.5;">' +
          '<i data-lucide="x" style="width:12px;height:12px;pointer-events:none;"></i>' +
        '</button>';

      // Drag to reorder
      li.addEventListener('dragstart', function (e) {
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData(DRAG_TYPE_IDX, String(idx));
        li.classList.add('opacity-50');
      });
      li.addEventListener('dragend', function () {
        li.classList.remove('opacity-50');
        clearDropIndicators();
      });
      li.addEventListener('dragover', function (e) {
        if (!e.dataTransfer.types.includes(DRAG_TYPE_IDX)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        clearDropIndicators();
        li.classList.add('bookmark-drop-above');
      });
      li.addEventListener('drop', function (e) {
        var fromIdx = parseInt(e.dataTransfer.getData(DRAG_TYPE_IDX), 10);
        if (!isNaN(fromIdx) && fromIdx !== idx) {
          reorderBookmarks(fromIdx, idx);
        }
        clearDropIndicators();
      });

      list.appendChild(li);
    });

    // Re-init Lucide icons for the newly injected remove buttons
    if (window.lucide) window.lucide.createIcons();
  }

  function clearDropIndicators() {
    document.querySelectorAll('.bookmark-drop-above').forEach(function (el) {
      el.classList.remove('bookmark-drop-above');
    });
  }

  function escapeHtml(str) {
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  // ── Drop zone (drop a /resource/… link from the page) ─────────────────────

  function initDropZone() {
    var dropZone = document.getElementById('bookmarks-drop-zone');
    var list = document.getElementById('bookmarks-list');
    if (!dropZone) return;

    [dropZone, list].forEach(function (target) {
      if (!target) return;

      target.addEventListener('dragover', function (e) {
        // Accept drops from external resource links (not bookmark reorders)
        if (e.dataTransfer.types.includes(DRAG_TYPE_IRI) ||
            e.dataTransfer.types.includes('text/uri-list') ||
            e.dataTransfer.types.includes('text/plain')) {
          if (!e.dataTransfer.types.includes(DRAG_TYPE_IDX)) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'copy';
            dropZone.classList.add('bookmarks-drop-zone--over');
          }
        }
      });

      target.addEventListener('dragleave', function () {
        dropZone.classList.remove('bookmarks-drop-zone--over');
      });

      target.addEventListener('drop', function (e) {
        dropZone.classList.remove('bookmarks-drop-zone--over');
        if (e.dataTransfer.types.includes(DRAG_TYPE_IDX)) return; // reorder handled by item

        e.preventDefault();

        var iri = e.dataTransfer.getData(DRAG_TYPE_IRI);
        var label = e.dataTransfer.getData('text/plain') || '';
        var icon = e.dataTransfer.getData(DRAG_TYPE_ICON) || '';

        // Fall back to text/uri-list (browser native for dragged links)
        if (!iri) {
          var uriList = e.dataTransfer.getData('text/uri-list') || e.dataTransfer.getData('text/plain');
          if (uriList) {
            var lines = uriList.split(/\r?\n/).filter(function (l) { return l && !l.startsWith('#'); });
            var firstUrl = lines[0] || '';
            iri = extractIri(firstUrl);
          }
        }

        if (!iri) return;
        if (!label) label = lastSegment(iri);
        addBookmark(iri, label, '', icon);

        // Switch to bookmarks tab so user sees the result
        var bookmarksTab = document.getElementById('sidebar-tab-bookmarks');
        if (bookmarksTab) bookmarksTab.click();
      });
    });
  }

  // ── Make all resource links draggable ──────────────────────────────────────

  // Populate the drag payload so a resource can be dropped onto the bookmarks
  // sidebar or the Graph Explorer canvas. `href` is the native /resource/ link
  // (may be omitted, e.g. when dragging the page title, which has no anchor).
  function setDragPayload(e, iri, label, href, icon) {
    e.dataTransfer.setData(DRAG_TYPE_IRI, iri);
    e.dataTransfer.setData('text/plain', label || '');
    // Carry the type icon (e.g. .../Person.svg) so a bookmark drop can show it.
    if (icon) e.dataTransfer.setData(DRAG_TYPE_ICON, icon);
    // Graph Explorer canvas reads this key first; give it the clean RDF IRI
    // so dropped nodes resolve their real label instead of the /resource/ URL.
    e.dataTransfer.setData('application/x-graph-explorer-elements', JSON.stringify([iri]));
    // Also set uri-list so it works as a native browser link drag
    e.dataTransfer.setData('text/uri-list', href || ('/resource/' + encodeURIComponent(iri)));
    e.dataTransfer.effectAllowed = 'copyMove';
  }

  // Look for a class/type icon rendered near a dragged resource link (e.g. the
  // Person.svg / Organization.svg image that SPARQL tables place next to each
  // row). Returns the icon URL if it points at a real type icon, else ''.
  function findTypeIcon(anchor) {
    var row = anchor.closest('tr, li, .card, td') || anchor.parentElement;
    var img = anchor.querySelector && anchor.querySelector('img[src*="/img/resource/"]');
    if (!img && row) img = row.querySelector('img[src*="/img/resource/"]');
    if (!img) return '';
    var src = img.getAttribute('src') || '';
    // Ignore the generic fallbacks — they carry no type information.
    if (/\/(default|defaultClass|defaultInstance|standard)\.svg$/.test(src)) return '';
    return src;
  }

  function initResourceLinkDrag() {
    document.addEventListener('dragstart', function (e) {
      // Draggable page title on resource pages (see initTitleDrag).
      var title = e.target.closest('[data-drag-iri]');
      if (title) {
        setDragPayload(e, title.dataset.dragIri, title.textContent.trim());
        return;
      }

      var anchor = e.target.closest('a[href]');
      if (!anchor) return;

      var href = anchor.href || '';
      var iri = extractIri(href);
      if (!iri) return;

      setDragPayload(e, iri, anchor.textContent.trim(), href, findTypeIcon(anchor));
    });
  }

  // ── Remove button (event delegation on list) ───────────────────────────────

  function initRemoveButtons() {
    var list = document.getElementById('bookmarks-list');
    if (!list) return;
    list.addEventListener('click', function (e) {
      var btn = e.target.closest('.bookmark-remove');
      if (!btn) return;
      e.preventDefault();
      e.stopPropagation();
      removeBookmark(btn.dataset.iri);
    });
  }

  // ── Make the resource-page title a drag handle ─────────────────────────────

  // On a resource page, make the <h1> title draggable so the resource can be
  // dragged onto the bookmarks sidebar or the Graph Explorer canvas. The clean
  // IRI is read from the IRI dropdown link in the title header (see header.html),
  // which is always present on resource pages.
  function initTitleDrag() {
    var wrapper = document.querySelector('.iri-title-wrapper');
    if (!wrapper) return;
    var h1 = wrapper.querySelector('h1');
    if (!h1) return;

    // The dropdown link points at the raw resource IRI; fall back to a copy button.
    var iriLink = wrapper.querySelector('.iri-dropdown a[href]');
    var copyBtn = wrapper.querySelector('[data-copy]');
    var iri = iriLink ? iriLink.getAttribute('href')
            : copyBtn ? copyBtn.getAttribute('data-copy')
            : null;
    if (!iri) return;

    h1.draggable = true;
    h1.dataset.dragIri = iri;
    h1.style.cursor = 'grab';
    h1.title = 'Drag to bookmark or graph';
  }

  // ── Init ───────────────────────────────────────────────────────────────────

  document.addEventListener('DOMContentLoaded', function () {
    renderBookmarks();
    initDropZone();
    initRemoveButtons();
    initResourceLinkDrag();
    initTitleDrag();
  });

})();
