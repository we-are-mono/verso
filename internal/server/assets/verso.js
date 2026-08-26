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
      _return: null,
      show: function () {
        this._return = document.activeElement;
        this.open = true;
        var self = this;
        this.$nextTick(function () {
          if (self.$refs.dialog) self.$refs.dialog.focus();
        });
      },
      hide: function () {
        this.open = false;
        if (this._return && this._return.focus) this._return.focus();
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

// Staged-changes capsule: every edit stages, one pill tells the truth. Nothing
// when clean; when dirty — count · Discard · Review · Apply. Switch flips stage
// here with a real undo (switches inside a drawer's form stage through that
// form's Save instead — the server's side of the contract). Apply plays the
// commit → reload → confirmed sequence with the auto-rollback promise.
(function () {
  var capsule = document.getElementById("verso-capsule");
  if (!capsule) return;
  var text = document.getElementById("verso-capsule-text");
  var listWrap = document.getElementById("verso-capsule-list");
  var listUl = listWrap.querySelector("ul");
  var staged = [];

  function render() {
    text.textContent = staged.length + " staged change" + (staged.length === 1 ? "" : "s");
    capsule.classList.toggle("verso-show", staged.length > 0);
    listUl.innerHTML = "";
    staged.forEach(function (c) {
      var li = document.createElement("li");
      li.textContent = c.label;
      listUl.appendChild(li);
    });
    if (!staged.length) listWrap.classList.add("hidden");
  }
  function stage(label, undo) {
    staged.push({ label: label, undo: undo });
    render();
  }
  function discardAll() {
    staged
      .slice()
      .reverse()
      .forEach(function (c) {
        if (c.undo) c.undo();
      });
    staged = [];
    render();
  }
  function applyAll() {
    capsule.classList.add("verso-busy");
    listWrap.classList.add("hidden");
    text.textContent = "Applying — auto-reverts if the router is unreachable for 30 s…";
    setTimeout(function () {
      capsule.classList.add("verso-done");
      text.textContent = "Applied — firewall reloaded";
      staged = [];
      setTimeout(function () {
        capsule.classList.remove("verso-show");
        setTimeout(function () {
          capsule.classList.remove("verso-busy", "verso-done");
          render();
        }, 260);
      }, 1400);
    }, 1500);
  }

  document.getElementById("verso-capsule-discard").addEventListener("click", discardAll);
  document.getElementById("verso-capsule-apply").addEventListener("click", applyAll);
  document.getElementById("verso-capsule-review").addEventListener("click", function () {
    if (staged.length) listWrap.classList.toggle("hidden");
  });

  document.addEventListener("change", function (e) {
    var sw = e.target.closest("[data-verso-switch]");
    if (!sw || sw.closest("form") || sw.closest('[role="dialog"]')) return;
    var on = sw.checked;
    stage((on ? "Enable " : "Disable ") + (sw.name || "option"), function () {
      sw.checked = !on;
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
      if (q && !d.open && d.querySelector("tbody tr:not(.verso-filter-out)")) {
        d.dataset.versoQuietOpen = "1";
        d.open = true;
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
