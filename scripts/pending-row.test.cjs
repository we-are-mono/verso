// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/pending-row.test.cjs
// The rail's Pending changes row (verso-commit.js) over a small DOM: what it
// says when the stage takes a change, and what it shows of an edit not yet
// saved. No request leaves the process.
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
  appendChild(child) { this.children.push(child); return child; }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.has(name) ? this.attributes.get(name) : null; }
  hasAttribute(name) { return this.attributes.has(name); }
  removeAttribute(name) { this.attributes.delete(name); }
  get offsetWidth() { return 0; }
  addEventListener(type, listener) { this.listeners.set(type, [...(this.listeners.get(type) || []), listener]); }
  emit(type, properties = {}) { for (const l of this.listeners.get(type) || []) l({type, ...properties}); }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  closest() { return null; }
  matches() { return false; }
}

// railPage is a page whose rail holds the row with count waiting changes,
// latest the words of the newest; stored is the session's storage, holding
// what a save left on its way out.
function railPage(count, {latest = '', stored = new Map()} = {}) {
  const document = new Node();
  const ids = new Map();
  const row = new Node();
  row.setAttribute('data-count', String(count));
  if (latest) row.setAttribute('data-latest', latest);
  row.hidden = count === 0;
  for (const [id, node] of [['verso-pending-row', row], ['verso-pending-count', new Node(String(count))], ['verso-pending-line', new Node()],
    ['verso-menu-count', new Node(String(count))], ['verso-menu-ghost', new Node()]]) ids.set(id, node);
  const menu = new Node();
  menu.setAttribute('data-count', String(count));
  document.getElementById = id => ids.get(id) || null;
  document.querySelector = selector => (selector === '.verso-menu-button' ? menu : null);
  document.createTextNode = text => new Node(text);
  document.documentElement = new Node();
  const timers = [];
  const announced = [];
  const window = {
    sessionStorage: {getItem: key => stored.get(key) ?? null, removeItem: key => stored.delete(key), setItem: (key, value) => stored.set(key, value)},
    location: {pathname: '/plugins/firewall/'},
    scrollBy() {},
  };
  const context = vm.createContext({
    document, window, CSS: {escape: s => s}, T: s => s,
    setTimeout: f => timers.push(f), clearTimeout() {}, setInterval() {}, clearInterval() {},
    versoTurn: (el, text) => { el.textContent = text; },
    versoEl: (tag, cls, text) => { const n = new Node(text); n.className = cls; return n; },
    versoAnnounce: text => announced.push(text),
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-commit.js'), 'utf8'), context, {filename: 'verso-commit.js'});
  return {document, ids, row, menu, timers, announced, stored, staged: window.versoStaged};
}

// pageForm is a page form whose save stages, holding one setting per edit
// as {label, control}.
function pageForm(...settings) {
  const form = new Node();
  const fields = settings.map(({label, control}) => {
    const field = new Node();
    field.getAttribute = name => (name === 'data-verso-change-label' ? label : null);
    field.querySelectorAll = () => [control];
    control.closest = selector => (selector === 'form[data-verso-stages]' ? form : null);
    return field;
  });
  form.querySelectorAll = selector => (selector === '[data-verso-change-field]' ? fields : []);
  return form;
}

// rendered is a page as a fetch hands it back, its row saying count and latest.
function rendered(count, latest) {
  const row = new Node();
  row.setAttribute('data-count', String(count));
  if (latest) row.setAttribute('data-latest', latest);
  return {getElementById: id => (id === 'verso-pending-row' ? row : null)};
}

test('a change the stage takes is told in the row, its count turning over, then folds away', () => {
  const page = railPage(2);
  page.staged.sync(rendered(3, 'Allow-Ping turned off'));
  assert.equal(page.ids.get('verso-pending-count').textContent, '3');
  assert.equal(page.ids.get('verso-menu-count').textContent, '3');
  assert.equal(page.row.getAttribute('data-state'), 'told');
  assert.equal(page.ids.get('verso-pending-line').textContent, 'Allow-Ping turned off');
  assert.deepEqual(page.announced, ['Pending: Allow-Ping turned off']);
  page.timers.shift()();
  assert.equal(page.row.getAttribute('data-state'), null, 'the line folds away after a moment');
  assert.equal(page.row.hidden, false);
});

test('a render with nothing new leaves the row as it is', () => {
  const page = railPage(1);
  page.row.setAttribute('data-latest', 'Lease time → 6h');
  page.staged.sync(rendered(1, 'Lease time → 6h'));
  assert.equal(page.row.getAttribute('data-state'), null);
});

test('an edit not yet saved shows dashed, and on a clean stage the whole row stands as a ghost', () => {
  const page = railPage(0);
  page.staged.preview('Allow-DHCPv6 · Action → DROP', '+1');
  assert.equal(page.row.hidden, false);
  assert.equal(page.row.getAttribute('data-state'), 'ghost');
  assert.equal(page.row.hasAttribute('data-clean'), true);
  assert.equal(page.ids.get('verso-pending-line').textContent, '+1Allow-DHCPv6 · Action → DROP');
  assert.equal(page.menu.hasAttribute('data-ghost'), true);
  assert.equal(page.ids.get('verso-menu-ghost').textContent, '+1');
  page.staged.clear();
  assert.equal(page.row.hidden, true, 'letting the edit go leaves a clean stage with no row');
  assert.equal(page.menu.hasAttribute('data-ghost'), false);
});

test('an edit to a page form shows what its save would stage, and putting it back clears it', () => {
  const page = railPage(4);
  const control = {type: 'text', name: 'leasetime', value: '6h', defaultValue: '12h'};
  const field = new Node();
  field.getAttribute = name => (name === 'data-verso-change-label' ? 'Lease time' : null);
  field.querySelectorAll = () => [control];
  const form = new Node();
  form.querySelectorAll = selector => (selector === '[data-verso-change-field]' ? [field] : []);
  control.closest = selector => (selector === 'form[data-verso-stages]' ? form : null);
  page.document.emit('input', {target: control});
  assert.equal(page.row.getAttribute('data-state'), 'ghost');
  assert.equal(page.ids.get('verso-pending-line').textContent, '+1Lease time → 6h');
  assert.equal(page.row.hasAttribute('data-clean'), false, 'with changes waiting the row keeps its wash');
  control.value = '12h';
  page.document.emit('input', {target: control});
  assert.equal(page.row.getAttribute('data-state'), null);
});

test('an edit to a setting already pending says no "+1"', () => {
  const page = railPage(4);
  const control = {type: 'text', name: 'leasetime', value: '1h', defaultValue: '6h'};
  const field = new Node();
  field.getAttribute = name => (name === 'data-verso-change-label' ? 'Lease time' : null);
  field.querySelectorAll = () => [control];
  field.querySelector = selector => (selector === '[data-verso-staged-row]' ? new Node() : null);
  const form = new Node();
  form.querySelectorAll = selector => (selector === '[data-verso-change-field]' ? [field] : []);
  control.closest = selector => (selector === 'form[data-verso-stages]' ? form : null);
  page.document.emit('change', {target: control});
  assert.equal(page.ids.get('verso-pending-line').textContent, 'Lease time → 1h');
});

test('two edits say the first and how many more, each new setting counted', () => {
  const page = railPage(1);
  const lease = {type: 'text', name: 'leasetime', value: '6h', defaultValue: '12h'};
  const cache = {type: 'text', name: 'cachesize', value: '151', defaultValue: '150'};
  pageForm({label: 'Lease time', control: lease}, {label: 'Cache size', control: cache});
  page.document.emit('input', {target: cache});
  assert.equal(page.ids.get('verso-pending-line').textContent, '+2Lease time → 6h +1 more');
});

test('Cancel or closing the drawer lets the line go; hover and focus change nothing', () => {
  const page = railPage(1);
  const control = {type: 'text', name: 'leasetime', value: '6h', defaultValue: '12h'};
  pageForm({label: 'Lease time', control});
  page.document.emit('input', {target: control});
  for (const kind of ['mouseover', 'focusin', 'mouseout', 'focusout']) page.document.emit(kind, {target: control});
  assert.equal(page.row.getAttribute('data-state'), 'ghost', 'pointing at or into the form leaves the line as it is');
  page.document.emit('reset', {target: control});
  assert.equal(page.row.getAttribute('data-state'), null, 'Cancel resets the form and lets the line go');
  page.document.emit('input', {target: control});
  page.document.emit('verso-panel-hidden');
  assert.equal(page.row.getAttribute('data-state'), null, 'a drawer closing lets the line go');
});

test('a save that lands turns the count over from the one it had and says what it staged', () => {
  const page = railPage(5);
  page.row.setAttribute('data-latest', 'Cache size → 151');
  page.staged.landed(4);
  assert.equal(page.ids.get('verso-pending-count').textContent, '5');
  assert.equal(page.row.getAttribute('data-state'), 'told');
  assert.equal(page.ids.get('verso-pending-line').textContent, 'Cache size → 151');
});

test('a staging save notes the count on its way out, and the page it lands on tells it', () => {
  const page = railPage(2);
  const save = {getBoundingClientRect: () => ({top: 400})};
  const form = new Node();
  form.matches = selector => selector === 'main form[data-verso-stages]';
  page.document.emit('submit', {target: form, submitter: save});
  const note = JSON.parse(page.stored.get('verso-staged-landing'));
  assert.equal(note.count, 2);
  assert.equal(note.path, '/plugins/firewall/');

  const landed = railPage(3, {latest: 'Allow-Ping turned off', stored: page.stored});
  assert.equal(landed.row.getAttribute('data-state'), 'told');
  assert.equal(landed.ids.get('verso-pending-line').textContent, 'Allow-Ping turned off');
  assert.equal(landed.ids.get('verso-pending-count').textContent, '3');
  assert.equal(landed.stored.has('verso-staged-landing'), false, 'a note is read once');
});

test('a page reached with no save behind it tells nothing', () => {
  const page = railPage(3, {latest: 'Allow-Ping turned off'});
  assert.equal(page.row.getAttribute('data-state'), null);
  assert.equal(page.timers.length, 0);
  assert.equal(page.document.documentElement.hasAttribute('data-verso-landing'), false);
});
