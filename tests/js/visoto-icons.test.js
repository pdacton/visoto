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

// ---- the resource's own icon (the "icon" magic property) ----

const P = require('../../static/js/prefixes.js');
const THUMB = 'http://schema.org/thumbnail';
const FOAF_THUMB = 'http://xmlns.com/foaf/0.1/thumbnail';
const WITH_PROPS = Object.assign({}, CONFIG, { properties: ['schema:thumbnail', '<' + FOAF_THUMB + '>'] });

function withOwnIcons() {
  global.window = { VisotoPrefixes: { fromPage: () => P.create([{ name: 'schema', iri: 'http://schema.org/' }]) } };
  I.use(WITH_PROPS, { Canton: true, Person: true, 'DefinedTerm.fallback': true });
}

test('override accepts https, site paths and names; refuses the rest (mirrors icon.override)', () => {
  withOwnIcons();
  const cases = [
    ['https://img.example/zh.svg', 'https://img.example/zh.svg'],
    ['/static/img/custom/zh.svg', '/static/img/custom/zh.svg'],
    ['Person', BASE + 'Person.svg'],
    ['Person.svg', BASE + 'Person.svg'],
    ['DefinedTerm.fallback', BASE + 'DefinedTerm.fallback.svg'],
    ['  Person ', BASE + 'Person.svg'],
    ['Nope', ''],
    ['DefinedTerm', ''],
    ['javascript:alert(1)', ''],
    ['data:image/svg+xml,<svg/>', ''],
    ['//evil.example/x.svg', ''],
    ['http://img.example/zh.png', ''],
    ['', ''],
  ];
  for (const [value, want] of cases) assert.equal(I.override(value), want, value);
});

test('a usable override beats the own name, the types and the default', () => {
  withOwnIcons();
  assert.equal(I.resolve({ iri: CANTON, override: 'Person' }, 'cell'), BASE + 'Person.svg');
  assert.equal(I.resolve({ iri: 'https://example.org/x', override: 'https://img.example/a.svg' }, 'class'), 'https://img.example/a.svg');
  assert.equal(I.resolve({ iri: CANTON, override: 'javascript:x' }, 'cell'), BASE + 'Canton.svg');
});

test('properties expands CURIEs and <IRIs>, skips the unusable', () => {
  withOwnIcons();
  assert.deepEqual(I.properties(), [THUMB, FOAF_THUMB]);
  I.use(Object.assign({}, CONFIG, { properties: ['undeclared:thumb', 'schema:a/schema:b', '^schema:thumbnail'] }), {});
  const orig = console.warn; console.warn = () => {};
  try { assert.deepEqual(I.properties(), []); } finally { console.warn = orig; }
  I.use(CONFIG, {});
  assert.deepEqual(I.properties(), []);
});

test('pick: earliest property, then smallest value, validated after the pick', () => {
  withOwnIcons();
  assert.equal(I.pick([{ property: FOAF_THUMB, value: 'Canton' }, { property: THUMB, value: 'Person' }]), BASE + 'Person.svg');
  assert.equal(I.pick([{ property: THUMB, value: 'https://b.example/x.svg' }, { property: THUMB, value: 'https://a.example/x.svg' }]), 'https://a.example/x.svg');
  // Like the server: the best value is chosen first; an unusable one is no icon.
  assert.equal(I.pick([{ property: THUMB, value: 'javascript:x' }, { property: FOAF_THUMB, value: 'Person' }]), '');
  assert.equal(I.pick([{ property: 'http://example.org/other', value: 'Person' }]), '');
  assert.equal(I.pick([]), '');
});

test('fetchOverrides sends one SELECT and maps each resource to its icon', async () => {
  withOwnIcons();
  const sent = [];
  const select = (q) => {
    sent.push(q);
    return Promise.resolve([
      { inst: { type: 'uri', value: 'https://ld.admin.ch/canton/1' }, prop: { type: 'uri', value: THUMB }, icon: { type: 'uri', value: 'https://img.example/zh.svg' } },
      { inst: { type: 'uri', value: 'https://ld.admin.ch/canton/2' }, prop: { type: 'uri', value: THUMB }, icon: { type: 'literal', value: 'javascript:x' } },
      { inst: { type: 'uri', value: 'https://ld.admin.ch/canton/3' }, prop: { type: 'uri', value: FOAF_THUMB }, icon: { type: 'bnode', value: 'b0' } },
    ]);
  };
  const got = await I.fetchOverrides(['https://ld.admin.ch/canton/1', 'https://ld.admin.ch/canton/2', 'https://ld.admin.ch/canton/3', 'not an iri'], select);
  assert.deepEqual(got, { 'https://ld.admin.ch/canton/1': 'https://img.example/zh.svg' });
  assert.equal(sent.length, 1);
  assert.match(sent[0], /VALUES \?prop \{ <http:\/\/schema\.org\/thumbnail> <http:\/\/xmlns\.com\/foaf\/0\.1\/thumbnail> \}/);
  assert.doesNotMatch(sent[0], /not an iri/);
});

test('fetchOverrides asks nothing without properties, and swallows failures', async () => {
  I.use(CONFIG, {});
  let calls = 0;
  assert.deepEqual(await I.fetchOverrides([CANTON], () => { calls++; return Promise.resolve([]); }), {});
  assert.equal(calls, 0);

  withOwnIcons();
  const orig = console.warn; console.warn = () => {};
  try {
    assert.deepEqual(await I.fetchOverrides([CANTON], () => Promise.reject(new Error('down'))), {});
  } finally { console.warn = orig; }
});
