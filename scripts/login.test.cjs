// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node scripts/login.test.cjs
// Exercise the sign-in page's script against a small DOM boundary: the act's
// busy face is the shared one (verso-buttons.js), and the public status
// refresh writes the card's live facts in place. No request leaves the
// process.
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
    const event = {defaultPrevented: false, preventDefault() { this.defaultPrevented = true; }, ...properties};
    for (const listener of this.listeners.get(type) || []) listener(event);
    return event;
  }
}
class Element extends Events {
  constructor(text = '') {
    super();
    this.childNodes = [{textContent: text}];
    this.attributes = new Map();
    this.dataset = {};
    this.disabled = false;
    this.name = '';
  }
  get textContent() { return this.childNodes.map(n => n.textContent).join(''); }
  set textContent(value) { this.childNodes = [{textContent: value}]; }
  get children() { return this.childNodes; }
  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.get(name) ?? null; }
  hasAttribute(name) { return this.attributes.has(name); }
  removeAttribute(name) { this.attributes.delete(name); }
  replaceChildren(...nodes) { this.childNodes = nodes; }
  appendChild(node) { this.childNodes.push(node); }
  querySelector() { return null; }
  querySelectorAll() { return []; }
}

// page draws the sign-in page as the server does: the card's frame with the
// router's clock and the refresh's words, the form and its act, and the live
// facts by their keys.
function page({status} = {}) {
  const document = new Events();
  const window = new Events();
  document.hidden = false;
  document.documentElement = {lang: 'en'};
  const root = new Element();
  Object.assign(root.dataset, {unix: '1792350000', offset: '7200', zone: 'CEST',
    onlineLabel: 'Online', offlineLabel: 'Offline', unknownLabel: 'Unknown', busyLabel: 'Signing in…'});
  const button = new Element('Sign in');
  button.type = 'submit';
  const form = new Element();
  form.querySelectorAll = () => [button];
  form.querySelector = () => button;
  const facts = {
    '[data-verso-prop="clock"]': new Element('19:00:00 CEST'),
    '[data-verso-prop="uptime"]': new Element('56 d 5 h'),
    '[data-verso-prop="internet"]': new Element('Online'),
    '[data-verso-prop-dot="internet"]': new Element(),
  };
  facts['[data-verso-prop-dot="internet"]'].setAttribute('data-verso-tone', 'success');
  root.querySelector = selector => selector === 'form' ? form : facts[selector] || null;
  document.querySelector = selector => ({
    '[data-login]': root,
    '[data-verso-button-waiting]': {content: {cloneNode: () => ({textContent: '', spinner: true})}},
  })[selector] || null;
  document.createElement = () => new Element();
  document.createTextNode = textContent => ({textContent});
  const intervals = [];
  const requests = [];
  const context = vm.createContext({document, window, URL, Number, String, Date, Promise,
    performance: {now: () => 0},
    AbortController: class { signal = {}; abort() {} },
    setTimeout: () => 0, clearTimeout() {},
    setInterval: f => intervals.push(f),
    fetch: (url, options) => {
      requests.push({url, options});
      if (status instanceof Error) return Promise.reject(status);
      return Promise.resolve({ok: true, json: () => Promise.resolve(status)});
    }});
  context.window.setInterval = context.setInterval;
  context.window.setTimeout = context.setTimeout;
  context.window.clearTimeout = context.clearTimeout;
  for (const name of ['verso-buttons.js', 'verso-login.js']) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets', name), 'utf8'), context, {filename: name});
  }
  const fact = key => facts[`[data-verso-prop="${key}"]`];
  return {document, window, form, button, intervals, requests, fact, mark: facts['[data-verso-prop-dot="internet"]']};
}
const settle = () => new Promise(resolve => setImmediate(resolve));

test('signing in wears the shared busy face, refuses a second submit, and recovers on history return', () => {
  const p = page();
  const first = p.form.emit('submit', {target: p.form, submitter: p.button});
  assert.equal(first.defaultPrevented, false);
  assert.equal(p.button.disabled, true);
  assert.equal(p.button.getAttribute('aria-busy'), 'true');
  assert.equal(p.button.textContent, 'Signing in…');
  assert.equal(p.button.childNodes.filter(n => n.spinner).length, 1);
  // The mark's motion is the stylesheet's, gated on the reader's motion
  // preference; no script animates it.
  const css = fs.readFileSync(path.join(__dirname, '../internal/server/assets/blocks/waiting.css'), 'utf8');
  assert.match(css, /@media \(prefers-reduced-motion: no-preference\) \{\s*\.verso-waiting__square \{\s*animation-duration:/);
  assert.equal(p.form.emit('submit', {target: p.form, submitter: p.button}).defaultPrevented, true);
  p.window.emit('pageshow');
  assert.equal(p.button.disabled, false);
  assert.equal(p.button.textContent, 'Sign in');
});

for (const [internet, words, tone] of [[true, 'Online', 'success'], [false, 'Offline', 'danger'], [null, 'Unknown', 'unknown']]) {
  test(`a refresh says the internet is ${words.toLowerCase()} in the Internet row`, async () => {
    const p = page({status: {internet, uptime: '56 d 6 h', clock: '19:00:05', zone: 'CEST', unix: 1792350005, offset: 7200}});
    p.document.emit('visibilitychange');
    await settle();
    assert.equal(p.requests.length, 1);
    assert.equal(p.requests[0].url, '/login/status');
    assert.equal(p.fact('internet').textContent, words);
    assert.equal(p.mark.getAttribute('data-verso-tone'), tone);
    assert.equal(p.fact('uptime').textContent, '56 d 6 h');
  });
}

test('a refresh that fails says the state is unknown', async () => {
  const p = page({status: new Error('unreachable')});
  p.document.emit('visibilitychange');
  await settle();
  assert.equal(p.requests.length, 1);
  assert.equal(p.fact('internet').textContent, 'Unknown');
  assert.equal(p.mark.getAttribute('data-verso-tone'), 'unknown');
});

test('the clock keeps the router\'s time and zone, not the browser\'s', () => {
  const p = page();
  p.intervals[0]();
  // 1792350000 is 19:00:00 UTC; the router is two hours ahead.
  assert.equal(p.fact('clock').textContent, '21:00:00 CEST');
});

test('no refresh runs while signing in', async () => {
  const p = page({status: {internet: false, uptime: '1 min', clock: '19:00:05', zone: 'CEST', unix: 1792350005, offset: 7200}});
  p.form.emit('submit', {target: p.form, submitter: p.button});
  p.document.emit('visibilitychange');
  await settle();
  assert.equal(p.requests.length, 0);
  assert.equal(p.fact('internet').textContent, 'Online');
});
