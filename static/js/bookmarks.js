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

  function addBookmark(iri, label, shortIri) {
    var items = loadBookmarks();
    if (items.some(function (b) { return b.iri === iri; })) return; // deduplicate
    items.unshift({ iri: iri, label: label || shortIri || iri, shortIri: shortIri || '' });
    saveBookmarks(items);
    renderBookmarks();
    updateBookmarkButton(iri, true);
  }

  function removeBookmark(iri) {
    var items = loadBookmarks().filter(function (b) { return b.iri !== iri; });
    saveBookmarks(items);
    renderBookmarks();
    updateBookmarkButton(iri, false);
  }

  function reorderBookmarks(fromIdx, toIdx) {
    if (fromIdx === toIdx) return;
    var items = loadBookmarks();
    var moved = items.splice(fromIdx, 1)[0];
    items.splice(toIdx, 0, moved);
    saveBookmarks(items);
    renderBookmarks();
  }

  function isBookmarked(iri) {
    return loadBookmarks().some(function (b) { return b.iri === iri; });
  }

  // ── Rendering ──────────────────────────────────────────────────────────────

  function lastSegment(iri) {
    iri = iri.replace(/\/$/, '');
    var hash = iri.lastIndexOf('#');
    if (hash !== -1 && hash < iri.length - 1) return iri.slice(hash + 1);
    var slash = iri.lastIndexOf('/');
    if (slash !== -1 && slash < iri.length - 1) return iri.slice(slash + 1);
    return iri;
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

      var iconSrc = '/static/img/resource/' + lastSegment(item.iri) + '.svg';
      var label = item.label || item.shortIri || lastSegment(item.iri);
      var href = '/resource/' + encodeURIComponent(item.iri);

      li.innerHTML =
        '<a class="nav-link pe-1" href="' + href + '" title="' + escapeHtml(item.iri) + '">' +
          '<span class="nav-link-icon d-none d-lg-inline-block me-2 flex-shrink-0">' +
            '<img src="' + iconSrc + '" alt="" width="16" height="16" ' +
              'onerror="this.style.display=\'none\'">' +
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

  // ── "Add bookmark" button on resource pages ────────────────────────────────

  function updateBookmarkButton(iri, bookmarked) {
    var btn = document.getElementById('bookmark-this-btn');
    if (!btn || btn.dataset.iri !== iri) return;
    var icon = btn.querySelector('[data-lucide]');
    if (bookmarked) {
      btn.title = 'Remove bookmark';
      btn.classList.add('active');
      if (icon) icon.setAttribute('data-lucide', 'bookmark-check');
    } else {
      btn.title = 'Bookmark this resource';
      btn.classList.remove('active');
      if (icon) icon.setAttribute('data-lucide', 'bookmark');
    }
    if (window.lucide) window.lucide.createIcons();
  }

  function injectBookmarkButton() {
    var dataEl = document.getElementById('resource-data');
    if (!dataEl) return; // not a resource page

    var data;
    try { data = JSON.parse(dataEl.textContent); } catch (e) { return; }
    var iri = data.ResourceIRI;
    if (!iri) return;

    // Find the h1 inside .iri-title-wrapper and insert button next to it
    var wrapper = document.querySelector('.iri-title-wrapper');
    if (!wrapper) return;

    var btn = document.createElement('button');
    btn.id = 'bookmark-this-btn';
    btn.dataset.iri = iri;
    btn.className = 'btn btn-sm btn-ghost-secondary ms-2 align-middle';
    btn.title = isBookmarked(iri) ? 'Remove bookmark' : 'Bookmark this resource';
    if (isBookmarked(iri)) btn.classList.add('active');
    btn.innerHTML = '<i data-lucide="' + (isBookmarked(iri) ? 'bookmark-check' : 'bookmark') +
      '" style="width:16px;height:16px;pointer-events:none;"></i>';

    btn.addEventListener('click', function () {
      var label = document.querySelector('h1') ? document.querySelector('h1').textContent.trim() : '';
      var shortIri = data.ShortIRI || '';
      if (isBookmarked(iri)) {
        removeBookmark(iri);
      } else {
        addBookmark(iri, label, shortIri);
      }
    });

    // Insert after the h1
    var h1 = wrapper.querySelector('h1');
    if (h1) {
      h1.style.display = 'inline';
      h1.insertAdjacentElement('afterend', btn);
    } else {
      wrapper.appendChild(btn);
    }

    if (window.lucide) window.lucide.createIcons();
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

        // Fall back to text/uri-list (browser native for dragged links)
        if (!iri) {
          var uriList = e.dataTransfer.getData('text/uri-list') || e.dataTransfer.getData('text/plain');
          if (uriList) {
            var lines = uriList.split(/\r?\n/).filter(function (l) { return l && !l.startsWith('#'); });
            var firstUrl = lines[0] || '';
            // Extract IRI from /resource/<iri>
            var match = firstUrl.match(/\/resource\/(.+)/);
            if (match) iri = decodeURIComponent(match[1]);
          }
        }

        if (!iri) return;
        if (!label) label = lastSegment(iri);
        addBookmark(iri, label, '');

        // Switch to bookmarks tab so user sees the result
        var bookmarksTab = document.getElementById('sidebar-tab-bookmarks');
        if (bookmarksTab) bookmarksTab.click();
      });
    });
  }

  // ── Make all resource links draggable ──────────────────────────────────────

  function initResourceLinkDrag() {
    document.addEventListener('dragstart', function (e) {
      var anchor = e.target.closest('a[href]');
      if (!anchor) return;

      var href = anchor.href || '';
      var match = href.match(/\/resource\/(.+)/);
      if (!match) return;

      var iri = decodeURIComponent(match[1]);
      var label = anchor.textContent.trim();

      e.dataTransfer.setData(DRAG_TYPE_IRI, iri);
      e.dataTransfer.setData('text/plain', label);
      // Also set uri-list so it works as a native browser link drag
      e.dataTransfer.setData('text/uri-list', href);
      e.dataTransfer.effectAllowed = 'copyMove';
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

  // ── Init ───────────────────────────────────────────────────────────────────

  document.addEventListener('DOMContentLoaded', function () {
    renderBookmarks();
    initDropZone();
    initRemoveButtons();
    initResourceLinkDrag();
    injectBookmarkButton();
  });

})();
