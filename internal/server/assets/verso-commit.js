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
// drawer shows the empty stage, and the page under it shows the settings as
// they are at once (read whole on close only where rows came or went). Apply
// commits with the device-side rollback
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

  // arrive: the chip takes the change that just reached it — its mark pops
  // from twice its size and its ground warms to the marigold line and settles,
  // once. Colour and scale only; nothing around it moves.
  function arrive() {
    if (typeof chip.animate !== "function") return;
    var line = getComputedStyle(chip).getPropertyValue("--color-marigold-line").trim();
    if (line) chip.animate([{ backgroundColor: line }, {}], { duration: 900, easing: "ease-out" });
    if (mark && !still()) mark.animate([{ transform: "scale(2.4)" }, { transform: "scale(1)" }], { duration: 420, easing: "cubic-bezier(0.22, 1, 0.36, 1)" });
  }

  function still() {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  // release lets go of a chip the page held back for a landing (verso-boot.js):
  // from here it shows whatever the flight has set it to say.
  function release() {
    document.documentElement.removeAttribute("data-verso-landing");
  }

  // Take a fresh render of the page as the stage's truth: the count it carries
  // is the count, and the chip shows it. No node is replaced, so the chip's
  // own scope stays bound.
  //
  // fly shows where a change that was just staged went. Nothing on the page
  // says a staged save happened, because it has not: the chip says a change
  // waits. So the change is seen going there, once — a marigold square, the
  // chip's own mark, lifts off the row that changed and travels in an arc to
  // the chip, which shows the count it had until the square lands and then
  // takes the new one. A chip that was empty appears as the square arrives.
  // It flies only when the stage grew since `before`; under reduced motion
  // the chip only warms.
  window.versoStaged = {
    sync: function (doc) {
      var fresh = doc && doc.getElementById("verso-staged");
      rest(fresh ? parseInt(fresh.getAttribute("data-count") || "0", 10) : 0);
    },
    count: count,
    fly: function (from, before) {
      var now = count();
      if (!(now > before)) { release(); return; }
      versoAnnounce(T("Staged — %s. Review and apply from the top bar.").replace("%s", stagedLabel(now)));
      var a = from && from.getBoundingClientRect();
      if (still() || !a || !a.width || typeof chip.animate !== "function") { release(); arrive(); return; }
      // Measured while still held out of sight: visibility keeps its box.
      var target = (mark || chip).getBoundingClientRect();
      // A chip the layout does not show (a narrow top bar) has nowhere to be
      // flown to; it only warms.
      if (!target.width) { release(); arrive(); return; }
      var x0 = a.left + a.width / 2, y0 = a.top + a.height / 2;
      var dx = target.left + target.width / 2 - x0, dy = target.top + target.height / 2 - y0;
      var square = document.createElement("span");
      square.setAttribute("aria-hidden", "true");
      square.className = "pointer-events-none fixed z-[60] size-1.5 rounded-[1px] bg-marigold";
      square.style.left = (x0 - 3) + "px";
      square.style.top = (y0 - 3) + "px";
      document.body.appendChild(square);
      // Until the square lands the chip says what it said before the save: the
      // old count, or nothing at all.
      if (before > 0) { if (label) label.textContent = stagedLabel(before); }
      else chip.style.opacity = "0";
      release();
      // The arc: the square lifts off the row first, then crosses to the chip.
      var lift = Math.min(64, Math.abs(dy) * 0.25 + 24);
      square.animate([
        { transform: "translate(0, 0) scale(1)" },
        { transform: "translate(" + dx * 0.3 + "px, " + (dy * 0.2 - lift) + "px) scale(1.8)", offset: 0.35 },
        { transform: "translate(" + dx + "px, " + dy + "px) scale(1)" },
      ], { duration: 680, easing: "cubic-bezier(0.45, 0, 0.2, 1)" }).finished.then(land, land);
      function land() {
        square.remove();
        if (label) label.textContent = stagedLabel(now);
        if (before <= 0) {
          chip.style.opacity = "";
          chip.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 180, easing: "ease-out" });
        }
        arrive();
      }
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

  // Lock both acts, with the waiting mark on the one that was requested.
  function waiting(on, activeID) {
    var apply = document.getElementById("verso-staged-apply");
    var discard = document.getElementById("verso-staged-discard");
    [apply, discard].forEach(function (button) {
      if (!button) return;
      if (on) {
        // Keep focus in the drawer when its active button becomes disabled.
        if (document.activeElement === button) {
          var dialog = button.closest('[role="dialog"]');
          if (dialog) dialog.focus();
        }
        var label = button.id === activeID ? button.getAttribute("data-busy-label") : "";
        versoButtons.start(button, label);
        if (label) versoAnnounce(label);
      } else versoButtons.finish(button);
    });
  }

  function close() {
    var panel = drawer();
    if (panel) panel.dispatchEvent(new CustomEvent("verso-panel-done", { bubbles: true }));
  }

  // behind brings the page under the drawer up to date the moment the stage
  // changes under it — discarded or applied — rather than when the drawer
  // closes. Each form that shows settings is read again and replaced by its
  // fresh self: the staged marks go, the controls show what is on the router,
  // and the replacement is taken as the form's new baseline, so nothing reads
  // as unsaved. A listing's rows are reconciled. It answers whether the page
  // is whole again: a change to which forms or rows there are is not
  // something to patch, and waits for the page to be read whole, on close.
  function behind() {
    return fetch(window.location.pathname + window.location.search, { headers: { Accept: "text/html" }, credentials: "same-origin" })
      .then(function (res) { return res.ok && !res.redirected ? res.text() : ""; })
      .then(function (html) {
        if (!html) return false;
        var doc = new DOMParser().parseFromString(html, "text/html");
        var mine = document.querySelectorAll("main form");
        var fresh = doc.querySelectorAll("main form");
        var rows = function (d) { return [].map.call(d.querySelectorAll("tr[data-verso-row-id]"), function (r) { return r.getAttribute("data-verso-row-id"); }).join(" "); };
        var whole = mine.length === fresh.length && rows(document) === rows(doc);
        if (mine.length === fresh.length) {
          [].forEach.call(mine, function (form, i) {
            if (!form.querySelector("[data-verso-change-field]")) return;
            var next = document.importNode(fresh[i], true);
            form.replaceWith(next);
            if (window.htmx) window.htmx.process(next);
          });
        }
        if (window.versoReconcile) window.versoReconcile(doc);
        return whole;
      })
      .catch(function () {
        // The page keeps what it shows; closing the drawer reads it whole.
        return false;
      });
  }

  // reload reads the page again — a fresh GET of its address, never a reload,
  // which on a page drawn as the answer to a post (a refusal) would post it
  // again and could stage what was just discarded. The fragment is left off:
  // an address that differs from this one only by it is a jump within the
  // page, not a read of it.
  function reload() {
    if (window.versoDirtyState) window.versoDirtyState.suppress();
    window.location.replace(window.location.pathname + window.location.search);
  }

  function apply() {
    var n = count();
    waiting(true, "verso-staged-apply");
    var deadline = Date.now() + 28000;

    function applied() {
      close();
      verdict("green", n === 1 ? T("1 change applied") : T("%d changes applied").replace("%d", n), false);
      behind();
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
    waiting(true, "verso-staged-discard");
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
        // rather than swept away by a reload, and the page under it shows
        // the settings as they are again at once.
        stale = true;
        behind().then(function (whole) { if (whole) stale = false; });
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

// A page form's save reloads the page, so where its change went is noted as it
// is submitted and shown when the page comes back: which rows differ from what
// the page was drawn with, where its Save stood on screen, and the count the
// chip had. On landing the page opens where the person was (the Save back at
// the same height), each row that changed takes the marigold "staged" mark,
// and the change flies from the first of them to the chip (versoStaged.fly).
// A refusal lands differently — at the refused box (verso-forms.js) — and
// nothing is marked. A note older than a few seconds, or from another page,
// is not this save's.
(function () {
  var KEY = "verso-staged-origin";

  function differs(control) {
    var type = (control.type || "").toLowerCase();
    if (type === "checkbox" || type === "radio") return control.checked !== control.defaultChecked;
    if (control.tagName === "SELECT") return [].some.call(control.options, function (o) { return o.selected !== o.defaultSelected; });
    if (type === "hidden" || type === "submit" || type === "button" || type === "file") return false;
    return control.value !== control.defaultValue;
  }

  function changedRows(form) {
    return [].filter.call(form.querySelectorAll("[data-verso-change-field][data-verso-change-name]"), function (row) {
      return [].some.call(row.querySelectorAll("input, select, textarea"), differs);
    });
  }

  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (!form || !form.matches || !form.matches("main form") || form.hasAttribute("hx-post") || form.hasAttribute("data-verso-act")) return;
    var rows = changedRows(form);
    if (!rows.length || !window.versoStaged) return;
    var save = e.submitter || form.querySelector('[type="submit"]');
    var note = {
      path: window.location.pathname,
      names: rows.map(function (row) { return row.getAttribute("data-verso-change-name"); }),
      at: save ? save.getBoundingClientRect().top : null,
      count: window.versoStaged.count(),
      time: Date.now(),
    };
    try { window.sessionStorage.setItem(KEY, JSON.stringify(note)); } catch (err) { /* the save lands at the top, as a plain reload */ }
  }, true);

  // mark: the row's change waits on the stage — the chip's own mark and word
  // beside the row's name. The server draws it wherever a control says where
  // its option lives (verso-staged-row); a row that does not say is marked
  // here, the same way. It returns the square, which the flight leaves from.
  // A part of a fused box is a change of its own inside a row that holds
  // several; the row's one label line carries the one mark for them all.
  function mark(row) {
    row = row.closest(".verso-field-row") || row;
    var drawn = row.querySelector("[data-verso-staged-row] [aria-hidden]");
    if (drawn) return drawn;
    var label = row.querySelector("label");
    var host = row.querySelector(".verso-field-label") || (label ? label.parentElement : row.firstElementChild || row);
    var tag = document.createElement("span");
    tag.setAttribute("data-verso-staged-row", "");
    tag.className = "inline-flex items-center gap-1.5 text-sm font-medium text-marigold-deep";
    var square = document.createElement("span");
    square.setAttribute("aria-hidden", "true");
    square.className = "size-1.5 shrink-0 rounded-[1px] bg-marigold";
    tag.append(square, document.createTextNode(T("staged")));
    host.appendChild(tag);
    return square;
  }

  // letGo shows the chip the page held back for this landing (verso-boot.js)
  // when there is no flight to wait for after all.
  function letGo() {
    document.documentElement.removeAttribute("data-verso-landing");
  }

  var note = null;
  try {
    note = JSON.parse(window.sessionStorage.getItem(KEY) || "null");
    window.sessionStorage.removeItem(KEY);
  } catch (err) { note = null; }
  if (!note || note.path !== window.location.pathname || Date.now() - note.time > 15000) return letGo();
  if (document.querySelector('main [aria-invalid="true"]') || !window.versoStaged) return letGo();
  var rows = (note.names || []).map(function (name) {
    return document.querySelector('main [data-verso-change-field][data-verso-change-name="' + CSS.escape(name) + '"]');
  }).filter(Boolean);
  if (!rows.length) return letGo();
  var form = rows[0].closest("form");
  var save = form && form.querySelector('[type="submit"]');
  if (save && typeof note.at === "number") window.scrollBy(0, save.getBoundingClientRect().top - note.at);
  if (!(window.versoStaged.count() > note.count)) return letGo();
  var squares = rows.map(mark);
  // A beat after the page is drawn, so the eye has found the row before the
  // change leaves it.
  window.setTimeout(function () { window.versoStaged.fly(squares[0], note.count); }, 240);
})();
