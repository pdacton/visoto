// Tests for static/js/visoto-icons.js (run: node --test tests/js/).
// resolve() mirrors Resolver.Resolve in internal/icon/icon.go; these cases mirror
// internal/icon/icon_test.go so the two stay in step.
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const I = require('../../static/js/visoto-icons.js');

const BASE = '/static/img/resource/';
const CONFIG = {
  manifestUrl: '/api/icons.json?v=test',
  base: BASE,
  defaults: { page: BASE + 'default.svg', class: BASE + 'defaultClass.svg', instance: BASE + 'defaultInstance.svg' },
};
const CANTON = 'https://schema.ld.admin.ch/Canton';
const DEFINED_TERM = 'http://schema.org/DefinedTerm';

test.beforeEach(() => I.use(CONFIG, { Canton: true, Municipality: true, 'DefinedTerm.fallback': true }));

test('localName', () => {
  assert.equal(I.localName(CANTON), 'Canton');
  assert.equal(I.localName('http://www.w3.org/2004/02/skos/core#Concept'), 'Concept');
  assert.equal(I.localName('schema%3APerson'), 'Person');
  assert.equal(I.localName('skos:Concept'), 'Concept');
  assert.equal(I.localName('https://example.org/Person/'), 'Person');
  assert.equal(I.localName(''), '');
});

test('own name, then types', () => {
  assert.equal(I.resolve({ iri: CANTON }, 'cell'), BASE + 'Canton.svg');
  assert.equal(I.resolve({ iri: 'https://ld.admin.ch/canton/1', types: [CANTON] }, 'cell'), BASE + 'Canton.svg');
  assert.equal(I.resolve({ iri: 'https://schema.ld.admin.ch/Municipality', types: [CANTON] }, 'cell'), BASE + 'Municipality.svg');
});

test('any exact type beats any fallback', () => {
  assert.equal(I.resolve({ iri: 'https://ld.admin.ch/canton/1', types: [DEFINED_TERM, CANTON] }, 'cell'), BASE + 'Canton.svg');
  assert.equal(I.resolve({ iri: 'https://example.org/term/42', types: [DEFINED_TERM] }, 'cell'), BASE + 'DefinedTerm.fallback.svg');
  assert.equal(I.resolve({ iri: DEFINED_TERM, types: [CANTON] }, 'cell'), BASE + 'DefinedTerm.fallback.svg');
});

test('a miss returns the default of its kind', () => {
  const miss = { iri: 'https://example.org/x/1' };
  assert.equal(I.resolve(miss, 'cell'), '');
  assert.equal(I.resolve(miss, 'page'), BASE + 'default.svg');
  assert.equal(I.resolve(miss, 'class'), BASE + 'defaultClass.svg');
  assert.equal(I.resolve(miss, 'instance'), BASE + 'defaultInstance.svg');
  assert.equal(I.resolve({ types: [] }, 'cell'), '');
});

test('stamp sets the own-IRI icon and never replaces an image', () => {
  assert.equal(I.stamp({}, CANTON).image, BASE + 'Canton.svg');
  assert.equal(I.stamp({ image: 'https://img.example/x.png' }, CANTON).image, 'https://img.example/x.png');
  assert.equal(I.stamp({}, 'https://example.org/x/1').image, undefined);
});

test('isDefault recognises exactly the configured defaults', () => {
  assert.equal(I.isDefault(BASE + 'defaultClass.svg'), true);
  assert.equal(I.isDefault(BASE + 'Canton.svg'), false);
  assert.equal(I.isDefault(''), false);
});

test('load() resolves even without a manifest URL', async () => {
  I.use({ base: BASE, defaults: {} }, {});
  await I.load();
  assert.equal(I.resolve({ iri: CANTON }, 'cell'), '');
});
