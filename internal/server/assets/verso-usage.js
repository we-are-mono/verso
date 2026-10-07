// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-usage.js — the Devices roster's usage, live (ADR-018). One EventSource
// the shell pushes each device's rate into every second, and its month every
// half minute. The server drew every figure's slot, hidden when empty, so a
// frame only sets text, a meter's clip and which of the slots and the dash
// show; nothing here builds markup.
(function () {
  "use strict";
  if (!window.EventSource || !document.querySelector('[data-verso-cell="usage-now"]')) return;

  // The meter's clip, as the server draws it (widget figureClip): the bar
  // spans the slot and the clip shows the share, square-ended.
  function clip(fill) {
    return "inset(0 " + (100 - Math.min(100, Math.max(0, fill || 0))) + "% 0 0)";
  }
  // A figure in no series (a month) has no meter, so only its text moves.
  function setFigure(cell, role, text, fill) {
    var slot = cell.querySelector('[data-verso-figure="' + role + '"]');
    if (!slot) return false;
    slot.querySelector("[data-verso-figure-text]").textContent = text || "";
    var meter = slot.querySelector("[data-verso-figure-meter]");
    if (meter) meter.style.clipPath = clip(fill);
    slot.hidden = !text;
    return !!text;
  }
  function setDash(cell, shown) {
    var none = cell.querySelector("[data-verso-figures-none]");
    if (none) none.hidden = shown;
  }

  function rowsByMAC() {
    var rows = {};
    document.querySelectorAll("tr[data-verso-row-id]").forEach(function (row) {
      rows[row.getAttribute("data-verso-row-id").toLowerCase()] = row;
    });
    return rows;
  }
  // A device's panel open over the roster shows its rate as two figures,
  // named by the device; an idle device reads as nothing moving.
  function setStat(name, text) {
    var stat = document.querySelector('[data-verso-stat="' + CSS.escape(name) + '"] [data-verso-stat-v]');
    if (stat) stat.textContent = text;
  }
  function apply(frame, withMonth) {
    var rows = rowsByMAC();
    frame.forEach(function (d) {
      setStat("usage-down:" + d.mac, d.busy ? d.down : "0");
      setStat("usage-up:" + d.mac, d.busy ? d.up : "0");
      var row = rows[d.mac];
      if (!row) return;
      var now = row.querySelector('[data-verso-cell="usage-now"]');
      if (now) {
        var down = setFigure(now, "emerald", d.busy ? d.down : "", d.down_fill);
        var up = setFigure(now, "violet", d.busy ? d.up : "", d.up_fill);
        setDash(now, down || up);
      }
      var month = withMonth && row.querySelector('[data-verso-cell="usage-month"]');
      if (month) setDash(month, setFigure(month, "", d.month, 0));
    });
  }

  // Open only while the page is seen: every tick reads the router's conntrack
  // table, which a background tab would pay for nobody. Shown again, the first
  // frame carries the month too, so the page is current the moment it is seen.
  var es = null;
  function connect() {
    if (es) return;
    es = new EventSource("/devices/usage");
    es.addEventListener("usage", function (e) {
      var data;
      try {
        data = JSON.parse(e.data);
      } catch (err) {
        return;
      }
      if (data && data.devices) apply(data.devices, data.month);
    });
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
})();
