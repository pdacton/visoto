// Tests for static/js/wkt.js (run: node --test tests/js/).
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const WKT = require('../../static/js/wkt.js');

const geom = (s) => WKT.parse(s).geometry;

test('POINT: lon lat pass through, whitespace and case tolerated', () => {
  assert.deepEqual(geom('POINT(8.333533 46.575339)'), { type: 'Point', coordinates: [8.333533, 46.575339] });
  assert.deepEqual(geom('  point ( -1.5e1  +2 ) '), { type: 'Point', coordinates: [-15, 2] });
});

test('Z / M ordinates are dropped', () => {
  assert.deepEqual(geom('POINT Z (1 2 3)'), { type: 'Point', coordinates: [1, 2] });
  assert.deepEqual(geom('LINESTRING ZM (1 2 3 4, 5 6 7 8)'), { type: 'LineString', coordinates: [[1, 2], [5, 6]] });
});

test('LINESTRING, POLYGON, MULTILINESTRING', () => {
  assert.deepEqual(geom('LINESTRING(0 0,1 1)'), { type: 'LineString', coordinates: [[0, 0], [1, 1]] });
  assert.deepEqual(geom('POLYGON((0 0,1 0,1 1,0 0),(0.2 0.2,0.3 0.2,0.3 0.3,0.2 0.2))').coordinates.length, 2);
  assert.deepEqual(geom('MULTILINESTRING((0 0,1 1),(2 2,3 3))'),
    { type: 'MultiLineString', coordinates: [[[0, 0], [1, 1]], [[2, 2], [3, 3]]] });
});

test('MULTIPOLYGON with the swisstopo space after the type name', () => {
  const g = geom('MULTIPOLYGON (((7.69 47.60, 7.67 47.58, 7.67 47.59, 7.69 47.60)), ((8 46, 9 46, 9 47, 8 46)))');
  assert.equal(g.type, 'MultiPolygon');
  assert.equal(g.coordinates.length, 2);
  assert.deepEqual(g.coordinates[0][0][0], [7.69, 47.60]);
});

test('MULTIPOINT in both spellings', () => {
  const want = { type: 'MultiPoint', coordinates: [[1, 2], [3, 4]] };
  assert.deepEqual(geom('MULTIPOINT(1 2, 3 4)'), want);
  assert.deepEqual(geom('MULTIPOINT((1 2), (3 4))'), want);
});

test('GEOMETRYCOLLECTION', () => {
  const g = geom('GEOMETRYCOLLECTION(POINT(1 2), LINESTRING(0 0, 1 1))');
  assert.equal(g.type, 'GeometryCollection');
  assert.deepEqual(g.geometries.map((x) => x.type), ['Point', 'LineString']);
});

test('CRS prefix: CRS84 kept, EPSG:4326 swapped, others unsupported', () => {
  assert.deepEqual(geom('<http://www.opengis.net/def/crs/OGC/1.3/CRS84> POINT(8 46)').coordinates, [8, 46]);
  assert.deepEqual(geom('<http://www.opengis.net/def/crs/EPSG/0/4326> POINT(46 8)').coordinates, [8, 46]);
  assert.deepEqual(geom('<http://www.opengis.net/def/crs/EPSG/0/4326> LINESTRING(46 8, 47 9)').coordinates, [[8, 46], [9, 47]]);
  assert.deepEqual(WKT.parse('<http://www.opengis.net/def/crs/EPSG/0/2056> POINT(2600000 1200000)'),
    { geometry: null, reason: 'unsupported-crs' });
});

test('malformed and empty input', () => {
  for (const s of ['', 'POINT', 'POINT(1)', 'POINT(1 2', 'POINT(1 2) trailing', 'CIRCLE(1 2)', 'POINT EMPTY', null]) {
    assert.deepEqual(WKT.parse(s), { geometry: null, reason: 'malformed' }, String(s));
  }
});
