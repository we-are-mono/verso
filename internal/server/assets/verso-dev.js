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
//
// A redeploy that lands while a form holds unsaved work waits for it: the reload is
// held until the work is saved or undone, so a redeploy never throws the browser's
// leave prompt over a half-made edit. The dev shell hands its sessions to the next
// (Server.Close), so the save still goes through on the new build.
(function () {
  var url = "/assets/verso.css";
  var style = document.getElementById("verso-css");
  if (!style) return;
  // The served stylesheet and the inlined one are the same currentCSS() bytes, so
  // the first paint itself is the exact baseline — no first-fetch race.
  var last = style.textContent;
  var boot = null;
  var redeployed = false;

  function unsaved() {
    return !!(window.versoDirtyState && window.versoDirtyState.dirty());
  }

  function poll() {
    if (redeployed) {
      if (!unsaved()) location.reload();
      return;
    }
    fetch(url, { cache: "no-store" })
      .then(function (r) {
        if (!r.ok) return null;
        var id = r.headers.get("X-Verso-Boot");
        if (boot === null) {
          boot = id;
        } else if (id !== null && id !== boot) {
          redeployed = true;
          if (!unsaved()) location.reload();
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

// The notebook's grid fades down the page; while laying a page out it helps to
// see it whole. G, pressed outside a field, switches between the two, and the
// choice holds across reloads in this browser.
(function () {
  var key = "verso-dev-grid";
  var root = document.documentElement;
  function apply(full) { root.toggleAttribute("data-verso-grid-full", full); }
  try { apply(localStorage.getItem(key) === "full"); } catch (_) { /* storage blocked */ }
  document.addEventListener("keydown", function (event) {
    if (event.key.toLowerCase() !== "g" || event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.target.closest("input, textarea, select, [contenteditable]")) return;
    var full = !root.hasAttribute("data-verso-grid-full");
    apply(full);
    try { localStorage.setItem(key, full ? "full" : "fade"); } catch (_) { /* storage blocked */ }
  });
})();
