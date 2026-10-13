/* eslint-disable */
/*
  Ontology projection: turns the normalized facts the ontology diagram's
  CONSTRUCT queries return into the triples graph-memory-store.js draws.

  The queries only extract facts (templates/instances/owl%3AOntology.html);
  every drawing decision is made here. Requirements: .project/intent/ontology/
  ontology-construct.md (IDs J1–J12 below refer to it).

  Input (u: = urn:visoto:):
    ?t a u:Member               a term of the ontology
    ?t a u:Known                a term of an ontology it imports (never external)
    ?t a <kind>                 owl:Class, rdfs:Class, owl:ObjectProperty, …
    ?sub rdfs:subClassOf ?super
    _:u a u:Use ; u:property ?P ; u:class ?C ; u:range ?R ; u:min ?n ; u:max ?n ;
        u:source "rdfs"|"schema"|"owl"|"shacl"
                                one use of P; class, range and bounds may be absent
    ?x rdfs:label ?l            labels; also skos:prefLabel, schema:name, …
                                (LABEL_PREDICATES) — the best one per language wins

  Output: the graph-memory-store.js contract — class boxes (`a owl:Class`),
  u:ExternalClass / u:AnyResource markers, generalizations, `C P R` edges,
  `C P "xsd:type"` attribute rows, labels.

    var triples = VisotoOntologyProjection.project(parsedTriples);
*/
(function () {
  'use strict';

  var RDF = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#';
  var RDFS = 'http://www.w3.org/2000/01/rdf-schema#';
  var OWL = 'http://www.w3.org/2002/07/owl#';
  var XSD = 'http://www.w3.org/2001/XMLSchema#';
  var U = 'urn:visoto:';

  var RDF_TYPE = RDF + 'type';
  var RDFS_LABEL = RDFS + 'label';
  var RDFS_SUBCLASS = RDFS + 'subClassOf';
  var OWL_CLASS = OWL + 'Class';

  var CLASS_KINDS = [OWL_CLASS, RDFS + 'Class'];
  var OWL_PROPERTY_KINDS = [OWL + 'ObjectProperty', OWL + 'DatatypeProperty', OWL + 'AnnotationProperty'];
  // The projection's own node-type and node-label predicates (P3/S1).
  var SKIPPED_PROPERTIES = [RDF_TYPE, RDFS_LABEL];
  // A declared end that says nothing more than "anything" (J5).
  var UNDECLARED = [OWL + 'Thing', RDFS + 'Resource'];
  var LITERAL_TYPES = [RDFS + 'Literal', RDF + 'langString', RDF + 'PlainLiteral', RDF + 'HTML',
    RDF + 'XMLLiteral', RDF + 'JSON'];
  // J3: schema.org's datatypes, in both namespace spellings.
  var SCHEMA_DATATYPES = ['Text', 'Number', 'Integer', 'Float', 'Boolean', 'Date', 'DateTime',
    'Time', 'URL', 'DataType'].reduce(function (all, n) {
    return all.concat(['http://schema.org/' + n, 'https://schema.org/' + n]);
  }, []);
  // GeoSPARQL's literal types, which stores rarely declare as rdfs:Datatype.
  var GEO_LITERALS = ['wktLiteral', 'gmlLiteral', 'geoJSONLiteral', 'kmlLiteral', 'dggsLiteral']
    .map(function (n) { return 'http://www.opengis.net/ont/geosparql#' + n; });
  var CURIES = [[XSD, 'xsd:'], [RDFS, 'rdfs:'], [RDF, 'rdf:'], ['http://schema.org/', 'schema:'],
    ['https://schema.org/', 'schema:'], ['http://www.opengis.net/ont/geosparql#', 'geo:']];
  // S3: label predicates, best first — the order of the "label" magic property
  // in visoto.config, written out because the queries bypass visoto: expansion.
  var LABEL_PREDICATES = [RDFS_LABEL, 'http://www.w3.org/2004/02/skos/core#prefLabel',
    'http://schema.org/name', 'http://purl.org/dc/terms/title', 'http://purl.org/dc/elements/1.1/title',
    'https://www.ica.org/standards/RiC/ontology#title', 'https://www.ica.org/standards/RiC/ontology#name',
    // A shape's sh:name for its path: last resort, for profiles whose
    // vocabularies are not in the store.
    'http://www.w3.org/ns/shacl#name'];
  // J8: whose multiplicity wins when several sources state one.
  var SOURCE_RANK = { shacl: 3, owl: 2, schema: 1, rdfs: 1 };

  function iri(v) { return { type: 'iri', value: v }; }
  function literal(v, lang) { return { type: 'literal', value: v, language: lang || '' }; }

  // J3: a range that is a value, not a resource.
  function isDatatype(r) {
    return r.indexOf(XSD) === 0 || LITERAL_TYPES.indexOf(r) >= 0 || SCHEMA_DATATYPES.indexOf(r) >= 0 ||
      GEO_LITERALS.indexOf(r) >= 0;
  }

  function curie(r) {
    for (var i = 0; i < CURIES.length; i++) {
      if (r.indexOf(CURIES[i][0]) === 0) return CURIES[i][1] + r.slice(CURIES[i][0].length);
    }
    return r;
  }

  function localName(v) { return v.split(/[#/]/).pop() || v; }

  // J9: UML multiplicity of combined bounds; '' for the unstated default 0..*.
  // Bounds from the highest-ranked source win; within it the tightest
  // (largest min, smallest max).
  function multiplicity(bounds) {
    var top = 0;
    bounds.forEach(function (b) { top = Math.max(top, SOURCE_RANK[b.source] || 1); });
    var min = 0, max = Infinity;
    bounds.forEach(function (b) {
      if ((SOURCE_RANK[b.source] || 1) !== top) return;
      if (b.min != null) min = Math.max(min, b.min);
      if (b.max != null) max = Math.min(max, b.max);
    });
    if (min === 0 && max === Infinity) return '';
    var hi = max === Infinity ? '*' : String(max);
    return String(min) === hi ? hi : min + '..' + hi;
  }

  // Read the facts into lookups; uses are grouped by their (blank) subject.
  function readFacts(triples) {
    var f = { member: {}, known: {}, kinds: {}, subclass: [], uses: {}, labels: {}, oneOf: {} };
    triples.forEach(function (t) {
      var o = t.o.value;
      var rank = LABEL_PREDICATES.indexOf(t.p);
      if (rank >= 0 && t.o.type === 'literal') {
        // Best-ranked predicate per (subject, language).
        var byLang = f.labels[t.s] = f.labels[t.s] || {};
        var cur = byLang[t.o.language];
        if (!cur || rank < cur.rank) byLang[t.o.language] = { rank: rank, values: [o] };
        else if (rank === cur.rank && cur.values.indexOf(o) < 0) cur.values.push(o);
        return;
      }
      if (t.p === RDF_TYPE) {
        if (o === U + 'Member') f.member[t.s] = true;
        else if (o === U + 'Known') f.known[t.s] = true;
        else if (o === U + 'Use') f.uses[t.s] = f.uses[t.s] || {};
        else (f.kinds[t.s] = f.kinds[t.s] || {})[o] = true;
        return;
      }
      if (t.p === RDFS_SUBCLASS) { f.subclass.push([t.s, o]); return; }
      if (t.p === U + 'oneOf') { (f.oneOf[t.s] = f.oneOf[t.s] || []).push(t.o); return; }
      if (t.p === U + 'in') {
        var use = f.uses[t.s] = f.uses[t.s] || {};
        (use['in'] = use['in'] || []).push(t.o);
        return;
      }
      if (t.p.indexOf(U) === 0) {
        (f.uses[t.s] = f.uses[t.s] || {})[t.p.slice(U.length)] = o;
      }
    });
    return f;
  }

  function project(triples) {
    var f = readFacts(triples);
    var out = [];
    var seen = {};
    function emit(s, p, o) {
      var key = s + ' ' + p + ' ' + o.type + ' ' + o.value + '@' + (o.language || '');
      if (seen[key]) return;
      seen[key] = true;
      out.push({ s: s, p: p, o: o });
    }
    function hasKind(t, kinds) {
      var k = f.kinds[t];
      return !!k && kinds.some(function (x) { return k[x]; });
    }
    // J3: also any range the store declares rdfs:Datatype.
    function isValue(r) { return isDatatype(r) || hasKind(r, [RDFS + 'Datatype']); }
    // J6: a drawn end that belongs to neither the ontology nor its imports.
    function classEnd(c) {
      if (!f.member[c] && !f.known[c] && !isValue(c)) emit(c, RDF_TYPE, iri(U + 'ExternalClass'));
    }
    function placeholder(id, label) {
      emit(id, RDF_TYPE, iri(U + 'AnyResource'));
      emit(id, RDFS_LABEL, literal(label));
    }
    function declared(v) { return v && UNDECLARED.indexOf(v) < 0 ? v : null; }
    // J13: an enumeration value as a row: an IRI by its label (untagged or
    // English first: a row is one string), a literal as written.
    function valueText(o) {
      if (o.type !== 'iri') return o.value;
      var byLang = f.labels[o.value];
      if (byLang) {
        var lang = ['', 'en'].filter(function (l) { return byLang[l]; })[0] || Object.keys(byLang)[0];
        return byLang[lang].values[0];
      }
      return curie(o.value) === o.value ? localName(o.value) : curie(o.value);
    }
    var enumValues = {};  // enumeration box id -> [term]
    function enumerationRows(id, values) {
      values.forEach(function (v) { emit(id, U + 'value', literal(valueText(v))); });
    }
    function number(v) { return v == null || isNaN(parseInt(v, 10)) ? null : parseInt(v, 10); }

    // J1: member classes.
    Object.keys(f.member).forEach(function (m) {
      if (hasKind(m, CLASS_KINDS)) emit(m, RDF_TYPE, iri(OWL_CLASS));
    });

    var multTypes = {};   // filled by draw(), labelled after the uses

    // Normalize the uses and group them by property.
    var byProperty = {};
    Object.keys(f.uses).forEach(function (u) {
      var use = f.uses[u];
      var p = use.property;
      if (!p || SKIPPED_PROPERTIES.indexOf(p) >= 0) return;
      var r = declared(use.range);
      // J13: sh:in makes the range an enumeration box of its own.
      if (use['in']) {
        // Each value arrives on a row (so a use) of its own: collect them.
        r = U + 'enum:' + (use['class'] || '') + '|' + p;
        enumValues[r] = (enumValues[r] || []).concat(use['in']);
      }
      var bounds = [];
      if (use.min != null || use.max != null) {
        bounds.push({ min: number(use.min), max: number(use.max), source: use.source });
      }
      if (hasKind(p, [OWL + 'FunctionalProperty'])) bounds.push({ min: null, max: 1, source: 'owl' });
      (byProperty[p] = byProperty[p] || []).push({ c: declared(use['class']), r: r, bounds: bounds, source: use.source });
    });

    // J8: merge per property. A use that knows less than another one of the
    // same property folds into it: without a class when another use has one,
    // without a range when another use of the same class has one (its bounds
    // join that class's edges). A class's use that still has no range takes
    // the property's global ranges (rdfs:range, schema:rangeIncludes): an OWL
    // cardinality restriction or a SHACL minCount says how many, not what.
    Object.keys(byProperty).forEach(function (p) {
      var uses = byProperty[p];
      var defaultRanges = [];
      uses.forEach(function (u) {
        if (u.r && (u.source === 'rdfs' || u.source === 'schema' || !u.c) && defaultRanges.indexOf(u.r) < 0) {
          defaultRanges.push(u.r);
        }
      });
      var classed = uses.filter(function (u) { return u.c; });
      if (classed.length) uses = classed;
      var byClass = {};
      uses.forEach(function (u) {
        var k = u.c || '';
        var g = byClass[k] = byClass[k] || { ranges: {}, bounds: [] };
        var into = u.r ? (g.ranges[u.r] = g.ranges[u.r] || []) : g.bounds;
        into.push.apply(into, u.bounds);
      });
      Object.keys(byClass).forEach(function (k) {
        var g = byClass[k];
        var ranges = Object.keys(g.ranges);
        if (!ranges.length) {
          // J3: an owl:DatatypeProperty without any range is an rdfs:Literal.
          if (!defaultRanges.length && hasKind(p, [OWL + 'DatatypeProperty'])) defaultRanges = [RDFS + 'Literal'];
          defaultRanges.forEach(function (r) { g.ranges[r] = []; });
          ranges = defaultRanges.length ? defaultRanges : [null];
        }
        ranges.forEach(function (r) {
          draw(p, k || null, r, multiplicity((g.ranges[r] || []).concat(g.bounds)));
        });
      });
    });

    // J3–J5, J9: one use is an attribute row or an edge.
    function draw(p, c, r, mult) {
      // What a missing axiom means: owl:Thing for OWL, rdfs:Resource for RDFS.
      var anyLabel = hasKind(p, OWL_PROPERTY_KINDS) ? 'Thing' : 'Resource';
      if (r && isValue(r)) {
        // Domain-less attributes share one box per label: an attribute has no
        // far end to loop back to.
        var owner = c || U + 'any-domain:' + anyLabel;
        if (c) classEnd(c); else placeholder(owner, anyLabel);
        emit(owner, p, literal(curie(r) + (mult ? ' [' + mult + ']' : '')));
        return;
      }
      // Edges get one placeholder per property, so domain-less properties do
      // not collapse into self-loops on one hub.
      var from = c || U + 'any-domain:' + p;
      var to = r || U + 'any-range:' + p;
      if (c) classEnd(c); else placeholder(from, anyLabel);
      if (r && enumValues[r]) {
        emit(r, RDF_TYPE, iri(U + 'Enumeration'));
        enumerationRows(r, enumValues[r]);
      } else if (r) classEnd(r); else placeholder(to, anyLabel);
      emit(from, mult ? multLinkType(p, mult) : p, iri(to));
    }

    // A link type's label is global in GE, so an edge with a multiplicity gets
    // a link type of its own per (property, multiplicity), labelled "p [m]".
    function multLinkType(p, mult) {
      var id = U + 'mult:' + mult + ':' + p;
      multTypes[id] = { p: p, mult: mult };
      return id;
    }
    Object.keys(multTypes).forEach(function (id) {
      var m = multTypes[id];
      var byLang = f.labels[m.p] || { '': { values: [localName(m.p)] } };
      Object.keys(byLang).forEach(function (lang) {
        byLang[lang].values.forEach(function (v) { emit(id, RDFS_LABEL, literal(v.trim() + ' [' + m.mult + ']', lang)); });
      });
    });

    // J13: a member class defined by owl:oneOf lists its individuals.
    Object.keys(f.oneOf).forEach(function (c) { enumerationRows(c, f.oneOf[c]); });

    // J2: generalizations, owl:Thing left out. A member's are always drawn;
    // one an imported term declares (DCAT-AP CH: dct:Location under
    // dct:LocationPeriodOrJurisdiction) only when that term is on the diagram
    // already — imported terms never add boxes of their own (M6).
    var drawn = {};
    out.forEach(function (t) {
      drawn[t.s] = true;
      if (t.o.type === 'iri' && t.p !== RDF_TYPE) drawn[t.o.value] = true;
    });
    f.subclass.forEach(function (sc) {
      if (!declared(sc[1])) return;
      if (!f.member[sc[0]] && !drawn[sc[0]]) return;
      emit(sc[0], RDFS_SUBCLASS, iri(sc[1]));
      classEnd(sc[0]);
      classEnd(sc[1]);
    });

    // S3/J12: the best label per language, as rdfs:label.
    Object.keys(f.labels).forEach(function (s) {
      var byLang = f.labels[s];
      Object.keys(byLang).forEach(function (lang) {
        byLang[lang].values.forEach(function (v) { emit(s, RDFS_LABEL, literal(v, lang)); });
      });
    });
    return out;
  }

  var api = { project: project, isDatatype: isDatatype, curie: curie, multiplicity: multiplicity };
  if (typeof module === 'object' && module.exports) module.exports = api;
  else window.VisotoOntologyProjection = api;
})();
