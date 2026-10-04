// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-listing.js — the live consoles (ADR-004, ADR-005 §7): the firewall's
// activity log and the router's own log. A console declares that its lines
// arrive over time and names its source; everything here is the shell's.
//
// The two share one core (versoConsole): the transport, the live control on
// the heading line that says what the log is doing, and the pause, which holds
// what arrives — reset included — until the person lets it go. Each log brings
// only what is its own: how a line is built, and what else a frame says.
//
// The wire carries data, never markup — a log line's contents are whatever a
// stranger sent this router — so every line is built with createElement and
// textContent (versoEl).

// versoConsole wires one live log. What it is given:
//
//   pause    the live control (a button), and label the part of it that
//            says Live, Paused or Connecting…; counted makes a paused label
//            say how many events wait
//   ring     how much a pause holds before the oldest goes
//   url      where the stream is, read on every (re)connect
//   draw     what a frame's rows become while the log is live
//   resume   what the rows held by a pause become (draw, unless given)
//   clear    the log's lines gone, for a stream that started over
//   accepts  whether a frame is one this log reads at all
//   available what the frame says of the source, folded into the log's own
//            state; answers whether lines can arrive (true, unless given)
//   reset    what else a stream that started over forgets
//   lost     what else a dropped connection means
//   status   what else is said whenever the live control is
//   liveOnOpen whether the connection landing is itself the log being live
//   wire     the stream's own further events
//
// It answers {status, close, restart}.
function versoConsole(o) {
  var es = null;
  var paused = false;
  var connected = false;
  var buffer = [];
  var pending = 0; // events that arrived while paused — every one of them
  var pendingReset = false;
  var waiting = o.pause && o.pause.querySelector("[data-verso-wait]");

  // The control says what the log is doing; its title, what pressing it does.
  // The spinner turns only while lines can arrive.
  function status() {
    if (o.pause) {
      var label = paused
        ? (o.counted && pending ? T("Paused · %d new").replace("%d", pending) : T("Paused"))
        : connected ? T("Live") : T("Connecting…");
      if (o.label) o.label.textContent = label;
      else o.pause.textContent = label;
      o.pause.title = paused ? T("Resume") : T("Pause");
      if (waiting) waiting.toggleAttribute("data-verso-wait-paused", paused || !connected);
    }
    if (o.status) o.status();
  }

  function receive(event) {
    var frame;
    try {
      frame = JSON.parse(event.data);
    } catch (e) {
      return; // a malformed frame costs its own second, nothing more
    }
    if (!frame || (o.accepts && !o.accepts(frame))) return;
    if (frame.reset) {
      buffer = [];
      pending = 0;
      if (o.reset) o.reset();
      if (paused) pendingReset = true;
      else o.clear();
    }
    connected = o.available ? o.available(frame) : true;
    status();
    var rows = frame.rows || [];
    if (o.arrived) o.arrived(rows);
    if (paused) {
      // Nothing moves while paused: the events wait, and a counted control
      // says how many, so the wait is stated rather than hidden. The count is
      // every event that arrived; the buffer keeps only what the ring could
      // hold, since resuming would evict the rest on the spot anyway.
      pending += rows.length;
      if (rows.length) {
        buffer = buffer.concat(rows);
        if (buffer.length > o.ring) buffer = buffer.slice(buffer.length - o.ring);
      }
      status();
      return;
    }
    o.draw(rows);
  }

  // The live control is the log's only live indicator, so it has to mean
  // what it shows. EventSource reconnects on its own, and "open" is that
  // reconnect landing. Neither touches the pause itself — a stream that came
  // back while a person was reading stays held until they say otherwise.
  function open() {
    es = new EventSource(o.url());
    es.addEventListener("stream", receive);
    es.addEventListener("open", function () {
      if (o.liveOnOpen) connected = true;
      status();
    });
    es.addEventListener("error", function () {
      connected = false;
      if (o.lost) o.lost();
      status();
    });
    if (o.wire) o.wire(es);
  }

  function close() {
    if (es) es.close();
  }

  if (o.pause) {
    o.pause.addEventListener("click", function () {
      paused = !paused;
      if (!paused) {
        if (pendingReset) {
          o.clear();
          pendingReset = false;
        }
        var held = buffer;
        buffer = [];
        pending = 0;
        (o.resume || o.draw)(held);
      }
      status();
    });
  }

  status();
  open();
  return {
    status: status,
    close: close,
    // restart reads the stream again from its start: what waits is dropped,
    // the control says Connecting… until it lands.
    restart: function () {
      close();
      buffer = [];
      pending = 0;
      pendingReset = false;
      connected = false;
      status();
      open();
    },
    paused: function () { return paused; },
  };
}

// The firewall's activity log: a table of style "console" that names its
// source. Its lines collapse a run of the same event into one with a count,
// and its values are their own filter: click a verdict, a zone, an address,
// and what was clicked travels to a shelf above the log wearing exactly the
// treatment it had in the line. A second click, the shelf's ×, or Clear lets
// go; with nothing held there is no shelf at all. The page-wide lens (the
// still-lens) is the other half and the two compose — a plucked value hides
// what does not match, the lens dims it.
(function () {
  if (!window.EventSource) return;
  var consoles = document.querySelectorAll('[data-verso-stream][data-verso-stream-style="console"]');
  if (!consoles.length) return;

  // What a verdict means in a log is not what it means in the config. There, a
  // reject is the refusal you wrote and a drop is the silence you chose. Here,
  // every line is something that already happened to real traffic, and the
  // loudest of them is the one that vanished without an answer.
  //
  // The mark down a console line's left edge: the hue at full chroma, which is
  // what a mark is for.
  var VERDICT_MARK = { accept: "bg-green", reject: "bg-marigold", drop: "bg-crimson" };
  var VERDICT_INK = {
    accept: "text-green-deep",
    reject: "text-marigold-deep",
    drop: "text-crimson-deep",
  };
  // One console line. Fixed columns so the eye reads down one without a rule to
  // guide it; everything in mono at the reading size, because every value on
  // the line is a machine string.
  var CONSOLE = {
    row: "group flex cursor-pointer items-stretch gap-3 py-px pr-11 pl-7.25 leading-6 hover:bg-mid/50",
    mark: "my-0.5 w-0.75 shrink-0 rounded-full ",
    time: "w-18 shrink-0 font-mono text-base font-medium text-body",
    verdict: "w-22 shrink-0 font-mono text-base font-medium ",
    path: "flex w-66 shrink-0 items-baseline gap-2 font-mono text-base font-medium",
    arrow: "flex w-6 shrink-0 justify-center text-faint",
    proto: "flex w-24 shrink-0 items-baseline gap-1.5 font-mono text-base font-medium text-body",
    rule: "flex min-w-0 flex-1 items-baseline gap-2 pr-3",
    zone: "shrink-0 text-meta",
    addr: "min-w-0 truncate",
    port: "shrink-0 text-glyph",
    count: "shrink-0 font-mono text-sm text-meta",
  };
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
  // How much of the recent past the rate is measured over, and how often it
  // decays. Both are the page's own clock, and neither runs while the stream
  // is paused.
  var RATE_WINDOW_MS = 10000;
  var TICK_MS = 5000;
  var el = versoEl;

  [].forEach.call(consoles, start);

  function start(wrapper) {
    var source = wrapper.getAttribute("data-verso-stream");
    if (!source) return;
    var ring = parseInt(wrapper.getAttribute("data-verso-stream-ring"), 10) || 200;
    var body = wrapper.querySelector("[data-verso-console-rows]");
    if (!body) return;
    var section = wrapper.closest("section");
    var meta = section && section.querySelector("[data-verso-section-meta]");
    var metaRest = meta ? meta.textContent : "";
    // The live control sits on the heading line, outside any section.
    var pause = (section && section.querySelector("button[data-verso-live]")) || document.querySelector("button[data-verso-live]");
    var lens = document.querySelector("[data-verso-filter]");

    // rows is the ring, newest first; each remembers the tuple it collapses on
    // and the values it can be narrowed by.
    var empty = body.querySelector("[data-verso-stream-empty]");
    var emptyTemplate = empty && empty.cloneNode(true);
    var rows = [];
    var arrivals = []; // arrival stamps, for the rolling rate
    var held = []; // the plucked values, in the order they were picked up
    var shelf = null;
    // The first frame is the backlog: what the device had already logged before
    // this page opened. Everything after it is the present arriving.
    var backlog = true;
    // Whether the source can be read, and whether events were lost on the way.
    var available = true;
    var lost = false;
    var health = el("p", "verso-console-notice border-b border-rule-strong bg-marigold-soft px-10 py-3 text-sm text-marigold-deep");
    health.setAttribute("role", "status");
    health.hidden = true;
    wrapper.parentNode.insertBefore(health, wrapper);
    function sayHealth() {
      health.hidden = available && !lost;
      health.textContent = available ? T("Some firewall events were lost.") : T("Firewall logs unavailable");
    }
    function clearHistory() {
      rows.forEach(function (row) { row.tr.remove(); });
      rows = []; arrivals = []; backlog = true;
      if (emptyTemplate && !body.querySelector("[data-verso-stream-empty]")) body.appendChild(emptyTemplate.cloneNode(true));
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

    // build renders one event as a line of the log: a mark in the verdict's
    // own hue down the left edge, then the fixed columns. No cells, no borders
    // — a terminal does not draw a grid around what it prints.
    function build(ev) {
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

    // key is the tuple a repeat has to match exactly to be the same event
    // happening again: everything about the packet except when it happened.
    function key(ev) {
      return [ev.verdict, ev.from, ev.src, ev.to, ev.proto, ev.port, ev.rule].join(" ");
    }

    // A console prints the clock the device stamped, not how long ago it was:
    // a log is read down, and a column of "12 s" ages under the eye.
    function paint(row) {
      row.tr.children[1].textContent = versoClock(new Date(row.at * 1000));
      row.tr.lastChild.lastChild.textContent = row.count > 1 ? TIMES + row.count : "";
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
        if (wrapper.parentNode) wrapper.parentNode.insertBefore(shelf, wrapper);
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

    // ingest folds one event in. A repeat of the newest row is that row
    // happening again: its counter climbs and its clock moves up, and nothing
    // else on the page moves. A repeat that is not consecutive starts a fresh
    // row, so the order of the list never lies about the order of events.
    // The rate is the frame's business, not this one's: an event is counted
    // when it arrives, which is once, whether it was rendered then or held in
    // the pause buffer and folded in later.
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

    // The still-lens is the page's own control and owns its behaviour; a batch
    // of new rows simply asks it to look again, so the two never drift apart.
    function relens() {
      if (lens && lens.value) lens.dispatchEvent(new Event("input", { bubbles: true }));
    }

    var log = versoConsole({
      pause: pause,
      label: pause && pause.querySelector("[data-verso-live-label]"),
      counted: true,
      ring: ring,
      url: function () { return "/streams/" + encodeURIComponent(source); },
      clear: clearHistory,
      reset: function () { lost = false; },
      // A frame says whether the source can be read and whether events were
      // lost; a source that cannot be read is one no lines arrive from.
      available: function (frame) {
        lost = lost || !!frame.lost;
        available = frame.available !== false;
        return available;
      },
      lost: function () { available = false; },
      status: sayHealth,
      arrived: function (batch) {
        if (!batch.length) return;
        if (backlog) {
          // The opening frame is what the device had already logged. It did
          // not arrive at the rate the meta measures — counting it would claim
          // a hammered uplink on a silent network.
          backlog = false;
          return;
        }
        // Live: every row of it arrived this second, whether the listing is
        // rendering them or holding them in the pause buffer.
        var at = Date.now();
        batch.forEach(function () {
          arrivals.push(at);
        });
      },
      draw: function (batch) {
        if (!batch.length) return;
        batch.forEach(ingest);
        updateMeta();
        relens();
      },
      resume: function (batch) {
        batch.forEach(ingest);
        applyHeld();
        relens();
      },
    });

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

    // The page's own clock: the rate decays. Frozen while paused, because a
    // paused list that kept re-labelling itself would still be moving.
    setInterval(function () {
      if (log.paused()) return;
      updateMeta();
    }, TICK_MS);
  }
})();

// The router's own log (logs.html.tmpl): every service's lines, oldest first,
// with the firewall's traffic folded in on request. A line is kept once,
// however many frames repeat it, and the newest thousand stay.
(function () {
  "use strict";
  var root = document.querySelector("[data-verso-system-log]");
  if (!root || !window.EventSource) return;
  var body = root.querySelector("[data-verso-console-rows]");
  var health = root.querySelector("[data-log-health]");
  // The log's acts sit on the heading line, outside the log itself.
  var pause = document.querySelector("[data-log-pause]");
  // Whether the firewall's traffic is folded in is this reader's way of reading
  // the log, kept in the browser so a reload keeps it; its switch is in the
  // settings drawer, fetched later. A browser that keeps nothing reads it out.
  var FIREWALL_KEY = "verso-log-firewall";
  var firewall = false;
  try { firewall = window.localStorage.getItem(FIREWALL_KEY) === "1"; } catch (_) { /* kept nowhere: out */ }
  var unreadable = false, firewallUnavailable = false, seen = new Set();
  var el = versoEl;

  function draw(rows) {
    var pinned = body.scrollHeight - body.scrollTop - body.clientHeight < 32;
    var fragment = document.createDocumentFragment();
    rows.forEach(function (row) {
      if (seen.has(row.id)) return;
      seen.add(row.id);
      var error = ["emerg", "alert", "crit", "err"].indexOf(row.severity) !== -1;
      var warning = error || row.severity === "warn";
      var line = el("div", "grid grid-cols-[0.1875rem_4rem_minmax(0,1fr)_4rem] items-stretch gap-x-3 py-1 pr-10 pl-6.25 leading-6 hover:bg-mid/50 lg:flex lg:py-px");
      line.setAttribute("data-log-row", String(row.id));
      line.dataset.logAt = String(row.at);
      line.appendChild(el("span", "row-span-2 my-0.5 w-0.75 shrink-0 rounded-full " + (error ? "bg-crimson" : warning ? "bg-marigold" : "bg-transparent")));
      var at = new Date(row.at * 1000);
      var stamp = el("time", "shrink-0 font-mono lg:w-20 text-base font-medium text-body", versoClock(at));
      stamp.dateTime = at.toISOString(); stamp.title = at.toLocaleString(); line.appendChild(stamp);
      var origin = el("span", "min-w-0 truncate font-mono lg:w-34 lg:shrink-0 text-base font-medium text-ink", row.source); origin.title = row.source; line.appendChild(origin);
      line.appendChild(el("span", "w-16 shrink-0 font-mono text-base " + (error ? "font-bold text-crimson-deep" : warning ? "font-medium text-marigold-deep" : "font-medium text-meta"), row.severity));
      line.appendChild(el("span", "col-span-3 col-start-2 min-w-0 flex-1 wrap-anywhere font-mono text-base font-medium " + (error ? "text-crimson-deep" : "text-ink"), row.message));
      line.dataset.logText = at.toISOString() + " " + row.source + " " + row.severity + " " + row.message;
      fragment.appendChild(line);
    });
    body.appendChild(fragment);
    var lines = Array.from(body.querySelectorAll("[data-log-row]"));
    // Separate buffers are sampled at different instants. A delayed batch
    // still belongs beside its timestamp, not below a newer service event.
    if (firewall && rows.length) {
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

  // The live control is the log's state; a source that cannot be read is
  // said on the notice line above the log.
  var log = versoConsole({
    pause: pause,
    label: pause.querySelector("[data-log-pause-label]"),
    ring: 1000,
    url: function () { return "/streams/system-log" + (firewall ? "?firewall=1" : ""); },
    accepts: function (frame) { return Array.isArray(frame.rows); },
    available: function (frame) {
      unreadable = false;
      firewallUnavailable = !!frame.firewall_unavailable;
      return true;
    },
    status: function () {
      var problem = unreadable ? T("Logs unavailable") : firewallUnavailable ? T("Firewall logs unavailable") : "";
      health.textContent = problem;
      health.hidden = !problem;
    },
    liveOnOpen: true,
    wire: function (es) {
      es.addEventListener("unavailable", function () { unreadable = true; log.status(); });
    },
    clear: clearRows,
    draw: draw,
  });

  // The settings drawer's switch arrives with the drawer: it is set from the
  // log's state when it lands, and turning it reads the log again.
  document.addEventListener("htmx:afterSwap", function () {
    var toggle = document.querySelector("[data-log-include-firewall]");
    if (toggle) toggle.checked = firewall;
  });
  document.addEventListener("change", function (e) {
    if (!e.target.matches || !e.target.matches("[data-log-include-firewall]")) return;
    firewall = e.target.checked;
    log.close();
    clearRows();
    unreadable = false; firewallUnavailable = false;
    try { window.localStorage.setItem(FIREWALL_KEY, firewall ? "1" : "0"); } catch (_) { /* this visit only */ }
    log.restart();
  });
  document.querySelector("[data-log-download]").addEventListener("click", function () {
    var text = Array.from(body.querySelectorAll("[data-log-row]")).filter(function (n) { return !n.hidden; }).map(function (n) { return n.dataset.logText; }).join("\n");
    var url = URL.createObjectURL(new Blob([text + "\n"], { type: "text/plain;charset=utf-8" }));
    var a = document.createElement("a"); a.href = url; a.download = "router-log.txt"; a.click(); setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
  });
  window.addEventListener("pagehide", log.close);
})();
