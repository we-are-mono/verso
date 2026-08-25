// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Runs synchronously in <head> before the body paints, so the Advanced-settings sidebar
// seam renders in its saved open/closed state with no flash and no re-animation on every
// navigation. It only sets a class from storage; the toggle (verso.js "advseam") flips
// the same class and storage key. CSP-safe: served first-party, no inline script.
(function () {
  try {
    if (localStorage.getItem("verso-adv") === "1") {
      document.documentElement.classList.add("verso-adv-open");
    }
  } catch (e) {
    /* storage blocked; seam stays collapsed until toggled */
  }
})();
