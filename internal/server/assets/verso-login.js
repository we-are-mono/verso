// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// The public sign-in page keeps native form submission. JavaScript supplies
// only the act's busy face — the shared one (verso-buttons.js), as every
// other act's — and the small public status refresh, written into the card's
// live facts by their keys; it holds no credentials and never needs the
// authenticated shell's client runtime.
(function () {
  "use strict";
  var root = document.querySelector("[data-login]");
  if (!root) return;
  var form = root.querySelector("form");
  var button = form && form.querySelector('button[type="submit"]');
  if (form) {
    form.addEventListener("submit", function (event) {
      versoButtons.submit(event, root.dataset.busyLabel);
    });
  }

  var clock = root.querySelector('[data-verso-prop="clock"]');
  var uptime = root.querySelector('[data-verso-prop="uptime"]');
  var internet = root.querySelector('[data-verso-prop="internet"]');
  var mark = root.querySelector('[data-verso-prop-dot="internet"]');
  if (!clock || !uptime || !internet || !mark) return;
  var unix = Number(root.dataset.unix);
  var offset = Number(root.dataset.offset);
  var zone = root.dataset.zone;
  var received = performance.now();
  var lastRefresh = received;
  var refreshing = false;

  // The uplink's words and its packet's tone: online, offline, or unknown —
  // the hollow packet — when the router cannot say (blocks/mark.css).
  var states = {
    up: { tone: "success", words: root.dataset.onlineLabel },
    down: { tone: "danger", words: root.dataset.offlineLabel },
    unknown: { tone: "unknown", words: root.dataset.unknownLabel },
  };
  function setInternet(value) {
    var state = states[value === true ? "up" : value === false ? "down" : "unknown"];
    internet.textContent = state.words;
    mark.setAttribute("data-verso-tone", state.tone);
  }

  function tick() {
    // Offset and epoch come from the router. UTC getters avoid substituting
    // the browser's timezone, including when the two sides cross midnight.
    var date = new Date((unix + offset) * 1000 + performance.now() - received);
    clock.textContent = [date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds()]
      .map(function (n) { return String(n).padStart(2, "0"); }).join(":") + " " + zone;
  }

  function signingIn() {
    return Boolean(button) && versoButtons.has(button);
  }

  async function refresh() {
    if (refreshing || document.hidden || signingIn()) return;
    refreshing = true;
    var controller = new AbortController();
    var timeout = window.setTimeout(function () { controller.abort(); }, 5000);
    try {
      var response = await fetch("/login/status", {
        credentials: "same-origin", cache: "no-store", signal: controller.signal,
        headers: { "Accept": "application/json", "Accept-Language": document.documentElement.lang },
      });
      if (!response.ok) throw new Error("status unavailable");
      var status = await response.json();
      if (!Number.isFinite(status.unix) || !Number.isFinite(status.offset)) throw new Error("invalid clock");
      unix = status.unix;
      offset = status.offset;
      zone = status.zone;
      received = performance.now();
      uptime.textContent = status.uptime;
      setInternet(status.internet);
      tick();
    } catch (_) {
      setInternet(null);
    } finally {
      window.clearTimeout(timeout);
      refreshing = false;
      lastRefresh = performance.now();
    }
  }

  window.setInterval(function () {
    if (document.hidden) return;
    tick();
    if (performance.now() - lastRefresh >= 15000) refresh();
  }, 1000);
  document.addEventListener("visibilitychange", function () {
    if (!document.hidden) refresh();
  });
})();
