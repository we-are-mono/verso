// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Diagnostics (diagnostics.html.tmpl): the row on the heading line starts a
// run on the router, and its output is read into the sand under it as the
// router writes it, until the stream says how the run ended. Every word the
// page shows is the server's or the router's: the run's own lines, and the
// ending the stream states in the page's language.
(function () {
  "use strict";
  var form = document.querySelector("[data-verso-diag-form]");
  var output = document.querySelector("[data-verso-diag-output]");
  if (!form || !output || !window.EventSource) return;
  var tool = form.elements.tool, target = form.elements.target, via = form.elements.interface;
  var run = form.querySelector("[data-verso-diag-run]");
  var stop = form.querySelector("[data-verso-diag-stop]");
  var error = form.querySelector("[data-verso-diag-error]");
  var waiting = stop.querySelector("[data-verso-wait]");
  var glyphs = form.querySelectorAll("[data-verso-diag-glyph]");
  var recent = form.querySelector("[data-verso-diag-recent]");
  var el = versoEl;
  var job = "", stream = null;
  // The stream reads each line into tokens (diagnostics.go); they wear the
  // code block's inks (code.html.tmpl), so a run reads as Verso's code does.
  var INKS = { value: "text-green-deep", keyword: "text-denim-deep", meta: "text-meta" };

  function running() { return job !== ""; }
  // Run rests until there is something to reach for; while a run goes, Stop
  // stands in its place.
  function sync() {
    run.hidden = running(); stop.hidden = !running();
    run.disabled = target.value.trim() === "";
    if (!running()) {
      stop.disabled = false;
      if (waiting) waiting.removeAttribute("data-verso-wait-paused");
    }
    // A DNS lookup asks the resolver, which no interface binds; a disabled
    // choice is not sent.
    via.disabled = tool.value === "nslookup";
    glyphs.forEach(function (glyph) { glyph.hidden = glyph.dataset.versoDiagGlyph !== tool.value; });
    // "any interface" is words; a picked interface is its name, verbatim.
    var named = via.value !== "";
    via.classList.toggle("font-mono", named); via.classList.toggle("text-base", named);
    via.classList.toggle("font-sans", !named); via.classList.toggle("text-sm", !named);
  }

  // What this browser reached for before comes back as the target's
  // suggestions, newest first. It is a convenience of this browser alone, so
  // storage that is missing or refused only leaves the list empty.
  var RECENT = "verso.diagnostics.targets";
  function remembered() {
    try {
      var list = JSON.parse(localStorage.getItem(RECENT) || "[]");
      return Array.isArray(list) ? list.filter(function (t) { return typeof t === "string"; }) : [];
    } catch (_) { return []; }
  }
  function offer(list) {
    if (recent) recent.replaceChildren.apply(recent, list.map(function (t) { var o = document.createElement("option"); o.value = t; return o; }));
  }
  function remember(value) {
    var list = [value].concat(remembered().filter(function (t) { return t !== value; })).slice(0, 8);
    try { localStorage.setItem(RECENT, JSON.stringify(list)); } catch (_) { /* this browser keeps nothing */ }
    offer(list);
  }
  // A refusal is said on the line under the row. The row is not a settings
  // form, so it marks no field refused: the page's refusal navigator counts
  // settings to correct before a save, and a run saves nothing.
  function refuse(message) {
    if (!message) { error.hidden = true; return; }
    versoErrorLine(error, message); error.hidden = false;
  }
  function pinned() { return output.scrollHeight - output.scrollTop - output.clientHeight < 32; }
  function show(nodes) {
    var stay = pinned();
    nodes.forEach(function (node) { output.appendChild(node); });
    if (stay) output.scrollTop = output.scrollHeight;
  }
  function finish(summary, tone) {
    if (stream) { stream.close(); stream = null; }
    if (summary) {
      var colour = tone === "danger" ? "text-crimson-deep" : tone === "warning" ? "text-marigold-deep" : "text-meta";
      show([el("p", "px-10 pt-2 text-sm " + colour, summary)]);
    }
    job = ""; sync();
  }

  function read(id) {
    stream = new EventSource("/system/diagnostics/run?job=" + encodeURIComponent(id));
    stream.addEventListener("output", function (event) {
      var frame;
      try { frame = JSON.parse(event.data); } catch (_) { return; }
      show((frame.lines || []).map(function (tokens) {
        var line = el("div", "px-10 font-mono text-base font-medium leading-6 whitespace-pre-wrap wrap-anywhere text-ink");
        tokens.forEach(function (token) {
          line.appendChild(token[0] ? el("span", INKS[token[0]], token[1]) : document.createTextNode(token[1]));
        });
        return line;
      }));
      if (frame.done) finish(frame.summary, frame.tone);
    });
    // A stream the server closed without an ending (the sign-in ran out) is
    // not reopened: the run is over as far as this page can tell.
    stream.addEventListener("error", function () {
      if (stream && stream.readyState === EventSource.CLOSED) finish("", "");
    });
  }

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    if (running() || run.disabled) return;
    refuse("");
    run.disabled = true;
    var res;
    try { res = await versoPost("/system/diagnostics/run", versoBody(new FormData(form))); } catch (_) { res = null; }
    if (res && res.redirected) { window.location.reload(); return; }
    var answer = {};
    try { answer = res ? await res.json() : {}; } catch (_) { /* no words: the generic refusal below */ }
    if (!res || !res.ok || !answer.job) {
      refuse(answer.error || T("The router could not start the run."));
      sync();
      return;
    }
    // The program's own first line says what it is doing; the run needs no
    // head of the page's.
    output.replaceChildren();
    remember(target.value.trim());
    job = answer.job; sync();
    read(job);
  });
  // Stop is asked once; its mark holds still until the run's ending lands.
  stop.addEventListener("click", function () {
    if (!running() || stop.disabled) return;
    stop.disabled = true;
    if (waiting) waiting.setAttribute("data-verso-wait-paused", "");
    versoPost("/system/diagnostics/stop", versoBody({ job: job }));
  });
  target.addEventListener("input", function () { refuse(""); sync(); });
  tool.addEventListener("change", function () { form.setAttribute("data-verso-diag-picked", ""); sync(); });
  via.addEventListener("change", sync);
  // Leaving the page ends the run it started rather than leave it running on
  // the router with no one reading it.
  window.addEventListener("pagehide", function () {
    if (!running()) return;
    if (stream) stream.close();
    navigator.sendBeacon("/system/diagnostics/stop", versoBody({ job: job }));
  });
  offer(remembered());
  sync();
  // The page is here to be asked something: with a keyboard at hand, the
  // caret waits in the target. A touch screen keeps its keyboard down until
  // the reader taps.
  if (window.matchMedia("(pointer: fine)").matches && document.activeElement === document.body) target.focus();
})();
