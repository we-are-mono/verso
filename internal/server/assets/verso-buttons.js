// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// One waiting state for an action and its companions. Keep the original nodes
// and attributes so a failed request gives back the same usable controls.
var versoButtons = (function () {
  var waiting = new WeakMap();
  var submissions = new Map();
  function start(button, label) {
    if (!button || waiting.has(button)) return false;
    waiting.set(button, {
      nodes: Array.from(button.childNodes),
      disabled: button.disabled,
      ariaDisabled: button.getAttribute("aria-disabled"),
      ariaBusy: button.getAttribute("aria-busy"),
      minWidth: button.style.minWidth,
    });
    button.disabled = true;
    button.setAttribute("aria-disabled", "true");
    if (label) {
      button.style.minWidth = button.getBoundingClientRect().width + "px";
      button.setAttribute("aria-busy", "true");
      button.classList.add("verso-button-waiting");
      var mark = document.querySelector("[data-verso-button-waiting]");
      button.replaceChildren(mark.content.cloneNode(true), document.createTextNode(label));
    }
    return true;
  }
  function finish(button) {
    var saved = waiting.get(button);
    if (!saved) return;
    button.replaceChildren.apply(button, saved.nodes);
    button.disabled = saved.disabled;
    [["aria-disabled", saved.ariaDisabled], ["aria-busy", saved.ariaBusy]].forEach(function (attribute) {
      if (attribute[1] === null) button.removeAttribute(attribute[0]);
      else button.setAttribute(attribute[0], attribute[1]);
    });
    button.style.minWidth = saved.minWidth;
    button.classList.remove("verso-button-waiting");
    waiting.delete(button);
  }
  function submit(event, label) {
    var form = event.target;
    if (submissions.has(form)) { event.preventDefault(); return; }
    if (event.defaultPrevented) return;
    var button = event.submitter || form.querySelector('button[type="submit"]');
    if (!button || button.disabled) { event.preventDefault(); return; }
    // Disabled submitters are omitted by native serialization. Preserve the
    // clicked action while the browser sends the rest of the form normally.
    var carrier;
    if (button.name) {
      carrier = document.createElement("input");
      carrier.type = "hidden"; carrier.name = button.name; carrier.value = button.value;
      form.appendChild(carrier);
    }
    var buttons = Array.from(form.querySelectorAll("button"));
    buttons.forEach(function (control) { start(control, control === button ? label : ""); });
    submissions.set(form, { buttons: buttons, carrier: carrier });
  }
  // Browser history can restore the submitted document without reloading it.
  window.addEventListener("pageshow", function () {
    submissions.forEach(function (submission) {
      submission.buttons.forEach(finish);
      if (submission.carrier) submission.carrier.remove();
    });
    submissions.clear();
  });
  return { start: start, finish: finish, submit: submit, has: function (button) { return waiting.has(button); } };
})();
