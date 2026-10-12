// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/units.test.cjs
// The page's live rates (verso-units.js) against the cases the shell's own
// rule is held to (internal/units/testdata/rates.json), so both round alike.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function units() {
  const context = vm.createContext({window: {}});
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-units.js'), 'utf8'), context);
  return context.window.versoUnits;
}

const U = units();
const fixtures = JSON.parse(fs.readFileSync(path.join(__dirname, '../internal/units/testdata/rates.json'), 'utf8'));
const comma = s => s.replace('.', ',');

test('rates read at their scale', () => {
  for (const tc of fixtures.rates) {
    const got = U.numbers('.').rate(tc.bps);
    assert.deepEqual({value: got.value, unit: got.unit}, {value: tc.value, unit: tc.unit}, `rate(${tc.bps})`);
    assert.equal(U.numbers(',').rate(tc.bps).value, comma(tc.value), `rate(${tc.bps}) with a decimal comma`);
  }
});

test('a chart scale is round', () => {
  for (const tc of fixtures.scales) {
    const got = U.numbers('.').chartScale(tc.peak_bps);
    assert.ok(Math.abs(got.top - tc.top_bps) <= 1, `chartScale(${tc.peak_bps}).top = ${got.top}`);
    assert.deepEqual([...got.labels], tc.labels, `chartScale(${tc.peak_bps})`);
    assert.deepEqual([...U.numbers(',').chartScale(tc.peak_bps).labels], tc.labels.map(comma), `chartScale(${tc.peak_bps}) with a decimal comma`);
  }
  for (const peak of [NaN, Infinity, -3]) assert.equal(U.numbers('.').chartScale(peak).top, 400000, `chartScale(${peak})`);
});

test('a missing separator keeps the point', () => {
  assert.equal(U.numbers('').figure(2.5), '2.5');
});
