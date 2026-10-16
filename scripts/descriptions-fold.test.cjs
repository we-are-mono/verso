// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/descriptions-fold.test.cjs
// The descriptions' fold (form-fields): pressing the act on a heading line or
// in a drawer's header folds every description on the page by marking the
// root, and keeps the state for this browser in the cookie the shell reads
// back (descriptionsCookie); pressing it again clears both.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// page loads verso-page.js into a document that has nothing in it but the
// root and the act, and returns a press of the act and what the page wrote.
function page(protocol) {
  const attrs = {};
  const root = {
    getAttribute: name => (name in attrs ? attrs[name] : null),
    setAttribute: (name, value) => { attrs[name] = String(value); },
    removeAttribute: name => { delete attrs[name]; },
  };
  const clicks = [];
  const cookies = [];
  const nothing = () => null;
  const document = {
    documentElement: root,
    addEventListener: (type, fn) => { if (type === 'click') clicks.push(fn); },
    querySelector: nothing,
    querySelectorAll: () => [],
    getElementById: nothing,
    set cookie(value) { cookies.push(value); },
    get cookie() { return ''; },
  };
  const window = {location: {protocol}, addEventListener: () => {}, matchMedia: () => ({matches: false})};
  const context = vm.createContext({window, document, getComputedStyle: () => ({}), requestAnimationFrame: () => 0});
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-page.js'), 'utf8'), context);
  const act = {closest: selector => (selector === '[data-verso-fold]' ? act : null)};
  const elsewhere = {closest: () => null};
  const pressOn = target => clicks.forEach(fn => fn({target, preventDefault() {}}));
  return {press: () => pressOn(act), pressElsewhere: () => pressOn(elsewhere), attrs, cookies};
}

test('pressing the act folds the page and keeps it for this browser', () => {
  const p = page('http:');
  p.press();
  assert.equal(p.attrs['data-verso-descriptions'], 'folded');
  assert.deepEqual(p.cookies, ['verso_descriptions=folded; Path=/; Max-Age=31536000; SameSite=Strict']);
});

test('pressing it again brings the descriptions back and forgets the fold', () => {
  const p = page('http:');
  p.press();
  p.press();
  assert.equal('data-verso-descriptions' in p.attrs, false);
  assert.equal(p.cookies[1], 'verso_descriptions=; Path=/; Max-Age=0; SameSite=Strict');
});

test('over HTTPS the cookie is kept for HTTPS alone', () => {
  const p = page('https:');
  p.press();
  assert.equal(p.cookies[0], 'verso_descriptions=folded; Path=/; Max-Age=31536000; SameSite=Strict; Secure');
});

test('a press anywhere else leaves the page as it is', () => {
  const p = page('http:');
  p.pressElsewhere();
  assert.equal('data-verso-descriptions' in p.attrs, false);
  assert.deepEqual(p.cookies, []);
});
