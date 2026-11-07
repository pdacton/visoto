// Tests for static/js/feedback.js. No dependencies: run with
//   node --test tests/js/
'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const F = require('../../static/js/feedback.js');

const FORM = 'https://github.com/pdacton/visoto/issues/new?template=feedback.yml';
const PLAIN = 'https://github.com/pdacton/visoto/issues/new';

test('issue form: form fields plus a body fallback, template kept', () => {
  const u = new URL(F.buildIssueURL(FORM, {
    kind: 'Bug', title: '  Table empty ', description: 'It broke', context: '- Page: x',
  }));
  assert.equal(u.searchParams.get('template'), 'feedback.yml');
  assert.equal(u.searchParams.get('title'), '[Bug] Table empty');
  assert.equal(u.searchParams.get('description'), 'It broke');
  assert.equal(u.searchParams.get('context'), '- Page: x');
  // Used by GitHub while the form is not on the default branch.
  assert.equal(u.searchParams.get('body'), 'It broke\n\n### Context\n\n- Page: x');
});

test('URL stays under the GitHub limit, context intact', () => {
  const ctx = '- Page: https://visoto.example/resource?iri=' + 'x'.repeat(300);
  const href = F.buildIssueURL(FORM, { title: 't', description: 'ä ü '.repeat(750), context: ctx });
  assert.ok(href.length <= 7500, 'length ' + href.length);
  const u = new URL(href);
  assert.equal(u.searchParams.get('context'), ctx);
  assert.ok(u.searchParams.get('description').length > 0);
});

test('shrinking terminates on a huge context', () => {
  const href = F.buildIssueURL(FORM, { title: 't', description: 'abc', context: 'y'.repeat(9000) });
  assert.equal(new URL(href).searchParams.get('description'), '');
});

test('no template: everything goes into body', () => {
  const u = new URL(F.buildIssueURL(PLAIN, { kind: 'Idea', title: 't', description: 'd', context: 'c' }));
  assert.equal(u.searchParams.get('body'), 'd\n\n### Context\n\nc');
  assert.equal(u.searchParams.has('description'), false);
});

test('page URL with its own query survives encoding', () => {
  const page = 'https://visoto.example/resource?iri=https%3A%2F%2Fld.admin.ch%2Fx%23y&endpoint=lindas-prod#Graph';
  const ctx = F.contextText({ page, endpoint: 'lindas-prod', language: 'de', browser: 'UA' });
  const u = new URL(F.buildIssueURL(FORM, { kind: 'Bug', title: 't', description: '', context: ctx }));
  assert.ok(u.searchParams.get('context').includes(page));
});

test('description is capped', () => {
  const u = new URL(F.buildIssueURL(FORM, { title: 't', description: 'a'.repeat(10000), context: '' }));
  assert.equal(u.searchParams.get('description').length, F.MAX_DESCRIPTION);
});

test('context labels the no-language choice and a missing endpoint', () => {
  const s = F.contextText({ page: 'p', endpoint: '', language: '', browser: 'b' });
  assert.match(s, /Endpoint: \(default\)/);
  assert.match(s, /Language: \(none\)/);
});
