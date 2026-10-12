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
    // A chart's scale for a plot whose tallest point is peak bits per second:
    // a round top four round steps high, at least a tenth over the peak and
    // at least 0.4 Mb/s, in Gb/s once it would reach 1000 Mb/s, and its
    // labels at the quarter lines, the first that rate in full.
    function chartScale(peak) {
      if (!(peak > 0) || peak === Infinity) peak = 0;
      var need = Math.max(peak * 1.1, 400000);
      var unit = "Mb/s", per = 1e6;
      var step = roundStep(need / per / 4);
      if (4 * step >= 1000) {
        unit = "Gb/s"; per = 1e9;
        step = roundStep(need / per / 4);
      }
      var labels = AXIS.map(function (f, i) {
        return figure(4 * step * f) + (i === 0 ? " " + unit : "");
      });
      return { top: 4 * step * per, labels: labels };
    }
    return { figure: figure, rate: rate, chartScale: chartScale };
  }

  // The steps a scale counts in, times a power of ten; a step must be whole
  // tenths, since a figure keeps one decimal.
  var STEPS = [1, 1.5, 2, 2.5, 3, 4, 5, 6, 8];
  var AXIS = [1, 0.75, 0.5, 0.25];
  function roundStep(v) {
    for (var p = Math.pow(10, Math.floor(Math.log10(v))); ; p *= 10) {
      for (var i = 0; i < STEPS.length; i++) {
        var s = STEPS[i] * p;
        if (Math.abs(s * 10 - Math.round(s * 10)) > 1e-9) continue;
        if (s >= v * (1 - 1e-9)) return s;
      }
    }
  }

  root.versoUnits = { numbers: numbers };
})(window);
