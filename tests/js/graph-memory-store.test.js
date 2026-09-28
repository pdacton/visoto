// Tests for makeHybridProvider in static/js/graph-memory-store.js
// (run: node --test tests/js/). The live endpoint is a stub that records calls.
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
// buildStore resolves own-IRI icons through visoto-icons.js; none here.
global.window = { VisotoIcons: { resolve: () => '' } };
const M = require('../../static/js/graph-memory-store.js');

const RDF_TYPE = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type';
const SUBCLASS = 'http://www.w3.org/2000/01/rdf-schema#subClassOf';
const OWL_CLASS = 'http://www.w3.org/2002/07/owl#Class';
const EX = 'https://ex.org/';

function iri(v) { return { type: 'iri', value: v }; }
function t(s, p, o) { return { s, p, o }; }

// FMIS subClassOf Software, plus a placeholder box linked to FMIS.
function diagram() {
  return M.buildStore([
    t(EX + 'FMIS', RDF_TYPE, iri(OWL_CLASS)),
    t(EX + 'Software', RDF_TYPE, iri(OWL_CLASS)),
    t(EX + 'FMIS', SUBCLASS, iri(EX + 'Software')),
    t('urn:visoto:any-domain:' + EX + 'p', EX + 'p', iri(EX + 'FMIS')),
  ], {});
}

function el(id, types) {
  return { id, types: types || [], label: { values: [] }, properties: {} };
}

function stubLive(answers) {
  const calls = [];
  const live = {};
  ['classInfo', 'propertyInfo', 'linkTypesInfo', 'elementInfo', 'linksInfo',
    'linkTypesOf', 'linkElements', 'filter'].forEach((m) => {
    live[m] = (params) => {
      calls.push({ m, params });
      const a = answers[m];
      if (a instanceof Error) return Promise.reject(a);
      return Promise.resolve(typeof a === 'function' ? a(params) : a);
    };
  });
  return { live, calls };
}

test('the first picture never asks the endpoint', async () => {
  const { live, calls } = stubLive({});
  const p = M.makeHybridProvider(diagram(), live);
  const ids = [EX + 'FMIS', EX + 'Software', 'urn:visoto:any-domain:' + EX + 'p'];
  const els = await p.elementInfo({ elementIds: ids });
  const links = await p.linksInfo({ elementIds: ids, linkTypeIds: [] });
  assert.deepEqual(Object.keys(els).sort(), ids.slice().sort());
  assert.equal(links.length, 2);
  assert.deepEqual(calls, []);
});

test('connections merge by link type without double counting', async () => {
  const { live } = stubLive({
    linkTypesOf: [
      { id: SUBCLASS, inCount: 0, outCount: 1 },
      { id: RDF_TYPE, inCount: 12, outCount: 1 },
    ],
  });
  const p = M.makeHybridProvider(diagram(), live);
  const counts = await p.linkTypesOf({ elementId: EX + 'FMIS' });
  const byId = Object.fromEntries(counts.map((c) => [c.id, c]));
  assert.deepEqual(byId[SUBCLASS], { id: SUBCLASS, inCount: 0, outCount: 1 });
  assert.deepEqual(byId[RDF_TYPE], { id: RDF_TYPE, inCount: 12, outCount: 1 });
  assert.equal(byId[EX + 'p'].inCount, 1);
});

test('placeholders never reach the endpoint', async () => {
  const { live, calls } = stubLive({ linkTypesOf: [], linkElements: {}, filter: {} });
  const p = M.makeHybridProvider(diagram(), live);
  const ph = 'urn:visoto:any-domain:' + EX + 'p';
  await p.linkTypesOf({ elementId: ph });
  await p.linkElements({ elementId: ph, linkId: EX + 'p', offset: 0 });
  await p.filter({ refElementId: ph, offset: 0, languageCode: 'en' });
  await p.elementInfo({ elementIds: [ph, 'urn:visoto:unknown'] });
  assert.deepEqual(calls, []);
});

test('a class picked in the tree lists its instances from the endpoint', async () => {
  const { live, calls } = stubLive({ filter: { [EX + 'i1']: el(EX + 'i1', [EX + 'FMIS']) } });
  const p = M.makeHybridProvider(diagram(), live);
  const res = await p.filter({ elementTypeId: EX + 'FMIS', offset: 0, languageCode: 'en' });
  assert.deepEqual(Object.keys(res), [EX + 'i1']);
  assert.equal(calls[0].m, 'filter');
});

test('text search stays on the diagram', async () => {
  const { live, calls } = stubLive({});
  const p = M.makeHybridProvider(diagram(), live);
  const res = await p.filter({ text: 'fmis', offset: 0, languageCode: 'en' });
  assert.deepEqual(Object.keys(res), [EX + 'FMIS']);
  assert.deepEqual(calls, []);
});

test('connection lists merge, and a diagram node keeps its own model', async () => {
  const fromLive = el(EX + 'Software', ['http://other/Type']);
  const { live } = stubLive({
    filter: { [EX + 'i1']: el(EX + 'i1'), [EX + 'Software']: fromLive },
  });
  const store = diagram();
  const p = M.makeHybridProvider(store, live);
  const res = await p.filter({ refElementId: EX + 'FMIS', offset: 0, languageCode: 'en' });
  assert.deepEqual(Object.keys(res).sort(), [EX + 'Software', EX + 'i1', 'urn:visoto:any-domain:' + EX + 'p'].sort());
  assert.equal(res[EX + 'Software'], store.elements[EX + 'Software']);
});

test('pulled-in nodes get endpoint data and links, but diagram pairs gain none', async () => {
  const { live, calls } = stubLive({
    elementInfo: { [EX + 'i1']: el(EX + 'i1', [EX + 'FMIS']) },
    linksInfo: [
      { sourceId: EX + 'i1', linkTypeId: RDF_TYPE, targetId: EX + 'FMIS' },
      { sourceId: EX + 'FMIS', linkTypeId: 'http://www.w3.org/2002/07/owl#equivalentClass', targetId: EX + 'Software' },
      { sourceId: EX + 'FMIS', linkTypeId: SUBCLASS, targetId: EX + 'Software' },
    ],
  });
  const p = M.makeHybridProvider(diagram(), live);
  const ids = [EX + 'FMIS', EX + 'Software', EX + 'i1', 'urn:visoto:any-domain:' + EX + 'p'];
  const els = await p.elementInfo({ elementIds: ids });
  assert.ok(els[EX + 'i1']);
  assert.deepEqual(calls[0].params.elementIds, [EX + 'i1']);
  const links = await p.linksInfo({ elementIds: ids, linkTypeIds: [] });
  const keys = links.map((l) => l.linkTypeId + ' ' + l.sourceId);
  assert.ok(keys.includes(RDF_TYPE + ' ' + EX + 'i1'));
  assert.ok(!keys.some((k) => k.startsWith('http://www.w3.org/2002/07/owl#equivalentClass')));
  assert.equal(links.filter((l) => l.linkTypeId === SUBCLASS).length, 1);
  assert.ok(!calls[1].params.elementIds.some((id) => id.startsWith('urn:')));
});

test('a failing endpoint leaves the diagram answer standing', async () => {
  const { live } = stubLive({ linkTypesOf: new Error('down'), linkElements: new Error('down') });
  const p = M.makeHybridProvider(diagram(), live);
  const counts = await p.linkTypesOf({ elementId: EX + 'FMIS' });
  assert.ok(counts.some((c) => c.id === SUBCLASS));
  const res = await p.linkElements({ elementId: EX + 'FMIS', linkId: SUBCLASS, offset: 0 });
  assert.deepEqual(Object.keys(res), [EX + 'Software']);
});

test('link type labels the diagram lacks come from the endpoint', async () => {
  const { live, calls } = stubLive({
    linkTypesInfo: [{ id: RDF_TYPE, label: { values: [{ value: 'type', language: '' }] } }],
  });
  const p = M.makeHybridProvider(diagram(), live);
  const res = await p.linkTypesInfo({ linkTypeIds: [RDF_TYPE] });
  assert.equal(res[0].label.values[0].value, 'type');
  assert.deepEqual(calls[0].params, { linkTypeIds: [RDF_TYPE] });
});
