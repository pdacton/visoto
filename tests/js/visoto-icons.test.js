'use strict';
// Mirror of internal/icon's Resolve tests: the JS resolver must pick the same
// icon as the Go one for the same icon.Names() map.
const test = require('node:test');
const assert = require('node:assert');

global.window = {};
require('../../static/js/visoto-icons.js');
const { resolve } = global.window.VisotoIcons;

const RDF = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#';
const available = {
  'Canton': 'Canton.svg',
  'Property': 'Property.svg',
  'DefinedTerm.fallback': 'DefinedTerm.fallback.svg',
  [RDF + 'Property']: 'rdf/Property.svg',
  'rdf:Property': 'rdf/Property.svg',
};
const B = '/static/img/resource/';

test('namespaced icon serves its namespace in IRI, CURIE and encoded form', () => {
  assert.strictEqual(resolve(RDF + 'Property', [], available), B + 'rdf/Property.svg');
  assert.strictEqual(resolve('rdf:Property', [], available), B + 'rdf/Property.svg');
  assert.strictEqual(resolve('rdf%3AProperty', [], available), B + 'rdf/Property.svg');
  assert.strictEqual(resolve('', [RDF + 'Property'], available), B + 'rdf/Property.svg');
});

test('other namespaces fall back to the root icon', () => {
  assert.strictEqual(resolve('http://schema.org/Property', [], available), B + 'Property.svg');
});

test('exact type beats fallback type regardless of order', () => {
  const types = ['http://schema.org/DefinedTerm', 'https://schema.ld.admin.ch/Canton'];
  assert.strictEqual(resolve('https://ld.admin.ch/canton/1', types, available), B + 'Canton.svg');
  assert.strictEqual(resolve('https://x.org/1', types.slice(0, 1), available), B + 'DefinedTerm.fallback.svg');
});

test('miss returns empty string', () => {
  assert.strictEqual(resolve('https://x.org/1', ['https://x.org/Nope'], available), '');
  assert.strictEqual(resolve('x', [], null), '');
});
