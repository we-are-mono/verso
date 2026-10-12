// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-units.js — how the page writes a live rate, the shell's rule
// (internal/units) restated for the figures a stream moves every second. A
// figure keeps one decimal under ten and is whole from ten; a rate is in Mb/s
// up to 999 and in Gb/s from there, and reads at least 0.1 Mb/s above zero.
// Both are held to the cases in internal/units/testdata/rates.json.
//
// The decimal separator is the catalog's "." (ADR-012), passed in by the
// caller: T(".") on the page.
(function (root) {
  "use strict";

  function numbers(decimal) {
    decimal = decimal || ".";

    function figure(v) {
      var tenths = Math.round(v * 10) / 10;
      if (tenths >= 10) return String(Math.round(v));
      return tenths.toFixed(1).replace(/\.0$/, "").replace(".", decimal);
    }
    // The unit a rate of bps is written in, and the bits per second in one of
    // it. A rate that rounds to 1000 Mb/s is already 1 Gb/s.
    function scale(bps) {
      return Math.round(bps / 1e6) >= 1000 ? { unit: "Gb/s", per: 1e9 } : { unit: "Mb/s", per: 1e6 };
    }
    function rate(bps) {
      if (!(bps > 0)) return { value: "0", unit: "Mb/s" };
      bps = Math.max(bps, 100000);
      var s = scale(bps);
      return { value: figure(bps / s.per), unit: s.unit };
    }
    // A chart's value labels for a plot topped at top bits per second: the
    // first that rate in full, the rest figures in its unit.
    function axis(top, fractions) {
      var s = scale(top);
      return fractions.map(function (f, i) {
        return figure((top * f) / s.per) + (i === 0 ? " " + s.unit : "");
      });
    }
    return { figure: figure, rate: rate, axis: axis };
  }

  root.versoUnits = { numbers: numbers, axisFractions: [1, 0.75, 0.5, 0.25] };
})(window);
