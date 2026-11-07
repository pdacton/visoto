// Tests for static/js/graph-touch.js — the pinch arithmetic. No dependencies:
//   node --test tests/js/
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const T = require('../../static/js/graph-touch.js');

const p = (x, y) => ({ x, y });

test('spreading the fingers zooms in around their midpoint', () => {
  const s = T.pinchStep([p(100, 100), p(200, 100)], [p(50, 100), p(250, 100)]);
  assert.equal(s.ratio, 2);
  assert.deepEqual(s.pivot, p(150, 100));
  assert.deepEqual(s.pan, { dx: 0, dy: 0 });
});

test('moving both fingers together pans without zooming', () => {
  const s = T.pinchStep([p(100, 100), p(200, 100)], [p(130, 60), p(230, 60)]);
  assert.equal(s.ratio, 1);
  assert.deepEqual(s.pan, { dx: 30, dy: -40 });
  assert.deepEqual(s.pivot, p(180, 60));
});

test('pinching in zooms out; rotation alone keeps the scale', () => {
  assert.equal(T.pinchStep([p(0, 0), p(200, 0)], [p(50, 0), p(150, 0)]).ratio, 0.5);
  const r = T.pinchStep([p(0, 0), p(100, 0)], [p(50, -50), p(50, 50)]);
  assert.ok(Math.abs(r.ratio - 1) < 1e-9);
});

test('two fingers on one spot do not divide by zero', () => {
  assert.equal(T.pinchStep([p(5, 5), p(5, 5)], [p(0, 0), p(10, 10)]).ratio, 1);
});
