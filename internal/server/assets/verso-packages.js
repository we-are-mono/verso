// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

(function () {
  "use strict";
  var root = document.querySelector("[data-verso-packages]");
  var pageURL = new URL(window.location.href);
  var request, sequence = 0, timer, pollTimer, refreshing = false;
  var submissions = new WeakMap();
  var pendingForms = new WeakSet();

  function actionLabel(form, submitter) {
    var primary = form.querySelector('[name="_primary"]');
    var verb = submitter && submitter.name === "_action" ? submitter.value : primary && primary.value;
    var labels = { search: "Searching…", install: "Installing…", remove: "Removing…", upgrade: "Upgrading…", refresh: "Refreshing index…" };
    return labels[verb] ? T(labels[verb]) : "";
  }

  // Catch repeated Enter submissions before htmx can queue another request
  // on the form. Disabling its button alone does not cover that path.
  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!/^\/system\/packages(?:\/discover)?$/.test(new URL(form.action).pathname)) return;
    if (pendingForms.has(form) || Array.from(form.querySelectorAll('button[type="submit"]')).some(versoButtons.has)) {
      event.preventDefault();
      event.stopImmediatePropagation();
    }
  }, true);

  function failure(message) {
    var error = root && root.querySelector("[data-package-error]");
    if (error) { versoErrorLine(error, message || T("Packages could not be loaded. Try again.")); error.hidden = false; }
  }
  function swap(target, fresh) {
    // Dispose teleported drawers with their owners before replacing the listing.
    if (window.Alpine) window.Alpine.mutateDom(function () {
      window.Alpine.destroyTree(target);
      target.replaceWith(fresh);
      window.Alpine.initTree(fresh);
    });
    else target.replaceWith(fresh);
    if (window.htmx) window.htmx.process(fresh);
    if (window.versoTableFilters) window.versoTableFilters(fresh);
  }
  function filterState() {
    var field = root && root.querySelector("[data-package-query]");
    var cut = root && root.querySelector("[data-verso-listing-cut]");
    return { query: field ? field.value : "", tab: cut ? cut.value : "", focused: document.activeElement === field };
  }
  function restore(state, url) {
    var field = root.querySelector("[data-package-query]");
    field.value = state.query;
    var tab = new URL(url).searchParams.get("tab") === "upgradable" ? "upgradable" : state.tab;
    // Only the inventory narrows in place; the index's cut is already the page.
    var cut = root.querySelector("[data-verso-listing-cut]");
    if (cut && cut.value !== tab) {
      cut.value = tab;
      cut.dispatchEvent(new Event("change"));
    }
    field.dispatchEvent(new Event("input", { bubbles: false }));
    if (state.focused) field.focus();
  }
  async function listing(url, keepQuery) {
    if (!root || refreshing) return;
    var state = filterState(), id = ++sequence;
    if (request) request.abort();
    request = new AbortController();
    root.setAttribute("aria-busy", "true");
    try {
      var res = await fetch(url, { credentials: "same-origin", signal: request.signal, headers: { "X-Verso-Interaction": "packages" } });
      if (!res.ok || res.redirected) throw new Error();
      var doc = new DOMParser().parseFromString(await res.text(), "text/html");
      var fresh = doc.querySelector("[data-verso-packages]");
      if (!fresh || id !== sequence) return;
      // Another operator started a refresh: retain the listing already visible.
      if (fresh.hasAttribute("data-verso-package-busy")) { refreshStarted(); return; }
      fresh = document.importNode(fresh, true);
      swap(root, fresh); root = fresh;
      pageURL = new URL(url, window.location.origin);
      if (!keepQuery) state.query = pageURL.searchParams.get("q") || "";
      restore(state, pageURL);
      // Drawers manage their own address; changing the listing must not steal it.
      if (!document.querySelector('[data-verso-panel] [name="q"]')) history.replaceState(null, "", pageURL.pathname + pageURL.search);
    } catch (e) { if (e.name !== "AbortError") failure(); }
    finally { if (id === sequence && root) root.removeAttribute("aria-busy"); }
  }
  function refreshStarted() {
    if (refreshing) return;
    refreshing = true;
    if (request) request.abort();
    if (root) {
      var button = root.querySelector("[data-package-refresh] button");
      versoButtons.start(button, T("Refreshing index…"));
      root.querySelector("[data-package-note]").textContent = T("Refreshing index…");
    }
    pollTimer = setTimeout(poll, 500);
  }
  function refreshStopped() {
    refreshing = false;
    if (!root) return;
    var button = root.querySelector("[data-package-refresh] button");
    versoButtons.finish(button);
    button.disabled = false; button.removeAttribute("aria-disabled");
    button.removeAttribute("aria-busy");
    button.className = "flex h-9 items-center rounded-xs border border-rule-strong px-4 text-sm font-semibold text-body hover:bg-quiet";
    button.textContent = T("Refresh index");
  }
  async function poll() {
    try {
      var res = await fetch("/system/packages/status", { credentials: "same-origin" });
      if (!res.ok || res.redirected) throw new Error();
      var status = await res.json();
      if (status.refreshing || status.checking) { pollTimer = setTimeout(poll, 1500); return; }
      refreshStopped();
      await listing(pageURL, true);
      refreshSearchPanels();
      if (status.error) failure(status.error);
    } catch (_) {
      // Keep the data and a retryable Refresh control after a lost connection.
      refreshStopped();
      failure(T("Could not check refresh status. Try again."));
    }
  }
  function refreshSearchPanels() {
    document.querySelectorAll('[data-verso-panel] form[action="/system/packages/discover"]').forEach(function (form) {
      var field = form.querySelector('[name="q"]');
      if (field && window.htmx) window.htmx.ajax("GET", "/system/packages/discover?q=" + encodeURIComponent(field.value), { target: form.closest("[data-verso-panel]"), swap: "innerHTML" });
    });
  }
  // The cut's options that name another listing load it rather than narrow
  // this one. Caught on the way down, so the narrowing never sees a value it
  // has no rows for; the rest narrow in place and the address follows them.
  document.addEventListener("change", function (event) {
    var cut = event.target.closest("[data-package-cut]");
    if (!cut || !root || !root.contains(cut)) return;
    var chosen = cut.options[cut.selectedIndex];
    if (chosen && chosen.dataset.href) {
      event.stopPropagation();
      var url = new URL(chosen.dataset.href, window.location.origin);
      if (!url.searchParams.has("q")) url.searchParams.set("q", filterState().query);
      listing(url, true);
      return;
    }
    if (cut.value) pageURL.searchParams.set("tab", cut.value);
    else pageURL.searchParams.delete("tab");
    history.replaceState(null, "", pageURL.pathname + pageURL.search);
  }, true);
  document.addEventListener("click", function (event) {
    var link = event.target.closest("a[data-package-page]");
    if (link && root && !event.ctrlKey && !event.metaKey && !event.shiftKey && event.button === 0) {
      event.preventDefault();
      var url = new URL(link.href);
      // The find field follows the selected listing, including when All is opened.
      if (!url.searchParams.has("q")) url.searchParams.set("q", filterState().query);
      listing(url, true);
      return;
    }
    var files = event.target.closest(FILES);
    if (!files || !files.closest("[data-verso-panel]")) return;
    event.preventDefault();
    loadFiles(files);
  });

  // A package's installed files are always visible in its drawer: the drawer
  // reads them in as it opens — the link coming into view is the drawer
  // opening, since a closed drawer shows nothing — so no row pays for them at
  // page load. The link stays as the way in without script, and a click after
  // a failed read tries again.
  var FILES = 'a[href^="/system/packages/files?"]';
  function loadFiles(files) {
    if (files.dataset.loading || !files.isConnected) return;
    var previousError = files.parentNode.querySelector("[data-package-files-error]");
    if (previousError) previousError.remove();
    files.dataset.loading = "true"; files.setAttribute("aria-busy", "true");
    fetch(files.href, { credentials: "same-origin" }).then(async function (res) {
      if (!res.ok || res.redirected) throw new Error(T("Installed files could not be loaded. Try again."));
      var doc = new DOMParser().parseFromString(await res.text(), "text/html");
      var list = doc.querySelector("[data-package-files]");
      if (!list) throw new Error(T("Installed files could not be loaded. Try again."));
      files.replaceWith(document.importNode(list, true));
    }).catch(function (error) {
      var note = files.parentNode.querySelector("[data-package-files-error]");
      if (!note) { note = versoErrorLine(null, ""); note.dataset.packageFilesError = ""; note.setAttribute("role", "alert"); files.after(note); }
      versoErrorLine(note, error.message);
    }).finally(function () { delete files.dataset.loading; files.removeAttribute("aria-busy"); });
  }
  if ("IntersectionObserver" in window) {
    var seen = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        seen.unobserve(entry.target);
        loadFiles(entry.target);
      });
    });
    var watch = function (scope) {
      [].forEach.call(scope.querySelectorAll(FILES), function (link) {
        if (link.closest("[data-verso-panel]")) seen.observe(link);
      });
    };
    watch(document);
    // A listing read in place, or a drawer fetched when it opens, brings links
    // of its own.
    new MutationObserver(function (records) {
      records.forEach(function (record) {
        [].forEach.call(record.addedNodes, function (node) {
          if (node.nodeType === 1) watch(node.matches && node.matches(FILES) ? node.parentNode : node);
        });
      });
    }).observe(document.body, { childList: true, subtree: true });
  }
  document.addEventListener("input", function (event) {
    if (!event.target.matches("[data-package-query]") || !root) return;
    if (root.dataset.packageAll !== "true") {
      pageURL.searchParams.set("q", event.target.value);
      history.replaceState(null, "", pageURL.pathname + pageURL.search);
      return;
    }
    clearTimeout(timer);
    var url = new URL("/system/packages?tab=all", window.location.origin);
    url.searchParams.set("q", event.target.value);
    pageURL = url;
    // Superseded queries must never replace the current field or its results.
    ++sequence; if (request) request.abort();
    timer = setTimeout(function () { listing(url, true); }, 300);
  });
  document.addEventListener("submit", async function (event) {
    var form = event.target;
    if (!form.matches("[data-package-refresh]")) {
      if (!form.hasAttribute("hx-post") && /^\/system\/packages(?:\/discover)?$/.test(new URL(form.action).pathname)) {
        var label = actionLabel(form, event.submitter);
        if (label) versoButtons.submit(event, label);
      }
      return;
    }
    event.preventDefault();
    if (refreshing) return;
    var data = new URLSearchParams(new FormData(event.target)); data.set("_action", "refresh");
    refreshStarted();
    clearTimeout(pollTimer);
    try {
      var res = await fetch(event.target.action, { method: "POST", credentials: "same-origin", headers: { "X-Verso-Interaction": "packages" }, body: data });
      if (!res.ok || res.redirected) throw new Error();
      pollTimer = setTimeout(poll, 500);
    } catch (_) { poll(); }
  });
  document.addEventListener("htmx:afterRequest", function (event) {
    var xhr = event.detail.xhr;
    var submission = xhr && submissions.get(xhr);
    if (submission) {
      submission.buttons.forEach(versoButtons.finish);
      pendingForms.delete(submission.form);
      submissions.delete(xhr);
    }
    if (xhr && xhr.getResponseHeader("X-Verso-Packages") === "refreshing") refreshStarted();
  });
  document.addEventListener("htmx:beforeRequest", function (event) {
    var elt = event.detail.elt, form = elt && elt.closest("form");
    if (!form || !/^\/system\/packages(?:\/discover)?$/.test(new URL(form.action).pathname)) return;
    if (pendingForms.has(form)) { event.preventDefault(); return; }
    var trigger = event.detail.requestConfig && event.detail.requestConfig.triggeringEvent;
    var active = trigger && trigger.submitter || form.querySelector('button[type="submit"]');
    var label = actionLabel(form, active);
    if (!label) return;
    var controls = form.closest("[data-verso-panel]") || form;
    var buttons = Array.from(controls.querySelectorAll('button[type="submit"]'));
    if (buttons.some(versoButtons.has)) { event.preventDefault(); return; }
    buttons.forEach(function (button) { versoButtons.start(button, button === active ? label : ""); });
    pendingForms.add(form);
    submissions.set(event.detail.xhr, { form: form, buttons: buttons });
  });
  document.addEventListener("verso-packages-changed", function (event) {
    var template = event.detail && event.detail.navigation;
    var navigation = template && template.content.querySelector("[data-verso-nav-rows]");
    var current = document.querySelector("[data-verso-nav-rows]");
    if (navigation && current) swap(current, document.importNode(navigation, true));
    // Allow the closing drawer's transition to finish before disposing its row.
    setTimeout(function () { listing(pageURL, true).then(refreshSearchPanels); }, 280);
  });
  window.addEventListener("pagehide", function () { clearTimeout(pollTimer); clearTimeout(timer); if (request) request.abort(); });
  if (root) {
    restore(filterState(), pageURL);
    if (root.hasAttribute("data-verso-package-busy")) refreshStarted();
  }
})();
