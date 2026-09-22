// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-tables.js — reading a listing (ADR-004). Everything that changes which
// rows are on screen or what order they sit in, none of which is a request: the
// rows are all here, and this decides what is drawn and what the file will say
// about their sequence.
//
// Reordering announces itself as a bubbling verso:reorder event and the order
// form listens for it — the form is the truth and the rows follow it, so a
// discard restores the form and the rows drag themselves back to match.

// Reorderable table rows. The shell owns the interaction so plugins only declare
// a reorder column and stable row IDs. Dragging is constrained to the row's
// evaluation group; the drop changes DOM order and announces the table's whole
// new ID sequence as a bubbling verso:reorder event, which the pending-change
// block below writes into the page's order form.
(function () {
  var drag = null;
  var hoverTimer = 0;
  var hoverRow = null;
  var hoverBefore = false;
  var hoverDelay = 80;

  function clearHover() {
    window.clearTimeout(hoverTimer);
    hoverTimer = 0;
    hoverRow = null;
  }

  function stripBehaviour(root) {
    var nodes = [root].concat([].slice.call(root.querySelectorAll("*")));
    nodes.forEach(function (node) {
      [].slice.call(node.attributes || []).forEach(function (attr) {
        if (attr.name === "id" || attr.name.indexOf("x-") === 0 || attr.name.charAt(0) === "@" || attr.name.charAt(0) === ":") {
          node.removeAttribute(attr.name);
        }
      });
      if (/^(BUTTON|INPUT|SELECT|TEXTAREA|A)$/.test(node.tagName)) {
        node.setAttribute("tabindex", "-1");
      }
    });
  }

  function lockColumns(table, row, width) {
    var group = document.createElement("colgroup");
    [].forEach.call(row.cells, function (cell) {
      var col = document.createElement("col");
      col.style.width = cell.getBoundingClientRect().width + "px";
      group.appendChild(col);
    });
    group.dataset.versoReorderLock = "";
    // Colgroups add columns; they do not override an earlier group's widths.
    // Temporarily replace the declared grid with the measured one.
    var originals = [].filter.call(table.children, function (node) { return node.tagName === "COLGROUP"; });
    originals.forEach(function (node) { node.remove(); });
    table.insertBefore(group, table.firstChild);
    var previousLayout = table.style.tableLayout;
    var previousWidth = table.style.width;
    table.style.tableLayout = "fixed";
    table.style.width = width + "px";
    return { group: group, originals: originals, layout: previousLayout, width: previousWidth };
  }

  function floatingRow(row, tableRect, rowRect) {
    var shell = document.createElement("div");
    shell.className = "verso-reorder-float fixed z-[70] overflow-hidden rounded-xs bg-ground ring-1 ring-rule-strong shadow-[0_1.25rem_2.5rem_-0.75rem_rgb(27_25_23/0.24),0_0.5rem_1rem_-0.5rem_rgb(27_25_23/0.18)] pointer-events-none origin-center scale-y-[1.01] opacity-[0.98] will-change-[top,transform] motion-reduce:transform-none [&_td]:border-b-transparent [&_td]:bg-ground";
    // The copy leaves the table wrapper. Carry its cell-inset utilities along
    // so the first and last values stay on the same grid while dragging.
    var wrap = row.closest(".overflow-x-auto");
    if (wrap) [].forEach.call(wrap.classList, function (cls) {
      if (cls.indexOf("[&_td") === 0) shell.classList.add(cls);
    });
    shell.setAttribute("aria-hidden", "true");
    shell.inert = true;
    shell.style.left = tableRect.left + "px";
    shell.style.top = rowRect.top + "px";
    shell.style.width = tableRect.width + "px";
    shell.style.height = rowRect.height + "px";

    var table = document.createElement("table");
    table.className = "w-full table-fixed border-collapse text-sm";
    table.style.width = tableRect.width + "px";
    var body = document.createElement("tbody");
    var copy = document.createElement("tr");
    copy.className = row.className;
    [].forEach.call(row.cells, function (cell) {
      var cloned = cell.cloneNode(true);
      cloned.style.width = cell.getBoundingClientRect().width + "px";
      stripBehaviour(cloned);
      copy.appendChild(cloned);
    });
    body.appendChild(copy);
    table.appendChild(body);
    shell.appendChild(table);
    document.body.appendChild(shell);
    return shell;
  }

  function placeholder(row, height) {
    var tr = document.createElement("tr");
    tr.className = "verso-reorder-placeholder";
    tr.dataset.versoReorderGroup = row.dataset.versoReorderGroup || "";
    var td = document.createElement("td");
    td.className = "border-b border-rule bg-quiet p-0!";
    td.colSpan = row.cells.length;
    var space = document.createElement("div");
    space.className = "w-full";
    space.style.height = Math.max(0, height - 1) + "px";
    td.appendChild(space);
    tr.appendChild(td);
    return tr;
  }

  function rowsInGroup() {
    if (!drag) return [];
    return [].filter.call(drag.table.querySelectorAll("tr[data-verso-reorder-row]"), function (row) {
      return row !== drag.row && row.dataset.versoReorderGroup === drag.group && row.style.display !== "none";
    });
  }

  function animateRows(before) {
    rowsInGroup().forEach(function (row) {
      var oldTop = before.get(row);
      if (oldTop === undefined) return;
      var delta = oldTop - row.getBoundingClientRect().top;
      if (!delta || typeof row.animate !== "function") return;
      row.animate(
        [{ transform: "translateY(" + delta + "px)" }, { transform: "translateY(0)" }],
        { duration: 170, easing: "cubic-bezier(0.2, 0.8, 0.2, 1)" }
      );
    });
  }

  function placeAt(target, beforeTarget) {
    if (!drag || !target || target === drag.row) return;
    var rows = rowsInGroup();
    var before = new Map();
    rows.forEach(function (row) { before.set(row, row.getBoundingClientRect().top); });

    var reference = beforeTarget ? target : target.nextSibling;
    if (reference === drag.row) reference = drag.row.nextSibling;
    target.parentNode.insertBefore(drag.placeholder, reference);
    target.parentNode.insertBefore(drag.row, drag.placeholder.nextSibling);
    animateRows(before);
  }

  function scheduleAt(clientX, clientY) {
    var hit = document.elementFromPoint(clientX, clientY);
    var row = hit && hit.closest && hit.closest("tr[data-verso-reorder-row]");
    if (!row || row === drag.row || row.closest("table") !== drag.table || row.dataset.versoReorderGroup !== drag.group) {
      clearHover();
      return;
    }
    var rect = row.getBoundingClientRect();
    var before = clientY < rect.top + rect.height / 2;
    if (row === hoverRow && before === hoverBefore) return;
    clearHover();
    hoverRow = row;
    hoverBefore = before;
    hoverTimer = window.setTimeout(function () {
      placeAt(row, before);
      hoverTimer = 0;
    }, hoverDelay);
  }

  function move(clientX, clientY) {
    if (!drag) return;
    var top = clientY - drag.offsetY;
    top = Math.max(8, Math.min(window.innerHeight - drag.height - 8, top));
    drag.floating.style.top = top + "px";
    if (clientY < 48) window.scrollBy(0, -10);
    else if (clientY > window.innerHeight - 48) window.scrollBy(0, 10);
    scheduleAt(clientX, clientY);
  }

  function restoreTable() {
    if (!drag) return;
    drag.lock.group.remove();
    drag.lock.originals.slice().reverse().forEach(function (node) { drag.table.insertBefore(node, drag.table.firstChild); });
    drag.table.style.tableLayout = drag.lock.layout;
    drag.table.style.width = drag.lock.width;
  }

  function announceOrder(table, group) {
    var order = [].map.call(table.querySelectorAll("tr[data-verso-reorder-row]"), function (row) {
      return row.dataset.versoReorderId;
    }).filter(Boolean);
    table.dispatchEvent(new CustomEvent("verso:reorder", {
      bubbles: true,
      detail: { group: group, order: order },
    }));
  }

  function complete() {
    if (!drag) return;
    drag.row.style.display = drag.display;
    drag.row.removeAttribute("data-verso-reorder-active");
    drag.placeholder.remove();
    drag.origin.remove();
    drag.floating.remove();
    restoreTable();
    document.body.classList.remove("verso-reordering", "cursor-grabbing!", "select-none!", "[&_*]:cursor-grabbing!", "[&_*]:select-none!");
    announceOrder(drag.table, drag.group);
    drag = null;
  }

  function drop() {
    if (!drag) return;
    clearHover();
    var target = drag.placeholder.getBoundingClientRect();
    var currentTop = parseFloat(drag.floating.style.top) || target.top;
    if (typeof drag.floating.animate !== "function") {
      complete();
      return;
    }
    var animation = drag.floating.animate([
      { top: currentTop + "px", transform: "scaleY(1.01)", opacity: 0.98 },
      { top: target.top + "px", transform: "scaleY(1)", opacity: 1 },
    ], { duration: 150, easing: "cubic-bezier(0.2, 0.8, 0.2, 1)", fill: "forwards" });
    animation.onfinish = complete;
    animation.oncancel = complete;
  }

  function cancel() {
    if (!drag) return;
    clearHover();
    drag.origin.parentNode.insertBefore(drag.placeholder, drag.origin.nextSibling);
    drag.origin.parentNode.insertBefore(drag.row, drag.placeholder.nextSibling);
    complete();
  }

  document.addEventListener("pointerdown", function (event) {
    var handle = event.target.closest && event.target.closest("[data-verso-reorder-handle]");
    if (!handle || drag || event.button !== 0) return;
    var row = handle.closest("tr[data-verso-reorder-row]");
    var table = row && row.closest("table[data-verso-reorder-table]");
    if (!row || !table) return;
    event.preventDefault();

    var rowRect = row.getBoundingClientRect();
    var tableRect = table.getBoundingClientRect();
    var origin = document.createComment("verso-reorder-origin");
    row.parentNode.insertBefore(origin, row);
    var lock = lockColumns(table, row, tableRect.width);
    var floating = floatingRow(row, tableRect, rowRect);
    var gap = placeholder(row, rowRect.height);
    row.parentNode.insertBefore(gap, row);
    var display = row.style.display;
    row.style.display = "none";
    row.setAttribute("data-verso-reorder-active", "");
    document.body.classList.add("verso-reordering", "cursor-grabbing!", "select-none!", "[&_*]:cursor-grabbing!", "[&_*]:select-none!");

    drag = {
      pointer: event.pointerId,
      handle: handle,
      row: row,
      table: table,
      group: row.dataset.versoReorderGroup || "",
      placeholder: gap,
      origin: origin,
      lock: lock,
      floating: floating,
      display: display,
      offsetY: event.clientY - rowRect.top,
      height: rowRect.height,
    };
    try { handle.setPointerCapture(event.pointerId); } catch (e) { /* capture is optional */ }
    move(event.clientX, event.clientY);
  });

  document.addEventListener("pointermove", function (event) {
    if (!drag || event.pointerId !== drag.pointer) return;
    event.preventDefault();
    move(event.clientX, event.clientY);
  }, { passive: false });

  document.addEventListener("pointerup", function (event) {
    if (!drag || event.pointerId !== drag.pointer) return;
    try { drag.handle.releasePointerCapture(event.pointerId); } catch (e) { /* already released */ }
    drop();
  });

  document.addEventListener("pointercancel", function (event) {
    if (!drag || event.pointerId !== drag.pointer) return;
    cancel();
  });

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape" && drag) cancel();
  });

  // The keyboard's way to reorder: an arrow on a focused handle moves its row one
  // place within its group, among the rows the filter still shows, and the
  // handle keeps focus so the next press moves it again. The row moves at once;
  // the order is staged once the keys go quiet, so a run of presses posts one
  // order rather than a race of them.
  var keyed = { timer: 0, table: null, group: "" };

  function stageKeyed() {
    window.clearTimeout(keyed.timer);
    keyed.timer = window.setTimeout(function () {
      if (keyed.table) announceOrder(keyed.table, keyed.group);
      keyed.table = null;
    }, 400);
  }

  document.addEventListener("keydown", function (event) {
    if (drag || (event.key !== "ArrowUp" && event.key !== "ArrowDown")) return;
    if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
    var handle = event.target.closest && event.target.closest("[data-verso-reorder-handle]");
    var row = handle && handle.closest("tr[data-verso-reorder-row]");
    var table = row && row.closest("table[data-verso-reorder-table]");
    if (!table) return;
    event.preventDefault();

    var group = row.dataset.versoReorderGroup || "";
    var peers = [].filter.call(table.querySelectorAll("tr[data-verso-reorder-row]"), function (peer) {
      return peer.dataset.versoReorderGroup === group && peer.getClientRects().length > 0;
    });
    var from = peers.indexOf(row);
    var to = event.key === "ArrowUp" ? from - 1 : from + 1;
    if (from < 0 || to < 0 || to >= peers.length) return;

    var target = peers[to];
    target.parentNode.insertBefore(row, event.key === "ArrowUp" ? target : target.nextSibling);
    handle.focus();
    versoAnnounce(T("Moved to position %d of %d").replace("%d", to + 1).replace("%d", peers.length));

    if (keyed.table && keyed.table !== table) announceOrder(keyed.table, keyed.group);
    keyed.table = table;
    keyed.group = group;
    stageKeyed();
  });
})();

// Opening any <details> (a table seam, a disclosure) reveals content the browser
// won't scroll to on its own — nudge it into view. Short content scrolls minimally
// ("nearest"); content taller than the viewport aligns its summary to the top so
// the reader starts at the beginning. `toggle` doesn't bubble, so listen in capture.
document.addEventListener(
  "toggle",
  function (e) {
    var d = e.target;
    if (!(d instanceof HTMLDetailsElement) || !d.open) return;
    // The page filter opens seams quietly to reveal matches — no scrolling then.
    if (d.dataset.versoQuietOpen) {
      delete d.dataset.versoQuietOpen;
      return;
    }
    var reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    requestAnimationFrame(function () {
      var fits = d.getBoundingClientRect().height < window.innerHeight * 0.8;
      d.scrollIntoView({ block: fits ? "nearest" : "start", behavior: reduce ? "auto" : "smooth" });
    });
  },
  true
);

// Page-wide filter (the still-lens): one field narrows every listing at once,
// and typing never moves the page — non-matching rows dim in place, zero-match
// sections ghost whole, and a collapsed seam opens only when it holds a match.
// "/" focuses the field from anywhere; Escape clears and releases it. The
// sentinel div rendered just before the dock drives the pinned state.
(function () {
  var input = document.querySelector("[data-verso-filter]");
  if (!input) return;
  var dock = input.closest(".verso-filter-dock");
  var sentinel = dock && dock.previousElementSibling;
  if (dock && sentinel && "IntersectionObserver" in window) {
    new IntersectionObserver(function (entries) {
      dock.classList.toggle("verso-filter-stuck", !entries[0].isIntersecting);
    }).observe(sentinel);
  }
  function apply() {
    var q = input.value.trim().toLowerCase();
    [].forEach.call(document.querySelectorAll("main tbody tr"), function (tr) {
      tr.classList.toggle("verso-filter-out", !!q && tr.textContent.toLowerCase().indexOf(q) === -1);
    });
    [].forEach.call(document.querySelectorAll("main details"), function (d) {
      var seam = d.closest("tbody");
      var seamMatch = seam && seam.querySelector(".verso-table-seam-row:not(.verso-filter-out)");
      if (q && !d.open && (d.querySelector("tbody tr:not(.verso-filter-out)") || seamMatch)) {
        d.dataset.versoQuietOpen = "1";
        d.open = true;
        var control = d.closest(".verso-table-seam-control");
        if (control) control.classList.remove("verso-filter-out");
      }
    });
    [].forEach.call(document.querySelectorAll("main section"), function (sec) {
      var rows = sec.querySelectorAll("tbody tr");
      if (!rows.length) return;
      var visible = 0;
      [].forEach.call(rows, function (r) {
        if (!r.classList.contains("verso-filter-out")) visible++;
      });
      sec.classList.toggle("verso-filter-ghost", !!q && visible === 0);
    });
  }
  input.addEventListener("input", apply);
  document.addEventListener("keydown", function (e) {
    var active = document.activeElement;
    var typing = active && /^(input|select|textarea)$/i.test(active.tagName);
    if (e.key === "/" && !typing) {
      e.preventDefault();
      input.focus();
    }
    if (e.key === "Escape" && active === input) {
      input.value = "";
      apply();
      input.blur();
    }
  });
})();

// Dropped rows are a staged change, not a write. The reorderable table renders
// the page form beside itself, holding the config and one hidden input per row id
// in current order; a drop rewrites that sequence and posts the form where it
// stands, so the new order is in the stage the moment the row lands — the
// staged-changes chip counts it, and Apply makes it live. The whole table's ids
// travel, in DOM order, because the file holds one sequence — the drag is
// constrained to a group, the order is not.
(function () {
  function orderForm(config) {
    return [].filter.call(document.querySelectorAll("form[data-verso-page-form]"), function (form) {
      var declared = form.querySelector('input[name="_uci_order_config"]');
      return !!declared && declared.value === config;
    })[0];
  }

  function writeSequence(form, order) {
    var field = form.querySelector('[data-verso-change-name="_uci_order"]');
    if (!field) return;
    field.replaceChildren();
    order.forEach(function (id) {
      var input = document.createElement("input");
      input.type = "hidden";
      input.name = "_uci_order";
      input.value = id;
      field.appendChild(input);
    });
  }

  // The drop is the save: the order posts to the page it sits on, the answer is
  // the page re-rendered with that order staged, and the chip is brought in
  // step from it. A refusal, or a router out of reach, reloads: the page is
  // what says where the stage still has the rows.
  function stage(form) {
    fetch(window.location.pathname, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams(new FormData(form)).toString(),
      credentials: "same-origin",
    }).then(function (res) {
      if (!res.ok && !res.redirected) throw new Error("order refused");
      return res.text();
    }).then(function (html) {
      var doc = new DOMParser().parseFromString(html, "text/html");
      if (window.versoStaged && window.versoStaged.sync) window.versoStaged.sync(doc);
    }).catch(function () {
      if (window.versoDirtyState) window.versoDirtyState.suppress();
      window.location.reload();
    });
  }

  document.addEventListener("verso:reorder", function (e) {
    var table = e.target;
    if (!table || !table.dataset || !table.dataset.versoReorderConfig) return;
    var form = orderForm(table.dataset.versoReorderConfig);
    if (!form) return;
    writeSequence(form, (e.detail && e.detail.order) || []);
    stage(form);
  });
})();

// The listing's action bar narrows what is already on screen: a tab cuts by the
// flags a row carries, the select by one of its facets, the field by the words
// in it. None of it is a request — the rows are all here, and the bar only
// decides which of them are drawn. A group band disappears with the last row it
// introduces, so a network with nothing left does not sit there as an empty
// heading.
(function () {
 function initialize(scope) {
  var bar = scope.querySelector("[data-verso-actionbar]");
  if (!bar || bar.dataset.versoFilterReady) return;
  bar.dataset.versoFilterReady = "true";
  // The rows the bar narrows: a grid's tbody, or a console's block of lines.
  // Both are a flat run of row elements in render order, which is all the walk
  // below needs.
  var rows = scope.querySelector("[data-verso-console-rows]");
  var table = rows || scope.querySelector("table");
  if (!table) return;

  var tabs = [].slice.call(bar.querySelectorAll("[data-verso-tab]"));
  var cut = bar.querySelector("[data-verso-listing-cut]");
  var field = bar.querySelector("[data-verso-listing-filter]");
  var select = bar.querySelector("[data-verso-listing-select]");
  var facetKey = select ? select.getAttribute("data-verso-listing-select") : "";

  // On a live listing the counts are not a fact the page was rendered with —
  // they are what is on screen this second, and they climb as events arrive. So
  // the bar prices its own cuts from the rows it can see, and re-prices them
  // whenever the set of rows changes.
  function recount() {
    if (!rows) return;
    var lines = rows.querySelectorAll(":scope > div:not([data-verso-stream-empty])");
    function priced(tag) {
      var n = 0;
      for (var j = 0; j < lines.length; j++) {
        var carried = " " + (lines[j].getAttribute("data-verso-tags") || "") + " ";
        if (!tag || carried.indexOf(" " + tag + " ") !== -1) n++;
      }
      return n;
    }
    for (var i = 0; i < tabs.length; i++) {
      var slot = tabs[i].querySelector("span");
      if (slot) slot.textContent = String(priced(tabs[i].getAttribute("data-verso-tab") || ""));
    }
    if (cut) {
      for (var k = 0; k < cut.options.length; k++) {
        var option = cut.options[k];
        option.textContent = option.getAttribute("data-label") + " · " + priced(option.value);
      }
    }
  }

  // Row order matters here: a band governs every row after it until the next
  // band, so the pass walks the body once, in order, and hides a band only once
  // it knows nothing under it survived.
  function apply() {
    var tag = "";
    for (var i = 0; i < tabs.length; i++) {
      if (tabs[i].getAttribute("data-active") === "true") tag = tabs[i].getAttribute("data-verso-tab") || "";
    }
    if (cut) tag = cut.value;
    var needle = field ? (field.value || "").trim().toLowerCase() : "";
    var facet = select ? select.value : "";
    if (table.closest("[data-verso-inventory]")) table.dataset.filtered = String(!!(needle || tag));

    var band = null;
    var bandKept = 0;
    var matched = 0;
    var list = table.querySelectorAll("tbody > tr, [data-verso-console-rows] > div");
    for (var r = 0; r < list.length; r++) {
      var row = list[r];
      if (row.hasAttribute("data-verso-stream-empty")) continue;
      if (row.hasAttribute("data-verso-expanded-row")) {
        var parent = row.previousElementSibling;
        var trigger = parent && parent.querySelector("[data-verso-expand-row]");
        row.hidden = !parent || parent.hidden || !trigger || trigger.getAttribute("aria-expanded") !== "true";
        continue;
      }
      if (row.classList.contains("verso-table-group")) {
        if (band) band.hidden = bandKept === 0;
        band = row;
        bandKept = 0;
        continue;
      }
      var keep = true;
      if (tag) {
        var tags = " " + (row.getAttribute("data-verso-tags") || "") + " ";
        keep = tags.indexOf(" " + tag + " ") !== -1;
      }
      if (keep && facet && facetKey) {
        keep = row.getAttribute("data-verso-facet-" + facetKey) === facet;
      }
      if (keep && needle) {
        keep = (row.textContent || "").toLowerCase().indexOf(needle) !== -1;
      }
      row.hidden = !keep;
      if (keep) { bandKept++; matched++; }
    }
    if (band) band.hidden = bandKept === 0;
    var empty = scope.querySelector("[data-verso-listing-empty], [data-verso-stream-empty]");
    if (empty) {
      // The moment the cut leaves nothing is said, since the rows vanish
      // without anything taking focus; while it keeps matching, nothing is.
      if (empty.hidden && matched === 0) versoAnnounce(empty.textContent.trim());
      empty.hidden = matched > 0;
    }
  }

  for (var i = 0; i < tabs.length; i++) {
    // A problem filter is a chip pressed on and off, not one of a set.
    tabs[i].addEventListener("click", function (e) {
      var pressed = e.currentTarget.getAttribute("aria-pressed") !== "true";
      e.currentTarget.setAttribute("data-active", String(pressed));
      e.currentTarget.setAttribute("aria-pressed", String(pressed));
      apply();
    });
  }
  bar.versoApplyFilters = apply;
  if (field) field.addEventListener("input", apply);
  if (select) select.addEventListener("change", apply);
  if (cut) cut.addEventListener("change", apply);
  // A live listing's rows arrive after this ran, so the bar watches for them:
  // every batch re-prices the cuts and re-applies the one in force, and a line
  // that arrives under a cut it does not survive is hidden as it lands.
  if (rows && window.MutationObserver) {
    recount();
    new MutationObserver(function () {
      recount();
      apply();
    }).observe(rows, { childList: true });
  }
  apply();
 }
 window.versoTableFilters = initialize;
 initialize(document);
})();

// Only the identity opens a row. Keep one open in the inventory, as in the
// reference; the remaining cells stay available for selection and copying.
function versoExpandRow(button, open) {
  var row = button.closest("tr");
  var detail = row && row.nextElementSibling;
  if (!detail || !detail.hasAttribute("data-verso-expanded-row")) return;
  detail.hidden = !open;
  row.dataset.expanded = String(open);
  button.setAttribute("aria-expanded", String(open));
  var chevron = button.querySelector("[data-verso-expand-chevron] svg") || button.querySelector("svg");
  if (chevron && !row.closest("[data-verso-inventory]")) chevron.classList.toggle("rotate-90", open);
}
document.addEventListener("click", function (event) {
  var button = event.target.closest("[data-verso-expand-row]");
  if (!button) return;
  var open = button.getAttribute("aria-expanded") !== "true";
  var inventory = button.closest("[data-verso-inventory]");
  if (inventory) inventory.querySelectorAll('[data-verso-expand-row][aria-expanded="true"]').forEach(function (other) { if (other !== button) versoExpandRow(other, false); });
  versoExpandRow(button, open);
});

// Refresh shell-rendered live tables so topology and service state cannot
// drift into separate client-side implementations. Keep the user's open row,
// search, scroll position and focus; no polling while a dialog is in use.
(function () {
  var inventories = document.querySelectorAll("[data-verso-inventory], [data-verso-live-table]");
  inventories.forEach(function (inventory, index) {
    var busy = false;
    var interval = setInterval(async function () {
      if (!inventory.isConnected) { clearInterval(interval); return; }
      if (busy || document.hidden || inventory.matches(":active")) return;
      if ([].some.call(document.querySelectorAll('[role="dialog"]'), function (d) { return d.getClientRects().length > 0; })) return;
      var selection = window.getSelection();
      if (selection && !selection.isCollapsed && inventory.contains(selection.anchorNode)) return;
      busy = true;
      var controller = new AbortController();
      var timeout = setTimeout(function () { controller.abort(); }, 8000);
      try {
        var response = await fetch(location.pathname + location.search, { credentials: "same-origin", cache: "no-store", headers: { "X-Verso-Refresh": "1" }, signal: controller.signal });
        if (!response.ok) throw new Error("inventory unavailable");
        var doc = new DOMParser().parseFromString(await response.text(), "text/html");
        var source = doc.querySelectorAll("[data-verso-inventory], [data-verso-live-table]")[index];
        var fresh = source && source.querySelector("tbody");
        if (!fresh || !inventory.isConnected) throw new Error("inventory unavailable");
        var current = inventory.querySelector("tbody");
        var trigger = current.querySelector('[data-verso-expand-row][aria-expanded="true"]');
        var openID = trigger && trigger.closest("tr").getAttribute("data-verso-row-id");
        var focused = document.activeElement;
        var focusedRow = inventory.contains(focused) && focused.closest("[data-verso-row-id]");
        var focusedID = focusedRow && focusedRow.getAttribute("data-verso-row-id");
        var focusedLabel = focusedID && focused.getAttribute("aria-label");
        current.replaceChildren(...fresh.childNodes);
        if (focusedID && focusedLabel) {
          var replacement = current.querySelector('[data-verso-row-id="' + CSS.escape(focusedID) + '"] [aria-label="' + CSS.escape(focusedLabel) + '"]');
          if (replacement) replacement.focus({ preventScroll: true });
        }
        if (openID) {
          var next = current.querySelector('[data-verso-row-id="' + CSS.escape(openID) + '"] [data-verso-expand-row]');
          if (next) versoExpandRow(next, true);
        }
        var bar = document.querySelector("[data-verso-actionbar]");
        var problem = bar && bar.querySelector("[data-verso-problem-filter]");
        var nextProblem = doc.querySelector("[data-verso-problem-filter]");
        if (problem && nextProblem) {
          problem.hidden = nextProblem.hidden;
          problem.querySelector("[data-verso-problem-count]").textContent = nextProblem.querySelector("[data-verso-problem-count]").textContent;
          if (problem.hidden) { problem.setAttribute("aria-pressed", "false"); problem.dataset.active = "false"; }
        }
        if (bar && bar.versoApplyFilters) bar.versoApplyFilters();
        inventory.querySelector("[data-verso-inventory-stale]").hidden = true;
      } catch (_) {
        inventory.querySelector("[data-verso-inventory-stale]").hidden = false;
      } finally { clearTimeout(timeout); busy = false; }
    }, 10000);
  });
})();
