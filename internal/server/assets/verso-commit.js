// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-commit.js — the stage (ADR-004, ADR-010). What is waiting to be written
// and what happens when someone writes it: the chip in the top bar that says
// how much waits, the drawer's two posts, the device-side rollback and the
// confirm poll inside its window, and the page following its own session out
// when that session ends.
//
// It publishes window.versoStaged (take this render of the stage as the new
// truth) and reads window.versoDirtyState from verso-forms.js, inside a
// handler rather than at load.

// A page left open outlives its session. The shell stamps the moment that
// session ends on every render, so the page can follow it out instead of
// learning about the sign-out from the next click — landing on the login page,
// which explains itself. The leave guard is suppressed first: warning someone
// about unsaved work in a session that can no longer hold it is a lie.
//
// The deadline is taken once, relative to load, so a browser clock that
// disagrees with the router's cannot pull it forward: a stamp already in the
// past on a page the shell just rendered is skew, not expiry, and the timer
// stands down. Drift the other way costs nothing — the middleware redirects any
// request that outlives the session anyway.
(function () {
  var meta = document.querySelector('meta[name="verso-session-expiry"]');
  if (!meta) return;
  var remaining = Date.parse(meta.content) - Date.now();
  if (!(remaining > 0)) return;
  setTimeout(function () {
    if (window.versoDirtyState) window.versoDirtyState.suppress();
    // Reload rather than navigate: the middleware decides. A dead session
    // lands on /login?expired=1 with its notice; a session another tab kept
    // alive re-renders this page and re-arms this timer from the fresh stamp.
    window.location.reload();
  }, remaining);
})();

// The staged-changes chip and its drawer (ADR-010). Both are server-rendered
// from UCI's own stage: the chip with the page, the drawer when it opens. This
// client keeps the chip in step when an act on the page stages something
// without a reload, and drives the drawer's two posts. Discard reverts, the
// drawer shows the empty stage, and the page reloads clean once the drawer is
// closed. Apply commits with the device-side rollback
// armed, then polls confirm inside the window (the LuCI cadence); confirmed,
// the drawer closes, the chip says so in green and fades, and the page reloads
// clean behind it. If confirm never lands the router reverts itself, and the
// chip says that instead.
(function () {
  var chip = document.getElementById("verso-staged");
  if (!chip) return;
  var label = document.getElementById("verso-staged-label");
  var mark = document.getElementById("verso-staged-mark");
  var word = document.getElementById("verso-staged-word");
  var csrfMeta = document.querySelector('meta[name="verso-csrf"]');
  var csrf = csrfMeta ? csrfMeta.content : "";
  var resting = chip.className;
  var settle = null;

  function count() {
    return parseInt(chip.getAttribute("data-count") || "0", 10);
  }

  function stagedLabel(n) {
    return n === 1 ? T("1 staged change") : T("%d staged changes").replace("%d", n);
  }

  // The chip in its resting state: the caveat colour, the count, the way in.
  function rest(n) {
    if (settle) clearTimeout(settle);
    settle = null;
    chip.className = resting;
    chip.setAttribute("data-count", String(n));
    chip.hidden = n === 0;
    if (label) label.textContent = stagedLabel(n);
    if (mark) mark.className = "size-1.5 shrink-0 rounded-[1px] bg-marigold";
    if (word) word.hidden = false;
  }

  // The chip as a verdict on an apply: green for what took effect, crimson for
  // what the router took back. Neither is a way in — green has nothing left to
  // open, and crimson opens the drawer again to show what stands.
  function verdict(tone, text, opensDrawer) {
    chip.hidden = false;
    chip.className = resting
      .replace("bg-marigold-soft", "bg-transparent")
      .replace("text-marigold-deep", tone === "green" ? "text-green-deep" : "text-crimson-deep")
      .replace("hover:border-marigold hover:bg-marigold-line", tone === "green" ? "pointer-events-none" : "hover:border-crimson hover:bg-crimson-soft");
    if (mark) mark.className = "size-1.5 shrink-0 rounded-[1px] " + (tone === "green" ? "bg-green" : "bg-crimson");
    if (label) label.textContent = text;
    if (word) word.hidden = !opensDrawer;
    // The chip changes out of sight of anyone not looking at it; how an apply
    // went is said as well as shown.
    versoAnnounce(text);
  }

  // Take a fresh render of the page as the stage's truth: the count it carries
  // is the count, and the chip shows it. No node is replaced, so the chip's
  // own scope stays bound.
  window.versoStaged = {
    sync: function (doc) {
      var fresh = doc && doc.getElementById("verso-staged");
      rest(fresh ? parseInt(fresh.getAttribute("data-count") || "0", 10) : 0);
    },
  };

  function post(path) {
    return fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: "_csrf=" + encodeURIComponent(csrf),
      credentials: "same-origin",
    });
  }

  function drawer() {
    return document.getElementById("verso-staged-review");
  }

  function note(text, tone) {
    var el = document.getElementById("verso-staged-note");
    var dot = document.getElementById("verso-staged-note-mark");
    if (el) el.textContent = text;
    if (text) versoAnnounce(text);
    if (!dot) return;
    dot.className = "mt-[7px] size-1.5 shrink-0 rounded-[1px] " + (tone === "crimson" ? "bg-crimson" : "bg-denim");
  }

  // The acts wait together: Apply goes to its waiting state and Discard fades,
  // because a stage that is being applied is not one to throw away.
  function waiting(on) {
    var apply = document.getElementById("verso-staged-apply");
    var discard = document.getElementById("verso-staged-discard");
    if (apply) {
      if (on) {
        apply.dataset.label = apply.textContent;
        apply.textContent = apply.getAttribute("data-applying") || "";
        // A disabled button lets go of focus, which would drop it behind the
        // drawer; the drawer itself holds it while the apply runs.
        if (document.activeElement === apply) {
          var dialog = apply.closest('[role="dialog"]');
          if (dialog) dialog.focus();
        }
        versoAnnounce(apply.textContent);
      } else if (apply.dataset.label) {
        apply.textContent = apply.dataset.label;
      }
      apply.disabled = on;
      apply.setAttribute("aria-disabled", on ? "true" : "false");
    }
    if (discard) discard.disabled = on;
  }

  function close() {
    var panel = drawer();
    if (panel) panel.dispatchEvent(new CustomEvent("verso-panel-done", { bubbles: true }));
  }

  function reload() {
    if (window.versoDirtyState) window.versoDirtyState.suppress();
    window.location.reload();
  }

  function apply() {
    var n = count();
    waiting(true);
    var deadline = Date.now() + 28000;

    function applied() {
      close();
      verdict("green", n === 1 ? T("1 change applied") : T("%d changes applied").replace("%d", n), false);
      // The word fades once it has been read, and the page comes back clean
      // behind it: what it shows was rendered against a stage that is now
      // empty, and a corner of it — the hostname, say — may have just changed.
      settle = setTimeout(function () {
        chip.classList.add("opacity-0");
        settle = setTimeout(reload, 600);
      }, 4000);
    }

    function rolledBack(text) {
      waiting(false);
      note(text, "crimson");
      verdict("crimson", T("Apply rolled back"), true);
    }

    function confirmLoop() {
      post("/uci/confirm")
        .then(function (res) {
          if (res.ok) {
            applied();
            return;
          }
          retry();
        })
        .catch(retry);
    }
    function retry() {
      if (Date.now() < deadline) {
        setTimeout(confirmLoop, 500);
        return;
      }
      rolledBack(T("Couldn’t confirm — the router may have rolled back"));
    }

    post("/uci/apply")
      .then(function (res) {
        if (!res.ok) {
          waiting(false);
          note(T("Couldn’t apply — check the settings and try again"), "crimson");
          return;
        }
        setTimeout(confirmLoop, 1000);
      })
      .catch(function () {
        // The apply itself may have severed our path (a network change); keep
        // trying to confirm — reaching the router again is the success signal.
        setTimeout(confirmLoop, 1000);
      });
  }

  // The frame the drawer's contents were fetched into — where a fresh reading
  // of the stage goes.
  function frame() {
    var panel = drawer();
    return panel ? panel.closest("[data-verso-panel]") : null;
  }

  // After a discard the page under the drawer still shows what was staged, so
  // it reloads once the drawer is closed — behind the person's own act, never
  // while they are still reading what the drawer says.
  var stale = false;

  function discard() {
    waiting(true);
    post("/uci/discard")
      .then(function (res) {
        if (!res.ok) throw new Error("discard failed");
        rest(0);
        var panel = frame();
        if (!panel || !window.htmx) {
          reload();
          return;
        }
        // The stage is empty. The drawer stays open and reads it again — its
        // empty state, with a Close — so what just happened is on screen
        // rather than swept away by a reload.
        stale = true;
        window.htmx.ajax("GET", panel.getAttribute("data-verso-panel-loaded") || "/uci/review", { target: panel, swap: "innerHTML" });
      })
      .catch(function () {
        waiting(false);
        note(T("Couldn’t discard — try again"), "crimson");
      });
  }

  document.addEventListener("verso-panel-hidden", function (e) {
    if (!stale || !e.target || !e.target.contains(chip)) return;
    stale = false;
    // Behind the drawer's leave transition, so it is seen sliding out.
    setTimeout(reload, 250);
  });

  // The drawer's contents arrive by fetch each time it opens, so its acts are
  // heard from the document rather than bound to nodes that will be replaced.
  document.addEventListener("click", function (e) {
    var target = e.target && e.target.closest ? e.target.closest("#verso-staged-apply, #verso-staged-discard") : null;
    if (!target || target.disabled) return;
    e.preventDefault();
    if (target.id === "verso-staged-apply") apply();
    else discard();
  });
})();
