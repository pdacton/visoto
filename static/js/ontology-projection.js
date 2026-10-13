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
    _:u a u:Use ; u:property ?P ; u:class ?C ; u:range ?R ; u:source "rdfs"
                                one use of P; class and range may be absent
    ?x rdfs:label ?l            passed through

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
  var CURIES = [[XSD, 'xsd:'], [RDFS, 'rdfs:'], [RDF, 'rdf:']];

  function iri(v) { return { type: 'iri', value: v }; }
  function literal(v) { return { type: 'literal', value: v, language: '' }; }

  // J3: a range that is a value, not a resource.
  function isDatatype(r) {
    return r.indexOf(XSD) === 0 || LITERAL_TYPES.indexOf(r) >= 0;
  }

  function curie(r) {
    for (var i = 0; i < CURIES.length; i++) {
      if (r.indexOf(CURIES[i][0]) === 0) return CURIES[i][1] + r.slice(CURIES[i][0].length);
    }
    return r;
  }

  // Read the facts into lookups; uses are grouped by their (blank) subject.
  function readFacts(triples) {
    var f = { member: {}, known: {}, kinds: {}, subclass: [], uses: {}, labels: [] };
    triples.forEach(function (t) {
      var o = t.o.value;
      if (t.p === RDFS_LABEL) { f.labels.push(t); return; }
      if (t.p === RDF_TYPE) {
        if (o === U + 'Member') f.member[t.s] = true;
        else if (o === U + 'Known') f.known[t.s] = true;
        else if (o === U + 'Use') f.uses[t.s] = f.uses[t.s] || {};
        else (f.kinds[t.s] = f.kinds[t.s] || {})[o] = true;
        return;
      }
      if (t.p === RDFS_SUBCLASS) { f.subclass.push([t.s, o]); return; }
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
      var key = s + ' ' + p + ' ' + o.type + ' ' + o.value;
      if (seen[key]) return;
      seen[key] = true;
      out.push({ s: s, p: p, o: o });
    }
    function hasKind(t, kinds) {
      var k = f.kinds[t];
      return !!k && kinds.some(function (x) { return k[x]; });
    }
    // J6: a drawn end that belongs to neither the ontology nor its imports.
    function classEnd(c) {
      if (!f.member[c] && !f.known[c] && !isDatatype(c)) emit(c, RDF_TYPE, iri(U + 'ExternalClass'));
    }
    function placeholder(id, label) {
      emit(id, RDF_TYPE, iri(U + 'AnyResource'));
      emit(id, RDFS_LABEL, literal(label));
    }
    function declared(v) { return v && UNDECLARED.indexOf(v) < 0 ? v : null; }

    // J1: member classes.
    Object.keys(f.member).forEach(function (m) {
      if (hasKind(m, CLASS_KINDS)) emit(m, RDF_TYPE, iri(OWL_CLASS));
    });

    // J3–J5: each use is an attribute row or an edge.
    Object.keys(f.uses).forEach(function (u) {
      var use = f.uses[u];
      var p = use.property;
      if (!p || SKIPPED_PROPERTIES.indexOf(p) >= 0) return;
      var c = declared(use['class']);
      var r = declared(use.range);
      if (!r && hasKind(p, [OWL + 'DatatypeProperty'])) r = RDFS + 'Literal';
      // What a missing axiom means: owl:Thing for OWL, rdfs:Resource for RDFS.
      var anyLabel = hasKind(p, OWL_PROPERTY_KINDS) ? 'Thing' : 'Resource';

      if (r && isDatatype(r)) {
        // Domain-less attributes share one box per label: an attribute has no
        // far end to loop back to.
        var owner = c || U + 'any-domain:' + anyLabel;
        if (c) classEnd(c); else placeholder(owner, anyLabel);
        emit(owner, p, literal(curie(r)));
        return;
      }
      // Edges get one placeholder per property, so domain-less properties do
      // not collapse into self-loops on one hub.
      var from = c || U + 'any-domain:' + p;
      var to = r || U + 'any-range:' + p;
      if (c) classEnd(c); else placeholder(from, anyLabel);
      if (r) classEnd(r); else placeholder(to, anyLabel);
      emit(from, p, iri(to));
    });

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

    f.labels.forEach(function (t) { emit(t.s, t.p, t.o); });
    return out;
  }

  var api = { project: project, isDatatype: isDatatype, curie: curie };
  if (typeof module === 'object' && module.exports) module.exports = api;
  else window.VisotoOntologyProjection = api;
})();
