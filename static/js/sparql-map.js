/* eslint-disable */
/*
  Behaviour for the "sparqlMap" partial (templates/partials/sparql-map.html):
  finds the resource's geometry and draws it with Leaflet.

  1. Probe. At load, one SELECT through the same-origin proxy asks for the
     resource's geo:asWKT (directly or via geo:hasGeometry) and for
     schema:latitude/longitude or WGS84 geo:lat/long pairs. The promise is cached
     per endpoint + IRI, so the Feature template's inline map and the header's
     Map view share one request.
  2. Reveal. Geometry found: the card is un-hidden, and for the map that backs
     the header's Map view (data-sparql-map-tab) the view button is too. None
     found: everything stays hidden, and a #Map deep link falls back to the Page
     view.
  3. Draw. Leaflet (cdnjs, SRI) is loaded on first draw. A lazy map waits for
     the 'map:init' event resource-view-toggle.js fires on first reveal, since
     Leaflet cannot measure a display:none container.

  Vocabulary IRIs come from VisotoPrefixes (static/js/prefixes.js), never
  hard-coded, so an endpoint that redeclares schema: is followed.

  Attributes read from the root element:
    data-sparql-map        marker; presence means "initialize me"
    data-sparql-map-id     DOM id prefix for this map's elements/islands
    data-sparql-map-lazy   draw on 'map:init' instead of at once
    data-sparql-map-tab    this map backs the header's Map view button
*/
(function () {
  'use strict';

  // Keep src and integrity in step: a bump without a new hash fails closed.
  var LEAFLET_JS = 'https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.js';
  var LEAFLET_JS_SRI = 'sha384-cxOPjt7s7Iz04uaHJceBmS+qpjv2JkIHNVcuOrM+YHwZOmJGBXI00mdUXEq65HTH';
  var LEAFLET_CSS = 'https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.css';
  var LEAFLET_CSS_SRI = 'sha384-sHL9NAb7lN7rfvG5lfHpm643Xkcjzp4jFvuavGOndn6pjVqS6ny56CAt3nsEVT4H';
  var LOAD_TIMEOUT = 15000;

  // Web-Mercator WMTS of the federal geoportal; free with attribution.
  var SWISSTOPO = 'https://wmts.geo.admin.ch/1.0.0/{layer}/default/current/3857/{z}/{x}/{y}.jpeg';
  var SWISSTOPO_ATTR = '&copy; <a href="https://www.swisstopo.admin.ch/" target="_blank" rel="noopener">swisstopo</a>';
  var OSM = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';
  var OSM_ATTR = '&copy; <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a>';
  // Where the swisstopo base maps have detail (lon/lat, generous).
  var CH_BBOX = { west: 5.8, east: 10.6, south: 45.7, north: 47.9 };

  var WGS84 = 'http://www.w3.org/2003/01/geo/wgs84_pos#';

  // Publishers of geometry nodes other endpoints link to without copying the
  // WKT: LINDAS municipalities and cantons point geo:hasGeometry at
  // geo.ld.admin.ch. Their WKT is fetched with a federated SERVICE query sent
  // to the CURRENT endpoint (through the proxy as always), so no second
  // endpoint is configured or reached from the browser. Namespace → SPARQL
  // service of the public store that owns it.
  var GEOMETRY_SERVICES = {
    'https://geo.ld.admin.ch/': 'https://geo.ld.admin.ch/query',
  };

  var PIN_SVG = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 42" width="32" height="42" aria-hidden="true">' +
    '<path class="vs-map-pin-body" d="M16 1C7.7 1 1 7.6 1 15.8 1 27 16 41 16 41s15-14 15-25.2C31 7.6 24.3 1 16 1z"/>' +
    '<circle cx="16" cy="15.5" r="5.5" fill="#fff"/></svg>';

  // --- Leaflet loader ------------------------------------------------------------
  // One shared <script>/<link> for every map on the page. Readiness is polled, as
  // in ge-adapter.js: another map may have injected the script already.
  var leafletLoading = null;
  function loadLeaflet() {
    if (window.L && window.L.map) return Promise.resolve(window.L);
    if (leafletLoading) return leafletLoading;
    leafletLoading = new Promise(function (resolve, reject) {
      if (!document.querySelector('link[data-leaflet-loader]')) {
        var css = document.createElement('link');
        css.rel = 'stylesheet';
        css.href = LEAFLET_CSS;
        css.integrity = LEAFLET_CSS_SRI;
        css.crossOrigin = 'anonymous';
        css.setAttribute('data-leaflet-loader', '');
        document.head.appendChild(css);
      }
      if (!document.querySelector('script[data-leaflet-loader]')) {
        var script = document.createElement('script');
        script.src = LEAFLET_JS;
        script.integrity = LEAFLET_JS_SRI;
        script.crossOrigin = 'anonymous';
        script.setAttribute('data-leaflet-loader', '');
        script.addEventListener('error', function () { reject(new Error('load')); });
        document.head.appendChild(script);
      }
      var waited = 0;
      var poll = setInterval(function () {
        if (window.L && window.L.map) {
          clearInterval(poll);
          resolve(window.L);
        } else if ((waited += 50) >= LOAD_TIMEOUT) {
          clearInterval(poll);
          reject(new Error('timeout'));
        }
      }, 50);
    });
    leafletLoading.catch(function () { leafletLoading = null; });
    return leafletLoading;
  }

  // --- Geometry probe ------------------------------------------------------------
  function vocab() {
    var P = window.VisotoPrefixes && window.VisotoPrefixes.fromPage();
    function iri(curie, fallback) {
      var full = P ? P.expand(curie) : curie;
      return full === curie ? fallback : full;
    }
    return {
      hasGeometry: iri('geo:hasGeometry', 'http://www.opengis.net/ont/geosparql#hasGeometry'),
      asWKT: iri('geo:asWKT', 'http://www.opengis.net/ont/geosparql#asWKT'),
      latitude: iri('schema:latitude', 'http://schema.org/latitude'),
      longitude: iri('schema:longitude', 'http://schema.org/longitude'),
    };
  }

  // Percent-encode the characters an IRIREF may not contain.
  function iriRef(iri) {
    return '<' + String(iri).replace(/[\s<>"{}|^`\\]/g, function (c) {
      return encodeURIComponent(c);
    }) + '>';
  }

  // With `via` (a property IRI) the geometry is that of the resources the
  // property leads to, e.g. a district's municipalities via schema:hasPart.
  // The hop is repeated inside each UNION branch rather than joined outside it:
  // QLever evaluates a branch bottom-up, so a branch must bind ?s itself.
  function probeQuery(iri, via) {
    var v = vocab();
    var hop = via ? iriRef(iri) + ' <' + via + '> ?s . ' : '';
    var s = via ? '?s' : iriRef(iri);
    // ?g is selected even without a local WKT: it is what the federated
    // fallback (remoteQuery) asks the geometry's publisher about.
    return 'SELECT ?g ?wkt ?lat ?long WHERE {\n' +
      '  { ' + hop + s + ' <' + v.hasGeometry + '> ?g . OPTIONAL { ?g <' + v.asWKT + '> ?wkt } }\n' +
      '  UNION { ' + hop + s + ' <' + v.asWKT + '> ?wkt }\n' +
      '  UNION { ' + hop + s + ' <' + v.latitude + '> ?lat ; <' + v.longitude + '> ?long }\n' +
      '  UNION { ' + hop + s + ' <' + WGS84 + 'lat> ?lat ; <' + WGS84 + 'long> ?long }\n' +
      '} LIMIT ' + (via ? 1000 : 50);
  }

  // Geometry nodes with no WKT on this endpoint, grouped by the SERVICE that
  // publishes them: { serviceUrl: [iri, …] }.
  function remoteGeometries(bindings) {
    var out = {};
    bindings.forEach(function (b) {
      if (!b.g || b.wkt || b.g.type !== 'uri') return;
      Object.keys(GEOMETRY_SERVICES).forEach(function (ns) {
        if (b.g.value.indexOf(ns) !== 0) return;
        var svc = GEOMETRY_SERVICES[ns];
        (out[svc] = out[svc] || []).push(b.g.value);
      });
    });
    return out;
  }

  function remoteQuery(serviceUrl, iris) {
    var v = vocab();
    return 'SELECT ?wkt WHERE {\n' +
      '  VALUES ?g { ' + iris.map(iriRef).join(' ') + ' }\n' +
      '  SERVICE <' + serviceUrl + '> { ?g <' + v.asWKT + '> ?wkt }\n' +
      '} LIMIT 1000';
  }

  function select(endpointUrl, query) {
    return fetch(endpointUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/sparql-query', 'Accept': 'application/sparql-results+json' },
      body: query,
    }).then(function (res) {
      if (!res.ok) throw new Error('SPARQL request failed: ' + res.status);
      return res.json();
    }).then(function (json) {
      return (json && json.results && json.results.bindings) || [];
    });
  }

  // → { features: [GeoJSON Feature], unsupported: bool }
  function toFeatures(bindings) {
    var features = [];
    var unsupported = false;
    bindings.forEach(function (b) {
      if (!b.wkt) return;
      var r = window.VisotoWKT.parse(b.wkt.value);
      if (r.geometry) features.push({ type: 'Feature', properties: {}, geometry: r.geometry });
      else if (r.reason === 'unsupported-crs') unsupported = true;
    });
    // A lat/long pair only stands in when no WKT could be drawn.
    if (!features.length) {
      bindings.forEach(function (b) {
        if (!b.lat || !b.long) return;
        var lat = parseFloat(b.lat.value);
        var lon = parseFloat(b.long.value);
        if (isFinite(lat) && isFinite(lon) && Math.abs(lat) <= 90 && Math.abs(lon) <= 180) {
          features.push({ type: 'Feature', properties: {}, geometry: { type: 'Point', coordinates: [lon, lat] } });
        }
      });
    }
    return { features: dedupe(features), unsupported: unsupported };
  }

  // Schema and WGS84 pairs often repeat one point; draw it once.
  function dedupe(features) {
    var seen = {};
    return features.filter(function (f) {
      var k = JSON.stringify(f.geometry);
      if (seen[k]) return false;
      seen[k] = true;
      return true;
    });
  }

  var probes = {};
  function probe(endpointUrl, iri, via) {
    var key = endpointUrl + '\n' + iri + '\n' + (via || '');
    if (!probes[key]) {
      probes[key] = select(endpointUrl, probeQuery(iri, via)).then(function (bindings) {
        var local = toFeatures(bindings);
        if (local.features.length) return local;
        // Nothing drawable here, but the geometry may live with its publisher.
        // Only then is SERVICE used, so an endpoint without federation support
        // never sees it, and a failure just leaves the local answer.
        var remote = remoteGeometries(bindings);
        var services = Object.keys(remote);
        if (!services.length) return local;
        return Promise.all(services.map(function (svc) {
          return select(endpointUrl, remoteQuery(svc, remote[svc])).catch(function () { return []; });
        })).then(function (lists) {
          var fetched = toFeatures([].concat.apply([], lists));
          return fetched.features.length ? fetched : local;
        });
      });
      // A failed probe is not cached: the next map (or a reload) asks again.
      probes[key].catch(function () { delete probes[key]; });
    }
    return probes[key];
  }

  // --- One map ---------------------------------------------------------------------
  function initSparqlMap(root) {
    if (root.dataset.sparqlMapReady) return;
    root.dataset.sparqlMapReady = 'true';

    var ID = root.getAttribute('data-sparql-map-id');
    var lazy = root.getAttribute('data-sparql-map-lazy') === 'true';
    var tab = root.getAttribute('data-sparql-map-tab') === 'true';
    var card = document.getElementById(ID + '-card');
    var overlay = document.getElementById(ID + '-overlay');

    function readIsland(suffix) {
      var el = document.getElementById(ID + suffix);
      if (!el) return null;
      try { return JSON.parse(el.innerHTML.trim()); } catch (e) { return null; }
    }

    var iri = readIsland('-iri');
    var endpointUrl = readIsland('-endpoint-url') || '/api/sparql';
    var via = readIsland('-via');
    if (via && window.VisotoPrefixes) via = window.VisotoPrefixes.fromPage().expand(via);
    root.style.height = readIsland('-height') || '400px';
    if (!iri || !window.VisotoWKT) return;

    var result = null;      // probe outcome, once known
    var wanted = !lazy;     // true once the map may draw (visible)
    var map = null;

    function message(text) {
      overlay.textContent = text;
      overlay.classList.remove('d-none');
    }

    function draw() {
      if (map || !result || !wanted) return;
      if (!result.features.length) {
        message(vsT('js.map.unsupportedCrs', 'This geometry uses a coordinate reference system the map cannot display.'));
        return;
      }
      loadLeaflet().then(function (L) {
        if (map) return;
        render(L, result.features);
      }, function () {
        message(vsT('js.map.libraryFailed', 'Failed to load the map library.'));
      });
    }

    function render(L, features) {
      var swisstopo = L.tileLayer(SWISSTOPO, {
        layer: 'ch.swisstopo.pixelkarte-farbe', maxZoom: 19, maxNativeZoom: 18, attribution: SWISSTOPO_ATTR,
      });
      var aerial = L.tileLayer(SWISSTOPO, {
        layer: 'ch.swisstopo.swissimage', maxZoom: 20, maxNativeZoom: 19, attribution: SWISSTOPO_ATTR,
      });
      var osm = L.tileLayer(OSM, { maxZoom: 19, attribution: OSM_ATTR });

      var color = getComputedStyle(document.documentElement).getPropertyValue('--tblr-primary').trim() || '#066fd1';
      var shapes = L.geoJSON({ type: 'FeatureCollection', features: features }, {
        style: { color: color, weight: 2, fillOpacity: 0.15 },
        // A pin whose tip sits on the coordinate. Inline SVG (map_overrides.css
        // colours it), so no marker images are fetched.
        pointToLayer: function (f, latlng) {
          return L.marker(latlng, {
            keyboard: false,
            icon: L.divIcon({
              className: 'vs-map-pin',
              html: PIN_SVG,
              iconSize: [32, 42],
              iconAnchor: [16, 41],
            }),
          });
        },
      });

      var bounds = shapes.getBounds();
      var c = bounds.getCenter();
      var inSwitzerland = c.lng >= CH_BBOX.west && c.lng <= CH_BBOX.east && c.lat >= CH_BBOX.south && c.lat <= CH_BBOX.north;

      // An inline map must not swallow the page's scroll; the full-size Map view
      // and fullscreen may.
      map = L.map(root, { scrollWheelZoom: lazy, layers: [inSwitzerland ? swisstopo : osm] });
      var bases = {};
      bases[vsT('js.map.layerMap', 'swisstopo map')] = swisstopo;
      bases[vsT('js.map.layerAerial', 'swisstopo aerial')] = aerial;
      bases['OpenStreetMap'] = osm;
      L.control.layers(bases, null, { position: 'topright' }).addTo(map);
      L.control.scale({ imperial: false }).addTo(map);
      shapes.addTo(map);
      map.fitBounds(bounds, { maxZoom: 15, padding: [24, 24] });

      // Fullscreen, collapse, a revealed view and window resizes all change the
      // canvas box; Leaflet must be told.
      if (window.ResizeObserver) {
        new ResizeObserver(function () { map.invalidateSize(); }).observe(root);
      }
    }

    // Fullscreen: .vs-map-maximized pins the frame (map_overrides.css).
    (function setupFullscreen() {
      var maximizeBtn = document.getElementById(ID + '-maximize');
      var exitBtn = document.getElementById(ID + '-exit');
      if (!card || !maximizeBtn || !exitBtn) return;
      maximizeBtn.addEventListener('click', function () {
        // Expand a collapsed card first. Tabler's bundle is window.tabler.
        var collapsed = document.getElementById('collapse-' + ID);
        var Collapse = window.tabler && window.tabler.Collapse;
        if (collapsed && !collapsed.classList.contains('show') && Collapse) {
          Collapse.getOrCreateInstance(collapsed).show();
        }
        card.classList.add('vs-map-maximized');
        if (map) map.scrollWheelZoom.enable();
      });
      exitBtn.addEventListener('click', function () {
        card.classList.remove('vs-map-maximized');
        if (map && !lazy) map.scrollWheelZoom.disable();
      });
      document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape' && card.classList.contains('vs-map-maximized')) exitBtn.click();
      });
    })();

    if (lazy) {
      root.addEventListener('map:init', function () {
        wanted = true;
        draw();
      });
    }

    function revealTab() {
      var input = document.getElementById('resource-view-map');
      var label = document.querySelector('label[for="resource-view-map"]');
      if (input) input.classList.remove('d-none');
      if (label) label.classList.remove('d-none');
      var item = document.querySelector('[data-view-menu="map"]');
      if (item) item.classList.remove('d-none');
    }

    // No geometry: a #Map deep link has nothing to show; fall back to the Page view.
    function leaveMapView() {
      var mapInput = document.getElementById('resource-view-map');
      var pageInput = document.getElementById('resource-view-table');
      if (mapInput && mapInput.checked && pageInput) {
        pageInput.checked = true;
        pageInput.dispatchEvent(new Event('change'));
      }
    }

    probe(endpointUrl, iri, via).then(function (r) {
      if (!r.features.length && !r.unsupported) {
        if (tab) leaveMapView();
        return;
      }
      result = r;
      card.classList.remove('d-none');
      if (tab) revealTab();
      draw();
    }, function () {
      // Geometry is an extra; a failed probe leaves the page as it was.
      if (tab) leaveMapView();
    });
  }

  function boot() {
    document.querySelectorAll('[data-sparql-map]').forEach(initSparqlMap);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
