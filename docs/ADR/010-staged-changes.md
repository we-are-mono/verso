# ADR-010 — Staged changes: stage in UCI, apply from the Pending changes page

- **Status:** Accepted
- **Date:** 2026-08-26
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the rail's Pending changes row and its page are
  shell chrome, like the rail itself), ADR-006 (amends the `commit` intent's
  semantics: plugins request *staging*; only the shell ever commits), ADR-007
  (staging, commit, and rollback all ride the same sid-gated rpcd `uci`
  object), ADR-008 (validation is unchanged — it happens at POST time, before
  anything stages).

## Context

Without staging, every UCI write is save-equals-commit: a plugin's successful
POST returns a `commit` intent, and the shell's broker runs `uci set`
followed immediately by `uci commit`. Each Save is live the moment it lands.
That model has three problems on a router:

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
nothing. Save stages; a Pending changes row at the top of the rail, on screen
while a change is pending, counts the stage from `uci changes` and leads to
the Pending changes page, whose Apply is rpcd's `uci apply` with its native
rollback.**

1. **The broker stages; it no longer commits.** On a successful plugin POST the
   shell still executes the declared operations through rpcd (`UCISet`, adds,
   deletes — ACL-gated by the caller's sid exactly as ADR-007 requires), and
   stops there. `uci changes` becomes the single source of truth for "what is
   pending". Staged edits survive navigation, shell restarts, and browser
   crashes; they are visible to `uci changes` on the CLI and to every other
   admin's Pending changes row — a shared stage is UCI's model, and showing it
   is honesty, not a leak.

2. **Interactions that read as instant also stage.** A listing row's enable
   switch posts a minimal form to its plugin page like any other edit; the
   plugin answers with a stage intent for `enabled`, and the Pending changes
   row's count goes up. A dropped row posts its listing's order the moment it lands. No
   client-side ledger, no divergence between what a control shows and what
   `uci changes` holds. The one setting outside the stage is one that changes
   only how the pages look: the theme (ADR-024) is written and committed on its
   own when it is picked, through the helper, and touches nothing the stage
   holds. Nothing on the network can break, so there is nothing to roll back.

3. **The stage has one surface: the Pending changes row, first in the rail,
   and the page it leads to.** The row stands above every destination only
   while a change is staged, in the caveat colour — a fact about the device,
   not an alarm — and says how many changes wait; on a phone the menu button
   carries the count. An edit not yet saved changes nothing outside its form:
   the Save is the act, and the row answers only once it has staged
   something, with the new count. Nothing travels across the screen. The page is
   rendered server-side from truth: the shell asks rpcd for the pending
   changes across the configs plugins have declared and lists them in one card
   per page they were made on — each change as its owning plugin's sentence
   (the describe hook, docs/plugins.md), led by what happened in a word, with
   its way to the page, its discard, and the uci lines it stands for. Above them a **When you apply** card numbers what an
   apply does, in the order it does it (§5), opening with a warning where a
   pending change touches the interface this browser reaches the router
   through. **Apply N** and **Discard all** stand under the changes, read
   before either; Discard all is a secondary act that asks nothing, since the
   router runs as it is either way.
   A page whose own act stages something (a row's switch, an inline field, a
   dropped row) brings the row in step in place; everything else re-renders on
   page load and does not poll. Because the stage outlives the page, leaving
   one needs no warning.

4. **The vocabulary is the LuCI pair.** A drawer's or form's submit says
   **Save** — true to what it does: the edit is kept (it survives navigation
   and restarts, and appears in `uci changes`), not live. The Pending changes
   page says **Apply** — the contrast teaches the model without a word of
   explanation. "Stage" is jargon and "Confirm" implies finality; both are
   wrong for the button.

5. **Apply = `uci apply` with a rollback window, then `confirm`.** It calls
   rpcd's `uci.apply` with a rollback timeout (30 s): rpcd commits every dirty
   config, procd's config triggers reload the affected services — the mapping
   from config to service belongs to the init scripts that declared it, not to
   us — and rpcd arms its rollback. The shell then reconfirms from the browser
   (`uci.confirm`); if the device became unreachable, no confirm arrives and
   the router reverts itself. The safety net runs **on the device**, which is
   the only place it works in exactly the failure it exists for. The page
   says this as the apply's own steps, one line each and in the order they
   happen: it saves the changes; it reloads the services procd's triggers name
   for their configs — read from procd's own table, never a copy the shell
   keeps, and named as one step since procd runs them together; it keeps them
   once the page checks in within the window. Each step shows only what the
   router reports: the save is done when the apply returns, the reloads are
   then started (procd reports no finish), the check-in is kept when the
   confirm lands. Apply never asks first; the warning on the page is the
   warning. When the router puts its settings back, the changes the apply
   carried are staged again as they were, so a rolled-back apply loses no
   work: the shell holds them, a secret's value with them, in memory for the
   window and no longer.

6. **Discard all = `uci revert` per dirty config; a change can also be undone
   alone.** Apply and Discard all act on everything staged, matching UCI's
   semantics. rpcd lets go of a config's stage only whole, so undoing one row,
   or one page's rows, lets go of each config they touch and stages the rest
   of it again through the same rpcd calls a save makes — never by writing
   rpcd's own stage files. A config's staged files are let go of only when a
   file is what is undone. The page offers the undone changes back once.

## Consequences

- `internal/openwrt.Backend` (the ADR-003 seam) grows the rest of the uci
  lifecycle: `UCIChanges`, `UCIRevert` (and its halves `UCIUnstage` and
  `UnstageFiles`), `UCIApply(timeout)`, `UCIConfirm` — all thin rpcd calls
  carrying the sid — and `ReloadTriggers`, procd's trigger table read through
  `service list`.
- The shell gains the Pending changes page's route and the posts it makes —
  apply, confirm, discard, undo and its put-back — CSRF-protected like every
  state-changing request (VS-04).
- ADR-006's `commit` intent keeps its wire shape and changes meaning: it is a
  *stage* request. Plugins never observe a commit; a plugin that needs to know
  whether its config is live can only ask the device, which is the point.
- Ordering on Apply belongs to rpcd and procd, not the shell — one `apply`
  commits all dirty configs. The shell does not sequence per-config commits,
  so it cannot get the sequence wrong.
- Two admins share one stage: either can see, apply, or discard the other's
  pending changes. Accepted — it mirrors two people holding `uci` shells, and
  the Pending changes row at least makes the situation visible, which the CLI
  never did.
- A staged-but-never-applied change persists indefinitely (also true of `uci`
  itself). The rail's row, on every page for as long as the change persists,
  is the standing reminder, and each card on the page names the page its
  changes were made on.
- An undo is not atomic: a write refused part way through its replay would
  leave the config short. The shell then lets go of the config again and
  stages it as it was before the undo, and says the undo failed.

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
- **A bar at the bottom of the viewport.** Built, in two forms — always
  visible and inert when clean, then rising only while the stage was dirty —
  and replaced. It covered the foot of the content, its Save & Apply competed
  with a page form's own Save as a second save-ish control, and it treated the
  stage as the page's when the stage is the device's, shared state that
  follows the operator everywhere.
- **A chip in the top bar that opens a review drawer.** Built and replaced.
  Beside the router's name the chip read as a fact about the router rather
  than a place to go; the drawer opened from the other side of the screen and
  looked like any object's drawer; a Save in a drawer felt finished, and the
  one sign that it was not — a small mark flying to the chip — was easy to
  miss and is no pattern anyone knows; and a drawer had no room to say what an
  apply does once changes piled up across pages. The rail's row is a place,
  shown before the Save, and the page has the room.
- **A floating pill beside the page's own actions.** Rejected: a second
  control that materializes is chrome. The rail's row is one control, in one
  place, on screen only while there is something to act on.
- **Auto-apply after every save (no surface at all).** Rejected: LuCI's
  perceived frictionlessness is a one-click apply in reach, not auto-commit —
  its modal Save stages. Auto-apply would reload services on every toggle flip
  and forfeit the batching a listing-grained UI needs more than a form-grained
  one.
- **Declaring config→service reload mappings in plugin manifests.** Rejected
  as duplication: procd's config triggers already own that mapping on-device,
  and a manifest copy would drift from it.

## Implementation notes

- The broker (`brokerStage` in `internal/server/gateway.go`) stages and never
  commits; the write gate (ADR-007) is otherwise untouched — staging is
  already the ACL-checked operation. Who staged a change is `stageAuthor`
  (`internal/server/stage_authors.go`), and the staged mark on a field is
  `MarkStaged` (`internal/widget/staged.go`).
- The count needs the set of configs to query: the union of `uci`
  declarations across installed plugin manifests (`declaredUCIConfigs` in
  `internal/server/gateway.go`), which the shell loads for the write gate,
  plus the shell's own (ADR-013). The rail's row is built in `buildSidebar`
  (`internal/server/nav.go`); the page is `pendingPath` (`/pending`) in
  `internal/server/pending.go`, and `/uci/review` redirects to it. The routes
  — `handleUCIApply`, `handleUCIConfirm`, `handleUCIDiscard` in
  `internal/server/staged.go`, `handlePendingUndo` and `handlePendingRestore`
  in `internal/server/pending.go` — reach the device through the Backend
  (`internal/openwrt/backend.go`). Undo's replay is `internal/server/undo.go`;
  the apply's steps are `applySteps` (`internal/server/apply_steps.go`); the
  disconnect check is `disconnectOf` (`internal/server/disconnect.go`); what a
  rollback stages again is `restageOf` and `stageAgain`
  (`internal/server/stage_ends.go`). The client is
  `internal/server/assets/verso-commit.js`.
- The confirm round-trip needs the page to come back after a firewall/network
  apply: post-apply, the page polls the confirm route inside the rollback
  window, mirroring LuCI's cadence — and keeps polling even when the apply
  response itself was lost, since reaching the router again is the success
  signal. Confirmed, the page says the changes were applied; a confirm that
  never lands is said in crimson, and the page reads back the changes staged
  again.
- Unsaved work is guarded once, shell-owned: dirty on-page form fields arm the
  browser's native leave-warning until a submit
  (`internal/server/assets/verso-forms.js`). Dirtiness is always a
  comparison with the rendered baseline, not a history of input events, so
  reverting a field to its original value makes the page clean again. What is
  already staged needs no guard — the stage outlives the page. Discard all
  reverts the stage on the device and reloads the page, which reads the
  authoritative state back.
- The page's plain language is the owning plugin's describe hook
  (docs/plugins.md; `serve_described` in
  `plugins/verso-plugin-sdk/src/lib.rs`, answered by each plugin's
  `describe.rs`). Before asking, the shell collapses repeated writes to one
  target (`coalesceChanges`) and normalizes each change into the closed op
  vocabulary the hook reads (`describeChange`), both in
  `internal/server/staged.go`.
  A change no plugin describes reads as its raw uci line.
  The count is the number of the page's rows — what a person did, not the
  number of writes it took.
