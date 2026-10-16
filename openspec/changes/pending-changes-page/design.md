# Design

## Context

See proposal.md (Why) and the two delta specs for behaviour. The clickable prototype the owner signed off on is https://claude.ai/artifact/VjcRMrhGu7C5DbQthaTNzf. This section covers only what the shipped code does today.

**The stage and its view model**
- The stage is UCI's own per-session stage, reached through sid-gated rpcd (ADR-010).
- `s.staged()` (`internal/server/staged.go`) builds `stagedView` on every page render: `coalesceChanges` → `humanizeChange` → `describedGroups` → `describeStaged` → `stagedItemOf`.
- Rows are grouped by the page that made them. That page comes from `stageAuthors` (`stage_authors.go`), an in-memory map from sid to change address to author. The count is the number of rows.
- `withWas` reads committed values to show from → to on settings.

**The chip, the drawer and their script**
- The top-bar chip `verso-staged-chip` is included at `page.html.tmpl:55`. The review drawer and the no-script page both render the `verso-staged-review` template from `GET /uci/review`.
- `verso-commit.js` owns:
  - the chip's verdicts;
  - `versoStaged.sync` and `fly`;
  - the save-landing code (sessionStorage `verso-staged-origin`);
  - the apply and confirm loop (`apply()` → `versoConfirmApply` polls `/uci/confirm` every 0.5 s for about 28 s).

**Apply, rollback and discard**
- `handleUCIApply` snapshots the stage (`stagedChange`, ADR-021), calls `uci apply {rollback:true, timeout:30}`, then `watchApply`.
- On rollback, `watchApply` records `ChangeRolledBack` and nothing else. The stage is gone, because rpcd committed it before arming the rollback.
- Config history's "Stage this change again" stages a recorded change through `stagePlan(forwardOf(change))`.
- `handleUCIDiscard` reverts per declared config. Nothing reverts a single option or section: neither the shell nor `verso-rpcd` can.

**Hooks the new design builds on**
- The rail's `navRow` (`nav.go`, `nav-row.html.tmpl`) already carries `Detail`, `Dot` and `Variant`. System uses them for updates, so the rail already has one status exception.
- `verso-forms.js` exposes `versoDirtyState` (`dirty()`, `set`, `suppress`) against a baseline signature per form. It has no events.
- `versoOutcome.show(el)` (`verso-page.js`) posts the top-right notification.
- The describe contract (`internal/plugin/transport.go`, SDK `Description`) has no notion of effects. ADR-010 rejects manifest-declared config→service mappings because procd owns that mapping.

## Goals / Non-Goals

**Goals:**
- One surface for the stage, the rail row and its page, built from the existing `stagedView` pipeline. There is no second describe path.
- Save is the feedback: the row answers a staged change, never an unsaved edit.
- Undo and the rollback re-stage use only rpcd's public `uci` methods.
- Effects come from the router's own reload triggers.

**Non-Goals:**
- A rollback-free ("apply unchecked") path for changes that sever this browser. The disconnect warning names the cause; the router still rolls back if the page cannot answer.
- Disconnect detection beyond the interface that holds the browser's address. Firewall input policy, dropbear and uhttpd listen addresses, and bridge member changes are not checked.
- Renaming System › Changes.
- Any change to the describe contract or to plugins.

## Decisions

### 1. The row is a `navRow`, first in `buildSidebar`, outside every group

`buildSidebar` prepends a pending row while `stagedView.Count > 0`. It uses the existing `navRow` fields: the label "Pending changes", the `list-todo` icon, the count in `Detail`, and a new `Variant` "pending" that draws Marigold Wash with Deep Marigold words.

The row always renders, hidden when the stage is clean, the way the chip did. That lets the client turn it on after a one-press stage without building one. Group logic (`railOrder`, open/closed state) never sees it.

On a phone, the hamburger at `page.html.tmpl:43` gains a count span. The same data comes from `pageData.Staged`.

*Alternative considered:* a separate rail section template. Rejected because `navRow` already models a row with a status detail, and a second row type would drift from it.

### 2. `/pending` is a page built from `stagedView`; the drawer template is retired

`handlePending` renders a page:
- the heading line, the count alone;
- the When you apply card;
- one card per `stagedGroup`;
- Apply N and Discard all under the last card, where a page form's Save stands.

`stagedGroup` gains the group's page heading and its href. The href comes from `stageAuthor`, which already knows the plugin and path, so it records that path too.

`stagedItem` gains:
- `Age`, from a staging time `stageAuthors.note` now records;
- `Anchor`, an element id the item's page puts on its staged mark. `MarkStaged` adds `id="staged-<config>.<section>.<option>"` for settings. A drawer item's row gets `id="staged-<config>.<section>"`.

When nothing is pending after an apply or a Discard all, the page's way back is "Back to <page>", where <page> is the page the most recent of those changes was made on. That comes from the apply's snapshot, or from the authors read just before the discard, and is kept per sid beside the rollback record. It does not come from the referrer: the referrer is the Pending page itself after an undo, a rollback reload or a no-script POST, it is absent from a bookmark, and it names wherever the operator wandered last. With no such change on record, no way back is offered.

`GET /uci/review` answers 308 to `/pending`. The `verso-staged-chip` and `verso-drawer-review` templates are deleted. The page composes existing widgets: cards, the settings-row from → to, `act` buttons, callout, confirm and the notification. No new widget is added.

*Alternative considered:* keep the drawer and add a page. Rejected because two surfaces for one stage was the original confusion.

### 3. Save puts the change in the row; nothing previews or animates

An unsaved edit changes nothing outside its form: the Save under it is the act. A page form's Save reloads the page, so the row is drawn anew with the stage's count. An act that stages in place (a drawer's Save, a row's one-press act) hands `versoStaged.sync` the page it read, and the row takes that count as it stands: it appears with the first change and leaves with the last.

`stagedView` gains `Latest`: the sentence of the most recently staged row, using the staging time from decision 2. The row carries it, and an in-place stage announces it to a screen reader.

The save's sessionStorage note keeps only where its Save stood, so the page comes back at the same height. `fly`, the flight CSS, the chip verdicts and `verso-boot.js`, which held the count back for the flight, are deleted.

*Alternative considered:* a dashed line in the row previewing an unsaved edit, settling into the page's words on Save. Built and dropped: a row that appears before Save reads as if the edit already waited, and Save already reloads the page, so a settling animation repeats what the reload shows.

### 5. Undo reverts the config and replays the rest

`POST /pending/undo` takes a row's change addresses, or a group's, and does this:

1. Reads the session's stage (`UCIChanges`) for each affected config.
2. Reverts each of those configs (`uci revert {config}`), plus `discardFiles` for file families.
3. Replays every remaining op in its original order through rpcd's `set`, `add`, `delete`, `rename` and `order` calls. Anonymous sections are re-added: rpcd returns a new name, and later ops in the replay are rewritten to use it.
4. Re-notes their authors with their original times.

"Put it back" posts the undone ops to `POST /pending/restore`. The response carries those ops in the notification's form and they are replayed the same way.

*Alternative considered:* a `verso-rpcd` method that deletes lines from rpcd's per-session delta files. Rejected because it couples the shell to rpcd's private savedir format. Revert-and-replay stays on the documented API.

### 6. Effects are read from procd's reload triggers

`internal/openwrt` adds a read of `ubus call service list {"verbose": true}`. The ACL is the `service list` the shell already probes. From each service's `config.change` triggers, it builds the map from package to services. The page lists the services the pending configs reach.

The words come from the shell catalog, keyed by service name: network, dnsmasq, firewall, wireless (netifd's wifi), odhcpd, dropbear, uhttpd, system and sysntpd. Any other service reads "<service> reloads."

The card numbers the apply's own steps, because only those happen in a known order. Each step is one line:
1. "Saves N changes." This is rpcd's commit.
2. "Reloads <services>." procd runs the triggers together, so the services are named in one step in no promised order, with network first in the sentence.
3. "Keeps them if this page checks in within 30 s." This is the confirm, inside the rollback window.

Each step shows only what the router reports:
- Step 1 reads Saving while the apply POST runs and Saved when it returns. At that point rpcd has committed and handed the triggers to procd.
- Step 2 then reads Started and never Done, because procd reports no finish.
- Step 3 counts the rollback window down, then reads Kept on `/uci/confirm` or Put back on rollback.

The page after a kept apply reads "Changes applied", not "Everything is live", because the reloads may still be settling.

*Alternatives considered:*
- One numbered row per service, ticking Working → Done. Rejected because the order is not the router's and the Done marks would be invented.
- Per-service completion by polling procd for restarted instances. Rejected because a reload often keeps the pid, so the signal would be untrue.

### 7. The disconnect check uses the address the browser connected to

`handlePending` reads the request's local address (`http.LocalAddrContextKey`), finds the interface holding it in `network.interface dump`, and flags a pending change that sets or removes an option of that interface's `network` section, or removes the section.

The callout names the interface's current address and the pending one. It is the whole warning: Apply never asks. The owner ruled out a confirm because the callout already stands on the page.

### 8. A rollback stages its snapshot again

`handleUCIApply` reads the stage once, before the apply empties it, for two uses: config history's change, and what a rollback would stage again. The second is the stage's coalesced changes as rpcd journals them, each with who staged it, plus each reordered config's staged order.

`watchApply`'s rollback callback records `ChangeRolledBack`, then stages those changes again through the same replay an undo uses (decision 5). It also stores the rollback time per sid, so `/pending` can draw its callout.

This does not use `stagePlan(forwardOf(change))`, the path "Stage this change again" takes (ADR-021). Config history keeps no secret's value, so forwardOf leaves a secret unset, and a rolled-back Wi-Fi password would be lost. The replay's copy of a secret is held in memory for the rollback window only and is never written to history.

The snapshot is taken whether or not the history store exists. Re-stage does not depend on history.

On the client, the confirm loop's timeout:
- posts the crimson notification;
- reloads `/pending`, which now reads the re-staged changes and the callout.

### 9. Without scripting

Undo, Undo these and Discard all are form POSTs that redirect back to `/pending`. Apply is replaced by a sentence saying it needs scripting. This is truthful, because the router keeps an apply only when the page confirms it.

## Risks / Trade-offs

- **Replay is not atomic.** An op failing mid-replay could lose staged changes. → Read the full stage before reverting. On any replay error, revert the config again and replay the original stage. Report the failure with the danger notification, and test it with the fake backend.
- **procd's trigger shape varies across OpenWrt releases.** → Parse defensively: an unknown trigger shape lists nothing. Pin fixtures from 24.10 and 25.12 `service list` output in tests.
- **The preview line and the saved sentence can word the same change differently** ("Action → DROP" before Save, the plugin's sentence after). → Accepted. After Save, the page's wording wins, so the row and the page always agree.
- **Another tab or session staging changes the count under the row.** → The row re-renders with every page, and `sync` reads the count the server sends. The count roll compares against the stored value only while it is the same session's save.
- **The rail gains status, against the "radical calm" direction.** → It appears only while something waits, as the System updates dot already does. DESIGN.md records it as the second named exception.
- **A session that expires inside the rollback window cannot be re-staged.** → History still records the rollback, and "Stage this change again" remains on the Changes page.

## Migration Plan

- Nothing on the router moves: no UCI, init or `/data` change, so no migration.
- `/uci/review` redirects for bookmarks.
- The release notes say the chip is gone and where pending changes now live.
- Plugins need no rebuild.

