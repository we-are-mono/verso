// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: APK=/path/to/apk node scripts/check-feed-arch.test.cjs
// A feed takes only packages built for its architecture, or for none: the check
// that stands before a feed is indexed and published. Fixture packages are built
// unsigned with the buildroot's apk tool (APK), which `make test` passes.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

const script = path.join(__dirname, 'check-feed-arch.sh');
const apk = process.env.APK || '';
const skip = (() => {
  try {
    fs.accessSync(apk, fs.constants.X_OK);
    return false;
  } catch {
    return `no executable apk tool at APK=${JSON.stringify(apk)}; it builds the fixture packages`;
  }
})();

// feed makes a directory holding one fixture package per {name: arch} entry.
function feed(packages) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'verso-feed-'));
  for (const [name, arch] of Object.entries(packages)) {
    const files = fs.mkdtempSync(path.join(os.tmpdir(), 'verso-pkg-'));
    fs.mkdirSync(path.join(files, 'usr/share', name), {recursive: true});
    fs.writeFileSync(path.join(files, 'usr/share', name, 'marker'), name);
    const built = spawnSync(apk, [
      'mkpkg', '--info', `name:${name}`, '--info', 'version:1.0.0-r0', '--info', `arch:${arch}`,
      '--files', files, '--output', path.join(dir, `${name}-1.0.0-r0.apk`),
    ], {encoding: 'utf8'});
    assert.equal(built.status, 0, built.stderr);
  }
  return dir;
}

function check(dir, arch) {
  return spawnSync('sh', [script, dir, arch], {encoding: 'utf8', env: {...process.env, APK: apk}});
}

test('a feed of its own architecture passes', {skip}, () => {
  const result = check(feed({verso: 'x86_64', 'verso-plugin-qos': 'x86_64'}), 'x86_64');
  assert.equal(result.status, 0, result.stderr);
});

test('an architecture-independent package passes in any feed', {skip}, () => {
  const packages = {verso: 'x86_64', 'verso-i18n-base-sl': 'noarch'};
  assert.equal(check(feed(packages), 'x86_64').status, 0);
  assert.equal(check(feed({verso: 'aarch64_generic', 'verso-i18n-base-sl': 'noarch'}), 'aarch64_generic').status, 0);
});

test('a package of another architecture stops the feed, named with both architectures', {skip}, () => {
  const result = check(feed({verso: 'x86_64', 'verso-plugin-vpn': 'aarch64_generic'}), 'x86_64');
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /verso-plugin-vpn-1\.0\.0-r0\.apk/);
  assert.match(result.stderr, /aarch64_generic/);
  assert.match(result.stderr, /x86_64/);
  assert.doesNotMatch(result.stderr, /verso-1\.0\.0-r0\.apk/);
});

test('an empty feed passes', {skip}, () => {
  assert.equal(check(feed({}), 'x86_64').status, 0);
});

test('a missing feed directory is an error', () => {
  const result = check(path.join(os.tmpdir(), 'verso-feed-that-is-not-there'), 'x86_64');
  assert.notEqual(result.status, 0);
});
