// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/choice-fields.test.cjs
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-forms.js'), 'utf8');

class Control {
  attributes = new Map([['aria-invalid', 'true'], ['aria-describedby', 'policy-tip policy-error']]);
  checked = false;
  focused = false;
  group = null;
  radios = [];
  getAttribute(name) { return this.attributes.get(name); }
  removeAttribute(name) { this.attributes.delete(name); }
  matches(selector) {
    return selector === '[aria-invalid="true"]' ? this.attributes.has('aria-invalid')
      : selector === '[role="radiogroup"]' && this.radios.length > 0;
  }
  closest(selector) { return selector === '[role="radiogroup"]' ? this.group : null; }
  querySelector(selector) { return selector === 'input:checked' ? this.radios.find(r => r.checked) : this.radios[0]; }
  querySelectorAll() { return this.radios.filter(r => r.attributes.has('aria-invalid')); }
  scrollIntoView() { this.scrolled = true; }
  focus() { this.focused = true; }
}

function environment(refused) {
  const listeners = new Map();
  const error = {hidden: false, hasAttribute: name => name === 'data-verso-error'};
  const help = {hidden: false, hasAttribute: () => false};
  const document = {
    body: {},
    querySelector: selector => selector === 'main [aria-invalid="true"]' ? refused : null,
    querySelectorAll: () => [],
    getElementById: id => ({'policy-error': error, 'policy-tip': help})[id],
    addEventListener(type, fn) {
      listeners.set(type, [...(listeners.get(type) || []), fn]);
    },
  };
  const window = {addEventListener() {}, requestAnimationFrame: fn => fn()};
  vm.runInNewContext(source, {
    document, window, MutationObserver: class { observe() {} },
  });
  return {
    error, help,
    emit(type, target) { for (const fn of listeners.get(type) || []) fn({target}); },
  };
}

test('a refused radio group focuses its selected option when the page opens', () => {
  const group = new Control();
  group.radios = [new Control(), new Control(), new Control()];
  group.radios[1].checked = true;
  const env = environment(group);
  env.emit('alpine:initialized');
  assert.equal(group.scrolled, true);
  assert.equal(group.focused, false);
  assert.deepEqual(group.radios.map(r => r.focused), [false, true, false]);
});

test('changing a radio clears the entire group refusal and keeps its help', () => {
  const group = new Control();
  group.radios = [new Control(), new Control(), new Control()];
  group.radios.forEach(radio => { radio.group = group; });
  const env = environment(group);
  env.emit('input', group.radios[2]);
  for (const control of [group, ...group.radios]) {
    assert.equal(control.attributes.has('aria-invalid'), false);
  }
  assert.equal(env.error.hidden, true);
  assert.equal(env.help.hidden, false);
});

test('ordinary refused controls still receive focus and clear their own error', () => {
  const control = new Control();
  const env = environment(control);
  env.emit('alpine:initialized');
  assert.equal(control.focused, true);
  env.emit('input', control);
  assert.equal(control.attributes.has('aria-invalid'), false);
  assert.equal(env.error.hidden, true);
  assert.equal(env.help.hidden, false);
});
