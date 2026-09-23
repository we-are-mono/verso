// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Runs synchronously in <head> before the body paints, so the page never flashes the
// wrong faces: it holds the first paint until the self-hosted fonts are ready.
// CSP-safe: served first-party, no inline script.
(function () {
  var el = document.documentElement;

  // Hold the first paint until the self-hosted UI faces are ready. The timeout is
  // deliberately defensive: a missing or corrupt font must never strand the UI.
  el.classList.add("verso-fonts-loading");
  var fontsRevealed = false;
  var revealFonts = function () {
    if (fontsRevealed) return;
    fontsRevealed = true;
    window.clearTimeout(fontFallback);
    el.classList.remove("verso-fonts-loading");
  };
  var fontFallback = window.setTimeout(revealFonts, 3000);
  if (document.fonts && typeof document.fonts.load === "function") {
    Promise.all([
      document.fonts.load('1em "Hanken Grotesk"'),
      document.fonts.load('1em "Inconsolata"'),
    ]).then(function () {
      return document.fonts.ready;
    }).then(revealFonts, revealFonts);
  } else {
    revealFonts();
  }
})();

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
