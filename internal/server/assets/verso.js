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

// versoErrorLine says a refusal on a line: the crimson square hung on the
// first line, then the words — the markup of verso-error-line and
// verso-error-words in shared.html.tmpl, so a refusal a script writes reads as
// one the server rendered. A slot the page already has keeps its own classes
// (the template gives it the line's); given no line, it makes one with them
// (the caller places it). Either way it returns the line.
function versoErrorLine(line, message) {
  if (!line) {
    line = document.createElement("p");
    line.className = versoErrorLine.lineClass;
  }
  var mark = document.createElement("span");
  mark.setAttribute("aria-hidden", "true");
  mark.className = "mt-1.75 size-1.5 shrink-0 rounded-[1px] bg-crimson";
  var words = document.createElement("span");
  words.setAttribute("data-verso-error-text", "");
  words.textContent = message;
  line.replaceChildren(mark, words);
  return line;
}
versoErrorLine.lineClass = "flex items-start gap-2 text-sm leading-5 text-crimson-deep";

// versoAnnounce says something to a screen reader without drawing it: what a
// keyboard move did, what an apply came to. The region is created empty and
// filled a frame later, because a live region that arrives already holding its
// words is often not read at all. Saying the same thing twice in a row still
// speaks, since the text is cleared before it is set.
var versoAnnounce = (function () {
  var region = null;
  return function (text) {
    if (!region) {
      region = document.createElement("div");
      region.id = "verso-announce";
      region.className = "sr-only";
      region.setAttribute("role", "status");
      region.setAttribute("aria-live", "polite");
      document.body.appendChild(region);
    }
    region.textContent = "";
    window.requestAnimationFrame(function () {
      region.textContent = text;
    });
  };
})();

// A dialog is announced by its title: the first heading in it names it. The
// frames are fetched and swapped, so the name is read from what the dialog holds
// now rather than written into a template that may not have its heading yet.
function versoNameDialog(dialog) {
  if (!dialog) return;
  var heading = dialog.querySelector("h1,h2,h3,h4");
  if (!heading) {
    dialog.removeAttribute("aria-labelledby");
    return;
  }
  if (!heading.id) heading.id = "verso-dialog-title-" + Math.random().toString(36).slice(2, 10);
  dialog.setAttribute("aria-labelledby", heading.id);
}

// While a dialog is open the page behind it is inert: out of the tab order and
// out of what a screen reader reads, which is what aria-modal promises and does
// not itself do. Each layer remembers what it made inert and gives back exactly
// that, so a dialog opened over a drawer leaves the drawer inert until it closes
// and live regions keep speaking throughout.
// The layer is the dialog's overlay: the element x-teleport hung on <body>.
function versoLayerOf(el) {
  while (el && el.parentElement && el.parentElement !== document.body) el = el.parentElement;
  return el && el.parentElement === document.body ? el : null;
}

function versoLayerOpen(layer) {
  if (!layer || layer._versoInerted) return;
  layer._versoInerted = [].filter.call(document.body.children, function (node) {
    if (node === layer || node.contains(layer) || node.inert) return false;
    if (/^(SCRIPT|TEMPLATE|STYLE)$/.test(node.tagName)) return false;
    return !node.matches('[role="status"],[role="alert"],[aria-live]');
  });
  layer._versoInerted.forEach(function (node) { node.inert = true; });
}

function versoLayerClose(layer) {
  if (!layer || !layer._versoInerted) return;
  layer._versoInerted.forEach(function (node) { node.inert = false; });
  layer._versoInerted = null;
}

// What Tab can reach inside a dialog: the focusable elements that are drawn. A
// pane hidden by x-show or [hidden] is still in the DOM, and a trap that counts
// it wraps from an element nobody can see.
function versoTabbable(root) {
  return [].filter.call(root.querySelectorAll(
    'a[href],button:not([disabled]),input:not([disabled]):not([type="hidden"]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])'
  ), function (el) {
    return el.getClientRects().length > 0 && !el.closest("[inert]");
  });
}

// A swap inside an open dialog (a drawer's tab, a refused form coming back)
// replaces the element that had focus, which drops focus to <body> behind the
// dialog. Put it back where the person was working: the tab now showing, else
// the dialog itself. The heading may have been replaced too, so the name is
// read again.
// While a panel's contents are on their way, the dialog it sits in is busy: a
// screen reader holds off reading a region that is about to be replaced.
function versoBusy(panel, on) {
  var dialog = panel && panel.closest && panel.closest('[role="dialog"]');
  if (!dialog) return;
  if (on) dialog.setAttribute("aria-busy", "true");
  else dialog.removeAttribute("aria-busy");
}

// A panel whose fetch failed is no longer on its way either.
document.addEventListener("htmx:responseError", function (e) { versoBusy(e.target, false); });
document.addEventListener("htmx:sendError", function (e) { versoBusy(e.target, false); });

document.addEventListener("htmx:afterSwap", function (e) {
  var dialog = e.target && e.target.closest && e.target.closest('[role="dialog"][aria-modal="true"]');
  if (!dialog) return;
  versoBusy(e.target, false);
  versoNameDialog(dialog);
  if (dialog.contains(document.activeElement)) return;
  var current = dialog.querySelector('[aria-current="page"]');
  (current || dialog).focus();
});

document.addEventListener("alpine:init", function () {
  // reveal: a shared secret's field (a Wi-Fi key) shown on request. The CSP
  // build binds properties, never expressions, so each state is a getter.
  Alpine.data("reveal", function () {
    return {
      shown: false,
      get masked() { return !this.shown; },
      get inputType() { return this.shown ? "text" : "password"; },
      get pressed() { return this.shown ? "true" : "false"; },
      toggle: function () { this.shown = !this.shown; },
    };
  });

  // copy: copy the widget's text and report the clipboard's actual answer.
  // Prefers the async Clipboard API; falls back to a hidden-textarea execCommand
  // for non-secure origins (a router reached over plain http on the LAN).
  Alpine.data("copy", function () {
    return {
      idle: true,
      done: false,
      failed: false,
      _timer: null,
      _attempt: 0,
      get ready() { return !this.done && !this.failed; },
      destroy: function () {
        clearTimeout(this._timer);
        this._attempt++;
      },
      run: function () {
        var src = this.$refs.src;
        var text = src ? src.textContent : "";
        var self = this;
        var attempt = ++this._attempt;
        clearTimeout(this._timer);
        this.idle = true;
        this.done = this.failed = false;
        var finish = function (copied) {
          // A late clipboard answer must not overwrite a newer attempt.
          if (attempt !== self._attempt) return;
          self.idle = !copied;
          self.done = copied;
          self.failed = !copied;
          versoAnnounce(T(copied ? "Copied" : "Couldn’t copy. Select the text and copy it manually."));
          self._timer = setTimeout(function () {
            self.done = false;
            self.failed = false;
            self.idle = true;
          }, copied ? 1500 : 4000);
        };
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(function () {
            finish(true);
          }, function () {
            if (attempt !== self._attempt) return;
            finish(self.fallback(text));
          });
        } else {
          finish(self.fallback(text));
        }
      },
      fallback: function (text) {
        var active = document.activeElement;
        var ta = document.createElement("textarea");
        try {
          ta.value = text;
          ta.readOnly = true;
          ta.tabIndex = -1;
          ta.style.position = "fixed";
          ta.style.top = "0";
          ta.style.left = "0";
          ta.style.opacity = "0";
          document.body.appendChild(ta);
          ta.select();
          return document.execCommand("copy");
        } catch (e) {
          return false;
        } finally {
          ta.remove();
          if (active && active.isConnected && active.focus) active.focus({ preventScroll: true });
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
      // The hamburger's aria-expanded, as the string the attribute takes.
      get expanded() {
        return this.open ? "true" : "false";
      },
      // Opening moves focus into the rail, onto its own way shut, so the next
      // Tab walks its destinations rather than the page under it.
      toggle: function () {
        if (this.open) {
          this.close();
          return;
        }
        this.open = true;
        var self = this;
        this.$nextTick(function () {
          if (self.$refs.railClose) self.$refs.railClose.focus();
        });
      },
      // Closing hands focus back to the hamburger that opened it. Escape reaches
      // here from anywhere on the page, so a rail that was not open is left
      // alone, along with wherever focus is.
      close: function () {
        if (!this.open) return;
        this.open = false;
        var button = this.$refs.menuButton;
        if (button && button.getClientRects().length) button.focus();
      },
    };
  });

  // panelFaces: which face of a board's panel art is shown, front or rear. The
  // CSP build evaluates no expressions in bindings, so each binding is a
  // property here; the face shown is filled in the body ink, as every selected
  // segment in the app is.
  Alpine.data("panelFaces", function () {
    var on = "bg-body font-semibold text-white";
    var off = "font-normal text-body hover:text-ink";
    return {
      face: "rear",
      get rear() {
        return this.face === "rear";
      },
      get front() {
        return this.face === "front";
      },
      get rearClass() {
        return this.rear ? on : off;
      },
      get frontClass() {
        return this.front ? on : off;
      },
      get rearPressed() {
        return this.rear ? "true" : "false";
      },
      get frontPressed() {
        return this.front ? "true" : "false";
      },
      showRear: function () {
        this.face = "rear";
      },
      showFront: function () {
        this.face = "front";
      },
    };
  });

  // confirm: a destructive act that asks first. The trigger gives way to the
  // question and focus goes to the first thing to answer with (the password, or
  // the confirm button); the way back, or Escape, returns to the trigger. An
  // Escape that answers the question stops there, so it does not also close the
  // drawer the question sits in.
  Alpine.data("confirm", function () {
    return {
      asking: false,
      // A slot the server answers open (a refused paste, kept as typed, with
      // its reason) starts asking, with the cursor back in its box.
      init: function () {
        if (this.$el.hasAttribute("data-verso-open")) this.ask();
      },
      get idle() {
        return !this.asking;
      },
      get expanded() {
        return this.asking ? "true" : "false";
      },
      ask: function () {
        this.asking = true;
        var self = this;
        this.$nextTick(function () {
          if (self.$refs.first) self.$refs.first.focus();
        });
      },
      cancel: function () {
        this.asking = false;
        var self = this;
        this.$nextTick(function () {
          if (self.$refs.trigger) self.$refs.trigger.focus();
        });
      },
      escape: function (e) {
        if (!this.asking) return;
        e.stopPropagation();
        this.cancel();
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
            self.enter();
          });
        });
      },
      // The dialog is on screen: name it, shut the page behind it, and put
      // focus in it.
      enter: function () {
        // Escape can close it before the queued opening callback runs.
        if (!this.open) return;
        var dialog = this.$refs.dialog;
        if (!dialog) return;
        versoNameDialog(dialog);
        versoLayerOpen(versoLayerOf(dialog));
        dialog.focus();
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
            self.enter();
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
        // The page comes back before focus does: focus cannot land on an inert
        // element, and the trigger it returns to is on that page.
        versoLayerClose(versoLayerOf(this.$refs.dialog));
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
      // The dialog's busy pane replaces its form while the submission runs; its
      // heading is said, since nothing moved focus to it.
      startBusy: function () {
        this.idle = false;
        this.busy = true;
        var dialog = this.$refs.dialog;
        var busy = dialog && dialog.querySelector("[data-verso-busy]");
        if (busy) versoAnnounce(busy.textContent.replace(/\s+/g, " ").trim());
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
        // A link to a thing of its own (a package, over a settings page) opens
        // its panel as the chrome's: the page keeps its own address.
        if (link && frame && link.hasAttribute("data-verso-panel-chrome")) frame.setAttribute("data-verso-panel-chrome", "");
        this.show();
      },
      loadPanel: function () {
        // The frame is teleported to body, so look inside this modal's dialog
        // rather than the row that opened it.
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
        // An address that names a part of the panel (an expanded row's
        // "Configure DHCP", into the editor's DHCP section) opens the panel
        // scrolled to that part, fetched or already here.
        var hash = url.indexOf("#") === -1 ? "" : url.slice(url.indexOf("#") + 1);
        function reveal() {
          if (!hash) return;
          var part = panel.querySelector("#" + CSS.escape(hash));
          if (part) part.scrollIntoView({ block: "start" });
        }
        // The fetch is the other question, and it has its own answer: ask only
        // for what is not already here, so opening the same panel again keeps what
        // is on screen, half-typed values included.
        if (!chrome && panel.getAttribute("data-verso-panel-loaded") === url) { reveal(); return; }
        panel.setAttribute("data-verso-panel-loaded", url);
        versoBusy(panel, true);
        window.htmx.ajax("GET", url, { target: panel, swap: "innerHTML" }).then(reveal, function () { /* the error handlers above have already said so */ });
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
        versoBusy(panel, true);
        window.htmx.ajax("GET", url, { target: panel, swap: "innerHTML" });
      },
      // A row whose drawer is not an entity panel has none, and asking for a
      // tab on one is simply an open.
      entityPanel: function () {
        return this.teleported("[data-verso-entity-body]");
      },
      // The dialog ref belongs to this modal, even after teleporting to body.
      // A row may also contain a modal for its removal confirmation; the first
      // teleport template under the row need not belong to its edit drawer.
      teleported: function (selector) {
        var dialog = this.$refs.dialog;
        return dialog ? dialog.querySelector(selector) : null;
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
        var f = versoTabbable(el);
        if (!f.length) {
          e.preventDefault();
          el.focus();
          return;
        }
        var first = f[0],
          last = f[f.length - 1],
          active = document.activeElement;
        // Focus on the dialog itself (where it lands on open) or anywhere
        // outside it is not inside the loop yet: Tab enters at the start,
        // Shift+Tab at the end, instead of stepping out to the page behind.
        var outside = active === el || !el.contains(active);
        if (e.shiftKey && (outside || active === first)) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && (outside || active === last)) {
          e.preventDefault();
          first.focus();
        }
      },
    };
  });
});
