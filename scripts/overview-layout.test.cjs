// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/overview-layout.test.cjs
// The Overview's card layout (ADR-022): the pure moves the page makes and the
// cookie value it writes, which the shell reads back (widget.ParseOverviewLayout).
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function model() {
  const context = vm.createContext({window: {}, document: {querySelector: () => null}});
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-overview.js'), 'utf8'), context);
  return context.window.versoOverviewLayout;
}

const L = model();
const base = () => ({
  cols: [['internet', 'firewall', 'ports', 'tunnels'], ['traffic', 'resources', 'hardware', 'firmware']],
  closed: [], hidden: [],
});
const plain = value => JSON.parse(JSON.stringify(value));

test('encode writes what the shell reads', () => {
  assert.equal(L.encode(base()), '1.internet-firewall-ports-tunnels.traffic-resources-hardware-firmware..');
  const l = base();
  l.closed = ['firmware', 'internet'];
  l.hidden = ['resources'];
  l.cols[1] = ['traffic', 'hardware', 'firmware'];
  assert.equal(L.encode(l), '1.internet-firewall-ports-tunnels.traffic-hardware-firmware.firmware-internet.resources');
});

test('move places a card anywhere in either column', () => {
  const across = L.move(base(), 'hardware', 0, 1);
  assert.deepEqual(plain(across.cols), [['internet', 'hardware', 'firewall', 'ports', 'tunnels'], ['traffic', 'resources', 'firmware']]);
  const within = L.move(base(), 'internet', 0, 3);
  assert.deepEqual(plain(within.cols[0]), ['firewall', 'ports', 'tunnels', 'internet']);
  const end = L.move(base(), 'firewall', 1, 99);
  assert.deepEqual(plain(end.cols[1]), ['traffic', 'resources', 'hardware', 'firmware', 'firewall']);
});

test('nudge moves one place, and across to the same place in the other column', () => {
  assert.deepEqual(plain(L.nudge(base(), 'internet', 'ArrowDown').cols[0]), ['firewall', 'internet', 'ports', 'tunnels']);
  assert.deepEqual(plain(L.nudge(base(), 'firewall', 'ArrowUp').cols[0]), ['firewall', 'internet', 'ports', 'tunnels']);
  const right = L.nudge(base(), 'ports', 'ArrowRight');
  assert.deepEqual(plain(right.cols), [['internet', 'firewall', 'tunnels'], ['traffic', 'resources', 'ports', 'hardware', 'firmware']]);
  const left = L.nudge(base(), 'firmware', 'ArrowLeft');
  assert.deepEqual(plain(left.cols[0]), ['internet', 'firewall', 'ports', 'firmware', 'tunnels']);
});

test('nudge stops at a column edge', () => {
  assert.equal(L.nudge(base(), 'internet', 'ArrowUp'), null);
  assert.equal(L.nudge(base(), 'tunnels', 'ArrowDown'), null);
  assert.equal(L.nudge(base(), 'internet', 'ArrowLeft'), null);
  assert.equal(L.nudge(base(), 'traffic', 'ArrowRight'), null);
  assert.equal(L.nudge(base(), 'internet', 'Enter'), null);
});

test('nudge into a shorter column lands at its end', () => {
  const l = base();
  l.cols[1] = ['traffic'];
  assert.deepEqual(plain(L.nudge(l, 'tunnels', 'ArrowRight').cols[1]), ['traffic', 'tunnels']);
});

test('hide takes a card off its column, and forgets it was closed', () => {
  const l = base();
  l.closed = ['resources'];
  const hidden = L.hide(l, 'resources');
  assert.deepEqual(plain(hidden.cols[1]), ['traffic', 'hardware', 'firmware']);
  assert.deepEqual(plain(hidden.hidden), ['resources']);
  assert.deepEqual(plain(hidden.closed), []);
});

test('restore brings a hidden card back open, at the end of the shorter column', () => {
  const l = L.hide(base(), 'firmware');
  const back = L.restore(l, 'firmware', [400, 900]);
  assert.deepEqual(plain(back.cols[0]), ['internet', 'firewall', 'ports', 'tunnels', 'firmware']);
  assert.deepEqual(plain(back.hidden), []);
  const right = L.restore(L.hide(base(), 'firmware'), 'firmware', [900, 400]);
  assert.deepEqual(plain(right.cols[1]), ['traffic', 'resources', 'hardware', 'firmware']);
});

test('toggle opens and closes a card', () => {
  const closed = L.toggle(base(), 'ports');
  assert.deepEqual(plain(closed.closed), ['ports']);
  assert.deepEqual(plain(L.toggle(closed, 'ports').closed), []);
});

test('the moves leave the layout they were given alone', () => {
  const l = base();
  L.move(l, 'hardware', 0, 0);
  L.hide(l, 'internet');
  L.toggle(l, 'ports');
  assert.deepEqual(plain(l), plain(base()));
});

// at is an element a click starts on, inside the ancestors it names: closest
// answers whether any selector in a list matches it or an ancestor.
const at = (...matches) => ({closest: list => list.split(',').some(sel => matches.includes(sel.trim())) || null});

test('only the header itself and its toggle open or close the card', () => {
  assert.equal(L.togglesFrom(at('a')), false, 'the door');
  assert.equal(L.togglesFrom(at('button')), false, 'another button');
  assert.equal(L.togglesFrom(at('button', '[data-overview-toggle]')), true, 'the toggle');
  assert.equal(L.togglesFrom(at()), true, 'the header itself');
});

// Never Secure: a Secure cookie an HTTPS visit leaves behind is one an HTTP
// page of the same router can neither read nor overwrite.
test('the cookie is first-party, long-lived and the same over HTTP and HTTPS', () => {
  assert.equal(L.cookie('1.a.b..'), 'verso_cards=1.a.b..; Path=/; Max-Age=34560000; SameSite=Lax');
  assert.deepEqual([...L.forget()], ['verso_cards=; Path=/; Max-Age=0; SameSite=Lax', 'verso_overview=; Path=/; Max-Age=0; SameSite=Lax']);
});
