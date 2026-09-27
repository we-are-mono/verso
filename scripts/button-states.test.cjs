// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node scripts/button-states.test.cjs
// Exercise the shipped event handlers with a small DOM boundary; no requests
// leave the process and no router configuration is changed.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

class Events {
  listeners = new Map();
  addEventListener(type, listener) {
    const listeners = this.listeners.get(type) || [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }
  emit(type, properties = {}) {
    const event = {defaultPrevented: false, preventDefault() { this.defaultPrevented = true; }, stopImmediatePropagation() { this.stopped = true; }, ...properties};
    for (const listener of this.listeners.get(type) || []) {
      listener(event);
      if (event.stopped) break;
    }
    return event;
  }
}
class Element extends Events {
  constructor(text = '') {
    super();
    this.childNodes = [{textContent: text}];
    this.attributes = new Map();
    this.classes = new Set();
    this.classList = {add: (...names) => names.forEach(n => this.classes.add(n)), remove: (...names) => names.forEach(n => this.classes.delete(n))};
    this.style = {minWidth: ''};
    this.dataset = {};
    this.disabled = false;
    this.name = '';
    this.value = '';
    this.isConnected = true;
  }
  get textContent() { return this.childNodes.map(n => n.textContent).join(''); }
  set textContent(value) { this.childNodes = [{textContent: value}]; }
  get children() { return this.childNodes; }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.get(name) ?? null; }
  hasAttribute(name) { return this.attributes.has(name); }
  removeAttribute(name) { this.attributes.delete(name); }
  replaceChildren(...nodes) { this.childNodes = nodes; }
  appendChild(node) { this.childNodes.push(node); node.parentElement = this; }
  remove() { this.parentElement.childNodes = this.parentElement.childNodes.filter(n => n !== this); }
  getBoundingClientRect() { return {width: 120}; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
  closest() { return null; }
  matches() { return false; }
}
function environment() {
  const document = new Element();
  const window = new Events();
  const ids = new Map(), selectors = new Map();
  document.documentElement = new Element();
  document.getElementById = id => ids.get(id) || null;
  document.querySelector = selector => selectors.get(selector) || null;
  document.createElement = () => new Element();
  document.createTextNode = textContent => ({textContent});
  selectors.set('[data-verso-button-waiting]', {content: {cloneNode: () => ({textContent: '', spinner: true})}});
  window.location = new URL('http://verso.test/plugins/dnsdhcp/');
  window.sessionStorage = {getItem() {}, removeItem() {}};
  const timers = [];
  const context = vm.createContext({document, window, URL, URLSearchParams, T: s => s,
    setTimeout: f => timers.push(f), clearTimeout() {}, setInterval() {}, clearInterval() {},
    versoAnnounce() {}, fetch: () => { throw Error('Unexpected request'); }});
  function load(name) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets', name), 'utf8'), context, {filename: name});
  }
  load('verso-buttons.js');
  return {context, document, window, ids, selectors, timers, load, buttons: context.versoButtons};
}
function formWith(buttons) {
  const form = new Element();
  form.querySelectorAll = () => buttons;
  form.querySelector = () => buttons[0];
  return form;
}
function busy(button, label) {
  assert.equal(button.disabled, true);
  assert.equal(button.getAttribute('aria-disabled'), 'true');
  assert.equal(button.getAttribute('aria-busy'), 'true');
  assert.equal(button.textContent, label);
  assert.equal(button.childNodes.filter(n => n.spinner).length, 1);
}

test('waiting twice cannot overwrite the original label, icon, or attributes', () => {
  const {buttons} = environment();
  const button = new Element('Install');
  const icon = {textContent: '', icon: true};
  button.childNodes.unshift(icon);
  button.setAttribute('aria-disabled', 'false');
  button.setAttribute('aria-busy', 'false');
  assert.equal(buttons.start(button, 'Installing…'), true);
  assert.equal(buttons.start(button, 'Installing…'), false);
  busy(button, 'Installing…');
  buttons.finish(button);
  assert.equal(button.disabled, false);
  assert.equal(button.textContent, 'Install');
  assert.equal(button.childNodes[0], icon);
  assert.equal(button.getAttribute('aria-disabled'), 'false');
  assert.equal(button.getAttribute('aria-busy'), 'false');
  assert.equal(button.style.minWidth, '');
});

test('locking a companion does not make an unavailable action available afterward', () => {
  const {buttons} = environment();
  const companion = new Element('Unavailable');
  companion.disabled = true;
  buttons.start(companion);
  buttons.finish(companion);
  assert.equal(companion.disabled, true);
  assert.equal(companion.getAttribute('aria-busy'), null);
});

test('native submits retain their action, reject Enter repeats, and recover on history return', () => {
  const {buttons, window} = environment();
  const button = new Element('Update now');
  button.name = 'action'; button.value = 'install';
  const form = formWith([button]);
  function submit() {
    const event = {target: form, submitter: button, defaultPrevented: false, preventDefault() { this.defaultPrevented = true; }};
    buttons.submit(event, 'Installing…');
    return event;
  }
  assert.equal(submit().defaultPrevented, false);
  busy(button, 'Installing…');
  assert.equal(form.children.find(n => n.name === 'action').value, 'install');
  assert.equal(submit().defaultPrevented, true);
  window.emit('pageshow');
  assert.equal(button.disabled, false);
  assert.equal(button.textContent, 'Update now');
  assert.equal(form.children.some(n => n.name === 'action'), false);
  assert.equal(submit().defaultPrevented, false);
});

test('a canceled native submission leaves the controls usable', () => {
  const {buttons} = environment();
  const button = new Element('Update now');
  buttons.submit({target: formWith([button]), submitter: button, defaultPrevented: true}, 'Installing…');
  assert.equal(button.disabled, false);
  assert.equal(button.textContent, 'Update now');
});

for (const [verb, label] of Object.entries({search: 'Searching…', install: 'Installing…', remove: 'Removing…', upgrade: 'Upgrading…'})) {
  test(`package ${verb} has a spinner outside Packages and recovers after request failure`, () => {
    const env = environment();
    env.load('verso-packages.js');
    const button = new Element(verb), companion = new Element('Other action');
    const form = formWith([button]);
    form.action = 'http://verso.test/system/packages';
    form.querySelector = selector => selector === '[name="_primary"]' ? {value: verb} : button;
    form.closest = selector => selector === 'form' ? form : {querySelectorAll: () => [button, companion]};
    const xhr = {getResponseHeader: () => null};
    const detail = {elt: form, xhr};
    env.document.emit('htmx:beforeRequest', {detail});
    busy(button, label);
    assert.equal(companion.disabled, true);
    const repeatedEnter = env.document.emit('submit', {target: form, submitter: button});
    assert.equal(repeatedEnter.defaultPrevented, true);
    assert.equal(repeatedEnter.stopped, true, 'stop before htmx can queue a second request');
    assert.equal(env.document.emit('htmx:beforeRequest', {detail: {...detail, xhr: {}}}).defaultPrevented, true);
    env.document.emit('htmx:afterRequest', {detail: {xhr, failed: true}});
    assert.equal(button.disabled, false);
    assert.equal(companion.disabled, false);
    assert.equal(button.textContent, verb);
    assert.equal(button.getAttribute('aria-busy'), null);
    assert.equal(env.document.emit('htmx:beforeRequest', {detail}).defaultPrevented, false);
    busy(button, label);
  });
}

test('maintenance checking locks immediately and restores on browser history', () => {
  const env = environment();
  env.load('verso-system.js');
  const button = new Element('Check again');
  button.setAttribute('data-busy-label', 'Checking…');
  const form = formWith([button]);
  form.matches = () => true;
  env.document.emit('submit', {target: form, submitter: button});
  busy(button, 'Checking…');
  assert.equal(env.document.emit('submit', {target: form, submitter: button}).defaultPrevented, true);
  env.window.emit('pageshow');
  assert.equal(button.disabled, false);
  assert.equal(button.textContent, 'Check again');
});

for (const verb of ['apply', 'discard']) {
  test(`${verb} locks both staged actions, marks the requested action, and recovers on refusal`, async () => {
    const env = environment();
    const chip = new Element(); chip.setAttribute('data-count', '1');
    env.ids.set('verso-staged', chip);
    const apply = new Element('Apply 1'), discard = new Element('Discard all');
    for (const [name, button, label] of [['apply', apply, 'Applying…'], ['discard', discard, 'Discarding…']]) {
      button.id = 'verso-staged-' + name;
      button.closest = () => button;
      button.setAttribute('data-busy-label', label);
      env.ids.set(button.id, button);
    }
    let resolve, requests = 0;
    env.context.fetch = () => { requests++; return new Promise(done => { resolve = done; }); };
    env.load('verso-commit.js');
    const active = verb === 'apply' ? apply : discard;
    env.document.emit('click', {target: active});
    busy(active, verb === 'apply' ? 'Applying…' : 'Discarding…');
    assert.equal(apply.disabled && discard.disabled, true);
    env.document.emit('click', {target: active});
    assert.equal(requests, 1);
    resolve({ok: false});
    await new Promise(setImmediate);
    assert.equal(apply.disabled || discard.disabled, false);
    assert.equal(apply.textContent, 'Apply 1');
    assert.equal(discard.textContent, 'Discard all');
  });
}

for (const reducedMotion of [false, true]) {
  test(`login rejects duplicate submits and restores after history (reduced motion: ${reducedMotion})`, () => {
    const env = environment();
    const button = new Element('Sign in to router'), form = new Element(), panel = new Element();
    const mark = new Element();
    let animations = 0;
    mark.replaceChildren(...Array.from({length: 4}, () => ({animate() { animations++; return {cancel() {}}; }})));
    button.animate = () => {};
    form.querySelector = selector => selector === '[data-login-submit]' ? button : mark;
    const clock = {dataset: {unix: '0', offset: '0'}};
    panel.querySelector = selector => selector === '[data-login-clock]' ? clock : new Element();
    env.selectors.set('[data-login-form]', form);
    env.selectors.set('[data-login-status]', panel);
    env.window.matchMedia = () => ({matches: reducedMotion, addEventListener() {}});
    env.window.setInterval = () => {};
    env.context.performance = {now: () => 0};
    env.context.getComputedStyle = () => ({getPropertyValue: () => '#999'});
    env.load('verso-login.js');
    assert.equal(form.emit('submit').defaultPrevented, false);
    assert.equal(button.disabled, true);
    assert.equal(button.getAttribute('aria-busy'), 'true');
    assert.equal(form.getAttribute('aria-busy'), 'true');
    assert.equal(form.emit('submit').defaultPrevented, true);
    assert.equal(mark.children.length, 4);
    assert.equal(animations, reducedMotion ? 0 : 4);
    env.window.emit('pageshow');
    assert.equal(button.disabled, false);
    assert.equal(button.getAttribute('aria-busy'), null);
    assert.equal(form.getAttribute('aria-busy'), null);
  });
}

test('package refresh stays locked through polling and unlocks after a network failure', async () => {
  const env = environment();
  const root = new Element(), button = new Element('Refresh index'), field = new Element(), error = new Element();
  field.value = ''; field.dispatchEvent = () => {};
  root.dataset.packageAll = 'false';
  root.querySelector = selector => ({
    '[data-package-query]': field,
    '[data-package-refresh] button': button,
    '[data-package-note]': new Element(),
    '[data-package-error]': error,
  })[selector] || null;
  env.selectors.set('[data-verso-packages]', root);
  env.context.Event = class {};
  env.context.FormData = class { *[Symbol.iterator]() { yield ['_csrf', 'test']; } };
  env.context.versoErrorLine = (line, text) => { line.textContent = text; };
  const form = formWith([button]);
  form.matches = () => true;
  form.action = 'http://verso.test/system/packages/discover';
  let requests = 0;
  env.context.fetch = async () => {
    requests++;
    if (requests === 1) return {ok: true};
    throw Error('offline');
  };
  env.load('verso-packages.js');
  env.document.emit('submit', {target: form, submitter: button});
  env.document.emit('submit', {target: form, submitter: button});
  await new Promise(setImmediate);
  assert.equal(requests, 1);
  busy(button, 'Refreshing index…');
  await env.timers.at(-1)();
  assert.equal(button.disabled, false);
  assert.equal(button.getAttribute('aria-busy'), null);
  assert.equal(button.textContent, 'Refresh index');
  assert.equal(error.hidden, false);
});
