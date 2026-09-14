// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-takeover.js — the firmware upgrade (ADR-004). The one screen that holds
// someone while the router rewrites itself: poll the status, move between the
// states the server already rendered, and say nothing in this file's own words.

// The firmware-upgrade takeover: while the server holds the person on the
// upgrading screen, this polls the status endpoint and moves the surface between
// states without a full reload — the calm path preparing → installing →
// restarting → done. It writes no user-facing prose: every state is a surface the
// server already rendered (i18n and escaping done there), and the client only
// toggles which one shows. The one motion is the empty widget's own pulse.
//
// The states are honest about the network. A failing poll after progress means
// the router is flashing and gone — that is "restarting", and the client keeps
// retrying. Reaching the server again is success arriving: on the new build the
// session is gone, so the status endpoint answers with a login redirect rather
// than JSON, and that reachable-but-not-status answer is exactly the "done"
// signal. A failure the server reports is terminal and reached by reload, so the
// tool's own words come from the server, never injected here.
(function () {
  var root = document.querySelector("[data-verso-upgrading]");
  if (!root) return;
  var STATUS_URL = "/system/maintenance/updates/firmware/status";
  var POLL_MS = 2000;
  var initial = root.getAttribute("data-verso-upgrading-state") || "preparing";

  // A failure is terminal and server-authoritative: the surface is already
  // shown, the buttons are real posts, and there is nothing to poll for. Polling
  // it would only reload onto the same failure.
  if (initial === "failed") return;

  var surfaces = {};
  [].forEach.call(root.querySelectorAll("[data-verso-upgrading-surface]"), function (el) {
    surfaces[el.getAttribute("data-verso-upgrading-surface")] = el;
  });

  // The leave guard: while the router is being written, an accidental close
  // warns (reusing the shell's dirty-state mechanism). The arrived and gone-back
  // states carry no such weight, so the guard stands down there.
  function guard(on) {
    if (window.versoDirtyState) window.versoDirtyState.set("upgrading", !!on);
  }
  function leave() {
    if (window.versoDirtyState) window.versoDirtyState.suppress();
  }

  var current = "";
  function show(name) {
    if (!surfaces[name] || current === name) {
      guard(name === "preparing" || name === "installing" || name === "restarting");
      return;
    }
    current = name;
    for (var key in surfaces) {
      if (Object.prototype.hasOwnProperty.call(surfaces, key)) surfaces[key].hidden = key !== name;
    }
    guard(name === "preparing" || name === "installing" || name === "restarting");
  }

  // sawProgress is true from the first frame: the takeover only paints here while
  // the job is under way (preparing) or its install has spawned (installing).
  var sawProgress = initial === "preparing" || initial === "installing";
  var done = false;

  // The stuck deadline: the whole upgrade's budget, counted from the first screen.
  // Building the image on the update server is the long, variable part (minutes);
  // the flash and reboot that follow are short. If the entire run — build, flash,
  // reboot, reconnect — has not landed within this long, something has gone wrong
  // and the person would otherwise be held forever, so a calm surface opens a door
  // instead. Generous on purpose: a healthy upgrade finishes well inside it and
  // the door never appears; it matters only when the normal path never arrives.
  var MAX_STUCK_MS = 8 * 60 * 1000;
  var stuckTimer = null;

  // The countdown makes the bound visible: a quiet clock ticking to the moment
  // the deadline fires. It is shell chrome (a sibling of the surfaces), driven
  // here so the words stay server-localized and only the number moves. It ticks
  // locally, so it keeps counting even while the router is gone and unreachable.
  var countdownEl = root.querySelector("[data-verso-upgrading-countdown]");
  var countdownTimeEl = root.querySelector("[data-verso-upgrading-countdown-time]");
  var countdownTick = null;
  var stuckDeadline = 0;
  function fmtRemaining(ms) {
    var total = Math.max(0, Math.round(ms / 1000));
    var mins = Math.floor(total / 60);
    var secs = total % 60;
    return mins + ":" + (secs < 10 ? "0" : "") + secs;
  }
  function paintCountdown() {
    if (countdownTimeEl) countdownTimeEl.textContent = fmtRemaining(stuckDeadline - Date.now());
  }
  function stopCountdown() {
    if (countdownTick) { window.clearInterval(countdownTick); countdownTick = null; }
    if (countdownEl) countdownEl.hidden = true;
  }
  function armStuck() {
    if (stuckTimer || done) return;
    stuckDeadline = Date.now() + MAX_STUCK_MS;
    if (countdownEl) {
      paintCountdown();
      countdownEl.hidden = false;
      countdownTick = window.setInterval(paintCountdown, 1000);
    }
    stuckTimer = window.setTimeout(function () {
      if (done) return;
      // The stalled surface carries a real dismiss form; without it (a render
      // miss) there is nothing to swap to, so leave the current state standing.
      if (surfaces.stalled) finish("stalled");
    }, MAX_STUCK_MS);
  }

  show(initial);
  // The budget runs from the first working screen — preparing included — so the
  // countdown is continuous from the moment the takeover appears, not only once
  // the flash begins.
  if (sawProgress) armStuck();

  var timer = null;
  function schedule() {
    if (done) return;
    timer = window.setTimeout(poll, POLL_MS);
  }
  function finish(name) {
    show(name);
    done = true;
    if (timer) window.clearTimeout(timer);
    if (stuckTimer) window.clearTimeout(stuckTimer);
    stopCountdown();
  }

  // The router came back — either the status endpoint answered "idle" on the new
  // build, or, more often on real hardware, the session is gone and the poll met
  // a login redirect (reachable, but not JSON). Either way, reaching it again
  // after progress is the upgrade landing.
  function reachedAgain() {
    if (sawProgress) {
      finish("done");
    } else {
      leave();
      window.location.reload();
    }
  }

  function handle(data) {
    switch (data && data.state) {
      case "working":
        sawProgress = true;
        show("preparing");
        schedule();
        break;
      case "done":
        sawProgress = true;
        show("installing");
        armStuck();
        schedule();
        break;
      case "failed":
        // The server owns the failure surface and its wording; land on it by
        // reload. initial === "failed" then returns early, so no reload loop.
        leave();
        window.location.reload();
        break;
      case "idle":
        reachedAgain();
        break;
      default:
        schedule();
    }
  }

  function poll() {
    fetch(STATUS_URL, { headers: { Accept: "application/json" }, cache: "no-store" })
      .then(function (res) {
        var ct = res.headers.get("Content-Type") || "";
        if (res.ok && ct.indexOf("application/json") !== -1) {
          // Malformed JSON on an OK response is a transient hiccup, not the
          // router leaving — keep polling rather than declare the upgrade done.
          return res.json().then(handle, function () { schedule(); });
        }
        if (res.ok) {
          // Reachable and OK but not the status JSON: the login page after a
          // redirect — this browser is signed out because the router came back
          // on the new build. That is the upgrade landing.
          reachedAgain();
          return null;
        }
        // Reachable but erroring (a 5xx blip, a proxy hiccup): the router is
        // still here and still working, not gone. Keep waiting rather than
        // mistake a transient error for the upgrade finishing.
        schedule();
        return null;
      })
      .catch(function () {
        // Unreachable: the router is flashing and gone. That is success in
        // progress, not an error — show restarting and keep retrying.
        if (sawProgress) {
          show("restarting");
          armStuck();
        }
        schedule();
      });
  }

  schedule();
})();
