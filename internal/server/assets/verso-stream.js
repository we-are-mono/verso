// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-stream.js — the overview's live numbers (ADR-004). One EventSource the
// shell pushes fresh truth into. Each event updates a rendered widget in place;
// the CSS transitions do the glides.
//
// The stream hands each WAN sample to the Internet graph by its name, "wan",
// through window.versoLiveSample (verso-chart.js), which draws the curve.

// The overview stream: one EventSource the shell pushes fresh truth into —
// system meters, interface rates and WAN traffic. Each type updates
// its rendered widget in place (the CSS transitions do the
// glides). Reconnection after a drop is EventSource's own; when the session
// ends the reconnect lands on the login redirect — not an event stream —
// which closes the client for good.
(function () {
  if (!window.EventSource) return;
  if (!document.querySelector("[data-overview]") && !document.querySelector("[data-verso-meter]") && !document.querySelector("[data-verso-row]") && !document.querySelector('[data-verso-live-chart="wan"]') && !document.querySelector("[data-verso-prop]") && !document.querySelector("[data-verso-port]")) return;
  // A page whose only live part is the router's panel (System › Hardware)
  // reads the interfaces and nothing else of the Overview's stream.
  var panelOnly = !document.querySelector("[data-overview]") && !document.querySelector("[data-verso-meter]") && !document.querySelector("[data-verso-row]") && !document.querySelector('[data-verso-live-chart="wan"]') && !document.querySelector("[data-verso-prop]");
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

  // applyPort relights a port on a router's panel: its link LED, its traffic
  // LED's activity level (the class keeps its blink running when the level
  // holds), its speed, the label a keyboard reaches it by, and its detail's
  // link line. The link words come in the reader's language from the panel.
  function applyPort(port, iface) {
    port.classList.toggle("is-linked", !!iface.linked);
    var level = "act-" + (iface.activity || 0);
    if (!port.classList.contains(level)) {
      for (var i = 0; i <= 4; i++) port.classList.remove("act-" + i);
      port.classList.add(level);
    }
    var speed = port.querySelector("[data-verso-port-speed]");
    if (speed) speed.textContent = iface.speed;
    var panel = port.closest(".verso-panel-art");
    if (!panel) return;
    var word = panel.getAttribute(iface.linked ? "data-verso-word-on" : "data-verso-word-off") || "";
    var tip = panel.querySelector('[data-verso-port-tip="' + CSS.escape(iface.name) + '"]');
    var name = tip && tip.querySelector("b") ? tip.querySelector("b").textContent : iface.name;
    // The WAN port keeps its role in its label: the word the shell wrote over it.
    var role = port.querySelector(".verso-port-role");
    if (role && role.textContent) name += ", " + role.textContent;
    var label = name + ", " + word + (iface.linked && iface.speed && iface.speed !== "—" ? ", " + iface.speed : "");
    if (port.getAttribute("aria-label") !== label) port.setAttribute("aria-label", label);
    if (tip) setText(tip, "[data-verso-port-link]", iface.linked ? iface.speed : word);
  }
  function applyInterface(iface) {
    document.querySelectorAll('[data-verso-port="' + CSS.escape(iface.name) + '"]').forEach(function (port) {
      applyPort(port, iface);
    });
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
  function connect() {
    if (es) return;
    es = new EventSource(panelOnly ? "/overview/events?reads=interfaces" : "/overview/events");
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

  // A stretch of time the stream sends written (internal/units). It is
  // redrawn only when it reads differently, so a minute's uptime is one
  // change, not sixty.
  function writeSpan(el, words) {
    if (el && words && el.textContent !== words) el.textContent = words;
  }

  function wire(es) {
  listen(es, "meters", "meters", applyMeter);
  es.addEventListener("clock", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    if (!d) return;
    // The System card's local time and uptime rows.
    var time = document.querySelector('[data-verso-prop="local-time"]');
    if (time && d.time && time.textContent !== d.time) time.textContent = d.time;
    writeSpan(document.querySelector('[data-verso-prop="uptime"]'), d.uptime);
  });
  // The verdict and every card's header (ADR-022): the words, and the tone the
  // card wears as data-tone, which its status and packet take their colour
  // from. A hidden card's tone goes to its chip on the Arrange shelf.
  es.addEventListener("overview", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    var root = document.querySelector("[data-overview]");
    if (!root || !d) return;
    root.dataset.overviewTone = d.tone;
    setText(root, "[data-overview-lead]", d.lead);
    setText(root, "[data-overview-accent]", d.accent);
    setText(root, "[data-overview-tail]", d.tail);
    (d.cards || []).forEach(function (card) {
      var chip = root.querySelector('[data-overview-restore="' + CSS.escape(card.id) + '"]');
      if (chip) chip.dataset.tone = card.tone;
      var el = root.querySelector('[data-overview-card="' + CSS.escape(card.id) + '"]');
      if (!el) return;
      el.dataset.tone = card.tone;
      setText(el, "[data-overview-card-summary]", card.summary);
      var status = el.querySelector("[data-overview-card-status]");
      if (!status) return;
      status.hidden = !card.status;
      setText(status, "[data-overview-card-word]", card.status);
    });
  });
  listen(es, "interfaces", "interfaces", applyInterface);
  // The WAN throughput sample hands off to the Internet graph, if present, and
  // the Internet card's row says how long the uplink has been connected.
  es.addEventListener("wan", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    if (d && window.versoLiveSample) window.versoLiveSample("wan", d.down, d.up);
    if (d) writeSpan(document.querySelector('[data-verso-prop="wan-uptime"]'), d.connected);
  });
  // The Hardware card's readings, each row written in place by its key.
  es.addEventListener("sensors", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    if (!d || !d.rows) return;
    Object.keys(d.rows).forEach(function (key) {
      var el = document.querySelector('[data-verso-prop="' + CSS.escape(key) + '"]');
      if (el && el.textContent !== d.rows[key]) el.textContent = d.rows[key];
    });
  });
  }
})();
