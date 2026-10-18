// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Dev-only stylesheet hot-reload. The shell injects this script only during a
// scripts/dev.sh session (page.Dev), never in a real deployment, and a dev page
// links the live stylesheet at its stable address. The script polls it and, when
// it changes, links the new one beside the old and drops the old once the new has
// loaded, so a CSS edit appears in ~1s with no rebuild, no full page refresh (scroll
// and widget state are kept) and no unstyled frame between. A template edit cannot
// land that way — templates are embedded in the binary — so the response also
// carries the shell's per-process id (X-Verso-Boot), and a change there means the
// shell was redeployed: reload the whole page. CSP-safe: a same-origin fetch and a
// same-origin stylesheet, no eval, no new script.
//
// A redeploy that lands while a form holds unsaved work waits for it: the reload is
// held until the work is saved or undone, so a redeploy never throws the browser's
// leave prompt over a half-made edit. The dev shell hands its sessions to the next
// (Server.Close), so the save still goes through on the new build.
(function () {
  var url = "/assets/verso.css";
  var link = document.querySelector('link[rel="stylesheet"][href="' + url + '"]');
  if (!link) return;
  // The first poll's stylesheet is the baseline: a change lands from the next.
  var last = null;
  var version = 0;
  var boot = null;
  var redeployed = false;

  // swap links the new stylesheet after the one in force and drops the old
  // once the new has loaded, so the page is never without one. A change that
  // lands while the last is still loading takes its place.
  var pending = null;
  function swap() {
    if (pending) pending.remove();
    var next = link.cloneNode();
    next.href = url + "?v=" + (++version);
    pending = next;
    next.addEventListener("load", function () {
      if (pending !== next) return;
      link.remove();
      link = next;
      pending = null;
    }, { once: true });
    link.after(next);
  }

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
        var first = last === null;
        last = css;
        if (!first) swap();
      })
      .catch(function () { /* server mid-reload; try again next tick */ });
  }

  setInterval(poll, 600);
})();

// The layout grid: every fixed measure lands on an 8px line, and G, pressed
// outside a field and without a modifier, draws those lines over the page and
// takes them away again (input.css, data-verso-grid). The choice holds for the
// tab, so a hot reload or a redeploy's reload keeps it.
(function () {
  var key = "verso-dev-grid";
  var root = document.documentElement;
  function apply(on) { root.toggleAttribute("data-verso-grid", on); }
  try { apply(sessionStorage.getItem(key) === "on"); } catch (_) { /* storage blocked */ }
  document.addEventListener("keydown", function (event) {
    if (event.key.toLowerCase() !== "g" || event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.target.closest && event.target.closest("input, textarea, select, [contenteditable]")) return;
    var on = !root.hasAttribute("data-verso-grid");
    apply(on);
    try { sessionStorage.setItem(key, on ? "on" : "off"); } catch (_) { /* storage blocked */ }
  });
})();
