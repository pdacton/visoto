// Tests for static/js/api-url.js. No dependencies: run with
//   node --test tests/js/
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const Api = require('../../static/js/api-url.js');

function env(over) {
  return Object.assign({
    activeEndpointSlug: () => 'qlever-local',
    activeSiteLang: () => 'de',
    activeTemplateSet: () => 'instances/cube%3ACube.html',
  }, over || {});
}

// The htmx hook, driven the way htmx drives it.
function fire(api, detail) {
  let listener;
  api.install({ addEventListener: (name, fn) => { assert.equal(name, 'htmx:configRequest'); listener = fn; } });
  listener({ detail });
  return detail.parameters;
}

test('hook: every /api path gets endpoint, lang and src — there is no route list', () => {
  const api = Api.create(env());
  for (const path of [
    '/api/metric/AnimaliaCount',
    '/api/async-table/instances',
    '/api/bnode-tree?_=1',
    '/api/shape-summary?_=1',
    // The route the three old per-file allowlists all missed.
    '/api/cube-table/observations?iri=https%3A%2F%2Fexample.org%2Fcube',
    '/api/some-route-added-next-year/x',
  ]) {
    assert.deepEqual(fire(api, { path, verb: 'get', parameters: {} }),
      { endpoint: 'qlever-local', lang: 'de', src: 'instances/cube%3ACube.html' }, path);
  }
});

test('hook: leaves non-/api paths and non-GET requests alone', () => {
  const api = Api.create(env());
  assert.deepEqual(fire(api, { path: '/resource?iri=x', verb: 'get', parameters: {} }), {});
  assert.deepEqual(fire(api, { path: '/apix/metric/a', verb: 'get', parameters: {} }), {});
  assert.deepEqual(fire(api, { path: '/api/chat', verb: 'post', parameters: {} }), {});
});

test('hook: a value set on the element or in the path wins', () => {
  const api = Api.create(env());
  assert.deepEqual(fire(api, { path: '/api/metric/a', verb: 'get', parameters: { endpoint: 'lindas-int' } }),
    { endpoint: 'lindas-int', lang: 'de', src: 'instances/cube%3ACube.html' });
  assert.deepEqual(fire(api, { path: '/api/metric/a?lang=fr&endpoint=x', verb: 'get', parameters: {} }),
    { src: 'instances/cube%3ACube.html' });
});

test('the empty language is a real code and is sent; an unresolved one is not', () => {
  const none = Api.create(env({ activeSiteLang: () => '' }));
  assert.equal(none.url('/api/metric/a', { src: null, endpoint: null }), '/api/metric/a?lang=');
  assert.deepEqual(fire(none, { path: '/api/metric/a', verb: 'get', parameters: {} }).lang, '');

  const unknown = Api.create(env({ activeSiteLang: () => null }));
  assert.equal(unknown.url('/api/metric/a', { src: null, endpoint: null }), '/api/metric/a');
});

test('missing readers contribute nothing and nothing throws', () => {
  const api = Api.create({});
  assert.deepEqual(api.defaults(), {});
  assert.equal(api.url('/api/search', { q: 'a b' }), '/api/search?q=a%20b');
});

test('url: params first, then the defaults they leave unset', () => {
  const api = Api.create(env());
  assert.equal(api.url('/api/facet-values/t/kind', { iri: 'http://ex.org/a#b' }),
    '/api/facet-values/t/kind?iri=http%3A%2F%2Fex.org%2Fa%23b&endpoint=qlever-local&lang=de&src=instances%2Fcube%253ACube.html');
});

test('url: undefined takes the default, null suppresses it, an explicit value wins', () => {
  const api = Api.create(env());
  assert.equal(api.url('/api/search', { q: 'x', src: null }), '/api/search?q=x&endpoint=qlever-local&lang=de');
  assert.equal(api.url('/api/x', { endpoint: undefined, lang: 'fr', src: null }), '/api/x?endpoint=qlever-local&lang=fr');
});

test('url: arrays repeat, and an existing query string is extended, not duplicated', () => {
  const api = Api.create(env({ activeTemplateSet: () => '' }));
  assert.equal(api.url('/api/lazy-tree/t/parents', { node: ['a', 'b'], endpoint: null, lang: null }),
    '/api/lazy-tree/t/parents?node=a&node=b');
  assert.equal(api.url('/api/faceted-table/t?f.kind=a&f.kind=b'),
    '/api/faceted-table/t?f.kind=a&f.kind=b&endpoint=qlever-local&lang=de');
  assert.equal(api.url('/api/faceted-table/t?'), '/api/faceted-table/t?endpoint=qlever-local&lang=de');
  assert.equal(api.url('/api/faceted-table/t?lang=it'), '/api/faceted-table/t?lang=it&endpoint=qlever-local');
});
