// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-chart.js — a live traffic graph (widget.Traffic) in two lanes,
// download over upload: the overview's Internet graph and a device's in its
// drawer. It seeds both lanes from the minute the server rendered
// (data-verso-live-seed) and then FLOWS left continuously as fresh samples
// arrive from its stream, named by data-verso-live-chart ("wan",
// "usage:<mac>"). Each frame the curves translate by a fraction of one sample;
// every interval the next sample is committed and the translate resets — a
// seamless treadmill drawing the same smooth curve the server drew, paced by
// a steady clock one sample behind the stream, so a sample that lands late
// or early never shows as a pause or a jump (math.player). Each lane
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
    // numbers (verso-units.js): its top in Mbit/s, its bare figures and the
    // unit written once over them.
    laneScale: function (vals, numbers) {
      var scaled = numbers.chartScale(math.peak(vals) * 1e6);
      return { top: scaled.top / 1e6, figures: scaled.figures, unit: scaled.unit };
    },
    // fit is the widest plot of whole cells, among cells that fit, cut into a
    // whole number of one of the window's steps; steps come in order of
    // preference, which breaks a tie.
    fit: function (cells, steps) {
      var best = { cols: 0, step: steps[0] };
      steps.forEach(function (n) {
        var cols = Math.floor(cells / n) * n;
        if (cols > best.cols) best = { cols: cols, step: n };
      });
      return best;
    },
    // minute is a minute the router sampled, fitted to a graph of n points:
    // its newest n, quiet before it where it holds fewer, as the server pads
    // a minute only part sampled.
    minute: function (vals, n) {
      var out = vals.slice(-n);
      while (out.length < n) out.unshift(0);
      return out;
    },
    // player paces a live graph by a steady clock instead of by when its
    // samples land. Samples arrive a second apart only on average: the router
    // sends one late when a read holds it, and the next one early. Gliding
    // each in as it landed made the graph stop short for a late one and jump
    // for an early one, so the player keeps one sample in hand and lets the
    // next glide in every interval: a sample up to a second late or early
    // lands in hand before it is wanted. Its tick says, for a frame at ts,
    // which samples to commit now, how far the glide toward the newest has
    // gone (0 to 1), and whether it stands waiting for a sample.
    player: function (interval) {
      var queue = [], start = null, primed = false;
      return {
        add: function (s) { queue.push(s); },
        reset: function () { queue = []; start = null; primed = false; },
        tick: function (ts) {
          var commits = [];
          if (start === null) {
            // Moving first waits for a sample in hand behind the one that
            // glides; after a stall the late one glides in as it lands.
            if (queue.length < (primed ? 1 : 2)) return { commits: commits, progress: primed ? 1 : 0, idle: true };
            primed = true;
            start = ts;
            commits.push(queue.shift());
          }
          while (ts - start >= interval) {
            if (!queue.length) {
              start = null;
              return { commits: commits, progress: 1, idle: true };
            }
            start += interval;
            commits.push(queue.shift());
          }
          // More than two in hand is a backlog (a page that was away): it
          // lands at once, so the graph never falls behind the router.
          while (queue.length > 2) commits.push(queue.shift());
          return { commits: commits, progress: (ts - start) / interval, idle: false };
        },
      };
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
    return rate.value + window.versoUnits.space + rate.unit;
  }

  // lane is one plot's live state: its series (the visible minute and one
  // extra point off-right until a sample lands), its scale, and the elements
  // it rewrites.
  function lane(el, vals, step) {
    var svg = el.querySelector(".verso-chart__svg");
    if (!svg) return null;
    var vb = svg.viewBox.baseVal;
    var W = vb.width, H = vb.height;
    var N = vals.length - 1;
    var g = svg.querySelector(".verso-chart__series");
    var self = {
      W: W, H: H, svg: svg, vals: vals, max: 0,
      line: g && g.querySelector(".verso-chart__line"),
      area: g && g.querySelector(".verso-chart__area"),
      peakLine: svg.querySelector(".verso-chart__peak"),
      labels: el.querySelectorAll(".verso-chart__scale:not([data-verso-chart-unit])"),
      unit: el.querySelector("[data-verso-chart-unit]"),
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

    // As the server places it (widget.plotY): one viewBox unit is one pixel,
    // and nothing stands lower than one pixel over the bottom border.
    function y(v) { return Math.min(H - (v / self.max) * H, H - 1); }
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
        if (scaled.figures[i] !== undefined) label.textContent = scaled.figures[i];
      });
      if (self.unit) self.unit.textContent = scaled.unit;
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
    var probe = downEl && downEl.querySelector(".verso-chart__svg");
    if (!probe || !upEl) return null;
    var step = probe.viewBox.baseVal.width / (N - 1);
    var lanes = [lane(downEl, down, step), lane(upEl, up, step)].filter(Boolean);
    if (lanes.length < 2) return null;

    // commit takes one sample into the lanes: the newest down/up stands
    // off-right, the oldest drops, and the figures say it.
    function commit(sample) {
      sample.forEach(function (v, i) {
        var l = lanes[i];
        l.vals.push(v); l.vals.shift();
        l.draw();
        if (l.figure) l.figure.textContent = rateText(v);
      });
    }
    // slide moves the curves the glide's share of one step left. A CSS
    // transform, not the SVG attribute: the attribute re-lays the chart out
    // on every frame, the style only repaints it. In the SVG's own user
    // units, which is what px means inside a viewBox.
    function slide(progress) {
      var tf = "translateX(" + (-progress * step).toFixed(2) + "px)";
      lanes.forEach(function (l) { l.slide(tf); });
    }
    // The player paces the glide (math.player): the loop runs while it has a
    // sample to glide in and stops when it waits for one, so a stream that
    // has stopped (the tab hidden, the router gone, the drawer closed) leaves
    // nothing running.
    var player = math.player(INTERVAL);
    var looping = false;
    function frame(ts) {
      var at = player.tick(ts);
      at.commits.forEach(commit);
      slide(at.progress);
      if (at.idle) looping = false;
      else requestAnimationFrame(frame);
    }

    lanes.forEach(function (l) { l.draw(); });
    host.__versoLive = {
      // A fresh sample goes to the player, which glides it in on its beat;
      // under reduced motion it lands at once, a step and no glide.
      sample: function (nd, nu) {
        if (typeof nd !== "number" || typeof nu !== "number") return;
        if (reduce) {
          commit([nd, nu]);
          return;
        }
        player.add([nd, nu]);
        if (!looping) {
          looping = true;
          requestAnimationFrame(frame);
        }
      },
      // The minute as the router sampled it, taken up when the stream opens
      // again: whatever the graph missed while it was away (a hidden tab, a
      // dropped stream) is drawn where it happened, what the player held
      // goes, and the curves stand still until it has a sample in hand
      // again, as on the first paint.
      seed: function (nd, nu) {
        if (!Array.isArray(nd) || !Array.isArray(nu)) return;
        player.reset();
        [nd, nu].forEach(function (vals, i) {
          var l = lanes[i];
          var fresh = math.minute(vals, N);
          fresh.push(fresh[N - 1]);
          l.vals.splice.apply(l.vals, [0, l.vals.length].concat(fresh));
          l.draw();
          if (l.figure) l.figure.textContent = rateText(fresh[N - 1]);
        });
      },
    };
    return host.__versoLive;
  }

  // fitGraph widens a chart's plot to the most whole cells its room holds,
  // cut into whole steps of its window, and shows that step's time axis; it
  // keeps fitting as the room changes.
  var fits = window.ResizeObserver ? new ResizeObserver(function (entries) {
    entries.forEach(function (e) { fitGraph(e.target); });
  }) : null;
  function fitGraph(g) {
    var plot = g.querySelector(".verso-chart__plot");
    var gutter = g.querySelector("[data-verso-chart-gutter]");
    if (!plot || !gutter) return;
    var cell = parseFloat(getComputedStyle(plot).backgroundSize);
    if (!(cell > 0)) return;
    var steps = (g.getAttribute("data-verso-steps") || "4").split(",").map(Number);
    var best = math.fit(Math.floor((g.clientWidth - gutter.getBoundingClientRect().width) / cell), steps);
    if (!best.cols) return;
    g.style.setProperty("--verso-chart-cols", String(best.cols));
    g.setAttribute("data-verso-axis", String(best.step));
  }

  function setUp(root) {
    (root || document).querySelectorAll(".verso-chart").forEach(function (g) {
      fitGraph(g);
      if (fits) fits.observe(g);
    });
    (root || document).querySelectorAll("[data-verso-live-chart]").forEach(liveChart);
  }

  // The live graph a stream names, wherever it stands now.
  function named(name) {
    var host = document.querySelector('[data-verso-live-chart="' + CSS.escape(name) + '"]');
    return host && liveChart(host);
  }
  // A stream's sample for the graph it names.
  window.versoLiveSample = function (name, down, up) {
    var chart = named(name);
    if (chart) chart.sample(down, up);
  };
  // A stream's whole minute for the graph it names, sent as it opens.
  window.versoLiveSeed = function (name, down, up) {
    var chart = named(name);
    if (chart) chart.seed(down, up);
  };

  setUp();
  // A swap can replace the graph itself (a window chosen, a reading picked),
  // so the whole page is looked over again; a graph set up already keeps its
  // state.
  document.addEventListener("htmx:afterSwap", function () { setUp(); });
})();
