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
      _dirty: false,
      show: function () {
        this._return = document.activeElement;
        this._dirty = false;
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
        if (this._dirty && !window.confirm("You have unsaved changes. Close without saving?")) {
          return;
        }
        this._dirty = false;
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

// Leaving the page with unsaved form edits loses them — warn first. Any edit to
// a field inside a <form> marks the page dirty; submitting a form is intentional
// navigation and clears the flag. The browser renders its native prompt.
(function () {
  var dirty = false;
  document.addEventListener(
    "input",
    function (e) {
      if (e.target.closest && e.target.closest("form")) dirty = true;
    },
    true
  );
  document.addEventListener(
    "submit",
    function () {
      dirty = false;
    },
    true
  );
  window.addEventListener("beforeunload", function (e) {
    if (!dirty) return;
    e.preventDefault();
    e.returnValue = ""; // required by Chromium for the prompt to show
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
  var listWrap = document.getElementById("verso-capsule-list"); // absent on a clean page
  var review = document.getElementById("verso-capsule-review");
  var csrf = capsule.getAttribute("data-csrf") || "";

  function post(path) {
    return fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: "_csrf=" + encodeURIComponent(csrf),
    });
  }

  if (review && listWrap) {
    review.addEventListener("click", function () {
      listWrap.classList.toggle("hidden");
    });
  }

  document.getElementById("verso-capsule-discard").addEventListener("click", function () {
    capsule.classList.add("verso-busy");
    post("/uci/discard")
      .then(function (res) {
        if (!res.ok) throw new Error("discard failed");
        location.reload();
      })
      .catch(function () {
        capsule.classList.remove("verso-busy");
        text.textContent = "Couldn’t discard — try again";
      });
  });

  document.getElementById("verso-capsule-apply").addEventListener("click", function () {
    capsule.classList.add("verso-busy");
    if (listWrap) listWrap.classList.add("hidden");
    text.textContent = "Applying — auto-reverts if the router is unreachable for 30 s…";
    var deadline = Date.now() + 28000;

    function confirmLoop() {
      post("/uci/confirm")
        .then(function (res) {
          if (res.ok) {
            capsule.classList.add("verso-done");
            text.textContent = "Applied";
            setTimeout(function () {
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
        if (!res.ok) throw new Error("apply failed");
        setTimeout(confirmLoop, 1000);
      })
      .catch(function () {
        // The apply itself may have severed our path (a network change); keep
        // trying to confirm — reaching the router again is the success signal.
        setTimeout(confirmLoop, 1000);
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

// The overview stream: one EventSource the shell pushes fresh truth into —
// `meters` readings every second, `ports` panel state when it changes. Each
// type updates its rendered widget in place (the CSS transitions do the
// glides). Reconnection after a drop is EventSource's own; when the session
// ends the reconnect lands on the login redirect — not an event stream —
// which closes the client for good.
(function () {
  if (!window.EventSource) return;
  if (!document.querySelector("[data-verso-meter]") && !document.querySelector("[data-verso-port]") && !document.querySelector("[data-verso-chart]")) return;
  var BANDS = {
    good: "stroke-green-600",
    warn: "stroke-amber-500",
    danger: "stroke-red-600",
    info: "stroke-sky-600",
  };
  function setText(root, selector, text) {
    var el = root.querySelector(selector);
    if (el) el.textContent = text;
  }
  function applyMeter(reading) {
    var root = document.querySelector('[data-verso-meter="' + reading.name + '"]');
    if (!root) return;
    setText(root, "[data-verso-meter-value]", reading.value);
    setText(root, "[data-verso-meter-unit]", reading.unit);
    setText(root, "[data-verso-meter-detail]", reading.detail);
    var ring = root.querySelector("[data-verso-meter-ring]");
    if (ring) {
      var c = 2 * Math.PI * parseFloat(ring.getAttribute("r"));
      var fill = Math.min(100, Math.max(0, reading.fill));
      ring.setAttribute("stroke-dasharray", ((fill / 100) * c).toFixed(1) + " " + c.toFixed(2));
      for (var band in BANDS) ring.classList.remove(BANDS[band]);
      ring.classList.add(BANDS[reading.band] || BANDS.good);
    }
    var svg = root.querySelector("svg");
    if (svg) svg.setAttribute("aria-label", (reading.label + " " + reading.value + " " + reading.unit).trim());
  }
  function applyPort(port) {
    var root = document.querySelector('[data-verso-port="' + port.iface + '"]');
    if (!root) return;
    root.classList.toggle("is-linked", port.linked);
    root.classList.toggle("is-empty", !port.linked);
    root.classList.toggle("is-active", port.active);
    setText(root, ".verso-port-speed", port.speed);
    setText(root, "[data-verso-port-link]", port.speed);
    setText(root, "[data-verso-port-addr]", port.addr);
  }
  // applyChart redraws a named chart from fresh series — the same geometry
  // the server drew (viewBox coordinates from the svg itself), so the live
  // layer and the first paint never disagree. Series pair with the rendered
  // groups by order; the role hook lets an idle-grey chart take its colours
  // when traffic starts.
  var CHART_ROLES = ["sky", "violet", "emerald", "amber", "idle"];
  function applyChart(dev) {
    var svg = document.querySelector('svg[data-verso-chart="' + dev.key + '"]');
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
    var labels = svg.parentElement.querySelectorAll(".verso-chart-yl");
    labels.forEach(function (el, i) {
      var frac = [1, 0.75, 0.5, 0.25][i];
      if (frac === undefined) return;
      if (el.dataset.unit === undefined) {
        el.dataset.unit = (el.textContent.match(/[\d.]+\s*(.*)$/) || ["", ""])[1];
      }
      var v = max * frac;
      var text = v >= 10 ? Math.round(v).toString() : (Math.round(v * 10) / 10).toString();
      el.textContent = el.dataset.unit ? text + " " + el.dataset.unit : text;
    });
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
  listen(es, "ports", "ports", applyPort);
  listen(es, "traffic", "devices", applyChart);
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
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
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
