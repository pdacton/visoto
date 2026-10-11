// Tests for static/js/prefixes.js (run: node --test tests/js/).
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const Prefixes = require('../../static/js/prefixes.js');

const P = Prefixes.create([
  { name: 'schema', iri: 'https://schema.org/' },
  { name: 'schema2', iri: 'https://schema.org/' },
  { name: 'ex', iri: 'http://example.org/' },
  { name: 'exv', iri: 'http://example.org/vocab/' },
]);

test('expand: declared CURIEs expand; unknown names and full IRIs pass through', () => {
  assert.equal(P.expand('schema:name'), 'https://schema.org/name');
  assert.equal(P.expand('nope:name'), 'nope:name');
  assert.equal(P.expand('http://schema.org/name'), 'http://schema.org/name');
  assert.equal(P.expand('plain'), 'plain');
});

test('expand follows the declared namespace in either direction', () => {
  const http = Prefixes.create([{ name: 'schema', iri: 'http://schema.org/' }]);
  assert.equal(http.expand('schema:isPartOf'), 'http://schema.org/isPartOf');
  assert.equal(P.expand('schema:isPartOf'), 'https://schema.org/isPartOf');
});

test('compact: longest namespace wins, first name wins a tie', () => {
  assert.deepEqual(P.compact('http://example.org/vocab/Thing'), { prefix: 'exv', local: 'Thing' });
  assert.deepEqual(P.compact('http://example.org/a'), { prefix: 'ex', local: 'a' });
  assert.deepEqual(P.compact('https://schema.org/Person'), { prefix: 'schema', local: 'Person' });
  assert.equal(P.compact('urn:x:1'), null);
});

test('sparqlBlock declares every entry', () => {
  const block = Prefixes.create([{ name: 'a', iri: 'urn:a:' }, { name: 'b', iri: 'urn:b:' }]).sparqlBlock();
  assert.equal(block, 'PREFIX a: <urn:a:>\nPREFIX b: <urn:b:>\n');
});

test('bad input yields an empty, harmless instance', () => {
  const empty = Prefixes.create(null);
  assert.deepEqual(empty.list, []);
  assert.equal(empty.expand('schema:name'), 'schema:name');
  assert.equal(empty.compact('https://schema.org/x'), null);
  assert.equal(empty.sparqlBlock(), '');
  assert.equal(Prefixes.fromPage().expand('schema:name'), 'schema:name'); // no DOM in node
});
