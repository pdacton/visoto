// Tests for static/js/graph-layout.js. No dependencies: run with
//   node --test tests/js/
// ELK and WebCola are stubbed — these check Visoto's own rules (packing,
// reversal, anchoring, selection, radial wedges), not the engines.
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const L = require('../../static/js/graph-layout.js');

const SUBCLASS = 'http://www.w3.org/2000/01/rdf-schema#subClassOf';

function node(id, extra) {
  return Object.assign({ id, x: 0, y: 0, width: 100, height: 40 }, extra);
}

// Stub ELK: records the graph it was given and stacks nodes by longest path,
// so "who is on top" follows the edge direction it received.
function stubElk(calls) {
  return () => Promise.resolve({
    layout(g) {
      calls.push(g);
      const layer = {};
      g.children.forEach((c) => { layer[c.id] = 0; });
      for (let i = 0; i < g.children.length; i++) {
        g.edges.forEach((e) => {
          const s = e.sources[0], t = e.targets[0];
          if (layer[t] < layer[s] + 1) layer[t] = layer[s] + 1;
        });
      }
      const seen = {};
      return Promise.resolve({
        children: g.children.map((c) => {
          const l = layer[c.id];
          seen[l] = (seen[l] || 0) + 1;
          return { id: c.id, x: (seen[l] - 1) * 150, y: l * 100 };
        }),
      });
    },
  });
}

const noForce = () => {}; // leaves the start positions

test('tree reverses subClassOf so the superclass is on top', async () => {
  const calls = [];
  const graph = {
    nodes: [node('child'), node('parent')],
    edges: [{ source: 'child', target: 'parent', type: SUBCLASS }],
  };
  const pos = await L.layout(graph, { algorithm: 'tree-down' }, { elk: stubElk(calls), force: noForce });
  assert.deepEqual(calls[0].edges[0].sources, ['parent']);
  assert.ok(pos.parent.y < pos.child.y, 'parent above child');
});

test('reversal does not apply to network', async () => {
  let seen;
  const graph = {
    nodes: [node('a'), node('b', { x: 300 })],
    edges: [{ source: 'a', target: 'b', type: SUBCLASS }],
  };
  await L.layout(graph, { algorithm: 'network' }, { force: (nodes, links) => { seen = links; } });
  assert.equal(seen[0].source.id, 'a');
});

test('disconnected components are packed without overlap, largest first', async () => {
  const graph = {
    nodes: [node('s1'), node('a'), node('b'), node('c'), node('s2')],
    edges: [
      { source: 'a', target: 'b', type: 'p' },
      { source: 'b', target: 'c', type: 'p' },
    ],
  };
  const pos = await L.layout(graph, { algorithm: 'tree-down' }, { elk: stubElk([]), force: noForce });
  const boxes = Object.keys(pos).map((id) => ({ id, x: pos[id].x, y: pos[id].y, w: 100, h: 40 }));
  for (let i = 0; i < boxes.length; i++) {
    for (let j = i + 1; j < boxes.length; j++) {
      const p = boxes[i], q = boxes[j];
      const overlap = p.x < q.x + q.w && q.x < p.x + p.w && p.y < q.y + q.h && q.y < p.y + p.h;
      assert.ok(!overlap, `${p.id} overlaps ${q.id}`);
    }
  }
  // The 3-node component starts at the packing origin (the old top-left).
  assert.equal(Math.min(pos.a.x, pos.b.x, pos.c.x), 0);
  assert.equal(Math.min(pos.a.y, pos.b.y, pos.c.y), 0);
});

test('fixed nodes keep their position', async () => {
  const graph = {
    nodes: [node('a', { x: 500, y: 700, fixed: true }), node('b'), node('c')],
    edges: [{ source: 'a', target: 'b', type: 'p' }, { source: 'a', target: 'c', type: 'p' }],
  };
  const pos = await L.layout(graph, { algorithm: 'radial' }, {});
  assert.deepEqual(pos.a, { x: 500, y: 700 });
});

test('selection is laid out alone and keeps its centroid', async () => {
  const graph = {
    nodes: [node('a', { x: 0, y: 0 }), node('b', { x: 1000, y: 1000 }), node('other', { x: 5000, y: 5000 })],
    edges: [{ source: 'a', target: 'b', type: 'p' }, { source: 'b', target: 'other', type: 'p' }],
  };
  const pos = await L.layout(graph, { algorithm: 'tree-right', selection: ['a', 'b'] }, { elk: stubElk([]) });
  assert.equal(pos.other, undefined, 'unselected node untouched');
  const cx = (pos.a.x + pos.b.x) / 2 + 50, cy = (pos.a.y + pos.b.y) / 2 + 20;
  assert.ok(Math.abs(cx - 550) < 1e-9 && Math.abs(cy - 520) < 1e-9, `centroid ${cx},${cy}`);
});

test('a single selected node is not a selection layout', async () => {
  const graph = { nodes: [node('a'), node('b', { x: 400 })], edges: [] };
  const pos = await L.layout(graph, { algorithm: 'radial', selection: ['a'] }, {});
  assert.ok(pos.b, 'whole graph laid out');
});

test('radial: rings by hop distance, subtrees do not share a wedge', async () => {
  // centre -> x -> x1, x2 ; centre -> y
  const nodes = ['c', 'x', 'y', 'x1', 'x2'].map((id) => node(id));
  const edges = [
    { source: 'c', target: 'x' }, { source: 'c', target: 'y' },
    { source: 'x', target: 'x1' }, { source: 'x', target: 'x2' },
  ];
  const pos = await L._radial(nodes, edges, 'c');
  const r = (id) => Math.hypot(pos[id].x + 50, pos[id].y + 20);
  assert.ok(r('c') < 1e-9);
  assert.ok(Math.abs(r('x') - r('y')) < 1e-6, 'x and y on one ring');
  assert.ok(r('x1') > r('x') + 1, 'grandchildren further out');
  // x has 3 of 4 descendants' weight: its children sit closer to x than to y.
  const ang = (id) => Math.atan2(pos[id].y + 20, pos[id].x + 50);
  const d = (a, b) => Math.abs(Math.atan2(Math.sin(a - b), Math.cos(a - b)));
  assert.ok(d(ang('x1'), ang('x')) < d(ang('x1'), ang('y')));
});

test('radial centre: explicit, else highest degree', async () => {
  const graph = {
    nodes: ['a', 'hub', 'b', 'c'].map((id) => node(id)),
    edges: [{ source: 'hub', target: 'a' }, { source: 'hub', target: 'b' }, { source: 'hub', target: 'c' }],
  };
  const dist = (pos, id) => Math.hypot(pos[id].x - pos.hub.x, pos[id].y - pos.hub.y);
  const auto = await L.layout(graph, { algorithm: 'radial' }, {});
  assert.ok(Math.abs(dist(auto, 'a') - dist(auto, 'b')) < 1e-6, 'hub is the centre');
  const chosen = await L.layout(graph, { algorithm: 'radial', centre: 'a' }, {});
  assert.ok(dist(chosen, 'b') > dist(chosen, 'a'), 'a is the centre, b one ring beyond hub');
});

test('an aborted signal rejects with AbortError', async () => {
  const ctrl = new AbortController();
  ctrl.abort();
  const graph = { nodes: [node('a'), node('b')], edges: [] };
  await assert.rejects(L.layout(graph, { algorithm: 'radial' }, { signal: ctrl.signal }), { name: 'AbortError' });
});

test('islands go below a dominant component, not beside it', async () => {
  const nodes = [];
  const edges = [];
  for (let i = 0; i < 10; i++) {
    nodes.push(node('m' + i));
    if (i) edges.push({ source: 'm' + (i - 1), target: 'm' + i, type: 'p' });
  }
  nodes.push(node('i1'), node('i2'));
  edges.push({ source: 'i1', target: 'i2', type: 'p' });
  const pos = await L.layout({ nodes, edges }, { algorithm: 'tree-down' }, { elk: stubElk([]) });
  const mainBottom = Math.max(...nodes.slice(0, 10).map((n) => pos[n.id].y)) + 40;
  assert.ok(pos.i1.y >= mainBottom, 'island below the main component');
});
