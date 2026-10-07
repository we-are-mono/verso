// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// System pages use the shell's authenticated routes, table filters and drawers.
// The router's log is a live console like the firewall's, and lives with it
// (verso-listing.js).
(function () {
  "use strict";
  // These native forms navigate after starting background work. Lock them for
  // the initial request too, before the server can render its busy state.
  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!form.matches('form[action^="/system/maintenance/updates/"]')) return;
    var button = event.submitter || form.querySelector('button[type="submit"]');
    versoButtons.submit(event, button && button.getAttribute("data-busy-label") || T("Installing…"));
  });

  // The web interface's Apply says what it is about to do: a change to the
  // port this page came in on — or the redirect turned on while it is on
  // HTTP — sends the page to the new address, so the act reads as the
  // redirect it is (ADR-017 §1).
  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!form.elements || !form.elements.web_https) return;
    var changed = function (name) {
      var field = form.elements[name];
      if (!field) return false;
      return field.type === "checkbox" ? field.checked !== field.defaultChecked : field.value.trim() !== field.defaultValue;
    };
    var moves = window.location.protocol === "https:"
      ? changed("web_https")
      : changed("web_http") || (changed("web_redirect") && form.elements.web_redirect.checked);
    versoButtons.submit(event, moves ? T("Redirecting…") : T("Applying…"));
  });

  // A check or an install under way draws its act busy; while one does, the
  // page waits for the work to end and reads the router again.
  if (document.querySelector('form[action^="/system/maintenance/updates/"] [aria-busy="true"]')) {
    var poll = setInterval(function () {
      fetch("/system/packages/status", { credentials: "same-origin" }).then(function (res) { if (!res.ok || res.redirected) throw new Error(); return res.json(); }).then(function (status) {
        if (!status.refreshing && !status.checking) { clearInterval(poll); window.location.reload(); }
      }).catch(function () { clearInterval(poll); });
    }, 2000);
    window.addEventListener("pagehide", function () { clearInterval(poll); });
  }
  // Factory reset waits for the hostname, typed as its box shows it.
  var reset = document.querySelector('form[action="/system/maintenance/factory-reset"]');
  if (reset) {
    var host = reset.querySelector('[name="hostname"]'), button = reset.querySelector('[type="submit"]');
    function gate() { button.disabled = !!host && host.value !== host.placeholder; }
    if (host) host.addEventListener("input", gate); gate();
  }
  // A reboot with changes staged applies or discards them first. A refusal is
  // said in a line under the acts, made the first time there is one to say.
  var reboot = document.querySelector('form[action="/system/maintenance/restart"]');
  if (reboot) reboot.addEventListener("submit", async function (event) {
    var submitter = event.submitter;
    if (!submitter || submitter.name !== "stage" || !submitter.value) return;
    event.preventDefault();
    var error = reboot.querySelector("[data-verso-reboot-error]");
    if (!error) {
      error = document.createElement("p");
      error.setAttribute("data-verso-reboot-error", "");
      error.setAttribute("role", "alert");
      error.className = "w-full flex items-start gap-2 text-sm leading-5 text-crimson-deep";
      reboot.appendChild(error);
    }
    error.hidden = true;
    var buttons = reboot.querySelectorAll("button"); buttons.forEach(function (b) { b.disabled = true; });
    var csrf = reboot.querySelector('[name="_csrf"]').value;
    async function post(url) {
      var res = await versoPost(url, new URLSearchParams({ _csrf: csrf }));
      if (!res.ok || res.redirected) throw new Error(T("The operation could not be completed. Review staged changes before retrying."));
    }
    try {
      if (submitter.value === "apply") {
        await post("/uci/apply");
        // Confirm only after this browser has reached the router again.
        await versoConfirmApply(function () { return post("/uci/confirm"); }, Date.now());
      } else { await post("/uci/discard"); }
      reboot.submit();
    } catch (e) { versoErrorLine(error, e.message); error.hidden = false; buttons.forEach(function (b) { b.disabled = false; }); }
  });
})();
