// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
// Run with: node --test scripts/datatypes.test.cjs
// A list's add (window.versoValidate in verso-forms.js) against the cases the
// shell's own datatype is held to (internal/datatype/testdata), so a token the
// browser takes is one the shell takes.
const assert = require('node:assert/strict');
const {test} = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function validate() {
  const window = {addEventListener() {}, requestAnimationFrame: fn => fn()};
  const document = {
    body: {},
    querySelector: () => null,
    querySelectorAll: () => [],
    getElementById: () => null,
    addEventListener() {},
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../internal/server/assets/verso-forms.js'), 'utf8'), {
    document, window, MutationObserver: class { observe() {} }, URL, T: s => s,
  });
  return window.versoValidate;
}

const versoValidate = validate();
const cidr = JSON.parse(fs.readFileSync(path.join(__dirname, '../internal/datatype/testdata/cidr.json'), 'utf8'));

test('a cidr token is an address and its prefix length', () => {
  for (const value of cidr.accept) {
    assert.equal(versoValidate('cidr', value), '', `accepts ${value}`);
  }
  for (const {value, why} of cidr.refuse) {
    assert.equal(versoValidate('cidr', value), 'Enter an IP address with its prefix length, such as 10.0.0.1/24.',
      `refuses ${JSON.stringify(value)}: ${why}`);
  }
});
