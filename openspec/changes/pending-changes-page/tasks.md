# Tasks

Every task ends green under `make lint` and `make test`. A task that touches a template or stylesheet stages the rebuilt `internal/server/assets/verso.css` with it. Browser-side logic is tested with `node --test scripts/*.test.cjs`. The visual reference is the prototype linked in design.md's Context.

## 1. What the stage records

- [x] 1.1 Write the tests first in `stage_authors_test.go`, then extend `stageAuthor` and `note` to pass them. The tests:
  - `note` records the time a change was staged and the path of the page it was made on;
  - re-noting an address keeps the newer time;
  - `forget` drops both.
- [x] 1.2 Write the tests first in `staged_test.go`, then implement in `staged.go` and verify:
  - `stagedGroup` carries its page heading and href;
  - `stagedItem` carries `Age` (empty when no time is on record) and `Anchor` (`staged-<config>.<section>[.<option>]`);
  - `stagedView.Latest` is the sentence of the most recently staged row;
  - the count is still rows, not writes (`TestStagedCountsItemsNotWrites` unchanged).
- [x] 1.3 Write the test first in `internal/widget/staged_test.go`, then implement in `MarkStaged` and the shared row template and verify: a staged setting's mark carries `id="staged-<config>.<section>.<option>"`. A table row already carries its section as `data-verso-row-id`, so going to an item's anchor (`staged-<config>.<section>`) finds its row by that id in the browser (task 8.2).

## 2. What applying will do

- [x] 2.1 Add a `service list {"verbose": true}` fixture captured from the OpenWrt 25.12 lab under `internal/openwrt/testdata`, beside synthetic shapes in the test (no 24.10 router was at hand; procd's trigger shape is the same). Write the tests first, then implement the read and verify. The tests:
  - the parse maps package → init scripts from `config.change` triggers, a script named by its own path, a non-init-script action naming nothing;
  - an unknown trigger shape yields nothing;
  - a missing ACL is an error from the read, which the page turns into no reload step (task 6.1).
- [x] 2.2 Write the tests first, then implement the card's steps and verify the i18n audit stays at zero:
  - 3 pending dhcp + firewall changes give 1 "Saves 3 changes.", 2 "Reloads DNS & DHCP and Firewall.", 3 "Keeps them if this page checks in within 30 s.", one line each;
  - Network leads the reload sentence;
  - a stage whose configs have no trigger has no reload step and numbers the check-in 2;
  - an unknown service is named by its procd name;
  - the words are in the shell catalog, with Slovenian added.

## 3. May disconnect you

- [x] 3.1 Write the tests first, then implement the check from `http.LocalAddrContextKey` and `network.interface dump` and verify. With a fake request whose local address is 192.168.1.1 on `lan`:
  - a pending `network.lan.ipaddr` is flagged with both addresses;
  - removing the `lan` section is flagged;
  - `network.wan.mtu` is not flagged.

## 4. Undo and put back

- [x] 4.1 Write the tests first against the fake backend, then implement revert-and-replay and verify:
  - undoing one of two dhcp changes leaves the other staged with its value and author time;
  - an added anonymous section that survives a replay keeps every later op on it;
  - a list-add and an order op replay in order;
  - a file-family config is discarded and replayed.
- [x] 4.2 Write the test first, then implement the recovery path and verify: a replay op that fails mid-way restores the original stage and answers with the danger notification.
- [x] 4.3 Write the tests first, then add the routes and verify:
  - `POST /pending/undo` (a row or a group) requires CSRF and redirects to `/pending` without scripting;
  - its notification carries a "Put it back" form;
  - `POST /pending/restore` restages the undone ops.

## 5. A rollback keeps the changes

- [x] 5.1 Write the tests first (`rollback_restage_test.go`), then implement in `watchApply` through the undo's replay of the stage read at apply time (not `stagePlan(forwardOf(change))`, which leaves a secret unset: config history keeps no secret's value) and verify:
  - after the rollback window, the apply's snapshot is staged again under the same sid;
  - the rollback time is recorded for the sid;
  - the re-stage runs without a history store;
  - an expired sid records the rollback and stages nothing.
- [x] 5.2 Amend ADR-010 in place, then verify the ADR's headings are intact and nothing cites a removed section. The amendments:
  - §3: the one surface is the rail row and the Pending changes page;
  - Discard gains per-change undo;
  - Apply re-stages on rollback;
  - effects read procd's own triggers, which keeps to the manifest-mapping rejection;
  - the Implementation notes name `/pending`, the undo routes and the retired `/uci/review`.

## 6. The Pending changes page

- [x] 6.1 Write the tests first, then add `handlePending` and the route and verify. The tests:
  - `GET /pending` renders "N pending changes" with no acts on the heading line and no lede, and Apply N and Discard all under the last card, 24px under it as cards stand;
  - When you apply comes first, with the disconnect callout (a packet, no buttons) when flagged, then the numbered steps, then the note;
  - a trigger read that fails (no ACL) leaves the card without a reload step and the page renders;
  - one card per group titled by its page heading, with Open page and (for more than one row) Undo these;
  - the rows carry their mark, verb, sentence, from → to, age and the three acts.
- [x] 6.2 Write the tests first, then implement and verify:
  - "1 pending change" / "Apply 1 change";
  - after an apply or a Discard all, the empty page offers "Back to <page>", the page of the most recent applied or discarded change, whatever the referrer (a bookmark, an undo's redirect, a rollback reload);
  - opened clean with nothing on record, it reads "Nothing pending" and offers no way back;
  - after a rollback, the callout names the rollback's time.
- [x] 6.3 Write the tests first, then implement and verify:
  - with scripting off, Undo, Undo these and Discard all are forms;
  - Apply is replaced by the sentence that it needs scripting;
  - `GET /uci/review` answers 308 to `/pending`;
  - the page composes existing widgets only (no new widget kind in the render).
- [x] 6.4 Delete the `verso-staged-review` and `verso-drawer-review` templates. Rewrite `TestReviewDrawer*` and `TestReviewPageForABrowserWithNoScript` to the page. Verify every test that named the drawer now names the page.
- [x] 6.5 Rewrite DESIGN.md's Staging and Drawers entries for the page, its cards and the When you apply card, with the reasons (one surface, the outcome before the act, no lede, card titles are page headings and carry no icon). Verify DESIGN.md's front matter is intact and the tests that read it pass.

## 7. The rail row and the phone

- [x] 7.1 Write the tests first in `staged_test.go`, then implement as the `verso-nav-pending` row in `staged.html.tmpl`, first in the rail's list, with `navModel.PendingActive`, and verify:
  - with a non-empty stage, the first rail row is Pending changes with the `list-todo` icon, the count and the "pending" variant, outside every group;
  - it carries the marker on `/pending`;
  - with a clean stage it renders hidden;
  - groups open and close exactly as the rail spec requires.
- [x] 7.2 Write the tests first, then implement and verify:
  - `page.html.tmpl` no longer renders `#verso-staged`;
  - the menu button carries the count span;
  - the `TestStagedChip*` tests are rewritten to the rail row and the menu button.
- [x] 7.3 Add the row's CSS (Marigold Wash, Deep Marigold words and icon, the plain marigold marker bar). Verify in a rendered preview, in Sand light and dark, that the row's words clear the 4.5 floor on the wash (the pair is gated for every shipped theme by `internal/theme`'s contrast floors).
- [x] 7.4 Record the rail's second status exception in DESIGN.md's Navigation (the row shows only while something waits, as the System updates dot does). Verify the sidebar direction entry and the Navigation section agree.

## 8. Apply, discard and undo in the browser

- [x] 8.1 Write the tests first in `scripts/`, then implement in `verso-commit.js` and verify:
  - Apply disables both acts and the save step reads Saving;
  - when the apply POST returns, the save step reads Saved, the reload step reads Started (and never a finish), and the check-in counts the rollback window down;
  - a confirm sets the check-in to Kept and reads the page again, which shows "Changes applied" and no rail row, with no notification repeating it;
  - a timeout sets the check-in to Put back in crimson and reads the page again once the router has staged the changes anew, which opens with the rollback callout.
- [x] 8.2 Write the tests first, then implement and verify:
  - Apply starts at once, with no confirmation, even with a flagged change;
  - Discard all is a secondary act that lets the stage go without asking, with or without a script;
  - an undo's notification offers "Put it back".
- [x] 8.3 Delete `versoStaged.fly`, the flight CSS, the chip verdict states and their callers in `verso-forms.js`, `verso-tables.js` and `verso-system.js`; the save's note keeps only where Save stood, as `verso-staged-landing`, and `verso-boot.js` goes with the count it held back. Verify with grep that none remain and that the reboot path still reads the stage.

## 9. Save puts the change in the row

- [x] 9.1 Write the tests first in `scripts/`, then implement `versoStaged.sync` in `verso-commit.js` and verify:
  - an act that stages in place sets the row's and the menu button's count, with nothing animated;
  - the first change brings the row and the last one gone takes it away;
  - a screen reader hears what the stage took;
  - an unsaved edit leaves the rail and the menu button as they are.
- [x] 9.2 Write the tests first, then implement and verify: a page form's save lands with its Save where it stood; a save that changed nothing leaves no note.
- [x] 9.3 Make the row a rail row like the rest: 40px, flush with Overview, no space of its own, sliding under the pointer as every rail row does but on its own page. Verify in the browser beside Overview.

## 10. Catalog, backlog and integration

- [x] 10.1 Add every new string to `i18n/sl/base.json` and retire the chip's and drawer's strings. Verify the i18n audit reports zero.
- [x] 10.2 Update `docs/BACKLOG.md`:
  - the change as landed;
  - the release-notes line (the chip is gone; pending changes live at the top of the sidebar).
  Verify the entries read as what is.
- [x] 10.3 End-to-end in the dev container (`make dev`), and verify each step in the browser as root:
  - edit a field with nothing pending and see the rail unchanged; save and see the row arrive reading 1;
  - undo one of two DHCP changes;
  - apply and see Saved, Started, Kept and "Changes applied";
  - force a rollback by blocking the confirm, and see the changes pending again with the callout.
- [ ] 10.4 On the DUT (never Zamanifi), verify that the reload step names exactly the services procd reloads for a dhcp + firewall + network apply.

## Workflow follow-up

- Archive the change once the owner has tried it on the dev container and the DUT checks in 10.4 are recorded.
