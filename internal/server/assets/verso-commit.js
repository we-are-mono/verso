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
// stage with every page and always there, hidden while the stage is clean.
// This client keeps it in step when an act on the page stages something
// without a reload, and says in its second line, for a moment, what the stage
// just took — the change's own words from the Pending changes page — while its
// count turns over to the new one. An edit not yet saved shows there before
// the save, dashed, with what it would add; with nothing staged yet the whole
// row stands dashed. Nothing travels across the screen. On a phone the menu
// button carries the same count, and the dashed "+1".
(function () {
  var row = document.getElementById("verso-pending-row");
  if (!row) return;
  var tally = document.getElementById("verso-pending-count");
  var line = document.getElementById("verso-pending-line");
  var menu = document.querySelector(".verso-menu-button");
  var menuCount = document.getElementById("verso-menu-count");
  var menuGhost = document.getElementById("verso-menu-ghost");
  var told = null;
  var TOLD = 3200;

  function count() {
    return parseInt(row.getAttribute("data-count") || "0", 10);
  }

  // shown puts the count on the row and the menu button: turned over, the old
  // count rising out of its line as the new one rises in (versoTurn).
  function shown(n, turned) {
    row.setAttribute("data-count", String(n));
    if (menu) menu.setAttribute("data-count", String(n));
    if (menuCount) menuCount.hidden = n === 0;
    [tally, menuCount].forEach(function (el) {
      if (!el) return;
      if (turned) versoTurn(el, String(n));
      else el.textContent = String(n);
    });
    if (row.getAttribute("data-state") !== "ghost") row.hidden = n === 0;
  }

  // say writes the row's second line: "ghost", dashed, what a save would add;
  // "told", plain words, what the stage just took; nothing, folded away.
  function say(state, words, delta) {
    clearTimeout(told);
    if (!state) {
      row.removeAttribute("data-state");
      row.removeAttribute("data-clean");
      row.hidden = count() === 0;
      if (menu) menu.removeAttribute("data-ghost");
      if (menuGhost) menuGhost.hidden = true;
      return;
    }
    line.textContent = "";
    if (delta) line.appendChild(versoEl("span", "verso-pending-delta", delta));
    line.appendChild(document.createTextNode(words));
    row.setAttribute("data-state", state);
    if (state === "ghost") {
      row.hidden = false;
      if (count() === 0) row.setAttribute("data-clean", "");
      if (menu) menu.setAttribute("data-ghost", "");
      if (menuGhost) {
        menuGhost.hidden = !delta;
        menuGhost.textContent = delta || "";
      }
      return;
    }
    row.removeAttribute("data-clean");
    if (menu) menu.removeAttribute("data-ghost");
    if (menuGhost) menuGhost.hidden = true;
    versoAnnounce(T("Pending: %s").replace("%s", words));
    told = setTimeout(function () { say(null); }, TOLD);
  }

  // warm: the row takes what just reached it — its wash warms toward the
  // stage's marigold and settles, once.
  function warm() {
    row.removeAttribute("data-warm");
    void row.offsetWidth;
    row.setAttribute("data-warm", "");
  }

  // tell says what the stage just took, its count turning over from before.
  function tell(before) {
    var n = count();
    var latest = row.getAttribute("data-latest") || "";
    if (typeof before === "number" && before !== n) {
      shown(before, false);
      shown(n, true);
    }
    if (n > 0 && latest) {
      say("told", latest);
      warm();
    } else {
      say(null);
    }
  }

  window.versoStaged = {
    count: count,
    // sync takes a fresh render of a page as the stage's truth: the count it
    // carries is the count, and where the stage holds a change it did not a
    // moment ago, or one it took anew, the row says so.
    sync: function (doc) {
      var fresh = doc && doc.getElementById("verso-pending-row");
      var n = fresh ? parseInt(fresh.getAttribute("data-count") || "0", 10) : 0;
      var latest = fresh ? fresh.getAttribute("data-latest") || "" : "";
      var before = count();
      var was = row.getAttribute("data-latest") || "";
      if (latest) row.setAttribute("data-latest", latest);
      else row.removeAttribute("data-latest");
      shown(n, false);
      if (n !== before || latest !== was) tell(before);
      else if (row.getAttribute("data-state") === "ghost") say(null);
    },
    // preview shows an edit not yet saved, dashed; clear lets it go.
    preview: function (words, delta) { say("ghost", words, delta); },
    clear: function () { if (row.getAttribute("data-state") === "ghost") say(null); },
    // landed is the page a save lands on: its count turns over from before
    // and the row says what the save staged.
    landed: function (before) { tell(before); },
  };
})();

// An edit not yet saved, in a form whose save stages (data-verso-stages, on a
// page or in a drawer), shows in the Pending changes row as it is made: what
// the save would stage, dashed, led by "+1" for each setting or item not
// already pending. A drawer's edit is said as the row it opened from. Putting
// the edit back, or closing the drawer, lets the line go. Hover changes
// nothing.
(function () {
  if (!window.versoStaged) return;

  function differs(control) {
    var type = (control.type || "").toLowerCase();
    if (type === "checkbox" || type === "radio") return control.checked !== control.defaultChecked;
    if (control.tagName === "SELECT") return [].some.call(control.options, function (o) { return o.selected !== o.defaultSelected; });
    if (type === "hidden" || type === "submit" || type === "button" || type === "file") return false;
    return control.value !== control.defaultValue;
  }

  // edited is each of the form's settings an edit has changed, with the
  // control that differs.
  function edited(form) {
    var out = [];
    [].forEach.call(form.querySelectorAll("[data-verso-change-field]"), function (row) {
      var control = [].filter.call(row.querySelectorAll("input, select, textarea"), differs)[0];
      if (control) out.push({ row: row, control: control });
    });
    return out;
  }

  function named(row, control) {
    var label = row.getAttribute("data-verso-change-label");
    if (label) return label;
    var el = row.querySelector("label");
    return el ? el.textContent.trim() : control.name;
  }

  // said is a value as the line says it: a switch's word, a choice's own
  // words, a secret never.
  function said(control) {
    var type = (control.type || "").toLowerCase();
    if (type === "checkbox") return control.checked ? T("On") : T("Off");
    if (type === "password") return T("changed");
    if (control.tagName === "SELECT") {
      var option = control.options[control.selectedIndex];
      return option ? option.textContent.trim() : "";
    }
    if (type === "radio") {
      var picked = control.form && control.form.querySelector('input[type="radio"][name="' + CSS.escape(control.name) + '"]:checked');
      var label = picked && picked.closest("label");
      return label ? label.textContent.trim() : picked ? picked.value : "";
    }
    return control.value === "" ? T("not set") : control.value;
  }

  // subject is the row a drawer opened from, by its name, where the form
  // stands in one.
  function subject(form) {
    var frame = form.closest("[data-verso-panel]");
    if (!frame || !window.Alpine) return "";
    var scope = window.Alpine.$data(frame);
    var opener = scope && scope._root;
    var tr = opener && opener.closest ? opener.closest("tr[data-verso-row-name]") : null;
    return tr ? tr.getAttribute("data-verso-row-name") : "";
  }

  function preview(form) {
    var edits = edited(form);
    if (!edits.length) {
      window.versoStaged.clear();
      return;
    }
    var first = edits[0];
    var name = subject(form);
    var words = (name ? name + " · " : "") + named(first.row, first.control) + " → " + said(first.control);
    if (edits.length > 1) words += " " + T("+%d more").replace("%d", edits.length - 1);
    // A drawer's edit is one item: new to the stage unless any of its
    // settings already waits. A page's settings each count on their own.
    var fresh = form.closest("[data-verso-panel]")
      ? (form.querySelector("[data-verso-staged-row]") ? 0 : 1)
      : edits.filter(function (e) { return !e.row.querySelector("[data-verso-staged-row]"); }).length;
    window.versoStaged.preview(words, fresh ? "+" + fresh : "");
  }

  function staging(target) {
    return target && target.closest ? target.closest("form[data-verso-stages]") : null;
  }

  ["input", "change"].forEach(function (kind) {
    document.addEventListener(kind, function (e) {
      var form = staging(e.target);
      if (form) preview(form);
    });
  });
  document.addEventListener("reset", function (e) {
    if (staging(e.target)) window.versoStaged.clear();
  });
  document.addEventListener("verso-panel-hidden", function () { window.versoStaged.clear(); });
})();

// A page form's save reloads the page, so what it left is noted as it is
// submitted and read when the page comes back: where its Save stood on
// screen, and the count the row had. On landing the page opens where the
// person was (the Save back at the same height) and the row says what the
// save staged, its count turning over from the one it had. A refusal lands
// differently — at the refused box (verso-forms.js). A note older than a few
// seconds, or from another page, is not this save's.
(function () {
  var KEY = "verso-staged-landing";

  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (!form || !form.matches || !form.matches("main form[data-verso-stages]") || form.hasAttribute("hx-post")) return;
    if (!window.versoStaged) return;
    var save = e.submitter || form.querySelector('[type="submit"]');
    var note = {
      path: window.location.pathname,
      at: save ? save.getBoundingClientRect().top : null,
      count: window.versoStaged.count(),
      time: Date.now(),
    };
    try { window.sessionStorage.setItem(KEY, JSON.stringify(note)); } catch (err) { /* the save lands at the top, as a plain reload */ }
  }, true);

  // letGo shows the count the page held back for this landing
  // (verso-boot.js).
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
  var form = document.querySelector("main form[data-verso-stages]");
  var save = form && form.querySelector('[type="submit"]');
  if (save && typeof note.at === "number") window.scrollBy(0, save.getBoundingClientRect().top - note.at);
  letGo();
  window.versoStaged.landed(note.count);
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
