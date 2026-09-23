// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// verso-forms.js — filling a form in (ADR-004). What someone types, and what the
// page does about it before anything is saved: the guard against walking away
// from unsaved work, the controls that are richer than an <input> (token boxes,
// the condition picker, the inline field), the acts that submit themselves
// because they are complete on their own (a named switch, a row's act), and
// what a panel's form is owed after it has answered in place.
//
// It publishes window.versoDirtyState (is there unsaved work, and stop asking)
// and window.versoInline (is a field half-typed, and flush it), and it asks
// window.versoStaged to take a fresh count when a value is staged. Every one of
// those is read inside a handler, never at load, so these files load in any
// order. The htmx events it listens for are DOM events like any other,
// dispatched by a script that loads after this one.

// Leaving the page with unsaved form edits loses them — warn first. Dirtiness is
// a comparison with the rendered baseline, never a history of input events: a
// toggle changed twice is clean again. Forms, drawer forms included, are
// tracked here; what is already staged needs no guard, because the stage
// outlives the page. The browser renders its native prompt.
(function () {
  var baselines = new Map();
  var sources = Object.create(null);
  var suppressed = false;

  function signature(form) {
    var values = [];
    [].forEach.call(form.elements || [], function (control) {
      if (!control.name || control.name === "_csrf" || control.matches(":disabled")) return;
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
    if (!form || baselines.has(form) || form.hasAttribute("data-verso-page-form") && !form.hasAttribute("data-verso-dirty-form")) return;
    baselines.set(form, signature(form));
    if (form.hasAttribute("data-verso-dirty-form")) form.querySelectorAll('button[type="submit"]').forEach(function(button){ button.disabled=true; });
  }

  function refreshForms() {
    var dirty = false;
    baselines.forEach(function (baseline, form) {
      if (!form.isConnected) return;
      var changed=signature(form)!==baseline;
      if(changed) dirty=true;
      if (form.hasAttribute("data-verso-dirty-form")) form.querySelectorAll('button[type="submit"]').forEach(function(button){ button.disabled=!changed; });
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
    if (!form || form.hasAttribute("data-verso-page-form") && !form.hasAttribute("data-verso-dirty-form")) return;
    track(form);
    refreshForms();
  }
  document.addEventListener("input", changed, true);
  document.addEventListener("change", changed, true);
  // A submit stands the guard down because the page is about to be left. A form
  // that answers in place — a panel's form posting back into its frame, a row's
  // act posting itself — is not leaving, so its submit is not that moment: the
  // answer swaps a fresh, clean form in or reconciles the row, and the guard
  // keeps watching everything else.
  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (form && form.hasAttribute && (form.hasAttribute("hx-post") || form.hasAttribute("data-verso-act"))) return;
    suppressed = true;
  }, true);
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
  // chipFor fills the list's own empty chip with one value. The template sits
  // beside the box the server rendered it in, so the clone carries that box's
  // field name and every class it was given — nothing about a chip's shape is
  // known here.
  function chipFor(list, value) {
    var control = list.parentElement;
    var template = control && control.querySelector("template[data-verso-token-chip]");
    if (!template) return null;
    var chip = template.content.firstElementChild.cloneNode(true);
    var hidden = chip.querySelector("input[type=hidden]");
    var label = chip.querySelector("[data-verso-token-label]");
    var remove = chip.querySelector("[data-verso-token-remove]");
    if (!hidden || !label || !remove) return null;
    hidden.value = value;
    label.textContent = value;
    remove.setAttribute("aria-label", T("Remove") + " " + value);
    return chip;
  }

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
    // The chip the server renders, cloned. A value added here and the same value
    // arriving from the plugin are one kind of thing, so there is one chip — the
    // empty one the list carries — rather than the same markup written twice,
    // once in the template and once here, where the two drifted apart the moment
    // either was touched.
    var chip = chipFor(list, value);
    if (!chip) return;
    list.insertBefore(chip, input);
    input.value = "";
    list.dispatchEvent(new Event("input", { bubbles: true }));
  }

  // The suggestions under the box. They are a reminder of what the daemon calls
  // things, never a restriction — the box still takes whatever is typed — so the
  // list narrows as it is typed into and hides when nothing is left to suggest or
  // when everything it offers is already chosen.
  function menuOf(input) {
    var control = input.closest("[data-verso-token-list]");
    return control && control.parentElement
      ? control.parentElement.querySelector("[data-verso-token-menu]")
      : null;
  }

  function chosen(input) {
    var list = input.closest("[data-verso-token-list]");
    return [].map.call(list.querySelectorAll("[data-verso-token] input"), function (item) {
      return item.value;
    });
  }

  function showMenu(input) {
    var menu = menuOf(input);
    if (!menu) return;
    var typed = input.value.trim().toLowerCase();
    var taken = chosen(input);
    var left = 0;
    [].forEach.call(menu.querySelectorAll("[data-verso-token-option]"), function (option) {
      var value = option.dataset.versoTokenOption;
      var fits = value.toLowerCase().indexOf(typed) === 0 && taken.indexOf(value) < 0;
      option.hidden = !fits;
      if (fits) left += 1;
    });
    menu.hidden = left === 0;
    input.setAttribute("aria-expanded", String(!menu.hidden));
    // What was showing has changed, so no option is the one in hand any more.
    activate(input, null);
  }

  function hideMenu(input) {
    var menu = menuOf(input);
    if (!menu) return;
    menu.hidden = true;
    input.setAttribute("aria-expanded", "false");
    activate(input, null);
  }

  // The option in hand, as the arrow keys move through what is showing. Focus
  // stays in the box being typed into; the option is pointed at, not focused.
  function activate(input, option) {
    var menu = menuOf(input);
    if (!menu) return;
    [].forEach.call(menu.querySelectorAll("[data-verso-token-option]"), function (each) {
      each.setAttribute("aria-selected", each === option ? "true" : "false");
    });
    if (option) {
      input.setAttribute("aria-activedescendant", option.id);
      option.scrollIntoView({ block: "nearest" });
    } else {
      input.removeAttribute("aria-activedescendant");
    }
  }

  function step(input, by) {
    var menu = menuOf(input);
    if (!menu || menu.hidden) return;
    var showing = [].filter.call(menu.querySelectorAll("[data-verso-token-option]"), function (option) {
      return !option.hidden;
    });
    if (!showing.length) return;
    var current = showing.indexOf(menu.querySelector('[aria-selected="true"]'));
    var next = current < 0 ? (by > 0 ? 0 : showing.length - 1) : (current + by + showing.length) % showing.length;
    activate(input, showing[next]);
  }

  function take(input, option) {
    input.value = option.dataset.versoTokenOption;
    addToken(input);
    // Taking one leaves the box open for the next: a list is a list.
    input.focus();
    showMenu(input);
  }

  document.addEventListener("input", function (event) {
    var input = event.target.closest && event.target.closest("[data-verso-token-input]");
    if (input) showMenu(input);
  });
  document.addEventListener("focusin", function (event) {
    var input = event.target.closest && event.target.closest("[data-verso-token-input]");
    if (input) showMenu(input);
  });
  document.addEventListener("keydown", function (event) {
    var input = event.target.closest && event.target.closest("[data-verso-token-input]");
    if (!input) return;
    if (event.key === "Escape") {
      hideMenu(input);
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      var menu = menuOf(input);
      if (!menu || menu.hidden) return;
      event.preventDefault();
      step(input, event.key === "ArrowDown" ? 1 : -1);
      return;
    }
    if (event.key !== "Enter" && event.key !== ",") return;
    event.preventDefault();
    var active = input.getAttribute("aria-activedescendant");
    var picked = active && document.getElementById(active);
    if (event.key === "Enter" && picked) {
      take(input, picked);
      return;
    }
    addToken(input);
    showMenu(input);
  });
  document.addEventListener("focusout", function (event) {
    var input = event.target.closest && event.target.closest("[data-verso-token-input]");
    if (!input) return;
    // A click on a suggestion moves focus out of the box; closing the menu here
    // would take the suggestion away before it could be pressed.
    var going = event.relatedTarget;
    if (going && going.closest && going.closest("[data-verso-token-menu]")) return;
    addToken(input);
    hideMenu(input);
  });
  document.addEventListener("click", function (event) {
    var option = event.target.closest && event.target.closest("[data-verso-token-option]");
    if (option) {
      take(option.closest("[data-verso-token-menu]").parentElement.querySelector("[data-verso-token-input]"), option);
      return;
    }
    var remove = event.target.closest && event.target.closest("[data-verso-token-remove]");
    if (remove) {
      var list = remove.closest("[data-verso-token-list]");
      var chip = remove.closest("[data-verso-token]");
      // The button pressed goes with its chip, so focus moves on to the next
      // chip's remove, or to the box once none follow, rather than falling out
      // of the form to the top of the page.
      var after = chip.nextElementSibling;
      var next = after && after.matches("[data-verso-token]") ? after.querySelector("[data-verso-token-remove]") : null;
      var box = list && list.querySelector("[data-verso-token-input]");
      chip.remove();
      if (next) next.focus();
      else if (box) box.focus();
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

  // The picker: the catalogue as a list to read or filter, where pressing a line
  // adds that condition to the rule. The list itself is the server's — every
  // entry, its option name and what it takes — so nothing here knows what a
  // condition is.
  function pickerOf(node) {
    return node.closest("[data-verso-conditions]");
  }

  function menu(builder) {
    return builder && builder.querySelector("[data-verso-condition-menu]");
  }

  function open(builder, show) {
    var panel = menu(builder);
    var button = builder && builder.querySelector("[data-verso-condition-open]");
    if (!panel || !button) return;
    panel.hidden = !show;
    button.setAttribute("aria-expanded", show ? "true" : "false");
    var filter = panel.querySelector("[data-verso-condition-filter]");
    if (!filter) return;
    if (show) {
      filter.value = "";
      sift(builder);
      filter.focus();
    }
  }

  function closeAll(except) {
    var open = document.querySelectorAll("[data-verso-condition-menu]:not([hidden])");
    for (var i = 0; i < open.length; i++) {
      var builder = pickerOf(open[i]);
      if (builder && builder !== except) {
        open[i].hidden = true;
        var button = builder.querySelector("[data-verso-condition-open]");
        if (button) button.setAttribute("aria-expanded", "false");
      }
    }
  }

  // sift hides what does not match, and hides a heading whose whole group has
  // gone — a heading over nothing reads as a group with no entries rather than as
  // one the filter excluded. What is matched is the words a person sees: the
  // condition's name and the option it writes, which is how somebody who knows
  // the config would look for it.
  function sift(builder) {
    var panel = menu(builder);
    if (!panel) return;
    var filter = panel.querySelector("[data-verso-condition-filter]");
    var needle = filter ? filter.value.trim().toLowerCase() : "";
    var groups = panel.querySelectorAll("[data-verso-condition-group]");
    var shown = 0;
    for (var g = 0; g < groups.length; g++) {
      var rows = groups[g].querySelectorAll("[data-verso-condition-choose]");
      var left = 0;
      for (var r = 0; r < rows.length; r++) {
        var hay = (rows[r].textContent || "").toLowerCase();
        var hit = !needle || hay.indexOf(needle) !== -1;
        rows[r].hidden = !hit;
        if (hit) left++;
      }
      groups[g].hidden = left === 0;
      shown += left;
    }
    var none = panel.querySelector("[data-verso-condition-none]");
    if (none) none.hidden = shown !== 0;
  }

  // add puts one condition into the rule and takes it off the offer.
  function add(choice) {
    var builder = pickerOf(choice);
    var key = choice.dataset.versoConditionChoose;
    var template = builder && builder.querySelector('template[data-verso-condition-template="' + CSS.escape(key) + '"]');
    var list = builder && builder.querySelector("[data-verso-condition-list]");
    if (!template || !list) return;
    list.appendChild(template.content.cloneNode(true));
    choice.disabled = true;
    open(builder, false);
    refresh(builder);
    builder.dispatchEvent(new Event("input", { bubbles: true }));
  }

  // drop takes it back out. It goes back to being on offer: a condition is in the
  // catalogue whether or not this rule carries it.
  function drop(remove) {
    var item = remove.closest("[data-verso-condition]");
    var builder = pickerOf(remove);
    if (!item || !builder) return;
    var offer = builder.querySelector('[data-verso-condition-choose="' + CSS.escape(item.dataset.versoCondition) + '"]');
    if (offer) offer.disabled = false;
    item.remove();
    refresh(builder);
    builder.dispatchEvent(new Event("input", { bubbles: true }));
  }

  document.addEventListener("click", function (event) {
    var target = event.target;
    if (!target.closest) return;
    var opener = target.closest("[data-verso-condition-open]");
    if (opener) {
      var builder = pickerOf(opener);
      var panel = menu(builder);
      closeAll(builder);
      open(builder, panel ? panel.hidden : true);
      return;
    }
    var choice = target.closest("[data-verso-condition-choose]");
    if (choice) return add(choice);
    var remove = target.closest("[data-verso-condition-remove]");
    if (remove) return drop(remove);
    // A press anywhere else puts an open picker away, which is what someone means
    // by clicking off a list they did not choose from. A press inside the open
    // list is not that: the filter has to be typeable.
    if (!target.closest("[data-verso-condition-menu]")) closeAll(null);
  });

  document.addEventListener("input", function (event) {
    var filter = event.target.closest && event.target.closest("[data-verso-condition-filter]");
    if (filter) sift(pickerOf(filter));
  });

  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape") return;
    var inside = event.target.closest && event.target.closest("[data-verso-condition-picker]");
    if (!inside) return;
    var builder = pickerOf(inside);
    var panel = menu(builder);
    if (!panel || panel.hidden) return;
    // The picker takes the key rather than the panel it sits in, so Escape over an
    // open list puts the list away and leaves the drawer where it is.
    event.stopPropagation();
    open(builder, false);
    var button = builder.querySelector("[data-verso-condition-open]");
    if (button) button.focus();
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
        el.disabled = false;
        // The flip is staged. Rather than reload the whole page (a jarring
        // reflow), bring the staged-changes chip in step from the re-rendered
        // response: the switch already shows the new state and the row is
        // otherwise unchanged, so only the staged count moves. Fall back to a
        // reload only when there is no chip to sync.
        return res.text().then(function (html) {
          var doc = new DOMParser().parseFromString(html, "text/html");
          if (window.versoStaged && window.versoStaged.sync) {
            window.versoStaged.sync(doc);
          } else {
            if (window.versoDirtyState) window.versoDirtyState.suppress();
            window.location.reload();
          }
        });
      }
      el.disabled = false;
      el.checked = !el.checked; // the device said no; show the truth
    }).catch(function () {
      el.disabled = false;
      el.checked = !el.checked;
    });
  });
})();

// The two acts that answer in place and leave the page around them where it
// was: a row's own act, and a panel's form posting back into its frame. A full
// render would have handed each of them two things for free — the row the act
// was about, drawn as it now stands, and the staged-changes chip's count — and in place
// they have to be read off a fresh copy of the page and reconciled into the one
// on screen.
//
// A row's act (the power glyph at its end) turns its section off or back on,
// which is a complete instruction the same way a named switch is (ADR-010). It
// posts its one pair to the page the row is on — the page's path, not its
// address: while a panel is open the address names the panel, and the panel's
// plugin reads a submission there as the panel's own unless it says otherwise,
// which the act does not — and the answer is that page, with the change in it.
//
// A panel's form carries hx-post, and htmx does the posting. A refusal comes
// back as the panel and swaps in where the form was; a save that went through
// comes back as the outcome alone, with the server saying there is nothing to
// swap — the panel closes on it, the outcome is said where the panel was, and
// the page behind it is read once more the way Discard reads it.
//
// Both reconcile the same way: each live row's cells are replaced by the ones
// the fresh page carries for the same section, and the chip re-counts. The
// <tr> itself stays — it holds the frame of its own panel, and what was typed
// into that panel survives the flip.
(function () {
  function cellsOf(row) {
    return [].filter.call(row.children, function (child) { return child.tagName === "TD"; });
  }

  function reconcileRows(doc) {
    [].forEach.call(document.querySelectorAll("tr[data-verso-row-id]"), function (row) {
      var id = row.getAttribute("data-verso-row-id");
      var fresh = doc.querySelector('tr[data-verso-row-id="' + CSS.escape(id) + '"]');
      if (!fresh) return;
      var have = cellsOf(row);
      var want = cellsOf(fresh);
      var patch = doc.querySelector("[data-verso-row-patch]");
      if (patch) {
        patch.dataset.versoRowPatch.split(",").map(Number).forEach(function (i) {
          if (have[i] && want[i]) have[i].replaceWith(document.importNode(want[i], true));
        });
        return;
      }
      var same = have.length === want.length && have.every(function (cell, i) {
        return cell.outerHTML === want[i].outerHTML;
      });
      if (same) return;
      // The cells go back in front of whatever follows them — the row's own
      // template, which is its panel's frame.
      var anchor = have.length ? have[have.length - 1].nextSibling : null;
      have.forEach(function (cell) { cell.remove(); });
      want.forEach(function (cell) { row.insertBefore(document.importNode(cell, true), anchor); });
    });
  }

  function reconcile(doc) {
    reconcileRows(doc);
    if (window.versoStaged && window.versoStaged.sync) window.versoStaged.sync(doc);
  }
  // The stage's drawer asks for the same when the stage changes under the page
  // (verso-commit.js), read inside its handler.
  window.versoReconcile = reconcile;

  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (!form || !form.matches || !form.matches("form[data-verso-act]")) return;
    e.preventDefault();
    var button = form.querySelector("button[type=submit]");
    if (button) button.disabled = true;
    // Never repeat an act after a lost response: it may already have succeeded.
    var failure = function (message) {
      if (button) button.disabled = false;
      var notice = versoErrorLine(null, message || T("Could not confirm the action. Check the service state before trying again."));
      notice.classList.add("mb-5");
      notice.setAttribute("role", "alert");
      notice.dataset.versoActError = "";
      var old = document.querySelector("[data-verso-act-error]");
      if (old) old.remove();
      var table = form.closest("table");
      if (table) table.parentNode.insertBefore(notice, table);
    };
    fetch(window.location.pathname, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "X-Verso-Interaction": "act",
      },
      body: new URLSearchParams(new FormData(form)).toString(),
      credentials: "same-origin",
    }).then(function (res) {
      if (res.redirected && new URL(res.url).pathname === "/login") return failure();
      if (!res.ok) return res.text().then(function (message) { failure(message); });
      return res.text().then(function (html) {
        reconcile(new DOMParser().parseFromString(html, "text/html"));
        if (button) button.disabled = false;
      });
    }).catch(function () { failure(); });
  }, true);

  function panelTarget(evt) {
    var target = evt.detail && evt.detail.target;
    return target && target.matches && target.matches("[data-verso-panel]") ? target : null;
  }

  // rowOf is the row whose panel this frame is. The frame lives in <body>, moved
  // there from the row's own template, and the template still knows its clone.
  function rowOf(frame) {
    return [].find.call(document.querySelectorAll("tr[data-verso-row-id]"), function (row) {
      var template = row.querySelector("template[x-teleport]");
      return !!(template && template._x_teleport && template._x_teleport.contains(frame));
    }) || null;
  }

  // landed washes the row a panel just saved in the action colour for a moment,
  // so the eye finds the row the outcome was about.
  function landed(row) {
    row.classList.add("verso-landed");
    row.addEventListener("animationend", function () { row.classList.remove("verso-landed"); }, { once: true });
  }

  // The panel's submission is done: the server said so by swapping nothing and
  // handing back the outcome alone. The panel is told to finish, the outcome is
  // said where the panel was, and the page behind it is read once more for the
  // row and the count that changed. A refusal never arrives here — it swaps in
  // as the panel — and neither does a change to which rows there are, which the
  // server answers with the page itself.
  document.addEventListener("htmx:afterRequest", function (evt) {
    var frame = panelTarget(evt);
    var xhr = evt.detail && evt.detail.xhr;
    var config = evt.detail && evt.detail.requestConfig;
    if (!frame || !xhr || !config || String(config.verb).toLowerCase() !== "post") return;
    if (xhr.status !== 200 || xhr.getResponseHeader("HX-Reswap") !== "none") return;
    var response = new DOMParser().parseFromString(xhr.responseText, "text/html");
    var outcome = response.querySelector(".verso-flash");
    var row = rowOf(frame);
    var before = window.versoStaged ? window.versoStaged.count() : 0;
    frame.dispatchEvent(new CustomEvent("verso-panel-done", { bubbles: true }));
    if (outcome && window.versoOutcome) window.versoOutcome.show(document.importNode(outcome, true));
    if (xhr.getResponseHeader("X-Verso-Packages") === "changed") {
      document.dispatchEvent(new CustomEvent("verso-packages-changed", { detail: { navigation: response.querySelector("[data-package-navigation]") } }));
      return;
    }
    fetch(window.location.pathname, { headers: { Accept: "text/html" }, credentials: "same-origin" })
      .then(function (res) { return res.ok ? res.text() : ""; })
      .then(function (html) {
        if (html) reconcile(new DOMParser().parseFromString(html, "text/html"));
      })
      .catch(function () {
        // The chip keeps its last count; the next page says the rest.
      })
      .then(function () {
        if (!row) return;
        landed(row);
        // The change the panel staged leaves its row for the chip.
        if (window.versoStaged) window.versoStaged.fly(row.querySelector("td") || row, before);
      });
  });

  // A panel's submission answered with a page the frame cannot hold — the
  // object was deleted, and the listing is where the page went. htmx follows
  // the redirect it was handed; the guard is stood down first, because the form
  // about to be left is the one that was just submitted.
  document.addEventListener("htmx:beforeOnLoad", function (evt) {
    var xhr = evt.detail && evt.detail.xhr;
    if (xhr && xhr.getResponseHeader("HX-Redirect") && window.versoDirtyState) window.versoDirtyState.suppress();
  });

  // A stage that failed answers with a notice at its own status, which htmx
  // keeps out of the swap: the values just typed are still in the form and must
  // stay there. The notice goes in above them, where the person is, in place of
  // the last one.
  document.addEventListener("htmx:responseError", function (evt) {
    var frame = panelTarget(evt);
    var xhr = evt.detail && evt.detail.xhr;
    if (!frame || !xhr || !xhr.responseText) return;
    var doc = new DOMParser().parseFromString(xhr.responseText, "text/html");
    var notice = doc.body && doc.body.firstElementChild;
    var scroller = frame.querySelector("[data-scroll]");
    if (!notice || !scroller) return;
    var previous = scroller.querySelector("[data-verso-panel-notice]");
    if (previous) previous.remove();
    notice.setAttribute("data-verso-panel-notice", "");
    scroller.insertBefore(document.importNode(notice, true), scroller.firstChild);
  });
})();

// Inline value field (settings row, [[inline-settings-commit-model]]): a plain
// input that stages its own change on blur or Enter — the natural "done" a person
// expects, no edit/confirm icons. Validation runs at that moment: an invalid value
// gets its error beneath the field and does NOT stage, and the page scrolls to the
// topmost error. window.versoInline exposes anyDirty()/flush() for a caller that
// must not race a focused, just-typed field's own blur.
(function () {
  function csrf() {
    var m = document.querySelector('meta[name="verso-csrf"]');
    return m ? m.content : "";
  }
  function input(field) { return field.querySelector("[data-verso-inline-input]"); }
  function committed(el) {
    return (el.dataset.committed !== undefined ? el.dataset.committed : el.defaultValue).trim();
  }
  function isDirty(field) {
    var el = input(field);
    return !!el && el.value.trim() !== committed(el);
  }
  function anyDirty() {
    return [].some.call(document.querySelectorAll("[data-verso-inline-field]"), isDirty);
  }
  function showError(field, message) {
    var slot = field.querySelector("[data-verso-inline-error]");
    var el = input(field);
    if (slot) { versoErrorLine(slot, message); slot.hidden = false; }
    if (el) el.setAttribute("aria-invalid", "true");
  }
  function clearError(field) {
    var slot = field.querySelector("[data-verso-inline-error]");
    var el = input(field);
    if (slot) { slot.textContent = ""; slot.hidden = true; }
    if (el) el.removeAttribute("aria-invalid");
  }
  function scrollToFirstError() {
    var first = document.querySelector("[data-verso-inline-error]:not([hidden])");
    var still = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (first) first.scrollIntoView({ behavior: still ? "auto" : "smooth", block: "center" });
  }
  // The shell's authoritative check still runs server-side; this is the immediate,
  // per-datatype feedback at the "done" moment. It mirrors the server's rules so
  // the two never disagree.
  function invalidMessage(datatype, value) {
    var v = value.trim();
    if (v === "") return T("Enter a value.");
    if (datatype === "hostname" && !/^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/.test(v)) {
      return T("Use letters, numbers and hyphens — no spaces.");
    }
    return "";
  }
  // commit validates and, when the value is valid and changed, stages just this
  // option (one POST, the staged-changes chip brought in step — the
  // counterpart of a row switch). Resolves true when the field is committed or was unchanged, false
  // when it is invalid or the device refused. A commit already in flight is shared
  // rather than fired twice (a blur then a click elsewhere hit the same field).
  function commit(field) {
    var el = input(field);
    if (!el) return Promise.resolve(true);
    if (field._versoCommit) return field._versoCommit;
    var value = el.value.trim();
    if (value === committed(el)) { clearError(field); return Promise.resolve(true); }
    var message = invalidMessage(el.getAttribute("data-verso-datatype") || "", value);
    if (message) { showError(field, message); return Promise.resolve(false); }
    clearError(field);
    el.value = value;
    field.classList.add("verso-busy");
    var body = new URLSearchParams();
    body.set(el.name, value);
    var token = csrf();
    if (token) body.set("_csrf", token);
    var p = fetch(window.location.pathname, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Verso-Interaction": "field" },
      body: body.toString(),
      credentials: "same-origin",
    }).then(function (res) {
      field.classList.remove("verso-busy");
      if (res.ok || res.redirected) {
        return res.text().then(function (html) {
          el.dataset.committed = value; // this is the saved baseline now
          var doc = new DOMParser().parseFromString(html, "text/html");
          if (window.versoStaged && window.versoStaged.sync) window.versoStaged.sync(doc);
          return true;
        });
      }
      showError(field, T("Couldn’t save that just now — try again."));
      return false;
    }).catch(function () {
      field.classList.remove("verso-busy");
      showError(field, T("Couldn’t save that just now — try again."));
      return false;
    });
    field._versoCommit = p.then(function (r) { field._versoCommit = null; return r; },
      function (e) { field._versoCommit = null; throw e; });
    return field._versoCommit;
  }
  function flush() {
    var dirty = [].filter.call(document.querySelectorAll("[data-verso-inline-field]"), isDirty);
    if (!dirty.length) return Promise.resolve(true);
    return Promise.all(dirty.map(commit)).then(function (oks) {
      var all = oks.every(Boolean);
      if (!all) scrollToFirstError();
      return all;
    });
  }
  // revert drops any typed-but-unstaged edits back to the saved value — what
  // Discard does for a field that never committed.
  function revert() {
    [].forEach.call(document.querySelectorAll("[data-verso-inline-field]"), function (field) {
      var el = input(field);
      if (el && isDirty(field)) { el.value = committed(el); clearError(field); }
    });
  }
  window.versoInline = { anyDirty: anyDirty, flush: flush, revert: revert };

  document.addEventListener("input", function (e) {
    var el = e.target.closest && e.target.closest("[data-verso-inline-input]");
    if (!el) return;
    clearError(el.closest("[data-verso-inline-field]")); // typing dismisses a stale error
  });
  document.addEventListener("keydown", function (e) {
    var el = e.target.closest && e.target.closest("[data-verso-inline-input]");
    if (!el) return;
    var field = el.closest("[data-verso-inline-field]");
    if (e.key === "Enter") {
      e.preventDefault();
      commit(field).then(function (ok) { if (!ok) scrollToFirstError(); });
    } else if (e.key === "Escape") {
      e.preventDefault();
      el.value = committed(el);
      clearError(field);
      el.blur();
    }
  });
  // Leaving the field is the "done": commit what changed. Captured, since blur
  // does not bubble.
  document.addEventListener("blur", function (e) {
    var el = e.target.closest && e.target.closest("[data-verso-inline-input]");
    if (!el) return;
    var field = el.closest("[data-verso-inline-field]");
    commit(field).then(function (ok) { if (!ok) scrollToFirstError(); });
  }, true);
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

// The live preview: the block at the foot of a form saying what it will write,
// kept current while the form is edited (ADR-005 §7 — the plugin declared the
// intent, this is the shell's realization of it).
//
// The answer comes from the plugin, never from here. Which option a switch
// writes, whether its off is a zero or a deletion, how a mark becomes
// `!0x10/0xff` — those are the plugin's readings of a daemon's grammar, and a
// preview this file computed would be a second copy of them that drifts from the
// one that does the writing. So the form is posted as it stands and the plugin
// answers with the block; the shell marks the request a preview, and nothing on
// that path is staged or applied.
(function () {
  // How long a pause counts as "stopped typing". It is only ever waited out for
  // text being typed: a half-written address is not a question worth asking, and
  // each answer costs the router a config read. Everything that settles in one
  // act — a switch, a choice, a chip — asks at once, because there is nothing
  // left to wait for and a control that answers late reads as a control that
  // did not take.
  var QUIET = 300;
  var pending = new Map();
  // Which answer is the current one, per form. A slow question asked early can
  // land after a quick one asked later, and swapping it in would put a stale
  // config on screen under a form that has moved on.
  var asked = new Map();

  // The preview this form answers for. Usually its own — a drawer or an editor
  // page puts the block at the foot of the form it describes. The settings page
  // puts it in the rail beside the form instead, where someone reads it while
  // they work, so a block that belongs to no form at all is taken as the page's
  // one preview. A block inside *another* form is never borrowed: that one is
  // its own form's answer.
  function previewOf(form) {
    var own = form.querySelector("[data-verso-preview]");
    if (own) return own;
    // A block belonging to no form is the page's own — the settings rail's. It
    // answers for the page form beside it, which is the one the shell composes
    // the page around; a search box or the chrome's own forms are not asking
    // about the config and must not send their fields as though they were.
    if (!form.hasAttribute("data-verso-page-form")) return null;
    var scope = form.closest("[data-verso-panel], [data-verso-entity-body], main") || document;
    return [].find.call(scope.querySelectorAll("[data-verso-preview]"), function (block) {
      return !block.closest("form");
    }) || null;
  }

  // Where this form posts. A panel's form posts back into its panel, a page's to
  // its page; either way the preview asks the same address the save would, so
  // the plugin answers about the object the form is actually editing.
  //
  // form.action — the property, never the attribute — is what a form with no
  // action of its own would post to, which is this document's URL *including its
  // query*. That is not a detail: a drawer's whole identity is in the query
  // (?open=cfg02dc81&tab=reaches), so posting to the path alone would ask the
  // plugin about no object at all, and the preview would quietly never update.
  function target(form) {
    return form.getAttribute("hx-post") || form.action || window.location.href;
  }

  function refresh(form, patched) {
    var preview = previewOf(form);
    if (!preview) return;
    var body = new URLSearchParams(new FormData(form));
    var token = csrfToken();
    if (token) body.set("_csrf", token);
    var mine = (asked.get(form) || 0) + 1;
    asked.set(form, mine);
    // Only the question still waiting for an answer wears the fade. An older one
    // finishing must not clear it while a newer one is still out.
    var current = function () { return asked.get(form) === mine; };
    var settle = function () { if (current()) preview.classList.remove("verso-preview-busy"); };
    // The fade says "this is a moment out of date". A block already carrying the
    // value that was just typed is not out of date in any way a reader would
    // care about, so it does not dim — the confirmation arrives under it and
    // usually changes nothing.
    if (!patched) preview.classList.add("verso-preview-busy");
    fetch(target(form), {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "X-Verso-Interaction": "preview",
      },
      body: body.toString(),
      credentials: "same-origin",
    }).then(function (res) {
      settle();
      // 204: the page answered with no preview of its own. Nothing to show and
      // nothing wrong — leave what is on screen.
      if (!res.ok || res.status === 204) return;
      return res.text().then(function (html) {
        // An answer to a question the form has already moved past is not an
        // answer any more: it describes values nobody is looking at.
        if (!current()) return;
        var live = previewOf(form);
        if (!live || !html.trim()) return;
        var parsed = new DOMParser().parseFromString(html, "text/html");
        var fresh = parsed.querySelector("[data-verso-preview]");
        if (fresh) live.replaceWith(fresh);
      });
    }).catch(function () {
      // A preview that could not be fetched leaves the last good one on screen:
      // it is a reading of the form, and a blank box would read as "this writes
      // nothing", which is the one thing it must never say by accident.
      settle();
    });
  }

  function csrfToken() {
    var m = document.querySelector('meta[name="verso-csrf"]');
    return m ? m.content : "";
  }

  // One request per pause, per form. Every control counts — typing, a switch, a
  // chip added to a token box, a condition taken off — because every one of them
  // changes what the form would write.
  function schedule(form, quiet, patched) {
    if (!form || !previewOf(form)) return;
    clearTimeout(pending.get(form));
    pending.set(form, setTimeout(function () {
      pending.delete(form);
      refresh(form, patched);
    }, quiet));
  }

  // Whether what just happened is still being done, or is done.
  //
  // The DOM already draws this line and it is the right one: `input` fires while
  // a value is being changed, `change` when it has settled. So a typed address
  // waits out the pause and a switch, a select or a ticked box does not.
  //
  // The two controls the shell builds itself are settled acts too — a chip is in
  // the box or it is not, a condition is on the object or it is not — and each
  // announces itself by dispatching on its own container. So the test is that
  // the container IS what fired: an input *inside* a token box or a condition is
  // somebody typing in it, and waiting that out is the whole point.
  var SETTLED = "[data-verso-token-list], [data-verso-conditions]";

  function settled(event) {
    return event.type === "change" || event.target.matches(SETTLED);
  }

  // The eager half: put the value on the line the moment it is typed, and let
  // the round-trip confirm it.
  //
  // The browser cannot compute this block — which option a switch writes,
  // whether its off is a zero or a deletion, how a mark becomes `!0x10/0xff` are
  // the plugin's readings of a daemon's grammar. But it does not have to: the
  // line is already there, and this changes the one thing it is allowed to know
  // has changed, which is the value a person just put in a box that says which
  // option it writes.
  //
  // So the rule is narrow on purpose. It patches a line that already exists, for
  // a control that names its option, and nothing else — it never adds a line,
  // never removes one, never guesses at a list. Everything it declines to touch
  // is still answered by the plugin a moment later, and for the controls that
  // settle in one act that moment is now.
  function patch(form, el) {
    var row = el.closest("[data-verso-writes]");
    var preview = previewOf(form);
    var body = preview && preview.querySelector("[data-verso-preview-body]");
    if (!row || !body) return false;
    // A switch is the one control whose off may be a written zero or a deleted
    // option, and only the plugin knows which. It asks at once anyway.
    var kind = (el.type || "").toLowerCase();
    if (kind === "checkbox" || kind === "radio") return false;
    var option = row.getAttribute("data-verso-writes");
    if (!option) return false;
    var line = new RegExp("^(\toption " + option.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + " ')(.*)(')$", "m");
    var was = body.textContent;
    if (!line.test(was)) return false;
    var value = el.value;
    // Emptying a control is not writing an empty value: absence is how this
    // config says "no condition", and a plugin that drops the option entirely is
    // the common case. Whether the line goes or stays is its answer to give, so
    // an emptied control patches nothing and simply asks.
    if (value === "") return false;
    // A function replacement, so a value carrying $ is the value and not a
    // capture-group reference.
    var now = was.replace(line, function (_, head, __, tail) { return head + value + tail; });
    if (now === was) return false;
    body.textContent = now;
    // The copy control holds its own copy of the value, and copying a stale one
    // is worse than not offering it.
    var src = preview.querySelector('[x-ref="src"]');
    if (src) src.textContent = now;
    return true;
  }

  function watch(event) {
    var el = event.target;
    if (!el || !el.closest) return;
    var form = el.closest("form");
    if (!form || !previewOf(form)) return;
    // Zero, not immediate: a burst that lands in one tick — a control that
    // rewrites several fields at once — still asks once.
    schedule(form, settled(event) ? 0 : QUIET, patch(form, el));
  }

  document.addEventListener("input", watch);
  document.addEventListener("change", watch);
})();

// The list's add action refuses malformed tokens before adding them to the form.
window.versoValidate = function (datatype, value) {
  function ipv4(v) { var p=v.split("."); return p.length===4 && p.every(function(n) { return /^(0|[1-9][0-9]{0,2})$/.test(n) && Number(n)<=255; }); }
  function ip(v) { if (ipv4(v)) return true; if (v.indexOf(":")<0 || !/^[0-9a-f:.]+$/i.test(v)) return false; try { new URL("http://["+v+"]/"); return true; } catch (_) { return false; } }
  function host(v) { return v.length<=253 && v.split(".").every(function(p) { return /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/i.test(p); }); }
  if (datatype==="host" && !(ip(value)||host(value))) return T("Enter a valid hostname or IP address.");
  function server(v) { var pair=v.split("#"); return pair.length<=2 && ip(pair[0]) && (pair.length===1 || /^[0-9]+$/.test(pair[1]) && Number(pair[1])>0 && Number(pair[1])<=65535); }
  function scoped(v, address) { if(v[0]!=="/") return false; var parts=v.slice(1).split("/"), target=parts.pop(); return parts.length>0 && parts.every(function(n){ return n==="#" || host(n); }) && (address ? target==="" || target==="#" || ip(target) : target==="#" || server(target)); }
  if (datatype==="dnsserver" && !server(value) || datatype==="dnsaddress" && !scoped(value,true) || datatype==="dnsforward" && !scoped(value,false)) return T("Enter a valid DNS address or domain/server entry.");
  if (datatype==="ip4addr" && !ipv4(value)) return T("Enter a valid IPv4 address.");
  if (datatype==="ipaddr" && !ip(value)) return T("Enter a valid IP address.");
  return "";
};

// Declarative select-dependent fields and ordered list editors.
(function () {
  function branches(scope) {
    (scope || document).querySelectorAll("[data-verso-when]").forEach(function (branch) {
      var form = branch.closest("form");
      var control = form && form.elements.namedItem(branch.dataset.versoWhen);
      // The branch's value may name several: it shows while the control holds any one of them.
      var active = control && branch.dataset.versoWhenValue.split(" ").indexOf(control.value) !== -1 && (control.type !== "checkbox" || control.checked);
      branch.hidden = !active;
      branch.disabled = !active;
    });
  }
  document.addEventListener("change", function (e) { branches(e.target.closest("form") || document); });
  document.addEventListener("DOMContentLoaded", function () { branches(document); });
  document.addEventListener("htmx:afterSwap", function () { branches(document); });
  // versoBreakable writes a machine string into el so that it wraps where it
  // divides itself: after each of the first separator it has, "/" before "."
  // before ":", and nowhere else — widget.Breakable's rule, for a value added
  // here rather than sent by the server.
  function versoBreakable(el, value) {
    var mark = ["/", ".", ":"].filter(function (s) { return value.indexOf(s) >= 0; })[0];
    el.textContent = "";
    if (!mark) { el.textContent = value; return; }
    value.split(mark).forEach(function (part, i, parts) {
      if (i > 0 && (part || i < parts.length - 1)) el.appendChild(document.createElement("wbr"));
      el.appendChild(document.createTextNode(i < parts.length - 1 ? part + mark : part));
    });
  }
  function add(list) {
    var input = list.querySelector("[data-verso-list-input]");
    var value = input.value.trim();
    if (!value) return;
    var error = window.versoValidate && input.dataset.datatype ? window.versoValidate(input.dataset.datatype, value) : "";
    input.setCustomValidity(error || "");
    input.classList.toggle("border-crimson!", !!error);
    if (error) { input.reportValidity(); return; }
    var duplicate = Array.from(list.querySelectorAll("input[type=hidden]")).some(function (i) { return i.value === value; });
    if (!duplicate) {
      var row = list.querySelector("template").content.cloneNode(true);
      row.querySelector("input").value = value;
      versoBreakable(row.querySelector("[data-verso-list-value]"), value);
      list.querySelector("[data-verso-list-items]").appendChild(row);
    }
    input.value = "";
    input.dispatchEvent(new Event("change", {bubbles:true}));
  }
  document.addEventListener("click", function (e) {
    var remove = e.target.closest("[data-verso-list-remove]");
    if (remove) {
      var list = remove.closest("[data-verso-list]");
      var row = remove.closest("[data-verso-list-row]");
      // Focus moves on with the list instead of dropping to the page: to the
      // next row's remove, else the box that adds one.
      var after = row.nextElementSibling;
      var next = (after && after.querySelector("[data-verso-list-remove]")) || list.querySelector("[data-verso-list-input]");
      row.remove();
      if (next) next.focus();
      list.dispatchEvent(new Event("change", {bubbles:true}));
    }
    var button = e.target.closest("[data-verso-list-add]");
    if (button) add(button.closest("[data-verso-list]"));
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Enter" && e.target.matches("[data-verso-list-input]")) { e.preventDefault(); add(e.target.closest("[data-verso-list]")); }
  });
  document.addEventListener("input", function (e) { if (e.target.matches("[data-verso-list-input]")) { e.target.setCustomValidity(""); e.target.classList.remove("border-crimson!"); } });
})();

// A clock command carries the computer's UTC wall time, sampled at the click.
document.addEventListener("click", function(event) {
 var button=event.target.closest('button[name="_action"][value="clock"]');
 if (!button || !button.form) return;
 var input=button.form.elements.namedItem("_client_time");
 if(input) input.value=new Date().toISOString().slice(0,19);
});

// A refusal comes back as a whole page, which a browser shows from its top.
// The page opens where the refusal is instead: the first refused box on it is
// brought to the middle of the view and given the cursor, so the correction
// starts where the eye already was. It waits for Alpine, which opens a folded
// form (a key's add box) that would otherwise have no place to scroll to.
(function () {
  function answer() {
    var refused = document.querySelector('main [aria-invalid="true"]');
    if (!refused) return;
    window.requestAnimationFrame(function () {
      refused.scrollIntoView({ block: "center" });
      refused.focus({ preventScroll: true });
    });
  }
  if (window.Alpine) answer();
  else document.addEventListener("alpine:initialized", answer, { once: true });
})();

// A refusal is about the value that was refused. The first change to a
// refused box is its correction: the box drops aria-invalid, which is all its
// crimson hangs on, and the refusal under it (data-verso-error, named by the
// box's aria-describedby) goes. Submitting asks again, and a value refused
// again comes back refused. An in-place setting keeps its own check.
document.addEventListener("input", function (e) {
  var box = e.target;
  if (!box.matches || !box.matches('[aria-invalid="true"]') || box.closest("[data-verso-inline-field]")) return;
  box.removeAttribute("aria-invalid");
  (box.getAttribute("aria-describedby") || "").split(/\s+/).forEach(function (id) {
    var said = id && document.getElementById(id);
    if (said && said.hasAttribute("data-verso-error")) said.hidden = true;
  });
});

// Code fields expose their logical line count beside the editor.
document.addEventListener("input",function(e){if(!e.target.matches("[data-verso-code-editor]"))return;var counter=e.target.parentElement.querySelector("[data-verso-code-lines]");if(counter)counter.textContent=e.target.value?e.target.value.replace(/\n$/,"").split("\n").length:0;});
