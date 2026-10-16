# Spec Delta

## Purpose

How changes saved on the router but not yet live are shown, previewed, reviewed, undone, applied and kept: the rail's Pending changes row, its in-place preview of an unsaved edit, the Pending changes page, and what an apply and a rollback leave behind.

## ADDED Requirements

### Requirement: The Pending changes row shows the stage
While the stage holds at least one change, the rail's Pending changes row SHALL show the count of pending changes and lead to the Pending changes page. The count SHALL be the number of rows the Pending changes page lists. While the stage is empty the row SHALL be absent.

#### Scenario: Twelve changes waiting
- **WHEN** the stage holds changes the Pending changes page lists as twelve rows
- **THEN** the rail's first row reads "Pending changes" with the count 12, and pressing it opens the Pending changes page

#### Scenario: Nothing waiting
- **WHEN** the stage is empty
- **THEN** the rail has no Pending changes row

### Requirement: An unsaved edit shows in the row before Save
While a form or drawer holds an edit that is not saved, the Pending changes row SHALL show a dashed line naming what Save would stage. The line SHALL lead with "+1" when the edited item or setting is not already pending. Hovering or focusing a control SHALL NOT change the row.

#### Scenario: Editing a rule
- **WHEN** the operator opens Allow-DHCPv6's drawer and changes its Action to DROP without saving
- **THEN** the Pending changes row shows a dashed line "+1 Allow-DHCPv6 · Action → DROP" and the count is unchanged

#### Scenario: Editing something already pending
- **WHEN** Lease time is already pending and the operator changes it again without saving
- **THEN** the dashed line names Lease time and its new value without "+1"

#### Scenario: Pointing at Save
- **WHEN** the operator moves the pointer over Save
- **THEN** the row does not change

### Requirement: With nothing pending, the first edit shows the row as a ghost
While the stage is empty and a form or drawer holds an unsaved edit, the Pending changes row SHALL stand in the rail drawn as a dashed outline, without a count, holding the dashed line for that edit.

#### Scenario: First edit on a clean router
- **WHEN** nothing is pending and the operator changes a field without saving
- **THEN** a dashed Pending changes row appears at the top of the rail with the edit's dashed line under its name

#### Scenario: Leaving the edit
- **WHEN** the operator cancels that drawer or puts the field back
- **THEN** the dashed row leaves the rail

### Requirement: Save settles the line in place
Saving an edit SHALL turn its dashed line, in the same place, into the plain words the Pending changes page uses for that change; the count SHALL change to the new count, and the line SHALL fold away after about three seconds. The change SHALL NOT be shown travelling across the screen. A press that stages at once, with no form to edit (a rule's on/off or remove act), SHALL show its line in plain words the same way.

#### Scenario: Saving the rule
- **WHEN** the operator saves Allow-DHCPv6 with Action DROP while 12 changes are pending
- **THEN** the line reads the Pending changes page's sentence for that change in plain words, the count reads 13, and the line folds away about three seconds later

#### Scenario: A one-press act
- **WHEN** the operator turns Allow-Ping off from its row
- **THEN** the row shows "Allow-Ping turned off" in plain words and the count rises by one, with nothing shown before the press

#### Scenario: Cancelling
- **WHEN** the operator cancels a drawer that held an unsaved edit
- **THEN** the dashed line leaves and the count does not change

### Requirement: The Pending changes page lists the stage by page
The Pending changes page SHALL head "N pending changes" with **Discard all** and **Apply N changes** on the heading line and no lede under the heading. It SHALL list the pending changes in one card per page they were made on, titled by that page's heading, each card offering to open that page and to undo its changes.

#### Scenario: Changes from three pages
- **WHEN** changes were saved on Firewall rules, DHCP and General
- **THEN** the page shows a "Firewall rules" card, a "DHCP" card and a "General" card, each with "Open page" and "Undo these"

#### Scenario: One change
- **WHEN** a single change is pending
- **THEN** the heading reads "1 pending change" and the primary act reads "Apply 1 change"

### Requirement: A pending change row says what it does
Each row SHALL show the change's kind (added, changed, removed, reordered) as its mark and word, the owning page's sentence for it, its old and new value where it is a setting, and how long ago it was staged when that is known. Each row SHALL offer to go to it on its page, to undo it, and to show the config lines it stands for.

#### Scenario: A setting that was not set
- **WHEN** Cache size was not set and 151 is pending
- **THEN** its row reads "Changed · Cache size `cachesize` not set → 151" with how long ago it was staged

#### Scenario: Going to it
- **WHEN** the operator presses a row's go-to act
- **THEN** its page opens with that setting or item marked as the one the row named

#### Scenario: A change staged before the router's UI last started
- **WHEN** no staging time is on record for a change
- **THEN** its row shows no age

### Requirement: Changes can be undone one at a time
Undoing a row SHALL drop that change from the stage and leave every other pending change as it was. Undoing a card's changes SHALL drop every change on that card and no other. After an undo, the top-right notification SHALL offer to put it back.

#### Scenario: Undoing one of two changes in one config
- **WHEN** Lease time and Cache size are pending in the same config and the operator undoes Lease time
- **THEN** Cache size is still pending with its value, and Lease time is no longer pending

#### Scenario: Putting it back
- **WHEN** the operator presses "Put it back" on the undo's notification
- **THEN** Lease time is pending again with the value it had

#### Scenario: Undoing a card
- **WHEN** the operator presses "Undo these" on the DHCP card
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
When the router keeps an apply, the page SHALL read "Changes applied", saying how many changes the router kept, when, and that what it reloaded can take a few more seconds to settle. The rail's row SHALL leave, and the top-right notification SHALL say how many changes were applied.

#### Scenario: Twelve kept
- **WHEN** the router keeps an apply of 12 changes at 15:42
- **THEN** the page reads "Changes applied" and "The router kept 12 changes at 15:42", the Pending changes row leaves the rail, and a notification says "12 changes applied."

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

### Requirement: Discard all asks first
Pressing Discard all SHALL ask in place, in the danger colour, before dropping every pending change. Answering yes SHALL empty the stage; the page SHALL read "Nothing pending" and the rail's row SHALL leave.

#### Scenario: Discarding
- **WHEN** the operator presses Discard all and answers "Discard all"
- **THEN** no change is pending and the router keeps running what it ran

### Requirement: The page works without scripting
With scripting unavailable, the Pending changes page SHALL list every pending change and SHALL undo and discard them. It SHALL say that applying needs scripting, because the router keeps an apply only when the page answers it.

#### Scenario: No script
- **WHEN** the operator opens the Pending changes page with scripting off
- **THEN** the changes are listed, Undo and Discard all work, and Apply says it needs scripting

### Requirement: The old review address leads to the page
A request for the review drawer's former address SHALL be redirected to the Pending changes page.

#### Scenario: An old bookmark
- **WHEN** the browser requests `/uci/review`
- **THEN** it is redirected to `/pending`
