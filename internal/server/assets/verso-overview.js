// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-overview.js — the Overview's cards (ADR-022): open and close a card,
// and Arrange them. The shell renders the page already in the browser's layout
// (the verso_overview cookie); this script reads the layout back off the page,
// changes it, and writes the cookie. It never reads the cookie itself: the
// shell is its one reader.
//
// The layout is a value, {cols: [[ids], [ids]], closed: [ids], hidden: [ids]},
// and every move below is a pure function of it, returning a new one (or null
// for a move that goes nowhere). The page's own handling is the thin part at
// the end. Event delegation only, never an Alpine expression: CSP mode.
(function () {
  var COOKIE = "verso_overview";
  var VERSION = "1";
  // 400 days, the longest a browser keeps a cookie.
  var MAX_AGE = 34560000;

  function copy(l) {
    return { cols: [l.cols[0].slice(), l.cols[1].slice()], closed: l.closed.slice(), hidden: l.hidden.slice() };
  }
  function without(list, id) {
    return list.filter(function (x) { return x !== id; });
  }
  function find(l, id) {
    for (var c = 0; c < 2; c++) {
      var i = l.cols[c].indexOf(id);
      if (i >= 0) return { col: c, index: i };
    }
    return null;
  }

  // encode writes the layout as the shell reads it: 1.<left>.<right>.<closed>.<hidden>.
  function encode(l) {
    return [VERSION, l.cols[0].join("-"), l.cols[1].join("-"), l.closed.join("-"), l.hidden.join("-")].join(".");
  }
  function cookie(value, protocol) {
    return COOKIE + "=" + value + "; Path=/; Max-Age=" + MAX_AGE + "; SameSite=Lax" + (protocol === "https:" ? "; Secure" : "");
  }
  // forget drops the cookie, so the shell renders its default layout.
  function forget(protocol) {
    return COOKIE + "=; Path=/; Max-Age=0; SameSite=Lax" + (protocol === "https:" ? "; Secure" : "");
  }

  // move puts a card at index in col, wherever it stood before.
  function move(l, id, col, index) {
    var next = copy(l);
    next.cols = [without(next.cols[0], id), without(next.cols[1], id)];
    var at = Math.max(0, Math.min(index, next.cols[col].length));
    next.cols[col].splice(at, 0, id);
    return next;
  }
  // nudge is a move by arrow key: up and down within the column, left and
  // right to the same place in the other one (its end, if it is shorter).
  function nudge(l, id, key) {
    var at = find(l, id);
    if (!at) return null;
    if (key === "ArrowUp") return at.index > 0 ? move(l, id, at.col, at.index - 1) : null;
    if (key === "ArrowDown") return at.index < l.cols[at.col].length - 1 ? move(l, id, at.col, at.index + 1) : null;
    if ((key === "ArrowRight" && at.col === 0) || (key === "ArrowLeft" && at.col === 1)) return move(l, id, 1 - at.col, at.index);
    return null;
  }
  function hide(l, id) {
    var next = copy(l);
    next.cols = [without(next.cols[0], id), without(next.cols[1], id)];
    next.closed = without(next.closed, id);
    if (next.hidden.indexOf(id) < 0) next.hidden.push(id);
    return next;
  }
  // restore brings a hidden card back open at the end of the shorter column,
  // by the columns' drawn heights.
  function restore(l, id, heights) {
    var next = copy(l);
    next.hidden = without(next.hidden, id);
    next.cols[heights[1] < heights[0] ? 1 : 0].push(id);
    return next;
  }
  function toggle(l, id) {
    var next = copy(l);
    next.closed = next.closed.indexOf(id) >= 0 ? without(next.closed, id) : next.closed.concat([id]);
    return next;
  }
  // togglesFrom is whether a click that starts at t, in a card's header, opens
  // or closes the card: the card's own control (the Traffic card's window),
  // its door and any button but the toggle keep their own click.
  function togglesFrom(t) {
    if (t.closest("[data-overview-card-control], a, select, input")) return false;
    return !(t.closest("button") && !t.closest("[data-overview-toggle]"));
  }

  window.versoOverviewLayout = { encode: encode, cookie: cookie, forget: forget, move: move, nudge: nudge, hide: hide, restore: restore, toggle: toggle, togglesFrom: togglesFrom };

  // ── the page ──
  var root = document.querySelector("[data-overview]");
  if (!root) return;
  var tr = typeof T === "function" ? T : function (s) { return s; };
  var ARRANGING = "verso-overview-arranging";

  function columns() {
    return [root.querySelector('[data-overview-col="0"]'), root.querySelector('[data-overview-col="1"]')];
  }
  function cardsIn(col) {
    return Array.prototype.filter.call(col.children, function (el) { return el.hasAttribute("data-overview-card"); });
  }
  function idOf(card) {
    return card.getAttribute("data-overview-card");
  }
  // read is the layout the page shows now.
  function read() {
    var cols = columns().map(function (col) { return cardsIn(col).map(idOf); });
    var closed = Array.prototype.map.call(root.querySelectorAll("[data-overview-card][data-closed]"), idOf);
    var hidden = Array.prototype.map.call(root.querySelectorAll("[data-overview-restore]"), function (el) { return el.getAttribute("data-overview-restore"); });
    return { cols: cols, closed: closed, hidden: hidden };
  }
  function persist(l) {
    document.cookie = cookie(encode(l), window.location.protocol);
  }
  function remember(arranging) {
    try {
      if (arranging) window.sessionStorage.setItem(ARRANGING, "1");
      else window.sessionStorage.removeItem(ARRANGING);
    } catch (e) { /* storage refused: Arrange simply ends with the reload */ }
  }
  // reload asks the shell for the page in its new layout: a hidden card is not
  // rendered, a restored one has to be, and the stream follows the layout from
  // its next connection. Arrange carries over.
  function reload(l) {
    if (l) persist(l);
    else document.cookie = forget(window.location.protocol);
    remember(true);
    window.location.reload();
  }
  var say = root.querySelector("[data-overview-say]");
  function announce(card) {
    if (!say) return;
    var col = card.parentElement.getAttribute("data-overview-col") === "0" ? "%s: left column, position %d." : "%s: right column, position %d.";
    var title = card.querySelector("h2").textContent;
    var position = cardsIn(card.parentElement).indexOf(card) + 1;
    say.textContent = "";
    window.setTimeout(function () { say.textContent = tr(col).replace("%s", title).replace("%d", String(position)); }, 30);
  }
  // place puts the page's cards where the layout says.
  function place(l) {
    var cols = columns();
    l.cols.forEach(function (ids, c) {
      ids.forEach(function (id) {
        var card = root.querySelector('[data-overview-card="' + CSS.escape(id) + '"]');
        if (card) cols[c].appendChild(card);
      });
    });
  }

  function setOpen(card, open) {
    card.toggleAttribute("data-closed", !open);
    var button = card.querySelector("[data-overview-toggle]");
    if (button) {
      button.setAttribute("aria-expanded", open ? "true" : "false");
      button.setAttribute("aria-label", button.getAttribute(open ? "data-close-label" : "data-open-label"));
    }
    var summary = card.querySelector("[data-overview-card-summary]");
    if (summary) summary.setAttribute("aria-hidden", open ? "true" : "false");
  }
  function arrange(on) {
    root.toggleAttribute("data-arranging", on);
    var button = root.querySelector("[data-overview-arrange]");
    if (button) button.setAttribute("aria-pressed", on ? "true" : "false");
    remember(on);
  }
  try {
    if (window.sessionStorage.getItem(ARRANGING)) arrange(true);
  } catch (e) { /* no storage: start at rest */ }

  root.addEventListener("click", function (e) {
    var t = e.target;
    if (t.closest("[data-overview-arrange]")) return arrange(!root.hasAttribute("data-arranging"));
    var restoreChip = t.closest("[data-overview-restore]");
    if (restoreChip) {
      var heights = columns().map(function (col) { return col.getBoundingClientRect().height; });
      return reload(restore(read(), restoreChip.getAttribute("data-overview-restore"), heights));
    }
    if (t.closest("[data-overview-reset]")) return reload(null);
    var card = t.closest("[data-overview-card]");
    if (!card) return;
    if (t.closest("[data-overview-hide]")) return reload(hide(read(), idOf(card)));
    if (root.hasAttribute("data-arranging") || !t.closest("[data-overview-card-head]")) return;
    // The door and any other control in the header keep their own click.
    if (!togglesFrom(t)) return;
    setOpen(card, card.hasAttribute("data-closed"));
    persist(read());
  });

  root.addEventListener("keydown", function (e) {
    var grip = e.target.closest && e.target.closest("[data-overview-grip]");
    if (!grip) return;
    var card = grip.closest("[data-overview-card]");
    var next = nudge(read(), idOf(card), e.key);
    if (!next) return;
    e.preventDefault();
    place(next);
    grip.focus();
    persist(next);
    announce(card);
  });

  // Dragging, while arranging: a header is picked up once the pointer has
  // moved a few pixels, so a press on the grip still only focuses it.
  var drag = null;
  root.addEventListener("pointerdown", function (e) {
    if (!root.hasAttribute("data-arranging") || e.button !== 0) return;
    var head = e.target.closest("[data-overview-card-head]");
    if (!head || e.target.closest("[data-overview-hide]")) return;
    drag = { card: head.closest("[data-overview-card]"), x: e.clientX, y: e.clientY, lifted: false };
  });
  function lift(e) {
    var card = drag.card, r = card.getBoundingClientRect();
    var slot = document.createElement("div");
    slot.className = "verso-overview__slot";
    slot.style.height = r.height + "px";
    card.before(slot);
    card.style.width = r.width + "px";
    card.style.left = r.left + "px";
    card.style.top = r.top + "px";
    card.setAttribute("data-lifted", "");
    document.body.setAttribute("data-verso-dragging", "");
    drag.slot = slot;
    drag.dx = e.clientX - r.left;
    drag.dy = e.clientY - r.top;
    drag.lifted = true;
  }
  document.addEventListener("pointermove", function (e) {
    if (!drag) return;
    if (!drag.lifted) {
      if (Math.abs(e.clientX - drag.x) + Math.abs(e.clientY - drag.y) < 5) return;
      lift(e);
    }
    e.preventDefault();
    drag.card.style.left = e.clientX - drag.dx + "px";
    drag.card.style.top = e.clientY - drag.dy + "px";
    var cols = columns(), r = cols.map(function (col) { return col.getBoundingClientRect(); });
    var side = r[1].left >= r[0].right - 1;
    var mid = function (b) { return b.left + b.width / 2; };
    var target = side
      ? (Math.abs(e.clientX - mid(r[0])) <= Math.abs(e.clientX - mid(r[1])) ? cols[0] : cols[1])
      : (e.clientY >= r[1].top ? cols[1] : cols[0]);
    var before = cardsIn(target).filter(function (c) { return c !== drag.card; }).find(function (c) {
      var b = c.getBoundingClientRect();
      return e.clientY < b.top + b.height / 2;
    });
    if (before) { if (drag.slot.nextSibling !== before) target.insertBefore(drag.slot, before); }
    else if (target.lastElementChild !== drag.slot) target.appendChild(drag.slot);
  });
  function drop() {
    if (!drag) return;
    var d = drag;
    drag = null;
    if (!d.lifted) return;
    d.slot.replaceWith(d.card);
    d.card.removeAttribute("data-lifted");
    d.card.style.width = d.card.style.left = d.card.style.top = "";
    document.body.removeAttribute("data-verso-dragging");
    persist(read());
    announce(d.card);
  }
  document.addEventListener("pointerup", drop);
  document.addEventListener("pointercancel", drop);
})();
