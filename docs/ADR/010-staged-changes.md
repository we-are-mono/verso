# ADR-010 — Staged changes: stage in UCI, apply from the page bar

- **Status:** Accepted
- **Date:** 2026-08-26
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the staged-changes bar is shell chrome, like the
  modal and the drawer), ADR-006 (amends the `commit` intent's semantics:
  plugins request *staging*; only the shell ever commits), ADR-007 (staging,
  commit, and rollback all ride the same sid-gated rpcd `uci` object), ADR-008
  (validation is unchanged — it happens at POST time, before anything stages).

## Context

Every UCI write in Verso is save-equals-commit: a plugin's successful
POST returns a `commit` intent, and the shell's broker executes `UCISet`
followed immediately by `UCICommit` (`internal/server/gateway.go`; that
immediate-commit `brokerCommit` is now the staged `brokerStage` this ADR
introduces). Each Save is live the moment it lands. That model has three
problems on a router:

- **No atomicity.** A change that only makes sense as a set (retarget a zone,
  then fix the rules that reference it) goes live piecemeal, with the firewall
  reloading in a half-edited state between saves.
- **No safety net.** A wrong firewall or network edit can sever the operator's
  own path to the device, with nothing to undo it.
- **It contradicts the platform.** UCI is natively a two-phase store: `uci set`
  stages into `/tmp/.uci`, `uci commit` persists, `uci changes` lists the
  difference — and rpcd's `uci` object exposes the whole lifecycle over the
  session-gated bus, including `apply` with a built-in rollback timer and
  `confirm` to disarm it (`sources/luci/…/resources/uci.js`, `callApply`
  declares `timeout`/`rollback`). LuCI's Save & Apply is this mechanism. Verso
  collapses the two phases and forfeits all of it.

LuCI is the precedent, verified in its source: a zone-edit modal's **Save**
button parses the form and pushes the edits over rpcd as `uci set/add/delete` —
onto the router's stage, no commit (`form.js` `handleModalSave` →
`uci.js` `save()`); the page-level **Save & Apply**, always sitting at the
bottom of the content, is what runs `uci apply` with the rollback. Fifteen
years of that pairing trained every OpenWrt user: *Save keeps it, Save & Apply
makes it live*. This ADR decides Verso's version of that model end to end: the
storage, the wire flow, and the surface.

## Decision

**Staged state lives in UCI's own stage, server-side. The browser holds
nothing. Save stages; the always-visible staged-changes bar at the bottom of
the content renders from `uci changes`, and its Save & Apply is rpcd's
`uci apply` with its native rollback.**

1. **The broker stages; it no longer commits.** On a successful plugin POST the
   shell still executes the declared operations through rpcd (`UCISet`, adds,
   deletes — ACL-gated by the caller's sid exactly as ADR-007 requires), and
   stops there. `uci changes` becomes the single source of truth for "what is
   pending". Staged edits survive navigation, shell restarts, and browser
   crashes; they are visible to `uci changes` on the CLI and to every other
   admin's bar — a shared stage is UCI's model, and showing it is honesty,
   not a leak.

2. **Interactions that read as instant also stage.** A listing row's enable
   switch posts a minimal form to its plugin page like any other edit; the
   plugin answers with a stage intent for `enabled`, and the bar's count goes
   up. No client-side ledger, no divergence between what a toggle shows and
   what `uci changes` holds.

3. **The staged-changes bar is in the flow, at the bottom of the content, on
   every page — always.** Form actions live at the bottom of forms; controls
   that materialize when state changes are chrome, and people's hands already
   expect the LuCI position. The bar is rendered server-side from truth: the
   shell asks rpcd for the pending changes across the configs plugins have
   declared and renders **Save & Apply · Discard · the count · Review** (the
   change tuples in plain words). A composed page form also layers its current
   browser-local diff over that server truth until Save & Apply validates and
   stages it; this is unsaved form state, not a second durable stage. A clean
   page shows the same bar inert — "No pending changes", actions disabled. It
   re-renders on page load and does not poll.

4. **The vocabulary is the LuCI pair.** A drawer's or form's submit says
   **Save** — true to what it does: the edit is kept (it survives navigation
   and restarts, and appears in `uci changes`), not live. The bar says
   **Save & Apply** — the contrast teaches the model without a word of
   explanation. "Stage" is jargon and "Confirm" implies finality; both are
   wrong for the button.

5. **Save & Apply = `uci apply` with a rollback window, then `confirm`.** It calls
   rpcd's `uci.apply` with a rollback timeout (30 s): rpcd commits every dirty
   config, procd's config triggers reload the affected services — the mapping
   from config to service belongs to the init scripts that declared it, not to
   us — and rpcd arms its rollback. The shell then reconfirms from the browser
   (`uci.confirm`); if the device became unreachable, no confirm arrives and
   the router reverts itself. The safety net runs **on the device**, which is
   the only place it works in exactly the failure it exists for.

6. **Discard = `uci revert` per dirty config.** The stage is one unit: Save &
   Apply and Discard act on everything staged, matching UCI's semantics.
   Review lists every pending change, but does not offer per-item revert —
   `uci revert` can target a single section, so nothing here forecloses finer
   granularity; it is simply not part of this decision.

## Consequences

- `internal/openwrt.Backend` (the ADR-003 seam) grows the rest of the uci
  lifecycle: `UCIChanges`, `UCIRevert`, `UCIApply(timeout)`, `UCIConfirm` —
  all thin rpcd calls carrying the sid.
- The shell gains three routes the bar posts to — apply, discard, confirm —
  CSRF-protected like every state-changing request (VS-04). On styleguide
  pages the bar sits inert, since that plugin declares no uci configs and
  stages nothing.
- ADR-006's `commit` intent keeps its wire shape and changes meaning: it is a
  *stage* request. Plugins never observe a commit; a plugin that needs to know
  whether its config is live can only ask the device, which is the point.
- Ordering on Apply belongs to rpcd and procd, not the shell — one `apply`
  commits all dirty configs. The shell does not sequence per-config commits,
  so it cannot get the sequence wrong.
- Two admins share one stage: either can see, apply, or discard the other's
  pending changes. Accepted — it mirrors two people holding `uci` shells, and
  the bar at least makes the situation visible, which the CLI never did.
- A staged-but-never-applied change persists indefinitely (also true of `uci`
  itself). The bar's presence on every page is the standing reminder.

## Alternatives considered

- **Keep save-equals-commit.** The simplest path; rejected for the three
  problems in Context — no atomicity, no safety net, and a platform fighting
  its own storage model.
- **Stage in shell memory.** An in-process ledger of pending ops, committed on
  Apply. Rejected: it dies with the process, is invisible to `uci changes` and
  to other admins, and re-implements — worse — what UCI already provides.
- **Stage in the browser** (the current demo, promoted). Rejected outright: a
  reload silently discards "pending" work, and the UI claims state the system
  does not hold.
- **A shell-side rollback timer.** Rejected: the failure Apply must survive is
  the operator cutting their own connectivity; a timer on the far side of the
  severed link cannot help. rpcd's device-side rollback exists precisely for
  this.
- **A floating pill that appears when the stage is dirty.** Built and
  rejected: controls that materialize are chrome, and hidden-until-dirty is
  not how forms work — actions belong at the bottom of the content, always
  visible, where LuCI put them and where hands expect them.
- **Auto-apply after every save (no bar at all).** Rejected: LuCI's perceived
  frictionlessness is an always-visible one-click apply, not auto-commit — its
  modal Save stages. Auto-apply would reload services on every toggle flip and
  forfeit the batching a listing-grained UI needs more than a form-grained
  one.
- **Declaring config→service reload mappings in plugin manifests.** Rejected
  as duplication: procd's config triggers already own that mapping on-device,
  and a manifest copy would drift from it.

## Implementation notes

- The broker (`brokerStage`) stages and never commits; the write gate
  (ADR-007) is otherwise untouched — staging is already the ACL-checked
  operation.
- The bar's state needs the set of configs to query: the union of `uci`
  declarations across installed plugin manifests, which the shell loads for
  the write gate. In code the bar's element and script keep the working name
  "capsule".
- The confirm round-trip needs the page to come back after a firewall/network
  apply: post-apply, the bar polls the confirm route inside the rollback
  window, mirroring LuCI's cadence — and keeps polling even when the apply
  response itself was lost, since reaching the router again is the success
  signal.
- Unsaved work is guarded at two levels, shell-owned: a dirty dialog (drawer,
  modal) asks before closing, and dirty on-page form fields arm the browser's
  native leave-warning until a submit. Dirtiness is always a comparison with
  the rendered baseline, not a history of input events, so reverting a field
  to its original value makes the page clean again. Discard restores that
  rendered baseline in JavaScript, including forms in drawers; when a real UCI
  stage exists, the shell reverts it and fetches the authoritative form state
  before restoring the clean capsule.
- Review's plain-language rendering starts mechanical (config, section,
  option, old → new). Per-plugin humanization of change tuples is a widget
  vocabulary question and stays out of this ADR.
