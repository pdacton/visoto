/* eslint-disable */
/*
  Touch gestures on a Graph Explorer canvas (GH #19). GE itself handles mouse
  events only; the mouse behaviour is untouched. Installed by graph-kit.js
  BEFORE graph-selection.js, so its capture listener sees a touch first.

  - One finger on a node: drag it, or tap to (de)select — graph-selection.js
    (touchDrag). Nodes never hand a finger to the browser (touch-action: none).
  - Two fingers anywhere on the canvas: pinch to zoom around their midpoint,
    panning with it. The non-passive touchstart cancels the browser's own
    gesture, which works only when the second finger lands before the browser
    started scrolling with the first (the Google Maps behaviour).
  - One finger on empty canvas:
      embedded card: scrolls the PAGE, so the graph never traps it (GE's
        scroller is overflow:hidden; touch-action in ontodia_overrides.css);
      fullscreen: pans the canvas; a long press without moving starts a box
        selection instead.
  - A tap on empty canvas clears the selection.

  pinchStep is pure and exported for tests (tests/js/graph-touch.test.js).
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.VisotoGraphTouch = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var TAP_SLOP = 8;      // px a finger may wander and still tap / long-press
  var LONG_PRESS = 500;  // ms, fullscreen: press-and-hold starts a box

  function distance(a, b) { return Math.hypot(b.x - a.x, b.y - a.y); }
  function midpoint(a, b) { return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }; }

  // One pinch frame from two finger positions before (prev) and now (cur),
  // each [p, q]: the zoom factor, the point to zoom around (the midpoint now)
  // and how far the midpoint moved (the pan).
  function pinchStep(prev, cur) {
    var d0 = distance(prev[0], prev[1]);
    var m0 = midpoint(prev[0], prev[1]);
    var m1 = midpoint(cur[0], cur[1]);
    return {
      ratio: d0 > 0 ? distance(cur[0], cur[1]) / d0 : 1,
      pivot: m1,
      pan: { dx: m1.x - m0.x, dy: m1.y - m0.y },
    };
  }

  function install(kit) {
    var c = kit.commands;
    var container = kit.container;
    if (!c || !container || container._visotoTouch) return;
    container._visotoTouch = true;

    function maximized() { return !!(kit.card && kit.card.classList.contains('graph-maximized')); }
    function inPaper(t) {
      return t instanceof Element && !!t.closest('.graph-explorer-paper-area') &&
        !t.closest('.graph-explorer-paper-area__widgets');
    }
    function emptyPaper(t) {
      return inPaper(t) && !t.closest('[data-element-id], .graph-explorer-link, .graph-explorer-halo');
    }

    // --- Two fingers: pinch zoom + pan (touch events: only they can cancel the
    // browser's gesture) ----------------------------------------------------
    var pinch = null;      // the two finger page positions of the last frame
    function fingers(e) {
      return [0, 1].map(function (i) { return { x: e.touches[i].pageX, y: e.touches[i].pageY }; });
    }
    container.addEventListener('touchstart', function (e) {
      if (e.touches.length < 2 || pinch) return;
      if (!inPaper(e.touches[0].target) || !inPaper(e.touches[1].target)) return;
      if (e.cancelable) e.preventDefault();
      if (cancelOneFinger) cancelOneFinger();
      if (kit.cancelTouchDrag) kit.cancelTouchDrag();
      pinch = fingers(e);
    }, { capture: true, passive: false });
    container.addEventListener('touchmove', function (e) {
      if (!pinch) return;
      if (e.cancelable) e.preventDefault();
      if (e.touches.length < 2) return;
      var cur = fingers(e);
      var step = pinchStep(pinch, cur);
      c.panBy(step.pan.dx, step.pan.dy);
      if (step.ratio !== 1) c.zoomAt(c.getScale() * step.ratio, step.pivot.x, step.pivot.y);
      pinch = cur;
    }, { capture: true, passive: false });
    function touchEnd(e) {
      if (pinch && e.touches.length < 2) pinch = null;
    }
    container.addEventListener('touchend', touchEnd, true);
    container.addEventListener('touchcancel', touchEnd, true);

    // --- One finger (pointer events) ----------------------------------------
    // Every finger after the first, and every finger during a pinch, is
    // swallowed here, so graph-selection.js never starts a drag for it.
    var down = {};         // pointerId -> true, touch pointers currently down
    var cancelOneFinger = null;
    container.addEventListener('pointerdown', function (ev) {
      if (ev.pointerType !== 'touch') return;
      var first = Object.keys(down).length === 0;
      down[ev.pointerId] = true;
      if (!first || pinch) {
        ev.preventDefault();
        ev.stopImmediatePropagation();
        return;
      }
      if (ev.button !== 0 || !emptyPaper(ev.target)) return;
      ev.preventDefault(); // no compatibility mouse events: GE would pan too
      oneFinger(ev);
    }, true);
    function lift(ev) { if (ev.pointerType === 'touch') delete down[ev.pointerId]; }
    window.addEventListener('pointerup', lift, true);
    window.addEventListener('pointercancel', lift, true);

    function oneFinger(ev) {
      var id = ev.pointerId;
      var start = { x: ev.clientX, y: ev.clientY };
      var last = start;
      var full = maximized();
      var moved = false, panning = false, pressed = false;
      var timer = full ? setTimeout(function () {
        pressed = true;
        finish();
        if (kit.boxSelect) kit.boxSelect(ev);
      }, LONG_PRESS) : null;

      function move(e) {
        if (e.pointerId !== id) return;
        if (!moved && Math.hypot(e.clientX - start.x, e.clientY - start.y) > TAP_SLOP) {
          moved = true;
          clearTimeout(timer);
          // Embedded, the browser scrolls the page with this finger.
          if (!full) { finish(); return; }
          panning = true;
        }
        if (!panning) return;
        c.panBy(e.clientX - last.x, e.clientY - last.y);
        last = { x: e.clientX, y: e.clientY };
      }
      function up(e) {
        if (e.pointerId !== id) return;
        finish();
        if (e.type === 'pointerup' && !moved && !pressed) c.setSelection([]);
      }
      function finish() {
        clearTimeout(timer);
        window.removeEventListener('pointermove', move);
        window.removeEventListener('pointerup', up);
        window.removeEventListener('pointercancel', up);
        cancelOneFinger = null;
      }
      window.addEventListener('pointermove', move);
      window.addEventListener('pointerup', up);
      window.addEventListener('pointercancel', up);
      cancelOneFinger = finish;
    }
  }

  return { install: install, pinchStep: pinchStep };
});
