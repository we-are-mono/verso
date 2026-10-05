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

// The outcome layer: where an act made in place says how it went. A page you
// arrive at says what happened before it at the top, in the flash; an act you
// take on this page says what just happened here, at the right, where the panel
// it came from has just gone — the same element, the same words and tones, a
// different place and a lifetime. One at a time: a newer outcome replaces the
// one showing. It stays long enough to read, holds while the pointer is on it,
// and leaves on its own or on the cross. It publishes window.versoOutcome,
// which the acts that answer in place call from inside their handlers.
(function () {
  var layer = document.getElementById("verso-outcome");
  if (!layer) return;
  var slot = layer.querySelector("[data-verso-outcome-slot]");
  var close = layer.querySelector("[data-verso-outcome-close]");
  // Six seconds: long enough to read two short sentences twice.
  var STAY = 6000;
  var timer = null;

  function leave() {
    clearTimeout(timer);
    timer = null;
    layer.classList.remove("is-shown");
    // Emptied once the slide is over (the layer's own 300 ms), unless a newer
    // outcome has arrived in the meantime.
    setTimeout(function () {
      if (layer.classList.contains("is-shown")) return;
      slot.replaceChildren();
      layer.hidden = true;
    }, 300);
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
    slot.replaceChildren(outcome);
    layer.hidden = false;
    var variant = outcome.getAttribute && outcome.getAttribute("data-verso-flash-variant");
    lasting = variant === "danger" || variant === "warning";
    // The layer was hidden a moment ago, and a live region that appears with
    // its words already in it is often not read, so the sentence is also said
    // through the page's announcer.
    versoAnnounce(outcome.textContent.trim());
    // Shown on the next frame, so the layer starts from its off-screen place
    // and the arrival is a slide rather than an appearance.
    requestAnimationFrame(function () {
      requestAnimationFrame(function () { layer.classList.add("is-shown"); });
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
  window.versoOutcome = { show: show };
})();

// A ledger's count: how many things a part of a section holds. Adding or
// removing one reloads the page, so the change would otherwise land as a
// different number with nothing to say it moved. The page remembers each
// count for the tab's session; when one differs from last time, the old
// number rolls out and the new one rolls in — upward for more, downward for
// fewer — and settles from ink to its resting meta, so the eye finds the
// change where it happened. Reduced motion, or no storage, shows the number
// as it is.
// versoRoll rolls a value from what it was to what it is: the old number
// rolls out and the new one rolls in — upward for more, downward for fewer,
// upward for a change that is not a number — inside a frame the size of the
// value, and the words in `warm` (the value itself, or the label beside it)
// go to ink for a moment and settle back. A frame the caller already keeps
// (a ledger count's) is used as it is; any other value is framed for the
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

(function () {
  var counts = document.querySelectorAll("[data-verso-count]");
  if (!counts.length) return;
  var still = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  function recall(key) {
    try { return window.sessionStorage.getItem(key); } catch (e) { return null; }
  }
  function remember(key, value) {
    try { window.sessionStorage.setItem(key, value); } catch (e) { /* the number simply does not roll next time */ }
  }

  counts.forEach(function (count) {
    var value = count.querySelector("[data-verso-count-value]");
    if (!value) return;
    var key = "verso-count:" + location.pathname + ":" + count.getAttribute("data-verso-count");
    var now = value.textContent.trim();
    var was = recall(key);
    remember(key, now);
    if (was === null || was === now || still || typeof value.animate !== "function") return;
    window.versoRoll(value, was, now, count);
  });
})();
