// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-page.js — the page's own furniture (ADR-004). Three small things that
// belong to a page rather than to anything on it: where an explanation hangs
// when there is no room below the label that raised it, which section of a
// long page the rail beside it should mark, and where an act made in place
// says how it went.

// A wide tip hangs below the label that raises it, which is where it belongs on
// a page: the label stays readable and the explanation follows the eye down.
// Inside a drawer that scrolls, a label low in the panel has no room below and
// the tip is cut off by the panel's own edge — so on raising one, measure the
// room against whatever would clip it and flip the tip above when there is more
// there. The placement is the shell's, as every behaviour is (ADR-005 §7): the
// widget declares the tip and nothing about where it lands.
(function () {
  var GAP = 8; // the tip's own offset from the label, both ways

  // clipper is the box the tip has to fit inside: the nearest ancestor that
  // scrolls (a drawer's body, a page's overflow region), else the viewport.
  function clipper(el) {
    for (var node = el.parentElement; node; node = node.parentElement) {
      var overflow = getComputedStyle(node).overflowY;
      if (overflow === "auto" || overflow === "scroll") return node.getBoundingClientRect();
    }
    return { top: 0, bottom: window.innerHeight };
  }

  function place(label) {
    var tip = label.querySelector('[role="tooltip"]');
    if (!tip) return;
    // Measure from the default placement, so a tip that has been flipped once
    // can come back down when the panel is scrolled and the room returns.
    label.removeAttribute("data-verso-tip-above");
    var box = clipper(label);
    var rect = label.getBoundingClientRect();
    var height = tip.offsetHeight + GAP;
    var below = box.bottom - rect.bottom;
    var above = rect.top - box.top;
    if (below < height && above > below) label.setAttribute("data-verso-tip-above", "");
  }

  function raised(event) {
    var label = event.target.closest && event.target.closest("[data-verso-tip]");
    if (label) place(label);
  }
  document.addEventListener("pointerover", raised);
  document.addEventListener("focusin", raised);
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

  function show(outcome) {
    clearTimeout(timer);
    slot.replaceChildren(outcome);
    layer.hidden = false;
    // Shown on the next frame, so the layer starts from its off-screen place
    // and the arrival is a slide rather than an appearance.
    requestAnimationFrame(function () {
      requestAnimationFrame(function () { layer.classList.add("is-shown"); });
    });
    stay();
  }

  layer.addEventListener("pointerenter", function () { clearTimeout(timer); });
  layer.addEventListener("pointerleave", stay);
  if (close) close.addEventListener("click", leave);
  window.versoOutcome = { show: show };
})();

// Shared waiting mark: the same four-square motion as the sign-in page.
(function () {
  var motion = window.matchMedia("(prefers-reduced-motion: reduce)");
  var marks = new Map();
  function start(mark) {
    var active = [];
    marks.set(mark, active);
    if (motion.matches || typeof mark.animate !== "function") return;
    var squares = mark.children;
    var colors = getComputedStyle(mark);
    var parked = colors.getPropertyValue("--color-sand-5").trim();
    var travelling = colors.getPropertyValue("--color-meta").trim();
    var corners = [[0, 0], [8, 0], [8, 8], [0, 8]];
    // The design's vq0–vq3 choreography: twelve moves over four seconds.
    // Appearance stays in Tailwind; the Web Animations API supplies motion.
    var paths = [
      [0, 0, 0, 0, 0, 1, 2, 3, 0, 0, 0, 0, 0],
      [0, 1, 1, 1, 1, 1, 2, 3, 0, 1, 2, 3, 0],
      [0, 1, 2, 2, 2, 2, 2, 2, 2, 2, 2, 3, 0],
      [0, 1, 2, 3, 0, 1, 2, 3, 3, 3, 3, 3, 0],
    ];
    var lifted = [
      [4, 5, 6, 7, 8],
      [0, 1, 5, 6, 7, 8, 9, 10, 11, 12],
      [0, 1, 2, 10, 11, 12],
      [0, 1, 2, 3, 4, 5, 6, 7, 11, 12],
    ];
    paths.forEach(function (path, index) {
      var frames = path.map(function (corner, step) {
        var active = lifted[index].indexOf(step) !== -1;
        return {
          offset: step === 12 ? 1 : step * 0.0833,
          transform: "translate(" + (corners[corner][0] - corners[index][0]) + "px," + (corners[corner][1] - corners[index][1]) + "px)",
          backgroundColor: active ? travelling : parked,
          zIndex: active ? 2 : 1,
          easing: "ease",
        };
      });
      active.push(squares[index].animate(frames, { duration: 4000, iterations: Infinity }));
    });
  }
  function sync() {
    marks.forEach(function (animations, mark) {
      if (!mark.isConnected) { animations.forEach(function (a) { a.cancel(); }); marks.delete(mark); }
      else animations.forEach(function (a) { if (mark.hasAttribute("data-verso-wait-paused")) a.pause(); else a.play(); });
    });
    document.querySelectorAll("[data-verso-wait]").forEach(function (mark) { if (!marks.has(mark)) { start(mark); if (mark.hasAttribute("data-verso-wait-paused")) marks.get(mark).forEach(function (a) { a.pause(); }); } });
  }
  motion.addEventListener("change", function () {
    marks.forEach(function (animations) { animations.forEach(function (a) { a.cancel(); }); });
    marks.clear(); sync();
  });
  new MutationObserver(sync).observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["data-verso-wait-paused"] });
  sync();
})();
