# Proposal

## Why

Operators find the staging flow confusing. The marigold chip next to the router's name reads as a fact about the router, not as a place to go. It opens a drawer from the other side of the screen that looks like any object's drawer. Save in a drawer feels finished, so people miss that nothing is live yet. The only hint is a 6px square that flies to the chip, which is easy to miss and not a pattern anyone knows from elsewhere. When changes pile up across several pages, the drawer has no room to say what applying them will do. After this change, everything waiting lives on one page, **Pending changes**, at the top of the sidebar. While an edit is still unsaved, the sidebar row already shows it as a dashed line. Save turns that line solid where it stands. The page says what the router will restart and whether the apply may cut this browser off before anyone presses Apply.

## What Changes

**Sidebar and phone**
- A **Pending changes** row stands at the top of the sidebar, above Overview, only while the stage holds a change. It shows the count, and a `list-todo` icon with its words in Deep Marigold on Marigold Wash. It leads to the Pending changes page.
- On a phone, the menu button carries the same count on Marigold Wash.

**Save is the feedback**
- An unsaved edit changes nothing outside its form; the Save under it is the act.
- A Save reloads the page, so the row arrives with the new count. An act that stages in place (a drawer's Save, a row's one-press act) sets the count where it stands. Nothing animates.
- Under the pointer the row slides as every rail row does.
- The 6px square that flies to the chip is removed.

**The Pending changes page**
- A page at `/pending` replaces the review drawer and `/uci/review`.
- The heading line holds "N pending changes" alone. There is no lede. **Apply N changes** and **Discard all** (a secondary act that asks nothing) stand under the changes.
- The first card, **When you apply**, holds:
  - a marigold "May disconnect you" callout when a change touches the interface this browser reaches the router through;
  - the apply's own steps, numbered because they happen in that order, one line each: "Saves N changes.", "Reloads <services>." (named together from procd's own triggers), "Keeps them if this page checks in within 30 s.";
  - the optional note for config history.
- Under it, one card per page the changes came from, titled by that page's heading. Each card has "Open page" and "Discard these".
- Each change row shows:
  - its verb mark, its sentence and its from → to;
  - when it was staged;
  - go-to, undo and config-lines icons.

**Undo, apply and rollback**
- **Undo** works per row and per page, not only Discard all.
- **Apply runs on the page, showing only what the router reports:**
  - the save step reads Saving, then Saved;
  - the reload step reads Started, and never claims a finish the router does not report;
  - the check-in counts the 30 s rollback window down, then reads Kept;
  - a kept apply empties the page into "Changes applied" and the sidebar row leaves;
  - the outcome is the top-right notification.
- **A rolled-back apply keeps the changes pending.** The shell stages the apply's snapshot again and the page explains what happened. Today the stage is lost.
- **Apply never asks.** A change that may disconnect this browser is warned about by the marigold callout already on the page, and Apply starts at once.
- **The way back after an apply or discard leads to the page of the latest change** ("Back to Firewall rules"), however the operator reached the page.

**Removals and compatibility**
- **BREAKING (visible):** the top-bar chip, the review drawer, `/uci/review` (it now redirects to `/pending`), the flying square and the chip's applied/rolled-back states are removed.
- Plugins and the describe contract are unchanged. Effects come from procd, not from plugins.
- Nothing stored on the router moves, so no migration ships.

## Capabilities

### New Capabilities

- `staged-changes`: how staged changes are shown, previewed, reviewed, undone, applied and kept across a rollback. This covers the sidebar row's content and states, the Pending changes page and its cards, the effects list and the disconnect warning, per-row and per-page undo, the apply sequence with its check-in, and what a rollback leaves behind.

### Modified Capabilities

- `rail`: the rail gains a first row that belongs to the stage. It stands above every destination only while the stage holds a change, it is not part of any group, and the phone's menu button carries its count.

## Impact

**ADRs**
- **ADR-010 (staged changes)** is amended in place:
  - §3 replaces the chip and drawer as "the one surface" with the sidebar row and the page;
  - Discard gains per-change undo;
  - Apply's rollback now re-stages its snapshot;
  - the effects list reads procd's own reload triggers. This keeps to ADR-010's rejection of manifest-declared config→service mappings, which it rejected because procd owns that mapping.
- **ADR-021 (config history)** is cited for the apply note and the rolled-back entry. Its "Stage this change again" path is the mechanism the automatic re-stage reuses.

**Shell**
- `internal/server`:
  - `staged.go`: the view model grows per-group page links, per-item ages and anchors; a new `/pending` handler; the `/uci/review` redirect.
  - `stage_authors.go`: records when each change was staged.
  - `apply_events.go`: re-stage on rollback.
  - New undo routes.
  - A procd-triggers read in `internal/openwrt` (`service list` verbose).
  - A disconnect check from the request's local address.
  - `nav.go` and `nav-row.html.tmpl`: the pending row.
  - `page.html.tmpl`: the chip removed; the menu-button count added.
- Assets:
  - `verso-commit.js`: the fly and chip-verdict code removed; the row's count and the apply-page logic added.
  - `verso-boot.js`: removed with the count it held back for the fly.
  - CSS for the row.

**Docs and catalog**
- DESIGN.md: Navigation, Staging and Drawers.
- `docs/BACKLOG.md`.
- The Slovenian catalog: new strings, and the chip and drawer strings retired.

**Tests**
- `staged_test.go` chip and drawer tests are rewritten to the row and page.
- New tests cover undo, the re-stage on rollback, effects from triggers and the disconnect check.

**Plugins and the router**
- No plugin rebuilds.
- No new rpcd ACL beyond `service list`, which the shell already probes.
