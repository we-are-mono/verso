// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-page.js — the page's own furniture (ADR-004). Small things that
// belong to a page rather than to anything on it: where an explanation stands
// beside the label that raised it, which section of a
// long page the rail beside it should mark, where an act made in place says
// how it went, and a ledger count that rolls when its number changed.

// A tip rises above the label that raises it, clear of the control under it,
// and stands where the pointer arrived along the label, as the system's own
// tooltips do — then holds still, so it can be read and reached. It is placed
// once, as the pointer comes onto the label (or focus does, at the label's
// start); moving within the label or onto the tip leaves it where it is. A
// label high in a panel that scrolls has no room above, and the tip would be
// cut off by the panel's own edge — so measure the room against whatever would
// clip it and drop the tip below when there is more there. The placement is
// the shell's, as every behaviour is (ADR-005 §7): the widget declares the tip
// and nothing about where it lands.
(function () {
  var GAP = 8; // the tip's own offset from the label, both ways
  var POINTER_LEAD = 12; // how far before the pointer the tip's corner stands

  // clipper is the box the tip has to fit inside: a drawer's scroll area or
  // a table's clipped overflow region, otherwise the viewport.
  function clipper(el) {
    for (var node = el.parentElement; node; node = node.parentElement) {
      var style = getComputedStyle(node);
      var overflow = style.overflowY;
      if (overflow === "auto" || overflow === "scroll" || overflow === "hidden" || overflow === "clip") {
        var rect = node.getBoundingClientRect();
        // The table's scroll hint fades its trailing edge, including anything
        // painted inside it. Keep explanations in the fully visible region.
        var fade = parseFloat(style.getPropertyValue("--verso-scroll-edge")) || 0;
        return { top: rect.top, bottom: rect.bottom, left: rect.left, right: rect.right - fade };
      }
    }
    return { top: 0, bottom: window.innerHeight, left: 0, right: window.innerWidth };
  }

  // place sets the tip's corner a little before x, the pointer's place across
  // the window (or at the label's start for focus, when x is null), kept
  // inside whatever would clip it.
  function place(label, x) {
    var tip = label.querySelector('[role="tooltip"]');
    if (!tip) return;
    // Measure from the default placement, so a tip that has been dropped once
    // can rise again when the panel is scrolled and the room returns.
    label.removeAttribute("data-verso-tip-below");
    // A waiting tip is out of the page and has no size; lay it out while it
    // is measured, still unseen.
    tip.style.display = "block";
    tip.style.left = "0px";
    tip.style.maxWidth = "";
    var box = clipper(label);
    // A short chip can sit near either edge of a horizontally scrolling table.
    // Keep its longer explanation inside the visible part of that table.
    var left = Math.max(0, box.left);
    var right = Math.min(window.innerWidth, box.right);
    if (tip.offsetWidth > right - left) tip.style.maxWidth = Math.max(0, right - left) + "px";
    var rect = label.getBoundingClientRect();
    var at = x == null ? 0 : Math.max(0, x - rect.left - POINTER_LEAD);
    tip.style.left = at + "px";
    var tipRect = tip.getBoundingClientRect();
    tip.style.left = at + Math.max(left - tipRect.left, Math.min(0, right - tipRect.right)) + "px";
    var height = tip.offsetHeight + GAP;
    tip.style.display = "";
    var above = rect.top - Math.max(0, box.top);
    var below = Math.min(window.innerHeight, box.bottom) - rect.bottom;
    if (above < height && below > above) label.setAttribute("data-verso-tip-below", "");
  }

  // A tip is placed as the pointer or focus arrives on its label, never as
  // the pointer moves within it: pointerover also fires on the way onto the
  // tip itself, which must not move away from the pointer reading it.
  document.addEventListener("pointerover", function (event) {
    var label = event.target.closest && event.target.closest("[data-verso-tip]");
    if (label && !label.contains(event.relatedTarget)) place(label, event.clientX);
  });
  document.addEventListener("focusin", function (event) {
    var label = event.target.closest && event.target.closest("[data-verso-tip]");
    if (label && !label.matches(":hover")) place(label, null);
  });

  // Escape puts a showing tip away without the pointer or focus having to
  // move (WCAG 1.4.13), and does only that: caught before the window's own
  // listeners, it does not also close the drawer the tip sits in. The tip
  // comes back the next time its label is pointed at or focused.
  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape") return;
    var showing = [].filter.call(document.querySelectorAll("[data-verso-tip]:hover, [data-verso-tip]:focus"), function (label) {
      return !label.hasAttribute("data-verso-tip-dismissed");
    });
    if (!showing.length) return;
    showing.forEach(function (label) { label.setAttribute("data-verso-tip-dismissed", ""); });
    event.stopPropagation();
    event.preventDefault();
  }, true);

  function restore(event) {
    var label = event.target.closest && event.target.closest("[data-verso-tip-dismissed]");
    if (label && !label.contains(event.relatedTarget)) label.removeAttribute("data-verso-tip-dismissed");
  }
  document.addEventListener("pointerout", restore);
  document.addEventListener("focusout", restore);

  // A port on a router's panel is a group inside the panel's SVG, which can
  // hold no HTML, so its detail is a sibling of the art, keyed by the port's
  // name. It shows over the port's middle as the pointer or focus arrives,
  // above it when there is room inside whatever would clip it and below when
  // there is not, and goes as the pointer or focus leaves, or on Escape.
  var PORT = ".verso-panel [data-verso-port]";
  var shownPort = null;
  function portTip(port) {
    var panel = port.closest(".verso-panel");
    return panel && panel.querySelector('[data-verso-port-tip="' + CSS.escape(port.getAttribute("data-verso-port")) + '"]');
  }
  function showPort(port) {
    var tip = portTip(port);
    if (!tip) return;
    hidePort();
    tip.hidden = false;
    var panel = port.closest(".verso-panel").getBoundingClientRect();
    var rect = port.getBoundingClientRect();
    var box = clipper(port);
    var height = tip.offsetHeight + GAP;
    var below = rect.top - Math.max(0, box.top) < height;
    tip.toggleAttribute("data-below", below);
    tip.style.left = rect.left - panel.left + rect.width / 2 + "px";
    tip.style.top = (below ? rect.bottom - panel.top + GAP : rect.top - panel.top - height) + "px";
    shownPort = tip;
  }
  function hidePort() {
    if (shownPort) shownPort.hidden = true;
    shownPort = null;
  }
  document.addEventListener("pointerover", function (event) {
    var port = event.target.closest && event.target.closest(PORT);
    if (port && !port.contains(event.relatedTarget)) showPort(port);
  });
  document.addEventListener("focusin", function (event) {
    var port = event.target.closest && event.target.closest(PORT);
    if (port) showPort(port);
  });
  function leavePort(event) {
    var port = event.target.closest && event.target.closest(PORT);
    if (port && !port.contains(event.relatedTarget)) hidePort();
  }
  document.addEventListener("pointerout", leavePort);
  document.addEventListener("focusout", leavePort);
  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape" || !shownPort) return;
    hidePort();
    event.stopPropagation();
    event.preventDefault();
  }, true);
})();

// The rail beside a long page: which of its sections you are reading, marked on
// the list of them (ADR-005 §7 — the page declares the places, the shell says
// where you are). Each link already states what it points at, so nothing here
// needs telling what a page's sections are.
//
// The current section is the last one whose heading has passed the reading line a
// third of the way down the viewport: that is the section whose content fills the
// screen, which is what someone means by "where I am" — an observer asking which
// section is visible answers several at once on a page of short sections, and
// answers none on a section taller than the window.
(function () {
  var LINE = 0.34; // the reading line, as a fraction of the viewport
  var pending = false;

  function rail() {
    var out = [];
    var links = document.querySelectorAll("[data-verso-rail-link]");
    for (var i = 0; i < links.length; i++) {
      var href = links[i].getAttribute("href") || "";
      var section = href.charAt(0) === "#" ? document.getElementById(href.slice(1)) : null;
      if (section) out.push({ link: links[i], section: section });
    }
    return out;
  }

  function mark() {
    pending = false;
    var places = rail();
    if (!places.length) return;
    var line = window.innerHeight * LINE;
    var current = places[0];
    for (var i = 0; i < places.length; i++) {
      if (places[i].section.getBoundingClientRect().top <= line) current = places[i];
    }
    // The last section can be too short to ever reach the line, so a page scrolled
    // to its end marks its final place rather than leaving the mark short of it.
    var end = document.documentElement;
    if (window.scrollY + window.innerHeight >= end.scrollHeight - 2) {
      current = places[places.length - 1];
    }
    for (var j = 0; j < places.length; j++) {
      if (places[j] === current) places[j].link.setAttribute("aria-current", "true");
      else places[j].link.removeAttribute("aria-current");
    }
  }

  function onScroll() {
    if (pending) return;
    pending = true;
    requestAnimationFrame(mark);
  }

  // Pressing a place on the rail travels to it rather than cutting to it: the
  // page is one long form and the rail is a way around it, so seeing the page
  // move is what tells you where the section you asked for sits in relation to
  // the one you were reading. A jump tells you nothing and loses your place.
  //
  // Scoped to these links rather than set on the document, because scroll-behavior
  // on the scrolling element would animate every programmatic scroll in the app,
  // restored positions included. Someone who has asked for less motion gets the
  // jump, which is what that setting means here.
  function travel(event) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey) return;
    var link = event.target.closest && event.target.closest("[data-verso-rail-link]");
    if (!link) return;
    var href = link.getAttribute("href") || "";
    var section = href.charAt(0) === "#" ? document.getElementById(href.slice(1)) : null;
    if (!section) return;
    event.preventDefault();
    var still = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    section.scrollIntoView({ behavior: still ? "auto" : "smooth", block: "start" });
    // The address says where you are without the browser taking you there a
    // second time, and a reload lands on the place that was asked for.
    if (window.history && window.history.replaceState) {
      window.history.replaceState(null, "", href);
    }
  }

  document.addEventListener("click", travel);
  window.addEventListener("scroll", onScroll, { passive: true });
  window.addEventListener("resize", onScroll, { passive: true });
  mark();
})();

// The outcome layer: the one place any act says how it went, at the right of
// the viewport under the bar, never in the content. An act taken on this page
// says so as its answer arrives; a page, or a panel's answer, arrived at after
// one brings its outcome in a template and the layer says it as it lands. The
// act's outcome has one place, a newer one replacing the one showing. Under
// it stand the events said to the session (ADR-028), each a notification of
// its own, newest nearest the act's place, at most three. Each stays long
// enough to read, holds while the pointer is on it, and leaves on its own or
// on its cross. It publishes window.versoOutcome, whose show the acts that
// answer in place call from inside their handlers, and whose event the
// events stream calls.
(function () {
  var layer = document.getElementById("verso-outcome");
  if (!layer) return;
  var slot = layer.querySelector("[data-verso-outcome-slot]");
  var close = layer.querySelector("[data-verso-outcome-close]");
  var column = layer.parentNode;
  var boxTemplate = document.querySelector("template[data-verso-event-box]");
  // An open drawer is a modal dialog: the browser draws it above the page and
  // leaves nothing outside it to point at. While one is open the notifications
  // stand inside it, at the drawer's own top right, and come home once
  // nothing shows.
  var home = column.parentNode;
  function place() {
    var host = home;
    document.querySelectorAll("dialog[open]").forEach(function (dialog) {
      if (dialog.matches(":modal")) host = dialog;
    });
    if (column.parentNode !== host) host.appendChild(column);
  }
  // Six seconds: long enough to read two short sentences twice.
  var STAY = 6000;
  // The events that stand at once: a fourth pushes out the oldest, which
  // System → Events still lists.
  var MOST_EVENTS = 3;
  var timer = null;
  var standing = []; // the events showing, newest first

  function settleHome() {
    if (layer.hidden && standing.length === 0 && column.parentNode !== home) home.appendChild(column);
  }

  function leave() {
    clearTimeout(timer);
    timer = null;
    layer.removeAttribute("data-verso-shown");
    // Emptied once the slide is over (the layer's own 300 ms), unless a newer
    // outcome has arrived in the meantime.
    setTimeout(function () {
      if (layer.hasAttribute("data-verso-shown")) return;
      slot.replaceChildren();
      layer.hidden = true;
      settleHome();
    }, 300);
  }

  // A refusal or a warning stays until it is closed, an event's as an act's.
  function lastingOf(outcome) {
    var variant = outcome.getAttribute && outcome.getAttribute("data-verso-flash-variant");
    return variant === "danger" || variant === "warning";
  }

  function leaveEvent(box) {
    var at = standing.indexOf(box);
    if (at < 0) return;
    standing.splice(at, 1);
    clearTimeout(box.versoTimer);
    box.removeAttribute("data-verso-shown");
    setTimeout(function () {
      box.remove();
      settleHome();
    }, 300);
  }

  // event says one event as a notification of its own under the act's place.
  function event(outcome) {
    if (!boxTemplate) return;
    place();
    var box = document.importNode(boxTemplate.content.firstElementChild, true);
    box.querySelector("[data-verso-outcome-slot]").replaceChildren(outcome);
    column.insertBefore(box, layer.nextSibling);
    standing.unshift(box);
    while (standing.length > MOST_EVENTS) leaveEvent(standing[standing.length - 1]);
    var lastingEvent = lastingOf(outcome);
    function stayEvent() {
      clearTimeout(box.versoTimer);
      box.versoTimer = setTimeout(function () { leaveEvent(box); }, STAY);
    }
    function resumeEvent() {
      if (!lastingEvent && !box.contains(document.activeElement)) stayEvent();
    }
    box.addEventListener("pointerenter", function () { clearTimeout(box.versoTimer); });
    box.addEventListener("pointerleave", resumeEvent);
    box.addEventListener("focusin", function () { clearTimeout(box.versoTimer); });
    box.addEventListener("focusout", function () { setTimeout(resumeEvent, 0); });
    var cross = box.querySelector("[data-verso-outcome-close]");
    if (cross) cross.addEventListener("click", function () { leaveEvent(box); });
    versoAnnounce(outcome.textContent.trim());
    requestAnimationFrame(function () {
      requestAnimationFrame(function () { box.setAttribute("data-verso-shown", ""); });
    });
    if (!lastingEvent) stayEvent();
  }

  function stay() {
    clearTimeout(timer);
    timer = setTimeout(leave, STAY);
  }

  // A refusal or a warning stays until it is closed: it is something to act
  // on, and a sentence that leaves on a timer can leave before it is read.
  var lasting = false;

  function show(outcome) {
    clearTimeout(timer);
    place();
    slot.replaceChildren(outcome);
    layer.hidden = false;
    lasting = lastingOf(outcome);
    // The layer was hidden a moment ago, and a live region that appears with
    // its words already in it is often not read, so the sentence is also said
    // through the page's announcer.
    versoAnnounce(outcome.textContent.trim());
    // Shown on the next frame, so the layer starts from its off-screen place
    // and the arrival is a slide rather than an appearance.
    requestAnimationFrame(function () {
      requestAnimationFrame(function () { layer.setAttribute("data-verso-shown", ""); });
    });
    if (!lasting) stay();
  }

  function resume() {
    if (!lasting && !layer.contains(document.activeElement)) stay();
  }

  // Pointing at it or tabbing into it holds it where it is.
  layer.addEventListener("pointerenter", function () { clearTimeout(timer); });
  layer.addEventListener("pointerleave", resume);
  layer.addEventListener("focusin", function () { clearTimeout(timer); });
  layer.addEventListener("focusout", function () { setTimeout(resume, 0); });
  if (close) close.addEventListener("click", leave);

  // arrive says the outcome a page, or a panel's answer, brought with it:
  // carried in a template, it is never drawn where it came, only said here,
  // and once.
  function arrive(root) {
    var carried = root && root.querySelector && root.querySelector("template[data-verso-arrival]");
    if (!carried) return;
    carried.remove();
    var outcome = carried.content.firstElementChild;
    if (outcome) show(document.importNode(outcome, true));
  }
  window.versoOutcome = { show: show, event: event };
  arrive(document);
  document.addEventListener("htmx:afterSwap", function (event) { arrive(event.target); });

  // An act whose request was never answered says so: otherwise a press the
  // router never heard looks like one that did nothing. Only an act someone
  // took — a press, a submit — speaks; a refresh the page makes on its own
  // stays quiet, and tries again on its own.
  document.addEventListener("htmx:sendError", function (event) {
    var config = event.detail && event.detail.requestConfig;
    var trigger = config && config.triggeringEvent;
    var template = document.querySelector("template[data-verso-unanswered]");
    if (!trigger || !trigger.isTrusted || !template) return;
    show(document.importNode(template.content.firstElementChild, true));
  });
})();

// The events said to the session (ADR-028) reach the browser on one stream:
// the tab holding the lock opens it and hands each event to the others, and
// when that tab closes the lock passes to another. A tab says an event only
// while someone is looking at it, a hidden one keeping it for its minute, and
// the browser says each once, whichever tab is first to claim it.
(function () {
  var column = document.querySelector("[data-verso-outcomes]");
  var url = column && column.getAttribute("data-verso-events");
  if (!url || !window.versoOutcome || typeof EventSource === "undefined") return;
  var SHOWN = "verso-events-shown";
  var REMEMBERED = 32; // ids remembered as said
  var FRESH = 60000; // an event unsaid after a minute is System → Events' alone
  var waiting = []; // events that came while this tab was hidden

  function said() {
    try {
      return JSON.parse(localStorage.getItem(SHOWN) || "[]");
    } catch (e) {
      return [];
    }
  }
  function markSaid(id) {
    try {
      var ids = said().filter(function (x) { return x !== id; });
      ids.push(id);
      localStorage.setItem(SHOWN, JSON.stringify(ids.slice(-REMEMBERED)));
    } catch (e) {
      // Without storage each tab says what it is first to see.
    }
  }
  function say(item) {
    var template = document.createElement("template");
    template.innerHTML = item.html;
    var outcome = template.content.firstElementChild;
    if (outcome) window.versoOutcome.event(outcome);
  }
  // claim says item unless another tab has, holding the event's own lock
  // while it looks and marks it, so two tabs in view cannot both say it.
  function claim(item) {
    if (said().indexOf(item.id) >= 0) return;
    if (!navigator.locks) {
      markSaid(item.id);
      say(item);
      return;
    }
    navigator.locks.request("verso-event-" + item.id, { ifAvailable: true }, function (lock) {
      if (!lock || said().indexOf(item.id) >= 0) return;
      markSaid(item.id);
      say(item);
    });
  }
  function arrived(id, html) {
    var item = { id: id, html: html, until: Date.now() + FRESH };
    if (document.visibilityState === "visible") claim(item);
    else waiting.push(item);
  }
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState !== "visible") return;
    var now = Date.now();
    var due = waiting;
    waiting = [];
    due.forEach(function (item) { if (item.until > now) claim(item); });
  });

  var channel = typeof BroadcastChannel === "undefined" ? null : new BroadcastChannel("verso-events");
  if (channel) {
    channel.onmessage = function (message) {
      if (message.data && message.data.id) arrived(message.data.id, message.data.html);
    };
  }
  function listen() {
    var source = new EventSource(url);
    source.addEventListener("said", function (e) {
      var data;
      try {
        data = JSON.parse(e.data);
      } catch (err) {
        return;
      }
      if (channel) channel.postMessage({ id: e.lastEventId, html: data.html });
      arrived(e.lastEventId, data.html);
    });
  }
  if (navigator.locks && channel) {
    // Held for as long as this page lives: the promise never settles.
    navigator.locks.request("verso-events", function () {
      listen();
      return new Promise(function () {});
    });
  } else {
    listen();
  }
})();

// versoRoll rolls a value from what it was to what it is: the old number
// rolls out and the new one rolls in — upward for more, downward for fewer,
// upward for a change that is not a number — inside a frame the size of the
// value, and the words in `warm` (the value itself, or the label beside it)
// go to ink for a moment and settle back. A frame the caller already keeps
// is used as it is; any other value is framed for the
// roll and unframed after it. A fact that changes under a live refresh rolls
// the same way (verso-tables.js).
window.versoRoll = function (value, from, to, frame, warm) {
  var EASE = "cubic-bezier(0.22, 1, 0.36, 1)";
  var a = Number(from), b = Number(to);
  var more = isNaN(a) || isNaN(b) || b >= a;
  var framed = !frame;
  if (framed) {
    frame = document.createElement("span");
    frame.className = "inline-grid overflow-hidden";
    value.parentNode.insertBefore(frame, value);
    frame.appendChild(value);
    value.style.gridArea = "1 / 1";
  }
  var gone = value.cloneNode(false);
  [].slice.call(gone.attributes).forEach(function (attr) {
    if (attr.name.indexOf("data-verso") === 0) gone.removeAttribute(attr.name);
  });
  gone.setAttribute("aria-hidden", "true");
  gone.textContent = from;
  if (framed) gone.style.gridArea = "1 / 1";
  frame.insertBefore(gone, value);
  var out = more ? "-100%" : "100%";
  var inn = more ? "100%" : "-100%";
  function settle() {
    gone.remove();
    if (!framed) return;
    frame.parentNode.insertBefore(value, frame);
    frame.remove();
    value.style.gridArea = "";
  }
  gone.animate(
    [{ transform: "translateY(0)", opacity: 1 }, { transform: "translateY(" + out + ")", opacity: 0 }],
    { duration: 420, easing: EASE, fill: "forwards" }
  ).finished.then(settle, settle);
  value.animate(
    [{ transform: "translateY(" + inn + ")", opacity: 0 }, { transform: "translateY(0)", opacity: 1 }],
    { duration: 420, easing: EASE }
  );
  warm = warm || frame;
  var ink = getComputedStyle(warm).getPropertyValue("--color-ink").trim();
  if (ink) warm.animate([{ color: ink }, { color: ink, offset: 0.35 }, {}], { duration: 1600, easing: "ease-out" });
};

// Help (form-fields): every setting's help stays folded until a reader asks
// for it. The act on a page's heading line and in every drawer's header, or
// the ? key, shows all of it and hides it again; the root carries the state,
// so the stylesheet opens every description at once and shows the act's
// other words, and a cookie keeps it for this browser, which the shell reads
// back so the next page arrives as this one was left.
//
// The row being read holds still through it all: help above it opens or
// closes, and the page scrolls with it frame by frame, so the row stays
// where the eye left it. Shown, help opens outward from that row, the rows
// nearest it first.
(function () {
  var COOKIE = "verso_help";
  var MAX_AGE = 31536000; // a year
  var GLIDE = 200; // a description's glide (blocks/field.css)
  var STEP = 16; // the ripple's step from one row to the next
  var REACH = 8; // how many rows away the ripple still waits

  // The cookie is never Secure: a router answers over HTTP and HTTPS alike,
  // and a Secure cookie written over one is one the other can neither read
  // nor overwrite.
  function cookie(shown) {
    return COOKIE + "=" + (shown ? "shown; Path=/; Max-Age=" + MAX_AGE : "; Path=/; Max-Age=0") + "; SameSite=Strict";
  }

  function still() {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  // scope is where help is being read: the open drawer, else the page.
  function scope() {
    return document.querySelector("dialog[open]") || document.querySelector("main") || document.body;
  }

  // scroller is the box that scrolls a row: a drawer's body, or the page.
  function scroller(el) {
    for (var node = el.parentElement; node && node !== document.body; node = node.parentElement) {
      var y = getComputedStyle(node).overflowY;
      if ((y === "auto" || y === "scroll") && node.scrollHeight > node.clientHeight) return node;
    }
    return document.scrollingElement || document.documentElement;
  }

  // view is the part of the scroller a reader sees, under the top bar on a
  // page.
  function view(box) {
    if (box === document.scrollingElement || box === document.documentElement) {
      var bar = document.querySelector("body > header");
      return { top: bar ? Math.max(0, bar.getBoundingClientRect().bottom) : 0, bottom: window.innerHeight };
    }
    var r = box.getBoundingClientRect();
    return { top: r.top, bottom: r.bottom };
  }

  function rows(within) {
    return [].filter.call(within.querySelectorAll(".verso-field"), function (row) {
      return row.getClientRects().length > 0;
    });
  }

  // reading is the row the eye is on: the one holding focus while it is in
  // view, else the first whose name is in view.
  function reading(all) {
    if (!all.length) return null;
    var seen = view(scroller(all[0]));
    var focused = document.activeElement && document.activeElement.closest && document.activeElement.closest(".verso-field");
    if (focused && all.indexOf(focused) >= 0) {
      var f = focused.getBoundingClientRect();
      if (f.bottom > seen.top && f.top < seen.bottom) return focused;
    }
    for (var i = 0; i < all.length; i++) {
      if (all[i].getBoundingClientRect().top >= seen.top) return all[i];
    }
    return all[all.length - 1];
  }

  // hold makes a change and keeps the row where it stood while the help
  // around it glides, until the glide ends or the reader scrolls on their
  // own. The browser's own scroll anchoring stands aside meanwhile, so the
  // page is corrected once, not twice.
  function hold(row, change, lasting) {
    var box = scroller(row);
    var was = row.getBoundingClientRect().top;
    var anchoring = box.style.overflowAnchor;
    var left = false;
    var leave = function () { left = true; };
    box.style.overflowAnchor = "none";
    window.addEventListener("wheel", leave, { passive: true, once: true });
    window.addEventListener("touchmove", leave, { passive: true, once: true });
    change();
    var until = performance.now() + lasting + 40;
    (function frame() {
      if (!left) {
        var drift = row.getBoundingClientRect().top - was;
        if (drift) box.scrollTop += drift;
      }
      if (!left && performance.now() < until) { requestAnimationFrame(frame); return; }
      box.style.overflowAnchor = anchoring;
      window.removeEventListener("wheel", leave);
      window.removeEventListener("touchmove", leave);
    })();
  }

  // ripple waits each description in view by how far its row stands from
  // the one being read, so help opens outward from it; it returns how long
  // the last one waits, and clears the waits once all have opened.
  function ripple(all, row) {
    var seen = view(scroller(row));
    var at = all.indexOf(row);
    var longest = 0;
    var waited = [];
    all.forEach(function (other, i) {
      var r = other.getBoundingClientRect();
      if (r.bottom < seen.top || r.top > seen.bottom) return;
      var desc = other.querySelector(":scope > .verso-field__ledger > .verso-field__question > .verso-field__desc");
      if (!desc) return;
      var wait = Math.min(Math.abs(i - at), REACH) * STEP;
      desc.style.setProperty("--verso-help-delay", wait + "ms");
      waited.push(desc);
      longest = Math.max(longest, wait);
    });
    setTimeout(function () {
      waited.forEach(function (desc) { desc.style.removeProperty("--verso-help-delay"); });
    }, GLIDE + longest + 40);
    return longest;
  }

  function toggle() {
    var root = document.documentElement;
    var shown = root.getAttribute("data-verso-help") !== "shown";
    var all = rows(scope());
    var row = reading(all);
    var change = function () {
      if (shown) root.setAttribute("data-verso-help", "shown");
      else root.removeAttribute("data-verso-help");
    };
    document.cookie = cookie(shown);
    if (!row) { change(); return; }
    var wait = shown && !still() ? ripple(all, row) : 0;
    hold(row, change, still() ? 0 : GLIDE + wait);
  }

  document.addEventListener("click", function (event) {
    var target = event.target && event.target.closest ? event.target : null;
    if (!target) return;
    if (target.closest("[data-verso-help-toggle]")) toggle();
  });

  // The ? key presses help wherever there is help to show, except while
  // the reader is typing.
  document.addEventListener("keydown", function (event) {
    if (event.key !== "?" || event.ctrlKey || event.metaKey || event.altKey || event.defaultPrevented) return;
    var target = event.target;
    if (target && target.closest && target.closest('textarea, select, [contenteditable]:not([contenteditable="false"]), input:not([type="checkbox"], [type="radio"], [type="button"], [type="submit"], [type="reset"])')) return;
    var act = scope().querySelector("[data-verso-help-toggle]");
    if (!act || !act.getClientRects().length) return;
    event.preventDefault();
    toggle();
  });
})();
