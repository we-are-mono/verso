// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Dev-only stylesheet hot-reload. The shell injects this script only during a
// scripts/dev.sh session (page.Dev), never in a real deployment. It polls the live
// stylesheet and swaps the inlined <style> in place, so a CSS edit appears in ~1s with
// no rebuild and no full page refresh (scroll and widget state are kept). A template
// edit cannot land that way — templates are embedded in the binary — so the response
// also carries the shell's per-process id (X-Verso-Boot), and a change there means the
// shell was redeployed: reload the whole page. CSP-safe: a same-origin fetch and an
// inline <style> edit, no eval, no new script.
(function () {
  var url = "/assets/verso.css";
  var style = document.getElementById("verso-css");
  if (!style) return;
  // The served stylesheet and the inlined one are the same currentCSS() bytes, so
  // the first paint itself is the exact baseline — no first-fetch race.
  var last = style.textContent;
  var boot = null;

  function poll() {
    fetch(url, { cache: "no-store" })
      .then(function (r) {
        if (!r.ok) return null;
        var id = r.headers.get("X-Verso-Boot");
        if (boot === null) {
          boot = id;
        } else if (id !== null && id !== boot) {
          location.reload();
          return null;
        }
        return r.text();
      })
      .then(function (css) {
        if (css === null || css === last) return;
        last = css;
        style.textContent = css;
      })
      .catch(function () { /* server mid-reload; try again next tick */ });
  }

  setInterval(poll, 600);
})();
