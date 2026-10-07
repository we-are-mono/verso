// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node scripts/takeover.test.cjs
// The restarting takeover's move (ADR-017 §2): a shell that moves is followed
// to the address it answers at next. A small DOM boundary; no request leaves.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function takeover(attributes, answers) {
  const root = {
    getAttribute: name => attributes[name] ?? null,
    querySelectorAll: () => [],
    querySelector: () => null,
  };
  const timers = [];
  const assigned = [];
  const asked = [];
  let now = 0;
  const context = vm.createContext({
    document: {querySelector: selector => (selector === '[data-verso-restarting]' ? root : null)},
    window: {
      setTimeout: f => { timers.push(f); return timers.length; },
      clearTimeout() {}, setInterval() { return 0; }, clearInterval() {},
      location: {assign: url => assigned.push(url)},
    },
    Date: {now: () => now},
    versoMinutes: () => '0:00',
    fetch: (url, options) => {
      asked.push([url, options && options.mode]);
      return answers(url, options);
    },
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-takeover.js'), 'utf8'), context);
  // The first timer is the stalled deadline, which these tests never reach;
  // run fires the one scheduled last: the next poll, or the next probe.
  const run = async () => { if (timers.length > 1) timers.pop()(); await new Promise(setImmediate); };
  return {timers, assigned, asked, run, advance: ms => { now += ms; }};
}
const json = {ok: true, redirected: false, headers: {get: () => 'application/json'}};
const attributes = {
  'data-verso-restarting-status': '/system/maintenance/restart/status',
  'data-verso-restarting-budget': '60',
  'data-verso-restarting-target': 'https://router.lan/',
};

test('a moving shell is followed once its old address stops answering as the shell', async () => {
  let up = true;
  const t = takeover(attributes, (url, options) => {
    if (url === attributes['data-verso-restarting-status']) return up ? Promise.resolve(json) : Promise.reject(Error('gone'));
    assert.equal(options.mode, 'no-cors', 'the new address is asked without reading its answer');
    return Promise.resolve({type: 'opaque'});
  });
  await t.run();
  await t.run();
  assert.equal(t.assigned.length, 0, 'the shell still answers here: keep watching');
  up = false;
  await t.run();
  assert.deepEqual(t.assigned, ['https://router.lan/']);
});

test('an old address that answers as something else is left for the new one too', async () => {
  const luci = {ok: true, redirected: false, headers: {get: () => 'text/html'}};
  const t = takeover(attributes, (url) => url === attributes['data-verso-restarting-status'] ? Promise.resolve(luci) : Promise.resolve({type: 'opaque'}));
  await t.run();
  assert.deepEqual(t.assigned, ['https://router.lan/']);
});

test('a new address the browser will not reach yet is loaded anyway after a short wait', async () => {
  const t = takeover(attributes, (url) => url === attributes['data-verso-restarting-status'] ? Promise.reject(Error('gone')) : Promise.reject(Error('untrusted')));
  // the poll fails, the follow starts, and its probes keep failing
  for (let i = 0; i < 40 && t.assigned.length === 0; i++) {
    t.advance(1000);
    await t.run();
  }
  assert.deepEqual(t.assigned, ['https://router.lan/'], 'the browser is sent to say why itself');
  assert.ok(t.asked.filter(([url]) => url === 'https://router.lan/').length > 1, 'it asked more than once first');
});

test('a restart that does not move waits for its own address as before', async () => {
  const {['data-verso-restarting-target']: _, ...here} = attributes;
  const t = takeover(here, () => Promise.reject(Error('gone')));
  for (let i = 0; i < 5; i++) await t.run();
  assert.equal(t.assigned.length, 0);
});
