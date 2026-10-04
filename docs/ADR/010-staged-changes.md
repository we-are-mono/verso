# ADR-010 — Staged changes: stage in UCI, apply from the review drawer

- **Status:** Accepted
- **Date:** 2026-08-26
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the staged-changes chip and its drawer are shell
  chrome, like the modal and the drawer), ADR-006 (amends the `commit` intent's
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
nothing. Save stages; a chip in the top bar, on screen while a change is
pending, counts the stage from `uci changes` and opens the review drawer,
whose Apply is rpcd's `uci apply` with its native rollback.**

1. **The broker stages; it no longer commits.** On a successful plugin POST the
   shell still executes the declared operations through rpcd (`UCISet`, adds,
   deletes — ACL-gated by the caller's sid exactly as ADR-007 requires), and
   stops there. `uci changes` becomes the single source of truth for "what is
   pending". Staged edits survive navigation, shell restarts, and browser
   crashes; they are visible to `uci changes` on the CLI and to every other
   admin's chip — a shared stage is UCI's model, and showing it is honesty,
   not a leak.

2. **Interactions that read as instant also stage.** A listing row's enable
   switch posts a minimal form to its plugin page like any other edit; the
   plugin answers with a stage intent for `enabled`, and the chip's count goes
   up. A dropped row posts its listing's order the moment it lands. No
   client-side ledger, no divergence between what a control shows and what
   `uci changes` holds.

3. **The stage has one surface: a chip in the top bar, left of Log out, on
   screen while a change is staged.** It says how many changes wait and that
   nothing is live yet, in the caveat colour — a fact about the device, not an
   alarm — and it opens the review drawer. It is rendered server-side from
   truth on every page: the shell asks rpcd for the pending changes across the
   configs plugins have declared, and the drawer lists them under the page
   they belong to — each change as its owning plugin's sentence (the describe
   hook, docs/plugins.md), led by what happened in a word and opening on the
   uci lines it stands for — with **Apply N · Discard all** and the rollback
   promise beneath. A page whose own act stages something (a row's switch, an
   inline field, a dropped row) brings the chip in step in place; everything
   else re-renders on page load and does not poll. Because the stage outlives
   the page, leaving one needs no warning.

4. **The vocabulary is the LuCI pair.** A drawer's or form's submit says
   **Save** — true to what it does: the edit is kept (it survives navigation
   and restarts, and appears in `uci changes`), not live. The drawer says
   **Apply** — the contrast teaches the model without a word of explanation.
   "Stage" is jargon and "Confirm" implies finality; both are wrong for the
   button.

5. **Apply = `uci apply` with a rollback window, then `confirm`.** It calls
   rpcd's `uci.apply` with a rollback timeout (30 s): rpcd commits every dirty
   config, procd's config triggers reload the affected services — the mapping
   from config to service belongs to the init scripts that declared it, not to
   us — and rpcd arms its rollback. The shell then reconfirms from the browser
   (`uci.confirm`); if the device became unreachable, no confirm arrives and
   the router reverts itself. The safety net runs **on the device**, which is
   the only place it works in exactly the failure it exists for.

6. **Discard all = `uci revert` per dirty config.** The stage is one unit:
   Apply and Discard all act on everything staged, matching UCI's semantics.
   The drawer lists every pending change, but does not offer per-item revert —
   `uci revert` can target a single section, so nothing here forecloses finer
   granularity; it is simply not part of this decision.

## Consequences

- `internal/openwrt.Backend` (the ADR-003 seam) grows the rest of the uci
  lifecycle: `UCIChanges`, `UCIRevert`, `UCIApply(timeout)`, `UCIConfirm` —
  all thin rpcd calls carrying the sid.
- The shell gains the drawer's route and the three the drawer posts to —
  apply, discard, confirm — CSRF-protected like every state-changing request
  (VS-04).
- ADR-006's `commit` intent keeps its wire shape and changes meaning: it is a
  *stage* request. Plugins never observe a commit; a plugin that needs to know
  whether its config is live can only ask the device, which is the point.
- Ordering on Apply belongs to rpcd and procd, not the shell — one `apply`
  commits all dirty configs. The shell does not sequence per-config commits,
  so it cannot get the sequence wrong.
- Two admins share one stage: either can see, apply, or discard the other's
  pending changes. Accepted — it mirrors two people holding `uci` shells, and
  the chip at least makes the situation visible, which the CLI never did.
- A staged-but-never-applied change persists indefinitely (also true of `uci`
  itself). The chip, on every page for as long as the change persists, is the
  standing reminder.

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
  follows the operator everywhere. The chip lives in the chrome beside Log
  out, where the app-wide facts already stand, and the drawer is the same
  drawer everything else opens.
- **A floating pill beside the page's own actions.** Rejected: a second
  control that materializes is chrome. The chip is one control, in one place,
  on screen only while there is something to act on.
- **Auto-apply after every save (no surface at all).** Rejected: LuCI's
  perceived frictionlessness is a one-click apply in reach, not auto-commit —
  its modal Save stages. Auto-apply would reload services on every toggle flip
  and forfeit the batching a listing-grained UI needs more than a form-grained
  one.
- **Declaring config→service reload mappings in plugin manifests.** Rejected
  as duplication: procd's config triggers already own that mapping on-device,
  and a manifest copy would drift from it.

## Implementation notes

- The broker (`brokerStage`) stages and never commits; the write gate
  (ADR-007) is otherwise untouched — staging is already the ACL-checked
  operation.
- The chip's count needs the set of configs to query: the union of `uci`
  declarations across installed plugin manifests, which the shell loads for
  the write gate, plus the shell's own (ADR-013). The chip is `#verso-staged`
  in the top bar; the drawer's contents render from `staged.html.tmpl` at
  `/uci/review`, fetched when it opens; the client is `verso-commit.js`.
- The confirm round-trip needs the page to come back after a firewall/network
  apply: post-apply, the drawer polls the confirm route inside the rollback
  window, mirroring LuCI's cadence — and keeps polling even when the apply
  response itself was lost, since reaching the router again is the success
  signal. Confirmed, the drawer closes, the chip says so in green and fades,
  and the page reloads clean behind it; a confirm that never lands is said in
  crimson, and the drawer reopens.
- Unsaved work is guarded once, shell-owned: dirty on-page form fields arm the
  browser's native leave-warning until a submit. Dirtiness is always a
  comparison with the rendered baseline, not a history of input events, so
  reverting a field to its original value makes the page clean again. What is
  already staged needs no guard — the stage outlives the page. Discard all
  reverts the stage on the device and reloads the page, which reads the
  authoritative state back.
- The drawer's plain language is the owning plugin's describe hook
  (docs/plugins.md); a change no plugin describes reads as its raw uci line.
  The count is the number of drawer rows — what a person did, not the number of
  writes it took.
