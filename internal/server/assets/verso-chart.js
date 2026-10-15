// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-chart.js — a live traffic graph (widget.Traffic) in two lanes,
// download over upload: the overview's Internet graph and a device's in its
// drawer. It seeds both lanes from the minute the server rendered
// (data-verso-live-seed) and then FLOWS left continuously as fresh samples
// arrive from its stream, named by data-verso-live-chart ("wan",
// "usage:<mac>"). Each frame the curves translate by a fraction of one sample;
// when a sample lands the value is committed and the translate resets — a
// seamless treadmill drawing the same smooth curve the server drew. Each lane
// keeps its own round scale, figure and peak. The grid stands still under the
// curve, so each time label under the plot keeps its line. A clip hides what
// scrolls past the edges. Reduced motion still updates, just in discrete steps
// instead of a glide.
//
// A graph is set up when it is on the page, and when a panel's contents
// arrive with one in them (htmx), so a graph in a drawer starts only once the
// drawer is opened. Streams hand it samples with window.versoLiveSample.
(function () {
  "use strict";
  var NS = "http://www.w3.org/2000/svg";
  var INTERVAL = 1000;

  // The pure arithmetic, apart so it can be held to cases
  // (scripts/chart.test.cjs).
  var math = {
    // peak is the highest finite value among vals, zero when there is none.
    peak: function (vals) {
      var p = 0;
      vals.forEach(function (v) { if (typeof v === "number" && isFinite(v) && v > p) p = v; });
      return p;
    },
    // laneScale is a lane's round scale over its points (Mbit/s), written by
    // numbers (verso-units.js): its top in Mbit/s and its labels.
    laneScale: function (vals, numbers) {
      var scaled = numbers.chartScale(math.peak(vals) * 1e6);
      return { top: scaled.top / 1e6, labels: scaled.labels };
    },
  };
  window.versoChartMath = math;
  if (typeof document === "undefined" || !window.matchMedia) return;

  var reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var clips = 0;
  // Figures and value labels are written as the shell writes every rate
  // (verso-units.js), in the page's language. The series are in Mbit/s.
  var tr = typeof T === "function" ? T : function (s) { return s; };
  var nums = window.versoUnits.numbers(tr("."));

  function rateText(mbps) {
    var rate = nums.rate(mbps * 1e6);
    return rate.value + " " + rate.unit;
  }

  // lane is one plot's live state: its series (the visible minute and one
  // extra point off-right until a sample lands), its scale, and the elements
  // it rewrites.
  function lane(el, vals, step) {
    var svg = el.querySelector("svg.verso-chart");
    if (!svg) return null;
    var vb = svg.viewBox.baseVal;
    var W = vb.width, H = vb.height;
    var N = vals.length - 1;
    var g = svg.querySelector("g.verso-chart-series");
    var self = {
      W: W, H: H, svg: svg, vals: vals, max: 0,
      line: g && g.querySelector(".verso-chart-line"),
      area: g && g.querySelector(".verso-chart-area"),
      peakLine: svg.querySelector(".verso-chart-peak"),
      labels: el.querySelectorAll(".verso-chart-yl"),
      figure: el.querySelector("[data-verso-live-figure]"),
      peakText: el.querySelector("[data-verso-live-peak]"),
    };
    // Clip the series to the plot so the treadmill's off-edge content hides;
    // each lane its own clip, since a drawer's graph sits beside the page's.
    var clipID = "verso-live-clip-" + (++clips);
    var clip = document.createElementNS(NS, "clipPath");
    clip.setAttribute("id", clipID);
    var rect = document.createElementNS(NS, "rect");
    rect.setAttribute("x", "0"); rect.setAttribute("y", "0");
    rect.setAttribute("width", String(W)); rect.setAttribute("height", String(H));
    clip.appendChild(rect); svg.appendChild(clip);
    if (g) g.setAttribute("clip-path", "url(#" + clipID + ")");

    function y(v) { return H - (v / self.max) * H; }
    // The same Catmull-Rom spline the server draws, so the live curve matches.
    function segments(p) {
      var s = "";
      for (var k = 0; k < p.length - 1; k++) {
        var p0 = k > 0 ? p[k - 1] : p[k], p1 = p[k], p2 = p[k + 1], p3 = k + 2 < p.length ? p[k + 2] : p2;
        var lo = Math.min(p1[1], p2[1]), hi = Math.max(p1[1], p2[1]);
        var c1y = Math.max(lo, Math.min(hi, p1[1] + (p2[1] - p0[1]) / 6));
        var c2y = Math.max(lo, Math.min(hi, p2[1] - (p3[1] - p1[1]) / 6));
        s += " C" + (p1[0] + (p2[0] - p0[0]) / 6).toFixed(1) + " " + c1y.toFixed(1) +
          " " + (p2[0] - (p3[0] - p1[0]) / 6).toFixed(1) + " " + c2y.toFixed(1) +
          " " + p2[0].toFixed(1) + " " + p2[1].toFixed(1);
      }
      return s;
    }
    // The lane's scale is the round one the server drew (units.RateScale),
    // over the whole visible minute and the point about to scroll in, so it
    // grows just before a new peak arrives; it moves only when the traffic
    // crosses a step, and then its labels are rewritten.
    function scale() {
      var scaled = math.laneScale(self.vals, nums);
      if (scaled.top === self.max) return;
      self.max = scaled.top;
      self.labels.forEach(function (label, i) {
        if (scaled.labels[i] !== undefined) label.textContent = scaled.labels[i];
      });
    }
    self.draw = function () {
      scale();
      var p = self.vals.map(function (v, j) { return [j * step, y(v)]; });
      var head = "M" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p);
      if (self.line) { self.line.setAttribute("d", head); self.line.style.transform = "none"; }
      if (self.area) {
        self.area.setAttribute("d", "M0 " + H.toFixed(1) + " L" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p) + " L" + p[N][0].toFixed(1) + " " + H.toFixed(1) + " Z");
        self.area.style.transform = "none";
      }
      // The minute's peak, its words and its dotted line.
      var peak = math.peak(self.vals.slice(0, N));
      if (self.peakText) self.peakText.textContent = rateText(peak);
      if (self.peakLine) {
        var py = y(peak).toFixed(1);
        self.peakLine.setAttribute("y1", py); self.peakLine.setAttribute("y2", py);
      }
    };
    self.slide = function (tf) {
      if (self.line) self.line.style.transform = tf;
      if (self.area) self.area.style.transform = tf;
    };
    return self;
  }

  function liveChart(host) {
    if (host.__versoLive) return host.__versoLive;
    var seed = {};
    try { seed = JSON.parse(host.getAttribute("data-verso-live-seed") || "{}"); } catch (e) { seed = {}; }
    var down = (seed.down || []).slice(), up = (seed.up || []).slice();
    var N = Math.min(down.length, up.length); // visible points fill 0..W
    if (N < 2) return null;
    down.push(down[N - 1]); up.push(up[N - 1]); // one extra, off-right, until a sample lands
    var downEl = host.querySelector('[data-verso-live-lane="down"]');
    var upEl = host.querySelector('[data-verso-live-lane="up"]');
    var probe = downEl && downEl.querySelector("svg.verso-chart");
    if (!probe || !upEl) return null;
    var step = probe.viewBox.baseVal.width / (N - 1);
    var lanes = [lane(downEl, down, step), lane(upEl, up, step)].filter(Boolean);
    if (lanes.length < 2) return null;

    var lastCommit = null;
    var gliding = false;
    // One glide per sample: the curves slide one step left over the sample
    // interval, and once they have arrived the loop stops until the next sample
    // restarts it. A stream that has stopped (the tab hidden, the router gone,
    // the drawer closed) leaves nothing running.
    function frame(ts) {
      var progress = Math.min(1, (ts - lastCommit) / INTERVAL);
      // A CSS transform, not the SVG attribute: the attribute re-lays the chart
      // out on every frame, the style only repaints it. In the SVG's own user
      // units, which is what px means inside a viewBox.
      var tf = "translateX(" + (-progress * step).toFixed(2) + "px)";
      lanes.forEach(function (l) { l.slide(tf); });
      if (progress < 1) requestAnimationFrame(frame);
      else gliding = false;
    }

    lanes.forEach(function (l) { l.draw(); });
    host.__versoLive = {
      // A fresh sample: commit the newest down/up, drop the oldest, redraw, and
      // start the glide toward it if one is not already running.
      sample: function (nd, nu) {
        if (typeof nd !== "number" || typeof nu !== "number") return;
        [nd, nu].forEach(function (v, i) {
          var l = lanes[i];
          l.vals.push(v); l.vals.shift();
          l.draw();
          if (l.figure) l.figure.textContent = rateText(v);
        });
        lastCommit = performance.now();
        if (!reduce && !gliding) {
          gliding = true;
          requestAnimationFrame(frame);
        }
      },
    };
    return host.__versoLive;
  }

  function setUp(root) {
    (root || document).querySelectorAll("[data-verso-live-chart]").forEach(liveChart);
  }

  // A stream's sample for the graph it names, wherever that graph stands now.
  window.versoLiveSample = function (name, down, up) {
    var host = document.querySelector('[data-verso-live-chart="' + CSS.escape(name) + '"]');
    var chart = host && liveChart(host);
    if (chart) chart.sample(down, up);
  };

  setUp();
  document.addEventListener("htmx:afterSwap", function (e) { setUp(e.target); });
})();
