// Tests for static/js/graph-turtle.js (run: node --test tests/js/).
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const T = require('../../static/js/graph-turtle.js');

test('types, labels, literal properties and edges', () => {
  const ttl = T.toTurtle({
    elements: [{
      iri: 'http://example.org/a',
      types: ['http://schema.org/Person'],
      labels: [{ value: 'Anna "A"', language: 'de' }],
      properties: {
        'http://example.org/age': { type: 'string', values: [{ value: '42', language: '', datatype: { value: 'http://www.w3.org/2001/XMLSchema#integer' } }] },
        'http://example.org/note': { type: 'string', values: [{ value: 'line1\nline2', language: '' }] },
      },
    }],
    links: [{ source: 'http://example.org/a', type: 'http://schema.org/knows', target: 'http://example.org/b c' }],
  });
  assert.match(ttl, /@prefix schema: <http:\/\/schema\.org\/> \./);
  assert.match(ttl, /@prefix xsd: /);
  assert.match(ttl, /<http:\/\/example\.org\/a>\n    a schema:Person ;/);
  assert.match(ttl, /rdfs:label "Anna \\"A\\""@de/);
  assert.match(ttl, /"42"\^\^xsd:integer/);
  assert.match(ttl, /"line1\\nline2"/);
  assert.match(ttl, /schema:knows <http:\/\/example\.org\/b\\u0020c>/);
});

test('unused prefixes are left out; duplicate objects written once', () => {
  const ttl = T.toTurtle({ elements: [], links: [
    { source: 'urn:x:1', type: 'urn:p', target: 'urn:x:2' },
    { source: 'urn:x:1', type: 'urn:p', target: 'urn:x:2' },
  ] });
  assert.doesNotMatch(ttl, /@prefix/);
  assert.equal((ttl.match(/<urn:x:2>/g) || []).length, 1);
});
