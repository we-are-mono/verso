// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Runs synchronously in <head> before the body paints, so the Advanced-settings sidebar
// seam renders in its saved open/closed state with no flash and no re-animation on every
// navigation. It only sets a class from storage; the toggle (verso.js "advseam") flips
// the same class and storage key. CSP-safe: served first-party, no inline script.
(function () {
  var el = document.documentElement;
  try {
    if (localStorage.getItem("verso-adv") === "1") {
      el.classList.add("verso-adv-open");
    }
  } catch (e) {
    /* storage blocked; seam stays collapsed until toggled */
  }
  // Theme: a stored choice wins; otherwise follow the OS. Applied here, before paint, so
  // dark mode never flashes light on load.
  try {
    var t = localStorage.getItem("verso-theme");
    if (t === "dark" || (t === null && window.matchMedia("(prefers-color-scheme: dark)").matches)) {
      el.classList.add("dark");
    }
  } catch (e) {
    /* storage/matchMedia blocked; stay light */
  }
})();
