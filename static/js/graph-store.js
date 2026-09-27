/* eslint-disable */
/*
  Saved canvases for the Graph Explorer embeds (GL-35, GL-36, GL-50), in this
  browser's localStorage. Pure storage: graph-kit.js builds and applies the
  saved form (see kit.snapshot / kit.open there).

    visoto-graph:auto:<page URL>#<graph id>   autosave of one graph on one page
    visoto-graph:saves                        index of named saves [{ key, name, … }]
    visoto-graph:save:<key>                   one named save

  localStorage can be absent, full, or throw (private windows, blocked site
  data), so every access is guarded; a failed write reports false and the
  caller tells the user.

  A saved canvas (FORMAT, VERSION 1):
    { format, version, savedAt, page, graph, endpoint, language, layout,
      pins: [id], fingerprint, diagram: <GE SerializedDiagram> }
*/
(function () {
  'use strict';

  if (window.VisotoGraphStore) return;

  var FORMAT = 'visoto-graph';
  var VERSION = 1;
  var PREFIX = 'visoto-graph:';
  var INDEX = PREFIX + 'saves';

  function read(key) {
    try {
      var raw = window.localStorage.getItem(key);
      return raw ? JSON.parse(raw) : null;
    } catch (e) {
      return null;
    }
  }
  function write(key, value) {
    try {
      window.localStorage.setItem(key, JSON.stringify(value));
      return true;
    } catch (e) {
      return false;
    }
  }
  function remove(key) {
    try { window.localStorage.removeItem(key); } catch (e) { /* nothing to do */ }
  }

  function valid(saved) {
    return !!saved && saved.format === FORMAT && saved.version === VERSION &&
      !!saved.diagram && !!saved.diagram.layoutData;
  }

  // The page URL without its fragment: /resource?iri=…&endpoint=… identifies
  // the page and the endpoint, so another endpoint keeps its own canvas.
  function autosaveKey(graphId) {
    return PREFIX + 'auto:' + window.location.pathname + window.location.search + '#' + graphId;
  }

  function newKey() {
    return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
  }

  // A short, stable hash of the page's starting resources (GL-35).
  function fingerprint(iris) {
    var s = iris.slice().sort().join('\n');
    var h = 5381;
    for (var i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) | 0;
    return (h >>> 0).toString(36) + ':' + iris.length;
  }

  window.VisotoGraphStore = {
    FORMAT: FORMAT,
    VERSION: VERSION,
    valid: valid,
    fingerprint: fingerprint,

    loadAutosave: function (graphId) {
      var saved = read(autosaveKey(graphId));
      return valid(saved) ? saved : null;
    },
    autosave: function (graphId, saved) { return write(autosaveKey(graphId), saved); },
    clearAutosave: function (graphId) { remove(autosaveKey(graphId)); },

    // Named saves (GL-50). The index holds what the list shows; the canvas
    // itself is stored under its own key.
    list: function () {
      var index = read(INDEX);
      return Array.isArray(index) ? index : [];
    },
    saveAs: function (name, saved) {
      var key = newKey();
      if (!write(PREFIX + 'save:' + key, saved)) return false;
      var index = this.list();
      index.unshift({ key: key, name: name, endpoint: saved.endpoint || '', savedAt: saved.savedAt, page: saved.page });
      if (!write(INDEX, index)) {
        remove(PREFIX + 'save:' + key);
        return false;
      }
      return true;
    },
    get: function (key) {
      var saved = read(PREFIX + 'save:' + key);
      return valid(saved) ? saved : null;
    },
    rename: function (key, name) {
      var index = this.list();
      index.forEach(function (entry) { if (entry.key === key) entry.name = name; });
      return write(INDEX, index);
    },
    remove: function (key) {
      remove(PREFIX + 'save:' + key);
      write(INDEX, this.list().filter(function (entry) { return entry.key !== key; }));
    },

    // A canvas handed from one page load to the next, when opening a file
    // switches the endpoint (GL-36): sessionStorage, read once.
    setPending: function (graphId, saved) {
      try {
        window.sessionStorage.setItem(PREFIX + 'pending:' + graphId, JSON.stringify(saved));
        return true;
      } catch (e) {
        return false;
      }
    },
    takePending: function (graphId) {
      try {
        var key = PREFIX + 'pending:' + graphId;
        var raw = window.sessionStorage.getItem(key);
        window.sessionStorage.removeItem(key);
        var saved = raw ? JSON.parse(raw) : null;
        return valid(saved) ? saved : null;
      } catch (e) {
        return null;
      }
    },
  };
})();
