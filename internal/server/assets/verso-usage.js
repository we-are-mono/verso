// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-usage.js — the Devices roster's usage, live (ADR-018). One EventSource
// the shell pushes each device's rate into every second, and its month every
// half minute. The server drew every figure's slot, hidden when empty, so a
// frame only sets text, the Now cell's ink and which of the slots and the
// dash show; nothing here builds markup.
(function () {
  "use strict";
  if (!window.EventSource || !document.querySelector('[data-verso-cell="usage-now"]')) return;

  function setFigure(cell, label, text) {
    var slot = cell.querySelector('[data-verso-figure="' + label + '"]');
    if (!slot) return false;
    slot.querySelector("[data-verso-figure-text]").textContent = text || "";
    // Unseen, not removed: an empty slot keeps its width, so the column holds
    // still as devices go busy and idle.
    slot.classList.toggle("invisible", !text);
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
  function apply(frame, withMonth) {
    var rows = rowsByMAC();
    frame.forEach(function (d) {
      // A device's panel open over the roster draws its traffic graph, which
      // takes each rate by the device's name (verso-chart.js).
      if (window.versoLiveSample) window.versoLiveSample("usage:" + d.mac, d.down_mbps, d.up_mbps);
      var row = rows[d.mac];
      if (!row) return;
      var now = row.querySelector('[data-verso-cell="usage-now"]');
      if (now) {
        var down = setFigure(now, "down", d.busy ? d.down : "");
        var up = setFigure(now, "up", d.busy ? d.up : "");
        setDash(now, down || up);
        // The device's load is the figures' ink; the stylesheet eases it from
        // one tone to the next, so only a change of load sets it.
        var ink = d.busy ? d.ink || "" : "";
        var was = now.getAttribute("data-verso-ink");
        if (was !== ink) {
          // A device waking from idle arrives already in its ink: figures
          // appearing while they shade from Ink would read as a flicker.
          if (!was) now.style.transition = "none";
          now.setAttribute("data-verso-ink", ink);
          if (!was) {
            void getComputedStyle(now).color;
            now.style.transition = "";
          }
        }
      }
      var month = withMonth && row.querySelector('[data-verso-cell="usage-month"]');
      if (month) setDash(month, setFigure(month, "", d.month));
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
