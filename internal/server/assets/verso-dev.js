// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Dev-only stylesheet hot-reload. The shell injects this script only during a
// scripts/dev.sh session (page.Dev), never in a real deployment. It polls the live
// stylesheet and swaps the inlined <style> in place, so a CSS edit appears in ~1s with
// no rebuild and no full page refresh (scroll and widget state are kept). CSP-safe: a
// same-origin fetch and an inline <style> edit, no eval, no new script.
(function () {
  var url = "/assets/verso.css";
  var style = document.querySelector("style");
  var last = null;

  function poll() {
    fetch(url, { cache: "no-store" })
      .then(function (r) { return r.ok ? r.text() : null; })
      .then(function (css) {
        if (css === null) return;
        if (last === null) { last = css; return; } // baseline: matches first paint
        if (css !== last) {
          last = css;
          if (style) style.textContent = css;
        }
      })
      .catch(function () { /* server mid-reload; try again next tick */ });
  }

  setInterval(poll, 600);
})();
