// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Runs synchronously in <head> before the body paints, for the one thing that
// has to be decided before anything is drawn. CSP-safe: served first-party, no
// inline script. The faces need no script: their @font-face rules hold the
// text until they are ready (font-display: block, input.css).

// A save that staged is about to fly to the chip (verso-commit.js). The chip
// must not show the count the change is flying to before the change gets
// there, so on the page the save lands on it is held out of sight from the
// first paint; the flight lets it go. The note is the one the save left.
(function () {
  try {
    var note = JSON.parse(window.sessionStorage.getItem("verso-staged-origin") || "null");
    if (note && note.path === window.location.pathname && Date.now() - note.time < 15000) {
      var el = document.documentElement;
      el.setAttribute("data-verso-landing", "");
      // A flight is under a second. If the page's script never runs, the
      // chip must not stay hidden: it comes back on its own.
      window.setTimeout(function () { el.removeAttribute("data-verso-landing"); }, 3000);
    }
  } catch (err) {
    /* no storage: the page lands as a plain read */
  }
})();
