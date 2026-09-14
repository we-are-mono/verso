// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// verso.js — the shell's client behaviours (ADR-004). Plugins ship no JS: every
// interaction is the shell's, and none of it is evaluated from a string, so the
// strict CSP needs no unsafe-eval. Widget templates reference only the property
// and method names these components expose.
//
// This file is what the rest of it stands on: the translator for the strings a
// script writes into a page, and the Alpine components the chrome is made of —
// the thing that copies, the rail that slides in on a phone, and the panel every
// drawer and dialog in the app is. It loads first, because T() is the one name
// every other file uses.
//
// The behaviours themselves sit beside it, one file per concern, each loaded after
// this one and in no order relative to each other:
//
//   verso-forms.js     filling a form in — rich controls, unsaved-work guard
//   verso-tables.js    reading a listing — ordering, narrowing, revealing
//   verso-commit.js    the stage — the chip and its drawer, the rollback, the session
//   verso-stream.js    the overview's live numbers and its traffic curve
//   verso-listing.js   a listing whose rows arrive over time
//   verso-takeover.js  the screen that holds someone through an upgrade
//   verso-page.js      a page's own furniture — tips, the section rail, the outcome
//
// They were one 3,250-line file. Nothing about the code needed them together: no
// file here calls into another at load, and the three things they do share
// (whether there is unsaved work, whether a field is half-typed, what the stage
// holds) were already passed between them on window, guarded, inside handlers.
// What the single file cost was the ability to read one behaviour without
// scrolling past nine others.

// Server-rendered translations for the strings this script writes into the page
// (ADR-012): client JS has no translator, so the render localizes a fixed set
// of keys into the #verso-i18n JSON blob. A missing blob or key falls back to
// the English source, exactly like the server's own translator.
var versoI18N = {};
try {
  var versoI18NEl = document.getElementById("verso-i18n");
  if (versoI18NEl) versoI18N = JSON.parse(versoI18NEl.textContent) || {};
} catch (e) {
  /* malformed blob; English fallback */
}
function T(s) {
  return Object.prototype.hasOwnProperty.call(versoI18N, s) && versoI18N[s] ? versoI18N[s] : s;
}

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
      _root: null,
      _tab: "",
      _closed: "",
      init: function () {
        // $el resolves to whatever element an expression is evaluated on, so a
        // method called from a nested button sees that button. Keep the
        // component's own root from here, where $el is still it.
        this._root = this.$el;
        if (!this.$el || this.$el.dataset.open !== "true") return;
        // A panel the address asked for still arrives the way a panel arrives:
        // from the edge. Opening it on the next frame rather than during init
        // is what lets the enter transition run — set here, it would already be
        // in its final place and would simply appear, which reads as a glitch
        // rather than as something opening.
        var self = this;
        requestAnimationFrame(function () {
          self.open = true;
          self.$nextTick(function () {
            if (self.$refs.dialog) self.$refs.dialog.focus();
          });
        });
      },
      show: function () {
        this._return = document.activeElement;
        this.idle = true;
        this.busy = false;
        this.loadEntity(this._tab || "");
        this.loadPanel();
        this._tab = "";
        this.open = true;
        var self = this;
        this.$nextTick(function () {
          if (self.$refs.dialog) {
            // A refreshed listing creates new teleported forms outside htmx's
            // swap target. Activate them before the drawer can be submitted.
            if (window.htmx) window.htmx.process(self.$refs.dialog);
            self.$refs.dialog.focus();
          }
        });
      },
      // Closing asks nothing, because closing loses nothing.
      //
      // It used to confirm, on the reasoning that Save means kept and closing
      // means lost. That reasoning was wrong about this code: the panel is shown
      // and hidden, never built and torn down, so what is typed into it is still
      // there when it opens again — half-finished values, added conditions and
      // all. The prompt was asking permission to discard something it was not
      // going to discard, and it fired on Escape, which is the fastest way
      // anybody closes anything.
      //
      // What genuinely loses the work is leaving the page, and that is still
      // guarded — by the browser's own prompt, from the dirty-state tracker in
      // verso-forms.js, where it is about something real.
      hide: function () {
        if (this.busy) return;
        this.open = false;
        if (this._return && this._return.focus) this._return.focus();
        this.releaseAddress();
        // Closing is something a script may be waiting on — a page that went
        // stale under its panel reloads once the panel is gone — so the root
        // announces it, whichever way it was closed.
        if (this._root) this._root.dispatchEvent(new CustomEvent("verso-panel-hidden", { bubbles: true }));
      },
      // The panel's submission is done and the object it was about is in the
      // stage: the panel closes, and what it holds is let go once it is out of
      // sight, so the next open reads the object as it now is rather than
      // showing a form that was true a moment ago — its tab strip, its values,
      // the notice a refusal left. The wait is the leave transition's 200 ms
      // and a margin; emptying the frame before the slide is over would blank
      // the panel mid-motion.
      finish: function () {
        this.hide();
        var panel = this.teleported("[data-verso-panel]");
        if (!panel) return;
        setTimeout(function () {
          panel.replaceChildren();
          panel.removeAttribute("data-verso-panel-loaded");
        }, 250);
      },
      // A panel the address opened is a place, so closing it has to leave that
      // place: otherwise the screen says "listing" while the address still says
      // "listing with this open", and the next reload reopens something the
      // person already dismissed. The page states where it goes when nothing is
      // open; replace rather than push, because closing is not a step back
      // through the panel, it is undoing having arrived at it.
      releaseAddress: function () {
        // Whichever way it was opened, the address now names the panel: a panel
        // fetched by click remembered where the page was, and one the address
        // itself opened was told where the page goes when nothing is open.
        var href = this._closed || (this._root && this._root.dataset.closedHref);
        if (!href) return;
        var fetched = this.teleported("[data-verso-panel-loaded]");
        if (this._root.dataset.open !== "true" && !fetched) return;
        this._root.dataset.open = "false";
        this._closed = "";
        this.setAddress(href);
      },
      startBusy: function () {
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
      // A row icon that names a panel tab opens the panel on it. The panel is
      // fetched the first time it is revealed, so before the first open the tab
      // is a query on the pending request; afterwards the strip is already on
      // screen and the same request swaps it. An icon naming no tab, or a panel
      // that is not an entity panel, is just an open.
      // A row icon that names a panel tab opens the panel on it. The icon is
      // found through the event's target rather than its currentTarget: Alpine
      // calls a bound method after the dispatch it came from has finished, and
      // currentTarget is null by then.
      showTab: function (e) {
        var icon = e && e.target && e.target.closest("[data-verso-entity-tab]");
        this._tab = icon ? icon.getAttribute("data-verso-entity-tab") : "";
        this.show();
      },
      // An entity panel is fetched when its drawer is opened, and only when what
      // is wanted is not already in it: the first open, or an open asking for a
      // different tab than the one showing. Opening the same panel again keeps
      // what is on screen, half-typed values included.
      // A panel the plugin renders arrives the same way an entity panel does: the
      // frame is already here, and what goes in it is one request away. Fetching
      // rather than carrying it means a listing of forty rows ships forty empty
      // frames instead of forty panels nobody asked for — and because the frame
      // is mounted once, it slides in once. The address is pushed as we open, so
      // the panel is a place you can link to and step back out of.
      // A door that opens a panel says which panel: the address is on the link,
      // so one blank frame serves every door in its scope — a listing's bar and
      // each of its lane headers all open "a new one", differing only in what
      // they seed it with. currentTarget is already gone by the time a bound
      // method runs, so the link is found from the event's target.
      showPanel: function (e) {
        var link = e && e.target && e.target.closest("a[href]");
        var frame = this.teleported("[data-verso-panel]");
        if (link && frame) frame.setAttribute("data-verso-panel-url", link.getAttribute("href"));
        this.show();
      },
      loadPanel: function () {
        // The frame is teleported to <body>, so it is no longer under the row
        // that owns it — Alpine keeps the moved node on the template, which is
        // the only honest way back to it.
        var panel = this.teleported("[data-verso-panel-url]");
        if (!panel) return;
        var url = panel.getAttribute("data-verso-panel-url");
        if (!url) return;
        // A panel that opens from the chrome — the review drawer, over every
        // page — is not a place on the page under it. Its address is only what
        // the frame fetches, never what the browser shows, so a reload or a
        // copied link still names that page; and it is read again on every
        // open, because what it shows is the stage as it stands now, not an
        // object of this page that is still being typed into.
        var chrome = panel.hasAttribute("data-verso-panel-chrome");
        if (!chrome) {
          // Where the page was before this opened, so closing can go back to it.
          // The frame knows the panel's address; only the browser knows what the
          // address was a moment ago.
          if (!this._closed) this._closed = window.location.pathname + window.location.search;
          // The address names what is on screen every time this opens, fetched or
          // not. These were inside the fetch below, which meant the second opening
          // of the same panel — already loaded, nothing to request — showed the
          // panel and left the address on the listing: the same screen reachable at
          // two addresses depending on whether you had opened it before.
          this.setAddress(url);
        }
        if (!window.htmx) return;
        // The fetch is the other question, and it has its own answer: ask only
        // for what is not already here, so opening the same panel again keeps what
        // is on screen, half-typed values included.
        if (!chrome && panel.getAttribute("data-verso-panel-loaded") === url) return;
        panel.setAttribute("data-verso-panel-loaded", url);
        window.htmx.ajax("GET", url, { target: panel, swap: "innerHTML" });
      },
      // The panel is a state of this page, not a page of its own: the address
      // follows what is on screen so it can be linked and reloaded, but it does
      // not stack up history entries nobody asked for. Stepping back from a
      // listing should leave the listing, not walk back through every reading of
      // every rule that was opened along the way.
      setAddress: function (href) {
        try {
          window.history.replaceState(null, "", href);
        } catch (e) {
          /* a browser that refuses the rewrite still shows the panel */
        }
      },
      loadEntity: function (slot) {
        var panel = this.entityPanel();
        if (!panel || !window.htmx) return;
        if (panel.getAttribute("data-verso-entity-tab") === (slot || "")) return;
        var url = panel.getAttribute("data-verso-entity-url");
        if (slot) url += "?tab=" + encodeURIComponent(slot);
        panel.setAttribute("data-verso-entity-tab", slot || "");
        window.htmx.ajax("GET", url, { target: panel, swap: "innerHTML" });
      },
      // The panel is teleported to <body>, which puts it outside this
      // component's element and so outside $refs. Alpine keeps the link on the
      // template it came from, and that is the only structural path back to it;
      // a row whose drawer is not an entity panel has none, and asking for a tab
      // on one is simply an open.
      entityPanel: function () {
        return this.teleported("[data-verso-entity-body]");
      },
      // Whatever this scope's teleported frame holds, found through the template
      // Alpine moved it from.
      teleported: function (selector) {
        var template = this._root && this._root.querySelector("template[x-teleport]");
        var moved = template && template._x_teleport;
        return moved ? moved.querySelector(selector) : null;
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
