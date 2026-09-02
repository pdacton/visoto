/* eslint-disable */
/*
  Collapse toggle for the split layout (.vs-split, see tabler_overrides.css).

  A page with a navigation column beside its content — the SKOS Concept and
  ConceptScheme pages put a hierarchy tree there — sometimes wants the full width
  for reading. This folds the aside away and gives the main column everything,
  then remembers the choice so it survives navigating to the next concept.

  Persisting matters more here than it looks: the tree is a navigator, so every
  click is a page load. Without persistence the aside would reappear on every
  single navigation, undoing the user's choice immediately.

  The divider between the columns is draggable, and that width persists too.

  Markup contract:
    .vs-split                 the row
    .vs-split-aside           the navigation column
    .vs-split-handle          the divider between them (optional)
    .vs-split-main            the content column
    [data-vs-split-toggle]    button inside the aside that collapses it
    [data-vs-split-reopen]    button outside the aside that brings it back

  Layout lives in CSS: this file only sets --vs-split-width and toggles a class,
  so nothing here needs to know the breakpoint or the column fractions.
*/
(function () {
  'use strict';

  // Distinct from visoto-sidebar-tab, which belongs to the app's left nav — a
  // different thing entirely despite the similar shape.
  var STORAGE_KEY = 'visoto-split-collapsed';
  var WIDTH_KEY = 'visoto-split-width';
  var COLLAPSED_CLASS = 'vs-split-collapsed';
  var DRAGGING_CLASS = 'vs-split-dragging';

  // The aside is a navigator: too narrow and the labels are unreadable, too wide
  // and it crowds out what it is navigating to. Percentages so the split survives
  // a window resize.
  var MIN_PCT = 15;
  var MAX_PCT = 60;

  function readCollapsed() {
    try {
      return localStorage.getItem(STORAGE_KEY) === '1';
    } catch (e) {
      return false;   // private mode, blocked storage: default to showing the aside
    }
  }

  function writeCollapsed(collapsed) {
    try {
      if (collapsed) localStorage.setItem(STORAGE_KEY, '1');
      else localStorage.removeItem(STORAGE_KEY);
    } catch (e) { /* the preference is a convenience, never worth breaking a page */ }
  }

  function readWidth() {
    try {
      var raw = parseFloat(localStorage.getItem(WIDTH_KEY));
      if (!isFinite(raw)) return null;
      return clamp(raw);
    } catch (e) {
      return null;
    }
  }

  function writeWidth(pct) {
    try {
      localStorage.setItem(WIDTH_KEY, String(Math.round(pct * 100) / 100));
    } catch (e) { /* the preference is a convenience, never worth breaking a page */ }
  }

  function clamp(pct) {
    return Math.min(MAX_PCT, Math.max(MIN_PCT, pct));
  }

  function applyWidth(split, pct) {
    split.style.setProperty('--vs-split-width', pct + '%');
    var handle = split.querySelector('.vs-split-handle');
    if (handle) handle.setAttribute('aria-valuenow', String(Math.round(pct)));
  }

  function apply(split, collapsed) {
    split.classList.toggle(COLLAPSED_CLASS, collapsed);

    // aria-expanded lives on the control, not the region, so both buttons stay
    // truthful about what they do.
    var toggle = split.querySelector('[data-vs-split-toggle]');
    if (toggle) toggle.setAttribute('aria-expanded', String(!collapsed));

    // Wunderbaum measures its viewport on init and does not watch for resizes, so
    // the tree keeps its old width after the columns change. Nudge every tree on
    // the page to re-measure.
    window.dispatchEvent(new Event('resize'));
  }

  // --- Drag to resize ------------------------------------------------------
  //
  // Pointer events rather than mouse+touch pairs: one code path for mouse, pen and
  // touch, and setPointerCapture keeps the drag alive when the pointer outruns the
  // 12px handle, which it will.
  function initHandle(split) {
    var handle = split.querySelector('.vs-split-handle');
    if (!handle) return;

    var dragging = false;

    function pctFromClientX(clientX) {
      var rect = split.getBoundingClientRect();
      if (!rect.width) return null;
      return clamp(((clientX - rect.left) / rect.width) * 100);
    }

    handle.addEventListener('pointerdown', function (e) {
      // Primary button only: a right-click or a two-finger tap is not a drag.
      if (e.button !== 0) return;
      dragging = true;
      split.classList.add(DRAGGING_CLASS);
      handle.setPointerCapture(e.pointerId);
      e.preventDefault();   // no text selection, no native drag
    });

    handle.addEventListener('pointermove', function (e) {
      if (!dragging) return;
      var pct = pctFromClientX(e.clientX);
      if (pct !== null) applyWidth(split, pct);
    });

    function endDrag(e) {
      if (!dragging) return;
      dragging = false;
      split.classList.remove(DRAGGING_CLASS);
      if (handle.hasPointerCapture && handle.hasPointerCapture(e.pointerId)) {
        handle.releasePointerCapture(e.pointerId);
      }
      var pct = pctFromClientX(e.clientX);
      if (pct !== null) writeWidth(pct);
      // The tree measures its viewport once and does not watch for resizes, so it
      // keeps the old width until something tells it to look again.
      window.dispatchEvent(new Event('resize'));
    }

    handle.addEventListener('pointerup', endDrag);
    handle.addEventListener('pointercancel', endDrag);

    // Keyboard: the handle is a real control, so it has to be operable without a
    // pointer. Arrows nudge, Home/End go to the limits.
    handle.addEventListener('keydown', function (e) {
      var current = parseFloat(getComputedStyle(split).getPropertyValue('--vs-split-width')) || 33.333;
      var step = e.shiftKey ? 10 : 2;
      var next = null;
      switch (e.key) {
        case 'ArrowLeft':  next = current - step; break;
        case 'ArrowRight': next = current + step; break;
        case 'Home':       next = MIN_PCT; break;
        case 'End':        next = MAX_PCT; break;
        default: return;
      }
      e.preventDefault();
      next = clamp(next);
      applyWidth(split, next);
      writeWidth(next);
      window.dispatchEvent(new Event('resize'));
    });

    // Double-click resets to the default, the usual escape hatch from a split
    // dragged somewhere unhelpful.
    handle.addEventListener('dblclick', function () {
      split.style.removeProperty('--vs-split-width');
      try { localStorage.removeItem(WIDTH_KEY); } catch (err) { /* ignore */ }
      handle.setAttribute('aria-valuenow', '33');
      window.dispatchEvent(new Event('resize'));
    });
  }

  function initSplit(split) {
    apply(split, readCollapsed());

    var storedWidth = readWidth();
    if (storedWidth !== null) applyWidth(split, storedWidth);

    initHandle(split);

    var toggle = split.querySelector('[data-vs-split-toggle]');
    if (toggle) {
      toggle.addEventListener('click', function (e) {
        e.preventDefault();
        writeCollapsed(true);
        apply(split, true);
      });
    }

    var reopen = split.querySelector('[data-vs-split-reopen]');
    if (reopen) {
      reopen.addEventListener('click', function (e) {
        e.preventDefault();
        writeCollapsed(false);
        apply(split, false);
      });
    }
  }

  // Re-entrant on purpose, matching the other behaviour scripts: a duplicate
  // <script src> tag, or this file re-executing inside an HTMX-swapped fragment,
  // must still pick up elements that were not in the DOM the first time. No
  // module-level latch — only the per-element guard below.
  function boot() {
    document.querySelectorAll('.vs-split').forEach(function (split) {
      if (split.__visotoSplitInit) return;
      split.__visotoSplitInit = true;
      initSplit(split);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
