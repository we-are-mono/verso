// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node scripts/dev-grid.test.cjs
// A dev session's G draws the 8px grid over the page (layout-grid spec). The
// shipped verso-dev.js runs against a small DOM boundary; nothing is fetched.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-dev.js'), 'utf8');

function session({stored = null, storage = 'works'} = {}) {
  const attributes = new Set();
  const listeners = [];
  const saved = new Map(stored === null ? [] : [['verso-dev-grid', stored]]);
  const sessionStorage = {
    getItem(key) { if (storage === 'throws') throw Error('blocked'); return saved.has(key) ? saved.get(key) : null; },
    setItem(key, value) { if (storage === 'throws') throw Error('blocked'); saved.set(key, value); },
  };
  const root = {
    hasAttribute: name => attributes.has(name),
    toggleAttribute(name, on) { if (on) attributes.add(name); else attributes.delete(name); return on; },
  };
  const context = vm.createContext({
    document: {
      documentElement: root,
      querySelector: () => null, // no linked stylesheet: the hot reload stands down
      addEventListener: (type, f) => { if (type === 'keydown') listeners.push(f); },
    },
    window: {},
    sessionStorage,
    setInterval() {},
  });
  context.window.sessionStorage = sessionStorage;
  vm.runInContext(source, context);
  const field = {closest: selector => (selector.includes('input') ? field : null)};
  const page = {closest: () => null};
  const press = (key, {target = page, ...mods} = {}) => {
    const event = {key, target, ctrlKey: false, metaKey: false, altKey: false, ...mods};
    listeners.forEach(f => f(event));
  };
  return {on: () => attributes.has('data-verso-grid'), press, saved, field};
}

test('G draws the grid, and G again takes it away', () => {
  const s = session();
  assert.equal(s.on(), false, 'no grid until asked');
  s.press('g');
  assert.equal(s.on(), true);
  s.press('G');
  assert.equal(s.on(), false);
});

test('a G typed into a field stays in the field', () => {
  const s = session();
  s.press('g', {target: s.field});
  assert.equal(s.on(), false);
});

test('G with a modifier is the browser\'s, not the grid\'s', () => {
  const s = session();
  for (const mod of ['ctrlKey', 'metaKey', 'altKey']) s.press('g', {[mod]: true});
  assert.equal(s.on(), false);
});

test('the choice survives a reload in the same tab', () => {
  const first = session();
  first.press('g');
  assert.equal(first.saved.get('verso-dev-grid'), 'on');
  const reloaded = session({stored: 'on'});
  assert.equal(reloaded.on(), true, 'the grid comes back after a hot reload');
});

test('blocked storage still lets G work', () => {
  const s = session({storage: 'throws'});
  s.press('g');
  assert.equal(s.on(), true);
});
