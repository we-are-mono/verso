// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/outcome-stream.test.cjs
// The events said to a session (ADR-028): a browser holds one stream for all
// its tabs, says each event once on a tab someone is looking at, and stacks
// each under the act's outcome as a notification of its own, at most three.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const SOURCE = fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-page.js'), 'utf8');

// El is as much of an element as the outcome layer touches.
class El {
  constructor(attrs = {}, children = [], text = '') {
    this.attrs = {...attrs};
    this.children = [];
    this.parentNode = null;
    this.listeners = {};
    this.hidden = 'hidden' in attrs;
    this.text = text;
    children.forEach(c => this.appendChild(c));
  }
  getAttribute(n) { return n in this.attrs ? this.attrs[n] : null; }
  setAttribute(n, v) { this.attrs[n] = String(v); }
  removeAttribute(n) { delete this.attrs[n]; }
  hasAttribute(n) { return n in this.attrs; }
  appendChild(c) { return this.insertBefore(c, null); }
  insertBefore(c, ref) {
    if (c.parentNode) c.remove();
    const at = ref ? this.children.indexOf(ref) : -1;
    if (at < 0) this.children.push(c); else this.children.splice(at, 0, c);
    c.parentNode = this;
    return c;
  }
  remove() {
    if (!this.parentNode) return;
    const siblings = this.parentNode.children;
    siblings.splice(siblings.indexOf(this), 1);
    this.parentNode = null;
  }
  replaceChildren(...nodes) {
    this.children.slice().forEach(c => c.remove());
    nodes.forEach(n => this.appendChild(n));
  }
  get nextSibling() {
    if (!this.parentNode) return null;
    const s = this.parentNode.children;
    return s[s.indexOf(this) + 1] || null;
  }
  get textContent() { return this.text + this.children.map(c => c.textContent).join(''); }
  matches() { return false; }
  contains(node) { return node === this || this.children.some(c => c.contains(node)); }
  all() { return this.children.flatMap(c => [c, ...c.all()]); }
  querySelectorAll(selector) {
    const attr = /^(?:template)?\[([a-z-]+)\]$/.exec(selector);
    return attr ? this.all().filter(e => e.hasAttribute(attr[1])) : [];
  }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
  cloneNode() {
    const copy = new El(this.attrs, this.children.map(c => c.cloneNode()), this.text);
    copy.hidden = this.hidden;
    return copy;
  }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  fire(type) { (this.listeners[type] || []).forEach(fn => fn({type})); }
}

// flash is the shell's notification fragment, as the stream sends it.
function flash(text, tone = 'success') {
  return `<div class="verso-notice" data-verso-flash-variant="${tone}"><span>${text}</span></div>`;
}

// parsed is the flash an html fragment holds.
function parsed(html) {
  const tone = /data-verso-flash-variant="([a-z]+)"/.exec(html);
  return new El(tone ? {'data-verso-flash-variant': tone[1]} : {}, [], html.replace(/<[^>]+>/g, ''));
}

// browser is what a browser's tabs share: their locks, channels, storage and
// the streams they open, and a clock the test moves.
function browser({locks = true, channels = true} = {}) {
  const b = {held: new Map(), queued: [], channels: [], storage: new Map(), sources: [], timers: [], now: 0};
  // release frees what tab holds, granting each lock to the first tab still
  // waiting for it, as the browser does.
  b.release = tab => {
    for (const [name, holder] of b.held) if (holder === tab) b.held.delete(name);
    b.queued = b.queued.filter(q => !q.tab.closed);
    const next = b.queued.find(q => !b.held.has(q.name));
    if (next) {
      b.queued.splice(b.queued.indexOf(next), 1);
      next.grant();
    }
  };
  // tick moves the clock on by ms, running every timer that falls due on the
  // way, those set by a timer included.
  b.tick = ms => {
    const end = b.now + ms;
    for (;;) {
      const due = b.timers.filter(t => !t.cancelled && t.at <= end).sort((x, y) => x.at - y.at)[0];
      if (!due) break;
      b.now = due.at;
      due.cancelled = true;
      due.fn();
    }
    b.now = end;
  };
  b.locks = locks;
  b.broadcast = channels;
  return b;
}

// tab is one page of the session in browser b.
function tab(b, {visible = true} = {}) {
  const t = {closed: false, visible, doc: {}};
  const slot = new El({'data-verso-outcome-slot': ''});
  const cross = new El({'data-verso-outcome-close': ''});
  const layer = new El({id: 'verso-outcome', hidden: ''}, [slot, cross]);
  const boxTemplate = new El({'data-verso-event-box': ''});
  boxTemplate.content = {firstElementChild: new El({'data-verso-event': ''}, [new El({'data-verso-outcome-slot': ''}), new El({'data-verso-outcome-close': ''})])};
  const column = new El({'data-verso-outcomes': '', 'data-verso-events': '/events/stream'}, [layer, boxTemplate]);
  const body = new El({}, [column]);
  const listeners = {};
  const document = {
    documentElement: new El(),
    body,
    activeElement: null,
    get visibilityState() { return t.visible ? 'visible' : 'hidden'; },
    getElementById: id => (id === 'verso-outcome' ? layer : null),
    querySelector: s => body.querySelector(s),
    querySelectorAll: s => body.querySelectorAll(s),
    addEventListener: (type, fn) => { (listeners[type] ||= []).push(fn); },
    createElement: () => {
      const template = {content: {firstElementChild: null}};
      Object.defineProperty(template, 'innerHTML', {set: html => { template.content.firstElementChild = parsed(html); }});
      return template;
    },
    importNode: node => node.cloneNode(true),
    set cookie(_) {},
    get cookie() { return ''; },
  };
  const navigator = {};
  if (b.locks) {
    navigator.locks = {
      request(name, options, callback) {
        if (typeof options === 'function') { callback = options; options = {}; }
        const grant = () => {
          b.held.set(name, t);
          Promise.resolve(callback({name})).then(() => { if (b.held.get(name) === t) b.release(t); }, () => {});
        };
        if (!b.held.has(name)) grant();
        else if (options.ifAvailable) callback(null);
        else b.queued.push({tab: t, name, grant});
        return Promise.resolve();
      },
    };
  }
  class BroadcastChannel {
    constructor(name) { this.name = name; this.tab = t; b.channels.push(this); }
    postMessage(data) { b.channels.filter(c => c !== this && c.name === this.name && !c.tab.closed).forEach(c => c.onmessage && c.onmessage({data})); }
  }
  class EventSource {
    constructor(url) { this.url = url; this.tab = t; this.listeners = {}; b.sources.push(this); }
    addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
    emit(id, html) { (this.listeners.said || []).forEach(fn => fn({data: JSON.stringify({html}), lastEventId: id})); }
  }
  const window = {location: {protocol: 'https:'}, addEventListener: () => {}, matchMedia: () => ({matches: false})};
  const context = vm.createContext({
    window, document, navigator, EventSource,
    BroadcastChannel: b.broadcast ? BroadcastChannel : undefined,
    localStorage: {getItem: k => (b.storage.has(k) ? b.storage.get(k) : null), setItem: (k, v) => b.storage.set(k, v)},
    setTimeout: (fn, ms) => { const timer = {fn, at: b.now + (ms || 0)}; b.timers.push(timer); return timer; },
    clearTimeout: timer => { if (timer) timer.cancelled = true; },
    requestAnimationFrame: fn => { fn(); return 0; },
    getComputedStyle: () => ({getPropertyValue: () => ''}),
    versoAnnounce: () => {},
    Date: {now: () => b.now},
  });
  vm.runInContext(SOURCE, context);
  t.window = window;
  t.events = () => column.children.filter(c => c.hasAttribute('data-verso-event'));
  t.words = () => t.events().map(e => e.textContent);
  t.act = () => (layer.hidden ? null : slot.textContent);
  t.show = text => window.versoOutcome.show(parsed(flash(text)));
  t.become = visible => { t.visible = visible; (listeners.visibilitychange || []).forEach(fn => fn()); };
  t.close = () => { t.closed = true; b.release(t); };
  return t;
}

test('of three tabs only the lock holder opens the stream, and another takes it when it closes', () => {
  const b = browser();
  const first = tab(b), second = tab(b), third = tab(b);
  assert.equal(b.sources.length, 1);
  assert.equal(b.sources[0].tab, first);
  assert.equal(b.sources[0].url, '/events/stream');
  first.close();
  assert.equal(b.sources.length, 2);
  assert.ok([second, third].includes(b.sources[1].tab));
});

test('an event is said by one visible tab and remembered as said', () => {
  const b = browser();
  const shown = tab(b), alsoShown = tab(b);
  b.sources[0].emit('7', flash('The certificate for router.example.com was renewed.'));
  const sayers = [shown, alsoShown].filter(t => t.events().length === 1);
  assert.equal(sayers.length, 1);
  assert.equal(sayers[0].words()[0], 'The certificate for router.example.com was renewed.');
  assert.deepEqual(JSON.parse(b.storage.get('verso-events-shown')), ['7']);
});

test('a hidden tab says an event when it comes into view within its minute, and not after', () => {
  const b = browser();
  const hidden = tab(b, {visible: false});
  b.sources[0].emit('1', flash('One.'));
  assert.equal(hidden.events().length, 0);
  b.tick(30000);
  hidden.become(true);
  assert.deepEqual(hidden.words(), ['One.']);

  const late = browser();
  const away = tab(late, {visible: false});
  late.sources[0].emit('2', flash('Two.'));
  late.tick(61000);
  away.become(true);
  assert.equal(away.events().length, 0);
});

test('an event said once is not said again after the stream reconnects', () => {
  const b = browser();
  const t = tab(b);
  b.sources[0].emit('3', flash('Three.'));
  b.sources[0].emit('3', flash('Three.'));
  assert.equal(t.events().length, 1);
});

test('an act takes its place at once, replacing the act before it and no event', () => {
  const b = browser();
  const t = tab(b);
  b.sources[0].emit('1', flash('An event.', 'danger'));
  t.show('Saved.');
  assert.equal(t.act(), 'Saved.');
  assert.deepEqual(t.words(), ['An event.']);
  t.show('Saved again.');
  assert.equal(t.act(), 'Saved again.');
  assert.deepEqual(t.words(), ['An event.']);
});

test('events stand newest first under the act, each leaving on its own, a danger one only when closed', () => {
  const b = browser();
  const t = tab(b);
  b.sources[0].emit('1', flash('Failed.', 'danger'));
  b.tick(1000);
  b.sources[0].emit('2', flash('Renewed.'));
  assert.deepEqual(t.words(), ['Renewed.', 'Failed.']);
  b.tick(6300);
  assert.deepEqual(t.words(), ['Failed.'], 'the success left on its own; the danger stays');
  const cross = t.events()[0].querySelector('[data-verso-outcome-close]');
  cross.fire('click');
  b.tick(300);
  assert.deepEqual(t.words(), []);
});

test('a fourth event pushes out the oldest', () => {
  const b = browser();
  const t = tab(b);
  ['1', '2', '3', '4'].forEach(id => b.sources[0].emit(id, flash('Event ' + id + '.', 'warning')));
  b.tick(300);
  assert.deepEqual(t.words(), ['Event 4.', 'Event 3.', 'Event 2.']);
});

test('without Web Locks a tab opens a stream of its own', () => {
  const b = browser({locks: false});
  tab(b);
  tab(b);
  assert.equal(b.sources.length, 2);
});
