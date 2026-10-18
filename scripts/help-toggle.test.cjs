// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/help-toggle.test.cjs
// Help (form-fields): pressing the act on a heading line or in a drawer's
// header shows every setting's help by marking the root, and keeps the
// choice for this browser in the cookie the shell reads back (helpCookie);
// pressing it again hides help and forgets the choice.
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
    body: {querySelectorAll: () => []},
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
  const act = {closest: selector => (selector === '[data-verso-help-toggle]' ? act : null)};
  const elsewhere = {closest: () => null};
  const pressOn = target => clicks.forEach(fn => fn({target, preventDefault() {}}));
  return {press: () => pressOn(act), pressElsewhere: () => pressOn(elsewhere), attrs, cookies};
}

test('pressing the act shows help and keeps it for this browser', () => {
  const p = page('http:');
  p.press();
  assert.equal(p.attrs['data-verso-help'], 'shown');
  assert.deepEqual(p.cookies, ['verso_help=shown; Path=/; Max-Age=31536000; SameSite=Strict']);
});

test('pressing it again hides help and forgets the choice', () => {
  const p = page('http:');
  p.press();
  p.press();
  assert.equal('data-verso-help' in p.attrs, false);
  assert.equal(p.cookies[1], 'verso_help=; Path=/; Max-Age=0; SameSite=Strict');
});

// A Secure cookie an HTTPS visit left behind is one an HTTP page can neither
// read nor overwrite, so the choice would stick to whatever HTTPS last chose.
test('over HTTPS the cookie is the same one HTTP writes', () => {
  const p = page('https:');
  p.press();
  assert.equal(p.cookies[0], 'verso_help=shown; Path=/; Max-Age=31536000; SameSite=Strict');
});

test('a press anywhere else leaves the page as it is', () => {
  const p = page('http:');
  p.pressElsewhere();
  assert.equal('data-verso-help' in p.attrs, false);
  assert.deepEqual(p.cookies, []);
});
