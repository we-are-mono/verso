// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// The public sign-in page keeps native form submission. JavaScript supplies
// only the waiting mark and the small public status refresh; it holds no
// credentials and never needs the authenticated shell's client runtime.
(function () {
  "use strict";
  var form = document.querySelector("[data-login-form]");
  var panel = document.querySelector("[data-login-status]");
  if (!form || !panel) return;

  var button = form.querySelector("[data-login-submit]");
  var motion = window.matchMedia("(prefers-reduced-motion: reduce)");
  var animations = [];

  function stopWaiting() {
    animations.forEach(function (animation) { animation.cancel(); });
    animations = [];
  }

  function startWaiting() {
    stopWaiting();
    if (motion.matches || typeof button.animate !== "function") return;
    var squares = form.querySelector("[data-login-wait]").children;
    var colors = getComputedStyle(panel);
    var parked = colors.getPropertyValue("--color-sand-5").trim();
    var travelling = colors.getPropertyValue("--color-meta").trim();
    var corners = [[0, 0], [8, 0], [8, 8], [0, 8]];
    // The design's vq0–vq3 choreography: twelve moves over four seconds.
    // Appearance stays in Tailwind; the Web Animations API supplies motion.
    var paths = [
      [0, 0, 0, 0, 0, 1, 2, 3, 0, 0, 0, 0, 0],
      [0, 1, 1, 1, 1, 1, 2, 3, 0, 1, 2, 3, 0],
      [0, 1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3, 0],
      [0, 1, 2, 3, 0, 1, 2, 3, 3, 3, 3, 3, 0],
    ];
    var lifted = [
      [4, 5, 6, 7, 8],
      [0, 1, 5, 6, 7, 8, 9, 10, 11, 12],
      [0, 1, 2, 10, 11, 12],
      [0, 1, 2, 3, 4, 5, 6, 7, 11, 12],
    ];
    paths.forEach(function (path, index) {
      var frames = path.map(function (corner, step) {
        var active = lifted[index].indexOf(step) !== -1;
        return {
          offset: step === 12 ? 1 : step * 0.0833,
          transform: "translate(" + (corners[corner][0] - corners[index][0]) + "px," + (corners[corner][1] - corners[index][1]) + "px)",
          backgroundColor: active ? travelling : parked,
          zIndex: active ? 2 : 1,
          easing: "ease",
        };
      });
      animations.push(squares[index].animate(frames, { duration: 4000, iterations: Infinity }));
    });
  }

  form.addEventListener("submit", function (event) {
    if (button.disabled) {
      event.preventDefault();
      return;
    }
    button.disabled = true;
    button.setAttribute("aria-disabled", "true");
    form.setAttribute("aria-busy", "true");
    startWaiting();
  });

  // Returning via browser history must restore a usable native form.
  window.addEventListener("pageshow", function () {
    button.disabled = false;
    button.setAttribute("aria-disabled", "false");
    form.removeAttribute("aria-busy");
    stopWaiting();
  });

  var clock = panel.querySelector("[data-login-clock]");
  var zone = panel.querySelector("[data-login-zone]");
  var uptime = panel.querySelector("[data-login-uptime]");
  var verdict = panel.querySelector("[data-login-verdict]");
  var live = panel.querySelector("[data-login-live-label]");
  var mark = panel.querySelector("[data-login-live-mark]");
  var unix = Number(clock.dataset.unix);
  var offset = Number(clock.dataset.offset);
  var received = performance.now();
  var lastRefresh = received;
  var refreshing = false;
  var pulse;

  function updateMotion() {
    if (pulse) pulse.cancel();
    pulse = null;
    if (!motion.matches && panel.dataset.internet !== "unknown" && typeof mark.animate === "function") {
      pulse = mark.animate([{ opacity: 1 }, { opacity: 0.28 }, { opacity: 1 }], {
        duration: 2600, iterations: Infinity, easing: "ease-in-out",
      });
    }
    if (button.disabled) startWaiting();
  }

  function setInternet(internet) {
    var state = internet === true ? "up" : internet === false ? "down" : "unknown";
    panel.dataset.internet = state;
    verdict.textContent = panel.dataset[state + "Label"];
    live.textContent = state === "unknown" ? live.dataset.staleLabel : live.dataset.liveLabel;
    updateMotion();
  }

  function tick() {
    // Offset and epoch come from the router. UTC getters avoid substituting
    // the browser's timezone, including when the two sides cross midnight.
    var date = new Date((unix + offset) * 1000 + performance.now() - received);
    clock.textContent = [date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds()]
      .map(function (n) { return String(n).padStart(2, "0"); }).join(":");
  }

  async function refresh() {
    if (refreshing || document.hidden || button.disabled) return;
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
      received = performance.now();
      zone.textContent = status.zone;
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
  motion.addEventListener("change", updateMotion);
  updateMotion();
})();
