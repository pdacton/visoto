// Tests for static/js/graph-turtle.js (run: node --test tests/js/).
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const T = require('../../static/js/graph-turtle.js');
const Prefixes = require('../../static/js/prefixes.js');
const BASE = [
  { name: 'rdf', iri: 'http://www.w3.org/1999/02/22-rdf-syntax-ns#' },
  { name: 'rdfs', iri: 'http://www.w3.org/2000/01/rdf-schema#' },
  { name: 'xsd', iri: 'http://www.w3.org/2001/XMLSchema#' },
];
const HTTP = Prefixes.create(BASE.concat({ name: 'schema', iri: 'http://schema.org/' }));
const HTTPS = Prefixes.create(BASE.concat({ name: 'schema', iri: 'https://schema.org/' }));

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
  }, HTTP);
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
  ] }, HTTP);
  assert.doesNotMatch(ttl, /@prefix/);
  assert.equal((ttl.match(/<urn:x:2>/g) || []).length, 1);
});

test('schema: is written with the endpoint namespace, in both directions', () => {
  const graph = (ns) => ({ elements: [{ iri: 'urn:x:1', types: [ns + 'Person'] }], links: [] });
  const https = T.toTurtle(graph('https://schema.org/'), HTTPS);
  assert.match(https, /@prefix schema: <https:\/\/schema\.org\/> \./);
  assert.match(https, /a schema:Person/);
  // An http IRI on an https endpoint is not abbreviated with the https prefix.
  const mixed = T.toTurtle(graph('http://schema.org/'), HTTPS);
  assert.doesNotMatch(mixed, /@prefix schema:/);
  assert.match(mixed, /a <http:\/\/schema\.org\/Person>/);
  const http = T.toTurtle(graph('http://schema.org/'), HTTP);
  assert.match(http, /@prefix schema: <http:\/\/schema\.org\/> \./);
});

test('two names for one namespace: the first is used, declared once', () => {
  const p = Prefixes.create([{ name: 'schema', iri: 'https://schema.org/' }, { name: 'schema2', iri: 'https://schema.org/' }]);
  const ttl = T.toTurtle({ elements: [], links: [{ source: 'urn:x:1', type: 'https://schema.org/knows', target: 'urn:x:2' }] }, p);
  assert.match(ttl, /schema:knows/);
  assert.doesNotMatch(ttl, /schema2/);
});

test('without prefixes every IRI is written in full', () => {
  const ttl = T.toTurtle({ elements: [], links: [{ source: 'urn:x:1', type: 'http://schema.org/knows', target: 'urn:x:2' }] });
  assert.doesNotMatch(ttl, /@prefix/);
  assert.match(ttl, /<http:\/\/schema\.org\/knows>/);
});
