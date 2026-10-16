// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/pending-row.test.cjs
// The rail's Pending changes row (verso-commit.js) over a small DOM: it says
// the stage's count as each page it is handed carries it, and a page form's
// save lands where it was made. No request leaves the process.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

class Node {
  constructor(text = '') {
    this.attributes = new Map();
    this.children = [];
    this.text = text;
    this.hidden = false;
    this.listeners = new Map();
  }
  get textContent() { return this.text + this.children.map(c => c.textContent).join(''); }
  set textContent(value) { this.text = String(value); this.children = []; }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.has(name) ? this.attributes.get(name) : null; }
  hasAttribute(name) { return this.attributes.has(name); }
  removeAttribute(name) { this.attributes.delete(name); }
  addEventListener(type, listener) { this.listeners.set(type, [...(this.listeners.get(type) || []), listener]); }
  emit(type, properties = {}) { for (const l of this.listeners.get(type) || []) l({type, ...properties}); }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  closest() { return null; }
  matches() { return false; }
}

// railPage is a page whose rail holds the row with count waiting changes,
// latest the words of the newest; stored is the session's storage, holding
// what a save left on its way out; found answers the page's queries.
function railPage(count, {latest = '', stored = new Map(), found = {}} = {}) {
  const document = new Node();
  const ids = new Map();
  const row = new Node();
  row.setAttribute('data-count', String(count));
  if (latest) row.setAttribute('data-latest', latest);
  row.hidden = count === 0;
  const menuCount = new Node(String(count));
  menuCount.hidden = count === 0;
  for (const [id, node] of [['verso-pending-row', row], ['verso-pending-count', new Node(String(count))], ['verso-menu-count', menuCount]]) ids.set(id, node);
  const menu = new Node();
  menu.setAttribute('data-count', String(count));
  document.getElementById = id => ids.get(id) || null;
  document.querySelector = selector => (selector === '.verso-menu-button' ? menu : found[selector] || null);
  document.documentElement = new Node();
  const timers = [];
  const announced = [];
  const scrolled = [];
  const window = {
    sessionStorage: {getItem: key => stored.get(key) ?? null, removeItem: key => stored.delete(key), setItem: (key, value) => stored.set(key, value)},
    location: {pathname: '/plugins/dnsdhcp/'},
    scrollBy: (x, y) => scrolled.push([x, y]),
  };
  const context = vm.createContext({
    document, window, CSS: {escape: s => s}, T: s => s,
    setTimeout: f => timers.push(f), clearTimeout() {}, setInterval() {}, clearInterval() {},
    versoAnnounce: text => announced.push(text),
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-commit.js'), 'utf8'), context, {filename: 'verso-commit.js'});
  return {document, ids, row, menu, timers, announced, scrolled, stored, staged: window.versoStaged};
}

// rendered is a page as a fetch hands it back, its row saying count and latest.
function rendered(count, latest) {
  const row = new Node();
  row.setAttribute('data-count', String(count));
  if (latest) row.setAttribute('data-latest', latest);
  return {getElementById: id => (id === 'verso-pending-row' ? row : null)};
}

// setting is one row of a page form, named for its option, holding control.
function setting(form, name, control) {
  const row = new Node();
  row.getAttribute = attr => (attr === 'data-verso-change-name' ? name : null);
  row.querySelectorAll = () => [control];
  row.closest = selector => (selector === 'form' ? form : null);
  control.closest = selector => (selector === 'form' ? form : null);
  return row;
}

test('a change the stage takes shows as the new count, with nothing animated', () => {
  const page = railPage(2);
  page.staged.sync(rendered(3, 'Allow-Ping turned off'));
  assert.equal(page.ids.get('verso-pending-count').textContent, '3');
  assert.equal(page.ids.get('verso-menu-count').textContent, '3');
  assert.equal(page.menu.getAttribute('data-count'), '3');
  assert.equal(page.row.hidden, false);
  assert.equal(page.row.getAttribute('data-state'), null, 'the row draws no second line');
  assert.equal(page.timers.length, 0, 'nothing waits to fold or settle');
  assert.deepEqual(page.announced, ['Pending: Allow-Ping turned off'], 'a screen reader hears what the stage took');
});

test('the first change brings the row and the last one gone takes it away', () => {
  const page = railPage(0);
  page.staged.sync(rendered(1, 'Log every lease → on'));
  assert.equal(page.row.hidden, false);
  assert.equal(page.ids.get('verso-menu-count').hidden, false);
  page.staged.sync(rendered(0));
  assert.equal(page.row.hidden, true);
  assert.equal(page.ids.get('verso-menu-count').hidden, true);
  assert.equal(page.menu.getAttribute('data-count'), '0');
});

test('a render with nothing new says nothing', () => {
  const page = railPage(1, {latest: 'Lease time → 6h'});
  page.staged.sync(rendered(1, 'Lease time → 6h'));
  assert.deepEqual(page.announced, []);
});

test('an edit before Save leaves the rail as it is', () => {
  for (const count of [0, 2]) {
    const page = railPage(count);
    const form = new Node();
    const control = {type: 'checkbox', name: 'logdhcp', checked: true, defaultChecked: false};
    form.querySelectorAll = () => [setting(form, 'logdhcp', control)];
    for (const kind of ['input', 'change']) page.document.emit(kind, {target: control});
    assert.equal(page.row.hidden, count === 0, 'Save is what brings the row');
    assert.equal(page.row.getAttribute('data-state'), null);
    assert.equal(page.ids.get('verso-pending-count').textContent, String(count));
    assert.equal(page.menu.hasAttribute('data-ghost'), false);
  }
});

test("a page form's save lands with its Save where it stood", () => {
  const page = railPage(0);
  const form = new Node();
  form.matches = selector => selector === 'main form';
  const control = {type: 'text', name: 'dhcpleasemax', value: '200', defaultValue: ''};
  form.querySelectorAll = () => [setting(form, 'dhcpleasemax', control)];
  page.document.emit('submit', {target: form, submitter: {getBoundingClientRect: () => ({top: 400})}});
  const note = JSON.parse(page.stored.get('verso-staged-landing'));
  assert.equal(note.name, 'dhcpleasemax');
  assert.equal(note.at, 400);

  const landedForm = new Node();
  landedForm.querySelector = () => ({getBoundingClientRect: () => ({top: 250})});
  const landedRow = setting(landedForm, 'dhcpleasemax', {});
  const landed = railPage(1, {stored: page.stored, found: {'main [data-verso-change-field][data-verso-change-name="dhcpleasemax"]': landedRow}});
  assert.deepEqual(landed.scrolled, [[0, -150]]);
  assert.equal(landed.stored.has('verso-staged-landing'), false, 'a note is read once');
  assert.equal(landed.row.getAttribute('data-state'), null, 'the row is as the page drew it');
});

test('a save that changed nothing leaves no note, and a page with none scrolls nowhere', () => {
  const page = railPage(0);
  const form = new Node();
  form.matches = selector => selector === 'main form';
  form.querySelectorAll = () => [setting(form, 'dhcpleasemax', {type: 'text', value: '', defaultValue: ''})];
  page.document.emit('submit', {target: form});
  assert.equal(page.stored.has('verso-staged-landing'), false);
  assert.deepEqual(railPage(1).scrolled, []);
});
