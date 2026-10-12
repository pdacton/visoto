/*
  WKT → GeoJSON geometry, for the sparqlMap partial (static/js/sparql-map.js).

  GeoSPARQL literals (geo:asWKT) are WKT, optionally preceded by a CRS IRI:

    "POINT(8.333533 46.575339)"
    "MULTIPOLYGON (((7.69 47.60, 7.67 47.58, …)))"     swisstopo puts a space
    "<http://www.opengis.net/def/crs/OGC/1.3/CRS84> POINT(8.3 46.5)"

  GeoSPARQL's default CRS is CRS84 (lon lat), which is GeoJSON's axis order, so
  coordinates pass through as written. EPSG:4326 declares lat lon and is
  swapped. Any other CRS (swisstopo's LV95 = EPSG:2056, say) would need a
  projection library; it is reported instead of drawn somewhere wrong.

    VisotoWKT.parse(text)   { geometry } on success,
                            { geometry: null, reason } otherwise, where reason
                            is 'unsupported-crs' or 'malformed'

  Z/M ordinates are dropped. EMPTY geometries parse to null ('malformed'), since
  there is nothing to draw.
*/
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.VisotoWKT = api;
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var CRS84 = /\/OGC\/1\.3\/CRS84$/i;
  var EPSG4326 = /\/EPSG\/0\/4326$/i;

  function Parser(text) {
    this.s = text;
    this.i = 0;
  }

  Parser.prototype.ws = function () {
    while (this.i < this.s.length && /\s/.test(this.s[this.i])) this.i++;
  };

  Parser.prototype.peek = function () {
    this.ws();
    return this.s[this.i];
  };

  Parser.prototype.expect = function (ch) {
    if (this.peek() !== ch) throw new Error('expected ' + ch);
    this.i++;
  };

  Parser.prototype.word = function () {
    this.ws();
    var m = /^[A-Za-z]+/.exec(this.s.slice(this.i));
    if (!m) return '';
    this.i += m[0].length;
    return m[0].toUpperCase();
  };

  // A type keyword, then an optional Z / M / ZM dimension tag.
  Parser.prototype.keyword = function () {
    var w = this.word();
    if (!w) throw new Error('expected a geometry type');
    var save = this.i;
    var dim = this.word();
    if (dim !== 'Z' && dim !== 'M' && dim !== 'ZM') this.i = save;
    return w;
  };

  Parser.prototype.empty = function () {
    var save = this.i;
    if (this.word() === 'EMPTY') return true;
    this.i = save;
    return false;
  };

  Parser.prototype.number = function () {
    this.ws();
    var m = /^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?/.exec(this.s.slice(this.i));
    if (!m) throw new Error('expected a number');
    this.i += m[0].length;
    return parseFloat(m[0]);
  };

  // "x y [z [m]]" → [x, y]
  Parser.prototype.position = function () {
    var p = [this.number(), this.number()];
    while (/[-+.\d]/.test(this.peek() || '')) this.number();
    return p;
  };

  // "(a, b, …)" where each item is read by fn
  Parser.prototype.list = function (fn) {
    this.expect('(');
    var out = [fn.call(this)];
    while (this.peek() === ',') {
      this.i++;
      out.push(fn.call(this));
    }
    this.expect(')');
    return out;
  };

  Parser.prototype.positions = function () { return this.list(this.position); };
  Parser.prototype.rings = function () { return this.list(this.positions); };
  Parser.prototype.polygons = function () { return this.list(this.rings); };

  // MULTIPOINT accepts both "(1 2, 3 4)" and "((1 2), (3 4))".
  Parser.prototype.multipoint = function () {
    return this.list(function () {
      if (this.peek() === '(') {
        this.i++;
        var p = this.position();
        this.expect(')');
        return p;
      }
      return this.position();
    });
  };

  Parser.prototype.geometry = function () {
    var type = this.keyword();
    if (this.empty()) return null;
    switch (type) {
      case 'POINT': {
        this.expect('(');
        var p = this.position();
        this.expect(')');
        return { type: 'Point', coordinates: p };
      }
      case 'LINESTRING': return { type: 'LineString', coordinates: this.positions() };
      case 'POLYGON': return { type: 'Polygon', coordinates: this.rings() };
      case 'MULTIPOINT': return { type: 'MultiPoint', coordinates: this.multipoint() };
      case 'MULTILINESTRING': return { type: 'MultiLineString', coordinates: this.rings() };
      case 'MULTIPOLYGON': return { type: 'MultiPolygon', coordinates: this.polygons() };
      case 'GEOMETRYCOLLECTION': {
        var parts = this.list(this.geometry).filter(Boolean);
        return parts.length ? { type: 'GeometryCollection', geometries: parts } : null;
      }
      default: throw new Error('unknown geometry type ' + type);
    }
  };

  function swapAxes(g) {
    if (!g) return g;
    if (g.type === 'GeometryCollection') {
      return { type: g.type, geometries: g.geometries.map(swapAxes) };
    }
    function walk(c) {
      return typeof c[0] === 'number' ? [c[1], c[0]] : c.map(walk);
    }
    return { type: g.type, coordinates: walk(g.coordinates) };
  }

  function parse(text) {
    if (typeof text !== 'string') return { geometry: null, reason: 'malformed' };
    var s = text.trim();
    var swap = false;
    var crs = /^<([^>]*)>\s*/.exec(s);
    if (crs) {
      s = s.slice(crs[0].length);
      if (EPSG4326.test(crs[1])) swap = true;
      else if (!CRS84.test(crs[1])) return { geometry: null, reason: 'unsupported-crs' };
    }
    try {
      var parser = new Parser(s);
      var g = parser.geometry();
      parser.ws();
      if (!g || parser.i !== s.length) return { geometry: null, reason: 'malformed' };
      return { geometry: swap ? swapAxes(g) : g };
    } catch (e) {
      return { geometry: null, reason: 'malformed' };
    }
  }

  return { parse: parse };
});
