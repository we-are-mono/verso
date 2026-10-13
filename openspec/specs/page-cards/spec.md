# page-cards Specification

## Purpose

How every page outside the Overview lays out: its subjects stand in Paper cards on a Quiet Sand ground, so each page reads as the same app as the Overview it opens from.

## Requirements

### Requirement: Pages stand on Quiet Sand with no visible grid
Every page except the Overview and the live pages (Logs, Firewall › Activity, Diagnostics) SHALL stand on a Quiet Sand ground from below the top bar to the colophon, with no notebook grid drawn behind it.

#### Scenario: A settings page
- **WHEN** a signed-in operator opens Firewall settings
- **THEN** the page ground is Quiet Sand
- **AND** no grid lines are drawn on the page

#### Scenario: The grid shortcut in a dev session
- **WHEN** a developer presses G on a page in a dev session
- **THEN** no grid appears

### Requirement: The heading line stands on the sand without a masthead bar
A page's heading line (its h1 and page acts) SHALL stand directly on the sand with 26px above and below the 34px line, with no bar and no hairline under it. The first card starts directly under that space.

#### Scenario: Access opens
- **WHEN** an operator opens System › Access
- **THEN** "Access" stands 26px under the top bar on the sand with no hairline under the heading line
- **AND** the first card's top edge is 26px under the heading line

#### Scenario: Page acts
- **WHEN** a page has acts (Packages: Refresh index, Update packages, Install)
- **THEN** they stand on the heading line at its right end, the primary act last

### Requirement: Each ruled section of a page is a card
Every top-level section of a page that has a title SHALL render as a card: Paper, a single Strong Hairline frame with 2px corners, a header holding the section's glyph when it has one and its title, and its content behind a hairline. Cards stand 20px apart in one column at the page's measure.

#### Scenario: Firewall settings
- **WHEN** an operator opens Firewall settings
- **THEN** "When no zone applies", "Protection", "Speed", "TCP and helpers" and "Custom rules" each stand in their own card, in that order, 20px apart

#### Scenario: A section's lede
- **WHEN** a section carries a lede ("Every file in /etc/nftables.d/ loads with the ruleset…")
- **THEN** the lede stands at the top of the card's body, a cell over the content under it

#### Scenario: A part inside a section
- **WHEN** a section holds a part (SSH's "Authorized keys")
- **THEN** the part stands inside the section's card on its ledger line, not as a card of its own

### Requirement: A page form's untitled fields stand in an untitled card
The fields a page form opens with outside any titled section SHALL stand in a card of the same frame with no header, so no field stands loose on the sand.

#### Scenario: System › General
- **WHEN** an operator opens System › General
- **THEN** Router name, Private IPv6 prefix and "Spread packet handling over all cores" stand in one card with no title, 20px over the Time card

#### Scenario: Network › DNS
- **WHEN** an operator opens Network › DNS
- **THEN** Upstream servers and "Use the servers the ISP hands out" stand in one card with no title, above the Local names card

### Requirement: A plugin page becomes cards without a plugin change
The card layout SHALL come from the shell's rendering of the existing widget schema, so a plugin page renders as cards without the plugin being rebuilt and without any new schema field.

#### Scenario: An installed plugin built against an older shell
- **WHEN** the shell runs with a plugin built before page cards existed
- **THEN** the plugin's titled sections render as cards

### Requirement: A laned listing is one card per lane
A page-level listing split into lanes SHALL render each lane as its own card: the lane's label, detail and tally become the card's header, and the card holds that lane's rows under its own column heads. Every card of one listing lines its columns up with the others.

#### Scenario: Devices by network
- **WHEN** an operator opens Devices with devices on "Local network", "mgmt" and "Elsewhere"
- **THEN** each network is a card titled with its name, the subnet beside it and "N online · N offline" at its end
- **AND** each card repeats the column heads and holds only that network's devices
- **AND** the Address column starts at the same horizontal position in every card

### Requirement: A page that is one listing is one untitled card
A page whose body is a single listing SHALL render it as one card with no title of its own, because the page heading already names it.

#### Scenario: Packages
- **WHEN** an operator opens System › Packages
- **THEN** the package table stands in one card with no header title

### Requirement: The control band is the top strip of its listing's card
The controls that narrow a listing (search, counted cuts) SHALL stand as the first strip inside that listing's card, on the card's Paper, behind a hairline: 13px over the 34px controls and 12px under them. No Mid Sand box is drawn on the page.

#### Scenario: Packages' search and cut
- **WHEN** an operator opens System › Packages
- **THEN** "Find a package" and "Installed · N" stand inside the card's top strip
- **AND** the column heads stand a cell under the strip's hairline

#### Scenario: A titled listing with controls
- **WHEN** a listing with controls is in a titled card
- **THEN** the control strip stands directly under the card's header

### Requirement: A form's Save stands with what it commits
A form that commits one section SHALL show its Save at the foot of that section's card. A form that commits the whole page SHALL show its Save on the sand under the last card.

#### Scenario: Access's per-section forms
- **WHEN** an operator opens System › Access
- **THEN** "Change password", "Save SSH settings" and the web interface's Save each stand at the foot of their own card

#### Scenario: A page-wide form
- **WHEN** an operator opens Firewall settings
- **THEN** "Save settings" stands on the sand under the last card, outside every card

### Requirement: A form's configuration preview is a card of its own
A page form's configuration preview SHALL stand as its own card directly above the form's Save, with the file's path as the card's title and the copy act in its header.

#### Scenario: Firewall settings' preview
- **WHEN** an operator opens Firewall settings
- **THEN** a card titled `/etc/config/firewall` stands after "Custom rules", holding the numbered file
- **AND** "Save settings" stands under it on the sand

### Requirement: Page cards do not collapse or move
A card on any page other than the Overview SHALL stay open and in place: its header carries no chevron and no grip, and clicking the header does nothing.

#### Scenario: Clicking a section card's header
- **WHEN** an operator clicks the header of the "Protection" card
- **THEN** the card stays open

### Requirement: A framed component drops its frame inside a card
A component that draws its own frame SHALL drop that frame and its fill when it stands inside a page card, so no card holds a framed box. Its title and sentence stay, unframed, at the top of the card's body.

#### Scenario: The HTTPS certificate on Access
- **WHEN** an operator opens System › Access
- **THEN** the certificate's facts stand directly in the "Certificate" card with no inner frame or fill
- **AND** "HTTPS certificate" and "What this router shows browsers that connect over HTTPS." stand above the facts

### Requirement: A page's warning stands in the card it concerns
A page callout about one section SHALL stand at the top of that section's card body.

#### Scenario: The web interface is reachable from the internet
- **WHEN** a firewall zone accepts every connection from the internet to the web interface
- **THEN** "The internet can reach this web interface" stands at the top of the "Web interface" card

### Requirement: Live pages stay as they are
The pages with the light masthead (System › Logs, Firewall › Activity and System › Diagnostics) SHALL keep their own layout: the light masthead on Paper with its hairline, and their output flush under it, with no card.

#### Scenario: System log
- **WHEN** an operator opens System › Logs
- **THEN** the light masthead on Paper ends on its hairline
- **AND** the log stands flush under it, not inside a card

#### Scenario: Firewall activity
- **WHEN** an operator opens Firewall › Activity
- **THEN** the activity stands flush under the light masthead, not inside a card

#### Scenario: Diagnostics
- **WHEN** an operator opens System › Diagnostics
- **THEN** the tool row and its output stand under the light masthead, not inside a card

### Requirement: Drawers keep their own layout
A drawer SHALL keep its layout (Paper, 56px header, heading bands, configuration card), and sections inside a drawer SHALL NOT render as cards.

#### Scenario: Editing a firewall rule
- **WHEN** an operator opens a rule's drawer from the Rules page
- **THEN** its sections stand under heading bands inside the drawer, not as cards

### Requirement: Cards keep their content usable on a phone
On a phone a card SHALL take the column's full width: a wide table scrolls sideways inside its card, a card header drops secondary counts before its title truncates, and a control strip stacks its controls.

#### Scenario: Devices on a phone
- **WHEN** an operator opens Devices at 390px wide
- **THEN** each network card's table scrolls sideways inside the card
- **AND** the page itself does not scroll sideways
