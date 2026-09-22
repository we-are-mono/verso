// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-listing.js — the live listing (ADR-004, ADR-005 §7). A table declares
// that its rows arrive over time and names its source; everything here is the
// shell's: the transport, the ring, the pause, the consecutive collapse, the rows
// themselves, and the shelf of plucked values that filters them.

// The wire carries data, never markup — a log line's contents are whatever a
// stranger sent this router — so every row is built with createElement and
// textContent, and the only markup cloned is the shell's own endpoint glyphs
// from the <template> the renderer left beside the table.
//
// The filtering is the stream's own values: click a verdict, a zone, an address,
// and what was clicked travels to a shelf above the table wearing exactly the
// treatment it had in the row. A second click, the shelf's ×, or Clear lets go;
// with nothing held there is no shelf at all. The page-wide lens (the
// still-lens) is the other half and the two compose — a plucked value hides
// what does not match, the lens dims it.
(function () {
  if (!window.EventSource) return;
  var tables = document.querySelectorAll("[data-verso-stream]");
  if (!tables.length) return;

  // The td treatments, copied from table.html.tmpl's own column kinds so a
  // streamed row is indistinguishable from a rendered one. The wrapper's
  // condensed/lined variants reach these through descendant selectors, so a row
  // built here inherits them for free.
  var CELL = {
    runtime:
      "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 align-middle whitespace-nowrap tabular-nums text-meta group-last:border-b-0",
    num: "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 text-right align-middle tabular-nums text-ink group-last:border-b-0",
    pill: "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 align-middle whitespace-nowrap group-last:border-b-0",
    endpoint: "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 align-middle group-last:border-b-0",
    keyword:
      "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 align-middle whitespace-nowrap font-mono text-base font-medium text-body group-last:border-b-0",
    link: "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 text-right align-middle whitespace-nowrap group-last:border-b-0",
    mono: "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 align-middle font-mono text-sm tabular-nums whitespace-nowrap text-ink group-last:border-b-0",
    monoEmphasis:
      "border-b border-rule px-3.5 py-2.5 leading-6 first:pl-0 last:pr-0 align-middle font-mono text-base font-medium tabular-nums whitespace-nowrap text-ink group-last:border-b-0",
  };
  // The badge vocabulary, from badge.html.tmpl. The plugin names a verdict; the
  // shell alone decides what colour a verdict wears.
  var PILL_BASE =
    "inline-flex items-center gap-1.5 rounded-xs border px-1.5 py-0.5 leading-4 whitespace-nowrap text-sm font-medium font-mono ";
  var PILL_TONE = {
    success: "border-green-line bg-green-soft text-green-deep",
    warning: "border-marigold-line bg-marigold-soft text-marigold-deep",
    danger: "border-crimson-line bg-crimson-soft text-crimson-deep",
    neutral: "border-rule-strong bg-mid text-ink",
  };
  // What a verdict means in a log is not what it means in the config. There, a
  // reject is the refusal you wrote and a drop is the silence you chose. Here,
  // every line is something that already happened to real traffic, and the
  // loudest of them is the one that vanished without an answer.
  var VERDICT_TONE = { accept: "success", reject: "warning", drop: "danger" };
  // The mark down a console line's left edge: the hue at full chroma, which is
  // what a mark is for.
  var VERDICT_MARK = { accept: "bg-green", reject: "bg-marigold", drop: "bg-crimson" };
  var VERDICT_INK = {
    accept: "text-green-deep",
    reject: "text-marigold-deep",
    drop: "text-crimson-deep",
  };
  var ENDPOINT_BASE =
    "inline-flex items-center gap-1.5 rounded-xs border px-1.5 py-0.5 leading-4 whitespace-nowrap ";
  var ENDPOINT_KIND = {
    router: "border-denim-line bg-denim-soft text-xs font-medium text-denim-deep",
    device: "border-transparent font-mono text-base font-medium tabular-nums text-ink",
    zone: "border-transparent font-mono text-base font-medium tabular-nums text-ink",
  };
  // One console line. Fixed columns so the eye reads down one without a rule to
  // guide it; everything in mono at the reading size, because every value on
  // the line is a machine string.
  var CONSOLE = {
    row: "group flex cursor-pointer items-stretch gap-3 py-px pr-11 pl-[29px] leading-6 hover:bg-mid/50",
    mark: "my-0.5 w-[3px] shrink-0 rounded-full ",
    time: "w-18 shrink-0 font-mono text-base font-medium text-body",
    verdict: "w-22 shrink-0 font-mono text-base font-medium ",
    path: "flex w-66 shrink-0 items-baseline gap-2 font-mono text-base font-medium",
    arrow: "flex w-6 shrink-0 justify-center text-faint",
    proto: "flex w-24 shrink-0 items-baseline gap-1.5 font-mono text-base font-medium text-body",
    rule: "flex min-w-0 flex-1 items-baseline gap-2 pr-3",
    zone: "shrink-0 text-meta",
    addr: "min-w-0 truncate",
    port: "shrink-0 text-glyph",
    svc: "shrink-0 text-faint",
    flag: "text-faint",
    count: "shrink-0 font-mono text-sm text-meta",
  };
  var EM_DASH = "—";
  var TIMES = "×";
  // SAFE_HREF is the whole URL policy for a destination that arrived over the
  // wire: a path inside this app, and demonstrably nothing else. It enforces,
  // exactly — the string starts with "/"; its second character is neither "/"
  // nor "\" (either would make it a host, since browsers read a backslash as a
  // slash in a special-scheme URL, so "/\evil.example" is protocol-relative);
  // it contains no backslash anywhere; and it contains no character below
  // U+0020 (tab, newline and carriage return are stripped *before* a URL is
  // parsed, so "/\t/evil.example" would become "//evil.example").
  var SAFE_HREF = /^\/(?![/\\])[^\\\x00-\x1f]*$/;
  // How much of the recent past the rate is measured over, and how often the
  // relative times age. Both are the page's own clock, and neither runs while
  // the stream is paused.
  var RATE_WINDOW_MS = 10000;
  var TICK_MS = 5000;

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined && text !== null) node.textContent = text;
    return node;
  }

  // relative states when an event happened in the words a person uses for the
  // last few minutes. Anything older is the wrong tool anyway — the ring does
  // not hold that much.
  function relative(age) {
    if (age < 5) return T("now");
    if (age < 60) return T("%d s").replace("%d", age);
    if (age < 3600) return T("%d min").replace("%d", Math.floor(age / 60));
    return T("%d h").replace("%d", Math.floor(age / 3600));
  }

  // clock is the wall time a console line carries — the stamp the device wrote,
  // in the browser's own locale, so a line can be matched against any other log
  // on the machine.
  function clock(at) {
    var d = new Date(at * 1000);
    return (
      String(d.getHours()).padStart(2, "0") +
      ":" +
      String(d.getMinutes()).padStart(2, "0") +
      ":" +
      String(d.getSeconds()).padStart(2, "0")
    );
  }

  [].forEach.call(tables, function (table) {
    start(table);
  });

  function start(table) {
    var source = table.getAttribute("data-verso-stream");
    if (!source) return;
    var ring = parseInt(table.getAttribute("data-verso-stream-ring"), 10) || 200;
    // A console draws its own lines into a plain scrolling block; a grid draws
    // rows into a tbody. Everything after this — the ring, the pause, the
    // pluck, the collapse — is the same either way.
    var console = table.getAttribute("data-verso-stream-style") === "console";
    var body = console ? table.querySelector("[data-verso-console-rows]") : table.tBodies && table.tBodies[0];
    if (!body) return;
    var wrapper = console ? table : table.parentElement;
    var section = table.closest("section");
    var meta = section && section.querySelector("[data-verso-section-meta]");
    var metaRest = meta ? meta.textContent : "";
    var pause = (section || document).querySelector("button[data-verso-live]");
    var pauseLabel = pause && pause.querySelector("[data-verso-live-label]");
    var waiting = pause && pause.querySelector("[data-verso-wait]");
    var icons = wrapper && wrapper.querySelector("template[data-verso-stream-icons]");
    var lens = document.querySelector("[data-verso-filter]");

    // rows is the ring, newest first; each remembers the tuple it collapses on
    // and the values it can be narrowed by.
    var empty = body.querySelector("[data-verso-stream-empty]");
    var emptyTemplate = empty && empty.cloneNode(true);
    var rows = [];
    var arrivals = []; // arrival stamps, for the rolling rate
    var paused = false;
    var buffer = [];
    var pending = 0; // events that arrived while paused — every one of them
    var held = []; // the plucked values, in the order they were picked up
    var shelf = null;
    // The first frame is the backlog: what the device had already logged before
    // this page opened. Everything after it is the present arriving.
    var backlog = true;
    // Whether the stream is up. Optimistic at load — the connection is being
    // made — and corrected by the transport's own open/error from then on.
    var connected = false;
    var lost = false;
    var pendingReset = false;
    var health = el("p", "border-b border-rule-strong bg-marigold-soft px-10 py-3 text-sm text-marigold-deep");
    health.setAttribute("role", "status");
    health.hidden = true;
    wrapper.parentNode.insertBefore(health, wrapper);
    function updateHealth(available) {
      connected = available;
      health.hidden = available && !lost;
      health.textContent = available ? T("Some firewall events were lost.") : T("Firewall logs unavailable");
      updatePause();
    }
    function clearHistory() {
      rows.forEach(function (row) { row.tr.remove(); });
      rows = []; arrivals = []; backlog = true;
      if (emptyTemplate && !body.querySelector("[data-verso-stream-empty]")) body.appendChild(emptyTemplate.cloneNode(true));
    }
    // The device stamps its own events and the browser reads them; the two
    // clocks need not agree. An event that arrives *live* is by definition
    // happening now, which is exactly the offset between them — so ages are
    // measured against the device's clock rather than against a disagreement.
    //
    // Only live frames set it. The backlog is history: taking its newest row as
    // "now" would relabel a ten-minute-old verdict as this second, which on a
    // quiet network is the whole page. The cost is the other way round — until
    // the first live event, a badly-skewed device clock mis-ages the backlog by
    // exactly its own error, which is at least the device's own account of it.
    var skew = 0;

    function ageOf(at) {
      return Math.max(0, Math.round(Date.now() / 1000 - skew - at));
    }

    function icon(kind) {
      if (!icons) return null;
      var holder = icons.content.querySelector('[data-verso-stream-icon="' + kind + '"]');
      var svg = holder && holder.firstElementChild;
      return svg ? svg.cloneNode(true) : null;
    }

    // pluckable wraps a value in the control it already is. The button carries
    // no treatment of its own: what is inside keeps exactly the look it has in
    // the row, because that is the thing that travels to the shelf.
    // Its name says what pressing it does, and aria-pressed whether it is one
    // of the values being held: the value alone ("ACCEPT") names neither.
    function pluckable(field, value, inner) {
      var button = el("button", "verso-pluck");
      button.type = "button";
      button.setAttribute("data-verso-pluck", field);
      button.setAttribute("data-verso-pluck-value", value);
      button.setAttribute("aria-label", T("Show only %s").replace("%s", value));
      button.setAttribute("aria-pressed", isHeld(field, value) ? "true" : "false");
      button.appendChild(inner);
      return button;
    }

    function isHeld(field, value) {
      for (var i = 0; i < held.length; i++) {
        if (held[i].field === field && held[i].value === value) return true;
      }
      return false;
    }

    function pillFor(verdict) {
      return el("span", PILL_BASE + (PILL_TONE[VERDICT_TONE[verdict]] || PILL_TONE.neutral), verdict);
    }

    function endpointFor(kind, label) {
      var span = el("span", ENDPOINT_BASE + (ENDPOINT_KIND[kind] || ENDPOINT_KIND.zone));
      var glyph = icon(kind);
      if (glyph) {
        // The glyph's ink is the endpoint's kind, exactly as table.html.tmpl sets
        // it: the router in the action colour, "anywhere" at the faintest step,
        // anything else the ordinary glyph grey.
        var slot = el("span", "shrink-0 " + (kind === "router" ? "text-denim" : kind === "any" ? "text-inert" : "text-faint"));
        slot.appendChild(glyph);
        span.appendChild(slot);
      }
      span.appendChild(document.createTextNode(label));
      return span;
    }

    function cell(kind, child) {
      var td = el("td", CELL[kind]);
      if (child) td.appendChild(child);
      return td;
    }

    // endpoint is one end of a path on a console line: where it is, what it is,
    // which port, and the service that port is usually. Each part is its own
    // step of ink, so the address reads first and the rest sits behind it.
    function consoleEnd(zone, addr, port, ink) {
      var span = el("div", CONSOLE.path);
      if (zone) span.appendChild(pluckable("from", zone, el("span", CONSOLE.zone, zone)));
      if (addr) span.appendChild(pluckable("src", addr, el("span", CONSOLE.addr + (ink ? " " + ink : ""), addr)));
      if (port) span.appendChild(el("span", CONSOLE.port, port));
      return span;
    }

    // buildConsole renders one event as a line of the log: a mark in the
    // verdict's own hue down the left edge, then the fixed columns. No cells,
    // no borders — a terminal does not draw a grid around what it prints.
    function buildConsole(ev) {
      var row = el("div", CONSOLE.row);
      // The cuts the bar above narrows by. The stream says which they are; this
      // only carries them, the way a rendered row carries the same attribute.
      if (ev.tags && ev.tags.length) row.setAttribute("data-verso-tags", ev.tags.join(" "));
      row.appendChild(el("span", CONSOLE.mark + (VERDICT_MARK[ev.verdict] || "bg-glyph")));
      row.appendChild(el("div", CONSOLE.time));
      var verdict = el("div", CONSOLE.verdict + (VERDICT_INK[ev.verdict] || "text-ink"));
      verdict.appendChild(pluckable("verdict", ev.verdict || "", document.createTextNode(ev.verdict || "")));
      row.appendChild(verdict);
      // Traffic that was stopped is named in the colour of the stopping, so a
      // run of blocked lines reads as one shape down the page.
      var blocked = ev.verdict && ev.verdict !== "accept";
      var port = function (p) {
        return p ? ":" + p : "";
      };
      row.appendChild(consoleEnd(ev.from, ev.src, port(ev.sport), blocked ? VERDICT_INK[ev.verdict] : ""));
      row.appendChild(el("div", CONSOLE.arrow, "→"));
      row.appendChild(consoleEnd(ev.to_zone || (ev.to_kind === "router" ? "router" : ""), ev.to, port(ev.port), ""));
      row.appendChild(el("div", CONSOLE.proto, ev.proto || ""));
      var rule = el("div", CONSOLE.rule);
      if (ev.rule && typeof ev.rule_href === "string" && SAFE_HREF.test(ev.rule_href)) {
        var link = el("a", "min-w-0 truncate text-sm text-ink transition-colors hover:text-denim-deep", ev.rule);
        link.setAttribute("href", ev.rule_href);
        rule.appendChild(link);
      } else if (ev.rule) {
        rule.appendChild(el("span", "min-w-0 truncate text-sm text-body", ev.rule));
      }
      rule.appendChild(el("span", CONSOLE.count));
      row.appendChild(rule);
      return row;
    }

    // build renders one event as the row the rendered table would have drawn.
    function build(ev) {
      if (console) return buildConsole(ev);
      var tr = el("tr", "group verso-stream-row");
      tr.appendChild(cell("runtime"));
      tr.appendChild(cell("num"));

      var verdict = cell("pill");
      if (ev.verdict) {
        verdict.appendChild(pluckable("verdict", ev.verdict, pillFor(ev.verdict)));
      } else {
        verdict.appendChild(el("span", "text-inert", EM_DASH));
      }
      tr.appendChild(verdict);

      var from = cell("endpoint");
      if (ev.from) {
        from.appendChild(pluckable("from", ev.from, endpointFor(ev.from === "router" ? "router" : "zone", ev.from)));
      }
      tr.appendChild(from);

      var src = el("td", CELL.monoEmphasis);
      if (ev.src) src.appendChild(pluckable("src", ev.src, el("span", null, ev.src)));
      tr.appendChild(src);

      var to = cell("endpoint");
      if (ev.to) to.appendChild(endpointFor(ev.to_kind === "router" ? "router" : "device", ev.to));
      tr.appendChild(to);

      tr.appendChild(cell("keyword", document.createTextNode(ev.proto || "")));
      tr.appendChild(cell("mono", document.createTextNode(ev.port || EM_DASH)));

      var rule = cell("link");
      // The same URL policy the shell applies to a rendered link applies here,
      // and tighter — see SAFE_HREF. Nothing on this row is trusted enough to
      // be anything but a path inside this app.
      if (ev.rule && typeof ev.rule_href === "string" && SAFE_HREF.test(ev.rule_href)) {
        // A cell's link wears what a cell's link wears (table.html.tmpl): the
        // value in ink, going to the action colour under the pointer. It is the
        // name of a rule, not a piece of chrome.
        var link = el("a", "text-sm font-semibold text-ink transition-colors hover:text-denim-deep", ev.rule);
        link.setAttribute("href", ev.rule_href);
        rule.appendChild(link);
      } else if (ev.rule) {
        rule.appendChild(el("span", "text-sm text-meta", ev.rule));
      } else {
        rule.appendChild(el("span", "text-inert", EM_DASH));
      }
      tr.appendChild(rule);
      return tr;
    }

    // key is the tuple a repeat has to match exactly to be the same event
    // happening again: everything about the packet except when it happened.
    function key(ev) {
      return [ev.verdict, ev.from, ev.src, ev.to, ev.proto, ev.port, ev.rule].join(" ");
    }

    function paint(row) {
      if (console) {
        // A console prints the clock the device stamped, not how long ago it
        // was: a log is read down, and a column of "12 s" ages under the eye.
        row.tr.children[1].textContent = clock(row.at);
        row.tr.lastChild.lastChild.textContent = row.count > 1 ? TIMES + row.count : "";
        return;
      }
      row.tr.cells[0].textContent = relative(ageOf(row.at));
      row.tr.cells[1].textContent = row.count > 1 ? TIMES + " " + row.count : "";
    }

    // matches reports whether a row survives everything currently held. Two
    // held values of one field are alternatives; two different fields both have
    // to hold.
    function matches(row) {
      var fields = {};
      held.forEach(function (item) {
        if (!fields[item.field]) fields[item.field] = [];
        fields[item.field].push(item.value);
      });
      for (var field in fields) {
        if (!Object.prototype.hasOwnProperty.call(fields, field)) continue;
        if (fields[field].indexOf(row.values[field]) === -1) return false;
      }
      return true;
    }

    // held counts what the ring holds, in events rather than in rows: a row that
    // collapsed forty repeats is forty events, and saying "3 of 12" while the
    // listing shows a hundred packets would be counting two different things.
    // Both sides are the ring's own contents — never a lifetime total, which the
    // ring stopped holding the moment it first overflowed.
    function counts() {
      var shown = 0;
      var total = 0;
      rows.forEach(function (row) {
        total += row.count;
        if (!row.tr.hidden) shown += row.count;
      });
      return { shown: shown, total: total };
    }

    // Holding or letting go of a value changes what is on screen without moving
    // focus, so the count it leaves is said as well as shown.
    function applyHeld() {
      rows.forEach(function (row) {
        row.tr.hidden = !matches(row);
      });
      [].forEach.call(body.querySelectorAll("[data-verso-pluck]"), function (button) {
        button.setAttribute("aria-pressed", isHeld(button.getAttribute("data-verso-pluck"), button.getAttribute("data-verso-pluck-value")) ? "true" : "false");
      });
      renderShelf();
      updateMeta();
      var seen = counts();
      versoAnnounce(T("%d of %d events shown").replace("%d", seen.shown).replace("%d", seen.total));
    }

    function renderShelf() {
      if (!held.length) {
        if (shelf) {
          shelf.remove();
          shelf = null;
        }
        return;
      }
      if (!shelf) {
        shelf = el("div", "verso-stream-shelf");
        shelf.setAttribute("data-verso-stream-shelf", "");
        if (wrapper && wrapper.parentNode) wrapper.parentNode.insertBefore(shelf, wrapper);
      }
      while (shelf.firstChild) shelf.removeChild(shelf.firstChild);
      held.forEach(function (item, index) {
        var release = el("button", "verso-pluck verso-stream-release");
        release.type = "button";
        release.setAttribute("data-verso-stream-release", String(index));
        release.setAttribute("aria-label", T("Remove filter %s").replace("%s", item.value));
        release.appendChild(item.chip.cloneNode(true));
        release.appendChild(el("span", "verso-stream-release-x", TIMES));
        shelf.appendChild(release);
      });
      var clear = el("button", "verso-pluck text-sm font-medium text-meta hover:text-ink", T("Clear"));
      clear.type = "button";
      clear.setAttribute("data-verso-stream-clear", "");
      shelf.appendChild(clear);
    }

    function rate() {
      var cutoff = Date.now() - RATE_WINDOW_MS;
      while (arrivals.length && arrivals[0] < cutoff) arrivals.shift();
      return Math.round((arrivals.length / RATE_WINDOW_MS) * 1000);
    }

    // The section's meta states one thing at a time: what is being held back
    // while values are plucked, how fast events arrive while they are not, and
    // the listing's own sentence when nothing is flowing at all.
    function updateMeta() {
      // The rate is read on every pass, whatever the meta ends up saying: it is
      // what ages the arrival stamps out of the window they are measured over.
      var perSecond = rate();
      if (!meta) return;
      if (held.length) {
        var seen = counts();
        meta.textContent = T("%d of %d events shown").replace("%d", seen.shown).replace("%d", seen.total);
        return;
      }
      meta.textContent = perSecond > 0 ? T("~%d events/s").replace("%d", perSecond) : metaRest;
    }

    function updatePause() {
      if (!pause) return;
      var label = paused ? (pending ? T("Resume · %d new").replace("%d", pending) : T("Resume")) : T("Pause");
      if (pauseLabel) pauseLabel.textContent = label;
      else pause.textContent = label;
      // Move the waiting mark only while events can arrive.
      if (waiting) waiting.toggleAttribute("data-verso-wait-paused", paused || !connected);
    }

    // ingest folds one event in. A repeat of the newest row is that row
    // happening again: its counter climbs and its clock moves up, and nothing
    // else on the page moves. A repeat that is not consecutive starts a fresh
    // row, so the order of the list never lies about the order of events.
    // The rate and the skew are the frame's business, not this one's: an event
    // is counted when it arrives, which is once, whether it was rendered then or
    // held in the pause buffer and folded in later.
    function ingest(ev) {
      var k = key(ev);
      var newest = rows[0];
      if (newest && newest.key === k) {
        newest.count++;
        newest.at = ev.at;
        paint(newest);
        return;
      }
      var placeholder = body.querySelector("[data-verso-stream-empty]");
      if (placeholder) placeholder.remove();
      var row = {
        key: k,
        at: ev.at,
        count: 1,
        tr: build(ev),
        values: { verdict: ev.verdict, from: ev.from, src: ev.src },
      };
      paint(row);
      row.tr.hidden = !matches(row);
      body.insertBefore(row.tr, body.firstChild);
      rows.unshift(row);
      while (rows.length > ring) {
        var dropped = rows.pop();
        if (dropped.tr.parentNode) dropped.tr.remove();
      }
    }

    function drain() {
      if (pendingReset) { clearHistory(); pendingReset = false; }
      var waiting = buffer;
      buffer = [];
      pending = 0;
      waiting.forEach(ingest);
    }

    // The still-lens is the page's own control and owns its behaviour; a batch
    // of new rows simply asks it to look again, so the two never drift apart.
    function relens() {
      if (lens && lens.value) lens.dispatchEvent(new Event("input", { bubbles: true }));
    }

    if (pause) {
      updatePause();
      pause.addEventListener("click", function () {
        paused = !paused;
        if (!paused) {
          drain();
          applyHeld();
          relens();
        }
        updatePause();
      });
    }

    // One listener for every value in the stream: the row stays inert, its
    // values do not.
    body.addEventListener("click", function (event) {
      var target = event.target.closest && event.target.closest("[data-verso-pluck]");
      if (!target) return;
      var field = target.getAttribute("data-verso-pluck");
      var value = target.getAttribute("data-verso-pluck-value");
      for (var i = 0; i < held.length; i++) {
        if (held[i].field === field && held[i].value === value) {
          held.splice(i, 1); // clicking it again lets it go
          applyHeld();
          return;
        }
      }
      // What travels to the shelf is what the value looks like in the row; a
      // value drawn as bare text (the console's verdict) travels as its text.
      held.push({ field: field, value: value, chip: target.firstElementChild || el("span", null, target.textContent) });
      applyHeld();
    });

    document.addEventListener("click", function (event) {
      if (!shelf || !event.target.closest) return;
      var clear = event.target.closest("[data-verso-stream-clear]");
      if (clear && shelf.contains(clear)) {
        held = [];
        applyHeld();
        settleFocus(-1);
        return;
      }
      var release = event.target.closest("[data-verso-stream-release]");
      if (!release || !shelf.contains(release)) return;
      var index = parseInt(release.getAttribute("data-verso-stream-release"), 10);
      if (index >= 0 && index < held.length) {
        held.splice(index, 1);
        applyHeld();
        settleFocus(index);
      }
    });

    // The shelf is redrawn on every change, so the button that was pressed is
    // gone: focus goes to the value now in its place, else the shelf's last,
    // else, with the shelf gone, to the pause control beside the stream.
    function settleFocus(index) {
      var next = null;
      if (shelf && index >= 0) {
        var releases = shelf.querySelectorAll("[data-verso-stream-release]");
        next = releases[Math.min(index, releases.length - 1)] || shelf.querySelector("[data-verso-stream-clear]");
      }
      if (!next) next = pause;
      if (next) next.focus();
    }

    var es = new EventSource("/streams/" + encodeURIComponent(source));
    es.addEventListener("stream", function (event) {
      var frame;
      try {
        frame = JSON.parse(event.data);
      } catch (e) {
        return; // a malformed frame costs its own second, nothing more
      }
      if (!frame) return;
      if (frame.reset) {
        buffer = []; pending = 0; lost = false;
        if (paused) pendingReset = true;
        else clearHistory();
      }
      lost = lost || !!frame.lost;
      updateHealth(frame.available !== false);
      if (!frame.rows || !frame.rows.length) return;
      if (backlog) {
        // The opening frame is what the device had already logged. It did not
        // arrive at the rate the meta measures — counting it would claim a
        // hammered uplink on a silent network — and its newest row is not now.
        backlog = false;
      } else {
        // Live: every row of it arrived this second, whether the listing is
        // rendering them or holding them in the pause buffer, and the newest of
        // them is happening now, which is the offset between the two clocks.
        var at = Date.now();
        frame.rows.forEach(function () {
          arrivals.push(at);
        });
        skew = at / 1000 - frame.rows[frame.rows.length - 1].at;
      }
      if (paused) {
        // Nothing moves while paused: the events wait, and the button counts
        // them so the wait is stated rather than hidden. The count is every
        // event that arrived; the buffer keeps only what a ring could hold,
        // since resuming would evict the rest on the spot anyway.
        pending += frame.rows.length;
        frame.rows.forEach(function (ev) {
          buffer.push(ev);
        });
        if (buffer.length > ring) buffer = buffer.slice(buffer.length - ring);
        updatePause();
        return;
      }
      frame.rows.forEach(ingest);
      updateMeta();
      relens();
    });

    // The Pause button's spinner is this listing's only live indicator, so it
    // has to mean what it shows: it spins while the stream is up and this page
    // is not holding it, and stops when the connection drops. EventSource
    // reconnects on its own, and "open" is that reconnect landing. Neither
    // touches the pause itself — a stream that came back while a person was
    // reading stays held until they say otherwise.
    es.addEventListener("error", function () {
      updateHealth(false);
    });
    es.addEventListener("open", function () {
      updatePause();
    });

    // The page's own clock: relative times age and the rate decays. Frozen
    // while paused, because a paused list that kept re-labelling itself would
    // still be moving.
    setInterval(function () {
      if (paused) return;
      // A console prints wall clock, which does not age; only a grid's relative
      // time has to be re-read.
      if (!console) {
        rows.forEach(function (row) {
          row.tr.cells[0].textContent = relative(ageOf(row.at));
        });
      }
      updateMeta();
    }, TICK_MS);
  }
})();
