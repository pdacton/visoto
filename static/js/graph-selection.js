/* eslint-disable */
/*
  Selection, pinning and selection actions for a Graph Explorer embed
  (GL-5–9, 12, 26, 51). Installed by graph-kit.js on kit.attach(); markup is the
  Pan/Select toggle and the selection bar in templates/partials/graph-toolbar.html.

  GE 2.1 selects one cell per click, ignores modifier clicks and has no box
  selection, multi-selection marking or group drag (GE-UPSTREAM: B3). So:

  - Box selection and touch dragging use a CAPTURE-phase pointerdown listener
    on the canvas container. preventDefault() on pointerdown suppresses the
    compatibility mouse events GE listens to, so GE neither pans nor drags.
  - Ctrl/Cmd+click, Select-mode taps and group drag ride on GE's own paper
    pointer events (workspace props onPointerDown/Move/Up, wired by
    kit.workspaceProps). GE's handler runs first and selects the clicked node
    alone; the handlers here then set the selection they mean.
  - Selected and pinned nodes are marked by a per-graph <style> element with
    one rule per node id: GE re-renders and remounts its DOM freely, and
    attribute selectors survive that where classes on its nodes would not.
  - Pins are Visoto state, changed only through undoable commands; a drag
    records its pin inside GE's own drag batch, so undo takes both back.
*/
(function () {
  'use strict';

  if (window.VisotoGraphSelection) return;

  var DRAG_THRESHOLD = 4;      // screen px before a press becomes a drag
  var EXPAND_LIMIT = 100;      // neighbours fetched per node for Expand all
  var CONFIRM_ADDITIONS = 20;  // GL-15

  var BADGE = 24;              // px, check and pin badges (paper coordinates)
  // Lucide "check" and "pin", white / dark stroke, as data URIs.
  var CHECK_ICON = "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='white' stroke-width='3.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M20 6 9 17l-5-5'/%3E%3C/svg%3E\")";
  var PIN_ICON = "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%23182433' stroke-width='2.2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M12 17v5'/%3E%3Cpath d='M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z'/%3E%3C/svg%3E\")";

  function quote(value) {
    return '"' + String(value).replace(/["\\]/g, '\\$&') + '"';
  }

  function install(kit) {
    var c = kit.commands;
    var id = kit.id;
    var container = kit.container;
    var stage = container.parentElement;
    var bar = document.getElementById(id + '-selectionbar');
    var toolbar = document.getElementById(id + '-toolbar');
    var pinned = {};
    var mode = 'pan';

    kit.isPinned = function (elementId) { return !!pinned[elementId]; };
    // Saved canvases carry the pins (A3); loading them is not an undo step.
    kit.pinnedIds = function () {
      return Object.keys(pinned).filter(function (eid) { return pinned[eid] && c.exists(eid); });
    };
    kit.loadPins = function (ids) {
      pinned = {};
      (ids || []).forEach(function (eid) { pinned[eid] = true; });
      decorate();
      updateBar();
    };
    kit.selectedIds = function () { return c.selectedIds(); };

    // --- Marking (GL-7, GL-26) ---------------------------------------------
    var style = document.createElement('style');
    style.setAttribute('data-graph-decor', id);
    document.head.appendChild(style);
    var scope = '#' + CSS.escape(id + '-root');

    function decorate() {
      var rules = [];
      c.selectedIds().forEach(function (eid) {
        var host = scope + ' [data-element-id=' + quote(eid) + ']';
        rules.push(host + ' .graph-explorer-standard-template{outline:2px solid var(--tblr-primary,#066fd1);outline-offset:3px}');
        rules.push(host + ' .graph-explorer-standard-template::before{content:"";position:absolute;left:-' + (BADGE / 2 + 3) + 'px;top:-' + (BADGE / 2 + 3) +
          'px;width:' + BADGE + 'px;height:' + BADGE + 'px;border-radius:50%;background:var(--tblr-primary,#066fd1) ' + CHECK_ICON +
          ' center/15px no-repeat;z-index:2;pointer-events:none}');
      });
      Object.keys(pinned).forEach(function (eid) {
        if (!pinned[eid]) return;
        var host = scope + ' [data-element-id=' + quote(eid) + ']';
        rules.push(host + '::before{content:"";position:absolute;right:-' + (BADGE / 2) + 'px;top:-' + (BADGE / 2) + 'px;width:' + BADGE +
          'px;height:' + BADGE + 'px;border-radius:50%;background:#fff ' + PIN_ICON +
          ' center/15px no-repeat;border:1px solid var(--tblr-border-color,#dce1e7);z-index:3;cursor:pointer}');
      });
      style.textContent = rules.join('\n');
    }

    // --- Pins (GL-26): undoable, title = what the user did ------------------
    function setPins(state, title) {
      return {
        title: title,
        invoke: function () {
          var before = {};
          Object.keys(state).forEach(function (eid) {
            before[eid] = !!pinned[eid];
            if (state[eid]) pinned[eid] = true; else delete pinned[eid];
          });
          decorate();
          updateBar();
          return setPins(before, title);
        },
      };
    }
    function pin(ids, value, title) {
      var state = {};
      ids.forEach(function (eid) { state[eid] = value; });
      c.history.execute(setPins(state, title || (value ? vsT('js.graph.cmd.pin', 'Pin') : vsT('js.graph.cmd.unpin', 'Unpin'))));
    }

    // --- Selection helpers ---------------------------------------------------
    function select(ids) { c.setSelection(ids); }
    function toggle(ids, eid) {
      return ids.indexOf(eid) >= 0 ? ids.filter(function (x) { return x !== eid; }) : ids.concat([eid]);
    }
    kit.clearSelection = function () { select([]); };
    kit.selectAll = function () { select(c.elementIds()); };

    function setMode(next) {
      mode = next;
      container.classList.toggle('graph-select-mode', mode === 'select');
      if (!toolbar) return;
      toolbar.querySelectorAll('[data-graph-mode]').forEach(function (btn) {
        var on = btn.getAttribute('data-graph-mode') === mode;
        btn.classList.toggle('active', on);
        btn.setAttribute('aria-pressed', String(on));
      });
    }
    kit.setMode = setMode;

    // --- GE pointer events: Ctrl/Cmd+click, Select-mode taps, group drag -----
    var press = null; // { ids: selection at press, target, last: {x,y}, moved }
    kit.onPointerDown = function (e) {
      if (!e.elementId) { press = null; return; }
      var b = c.box(e.elementId);
      press = { ids: c.selectedIds(), target: e.elementId, last: { x: b.x, y: b.y }, moved: false };
    };
    kit.onPointerMove = function (e) {
      if (!press || e.elementId !== press.target) return;
      var b = c.box(press.target);
      var dx = b.x - press.last.x, dy = b.y - press.last.y;
      if (!dx && !dy) return;
      press.moved = true;
      press.last = { x: b.x, y: b.y };
      // GL-9: dragging a selected node moves the whole selection. GE's drag
      // batch captured every position at pointer-down, so undo covers them.
      if (press.ids.length > 1 && press.ids.indexOf(press.target) >= 0) {
        var moves = {};
        press.ids.forEach(function (eid) {
          if (eid === press.target) return;
          var o = c.box(eid);
          if (o) moves[eid] = { x: o.x + dx, y: o.y + dy };
        });
        c.movePositions(moves);
      }
    };
    kit.onPointerUp = function (e) {
      var p = press;
      press = null;
      if (!p || e.elementId !== p.target) return;
      var group = p.ids.length > 1 && p.ids.indexOf(p.target) >= 0;
      if (!e.click || p.moved) {
        // Dragged: pin what moved (recorded in GE's still-open drag batch) and
        // keep the group selected (GE just selected the dragged node alone).
        pin(group ? p.ids : [p.target], true, vsT('js.graph.cmd.move', 'Move'));
        if (group) select(p.ids);
        return;
      }
      var ev = e.sourceEvent || {};
      if (ev.ctrlKey || ev.metaKey || mode === 'select') select(toggle(p.ids, p.target));
    };

    // --- Capture listener: pin badge, box selection, touch drag --------------
    function stagePoint(ev) {
      var r = stage.getBoundingClientRect();
      return { x: ev.clientX - r.left, y: ev.clientY - r.top };
    }
    function onPinBadge(host, ev) {
      var r = host.getBoundingClientRect();
      var tmpl = host.querySelector('.graph-explorer-standard-template');
      var scale = tmpl && tmpl.offsetWidth ? tmpl.getBoundingClientRect().width / tmpl.offsetWidth : 1;
      var half = (BADGE / 2 + 2) * scale;
      return pinned[host.getAttribute('data-element-id')] &&
        Math.abs(ev.clientX - r.right) <= half && Math.abs(ev.clientY - r.top) <= half;
    }

    container.addEventListener('pointerdown', function (ev) {
      if (ev.button !== 0 || !(ev.target instanceof Element)) return;
      var t = ev.target;
      var host = t.closest('[data-element-id]');

      if (host && onPinBadge(host, ev)) {
        ev.preventDefault();
        ev.stopPropagation();
        var eid = host.getAttribute('data-element-id');
        window.addEventListener('pointerup', function () { pin([eid], false); }, { once: true });
        return;
      }
      if (host && ev.pointerType === 'touch' && mode === 'select') {
        touchDrag(ev, host.getAttribute('data-element-id'));
        return;
      }
      var emptyPaper = t.closest('.graph-explorer-paper-area') &&
        !t.closest('[data-element-id], .graph-explorer-link, .graph-explorer-paper-area__widgets');
      if (emptyPaper && (mode === 'select' || ev.shiftKey)) boxSelect(ev);
    }, true);

    // GL-5: a drag on empty canvas draws a box that replaces the selection.
    function boxSelect(ev) {
      ev.preventDefault();
      ev.stopPropagation();
      var start = stagePoint(ev);
      var startPage = { x: ev.pageX, y: ev.pageY };
      var box = document.createElement('div');
      box.className = 'graph-selection-box';
      stage.appendChild(box);
      function draw(e) {
        var p = stagePoint(e);
        box.style.left = Math.min(p.x, start.x) + 'px';
        box.style.top = Math.min(p.y, start.y) + 'px';
        box.style.width = Math.abs(p.x - start.x) + 'px';
        box.style.height = Math.abs(p.y - start.y) + 'px';
      }
      function up(e) {
        window.removeEventListener('pointermove', draw);
        window.removeEventListener('pointerup', up);
        window.removeEventListener('pointercancel', up);
        box.remove();
        if (Math.abs(e.pageX - startPage.x) < DRAG_THRESHOLD && Math.abs(e.pageY - startPage.y) < DRAG_THRESHOLD) {
          select([]); // a click on empty canvas (GL-8)
          return;
        }
        var a = c.pageToPaper(Math.min(e.pageX, startPage.x), Math.min(e.pageY, startPage.y));
        var b = c.pageToPaper(Math.max(e.pageX, startPage.x), Math.max(e.pageY, startPage.y));
        select(c.elementIds().filter(function (eid) {
          var n = c.box(eid);
          return n.x < b.x && n.x + n.width > a.x && n.y < b.y && n.y + n.height > a.y;
        }));
      }
      draw(ev);
      window.addEventListener('pointermove', draw);
      window.addEventListener('pointerup', up);
      window.addEventListener('pointercancel', up);
    }

    // Touch has no mouse events for GE to drag with: in Select mode a finger
    // drags the node (or the selection it belongs to), and a tap toggles it.
    function touchDrag(ev, eid) {
      ev.preventDefault();
      ev.stopPropagation();
      var ids = c.selectedIds();
      var group = ids.indexOf(eid) >= 0 && ids.length > 1 ? ids : [eid];
      var origin = {};
      group.forEach(function (g) { var b = c.box(g); origin[g] = { x: b.x, y: b.y }; });
      var startPaper = c.pageToPaper(ev.pageX, ev.pageY);
      var startPage = { x: ev.pageX, y: ev.pageY };
      var batch = null;
      function move(e) {
        if (!batch) {
          if (Math.abs(e.pageX - startPage.x) < DRAG_THRESHOLD && Math.abs(e.pageY - startPage.y) < DRAG_THRESHOLD) return;
          batch = c.startGeometryBatch(vsT('js.graph.cmd.move', 'Move'));
        }
        var p = c.pageToPaper(e.pageX, e.pageY);
        var moves = {};
        group.forEach(function (g) {
          moves[g] = { x: origin[g].x + p.x - startPaper.x, y: origin[g].y + p.y - startPaper.y };
        });
        c.movePositions(moves);
      }
      function up() {
        window.removeEventListener('pointermove', move);
        window.removeEventListener('pointerup', up);
        window.removeEventListener('pointercancel', up);
        if (batch) {
          pin(group, true);
          batch.store();
        } else {
          select(toggle(c.selectedIds(), eid));
        }
      }
      window.addEventListener('pointermove', move);
      window.addEventListener('pointerup', up);
      window.addEventListener('pointercancel', up);
    }

    // --- Selection bar (GL-12, GL-51) ----------------------------------------
    function updateBar() {
      if (!bar) return;
      var ids = c.selectedIds();
      bar.hidden = ids.length === 0;
      var count = bar.querySelector('[data-graph-selection-count]');
      if (count) count.textContent = vsTf('js.graph.selected', '{n} selected', { n: ids.length });
      var allPinned = ids.length > 0 && ids.every(function (eid) { return pinned[eid]; });
      bar.querySelectorAll('[data-graph-action="pin-toggle"]').forEach(function (btn) {
        var label = allPinned ? vsT('js.graph.unpin', 'Unpin') : vsT('js.graph.pin', 'Pin');
        btn.title = label;
        btn.setAttribute('aria-label', label);
        btn.setAttribute('aria-pressed', String(allPinned));
        btn.classList.toggle('active', allPinned);
      });
      bar.querySelectorAll('[data-graph-needs-two]').forEach(function (btn) {
        btn.disabled = ids.length < 2;
        btn.classList.toggle('disabled', ids.length < 2);
      });
    }

    function selectedBoxes() {
      return c.selectedIds().map(c.box).filter(Boolean);
    }

    kit.fitSelection = function () {
      var boxes = selectedBoxes();
      if (!boxes.length) { kit.fit(); return; }
      var l = Infinity, t = Infinity, r = -Infinity, b = -Infinity;
      boxes.forEach(function (n) {
        l = Math.min(l, n.x); t = Math.min(t, n.y);
        r = Math.max(r, n.x + n.width); b = Math.max(b, n.y + n.height);
      });
      c.fitRect({ x: l, y: t, width: r - l, height: b - t });
    };

    function align(mode) {
      var positions = window.VisotoLayout.align(selectedBoxes(), mode);
      var ids = Object.keys(positions);
      if (!ids.length) return;
      kit.batch(vsT('js.graph.cmd.align', 'Align'), function () {
        var batch = c.startGeometryBatch();
        c.movePositions(positions);
        batch.store();
        pin(ids, true); // GL-51: aligned nodes are pinned
      });
    }

    function expandAll() {
      var ids = c.selectedIds();
      var done = kit.busy(vsT('js.graph.loadingNeighbours', 'Loading neighbours…'));
      c.neighbourIris(ids, EXPAND_LIMIT).then(function (items) {
        done();
        if (!items.length) {
          kit.showMessage(vsT('js.graph.noNeighbours', 'No further neighbours found.'), { level: 'info' });
          return;
        }
        if (items.length > CONFIRM_ADDITIONS &&
            !window.confirm(vsTf('js.graph.confirmAdd', 'Add {n} nodes to the diagram?', { n: items.length }))) {
          return;
        }
        // New nodes start on a ring around the node they came from, then are
        // placed by a Network pass with every existing node fixed (GL-14).
        var byFrom = {};
        items.forEach(function (item) { (byFrom[item.from] = byFrom[item.from] || []).push(item); });
        Object.keys(byFrom).forEach(function (from) {
          var src = c.box(from);
          var list = byFrom[from];
          list.forEach(function (item, i) {
            var a = (2 * Math.PI * i) / list.length;
            item.x = src.x + src.width / 2 + 300 * Math.cos(a) - 90;
            item.y = src.y + src.height / 2 + 300 * Math.sin(a) - 30;
          });
        });
        var added;
        kit.batch(vsT('js.graph.cmd.expandAll', 'Expand all'), function () { added = c.addElements(items); });
        return added.loaded.then(function (newIds) { return kit.placeNew(newIds); });
      }).catch(function (err) {
        done();
        if (err && err.name === 'AbortError') return;
        kit.showMessage(vsT('js.graph.endpointFailed', 'The endpoint did not answer.'), { retry: expandAll });
      });
    }

    function runAction(name) {
      var ids = c.selectedIds();
      switch (name) {
        case 'expand-all': expandAll(); break;
        case 'remove':
          kit.batch(vsT('js.graph.cmd.remove', 'Remove'), function () { c.removeElements(ids); });
          break;
        case 'keep-only':
          kit.batch(vsT('js.graph.cmd.keepOnly', 'Keep only these'), function () {
            c.removeElements(c.elementIds().filter(function (eid) { return ids.indexOf(eid) < 0; }));
          });
          break;
        case 'layout-selection': kit.layout(); break;
        case 'select-neighbours': select(ids.concat(c.neighbourIds(ids).filter(function (n) { return ids.indexOf(n) < 0; }))); break;
        case 'pin-toggle': pin(ids, !ids.every(function (eid) { return pinned[eid]; })); break;
        case 'clear-selection': select([]); break;
      }
    }

    if (bar) {
      bar.addEventListener('click', function (e) {
        var t = e.target instanceof Element ? e.target : null;
        var alignItem = t && t.closest('[data-graph-align]');
        if (alignItem) { e.preventDefault(); align(alignItem.getAttribute('data-graph-align')); return; }
        var btn = t && t.closest('[data-graph-action]');
        if (!btn || btn.disabled) return;
        e.preventDefault();
        runAction(btn.getAttribute('data-graph-action'));
      });
    }
    if (toolbar) {
      toolbar.addEventListener('click', function (e) {
        var btn = e.target instanceof Element && e.target.closest('[data-graph-mode]');
        if (!btn) return;
        e.preventDefault();
        setMode(btn.getAttribute('data-graph-mode'));
      });
    }

    c.onSelectionChange(function () { decorate(); updateBar(); });
    setMode('pan');
    decorate();
    updateBar();
  }

  window.VisotoGraphSelection = { install: install };
})();
