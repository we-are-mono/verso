// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-chart.js — a live traffic graph (widget.Traffic): the overview's
// Internet graph and a device's in its panel. Each seeds from the minute the
// server rendered (data-verso-live-seed) and then FLOWS left continuously as
// fresh samples arrive from its stream, named by data-verso-live-chart ("wan",
// "usage:<mac>"). Each frame the curves translate by a fraction of one sample;
// when a sample lands the value is committed and the translate resets — a
// seamless treadmill drawing the same smooth curve and dynamic shared scale the
// server drew. A clip hides what scrolls past the edges. Reduced motion still
// updates, just in discrete steps instead of a glide.
//
// A graph is set up when it is on the page, and when a panel's contents
// arrive with one in them (htmx), so a graph in a drawer starts only once the
// drawer is opened. Streams hand it samples with window.versoLiveSample.
(function () {
  "use strict";
  var NS = "http://www.w3.org/2000/svg";
  var INTERVAL = 1000;
  var GRID = 20;
  var reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var clips = 0;

  function liveChart(host) {
    if (host.__versoLive) return host.__versoLive;
    var svg = host.querySelector("svg.verso-chart");
    if (!svg) return null;
    var vb = svg.viewBox.baseVal;
    var W = vb.width, H = vb.height, PAD = svg.hasAttribute("data-chart-padding") ? Number(svg.getAttribute("data-chart-padding")) : 8;
    var groups = svg.querySelectorAll("g.verso-chart-series");
    var frameEl = host.closest("section") || host;
    var downEl = frameEl.querySelector("[data-verso-live-down]");
    var upEl = frameEl.querySelector("[data-verso-live-up]");

    // Seed the live series from the server's rendered minute.
    var seed = {};
    try { seed = JSON.parse(host.getAttribute("data-verso-live-seed") || "{}"); } catch (e) { seed = {}; }
    var down = (seed.down || []).slice(), up = (seed.up || []).slice();
    var N = Math.min(down.length, up.length); // visible points fill 0..W
    if (N < 2) return null;
    down.push(down[N - 1]); up.push(up[N - 1]); // one extra, off-right, until a sample lands
    var step = W / (N - 1);
    var series = [down, up];

    // Download and upload share one domain so their relative sizes remain
    // truthful. Recalculate it from the whole visible minute whenever a sample
    // arrives; the 15% headroom matches the server-rendered chart and keeps the
    // tallest point clear of the top edge. Including the off-right point grows
    // the scale just before that new value scrolls into view.
    var max = 1;
    function scaleMax() {
      var peak = 0;
      series.forEach(function (vals) {
        vals.forEach(function (v) {
          if (typeof v === "number" && isFinite(v) && v > peak) peak = v;
        });
      });
      return peak > 0 ? peak * 1.15 : 1;
    }
    function axisValue(v) {
      return v >= 10 ? Math.round(v).toString() : (Math.round(v * 10) / 10).toString();
    }
    function updateScale() {
      max = scaleMax();
      host.querySelectorAll(".verso-chart-yl").forEach(function (el, i) {
        var frac = [1, 0.75, 0.5, 0.25][i];
        if (frac === undefined) return;
        if (el.dataset.unit === undefined) {
          el.dataset.unit = (el.textContent.match(/[\d.]+\s*(.*)$/) || ["", ""])[1];
        }
        el.textContent = axisValue(max * frac) + (el.dataset.unit ? " " + el.dataset.unit : "");
      });
    }

    // Clip each series to the plot rect so the treadmill's off-edge content
    // hides; each graph its own clip, since a drawer's sits beside the page's.
    var clipID = "verso-live-clip-" + (++clips);
    var clip = document.createElementNS(NS, "clipPath");
    clip.setAttribute("id", clipID);
    var rect = document.createElementNS(NS, "rect");
    rect.setAttribute("x", "0"); rect.setAttribute("y", "0");
    rect.setAttribute("width", String(W)); rect.setAttribute("height", String(H));
    clip.appendChild(rect); svg.appendChild(clip);

    var lines = [], areas = [];
    groups.forEach(function (g) {
      g.setAttribute("clip-path", "url(#" + clipID + ")");
      lines.push(g.querySelector(".verso-chart-line"));
      areas.push(g.querySelector(".verso-chart-area"));
    });

    function y(v) { return H - PAD - (v / max) * (H - 2 * PAD); }
    function points(vals) { return vals.map(function (v, j) { return [j * step, y(v)]; }); }
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
    function draw() {
      updateScale();
      series.forEach(function (vals, idx) {
        var p = points(vals), head = "M" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p);
        if (lines[idx]) { lines[idx].setAttribute("d", head); lines[idx].style.transform = "none"; }
        if (areas[idx]) { areas[idx].setAttribute("d", "M0 " + H.toFixed(1) + " L" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p) + " L" + p[N][0].toFixed(1) + " " + H.toFixed(1) + " Z"); areas[idx].style.transform = "none"; }
      });
    }

    // The notebook grid travels with the curve: the section's 20px grid is a
    // background, and each frame shifts it by exactly the distance the curve has
    // slid, so the data and the paper under it move as one. The chart's rendered
    // width is cached (and refreshed on resize) so a frame never reads layout.
    var gridEl = host.closest("section");
    var gridBase = 0;
    var svgW = svg.getBoundingClientRect().width;
    if (window.ResizeObserver) new ResizeObserver(function () { svgW = svg.getBoundingClientRect().width; }).observe(svg);
    function stepPx() { return (svgW / W) * step; }
    function moveGrid(progress) {
      if (!gridEl) return;
      var off = (gridBase + progress * stepPx()) % GRID;
      gridEl.style.backgroundPositionX = (-1 - off).toFixed(2) + "px";
    }

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
      lines.forEach(function (l) { if (l) l.style.transform = tf; });
      areas.forEach(function (a) { if (a) a.style.transform = tf; });
      moveGrid(progress);
      if (progress < 1) requestAnimationFrame(frame);
      else gliding = false;
    }

    draw();
    host.__versoLive = {
      // A fresh sample: commit the newest down/up, drop the oldest, redraw, and
      // start the glide toward it if one is not already running.
      sample: function (nd, nu) {
        if (typeof nd !== "number" || typeof nu !== "number") return;
        // The curve drops its oldest point and snaps back to zero; the grid keeps
        // the step it already travelled, so the two stay in register.
        if (lastCommit !== null) gridBase = (gridBase + stepPx()) % GRID;
        down.push(nd); down.shift();
        up.push(nu); up.shift();
        draw();
        if (downEl) downEl.textContent = (Math.round(nd * 10) / 10).toFixed(1);
        if (upEl) upEl.textContent = (Math.round(nu * 10) / 10).toFixed(1);
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
