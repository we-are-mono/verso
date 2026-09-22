// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-stream.js — the overview's live numbers (ADR-004). One EventSource the
// shell pushes fresh truth into, and the traffic curve that flows from it. Each
// event updates a rendered widget in place; the CSS transitions do the glides.
//
// The stream hands each WAN sample to the graph through window.__versoWanSample,
// which the graph installs when it finds a canvas to draw in and nothing calls
// when there is none.

// The overview stream: one EventSource the shell pushes fresh truth into —
// system meters, interface rates, WAN traffic, and sensors. Each type updates
// its rendered widget in place (the CSS transitions do the
// glides). Reconnection after a drop is EventSource's own; when the session
// ends the reconnect lands on the login redirect — not an event stream —
// which closes the client for good.
(function () {
  if (!window.EventSource) return;
  if (!document.querySelector("[data-verso-meter]") && !document.querySelector("[data-verso-row]") && !document.querySelector("[data-verso-chart]") && !document.querySelector("[data-verso-traffic-chart]") && !document.querySelector("[data-verso-prop]")) return;
  // The meter's bar tints by its health band (the tone vocabulary), in the classes
  // meter.html.tmpl renders — the live layer and the first paint have to name the
  // same colour or a reading changes shade the moment it updates, which it did:
  // these were stock Tailwind a step darker than the template's own stock Tailwind,
  // and neither was a colour this app's palette has.
  var BAR_BANDS = {
    success: "bg-denim",
    warning: "bg-marigold",
    danger: "bg-crimson",
    info: "bg-denim",
  };
  function setText(root, selector, text) {
    var el = root.querySelector(selector);
    if (el) el.textContent = text;
  }
  function applyMeter(reading) {
    var root = document.querySelector('[data-verso-meter="' + CSS.escape(reading.name) + '"]');
    if (!root) return;
    setText(root, "[data-verso-meter-value]", reading.value);
    setText(root, "[data-verso-meter-unit]", reading.unit);
    setText(root, "[data-verso-meter-detail]", reading.detail);
    var fill = Math.min(100, Math.max(0, reading.fill));
    var bar = root.querySelector("[data-verso-meter-bar]");
    if (bar) {
      // The same clip the server draws (widget.MeterClip): the bar spans the
      // track and the clip shows the reading.
      bar.style.clipPath = "inset(0 " + (100 - fill) + "% 0 0 round 9999px)";
      // A role-accented bar keeps its fixed colour (a dashboard hue, not a health
      // band); only a band-coloured bar recolours with its reading.
      if (!reading.role) {
        for (var bb in BAR_BANDS) bar.classList.remove(BAR_BANDS[bb]);
        bar.classList.remove("bg-green");
        bar.classList.add(root.hasAttribute("data-overview-meter") && reading.band === "success" ? "bg-green" : (BAR_BANDS[reading.band] || BAR_BANDS.success));
      }
    }
    root.setAttribute("aria-label", (reading.label + " " + reading.value + " " + reading.unit).trim());
  }
  // The status dot's colour for a tone, which is verso-tone-dot in shared.html.tmpl
  // — the one place in the app that decides what a tone looks like as a dot. Success
  // pulses, because it marks something live. A reading repainted here has to come
  // out identical to the one the server rendered a moment earlier, and it did not:
  // it was three stock Tailwind colours the palette does not contain.
  function TONE_DOT(tone) {
    if (tone === "success") return "verso-live-dot bg-green";
    if (tone === "warning") return "bg-marigold";
    if (tone === "danger") return "bg-crimson";
    if (tone === "info") return "bg-denim";
    return "bg-glyph";
  }

  // The state dot a row carries, repainted. It is the dot the server rendered
  // (table.html.tmpl): a fill in the palette for a state worth a colour, a hollow
  // ring for one that is not. Both the fills and the ring have to come off before
  // one goes on — this used to add a class beside whatever the render had left
  // there, which is two fills arguing over the cascade, and it named colours from
  // stock Tailwind that the render never uses. A live interface going down could
  // therefore stay green.
  var DOT_FILL = { success: "bg-green", warning: "bg-marigold", danger: "bg-crimson" };
  var DOT_RING = ["border-[1.5px]", "border-faint"];
  function paintDot(dot, variant) {
    for (var tone in DOT_FILL) dot.classList.remove(DOT_FILL[tone]);
    DOT_RING.forEach(function (c) { dot.classList.remove(c); });
    if (DOT_FILL[variant]) dot.classList.add(DOT_FILL[variant]);
    else DOT_RING.forEach(function (c) { dot.classList.add(c); });
  }

  function applyInterface(iface) {
    var tunnel = document.querySelector('[data-overview-tunnel="' + CSS.escape(iface.name) + '"]');
    if (tunnel) {
      setText(tunnel, "[data-overview-tunnel-label]", iface.state);
      var state = tunnel.querySelector("[data-overview-tunnel-state]");
      if (state) state.dataset.tone = iface.operstate === "up" ? "success" : (iface.operstate === "down" || iface.operstate === "lowerlayerdown" ? "danger" : "neutral");
    }
    var rows = document.querySelectorAll("[data-verso-row]");
    var root = null, key = "interface:" + iface.name;
    rows.forEach(function (row) {
      if (row.getAttribute("data-verso-row") === key) root = row;
    });
    if (!root) return;
    var state = root.querySelector('[data-verso-cell="state"]');
    if (state) {
      setText(state, "[data-verso-value]", iface.state);
      var dot = state.querySelector("[data-verso-dot]");
      if (dot) paintDot(dot, iface.variant);
    }
    setText(root, '[data-verso-cell="rx-rate"]', iface.rx_rate);
    setText(root, '[data-verso-cell="tx-rate"]', iface.tx_rate);
  }
  // applyChart redraws a named chart from fresh series — the same geometry
  // the server drew (viewBox coordinates from the svg itself), so the live
  // layer and the first paint never disagree. Series pair with the rendered
  // groups by order; the role hook lets an idle-grey chart take its colours
  // when traffic starts.
  var CHART_ROLES = ["sky", "violet", "emerald", "amber", "idle"];
  function applyChart(dev) {
    var svg = document.querySelector('svg[data-verso-chart="' + CSS.escape(dev.key) + '"]');
    if (!svg) return;
    var vb = svg.viewBox.baseVal;
    var W = vb.width, H = vb.height, PAD = 8;
    var series = [dev.down || [], dev.up || []];
    var max = 0;
    series.forEach(function (vals) {
      vals.forEach(function (v) {
        if (v > max) max = v;
      });
    });
    var active = max > 0.01;
    max *= 1.15;
    if (max <= 0) max = 1;
    var y = function (v) {
      return (H - PAD - (v / max) * (H - 2 * PAD)).toFixed(1);
    };
    svg.querySelectorAll("g.verso-chart-series").forEach(function (g, i) {
      var vals = series[i];
      if (!vals || vals.length < 2) return;
      var pts = vals.map(function (v, j) {
        return ((j / (vals.length - 1)) * W).toFixed(1) + " " + y(v);
      });
      var line = g.querySelector(".verso-chart-line");
      if (line) line.setAttribute("d", "M" + pts.join(" L"));
      var area = g.querySelector(".verso-chart-area");
      if (area) area.setAttribute("d", "M0 " + H + " L" + pts.join(" L") + " L" + W + " " + H + " Z");
      var dot = g.querySelector(".verso-chart-dot");
      if (dot) dot.setAttribute("cy", y(vals[vals.length - 1]));
      var role = g.getAttribute("data-verso-chart-role") || "sky";
      CHART_ROLES.forEach(function (r) {
        g.classList.remove("verso-chart--" + r);
      });
      g.classList.add("verso-chart--" + (active ? role : "idle"));
    });
    // The value labels ride their gridlines: quarters of the new range, the
    // unit staying on the topmost (captured from the rendered text once).
    var fmt = function (v) {
      return v >= 10 ? Math.round(v).toString() : (Math.round(v * 10) / 10).toString();
    };
    var labels = svg.parentElement.querySelectorAll(".verso-chart-yl");
    labels.forEach(function (el, i) {
      var frac = [1, 0.75, 0.5, 0.25][i];
      if (frac === undefined) return;
      if (el.dataset.unit === undefined) {
        el.dataset.unit = (el.textContent.match(/[\d.]+\s*(.*)$/) || ["", ""])[1];
      }
      var v = max * frac;
      el.textContent = el.dataset.unit ? fmt(v) + " " + el.dataset.unit : fmt(v);
    });
    // The readout above the plot shows each series' newest value — whole
    // numbers only, the readout stays calm.
    var block = document.querySelector('[data-verso-chart-block="' + CSS.escape(dev.key) + '"]');
    if (block) {
      block.querySelectorAll("[data-verso-chart-rate-v]").forEach(function (el, i) {
        var vals = series[i];
        if (vals && vals.length) el.textContent = String(Math.round(vals[vals.length - 1]));
      });
    }
    // The running totals on the device's stat tiles.
    setStat(dev.key + ":down", fmtBytes(dev.rx));
    setStat(dev.key + ":up", fmtBytes(dev.tx));
    setStat(dev.key + ":conns", [String(dev.conns || 0), ""]);
  }
  // fmtBytes mirrors the server's byte formatting: whole bytes, then one
  // decimal per binary step.
  function fmtBytes(n) {
    n = n || 0;
    if (n < 1024) return [String(n), "B"];
    var units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
    var i = -1;
    do {
      n /= 1024;
      i++;
    } while (n >= 1024 && i < units.length - 1);
    return [n.toFixed(1), units[i]];
  }
  function setStat(name, parts) {
    var tile = document.querySelector('[data-verso-stat="' + CSS.escape(name) + '"]');
    if (!tile) return;
    var v = tile.querySelector("[data-verso-stat-v]");
    if (v) v.textContent = parts[0];
    var u = tile.querySelector("[data-verso-stat-u]");
    if (u) u.textContent = parts[1];
  }
  function listen(es, type, key, apply) {
    es.addEventListener(type, function (e) {
      var data;
      try {
        data = JSON.parse(e.data);
      } catch (err) {
        return;
      }
      if (data && data[key]) data[key].forEach(apply);
    });
  }
  // The stream is open only while the page is on screen. Every connection costs
  // the router its own sampling each second (system info and WAN status over
  // ubus, the sensors), and a tab left in the background would pay that for
  // nobody; hidden, it lets go, and shown again it reconnects, where the first
  // frame is a full snapshot, so the page is current the moment it is seen.
  var es = null;
  var live = document.querySelector("[data-overview-live]");
  var pulse = null;
  if (live && !window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
    pulse = live.animate([{opacity:1},{opacity:0.28},{opacity:1}], {duration:2600,iterations:Infinity});
  }
  function connect() {
    if (es) return;
    es = new EventSource("/overview/events");
    wire(es);
  }
  function disconnect() {
    if (!es) return;
    es.close();
    es = null;
  }
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) disconnect();
    else connect();
  });
  if (!document.hidden) connect();

  function wire(es) {
  listen(es, "meters", "meters", applyMeter);
  es.addEventListener("clock", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    var root = document.querySelector("[data-overview-clock]");
    if (!root || !d) return;
    setText(root, "[data-overview-time]", d.clock);
    setText(root, "[data-overview-uptime]", d.uptime);
  });
  es.addEventListener("overview", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    var root = document.querySelector("[data-overview]");
    if (!root || !d) return;
    var summary = root.querySelector("[data-overview-tone]");
    if (summary) summary.dataset.overviewTone = d.tone;
    setText(root, "[data-overview-kicker]", d.kicker);
    setText(root, "[data-overview-lead]", d.lead);
    setText(root, "[data-overview-accent]", d.accent);
    setText(root, "[data-overview-tail]", d.tail);
    (d.tiles || []).forEach(function (tile) {
      var el = root.querySelector('[data-overview-tile="' + CSS.escape(tile.id) + '"]');
      if (!el) return;
      el.dataset.tone = tile.tone;
      setText(el, "[data-overview-tile-status]", tile.status);
      setText(el, "[data-overview-tile-caption]", tile.caption);
      setText(el, "[data-overview-tile-identity]", tile.identity);
      setText(el, "[data-overview-tile-separator]", tile.caption && tile.identity ? " · " : "");
      var caption = el.querySelector("[data-overview-tile-caption]");
      if (caption) caption.parentElement.title = [tile.caption,tile.identity].filter(Boolean).join(" · ");
    });
  });
  if (live) {
    es.addEventListener("open", function () { live.classList.remove("bg-faint"); live.classList.add("bg-green"); if (pulse) pulse.play(); });
    es.addEventListener("error", function () { live.classList.remove("bg-green"); live.classList.add("bg-faint"); if (pulse) pulse.cancel(); });
  }
  listen(es, "interfaces", "interfaces", applyInterface);
  listen(es, "traffic", "devices", applyChart);
  // The WAN throughput sample hands off to the traffic-graph animator, if present.
  es.addEventListener("wan", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    var uptime = document.querySelector('[data-verso-tile-caption="internet-uptime"]');
    if (uptime && d && typeof d.uptime_label === "string") uptime.textContent = d.uptime_label;
    if (d && window.__versoWanSample) window.__versoWanSample(d.down, d.up);
  });
  // The hardware-sensor frame updates the System panel's temperature/fan/power
  // rows in place; an absent reading (a row hidden at load) has no element to
  // find, so it is silently skipped. The temperature dot recolours to match.
  function setSensor(name, val) {
    if (val == null) return;
    var el = document.querySelector('[data-verso-prop="' + name + '"]');
    if (!el) return;
    var note = name === "temperature" && document.querySelector("[data-overview-temp-note]");
    if (note) {
      var parts = val.split(" · ");
      el.textContent = parts.shift();
      note.textContent = parts.length ? "· " + parts.join(" · ") : "";
    } else el.textContent = val;
  }
  es.addEventListener("sensors", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    if (!d) return;
    setSensor("temperature", d.temperature);
    setSensor("fan", d.fan);
    setSensor("power", d.power);
    setSensor("summary", d.summary);
    var dot = document.querySelector('[data-verso-prop-dot="temperature"]');
    if (dot && d.tempLevel) {
      dot.className = "mr-2.5 size-1.5 shrink-0 rounded-[1px] " + TONE_DOT(d.tempLevel);
    }
  });
  }
})();

// The overview's Internet-traffic graph, live: it seeds from the real WAN minute
// the server rendered (data-verso-traffic-seed) and then FLOWS left continuously
// as fresh samples arrive on the overview stream's `wan` event (window.__verso-
// WanSample). Each frame the curves translate by a fraction of one sample; when a
// sample lands the value is committed and the translate resets — a seamless
// treadmill drawing the same smooth curve and dynamic shared scale the server
// drew. A clip hides what scrolls past the edges. Reduced-motion still updates,
// just in discrete steps instead of a glide.
(function () {
  var host = document.querySelector("[data-verso-traffic-chart]");
  if (!host) return;
  var svg = host.querySelector("svg.verso-chart");
  if (!svg) return;
  var reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  var NS = "http://www.w3.org/2000/svg";
  var vb = svg.viewBox.baseVal;
  var W = vb.width, H = vb.height, PAD = svg.hasAttribute("data-chart-padding") ? Number(svg.getAttribute("data-chart-padding")) : 8, INTERVAL = 1000;
  var groups = svg.querySelectorAll("g.verso-chart-series");
  var dots = host.querySelectorAll(".verso-chart-dot-html");
  var downEl = document.querySelector("[data-verso-traffic-down]");
  var upEl = document.querySelector("[data-verso-traffic-up]");

  // Seed the live series from the server's rendered minute.
  var seed = { down: [], up: [] };
  try { seed = JSON.parse(host.getAttribute("data-verso-traffic-seed") || "{}"); } catch (e) { seed = {}; }
  var down = (seed.down || []).slice(), up = (seed.up || []).slice();
  var N = Math.min(down.length, up.length); // visible points fill 0..W
  if (N < 2) return;
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

  // Clip each series to the plot rect so the treadmill's off-edge content hides.
  var clip = document.createElementNS(NS, "clipPath");
  clip.setAttribute("id", "verso-traffic-clip");
  var rect = document.createElementNS(NS, "rect");
  rect.setAttribute("x", "0"); rect.setAttribute("y", "0");
  rect.setAttribute("width", String(W)); rect.setAttribute("height", String(H));
  clip.appendChild(rect); svg.appendChild(clip);

  var lines = [], areas = [];
  groups.forEach(function (g) {
    g.setAttribute("clip-path", "url(#verso-traffic-clip)");
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
  // The live dots ride the curve's right edge. They move by transform against
  // the plot's height, measured once and again only when the chart resizes, so
  // a frame of the glide moves them without laying the page out; top, which the
  // server's first paint uses, is pinned to the plot's top edge from here on.
  var plotH = 0;
  function measure() {
    var parent = dots[0] && dots[0].offsetParent;
    plotH = parent ? parent.clientHeight : 0;
  }
  measure();
  if (window.ResizeObserver && dots[0] && dots[0].offsetParent) {
    new ResizeObserver(function () { measure(); }).observe(dots[0].offsetParent);
  }
  function placeDot(dot, v) {
    dot.style.top = "0";
    dot.style.transform = "translate(50%, -50%) translateY(" + ((y(v) / H) * plotH).toFixed(1) + "px)";
  }
  function draw() {
    updateScale();
    series.forEach(function (vals, idx) {
      var p = points(vals), head = "M" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p);
      if (lines[idx]) { lines[idx].setAttribute("d", head); lines[idx].style.transform = "none"; }
      if (areas[idx]) { areas[idx].setAttribute("d", "M0 " + H.toFixed(1) + " L" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p) + " L" + p[N][0].toFixed(1) + " " + H.toFixed(1) + " Z"); areas[idx].style.transform = "none"; }
      if (dots[idx]) placeDot(dots[idx], vals[N - 1]);
    });
  }

  // The notebook grid travels with the curve: the section's 20px grid is a
  // background, and each frame shifts it by exactly the distance the curve has
  // slid, so the data and the paper under it move as one. The chart's rendered
  // width is cached (and refreshed on resize) so a frame never reads layout.
  var gridEl = host.closest("section");
  var GRID = 20;
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
  // A fresh WAN sample: commit the newest down/up, drop the oldest, redraw, and
  // start the glide toward it if one is not already running.
  window.__versoWanSample = function (nd, nu) {
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
  };

  draw();
  if (reduce) return; // discrete live updates only; no glide
  // One glide per sample: the curves slide one step left over the sample
  // interval, and once they have arrived the loop stops until the next sample
  // restarts it. A stream that has stopped (the tab hidden, the router gone)
  // leaves nothing running.
  function frame(ts) {
    var progress = Math.min(1, (ts - lastCommit) / INTERVAL);
    // A CSS transform, not the SVG attribute: the attribute re-lays the chart
    // out on every frame, the style only repaints it. In the SVG's own user
    // units, which is what px means inside a viewBox.
    var tf = "translateX(" + (-progress * step).toFixed(2) + "px)";
    lines.forEach(function (l) { if (l) l.style.transform = tf; });
    areas.forEach(function (a) { if (a) a.style.transform = tf; });
    moveGrid(progress);
    series.forEach(function (vals, idx) {
      if (dots[idx]) placeDot(dots[idx], vals[N - 1] * (1 - progress) + vals[N] * progress);
    });
    if (progress < 1) requestAnimationFrame(frame);
    else gliding = false;
  }
})();
