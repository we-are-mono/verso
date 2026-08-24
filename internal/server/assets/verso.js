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
