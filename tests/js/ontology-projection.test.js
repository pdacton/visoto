// Tests for static/js/ontology-projection.js (run: node --test tests/js/).
// One case per drawing rule; IDs refer to .project/intent/ontology/ontology-construct.md.
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
global.window = { VisotoIcons: { resolve: () => '' } };
const M = require('../../static/js/graph-memory-store.js');
const P = require('../../static/js/ontology-projection.js');

const RDF = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#';
const RDFS = 'http://www.w3.org/2000/01/rdf-schema#';
const OWL = 'http://www.w3.org/2002/07/owl#';
const XSD = 'http://www.w3.org/2001/XMLSchema#';
const U = 'urn:visoto:';
const EX = 'https://ex.org/';

// Facts as N-Triples, the way the CONSTRUCT returns them.
function nt(lines) { return lines.join('\n'); }
function member(t, kind) { return `<${t}> <${RDF}type> <${U}Member> .\n<${t}> <${RDF}type> <${kind}> .`; }
function use(id, p, c, r) {
  return [
    `_:${id} <${RDF}type> <${U}Use> .`,
    `_:${id} <${U}property> <${p}> .`,
    c ? `_:${id} <${U}class> <${c}> .` : '',
    r ? `_:${id} <${U}range> <${r}> .` : '',
  ].join('\n');
}
function draw(text) { return M.buildStore(P.project(M.parseNTriples(text, 'q0_')), {}); }
function links(store) { return store.links.map((l) => `${l.sourceId} ${l.linkTypeId} ${l.targetId}`).sort(); }
function types(store, iri) { return store.elements[iri] ? store.elements[iri].types : null; }
function attr(store, iri, p) {
  const el = store.elements[iri];
  return el && el.properties[p] ? el.properties[p].values.map((v) => v.value) : null;
}

test('parseNTriples scopes blank nodes per response', () => {
  const line = `_:u0_u <${U}property> <${EX}p> .`;
  const a = M.parseNTriples(line, 'q0_');
  const b = M.parseNTriples(line, 'q1_');
  assert.equal(a[0].s, '_:q0_u0_u');
  assert.notEqual(a[0].s, b[0].s);
  assert.deepEqual(M.parseNTriples(`<${EX}a> <${EX}p> _:x .`, 'q0_')[0].o, { type: 'bnode', value: '_:q0_x' });
});

test('buildStore ignores blank nodes', () => {
  const store = M.buildStore(M.parseNTriples(`_:u <${U}property> <${EX}p> .\n<${EX}a> <${EX}p> _:x .`, 'q0_'), {});
  assert.deepEqual(Object.keys(store.elements), []);
});

test('J1: member classes are class boxes; member properties are not boxes', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), member(EX + 'B', RDFS + 'Class'), member(EX + 'p', OWL + 'ObjectProperty')]));
  assert.deepEqual(types(s, EX + 'A'), [OWL + 'Class']);
  assert.deepEqual(types(s, EX + 'B'), [OWL + 'Class']);
  assert.equal(s.elements[EX + 'p'], undefined);
});

test('J2: generalization of a member; owl:Thing is left out', () => {
  const s = draw(nt([
    member(EX + 'A', OWL + 'Class'), member(EX + 'B', OWL + 'Class'),
    `<${EX}A> <${RDFS}subClassOf> <${EX}B> .`,
    `<${EX}B> <${RDFS}subClassOf> <${OWL}Thing> .`,
  ]));
  assert.deepEqual(links(s), [`${EX}A ${RDFS}subClassOf ${EX}B`]);
});

test('J2/M6: an imported term\'s generalization is drawn only when the term is on the diagram', () => {
  const facts = [
    member(EX + 'Super', OWL + 'Class'), member(EX + 'p', OWL + 'ObjectProperty'),
    `<${EX}Imported> <${RDF}type> <${U}Known> .`,
    `<${EX}Imported> <${RDFS}subClassOf> <${EX}Super> .`,
  ];
  assert.deepEqual(links(draw(nt(facts))), []);
  const s = draw(nt(facts.concat([use('u1', EX + 'p', EX + 'Imported', EX + 'Super')])));
  assert.ok(links(s).includes(`${EX}Imported ${RDFS}subClassOf ${EX}Super`));
  assert.deepEqual(types(s, EX + 'Imported'), []);
});

test('J3/J4: a datatype range is an attribute row with a CURIE type', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), member(EX + 'name', OWL + 'DatatypeProperty'),
    use('u1', EX + 'name', EX + 'A', XSD + 'string'), use('u2', EX + 'note', EX + 'A', RDF + 'langString')]));
  assert.deepEqual(attr(s, EX + 'A', EX + 'name'), ['xsd:string']);
  assert.deepEqual(attr(s, EX + 'A', EX + 'note'), ['rdf:langString']);
  assert.deepEqual(links(s), []);
});

test('J3: owl:DatatypeProperty without range is an rdfs:Literal row', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), member(EX + 'v', OWL + 'DatatypeProperty'), use('u1', EX + 'v', EX + 'A')]));
  assert.deepEqual(attr(s, EX + 'A', EX + 'v'), ['rdfs:Literal']);
});

test('J4: an object range is an edge', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), member(EX + 'B', OWL + 'Class'), use('u1', EX + 'p', EX + 'A', EX + 'B')]));
  assert.deepEqual(links(s), [`${EX}A ${EX}p ${EX}B`]);
});

test('J5: missing ends are placeholders — per property for edges, shared per label for attributes', () => {
  const s = draw(nt([
    member(EX + 'p', OWL + 'ObjectProperty'), use('u1', EX + 'p'),
    member(EX + 'q', RDF + 'Property'), use('u2', EX + 'q'),
    member(EX + 'a', OWL + 'DatatypeProperty'), use('u3', EX + 'a', null, XSD + 'string'),
    member(EX + 'b', OWL + 'DatatypeProperty'), use('u4', EX + 'b', null, XSD + 'date'),
  ]));
  const dp = U + 'any-domain:' + EX + 'p';
  const rp = U + 'any-range:' + EX + 'p';
  assert.ok(links(s).includes(`${dp} ${EX}p ${rp}`));
  assert.deepEqual(types(s, dp), [U + 'AnyResource']);
  assert.equal(s.elements[dp].label.values[0].value, 'Thing');
  assert.equal(s.elements[U + 'any-domain:' + EX + 'q'].label.values[0].value, 'Resource');
  const shared = U + 'any-domain:Thing';
  assert.deepEqual(attr(s, shared, EX + 'a'), ['xsd:string']);
  assert.deepEqual(attr(s, shared, EX + 'b'), ['xsd:date']);
});

test('J5: owl:Thing and rdfs:Resource ends count as missing', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), member(EX + 'p', OWL + 'ObjectProperty'),
    use('u1', EX + 'p', EX + 'A', OWL + 'Thing'), use('u2', EX + 'q', RDFS + 'Resource', EX + 'A')]));
  assert.equal(s.elements[OWL + 'Thing'], undefined);
  assert.equal(s.elements[RDFS + 'Resource'], undefined);
  assert.ok(links(s).includes(`${EX}A ${EX}p ${U}any-range:${EX}p`));
  assert.ok(links(s).includes(`${U}any-domain:${EX}q ${EX}q ${EX}A`));
});

test('J6: non-member ends are external unless known or a datatype', () => {
  const s = draw(nt([
    member(EX + 'A', OWL + 'Class'), member(EX + 'p', OWL + 'ObjectProperty'),
    use('u1', EX + 'p', EX + 'A', 'https://other.org/X'),
    use('u2', EX + 'p', EX + 'A', 'https://imported.org/Y'),
    `<https://imported.org/Y> <${RDF}type> <${U}Known> .`,
    `<${EX}A> <${RDFS}subClassOf> <${XSD}string> .`,
  ]));
  assert.deepEqual(types(s, 'https://other.org/X'), [U + 'ExternalClass']);
  assert.deepEqual(types(s, 'https://imported.org/Y'), []);
  assert.deepEqual(types(s, XSD + 'string'), []);
});

test('J7: several domains or ranges draw several arrows', () => {
  const s = draw(nt([member(EX + 'p', OWL + 'ObjectProperty'),
    use('u1', EX + 'p', EX + 'A', EX + 'C'), use('u2', EX + 'p', EX + 'B', EX + 'C')]));
  assert.deepEqual(links(s), [`${EX}A ${EX}p ${EX}C`, `${EX}B ${EX}p ${EX}C`]);
});

test('S1: rdf:type and rdfs:label uses are skipped', () => {
  const s = draw(nt([use('u1', RDF + 'type', EX + 'A', EX + 'B'), use('u2', RDFS + 'label', EX + 'A', XSD + 'string')]));
  assert.deepEqual(Object.keys(s.elements), []);
});

test('labels pass through; a property label labels its edge', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), use('u1', EX + 'p', EX + 'A', EX + 'A'),
    `<${EX}A> <${RDFS}label> "A class"@en .`, `<${EX}p> <${RDFS}label> "has p"@en .`]));
  assert.equal(s.elements[EX + 'A'].label.values[0].value, 'A class');
  assert.equal(s.labels[EX + 'p'][0].value, 'has p');
});

test('blank-node uses from two responses do not merge', () => {
  const text0 = use('u0_u', EX + 'p', EX + 'A', EX + 'B');
  const text1 = use('u0_u', EX + 'q', EX + 'C', EX + 'D');
  const triples = M.parseNTriples(text0, 'q0_').concat(M.parseNTriples(text1, 'q1_'));
  const s = M.buildStore(P.project(triples), {});
  assert.deepEqual(links(s), [`${EX}A ${EX}p ${EX}B`, `${EX}C ${EX}q ${EX}D`]);
});

test('J3: schema.org datatypes are attribute rows', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), use('u1', EX + 'p', EX + 'A', 'http://schema.org/Text')]));
  assert.deepEqual(attr(s, EX + 'A', EX + 'p'), ['schema:Text']);
});

test('J8: a use without class lends its range to the classed uses and is not drawn itself', () => {
  // DCAT-AP style: schema:rangeIncludes without a domain, the class from elsewhere.
  const s = draw(nt([member(EX + 'A', OWL + 'Class'),
    use('u1', EX + 'p', null, XSD + 'string'), use('u2', EX + 'p', EX + 'A')]));
  assert.deepEqual(attr(s, EX + 'A', EX + 'p'), ['xsd:string']);
  assert.equal(s.elements[U + 'any-domain:Resource'], undefined);
});

test('J8: a use without range folds into a ranged use of the same class', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), member(EX + 'B', OWL + 'Class'),
    use('u1', EX + 'p', EX + 'A'), use('u2', EX + 'p', EX + 'A', EX + 'B'), use('u3', EX + 'p')]));
  assert.deepEqual(links(s), [`${EX}A ${EX}p ${EX}B`]);
});

test('J8/J9: multiplicity — highest source wins, tightest bounds within it', () => {
  assert.equal(P.multiplicity([]), '');
  assert.equal(P.multiplicity([{ min: 0, max: null, source: 'shacl' }]), '');
  assert.equal(P.multiplicity([{ min: 1, max: 1, source: 'shacl' }]), '1');
  assert.equal(P.multiplicity([{ min: 1, max: null, source: 'owl' }]), '1..*');
  assert.equal(P.multiplicity([{ min: null, max: 1, source: 'owl' }, { min: 1, max: null, source: 'shacl' }]), '1..*');
  assert.equal(P.multiplicity([{ min: 0, max: 3, source: 'shacl' }, { min: 1, max: 5, source: 'shacl' }]), '1..3');
});

test('J9: functional property → max 1 on the edge label and the attribute row', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'), member(EX + 'B', OWL + 'Class'),
    `<${EX}p> <${RDF}type> <${OWL}FunctionalProperty> .`, `<${EX}p> <${RDFS}label> "has p"@en .`,
    `<${EX}v> <${RDF}type> <${OWL}FunctionalProperty> .`,
    use('u1', EX + 'p', EX + 'A', EX + 'B'), use('u2', EX + 'v', EX + 'A', XSD + 'date')]));
  const lt = `${U}mult:0..1:${EX}p`;
  assert.deepEqual(links(s), [`${EX}A ${lt} ${EX}B`]);
  assert.equal(s.labels[lt][0].value, 'has p [0..1]');
  assert.deepEqual(attr(s, EX + 'A', EX + 'v'), ['xsd:date [0..1]']);
});

test('S3: the best-ranked label predicate wins per language', () => {
  const s = draw(nt([member(EX + 'A', OWL + 'Class'),
    `<${EX}A> <http://www.w3.org/2004/02/skos/core#prefLabel> "Pref"@en .`,
    `<${EX}A> <${RDFS}label> "Label"@en .`,
    `<${EX}A> <http://purl.org/dc/terms/title> "Titel"@de .`]));
  const vals = s.elements[EX + 'A'].label.values.map((v) => v.language + ':' + v.value).sort();
  assert.deepEqual(vals, ['de:Titel', 'en:Label']);
});
