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
  const upload = math.laneScale([0, 8.5], numbers);
  assert.deepEqual([...upload.figures], ['10', '7.5', '5', '2.5'], 'bare figures');
  assert.equal(upload.unit, 'Mb/s', 'the unit, written once over them');
});

test('a plot takes the most whole cells that cut into whole steps', () => {
  assert.deepEqual({...math.fit(17, [4, 6, 3])}, {cols: 16, step: 4}, 'quarters win at 17 cells');
  assert.deepEqual({...math.fit(18, [4, 6, 3])}, {cols: 18, step: 6}, 'sixths win at 18');
  assert.deepEqual({...math.fit(15, [4, 6, 3])}, {cols: 15, step: 3}, 'thirds win at 15');
  assert.deepEqual({...math.fit(12, [4, 6, 3])}, {cols: 12, step: 4}, 'a tie goes to the first offered');
  assert.deepEqual({...math.fit(29, [6, 3])}, {cols: 27, step: 3}, 'a month offers sixths and thirds');
});

test('a minute taken up again fills the graph\'s points, newest at the right', () => {
  assert.deepEqual([...math.minute([1, 2, 3, 4, 5], 3)], [3, 4, 5], 'a longer minute keeps its newest');
  assert.deepEqual([...math.minute([4, 5], 4)], [0, 0, 4, 5], 'a shorter one is quiet before it');
  assert.deepEqual([...math.minute([7, 8, 9], 3)], [7, 8, 9], 'a minute as long is itself');
});
