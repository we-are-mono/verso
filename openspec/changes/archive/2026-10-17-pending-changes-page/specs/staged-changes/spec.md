# Spec Delta

## Purpose

How changes saved on the router but not yet live are shown, reviewed, discarded, applied and kept: the rail's Pending changes row, the Pending changes page, and what an apply and a rollback leave behind.

## ADDED Requirements

### Requirement: The Pending changes row shows the stage
While the stage holds at least one change, the rail's Pending changes row SHALL show the count of pending changes and lead to the Pending changes page. The count SHALL be the number of rows the Pending changes page lists. While the stage is empty the row SHALL be absent.

#### Scenario: Twelve changes waiting
- **WHEN** the stage holds changes the Pending changes page lists as twelve rows
- **THEN** the rail's first row reads "Pending changes" with the count 12, and pressing it opens the Pending changes page

#### Scenario: Nothing waiting
- **WHEN** the stage is empty
- **THEN** the rail has no Pending changes row

### Requirement: Save puts the change in the row
An edit that is not saved SHALL NOT change the Pending changes row or the menu button. Once Save stages a change, the row SHALL show the new count: a page form's Save reloads the page, which draws the row; an act that stages in place (a drawer's Save, a row's one-press act) SHALL set the count where it stands. With nothing pending before, the row SHALL appear only then. Nothing about the row SHALL animate, and no change SHALL be shown travelling across the screen.

#### Scenario: First edit on a clean router
- **WHEN** nothing is pending and the operator changes a field without saving
- **THEN** the rail starts with Overview, as before the edit

#### Scenario: Saving it
- **WHEN** the operator then saves the form
- **THEN** the page reads again with the Pending changes row first in the rail, reading 1

#### Scenario: A one-press act
- **WHEN** the operator turns Allow-Ping off from its row while 12 changes are pending
- **THEN** the row reads 13, with nothing shown before the press

### Requirement: The Pending changes page lists the stage by page
The Pending changes page SHALL head "N pending changes" with no acts on the heading line and no lede under the heading. It SHALL list the pending changes in one card per page they were made on, titled by that page's heading, each card offering to open that page and to discard its changes. **Apply N changes** and **Discard all** SHALL stand under the last card, where a page form's Save stands.

#### Scenario: Changes from three pages
- **WHEN** changes were saved on Firewall rules, DHCP and General
- **THEN** the page shows a "Firewall rules" card, a "DHCP" card and a "General" card, each with "Open page" and "Discard these"

#### Scenario: One change
- **WHEN** a single change is pending
- **THEN** the heading reads "1 pending change" and the primary act reads "Apply 1 change"

### Requirement: A pending change row says what it does
Each row SHALL show the change's kind (added, changed, removed, reordered) as its mark and word, the owning page's sentence for it, and its old and new value where it is a setting. It SHALL NOT say how long ago it was staged: a pending change is minutes old, and its card names the page it came from. Each row SHALL offer to go to it on its page, to discard it, and to show the config lines it stands for.

#### Scenario: A setting that was not set
- **WHEN** Cache size was not set and 151 is pending
- **THEN** its row reads "Changed · Cache size `cachesize` not set → 151"

#### Scenario: Going to it
- **WHEN** the operator presses a row's go-to act
- **THEN** its page opens with that setting or item marked as the one the row named

### Requirement: Changes can be discarded one at a time
Letting changes go SHALL be named Discard at every reach: "Discard this change" on a row, "Discard these" on a card, "Discard all" under the cards. Discarding a row SHALL drop that change from the stage and leave every other pending change as it was. Discarding a card's changes SHALL drop every change on that card and no other. After a row's or a card's discard, the top-right notification SHALL say what was discarded and offer to put it back.

#### Scenario: Discarding one of two changes in one config
- **WHEN** Lease time and Cache size are pending in the same config and the operator discards Lease time
- **THEN** Cache size is still pending with its value, and Lease time is no longer pending

#### Scenario: Putting it back
- **WHEN** the operator presses "Put it back" on the discard's notification
- **THEN** Lease time is pending again with the value it had

#### Scenario: Discarding a card
- **WHEN** the operator presses "Discard these" on the DHCP card
- **THEN** every DHCP change leaves the stage and no other card changes

### Requirement: The page says what applying will do
A **When you apply** card SHALL list, numbered and one line each, the steps an apply takes, in the order it takes them: it saves the N changes; it reloads the services that use them; it keeps them if this page checks in within the rollback window. The services SHALL be read from the router's own reload triggers, named in one step in no promised order. The card SHALL hold the optional note for config history.

#### Scenario: DHCP and firewall changes
- **WHEN** 3 changes to the dhcp and firewall configs are pending
- **THEN** the card reads 1 "Saves 3 changes.", 2 "Reloads DNS & DHCP and Firewall.", 3 "Keeps them if this page checks in within 30 s."

#### Scenario: A config no service reloads for
- **WHEN** the only pending config has no reload trigger on the router
- **THEN** the card has no reload step and numbers the check-in 2

### Requirement: A change that may disconnect this browser is named first
When a pending change touches the network interface that holds the address this browser reaches the router through, the **When you apply** card SHALL open with a marigold callout saying the apply may disconnect this browser and naming the address it moves from and to. The callout SHALL lead with a packet, not an icon, and SHALL hold no buttons.

#### Scenario: Moving the LAN address
- **WHEN** the browser reaches the router at 192.168.1.1 on LAN and the LAN address 10.0.0.1 is pending
- **THEN** the card opens with "May disconnect you" naming 192.168.1.1 and 10.0.0.1

#### Scenario: A change on another interface
- **WHEN** only a WAN MTU change is pending
- **THEN** no disconnect callout shows

### Requirement: Apply never asks
Pressing Apply SHALL start the apply at once, with no confirmation, whether or not a change may disconnect this browser; the disconnect callout already standing on the page is the warning.

#### Scenario: Applying a LAN address move
- **WHEN** the LAN address change is pending, its callout shows, and the operator presses Apply
- **THEN** the apply starts with no question asked

### Requirement: The apply runs on the page
Pressing Apply SHALL disable both acts and show each step's state only as far as the router reports it. The save step SHALL read Saving, then Saved when the router has saved the changes. The reload step SHALL then read Started, never done, because the router reports no finish. The check-in step SHALL count down the rollback window and read Kept when the router keeps the changes.

#### Scenario: A clean apply
- **WHEN** the operator applies 12 changes and this page checks in
- **THEN** step 1 reads Saved, step 2 reads Started, and step 3 counts down and then reads Kept

#### Scenario: Waiting on the check-in
- **WHEN** the router has saved the changes and this page has not checked in yet
- **THEN** step 3 shows the seconds left in the rollback window

### Requirement: A kept apply empties the page
When the router keeps an apply, the page SHALL read "Changes applied", saying how many changes the router kept, when, and that what it reloaded can take a few more seconds to settle. The rail's row SHALL leave. No notification SHALL repeat it, because the page already says it.

#### Scenario: Twelve kept
- **WHEN** the router keeps an apply of 12 changes at 15:42
- **THEN** the page reads "Changes applied" and "The router kept 12 changes at 15:42", the Pending changes row leaves the rail, and no notification appears

### Requirement: The way back leads to the page of the latest change
When the page has nothing pending after an apply or a Discard all, it SHALL offer "Back to <page>", naming the page the most recent of those changes was made on, however the operator reached the Pending changes page. With no such change on record, it SHALL offer no way back.

#### Scenario: After applying a rule edit
- **WHEN** the operator edits a Firewall rule, visits Devices, opens Pending changes from a bookmark and applies
- **THEN** the page offers "Back to Firewall rules"

#### Scenario: Opened clean
- **WHEN** the operator opens the Pending changes page with nothing pending and no apply or discard on record
- **THEN** the page reads "Nothing pending" and offers no way back

### Requirement: A rolled-back apply keeps the changes pending
When the router puts its previous settings back because this page did not check in within the rollback window, the changes that apply carried SHALL be pending again, as they were before Apply. The page SHALL open with a callout saying the last apply was put back, when, and that this page could not check in. The top-right notification SHALL say the changes are still pending.

#### Scenario: The router goes silent
- **WHEN** an apply of 12 changes is not confirmed within the rollback window
- **THEN** the router runs its previous settings, the 12 changes are pending again, and the Pending changes page explains the apply was put back

### Requirement: Discard all is a secondary act
Discard all SHALL be drawn as a secondary act, not in the danger colour, because nothing was applied and the router runs as it is either way. Pressing it SHALL empty the stage without asking first; the page SHALL read "Nothing pending" and the rail's row SHALL leave.

#### Scenario: Discarding
- **WHEN** the operator presses Discard all
- **THEN** no change is pending and the router keeps running what it ran

### Requirement: The page works without scripting
With scripting unavailable, the Pending changes page SHALL list every pending change and SHALL discard them, one, a card's or all. It SHALL say that applying needs scripting, because the router keeps an apply only when the page answers it.

#### Scenario: No script
- **WHEN** the operator opens the Pending changes page with scripting off
- **THEN** the changes are listed, every Discard works, and Apply says it needs scripting

### Requirement: The old review address leads to the page
A request for the review drawer's former address SHALL be redirected to the Pending changes page.

#### Scenario: An old bookmark
- **WHEN** the browser requests `/uci/review`
- **THEN** it is redirected to `/pending`
