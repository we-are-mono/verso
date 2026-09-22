// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// System pages use the shell's authenticated routes, table filters and drawers.
(function () {
  "use strict";
  var log = document.querySelector("[data-verso-system-log]");
  if (log && window.EventSource) {
    var body = log.querySelector("[data-verso-console-rows]");
    var source = log.querySelector("[data-verso-listing-select]");
    var state = log.querySelector("[data-log-state]");
    var mark = log.querySelector("[data-log-state-mark]");
    var pause = log.querySelector("[data-log-pause]");
    var paused = false, connected = false, unreadable = false, pendingReset = false, pending = [], seen = new Set(), sources = new Set();
    var includeFirewall = log.querySelector("[data-log-include-firewall]");
    var es, firewallUnavailable = false;
    function status() {
      state.textContent = paused ? T("Paused") : unreadable ? T("Logs unavailable") : firewallUnavailable ? T("Firewall logs unavailable") : connected ? T("Live") : T("Connecting…");
      mark.classList.toggle("bg-green", connected && !paused && !unreadable && !firewallUnavailable);
      mark.classList.toggle("border", !connected || paused || unreadable || firewallUnavailable);
      pause.textContent = paused ? T("Resume") : T("Pause");
    }
    function node(tag, cls, text) {
      var n = document.createElement(tag); n.className = cls;
      if (text !== undefined) n.textContent = text;
      return n;
    }
    function draw(rows) {
      var pinned = body.scrollHeight - body.scrollTop - body.clientHeight < 32;
      var fragment = document.createDocumentFragment();
      rows.forEach(function (row) {
        if (seen.has(row.id)) return;
        seen.add(row.id);
        var error = ["emerg", "alert", "crit", "err"].indexOf(row.severity) !== -1;
        var warning = error || row.severity === "warn";
        var line = node("div", "grid grid-cols-[0.1875rem_4rem_minmax(0,1fr)_4rem] items-stretch gap-x-3 py-1 pr-10 pl-6.25 leading-6 hover:bg-mid/50 lg:flex lg:py-px");
        line.setAttribute("data-log-row", String(row.id));
        line.dataset.logAt = String(row.at);
        line.setAttribute("data-verso-tags", error ? "errors warnings" : warning ? "warnings" : "");
        line.setAttribute("data-verso-facet-source", row.source);
        line.appendChild(node("span", "row-span-2 my-0.5 w-0.75 shrink-0 rounded-full " + (error ? "bg-crimson" : warning ? "bg-marigold" : "bg-transparent")));
        var at = new Date(row.at * 1000);
        var clock = [at.getHours(), at.getMinutes(), at.getSeconds()].map(function (n) { return String(n).padStart(2, "0"); }).join(":");
        var stamp = node("time", "shrink-0 font-mono lg:w-20 text-base font-medium text-body", clock);
        stamp.dateTime = at.toISOString(); stamp.title = at.toLocaleString(); line.appendChild(stamp);
        var origin = node("span", "min-w-0 truncate font-mono lg:w-34 lg:shrink-0 text-base font-medium text-ink", row.source); origin.title = row.source; line.appendChild(origin);
        line.appendChild(node("span", "w-16 shrink-0 font-mono text-base " + (error ? "font-bold text-crimson-deep" : warning ? "font-medium text-marigold-deep" : "font-medium text-meta"), row.severity));
        line.appendChild(node("span", "col-span-3 col-start-2 min-w-0 flex-1 wrap-anywhere font-mono text-base font-medium " + (error ? "text-crimson-deep" : "text-ink"), row.message));
        line.dataset.logText = at.toISOString() + " " + row.source + " " + row.severity + " " + row.message;
        fragment.appendChild(line);
        if (!sources.has(row.source)) {
          sources.add(row.source); var opt = document.createElement("option"); opt.value = row.source; opt.textContent = row.source; source.appendChild(opt);
        }
      });
      body.appendChild(fragment);
      var lines = Array.from(body.querySelectorAll("[data-log-row]"));
      // Separate buffers are sampled at different instants. A delayed batch
      // still belongs beside its timestamp, not below a newer service event.
      if (includeFirewall.checked && rows.length) {
        lines.sort(function (a, b) { return Number(a.dataset.logAt) - Number(b.dataset.logAt); });
        lines.forEach(function (line) { body.appendChild(line); });
      }
      for (var i = 0; i < lines.length - 1000; i++) { seen.delete(Number(lines[i].getAttribute("data-log-row"))); lines[i].remove(); }
      var empty = body.querySelector("[data-verso-stream-empty]");
      if (empty) { empty.textContent = T("Nothing matches."); empty.hidden = body.querySelectorAll("[data-log-row]").length > 0; }
      // The shared filter reacts to the inserted rows before this frame paints.
      requestAnimationFrame(function () { if (pinned) body.scrollTop = body.scrollHeight; });
    }
    function clearRows() {
      seen.clear();
      body.querySelectorAll("[data-log-row]").forEach(function (n) { n.remove(); });
    }
    function connect() {
      es = new EventSource("/streams/system-log" + (includeFirewall.checked ? "?firewall=1" : ""));
      es.addEventListener("stream", function (event) {
        var frame;
        try { frame = JSON.parse(event.data); } catch (_) { return; }
        if (!frame || !Array.isArray(frame.rows)) return;
        unreadable = false; firewallUnavailable = !!frame.firewall_unavailable; connected = true; status();
        if (frame.reset) {
          pending = [];
          if (paused) pendingReset = true;
          else clearRows();
        }
        if (paused) { pending = pending.concat(frame.rows).slice(-1000); return; }
        draw(frame.rows);
      });
      es.addEventListener("open", function () { connected = true; status(); });
      es.addEventListener("error", function () { connected = false; status(); });
      es.addEventListener("unavailable", function () { unreadable = true; status(); });
    }
    connect();
    includeFirewall.addEventListener("change", function () {
      es.close();
      clearRows(); pending = []; pendingReset = false;
      sources.clear(); source.replaceChildren(new Option(T("Every source"), ""));
      source.dispatchEvent(new Event("change", { bubbles: true }));
      connected = false; unreadable = false; firewallUnavailable = false; status();
      var url = new URL(window.location.href);
      if (includeFirewall.checked) url.searchParams.set("firewall", "1");
      else url.searchParams.delete("firewall");
      window.history.replaceState(null, "", url);
      connect();
    });
    pause.addEventListener("click", function () {
      paused = !paused;
      if (!paused) {
        if (pendingReset) { clearRows(); pendingReset = false; }
        draw(pending); pending = [];
      }
      status();
    });
    log.querySelector("[data-log-download]").addEventListener("click", function () {
      var text = Array.from(body.querySelectorAll("[data-log-row]")).filter(function (n) { return !n.hidden; }).map(function (n) { return n.dataset.logText; }).join("\n");
      var url = URL.createObjectURL(new Blob([text + "\n"], { type: "text/plain;charset=utf-8" }));
      var a = document.createElement("a"); a.href = url; a.download = "router-log.txt"; a.click(); setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
    });
    window.addEventListener("pagehide", function () { es.close(); });
  }

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
      var res = await fetch(url, { method: "POST", credentials: "same-origin", body: new URLSearchParams({ _csrf: csrf }) });
      if (!res.ok || res.redirected) throw new Error(T("The operation could not be completed. Review staged changes before retrying."));
    }
    try {
      if (submitter.value === "apply") {
        await post("/uci/apply");
        // Confirm only after this browser has reached the router again.
        var deadline = Date.now() + 28000;
        while (true) {
          try { await post("/uci/confirm"); break; }
          catch (e) { if (Date.now() >= deadline) throw e; await new Promise(function (resolve) { setTimeout(resolve, 500); }); }
        }
      } else { await post("/uci/discard"); }
      reboot.submit();
    } catch (e) { error.textContent = e.message; error.hidden = false; buttons.forEach(function (b) { b.disabled = false; }); }
  });
})();
