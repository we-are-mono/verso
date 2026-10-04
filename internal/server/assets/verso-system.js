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

  if (document.querySelector("[data-verso-update-busy]")) {
    var poll = setInterval(function () {
      fetch("/system/packages/status", { credentials: "same-origin" }).then(function (res) { if (!res.ok || res.redirected) throw new Error(); return res.json(); }).then(function (status) {
        if (!status.refreshing && !status.checking) { clearInterval(poll); window.location.reload(); }
      }).catch(function () { clearInterval(poll); });
    }, 2000);
    window.addEventListener("pagehide", function () { clearInterval(poll); });
  }
  var reset = document.querySelector("[data-verso-reset]");
  if (reset) {
    var host = reset.querySelector('[name="hostname"]'), button = reset.querySelector('[type="submit"]');
    function gate() { button.disabled = !!host && host.value !== reset.dataset.hostname; }
    if (host) host.addEventListener("input", gate); gate();
  }
  var reboot = document.querySelector("[data-verso-reboot]");
  if (reboot) reboot.addEventListener("submit", async function (event) {
    var submitter = event.submitter;
    if (!submitter || !submitter.value) return;
    event.preventDefault();
    var error = reboot.querySelector("[data-verso-reboot-error]"); error.hidden = true;
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
