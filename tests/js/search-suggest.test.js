// Tests for static/js/search-suggest.js (run: node --test tests/js/).
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const Suggest = require('../../static/js/search-suggest.js');
const Prefixes = require('../../static/js/prefixes.js');

const P = Prefixes.create([
  { name: 'schema', iri: 'https://schema.org/' },
  { name: 'i14y', iri: 'https://register.ld.admin.ch/i14y/' },
]);

test('classify: full IRIs open directly, with or without angle brackets', () => {
  assert.deepEqual(Suggest.classify('https://ld.admin.ch/municipality/3871', P),
    { kind: 'iri', iri: 'https://ld.admin.ch/municipality/3871' });
  assert.deepEqual(Suggest.classify('  <http://example.org/a#b>  ', P),
    { kind: 'iri', iri: 'http://example.org/a#b' });
  assert.deepEqual(Suggest.classify('urn:uuid:1234-5678', P),
    { kind: 'iri', iri: 'urn:uuid:1234-5678' });
});

test('classify: a CURIE opens only when its prefix is declared', () => {
  assert.deepEqual(Suggest.classify('schema:Person', P), { kind: 'curie', iri: 'https://schema.org/Person' });
  assert.deepEqual(Suggest.classify('nope:Person', P), { kind: 'text' });
  assert.deepEqual(Suggest.classify('schema:Person', null), { kind: 'text' });
});

test('classify: ordinary text stays text', () => {
  for (const s of ['Bern', 'Note: x', '10:30', 'http', 'https://', 'schema:', '', '   ', 'http://a b']) {
    assert.equal(Suggest.classify(s, P).kind, 'text', JSON.stringify(s));
  }
});

test('collapse: versions of one resource fold into its shortest IRI', () => {
  const rows = Suggest.collapse([
    { iri: 'https://x/concept/DV_AGGL/351/version/2020.0.0', label: 'Bern', type: 'DefinedTerm' },
    { iri: 'https://x/concept/DV_AGGL/351/version/2024.1.0', label: 'Bern', type: 'DefinedTerm' },
    { iri: 'https://x/concept/DV_AGGL/351', label: 'Bern', type: 'DefinedTerm' },
    { iri: 'https://ld.admin.ch/municipality/351', label: 'Bern', type: 'Municipality' },
  ]);
  assert.deepEqual(rows, [
    { iri: 'https://x/concept/DV_AGGL/351', label: 'Bern', type: 'DefinedTerm', extra: 2 },
    { iri: 'https://ld.admin.ch/municipality/351', label: 'Bern', type: 'Municipality', extra: 0 },
  ]);
});

test('collapse: same label and type but unrelated IRIs stay separate', () => {
  const rows = Suggest.collapse([
    { iri: 'https://x/concept/DV_AGGL/351', label: 'Bern', type: 'DefinedTerm' },
    { iri: 'https://x/concept/MSREG/11/version/1.0.0', label: 'Bern', type: 'DefinedTerm' },
    { iri: 'https://x/concept/DV_AGGL/3510', label: 'Bern', type: 'DefinedTerm' },
  ]);
  assert.deepEqual(rows.map(r => [r.iri, r.extra]), [
    ['https://x/concept/DV_AGGL/351', 0],
    ['https://x/concept/MSREG/11/version/1.0.0', 0],
    ['https://x/concept/DV_AGGL/3510', 0],
  ]);
});

test('collapse: skips hits without an IRI and labels by IRI when unlabelled', () => {
  const rows = Suggest.collapse([{ label: 'x' }, null, { iri: 'urn:a:1' }]);
  assert.deepEqual(rows, [{ iri: 'urn:a:1', label: 'urn:a:1', type: '', extra: 0 }]);
  assert.deepEqual(Suggest.collapse('nope'), []);
});

test('shortIri: CURIE when a prefix covers it, else without the scheme', () => {
  assert.equal(Suggest.shortIri('https://register.ld.admin.ch/i14y/concept/X', P), 'i14y:concept/X');
  assert.equal(Suggest.shortIri('https://ld.admin.ch/municipality/351', P), 'ld.admin.ch/municipality/351');
  assert.equal(Suggest.shortIri('https://schema.org/', P), 'schema.org/');
});

const D = Prefixes.create([
  { name: 'dcat', iri: 'http://www.w3.org/ns/dcat#' },
  { name: 'dc', iri: 'http://purl.org/dc/elements/1.1/' },
  { name: 'dcterms', iri: 'http://purl.org/dc/terms/' },
  { name: 'schema', iri: 'https://schema.org/' },
]);

test('curieParts: declared prefix with or without a local name', () => {
  assert.deepEqual(Suggest.curieParts('dcat:', D), { name: 'dcat', ns: 'http://www.w3.org/ns/dcat#', local: '' });
  assert.deepEqual(Suggest.curieParts(' dcat:Data ', D), { name: 'dcat', ns: 'http://www.w3.org/ns/dcat#', local: 'Data' });
  for (const s of ['nope:', 'dcat', 'dcat: Data', 'https://x.org/', 'Bern']) {
    assert.equal(Suggest.curieParts(s, D), null, s);
  }
});

test('prefixMatches: declared prefixes starting with the text, shortest first', () => {
  assert.deepEqual(Suggest.prefixMatches('dc', D).map(p => p.name), ['dc', 'dcat', 'dcterms']);
  assert.deepEqual(Suggest.prefixMatches('DCA', D).map(p => p.name), ['dcat']);
  assert.deepEqual(Suggest.prefixMatches('Bern', D), []);
  assert.deepEqual(Suggest.prefixMatches('dc', D, 2).map(p => p.name), ['dc', 'dcat']);
});

const tree = {
  results: { bindings: [
    { class: { value: 'http://www.w3.org/ns/dcat#Dataset' }, label: { value: 'Dataset' }, instcount: { value: '120' } },
    { class: { value: 'http://www.w3.org/ns/dcat#Dataset' }, parent: { value: 'x' } },
    { class: { value: 'http://www.w3.org/ns/dcat#Distribution' }, instcount: { value: '900' } },
    { class: { value: 'http://www.w3.org/ns/dcat#Catalog' } },
    { class: { value: 'https://schema.org/Dataset' }, instcount: { value: '5' } },
  ] },
};

test('classesFromTree: one entry per class, label and count kept', () => {
  const ks = Suggest.classesFromTree(tree);
  assert.equal(ks.length, 4);
  assert.deepEqual(ks[0], { iri: 'http://www.w3.org/ns/dcat#Dataset', label: 'Dataset', count: 120 });
  assert.equal(ks[2].count, null);
  assert.deepEqual(Suggest.classesFromTree(null), []);
});

test('matchClasses: namespace and local prefix, most instances first', () => {
  const ks = Suggest.classesFromTree(tree);
  const ns = 'http://www.w3.org/ns/dcat#';
  assert.deepEqual(Suggest.matchClasses(ks, ns, '').map(k => k.iri.slice(ns.length)), ['Distribution', 'Dataset', 'Catalog']);
  assert.deepEqual(Suggest.matchClasses(ks, ns, 'da').map(k => k.iri.slice(ns.length)), ['Dataset']);
  assert.deepEqual(Suggest.matchClasses(ks, ns, 'x'), []);
});
