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
// A minute before the end, the page says so first (the session-ending dialog
// in page.html.tmpl): how long is left, and one act to stay. It asks the router
// before it warns, because activity in this page or another tab may already
// have moved the end, and a warning about an end that is no longer coming is
// noise. Staying posts to /session, which is activity, and the page re-arms
// from the answer; at the twelve-hour cap there is nothing to extend, and the
// dialog only says so.
//
// Every deadline is a span from now, never a timestamp compared with the
// browser's clock, so a clock that disagrees with the router's cannot pull the
// end forward: the first comes from the page's stamp against load (a stamp
// already past on a page just rendered is skew, and the timer stands down), the
// rest from the router's own count of seconds left.
(function () {
  var meta = document.querySelector('meta[name="verso-session-expiry"]');
  if (!meta) return;
  var first = Date.parse(meta.content) - Date.now();
  if (!(first > 0)) return;

  var WARN = 60 * 1000;
  var layer = document.getElementById("verso-session-ending");
  var box = layer && layer.querySelector('[role="alertdialog"]');
  var countdown = layer && layer.querySelector("[data-verso-session-countdown]");
  var stayButton = layer && layer.querySelector("[data-verso-session-stay]");
  var okButton = layer && layer.querySelector("[data-verso-session-dismiss]");
  var csrfMeta = document.querySelector('meta[name="verso-csrf"]');
  var csrf = csrfMeta ? csrfMeta.content : "";
  var deadline = 0;
  var warnTimer = null;
  var endTimer = null;
  var ticker = null;
  var returnTo = null;
  var extendable = true;

  // The end: reload rather than navigate, so the middleware decides. A dead
  // session lands on /login?expired=1 with its notice; one kept alive elsewhere
  // re-renders this page and re-arms from the fresh stamp.
  function end() {
    if (window.versoDirtyState) window.versoDirtyState.suppress();
    window.location.reload();
  }

  // Arm the end, and the look a minute before it. With the warning already up
  // there is nothing more to look for: only the end is armed, or a session with
  // under a minute left would ask the router again at once, and again.
  function arm(ms, warning) {
    clearTimeout(warnTimer);
    clearTimeout(endTimer);
    deadline = Date.now() + ms;
    endTimer = setTimeout(end, ms);
    if (layer && !warning) warnTimer = setTimeout(look, Math.max(0, ms - WARN));
  }

  // What the router says is left. An answer that is not the session's state is
  // the login page a redirect led to: the session is already gone.
  function state(method) {
    var init = { method: method, credentials: "same-origin", headers: {} };
    if (method === "GET") init.headers["X-Verso-Refresh"] = "1";
    else {
      init.headers["Content-Type"] = "application/x-www-form-urlencoded";
      init.body = "_csrf=" + encodeURIComponent(csrf);
    }
    return fetch("/session", init).then(function (res) {
      var type = res.headers.get("Content-Type") || "";
      if (!res.ok || type.indexOf("application/json") !== 0) throw new Error("signed out");
      return res.json();
    });
  }

  function look() {
    state("GET").then(function (st) {
      if (st.remaining * 1000 > WARN + 5000) {
        arm(st.remaining * 1000);
        return;
      }
      arm(st.remaining * 1000, true);
      show(st.extendable);
    }, function (err) {
      // Signed out already, or the router out of reach: either way the end is
      // coming, and the warning is still the truest thing to show.
      if (err && err.message === "signed out") end();
      else show(extendable);
    });
  }

  function clock() {
    var left = Math.max(0, Math.round((deadline - Date.now()) / 1000));
    countdown.textContent = Math.floor(left / 60) + ":" + String(left % 60).padStart(2, "0");
  }

  function show(canExtend) {
    extendable = canExtend;
    [].forEach.call(layer.querySelectorAll("[data-verso-session-extendable]"), function (el) { el.hidden = !canExtend; });
    [].forEach.call(layer.querySelectorAll("[data-verso-session-capped]"), function (el) { el.hidden = canExtend; });
    clock();
    clearInterval(ticker);
    ticker = setInterval(clock, 1000);
    if (layer.hidden) returnTo = document.activeElement;
    layer.hidden = false;
    versoLayerOpen(layer);
    (canExtend ? stayButton : okButton).focus();
  }

  function hide() {
    clearInterval(ticker);
    layer.hidden = true;
    versoLayerClose(layer);
    if (returnTo && returnTo.focus && document.contains(returnTo)) returnTo.focus();
    returnTo = null;
  }

  function stay() {
    stayButton.disabled = true;
    state("POST").then(function (st) {
      stayButton.disabled = false;
      arm(st.remaining * 1000);
      hide();
      versoAnnounce(T("You’re still signed in."));
    }, end);
  }

  if (layer) {
    stayButton.addEventListener("click", stay);
    okButton.addEventListener("click", hide);
    // The dialog keeps focus while it is up, and Escape answers it the way
    // its first button does: pressing a key is someone at the router.
    box.addEventListener("keydown", function (e) {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        if (extendable) stay();
        else hide();
        return;
      }
      if (e.key !== "Tab") return;
      var f = versoTabbable(box);
      if (!f.length) return;
      var active = document.activeElement;
      if (e.shiftKey && active === f[0]) {
        e.preventDefault();
        f[f.length - 1].focus();
      } else if (!e.shiftKey && active === f[f.length - 1]) {
        e.preventDefault();
        f[0].focus();
      }
    });
  }
  arm(first);
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
    dot.className = "mt-1.75 size-1.5 shrink-0 rounded-[1px] " + (tone === "crimson" ? "bg-crimson" : "bg-denim");
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
