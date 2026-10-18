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

// play runs a player over a timeline: samples arriving at the times given,
// the page drawing a frame every 16 ms until `until`. It answers each frame's
// progress and the time of each commit.
function play(arrivals, until) {
  const p = math.player(1000);
  const frames = [], commits = [];
  let next = 0;
  for (let ts = 0; ts <= until; ts += 16) {
    while (next < arrivals.length && arrivals[next] <= ts) p.add(next++);
    const at = p.tick(ts);
    at.commits.forEach(s => commits.push({ts, s}));
    frames.push({ts, progress: at.progress, idle: at.idle});
  }
  return {frames, commits};
}

test('the graph keeps one sample in hand before it moves', () => {
  const {frames, commits} = play([0], 3000);
  assert.equal(commits.length, 0, 'one sample alone waits');
  assert.ok(frames.every(f => f.idle && f.progress === 0), 'standing as first drawn');
  const two = play([0, 1000], 1500);
  assert.deepEqual(two.commits.map(c => c.s), [0], 'the second lets the first glide in');
});

test('samples arriving early or late glide in on a steady clock', () => {
  // A second's batch ~135 ms late, the next on time, as the router sent them.
  const arrivals = [0, 1000, 2000, 3135, 4000, 5000, 6140, 7000, 8000, 9000];
  const {frames, commits} = play(arrivals, 9000);
  const gaps = commits.slice(1).map((c, i) => c.ts - commits[i].ts);
  assert.ok(gaps.every(g => Math.abs(g - 1000) <= 16), `commits a second apart, a frame's leeway: ${gaps}`);
  const moving = frames.filter(f => f.ts > 1000);
  assert.ok(moving.every(f => !f.idle), 'never standing still for a late sample');
  assert.ok(moving.every((f, i) => i === 0 || f.progress > moving[i - 1].progress || f.progress < 0.05), 'the glide only goes forward, starting over at each commit');
});

test('a sample more than a second late stops the glide, and its arrival starts it again', () => {
  const {frames, commits} = play([0, 1000, 2000, 4500, 5500], 6000);
  assert.ok(frames.some(f => f.ts > 3000 && f.ts < 4500 && f.idle && f.progress === 1), 'waiting at the end of the last glide');
  assert.ok(commits.some(c => c.s === 3 && c.ts >= 4500 && c.ts < 4520), 'the late one glides in as it lands');
});

test('a backlog is caught up so the graph never falls behind', () => {
  const p = math.player(1000);
  for (let i = 0; i < 6; i++) p.add(i);
  const at = p.tick(0);
  assert.deepEqual([...at.commits], [0, 1, 2, 3], 'all but two in hand land at once');
  p.reset();
  assert.equal(p.tick(5000).commits.length, 0, 'reset forgets what was in hand');
});

test('a minute taken up again fills the graph\'s points, newest at the right', () => {
  assert.deepEqual([...math.minute([1, 2, 3, 4, 5], 3)], [3, 4, 5], 'a longer minute keeps its newest');
  assert.deepEqual([...math.minute([4, 5], 4)], [0, 0, 4, 5], 'a shorter one is quiet before it');
  assert.deepEqual([...math.minute([7, 8, 9], 3)], [7, 8, 9], 'a minute as long is itself');
});
