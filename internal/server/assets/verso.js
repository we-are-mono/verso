// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// verso.js — the shell's client behaviours (ADR-004). Plugins ship no JS: every
// interaction lives here, registered as Alpine components (the CSP build), so the
// strict CSP needs no unsafe-eval. Widget templates reference only the property
// and method names these components expose.
document.addEventListener("alpine:init", function () {
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
