/* eslint-disable */
/*
  Graph layout for the Graph Explorer embeds: pure functions over a plain graph,
  no GE and no DOM, so it survives a change of diagram library and can be tested
  with `node --test tests/js/`.

    graph   { nodes: [{ id, x, y, width, height, fixed }],
              edges: [{ source, target, type }] }        (x/y = top-left corner)
    options { algorithm: 'network' | 'tree-down' | 'tree-right' | 'radial',
              selection: [id, …] | null,   lay out only these, keep the rest
              centre: id | null,           Radial's centre (GL-19)
              reversed: { <type IRI>: true } }  edges flipped for Tree ↓ / →
    deps    { force(nodes, links, linkLength),  WebCola step, from ge-adapter.js
              elk(): Promise<ELK instance>,     lazy elkjs, for the trees
              signal: AbortSignal }             checked between components

  layout() resolves to { <id>: { x, y } } for every node it moved.

  Rules (see .project/todo/graph-layout.md):
    GL-13  a selection is laid out alone; its centroid stays where it was.
    GL-16  node positions only; edges are drawn straight (the caller clears
           link vertices).
    GL-17  connected components are laid out one by one and packed in rows,
           largest first, never scattered.
    GL-19  Radial: rings by hop distance from the centre, each subtree an
           angular wedge sized by its node count.
    GL-22  reversed edge types only change which end a tree puts on top.
    GL-26  fixed nodes keep their position; their component is laid out around
           them and is not moved by packing.
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.VisotoLayout = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var ALGORITHMS = ['network', 'tree-down', 'tree-right', 'radial'];

  // GL-22: superclasses, broader concepts and wholes on top.
  var DEFAULT_REVERSED = {
    'http://www.w3.org/1999/02/22-rdf-syntax-ns#type': true,
    'http://www.w3.org/2000/01/rdf-schema#subClassOf': true,
    'http://www.w3.org/2004/02/skos/core#broader': true,
    'http://schema.org/isPartOf': true,
    'https://schema.org/isPartOf': true,
  };

  var DEFAULT_SIZE = { width: 180, height: 60 }; // a node GE has not measured yet
  var COMPONENT_GAP = 80;   // between packed components
  var PACK_ASPECT = 1.6;    // target width:height of the packed rows
  var RING_SPACING = 30;    // min gap between neighbours on a Radial ring

  function abortError() {
    var e = new Error('Layout cancelled');
    e.name = 'AbortError';
    return e;
  }
  function checkAbort(signal) {
    if (signal && signal.aborted) throw abortError();
  }

  function sizeOf(n) {
    return {
      width: n.width > 0 ? n.width : DEFAULT_SIZE.width,
      height: n.height > 0 ? n.height : DEFAULT_SIZE.height,
    };
  }

  function bounds(nodes, pos) {
    var b = { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity };
    nodes.forEach(function (n) {
      var p = pos ? pos[n.id] : n;
      var s = sizeOf(n);
      b.left = Math.min(b.left, p.x);
      b.top = Math.min(b.top, p.y);
      b.right = Math.max(b.right, p.x + s.width);
      b.bottom = Math.max(b.bottom, p.y + s.height);
    });
    return b;
  }

  function centroid(nodes, pos) {
    var x = 0, y = 0;
    nodes.forEach(function (n) {
      var p = pos ? pos[n.id] : n;
      var s = sizeOf(n);
      x += p.x + s.width / 2;
      y += p.y + s.height / 2;
    });
    return { x: x / nodes.length, y: y / nodes.length };
  }

  function translate(ids, pos, dx, dy) {
    ids.forEach(function (id) { pos[id] = { x: pos[id].x + dx, y: pos[id].y + dy }; });
  }

  // Edges between two distinct nodes of `ids`, reversed where asked.
  function edgesWithin(edges, idSet, reversed) {
    var out = [];
    edges.forEach(function (e) {
      if (e.source === e.target || !idSet[e.source] || !idSet[e.target]) return;
      out.push(reversed && reversed[e.type]
        ? { source: e.target, target: e.source, type: e.type }
        : { source: e.source, target: e.target, type: e.type });
    });
    return out;
  }

  // Connected components (edge direction ignored), in node input order.
  function components(nodes, edges) {
    var adj = {};
    nodes.forEach(function (n) { adj[n.id] = []; });
    edges.forEach(function (e) { adj[e.source].push(e.target); adj[e.target].push(e.source); });
    var byId = {};
    nodes.forEach(function (n) { byId[n.id] = n; });
    var seen = {};
    var out = [];
    nodes.forEach(function (start) {
      if (seen[start.id]) return;
      seen[start.id] = true;
      var comp = [start];
      for (var i = 0; i < comp.length; i++) {
        adj[comp[i].id].forEach(function (id) {
          if (!seen[id]) { seen[id] = true; comp.push(byId[id]); }
        });
      }
      out.push(comp);
    });
    return out;
  }

  function degrees(nodes, edges) {
    var d = {};
    nodes.forEach(function (n) { d[n.id] = 0; });
    edges.forEach(function (e) { d[e.source]++; d[e.target]++; });
    return d;
  }

  // --- Network (WebCola through deps.force) -----------------------------------
  // Link length follows the box size, so large expanded cards get room and
  // compact nodes are not flung apart (GE uses a flat 200).
  function linkLengthFor(nodes) {
    var sum = 0;
    nodes.forEach(function (n) { var s = sizeOf(n); sum += Math.max(s.width, s.height); });
    return Math.max(120, Math.min(320, (sum / nodes.length) * 1.1));
  }

  function network(nodes, edges, deps) {
    var colaNodes = nodes.map(function (n) {
      var s = sizeOf(n);
      return { id: n.id, x: n.x, y: n.y, width: s.width, height: s.height, fixed: n.fixed ? 1 : 0 };
    });
    var byId = {};
    colaNodes.forEach(function (n) { byId[n.id] = n; });
    var links = edges.map(function (e) { return { source: byId[e.source], target: byId[e.target] }; });
    deps.force(colaNodes, links, linkLengthFor(nodes));
    var pos = {};
    colaNodes.forEach(function (n) { pos[n.id] = { x: n.x, y: n.y }; });
    return Promise.resolve(pos);
  }

  // --- Trees (ELK layered) -----------------------------------------------------
  function tree(nodes, edges, direction, deps) {
    return deps.elk().then(function (elk) {
      return elk.layout({
        id: 'root',
        layoutOptions: {
          'elk.algorithm': 'layered',
          'elk.direction': direction,
          'elk.spacing.nodeNode': '40',
          'elk.layered.spacing.nodeNodeBetweenLayers': '80',
          'elk.layered.spacing.edgeNodeBetweenLayers': '20',
          // Components are packed by pack() below, not by ELK.
          'elk.separateConnectedComponents': 'false',
        },
        children: nodes.map(function (n) {
          var s = sizeOf(n);
          return { id: n.id, width: s.width, height: s.height };
        }),
        edges: edges.map(function (e, i) {
          return { id: 'e' + i, sources: [e.source], targets: [e.target] };
        }),
      });
    }).then(function (result) {
      var pos = {};
      result.children.forEach(function (c) { pos[c.id] = { x: c.x, y: c.y }; });
      return pos;
    });
  }

  // --- Radial (GL-19) ----------------------------------------------------------
  // A BFS tree from the centre. Every node gets a wedge of its parent's wedge in
  // proportion to its subtree's node count, and sits at the middle of it; ring
  // radii grow until each node's box fits into its wedge's arc. Edges that are
  // not tree edges are drawn but do not place anything.
  function radial(nodes, edges, centreId) {
    var adj = {};
    nodes.forEach(function (n) { adj[n.id] = []; });
    edges.forEach(function (e) { adj[e.source].push(e.target); adj[e.target].push(e.source); });
    var byId = {};
    nodes.forEach(function (n) { byId[n.id] = n; });

    var depth = {}, children = {}, order = [centreId];
    depth[centreId] = 0;
    for (var i = 0; i < order.length; i++) {
      var id = order[i];
      children[id] = [];
      adj[id].forEach(function (nb) {
        if (depth[nb] === undefined) {
          depth[nb] = depth[id] + 1;
          children[id].push(nb);
          order.push(nb);
        }
      });
    }
    var weight = {};
    for (var j = order.length - 1; j >= 0; j--) {
      var w = 1;
      children[order[j]].forEach(function (c) { w += weight[c]; });
      weight[order[j]] = w;
    }

    var angle = {}, wedge = {};
    angle[centreId] = 0;
    wedge[centreId] = 2 * Math.PI;
    order.forEach(function (id) {
      var kids = children[id];
      if (!kids.length) return;
      var total = 0;
      kids.forEach(function (c) { total += weight[c]; });
      var span = wedge[id];
      var start = angle[id] - span / 2;
      kids.forEach(function (c) {
        var share = span * weight[c] / total;
        wedge[c] = share;
        angle[c] = start + share / 2;
        start += share;
      });
    });

    // Radius per ring: at least one ring gap beyond the previous ring, and
    // large enough that every node's box fits the arc of its own wedge.
    var maxSide = 0;
    nodes.forEach(function (n) { var s = sizeOf(n); maxSide = Math.max(maxSide, s.width, s.height); });
    var ringGap = maxSide + COMPONENT_GAP;
    var radius = [0];
    order.forEach(function (id) {
      var d = depth[id];
      if (d === 0) return;
      var s = sizeOf(byId[id]);
      var need = (Math.max(s.width, s.height) + RING_SPACING) / Math.min(wedge[id], Math.PI);
      if (radius[d] === undefined) radius[d] = radius[d - 1] + ringGap;
      radius[d] = Math.max(radius[d], need);
    });
    for (var d = 1; d < radius.length; d++) radius[d] = Math.max(radius[d], radius[d - 1] + ringGap);

    var pos = {};
    order.forEach(function (id) {
      var s = sizeOf(byId[id]);
      var r = radius[depth[id]];
      pos[id] = {
        x: r * Math.cos(angle[id]) - s.width / 2,
        y: r * Math.sin(angle[id]) - s.height / 2,
      };
    });
    return Promise.resolve(pos);
  }

  function pickCentre(comp, edges, preferred) {
    if (preferred && comp.some(function (n) { return n.id === preferred; })) return preferred;
    var deg = degrees(comp, edges);
    var best = comp[0].id;
    comp.forEach(function (n) { if (deg[n.id] > deg[best]) best = n.id; });
    return best;
  }

  // --- Packing (GL-17) ---------------------------------------------------------
  // Free components in rows, largest first, starting at `origin`; the row width
  // aims at PACK_ASPECT for the whole block but never cuts the widest one.
  function pack(blocks, origin) {
    if (!blocks.length) return;
    blocks.sort(function (a, b) { return b.nodes.length - a.nodes.length; });
    var area = 0, widest = 0;
    blocks.forEach(function (b) {
      area += (b.width + COMPONENT_GAP) * (b.height + COMPONENT_GAP);
      widest = Math.max(widest, b.width);
    });
    var rowWidth = Math.max(widest, Math.sqrt(area * PACK_ASPECT));
    // A dominant component (the main diagram next to a few islands) gets its
    // own row; the islands go in rows below it, not beside its top edge.
    var first = blocks[0];
    var alone = (first.width + COMPONENT_GAP) * (first.height + COMPONENT_GAP) > area / 2;
    var x = origin.x, y = origin.y, rowHeight = 0;
    blocks.forEach(function (b, i) {
      if (x > origin.x && (x + b.width > origin.x + rowWidth || (alone && i === 1))) {
        x = origin.x;
        y += rowHeight + COMPONENT_GAP;
        rowHeight = 0;
      }
      b.place(x, y);
      x += b.width + COMPONENT_GAP;
      rowHeight = Math.max(rowHeight, b.height);
    });
  }

  function layout(graph, options, deps) {
    options = options || {};
    deps = deps || {};
    var algorithm = ALGORITHMS.indexOf(options.algorithm) >= 0 ? options.algorithm : 'network';
    var all = graph.nodes || [];
    var selected = null;
    if (options.selection && options.selection.length >= 2) {
      selected = {};
      options.selection.forEach(function (id) { selected[id] = true; });
    }
    var nodes = selected ? all.filter(function (n) { return selected[n.id]; }) : all.slice();
    if (!nodes.length) return Promise.resolve({});

    var idSet = {};
    nodes.forEach(function (n) { idSet[n.id] = true; });
    var reversed = algorithm === 'tree-down' || algorithm === 'tree-right' ? (options.reversed || DEFAULT_REVERSED) : null;
    var edges = edgesWithin(graph.edges || [], idSet, reversed);
    var comps = components(nodes, edges);
    var before = bounds(nodes);
    var beforeCentre = centroid(nodes);

    var pos = {};
    var anchored = [], free = [];

    function one(comp) {
      checkAbort(deps.signal);
      var compSet = {};
      comp.forEach(function (n) { compSet[n.id] = true; });
      var compEdges = edges.filter(function (e) { return compSet[e.source]; });
      var run;
      if (comp.length === 1) {
        var single = {};
        single[comp[0].id] = { x: 0, y: 0 };
        run = Promise.resolve(single);
      } else if (algorithm === 'network') run = network(comp, compEdges, deps);
      else if (algorithm === 'radial') run = radial(comp, compEdges, pickCentre(comp, compEdges, options.centre));
      else run = tree(comp, compEdges, algorithm === 'tree-down' ? 'DOWN' : 'RIGHT', deps);
      return run.then(function (p) {
        checkAbort(deps.signal);
        var ids = comp.map(function (n) { return n.id; });
        ids.forEach(function (id) { pos[id] = p[id]; });
        var fixed = comp.filter(function (n) { return n.fixed; });
        if (fixed.length) {
          // Lay the component out around its fixed nodes: match their
          // centroid, then put them back exactly where they were.
          var was = centroid(fixed), now = centroid(fixed, pos);
          translate(ids, pos, was.x - now.x, was.y - now.y);
          fixed.forEach(function (n) { pos[n.id] = { x: n.x, y: n.y }; });
          anchored.push(comp);
          return;
        }
        var b = bounds(comp, pos);
        free.push({
          nodes: comp,
          width: b.right - b.left,
          height: b.bottom - b.top,
          place: function (x, y) { translate(ids, pos, x - b.left, y - b.top); },
        });
      });
    }

    var chain = Promise.resolve();
    comps.forEach(function (comp) { chain = chain.then(function () { return one(comp); }); });
    return chain.then(function () {
      if (anchored.length) {
        // Free components go in rows below everything that stays put.
        var fixedBox = bounds([].concat.apply([], anchored), pos);
        pack(free, { x: fixedBox.left, y: fixedBox.bottom + COMPONENT_GAP });
      } else {
        pack(free, { x: before.left, y: before.top });
        if (selected) {
          // GL-13: the selection stays centred where it was.
          var now = centroid(nodes, pos);
          translate(Object.keys(pos), pos, beforeCentre.x - now.x, beforeCentre.y - now.y);
        }
      }
      return pos;
    });
  }

  // --- Align / distribute (GL-51) ---------------------------------------------
  // mode: left | center | right | top | middle | bottom  (align to the
  // selection's bounding box), or distribute-h | distribute-v (first and last
  // stay, equal gaps between the boxes in between). Returns { id: {x, y} }.
  var ALIGN_MODES = ['left', 'center', 'right', 'top', 'middle', 'bottom', 'distribute-h', 'distribute-v'];

  function align(nodes, mode) {
    var pos = {};
    if (nodes.length < 2) return pos;
    var b = bounds(nodes);
    if (mode === 'distribute-h' || mode === 'distribute-v') {
      var horizontal = mode === 'distribute-h';
      var sorted = nodes.slice().sort(function (p, q) {
        return horizontal ? (p.x + sizeOf(p).width / 2) - (q.x + sizeOf(q).width / 2)
                          : (p.y + sizeOf(p).height / 2) - (q.y + sizeOf(q).height / 2);
      });
      var total = 0;
      sorted.forEach(function (n) { total += horizontal ? sizeOf(n).width : sizeOf(n).height; });
      var span = horizontal ? b.right - b.left : b.bottom - b.top;
      var gap = (span - total) / (sorted.length - 1);
      var at = horizontal ? b.left : b.top;
      sorted.forEach(function (n) {
        var s = sizeOf(n);
        pos[n.id] = horizontal ? { x: at, y: n.y } : { x: n.x, y: at };
        at += (horizontal ? s.width : s.height) + gap;
      });
      return pos;
    }
    nodes.forEach(function (n) {
      var s = sizeOf(n);
      var x = n.x, y = n.y;
      switch (mode) {
        case 'left': x = b.left; break;
        case 'center': x = (b.left + b.right) / 2 - s.width / 2; break;
        case 'right': x = b.right - s.width; break;
        case 'top': y = b.top; break;
        case 'middle': y = (b.top + b.bottom) / 2 - s.height / 2; break;
        case 'bottom': y = b.bottom - s.height; break;
      }
      pos[n.id] = { x: x, y: y };
    });
    return pos;
  }

  return {
    ALGORITHMS: ALGORITHMS,
    ALIGN_MODES: ALIGN_MODES,
    DEFAULT_REVERSED: DEFAULT_REVERSED,
    layout: layout,
    align: align,
    // exposed for tests
    _components: components,
    _radial: radial,
    _pack: pack,
  };
});
