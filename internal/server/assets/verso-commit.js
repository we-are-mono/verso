// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-commit.js — the stage (ADR-004, ADR-010). What is waiting to be written
// and what happens when someone writes it: the rail's Pending changes row that
// says how much waits and what an edit not yet saved would add, the Pending
// changes page's apply with the device-side rollback and the confirm poll
// inside its window, and the page following its own session out when that
// session ends.
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
  var countdown = layer && layer.querySelector("[data-verso-session-countdown]");
  var stayButton = layer && layer.querySelector("[data-verso-session-stay]");
  var okButton = layer && layer.querySelector("[data-verso-session-dismiss]");
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
    var asked = method === "GET"
      ? fetch("/session", { method: "GET", credentials: "same-origin", headers: { "X-Verso-Refresh": "1" } })
      : versoPost("/session", versoBody());
    return asked.then(function (res) {
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
    countdown.textContent = versoMinutes(Math.max(0, Math.round((deadline - Date.now()) / 1000)));
  }

  function show(canExtend) {
    extendable = canExtend;
    [].forEach.call(layer.querySelectorAll("[data-verso-session-extendable]"), function (el) { el.hidden = !canExtend; });
    [].forEach.call(layer.querySelectorAll("[data-verso-session-capped]"), function (el) { el.hidden = canExtend; });
    clock();
    clearInterval(ticker);
    ticker = setInterval(clock, 1000);
    // Opened last, it stands over whatever else is open.
    if (!layer.open) {
      returnTo = document.activeElement;
      layer.showModal();
    }
    (canExtend ? stayButton : okButton).focus();
  }

  function hide() {
    clearInterval(ticker);
    versoDialogClose(layer);
  }

  // Closed by its own acts or by the browser (a second Escape it will not let
  // a page refuse), the dialog lets go the same way, and focus goes back once
  // the page is no longer shut.
  function closed() {
    clearInterval(ticker);
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
    // Escape answers the dialog the way its first button does: pressing a key
    // is someone at the router. The browser's own closing is held back so
    // staying can take its round trip with the dialog still up.
    layer.addEventListener("cancel", function (e) {
      e.preventDefault();
      if (extendable) stay();
      else hide();
    });
    layer.addEventListener("close", closed);
  }
  arm(first);
})();

// The rail's Pending changes row (ADR-010), server-rendered from UCI's own
// stage with every page and always there, hidden while the stage is clean. A
// page form's Save reloads the page, which draws the row anew. An act that
// stages without a reload (a drawer's save, a row's one-press act) hands this
// client the page it read, and the row takes that page's count as it stands:
// nothing previews an edit before Save, and nothing animates. On a phone the
// menu button carries the same count. A screen reader hears what the stage
// took.
(function () {
  var row = document.getElementById("verso-pending-row");
  if (!row) return;
  var tally = document.getElementById("verso-pending-count");
  var menu = document.querySelector(".verso-menu-button");
  var menuCount = document.getElementById("verso-menu-count");

  window.versoStaged = {
    // sync takes a fresh render of a page as the stage's truth: the count it
    // carries is the count.
    sync: function (doc) {
      var fresh = doc && doc.getElementById("verso-pending-row");
      var n = fresh ? parseInt(fresh.getAttribute("data-count") || "0", 10) : 0;
      var latest = fresh ? fresh.getAttribute("data-latest") || "" : "";
      var before = parseInt(row.getAttribute("data-count") || "0", 10);
      var took = n > 0 && latest !== "" && (n > before || latest !== (row.getAttribute("data-latest") || ""));
      if (latest) row.setAttribute("data-latest", latest);
      else row.removeAttribute("data-latest");
      row.setAttribute("data-count", String(n));
      row.hidden = n === 0;
      if (tally) tally.textContent = String(n);
      if (menu) menu.setAttribute("data-count", String(n));
      if (menuCount) {
        menuCount.textContent = String(n);
        menuCount.hidden = n === 0;
      }
      if (took) versoAnnounce(T("Pending: %s").replace("%s", latest));
    },
  };
})();

// A page form's save reloads the page, so where its Save stood on screen is
// noted as it is submitted and read when the page comes back: the page opens
// where the person was, the Save back at the same height. A refusal lands
// differently — at the refused box (verso-forms.js). A note older than a few
// seconds, or from another page, is not this save's.
(function () {
  var KEY = "verso-staged-landing";

  function differs(control) {
    var type = (control.type || "").toLowerCase();
    if (type === "checkbox" || type === "radio") return control.checked !== control.defaultChecked;
    if (control.tagName === "SELECT") return [].some.call(control.options, function (o) { return o.selected !== o.defaultSelected; });
    if (type === "hidden" || type === "submit" || type === "button" || type === "file") return false;
    return control.value !== control.defaultValue;
  }

  // edited is the first of the form's settings an edit has changed, by the
  // option it writes.
  function edited(form) {
    return [].filter.call(form.querySelectorAll("[data-verso-change-field][data-verso-change-name]"), function (row) {
      return [].some.call(row.querySelectorAll("input, select, textarea"), differs);
    })[0];
  }

  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (!form || !form.matches || !form.matches("main form") || form.hasAttribute("hx-post") || form.hasAttribute("data-verso-act")) return;
    var row = edited(form);
    if (!row) return;
    var save = e.submitter || form.querySelector('[type="submit"]');
    var note = {
      path: window.location.pathname,
      name: row.getAttribute("data-verso-change-name"),
      at: save ? save.getBoundingClientRect().top : null,
      time: Date.now(),
    };
    try { window.sessionStorage.setItem(KEY, JSON.stringify(note)); } catch (err) { /* the save lands at the top, as a plain reload */ }
  }, true);

  var note = null;
  try {
    note = JSON.parse(window.sessionStorage.getItem(KEY) || "null");
    window.sessionStorage.removeItem(KEY);
  } catch (err) { note = null; }
  if (!note || note.path !== window.location.pathname || Date.now() - note.time > 15000) return;
  if (document.querySelector('main [aria-invalid="true"]')) return;
  var row = document.querySelector('main [data-verso-change-field][data-verso-change-name="' + CSS.escape(note.name || "") + '"]');
  var form = row && row.closest("form");
  var save = form && form.querySelector('[type="submit"]');
  if (save && typeof note.at === "number") window.scrollBy(0, save.getBoundingClientRect().top - note.at);
})();

// The Pending changes page's apply (ADR-010). Apply posts the When you apply
// card's form, which carries the note; rpcd commits with its device-side
// rollback armed, and the page polls confirm inside the window (the LuCI
// cadence). Each step says only what the router reports: the save is done
// when the apply returns, the reloads are then started (procd reports no
// finish), the check-in counts the window down and is kept when a confirm
// lands. Kept, the page reads again and says the changes were applied; put
// back, it reads again once the router has staged them anew. Apply stands
// hidden until this script is here to see it through.
(function () {
  var form = document.getElementById("verso-pending-apply");
  var button = document.querySelector("[data-verso-pending-apply]");
  if (!form || !button) return;
  button.hidden = false;
  var ROLLBACK = 30;
  var GRACE = 6000;

  function step(kind) {
    return document.querySelector('[data-verso-apply-step="' + kind + '"] [data-verso-apply-state]');
  }

  // mark sets a step's state: waiting, done, or a word alone.
  function mark(kind, word, how) {
    var slot = step(kind);
    if (!slot) return;
    slot.hidden = false;
    slot.querySelector("[data-verso-apply-wait]").hidden = how !== "wait";
    slot.querySelector("[data-verso-apply-done]").hidden = how !== "done";
    slot.querySelector("[data-verso-apply-word]").textContent = word;
    slot.classList.toggle("text-green-deep", how === "done");
    slot.classList.toggle("text-crimson-deep", how === "failed");
  }

  function acts(locked) {
    [].forEach.call(document.querySelectorAll("[data-verso-pending-acts] button, #verso-pending-apply input, main form[action^='/pending/'] button"), function (el) {
      el.disabled = locked;
    });
    if (locked) versoButtons.start(button, T("Applying…"));
    else versoButtons.finish(button);
  }

  // read reads the page again: a fresh GET, never a reload, which on a page
  // drawn as the answer to a post would post it again.
  function read() {
    if (window.versoDirtyState) window.versoDirtyState.suppress();
    window.location.replace(window.location.pathname);
  }

  function apply() {
    var since = Date.now();
    var ticker = null;
    acts(true);
    mark("save", T("Saving"), "wait");

    function count() {
      var left = Math.max(0, ROLLBACK - Math.floor((Date.now() - since) / 1000));
      mark("check", T("%d s left").replace("%d", left), "wait");
    }

    function kept() {
      clearInterval(ticker);
      mark("check", T("Kept"), "done");
      setTimeout(read, 600);
    }

    // The router puts its settings back at the window's end and the shell
    // stages the changes again just after; the page reads again once it has.
    function putBack() {
      clearInterval(ticker);
      mark("check", T("Put back"), "failed");
      setTimeout(read, Math.max(0, since + ROLLBACK * 1000 + GRACE - Date.now()));
    }

    function checkIn() {
      count();
      ticker = setInterval(count, 1000);
      versoConfirmApply(function () {
        return versoPost("/uci/confirm", versoBody()).then(function (res) {
          if (!res.ok) throw new Error("not confirmed");
        });
      }, since).then(kept, putBack);
    }

    function saved() {
      mark("save", T("Saved"), "done");
      mark("reload", T("Started"));
      setTimeout(checkIn, 1000);
    }

    var note = form.querySelector('[name="note"]');
    versoPost("/uci/apply", versoBody([["note", note ? note.value : ""]]))
      .then(function (res) {
        if (!res.ok) {
          acts(false);
          mark("save", "", "");
          step("save").hidden = true;
          var refused = document.querySelector("template[data-verso-apply-refused]");
          if (refused && window.versoOutcome) window.versoOutcome.show(document.importNode(refused.content.firstElementChild, true));
          return;
        }
        saved();
      })
      .catch(function () {
        // The apply itself may have severed this page's path (a network
        // change); it keeps checking in, since reaching the router again is
        // what keeps the changes.
        saved();
      });
  }

  form.addEventListener("submit", function (e) {
    e.preventDefault();
    if (!button.disabled) apply();
  });
})();
