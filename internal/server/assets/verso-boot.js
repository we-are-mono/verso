// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Runs synchronously in <head> before the body paints, so the page never flashes the
// wrong faces or the wrong palette: it holds the first paint until the self-hosted
// fonts are ready and re-applies the saved theme. CSP-safe: served first-party, no
// inline script.
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
      document.fonts.load('1em "Fraunces"'),
      document.fonts.load('1em "Inconsolata"'),
    ]).then(function () {
      return document.fonts.ready;
    }).then(revealFonts, revealFonts);
  } else {
    revealFonts();
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
