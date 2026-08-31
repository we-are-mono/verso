// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// verso.js — the shell's client behaviours (ADR-004). Plugins ship no JS: every
// interaction lives here, registered as Alpine components (the CSP build), so the
// strict CSP needs no unsafe-eval. Widget templates reference only the property
// and method names these components expose.
document.addEventListener("alpine:init", function () {
  // copy: copy the widget's text to the clipboard and briefly show "Copied!".
  // Prefers the async Clipboard API; falls back to a hidden-textarea execCommand
  // for non-secure origins (a router reached over plain http on the LAN).
  Alpine.data("copy", function () {
    return {
      idle: true,
      done: false,
      run: function () {
        var src = this.$refs.src;
        var text = src ? src.textContent : "";
        var self = this;
        var flash = function () {
          self.idle = false;
          self.done = true;
          setTimeout(function () {
            self.done = false;
            self.idle = true;
          }, 1500);
        };
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(flash, function () {
            self.fallback(text);
            flash();
          });
        } else {
          self.fallback(text);
          flash();
        }
      },
      fallback: function (text) {
        try {
          var ta = document.createElement("textarea");
          ta.value = text;
          ta.style.position = "fixed";
          ta.style.opacity = "0";
          document.body.appendChild(ta);
          ta.select();
          document.execCommand("copy");
          document.body.removeChild(ta);
        } catch (e) {
          /* clipboard unavailable; nothing more we can do */
        }
      },
    };
  });

  // advseam: the Advanced-settings sidebar seam. Toggles its open state (the template
  // binds it to aria-expanded; the grid-rows CSS animates the reveal) and remembers the
  // choice across loads. Default collapsed. CSP-safe: directives are property/method refs.
  Alpine.data("advseam", function () {
    return {
      open: false,
      init: function () {
        // The saved state was applied to <html> pre-paint by verso-boot.js; mirror it
        // into `open` so the button's aria-expanded reflects it for assistive tech.
        this.open = document.documentElement.classList.contains("verso-adv-open");
      },
      toggle: function () {
        this.open = document.documentElement.classList.toggle("verso-adv-open");
        try {
          localStorage.setItem("verso-adv", this.open ? "1" : "0");
        } catch (e) {
          /* storage blocked; state is still live this session */
        }
      },
    };
  });

  // sidebar: the mobile off-canvas navigation drawer. Opens the rail behind a hamburger;
  // the scrim, Escape, or a nav tap (a full-page load) closes it. No persistence — closed
  // by default on every page.
  Alpine.data("sidebar", function () {
    return {
      open: false,
      // The CSP-friendly Alpine build can't evaluate a ternary inside :class, so expose
      // the drawer's open-class as a plain (reactive) property to bind to.
      get drawerClass() {
        return this.open ? "verso-drawer-open" : "";
      },
      toggle: function () {
        this.open = !this.open;
      },
      close: function () {
        this.open = false;
      },
      // Flip light/dark by toggling .dark on <html> and remembering it. The palette
      // variables under :root.dark (input.css) do the rest.
      toggleTheme: function () {
        // Freeze transitions for the single frame the palette swaps, so filled
        // elements (the active row, the cog) flip instantly instead of animating
        // their colour change — otherwise the switch reads as a janky blink.
        var el = document.documentElement;
        el.classList.add("verso-theming");
        var dark = el.classList.toggle("dark");
        try {
          localStorage.setItem("verso-theme", dark ? "dark" : "light");
        } catch (e) {
          /* storage blocked; theme is still live this session */
        }
        requestAnimationFrame(function () {
          requestAnimationFrame(function () {
            el.classList.remove("verso-theming");
          });
        });
      },
    };
  });

  // modal: an overlay dialog. Open/close, focus the dialog on open, return focus
  // on close, close on Escape, and trap Tab within the dialog while open.
  Alpine.data("modal", function () {
    return {
      open: false,
      idle: true,
      busy: false,
      _return: null,
      _dirty: false,
      init: function () {
        this.open = this.$el && this.$el.dataset.open === "true";
        if (this.open) {
          var self = this;
          this.$nextTick(function () {
            if (self.$refs.dialog) self.$refs.dialog.focus();
          });
        }
      },
      show: function () {
        this._return = document.activeElement;
        this._dirty = false;
        this.idle = true;
        this.busy = false;
        this.open = true;
        var self = this;
        this.$nextTick(function () {
          if (self.$refs.dialog) self.$refs.dialog.focus();
        });
      },
      // Any edit inside the dialog marks it dirty (the panels bind @input/@change
      // to this); closing a dirty dialog asks first — Save means kept, closing
      // means lost, and losing work silently is never fine.
      markDirty: function () {
        this._dirty = true;
      },
      hide: function () {
		if (this.busy) return;
        if (this._dirty && !window.confirm("You have unsaved changes. Close without saving?")) {
          return;
        }
        this._dirty = false;
        this.open = false;
        if (this._return && this._return.focus) this._return.focus();
      },
      startBusy: function () {
        this._dirty = false;
        this.idle = false;
        this.busy = true;
      },
      // A table row as trigger: open the drawer unless the click landed on a
      // control inside the row (a toggle's label, a link, a button) — those keep
      // their own meaning.
      showFromRow: function (e) {
        if (e && e.target && e.target.closest("label,input,button,a,select,textarea")) return;
        this.show();
      },
      onKeydown: function (e) {
        if (!this.open) return;
        if (e.key === "Escape") {
          this.hide();
          return;
        }
        if (e.key !== "Tab") return;
        var el = this.$refs.dialog;
        if (!el) return;
        var f = el.querySelectorAll(
          'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])'
        );
        if (!f.length) {
          e.preventDefault();
          el.focus();
          return;
        }
        var first = f[0],
          last = f[f.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      },
    };
  });
});

// Leaving the page with unsaved form edits loses them — warn first. Dirtiness is
// a comparison with the rendered baseline, never a history of input events: a
// toggle changed twice is clean again. The capsule contributes its composed page
// form as a separate source below; ordinary forms (including drawer forms) are
// tracked here. The browser renders its native prompt.
(function () {
  var baselines = new Map();
  var sources = Object.create(null);
  var suppressed = false;

  function signature(form) {
    var values = [];
    [].forEach.call(form.elements || [], function (control) {
      if (!control.name || control.name === "_csrf" || control.disabled) return;
      var type = (control.type || "").toLowerCase();
      if (type === "submit" || type === "button" || type === "reset") return;
      if ((type === "checkbox" || type === "radio") && !control.checked) return;
      if (type === "file") {
        var files = [].map.call(control.files || [], function (file) { return file.name; });
        values.push([control.name, files]);
        return;
      }
      values.push([control.name, control.value]);
    });
    return JSON.stringify(values);
  }

  function track(form) {
    if (!form || baselines.has(form) || form.hasAttribute("data-verso-page-form")) return;
    baselines.set(form, signature(form));
  }

  function refreshForms() {
    var dirty = false;
    baselines.forEach(function (baseline, form) {
      if (form.isConnected && signature(form) !== baseline) dirty = true;
    });
    sources.forms = dirty;
  }

  [].forEach.call(document.querySelectorAll("form"), track);
  new MutationObserver(function (records) {
    records.forEach(function (record) {
      [].forEach.call(record.addedNodes, function (node) {
        if (!node.querySelectorAll) return;
        if (node.matches && node.matches("form")) track(node);
        [].forEach.call(node.querySelectorAll("form"), track);
      });
    });
  }).observe(document.body, { childList: true, subtree: true });

  window.versoDirtyState = {
    set: function (source, dirty) {
      sources[source] = !!dirty;
    },
    suppress: function () {
      suppressed = true;
    },
    resume: function () {
      suppressed = false;
    },
    reset: function () {
      baselines.forEach(function (_, form) {
        if (form.isConnected) form.reset();
      });
      sources.forms = false;
    },
  };

  function changed(e) {
    var form = e.target.closest && e.target.closest("form");
    if (!form || form.hasAttribute("data-verso-page-form")) return;
    track(form);
    refreshForms();
  }
  document.addEventListener("input", changed, true);
  document.addEventListener("change", changed, true);
  document.addEventListener("submit", function () { suppressed = true; }, true);
  window.addEventListener("beforeunload", function (e) {
    refreshForms();
    var dirty = Object.keys(sources).some(function (key) { return sources[key]; });
    if (suppressed || !dirty) return;
    e.preventDefault();
    e.returnValue = ""; // required by Chromium for the prompt to show
  });
})();

// Compact multi-value inputs. Each chip owns a hidden input, preserving the same
// repeated-field POST contract as the expanded list widget.
(function () {
  function addToken(input) {
    var list = input.closest("[data-verso-token-list]");
    var value = input.value.trim().replace(/,$/, "");
    if (!list || !value) return;
    var exists = [].some.call(list.querySelectorAll("[data-verso-token] input"), function (item) {
      return item.value === value;
    });
    if (exists) {
      input.value = "";
      return;
    }
    var chip = document.createElement("span");
    chip.dataset.versoToken = "";
    chip.className = "inline-flex items-center gap-1 rounded-md bg-slate-100 py-1 pr-1 pl-2 font-mono text-sm font-semibold text-slate-700 dark:bg-gray-700 dark:text-gray-200";
    var hidden = document.createElement("input");
    hidden.type = "hidden";
    hidden.name = list.dataset.versoTokenName;
    hidden.value = value;
    var label = document.createElement("span");
    label.textContent = value;
    var remove = document.createElement("button");
    remove.type = "button";
    remove.dataset.versoTokenRemove = "";
    remove.setAttribute("aria-label", "Remove " + value);
    remove.className = "grid size-5 place-items-center rounded text-slate-400 transition-colors hover:bg-slate-200 hover:text-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-600 dark:hover:bg-gray-600 dark:hover:text-gray-100";
    remove.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="size-3.5" aria-hidden="true"><path d="M18 6 6 18M6 6l12 12"/></svg>';
    chip.appendChild(hidden);
    chip.appendChild(label);
    chip.appendChild(remove);
    list.insertBefore(chip, input);
    input.value = "";
    list.dispatchEvent(new Event("input", { bubbles: true }));
  }

  document.addEventListener("keydown", function (event) {
    var input = event.target.closest && event.target.closest("[data-verso-token-input]");
    if (!input || (event.key !== "Enter" && event.key !== ",")) return;
    event.preventDefault();
    addToken(input);
  });
  document.addEventListener("focusout", function (event) {
    var input = event.target.closest && event.target.closest("[data-verso-token-input]");
    if (input) addToken(input);
  });
  document.addEventListener("click", function (event) {
    var remove = event.target.closest && event.target.closest("[data-verso-token-remove]");
    if (remove) {
      var list = remove.closest("[data-verso-token-list]");
      remove.closest("[data-verso-token]").remove();
      if (list) list.dispatchEvent(new Event("input", { bubbles: true }));
    }
  });
})();

// Typed optional-condition builders. Plugins declare the full catalogue in
// schema; the shell owns adding/removing the ordinary form widgets on demand.
(function () {
  function refresh(builder) {
    var empty = builder.querySelector("[data-verso-condition-empty]");
    var list = builder.querySelector("[data-verso-condition-list]");
    if (empty && list) empty.classList.toggle("hidden", list.children.length !== 0);
  }

  document.addEventListener("click", function (event) {
    var add = event.target.closest && event.target.closest("[data-verso-condition-add]");
    if (add) {
      var builder = add.closest("[data-verso-conditions]");
      var select = builder && builder.querySelector("[data-verso-condition-select]");
      var key = select && select.value;
      var template = key && builder.querySelector('template[data-verso-condition-template="' + CSS.escape(key) + '"]');
      var list = builder && builder.querySelector("[data-verso-condition-list]");
      if (!template || !list) return;
      list.appendChild(template.content.cloneNode(true));
      var option = select.querySelector('option[value="' + CSS.escape(key) + '"]');
      if (option) {
        option.disabled = true;
        option.hidden = true;
      }
      select.value = "";
      refresh(builder);
      builder.dispatchEvent(new Event("input", { bubbles: true }));
      return;
    }

    var remove = event.target.closest && event.target.closest("[data-verso-condition-remove]");
    if (!remove) return;
    var item = remove.closest("[data-verso-condition]");
    var builder = remove.closest("[data-verso-conditions]");
    if (!item || !builder) return;
    var key = item.dataset.versoCondition;
    var option = builder.querySelector('option[value="' + CSS.escape(key) + '"]');
    if (option) {
      option.disabled = false;
      option.hidden = false;
    }
    item.remove();
    refresh(builder);
    builder.dispatchEvent(new Event("input", { bubbles: true }));
  });
})();

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

  function lockColumns(table, row) {
    var group = document.createElement("colgroup");
    [].forEach.call(row.cells, function (cell) {
      var col = document.createElement("col");
      col.style.width = cell.getBoundingClientRect().width + "px";
      group.appendChild(col);
    });
    group.dataset.versoReorderLock = "";
    table.insertBefore(group, table.firstChild);
    var previousLayout = table.style.tableLayout;
    table.style.tableLayout = "fixed";
    return { group: group, layout: previousLayout };
  }

  function floatingRow(row, tableRect, rowRect) {
    var shell = document.createElement("div");
    shell.className = "verso-reorder-float";
    shell.setAttribute("aria-hidden", "true");
    shell.inert = true;
    shell.style.left = tableRect.left + "px";
    shell.style.top = rowRect.top + "px";
    // Keep the cloned table on its original grid and extend only the card's
    // trailing edge. The action remains aligned with the source row while the
    // wider card gives it room inside the rounded border.
    shell.style.width = tableRect.width + 16 + "px";
    shell.style.height = rowRect.height + "px";

    var table = document.createElement("table");
    table.className = "text-sm";
    table.style.width = tableRect.width + "px";
    var body = document.createElement("tbody");
    var copy = document.createElement("tr");
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
    td.colSpan = row.cells.length;
    var space = document.createElement("div");
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
    drag.table.style.tableLayout = drag.lock.layout;
  }

  function announceOrder() {
    if (!drag) return;
    var order = [].map.call(drag.table.querySelectorAll("tr[data-verso-reorder-row]"), function (row) {
      return row.dataset.versoReorderId;
    }).filter(Boolean);
    drag.table.dispatchEvent(new CustomEvent("verso:reorder", {
      bubbles: true,
      detail: { group: drag.group, order: order },
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
    document.body.classList.remove("verso-reordering");
    announceOrder();
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
    var gap = placeholder(row, rowRect.height);
    row.parentNode.insertBefore(gap, row);
    var lock = lockColumns(table, row);
    var floating = floatingRow(row, tableRect, rowRect);
    var display = row.style.display;
    row.style.display = "none";
    row.setAttribute("data-verso-reorder-active", "");
    document.body.classList.add("verso-reordering");

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

// Staged-changes capsule (ADR-010): the pill and its review list are
// server-rendered from UCI's own stage — this client only drives the three
// posts. Discard reverts and reloads. Apply commits with the device-side
// rollback armed, then polls confirm inside the window (the LuCI cadence);
// confirmed, it shows the applied state and reloads into a clean page. If
// confirm never lands, the router reverts itself — the capsule says so.
(function () {
  var capsule = document.getElementById("verso-capsule");
  if (!capsule) return;
  var text = document.getElementById("verso-capsule-text");
  var title = document.getElementById("verso-capsule-list-title");
  var localWrap = document.getElementById("verso-capsule-local-changes");
  var listWrap = document.getElementById("verso-capsule-list");
  var review = document.getElementById("verso-capsule-review");
  var closeReview = document.getElementById("verso-capsule-close");
  var applyButton = document.getElementById("verso-capsule-apply");
  var discardButton = document.getElementById("verso-capsule-discard");
  var dot = document.getElementById("verso-capsule-dot");
  var stagedCount = parseInt(capsule.getAttribute("data-staged-count") || "0", 10);
  var csrf = capsule.getAttribute("data-csrf") || "";
  var pageForm = document.querySelector("form[data-verso-page-form]");
  var baseline = pageForm ? collect(pageForm) : new Map();
  var pristinePageForm = pageForm ? pageForm.cloneNode(true) : null;
  var localChanges = [];

  function fieldState(wrapper) {
    var name = wrapper.getAttribute("data-verso-change-name") || "";
    var label = wrapper.getAttribute("data-verso-change-label") || name;
    var kind = wrapper.getAttribute("data-verso-change-kind") || "text";
    var controls = [].filter.call(wrapper.querySelectorAll("[name]"), function (control) {
      return control.name === name && !control.disabled;
    });
    var values = [];
    var displays = [];

    controls.forEach(function (control) {
      var type = (control.type || "").toLowerCase();
      if ((type === "checkbox" || type === "radio") && !control.checked) return;
      if (type === "file") {
        [].forEach.call(control.files || [], function (file) {
          values.push(file.name);
          displays.push(file.name);
        });
        return;
      }
      var value = control.value;
      if (kind === "list" && value.trim() === "") return;
      values.push(value);
      if (control.tagName === "SELECT") {
        var option = control.options[control.selectedIndex];
        displays.push(option ? option.textContent.trim() : value);
      } else if (type === "checkbox" && kind === "checks") {
        var optionLabel = control.closest("label");
        displays.push(optionLabel ? optionLabel.textContent.trim() : value);
      } else {
        displays.push(value);
      }
    });

    if (kind === "checks") {
      values.sort();
      displays.sort();
    }
    var display;
    if (kind === "toggle") display = values.length ? "On" : "Off";
    else if (kind === "password") display = values.some(Boolean) ? "Set" : "Not set";
    else display = displays.filter(Boolean).join(", ") || "Not set";
    return { name: name, label: label, kind: kind, key: JSON.stringify(values), display: display };
  }

  function collect(form) {
    var fields = new Map();
    if (!form) return fields;
    [].forEach.call(form.querySelectorAll("[data-verso-change-field]"), function (wrapper) {
      var state = fieldState(wrapper);
      if (state.name && !fields.has(state.name)) fields.set(state.name, state);
    });
    return fields;
  }

  function pendingLabel(count) {
    return count === 1 ? "1 pending change" : count + " pending changes";
  }

  function appendText(parent, tag, className, value) {
    var el = document.createElement(tag);
    el.className = className;
    el.textContent = value;
    parent.appendChild(el);
    return el;
  }

  function renderChanges(changes) {
    if (!localWrap) return;
    localWrap.replaceChildren();
    localWrap.classList.toggle("hidden", changes.length === 0);
    if (!changes.length) return;

    var scroll = document.createElement("div");
    scroll.className = "verso-drawer-scrollbar max-h-80 overflow-y-auto pr-1";
    var header = document.createElement("div");
    header.className = "hidden grid-cols-3 gap-4 border-b border-slate-200 pb-2 text-xs font-medium text-slate-400 sm:grid dark:border-gray-700 dark:text-gray-500";
    ["Field", "Previous", "New"].forEach(function (value) { appendText(header, "span", "", value); });
    scroll.appendChild(header);
    var list = document.createElement("ul");
    list.className = "divide-y divide-slate-200 dark:divide-gray-700";
    changes.forEach(function (change) {
      var row = document.createElement("li");
      row.className = "py-2 first:pt-1.5 last:pb-0 sm:grid sm:grid-cols-3 sm:gap-4";
      appendText(row, "div", "text-sm font-medium text-slate-700 dark:text-gray-300", change.label);
      var values = document.createElement("div");
      values.className = "mt-2 grid grid-cols-2 gap-4 sm:contents";
      var previous = document.createElement("div");
      previous.className = "min-w-0";
      appendText(previous, "div", "mb-1 text-xs font-medium text-slate-400 sm:hidden dark:text-gray-500", "Previous");
      appendText(previous, "div", "break-words font-mono text-base font-medium text-slate-500 dark:text-gray-400", change.previous);
      var next = document.createElement("div");
      next.className = "min-w-0";
      appendText(next, "div", "mb-1 text-xs font-medium text-slate-400 sm:hidden dark:text-gray-500", "New");
      appendText(next, "div", "break-words font-mono text-base font-semibold text-slate-900 dark:text-gray-100", change.next);
      values.appendChild(previous);
      values.appendChild(next);
      row.appendChild(values);
      list.appendChild(row);
    });
    scroll.appendChild(list);
    localWrap.appendChild(scroll);
  }

  function setReviewOpen(open) {
    if (!listWrap) return;
    listWrap.classList.toggle("is-open", !!open);
    capsule.classList.toggle("verso-review-open", !!open);
    listWrap.setAttribute("aria-hidden", open ? "false" : "true");
    if (review) review.setAttribute("aria-expanded", open ? "true" : "false");
  }

  function reviewIsOpen() {
    return !!(listWrap && listWrap.classList.contains("is-open"));
  }

  function refresh() {
    if (pageForm) {
      var current = collect(pageForm);
      localChanges = [];
      var names = new Set();
      baseline.forEach(function (_, name) { names.add(name); });
      current.forEach(function (_, name) { names.add(name); });
      names.forEach(function (name) {
        var before = baseline.get(name) || { label: name, key: "[]", display: "Not set" };
        var after = current.get(name) || { label: before.label, key: "[]", display: "Not set" };
        if (before.key !== after.key) {
          localChanges.push({ label: after.label || before.label, previous: before.display, next: after.display });
        }
      });
    }
    var total = stagedCount + localChanges.length;
    var label = total ? pendingLabel(total) : "No pending changes";
    text.textContent = label;
    if (title) title.textContent = label;
    applyButton.disabled = total === 0;
    discardButton.disabled = total === 0;
    if (review) review.disabled = total === 0;
    if (dot) {
      dot.classList.toggle("bg-sky-500", total > 0);
      dot.classList.toggle("bg-slate-300", total === 0);
      dot.classList.toggle("dark:bg-gray-600", total === 0);
    }
    if (!total) setReviewOpen(false);
    renderChanges(localChanges);
    if (window.versoDirtyState) window.versoDirtyState.set("capsule", localChanges.length > 0);
  }

  function bindPageForm() {
    if (!pageForm) return;
    pageForm.addEventListener("input", refresh);
    pageForm.addEventListener("change", refresh);
  }

  function replacePageForm(replacement, makeBaseline) {
    if (!pageForm || !replacement) return;
    pageForm.replaceWith(replacement);
    pageForm = replacement;
    if (makeBaseline) {
      baseline = collect(pageForm);
      pristinePageForm = pageForm.cloneNode(true);
    }
    bindPageForm();
    refresh();
  }

  // Restoring the baseline puts the form's values back, but a control the shell
  // renders elsewhere on the page — a listing's row order, say — mirrors the
  // form rather than living in it. Announcing the restore lets such a control
  // read the baseline back off the form and follow it.
  function announceRestored() {
    if (!pageForm) return;
    pageForm.dispatchEvent(new CustomEvent("verso:capsule-restored", { bubbles: true }));
  }

  function resetRenderedForms() {
    if (window.versoDirtyState) window.versoDirtyState.reset();
    if (pageForm && pristinePageForm) replacePageForm(pristinePageForm.cloneNode(true), false);
    setReviewOpen(false);
    announceRestored();
  }

  function post(path) {
    return fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: "_csrf=" + encodeURIComponent(csrf),
    });
  }

  if (review && listWrap) {
    review.addEventListener("click", function () {
      if (review.disabled) return;
      setReviewOpen(!reviewIsOpen());
    });
    document.addEventListener("click", function (event) {
      if (reviewIsOpen() && !capsule.contains(event.target)) setReviewOpen(false);
    });
    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape") setReviewOpen(false);
    });
  }

  if (closeReview && listWrap) {
    closeReview.addEventListener("click", function () {
      setReviewOpen(false);
      if (review) review.focus();
    });
  }

  bindPageForm();
  refresh();

  discardButton.addEventListener("click", function () {
    if (discardButton.disabled) return;
    if (!stagedCount) {
      resetRenderedForms();
      return;
    }
    capsule.classList.add("verso-busy");
    post("/uci/discard")
      .then(function (res) {
        if (!res.ok) throw new Error("discard failed");
        return fetch(location.href, { headers: { Accept: "text/html" } });
      })
      .then(function (res) {
        if (!res.ok) throw new Error("refresh failed");
        return res.text();
      })
      .then(function (html) {
        var parsed = new DOMParser().parseFromString(html, "text/html");
        var replacement = parsed.querySelector("form[data-verso-page-form]");
        if (!replacement || !pageForm) {
          if (window.versoDirtyState) window.versoDirtyState.suppress();
          location.reload();
          return;
        }
        stagedCount = 0;
        var staged = document.getElementById("verso-capsule-staged-changes");
        if (staged) staged.remove();
        replacePageForm(replacement, true);
        announceRestored();
        resetRenderedForms();
        capsule.classList.remove("verso-busy");
      })
      .catch(function () {
        capsule.classList.remove("verso-busy");
        text.textContent = "Couldn’t discard — try again";
      });
  });

  applyButton.addEventListener("click", function () {
    if (applyButton.disabled) return;
    capsule.classList.add("verso-busy");
    setReviewOpen(false);

    function apply() {
      text.textContent = "Applying — auto-reverts if the router is unreachable for 30 s…";
      var deadline = Date.now() + 28000;

      function confirmLoop() {
        post("/uci/confirm")
          .then(function (res) {
            if (res.ok) {
              capsule.classList.add("verso-done");
              text.textContent = "Applied";
              setTimeout(function () {
                if (window.versoDirtyState) window.versoDirtyState.suppress();
                location.reload();
              }, 900);
              return;
            }
            retry();
          })
          .catch(retry);
      }
      function retry() {
        if (Date.now() < deadline) {
          setTimeout(confirmLoop, 500);
          return;
        }
        capsule.classList.remove("verso-busy");
        text.textContent = "Couldn’t confirm — the router may have rolled back";
      }

      post("/uci/apply")
        .then(function (res) {
          if (!res.ok) {
            capsule.classList.remove("verso-busy");
            text.textContent = "Couldn’t apply — check the settings and try again";
            return;
          }
          setTimeout(confirmLoop, 1000);
        })
        .catch(function () {
          // The apply itself may have severed our path (a network change); keep
          // trying to confirm — reaching the router again is the success signal.
          setTimeout(confirmLoop, 1000);
        });
    }

    // A composed page form has no competing Save button: prepare and validate
    // its plugin intent first, then apply the resulting UCI stage. Validation
    // replaces just the form so the fixed navigation and capsule stay put.
    if (!pageForm || localChanges.length === 0) {
      apply();
      return;
    }
    text.textContent = "Saving changes…";
    fetch(location.href, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams(new FormData(pageForm)).toString(),
    })
      .then(function (res) {
        return res.text().then(function (html) { return { res: res, html: html }; });
      })
      .then(function (reply) {
        if (reply.res.status === 422) {
          var parsed = new DOMParser().parseFromString(reply.html, "text/html");
          var replacement = parsed.querySelector("form[data-verso-page-form]");
          if (replacement) {
            replacePageForm(replacement, false);
          }
          capsule.classList.remove("verso-busy");
          text.textContent = "Check the highlighted fields";
          return;
        }
        if (!reply.res.ok) throw new Error("save failed");
        apply();
      })
      .catch(function () {
        capsule.classList.remove("verso-busy");
        text.textContent = "Couldn’t save — try again";
      });
  });
})();

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

// The overview stream: one EventSource the shell pushes fresh truth into —
// system meters, interface rates, WAN traffic, and sensors. Each type updates
// its rendered widget in place (the CSS transitions do the
// glides). Reconnection after a drop is EventSource's own; when the session
// ends the reconnect lands on the login redirect — not an event stream —
// which closes the client for good.
(function () {
  if (!window.EventSource) return;
  if (!document.querySelector("[data-verso-meter]") && !document.querySelector("[data-verso-row]") && !document.querySelector("[data-verso-chart]") && !document.querySelector("[data-verso-traffic-chart]") && !document.querySelector("[data-verso-prop]")) return;
  // The meter's track tints by its health band (the tone vocabulary).
  var BAR_BANDS = {
    success: "bg-emerald-600",
    warning: "bg-amber-500",
    danger: "bg-red-600",
    info: "bg-sky-600",
  };
  function setText(root, selector, text) {
    var el = root.querySelector(selector);
    if (el) el.textContent = text;
  }
  function applyMeter(reading) {
    var root = document.querySelector('[data-verso-meter="' + CSS.escape(reading.name) + '"]');
    if (!root) return;
    setText(root, "[data-verso-meter-value]", reading.value);
    setText(root, "[data-verso-meter-unit]", reading.unit);
    setText(root, "[data-verso-meter-detail]", reading.detail);
    var fill = Math.min(100, Math.max(0, reading.fill));
    var bar = root.querySelector("[data-verso-meter-bar]");
    if (bar) {
      bar.style.width = fill + "%";
      // A role-accented bar keeps its fixed colour (a dashboard hue, not a health
      // band); only a band-coloured bar recolours with its reading.
      if (!reading.role) {
        for (var bb in BAR_BANDS) bar.classList.remove(BAR_BANDS[bb]);
        bar.classList.add(BAR_BANDS[reading.band] || BAR_BANDS.success);
      }
    }
    root.setAttribute("aria-label", (reading.label + " " + reading.value + " " + reading.unit).trim());
  }
  function applyInterface(iface) {
    var rows = document.querySelectorAll("[data-verso-row]");
    var root = null, key = "interface:" + iface.name;
    rows.forEach(function (row) {
      if (row.getAttribute("data-verso-row") === key) root = row;
    });
    if (!root) return;
    var state = root.querySelector('[data-verso-cell="state"]');
    if (state) {
      setText(state, "[data-verso-value]", iface.state);
      var dot = state.querySelector("[data-verso-dot]");
      if (dot) {
        ["bg-emerald-500", "bg-amber-500", "bg-red-500", "bg-slate-300"].forEach(function (c) { dot.classList.remove(c); });
        dot.classList.add(iface.variant === "success" ? "bg-emerald-500" : iface.variant === "warning" ? "bg-amber-500" : iface.variant === "danger" ? "bg-red-500" : "bg-slate-300");
      }
    }
    setText(root, '[data-verso-cell="rx-rate"]', iface.rx_rate);
    setText(root, '[data-verso-cell="tx-rate"]', iface.tx_rate);
  }
  // applyChart redraws a named chart from fresh series — the same geometry
  // the server drew (viewBox coordinates from the svg itself), so the live
  // layer and the first paint never disagree. Series pair with the rendered
  // groups by order; the role hook lets an idle-grey chart take its colours
  // when traffic starts.
  var CHART_ROLES = ["sky", "violet", "emerald", "amber", "idle"];
  function applyChart(dev) {
    var svg = document.querySelector('svg[data-verso-chart="' + CSS.escape(dev.key) + '"]');
    if (!svg) return;
    var vb = svg.viewBox.baseVal;
    var W = vb.width, H = vb.height, PAD = 8;
    var series = [dev.down || [], dev.up || []];
    var max = 0;
    series.forEach(function (vals) {
      vals.forEach(function (v) {
        if (v > max) max = v;
      });
    });
    var active = max > 0.01;
    max *= 1.15;
    if (max <= 0) max = 1;
    var y = function (v) {
      return (H - PAD - (v / max) * (H - 2 * PAD)).toFixed(1);
    };
    svg.querySelectorAll("g.verso-chart-series").forEach(function (g, i) {
      var vals = series[i];
      if (!vals || vals.length < 2) return;
      var pts = vals.map(function (v, j) {
        return ((j / (vals.length - 1)) * W).toFixed(1) + " " + y(v);
      });
      var line = g.querySelector(".verso-chart-line");
      if (line) line.setAttribute("d", "M" + pts.join(" L"));
      var area = g.querySelector(".verso-chart-area");
      if (area) area.setAttribute("d", "M0 " + H + " L" + pts.join(" L") + " L" + W + " " + H + " Z");
      var dot = g.querySelector(".verso-chart-dot");
      if (dot) dot.setAttribute("cy", y(vals[vals.length - 1]));
      var role = g.getAttribute("data-verso-chart-role") || "sky";
      CHART_ROLES.forEach(function (r) {
        g.classList.remove("verso-chart--" + r);
      });
      g.classList.add("verso-chart--" + (active ? role : "idle"));
    });
    // The value labels ride their gridlines: quarters of the new range, the
    // unit staying on the topmost (captured from the rendered text once).
    var fmt = function (v) {
      return v >= 10 ? Math.round(v).toString() : (Math.round(v * 10) / 10).toString();
    };
    var labels = svg.parentElement.querySelectorAll(".verso-chart-yl");
    labels.forEach(function (el, i) {
      var frac = [1, 0.75, 0.5, 0.25][i];
      if (frac === undefined) return;
      if (el.dataset.unit === undefined) {
        el.dataset.unit = (el.textContent.match(/[\d.]+\s*(.*)$/) || ["", ""])[1];
      }
      var v = max * frac;
      el.textContent = el.dataset.unit ? fmt(v) + " " + el.dataset.unit : fmt(v);
    });
    // The readout above the plot shows each series' newest value — whole
    // numbers only, the readout stays calm.
    var block = document.querySelector('[data-verso-chart-block="' + CSS.escape(dev.key) + '"]');
    if (block) {
      block.querySelectorAll("[data-verso-chart-rate-v]").forEach(function (el, i) {
        var vals = series[i];
        if (vals && vals.length) el.textContent = String(Math.round(vals[vals.length - 1]));
      });
    }
    // The running totals on the device's stat tiles.
    setStat(dev.key + ":down", fmtBytes(dev.rx));
    setStat(dev.key + ":up", fmtBytes(dev.tx));
    setStat(dev.key + ":conns", [String(dev.conns || 0), ""]);
  }
  // fmtBytes mirrors the server's byte formatting: whole bytes, then one
  // decimal per binary step.
  function fmtBytes(n) {
    n = n || 0;
    if (n < 1024) return [String(n), "B"];
    var units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
    var i = -1;
    do {
      n /= 1024;
      i++;
    } while (n >= 1024 && i < units.length - 1);
    return [n.toFixed(1), units[i]];
  }
  function setStat(name, parts) {
    var tile = document.querySelector('[data-verso-stat="' + CSS.escape(name) + '"]');
    if (!tile) return;
    var v = tile.querySelector("[data-verso-stat-v]");
    if (v) v.textContent = parts[0];
    var u = tile.querySelector("[data-verso-stat-u]");
    if (u) u.textContent = parts[1];
  }
  function listen(es, type, key, apply) {
    es.addEventListener(type, function (e) {
      var data;
      try {
        data = JSON.parse(e.data);
      } catch (err) {
        return;
      }
      if (data && data[key]) data[key].forEach(apply);
    });
  }
  var es = new EventSource("/overview/events");
  listen(es, "meters", "meters", applyMeter);
  listen(es, "interfaces", "interfaces", applyInterface);
  listen(es, "traffic", "devices", applyChart);
  // The WAN throughput sample hands off to the traffic-graph animator, if present.
  es.addEventListener("wan", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    var uptime = document.querySelector('[data-verso-tile-caption="internet-uptime"]');
    if (uptime && d && typeof d.uptime === "number") {
      var total = Math.max(0, Math.floor(d.uptime));
      var days = Math.floor(total / 86400);
      var hours = Math.floor(total / 3600) % 24;
      var minutes = Math.floor(total / 60) % 60;
      uptime.textContent = "for " + (days > 0 ? days + "d " + hours + "h " + minutes + "m" : hours > 0 ? hours + "h " + minutes + "m" : minutes + "m");
    }
    if (d && window.__versoWanSample) window.__versoWanSample(d.down, d.up);
  });
  // The hardware-sensor frame updates the System panel's temperature/fan/power
  // rows in place; an absent reading (a row hidden at load) has no element to
  // find, so it is silently skipped. The temperature dot recolours to match.
  function setSensor(name, val) {
    if (val == null) return;
    var el = document.querySelector('[data-verso-prop="' + name + '"]');
    if (el) el.textContent = val;
  }
  es.addEventListener("sensors", function (e) {
    var d;
    try { d = JSON.parse(e.data); } catch (err) { return; }
    if (!d) return;
    setSensor("temperature", d.temperature);
    setSensor("fan", d.fan);
    setSensor("power", d.power);
    setSensor("summary", d.summary);
    var dot = document.querySelector('[data-verso-prop-dot="temperature"]');
    if (dot && d.tempLevel) {
      var color = d.tempLevel === "danger" ? "bg-red-500" : d.tempLevel === "warning" ? "bg-amber-500" : "bg-emerald-500";
      dot.className = "mr-2 inline-block size-1.5 rounded-full align-middle " + color;
    }
  });
})();

// Named switches post themselves: flipping an on/off control outside a form is
// a complete instruction (ADR-010 reads them as instant interactions), so the
// change POSTs {name: "on"|"off"} with the session's CSRF token to the current
// page and reloads on success. A switch inside a <form> belongs to that form's
// submit; a nameless switch is presentation only — both are left alone.
(function () {
  document.addEventListener("change", function (e) {
    var el = e.target;
    if (!el || !el.matches || !el.matches("input[data-verso-switch][name]")) return;
    if (el.closest("form")) return;
    var meta = document.querySelector('meta[name="verso-csrf"]');
    var body = new URLSearchParams();
    body.set(el.name, el.checked ? "on" : "off");
    if (meta) body.set("_csrf", meta.content);
    el.disabled = true; // one flip, one round-trip; the reload re-renders truth
    fetch(window.location.pathname, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "X-Verso-Interaction": "switch",
      },
      body: body.toString(),
      credentials: "same-origin",
    }).then(function (res) {
      if (res.ok || res.redirected) {
        window.location.reload();
      } else {
        el.disabled = false;
        el.checked = !el.checked; // the device said no; show the truth
      }
    }).catch(function () {
      el.disabled = false;
      el.checked = !el.checked;
    });
  });
})();

// Dropped rows are a pending change, not a write. The reorderable table renders
// the page form beside itself, holding the config and one hidden input per row id
// in current order; a drop rewrites that sequence, and the capsule counts it,
// reviews it, and stages the uci order when Save & Apply posts the form. The whole
// table's ids travel, in DOM order, because the file holds one sequence — the drag
// is constrained to a group, the order is not.
//
// The form is the truth and the rows follow it: Discard restores the form's
// baseline and announces it, and the rows drag themselves back to match.
(function () {
  function orderForm(config) {
    return [].filter.call(document.querySelectorAll("form[data-verso-page-form]"), function (form) {
      var declared = form.querySelector('input[name="_uci_order_config"]');
      return !!declared && declared.value === config;
    })[0];
  }

  function orderTable(config) {
    return [].filter.call(document.querySelectorAll("table[data-verso-reorder-table]"), function (table) {
      return table.dataset.versoReorderConfig === config;
    })[0];
  }

  function sequence(form) {
    return [].map.call(form.querySelectorAll('input[name="_uci_order"]'), function (input) {
      return input.value;
    });
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

  // syncRows puts the rendered rows into the form's sequence. Each row's current
  // position is a slot its group keeps — a group header introduces the run below
  // it — so only which row sits in each slot changes, and the headers stay put.
  function syncRows(table, order) {
    var rows = [].slice.call(table.querySelectorAll("tr[data-verso-reorder-row]"));
    if (rows.length === 0) return;
    var byID = new Map();
    rows.forEach(function (row) { byID.set(row.dataset.versoReorderId, row); });
    var wanted = new Map();
    order.forEach(function (id) {
      var row = byID.get(id);
      if (!row) return;
      var group = row.dataset.versoReorderGroup || "";
      if (!wanted.has(group)) wanted.set(group, []);
      wanted.get(group).push(row);
    });
    var slots = rows.map(function (row) {
      var marker = document.createComment("verso-reorder-slot");
      row.parentNode.insertBefore(marker, row);
      return { marker: marker, group: row.dataset.versoReorderGroup || "" };
    });
    var filled = new Map();
    slots.forEach(function (slot) {
      var next = filled.get(slot.group) || 0;
      filled.set(slot.group, next + 1);
      var row = (wanted.get(slot.group) || [])[next];
      if (row) slot.marker.parentNode.insertBefore(row, slot.marker);
    });
    slots.forEach(function (slot) { slot.marker.remove(); });
  }

  document.addEventListener("verso:reorder", function (e) {
    var table = e.target;
    if (!table || !table.dataset || !table.dataset.versoReorderConfig) return;
    var form = orderForm(table.dataset.versoReorderConfig);
    if (!form) return;
    writeSequence(form, (e.detail && e.detail.order) || []);
    form.dispatchEvent(new Event("change", { bubbles: true }));
  });

  document.addEventListener("verso:capsule-restored", function (e) {
    var form = e.target;
    if (!form || !form.querySelector) return;
    var declared = form.querySelector('input[name="_uci_order_config"]');
    if (!declared) return;
    var table = orderTable(declared.value);
    if (table) syncRows(table, sequence(form));
  });
})();

// Auto-submit file forms: choosing a file in a form marked data-verso-autosubmit
// submits it immediately, anywhere on the page — not only inside a modal. These
// upload forms carry NoSubmit, so this is their only submit path.
(function () {
  document.addEventListener("change", function (e) {
    var input = e.target;
    if (!input || input.type !== "file" || !input.files || input.files.length === 0) return;
    var form = input.closest("form[data-verso-autosubmit]");
    if (form && form.requestSubmit) form.requestSubmit();
  });
})();

// The overview's Internet-traffic graph, live: it seeds from the real WAN minute
// the server rendered (data-verso-traffic-seed) and then FLOWS left continuously
// as fresh samples arrive on the overview stream's `wan` event (window.__verso-
// WanSample). Each frame the curves translate by a fraction of one sample; when a
// sample lands the value is committed and the translate resets — a seamless
// treadmill drawing the same smooth curve and dynamic shared scale the server
// drew. A clip hides what scrolls past the edges. Reduced-motion still updates,
// just in discrete steps instead of a glide.
(function () {
  var host = document.querySelector("[data-verso-traffic-chart]");
  if (!host) return;
  var svg = host.querySelector("svg.verso-chart");
  if (!svg) return;
  var reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  var NS = "http://www.w3.org/2000/svg";
  var vb = svg.viewBox.baseVal;
  var W = vb.width, H = vb.height, PAD = 8, INTERVAL = 1000;
  var groups = svg.querySelectorAll("g.verso-chart-series");
  var dots = host.querySelectorAll(".verso-chart-dot-html");
  var downEl = document.querySelector("[data-verso-traffic-down]");
  var upEl = document.querySelector("[data-verso-traffic-up]");

  // Seed the live series from the server's rendered minute.
  var seed = { down: [], up: [] };
  try { seed = JSON.parse(host.getAttribute("data-verso-traffic-seed") || "{}"); } catch (e) { seed = {}; }
  var down = (seed.down || []).slice(), up = (seed.up || []).slice();
  var N = Math.min(down.length, up.length); // visible points fill 0..W
  if (N < 2) return;
  down.push(down[N - 1]); up.push(up[N - 1]); // one extra, off-right, until a sample lands
  var step = W / (N - 1);
  var series = [down, up];

  // Download and upload share one domain so their relative sizes remain
  // truthful. Recalculate it from the whole visible minute whenever a sample
  // arrives; the 15% headroom matches the server-rendered chart and keeps the
  // tallest point clear of the top edge. Including the off-right point grows
  // the scale just before that new value scrolls into view.
  var max = 1;
  function scaleMax() {
    var peak = 0;
    series.forEach(function (vals) {
      vals.forEach(function (v) {
        if (typeof v === "number" && isFinite(v) && v > peak) peak = v;
      });
    });
    return peak > 0 ? peak * 1.15 : 1;
  }
  function axisValue(v) {
    return v >= 10 ? Math.round(v).toString() : (Math.round(v * 10) / 10).toString();
  }
  function updateScale() {
    max = scaleMax();
    host.querySelectorAll(".verso-chart-yl").forEach(function (el, i) {
      var frac = [1, 0.75, 0.5, 0.25][i];
      if (frac === undefined) return;
      if (el.dataset.unit === undefined) {
        el.dataset.unit = (el.textContent.match(/[\d.]+\s*(.*)$/) || ["", ""])[1];
      }
      el.textContent = axisValue(max * frac) + (el.dataset.unit ? " " + el.dataset.unit : "");
    });
  }

  // Clip each series to the plot rect so the treadmill's off-edge content hides.
  var clip = document.createElementNS(NS, "clipPath");
  clip.setAttribute("id", "verso-traffic-clip");
  var rect = document.createElementNS(NS, "rect");
  rect.setAttribute("x", "0"); rect.setAttribute("y", "0");
  rect.setAttribute("width", String(W)); rect.setAttribute("height", String(H));
  clip.appendChild(rect); svg.appendChild(clip);

  var lines = [], areas = [];
  groups.forEach(function (g) {
    g.setAttribute("clip-path", "url(#verso-traffic-clip)");
    lines.push(g.querySelector(".verso-chart-line"));
    areas.push(g.querySelector(".verso-chart-area"));
  });

  function y(v) { return H - PAD - (v / max) * (H - 2 * PAD); }
  function points(vals) { return vals.map(function (v, j) { return [j * step, y(v)]; }); }
  // The same Catmull-Rom spline the server draws, so the live curve matches.
  function segments(p) {
    var s = "";
    for (var k = 0; k < p.length - 1; k++) {
      var p0 = k > 0 ? p[k - 1] : p[k], p1 = p[k], p2 = p[k + 1], p3 = k + 2 < p.length ? p[k + 2] : p2;
      var lo = Math.min(p1[1], p2[1]), hi = Math.max(p1[1], p2[1]);
      var c1y = Math.max(lo, Math.min(hi, p1[1] + (p2[1] - p0[1]) / 6));
      var c2y = Math.max(lo, Math.min(hi, p2[1] - (p3[1] - p1[1]) / 6));
      s += " C" + (p1[0] + (p2[0] - p0[0]) / 6).toFixed(1) + " " + c1y.toFixed(1) +
        " " + (p2[0] - (p3[0] - p1[0]) / 6).toFixed(1) + " " + c2y.toFixed(1) +
        " " + p2[0].toFixed(1) + " " + p2[1].toFixed(1);
    }
    return s;
  }
  function draw() {
    updateScale();
    series.forEach(function (vals, idx) {
      var p = points(vals), head = "M" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p);
      if (lines[idx]) { lines[idx].setAttribute("d", head); lines[idx].setAttribute("transform", "translate(0 0)"); }
      if (areas[idx]) { areas[idx].setAttribute("d", "M0 " + H.toFixed(1) + " L" + p[0][0].toFixed(1) + " " + p[0][1].toFixed(1) + segments(p) + " L" + p[N][0].toFixed(1) + " " + H.toFixed(1) + " Z"); areas[idx].setAttribute("transform", "translate(0 0)"); }
      if (dots[idx]) dots[idx].style.top = ((y(vals[N - 1]) / H) * 100).toFixed(1) + "%";
    });
  }

  var lastCommit = null;
  // A fresh WAN sample: commit the newest down/up, drop the oldest, redraw.
  window.__versoWanSample = function (nd, nu) {
    if (typeof nd !== "number" || typeof nu !== "number") return;
    down.push(nd); down.shift();
    up.push(nu); up.shift();
    draw();
    if (downEl) downEl.textContent = (Math.round(nd * 10) / 10).toFixed(1);
    if (upEl) upEl.textContent = (Math.round(nu * 10) / 10).toFixed(1);
    lastCommit = performance.now();
  };

  draw();
  if (reduce) return; // discrete live updates only; no glide
  function frame(ts) {
    if (lastCommit !== null) {
      var progress = Math.min(1, (ts - lastCommit) / INTERVAL);
      var tf = "translate(" + (-progress * step).toFixed(2) + " 0)";
      lines.forEach(function (l) { if (l) l.setAttribute("transform", tf); });
      areas.forEach(function (a) { if (a) a.setAttribute("transform", tf); });
      series.forEach(function (vals, idx) {
        if (!dots[idx]) return;
        var v = vals[N - 1] * (1 - progress) + vals[N] * progress;
        dots[idx].style.top = ((y(v) / H) * 100).toFixed(1) + "%";
      });
    }
    requestAnimationFrame(frame);
  }
  requestAnimationFrame(frame);
})();
