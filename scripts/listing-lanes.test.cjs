// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/listing-lanes.test.cjs
// A page's laned listing is a card per lane, a table each (ADR-023): what the
// listing's scripts do must reach every lane's table, not the first alone.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

// A small element tree answering the few selectors the helpers ask.
class El {
  constructor(tag, attrs = {}, children = []) {
    this.tagName = tag.toUpperCase();
    this.attrs = attrs;
    this.hidden = false;
    this.childNodes = [];
    children.forEach(c => this.appendChild(c));
  }
  appendChild(c) { c.parentElement = this; this.childNodes.push(c); }
  replaceChildren(...nodes) { this.childNodes = []; nodes.forEach(n => this.appendChild(n)); }
  hasAttribute(name) { return name in this.attrs; }
  getAttribute(name) { return name in this.attrs ? this.attrs[name] : null; }
  get dataset() { return {versoReorderId: this.attrs['data-verso-reorder-id']}; }
  matches(sel) {
    if (sel === 'tbody > tr:not([hidden])') return this.tagName === 'TR' && !this.hidden && this.parentElement && this.parentElement.tagName === 'TBODY';
    const m = /^([a-z]*)(?:\[([a-z-]+)\])?$/.exec(sel);
    return !!m && (!m[1] || this.tagName === m[1].toUpperCase()) && (!m[2] || this.hasAttribute(m[2]));
  }
  closest(sel) { for (let n = this; n; n = n.parentElement) if (n.matches(sel)) return n; return null; }
  querySelectorAll(sel) {
    const out = [];
    const walk = n => n.childNodes.forEach(c => { if (c.matches(sel)) out.push(c); walk(c); });
    walk(this);
    return out;
  }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
}

const row = id => new El('tr', {'data-verso-reorder-row': '', 'data-verso-reorder-id': id});
const lane = (...rows) => new El('div', {'data-verso-lane': ''}, [new El('table', {'data-verso-reorder-table': ''}, [new El('tbody', {}, rows)])]);
const listing = (...lanes) => new El('div', {'data-verso-listing': '', 'data-verso-lanes': ''}, lanes);

function helpers() {
  const document = {addEventListener() {}, querySelector: () => null, querySelectorAll: () => []};
  const context = vm.createContext({window: {}, document});
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-tables.js'), 'utf8'), context);
  return context;
}
const h = helpers();

test('a dropped row announces the order of every lane, not its own alone', () => {
  const wan = lane(row('a'), row('b'));
  listing(wan, lane(row('c')));
  assert.deepEqual([...h.versoListingOrder(wan.querySelector('table'))], ['a', 'b', 'c']);
});

test('a table on its own announces its own order', () => {
  const table = new El('table', {}, [new El('tbody', {}, [row('x'), row('y')])]);
  assert.deepEqual([...h.versoListingOrder(table)], ['x', 'y']);
});

test('a lane the cut leaves nothing in goes, and comes back with its rows', () => {
  const kept = row('a');
  const gone = row('b');
  const list = listing(lane(kept), lane(gone));
  gone.hidden = true;
  h.versoHideEmptyLanes(list);
  assert.deepEqual(list.querySelectorAll('[data-verso-lane]').map(l => l.hidden), [false, true]);
  gone.hidden = false;
  h.versoHideEmptyLanes(list);
  assert.deepEqual(list.querySelectorAll('[data-verso-lane]').map(l => l.hidden), [false, false]);
});

test('a live laned listing refreshes every lane, its headers with them', () => {
  const live = listing(lane(row('a')));
  const fresh = listing(lane(row('a')), lane(row('b')));
  const [current, source] = h.versoRefreshParts(live, fresh);
  assert.equal(current, live);
  assert.equal(source, fresh);
  current.replaceChildren(...source.childNodes);
  assert.equal(live.querySelectorAll('[data-verso-lane]').length, 2);
});

test('a live table refreshes its rows alone', () => {
  const body = new El('tbody', {}, [row('a')]);
  const freshBody = new El('tbody', {}, [row('b')]);
  const [current, source] = h.versoRefreshParts(new El('div', {}, [new El('table', {}, [body])]), new El('div', {}, [new El('table', {}, [freshBody])]));
  assert.equal(current, body);
  assert.equal(source, freshBody);
});
