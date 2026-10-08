// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-stream.js — the overview's live numbers (ADR-004). One EventSource the
// shell pushes fresh truth into. Each event updates a rendered widget in place;
// the CSS transitions do the glides.
//
// The stream hands each WAN sample to the Internet graph by its name, "wan",
// through window.versoLiveSample (verso-chart.js), which draws the curve.

// The overview stream: one EventSource the shell pushes fresh truth into —
// system meters, interface rates, WAN traffic, and sensors. Each type updates
// its rendered widget in place (the CSS transitions do the
// glides). Reconnection after a drop is EventSource's own; when the session
// ends the reconnect lands on the login redirect — not an event stream —
// which closes the client for good.
(function () {
  if (!window.EventSource) return;
  if (!document.querySelector("[data-verso-meter]") && !document.querySelector("[data-verso-row]") && !document.querySelector('[data-verso-live-chart="wan"]') && !document.querySelector("[data-verso-prop]")) return;
  // The meter's bar tints by its health band (the tone vocabulary), in the classes
  // meter.html.tmpl renders — the live layer and the first paint have to name the
  // same colour or a reading changes shade the moment it updates, which it did:
  // these were stock Tailwind a step darker than the template's own stock Tailwind,
  // and neither was a colour this app's palette has.
  var BAR_BANDS = {
    success: "bg-green",
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
        bar.classList.add(BAR_BANDS[reading.band] || BAR_BANDS.success);
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
  // The WAN throughput sample hands off to the Internet graph, if present.
  es.addEventListener("wan", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    var uptime = document.querySelector('[data-verso-tile-caption="internet-uptime"]');
    if (uptime && d && typeof d.uptime_label === "string") uptime.textContent = d.uptime_label;
    if (d && window.versoLiveSample) window.versoLiveSample("wan", d.down, d.up);
  });
  // The hardware-sensor frame updates the System panel's temperature/fan/power
  // rows in place, including N/A when a reading disappears. The temperature
  // dot returns to its hollow neutral state when no reading is available.
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
    var dot = document.querySelector('[data-verso-prop-dot="temperature"]');
    if (dot && d.tempLevel) {
      dot.className = "mr-2.5 size-1.5 shrink-0 rounded-[1px] " + (d.tempLevel === "neutral" ? "border border-faint" : TONE_DOT(d.tempLevel));
    }
  });
  }
})();
