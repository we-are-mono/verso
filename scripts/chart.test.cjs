// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/chart.test.cjs
// The live traffic graph's arithmetic (verso-chart.js): each lane's scale and
// its peak.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function load() {
  const context = vm.createContext({window: {}});
  for (const file of ['verso-units.js', 'verso-chart.js']) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets', file), 'utf8'), context);
  }
  return {math: context.window.versoChartMath, numbers: context.window.versoUnits.numbers('.')};
}

const {math, numbers} = load();

test('a peak is the highest reading, a gap none', () => {
  assert.equal(math.peak([1, NaN, 7, undefined, 3]), 7);
  assert.equal(math.peak([]), 0);
});

test('each lane keeps its own round scale, moving only when a step is crossed', () => {
  assert.equal(math.laneScale([0, 57], numbers).top, 80, 'the download lane');
  assert.equal(math.laneScale([0, 8.5], numbers).top, 10, 'the upload lane');
  assert.equal(math.laneScale([0, 9], numbers).top, 10, 'a little more stays on its step');
  assert.equal(math.laneScale([0, 9.2], numbers).top, 12, 'past a tenth under the top, the next step');
  assert.deepEqual([...math.laneScale([0, 8.5], numbers).labels], ['10 Mb/s', '7.5', '5', '2.5']);
});
