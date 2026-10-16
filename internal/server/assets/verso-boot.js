// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Runs synchronously in <head> before the body paints, for the one thing that
// has to be decided before anything is drawn. CSP-safe: served first-party, no
// inline script. The faces need no script: their @font-face rules hold the
// text until they are ready (font-display: block, input.css).

// A save that staged is about to be told in the rail's Pending changes row
// (verso-commit.js), its count turning over from the one it had. The row must
// not show the new count before it turns, so on the page the save lands on
// the count is held out of sight from the first paint; the turn lets it go.
// The note is the one the save left.
(function () {
  try {
    var note = JSON.parse(window.sessionStorage.getItem("verso-staged-landing") || "null");
    if (note && note.path === window.location.pathname && Date.now() - note.time < 15000) {
      var el = document.documentElement;
      el.setAttribute("data-verso-landing", "");
      // A turn is under a second. If the page's script never runs, the
      // count must not stay hidden: it comes back on its own.
      window.setTimeout(function () { el.removeAttribute("data-verso-landing"); }, 3000);
    }
  } catch (err) {
    /* no storage: the page lands as a plain read */
  }
})();
