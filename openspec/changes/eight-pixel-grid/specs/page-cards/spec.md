# Spec Delta

## ADDED Requirements

### Requirement: Facts read as one list
The facts about one thing SHALL render as the same list wherever they stand (an Overview card, a page card, a drawer): 40px rows, a label and its value on one 24px line, the value on the right edge or beside a fixed label column. A list MAY carry a head over its rows, either a group's title with a chip or the names of its two columns, and MAY say in the empty set's slot that it has nothing to list.

#### Scenario: Maintenance's firmware
- **WHEN** an operator opens Maintenance
- **THEN** the firmware's parts stand as a list under the head "Part | Installed", each version beside its part
- **AND** the rows match the Overview's Internet card row for row

#### Scenario: A build on offer
- **WHEN** the update server offers a newer build
- **THEN** each part the build changes shows the new version under the installed one, led by an arrow
- **AND** a part the build leaves alone says it stays as it is

#### Scenario: An address family with no address
- **WHEN** the uplink has no IPv6 prefix
- **THEN** the Internet card's IPv6 head stands over the sentence "No prefix was delegated."

#### Scenario: A plugin's facts
- **WHEN** a plugin returns a fact list with a head and an empty sentence
- **THEN** the shell draws them as it draws its own

## RENAMED Requirements

- FROM: `### Requirement: A laned listing is one card per lane`
- TO: `### Requirement: A laned listing is one card with its lanes as bands`

## MODIFIED Requirements

### Requirement: Pages stand on Quiet Sand with no visible grid
Every page except the Overview and the live pages (Logs, Firewall › Activity, Diagnostics) SHALL stand on a Quiet Sand ground from below the top bar to the colophon, with no notebook grid drawn behind it.

#### Scenario: A settings page
- **WHEN** a signed-in operator opens Firewall settings
- **THEN** the page ground is Quiet Sand
- **AND** no grid lines are drawn on the page

#### Scenario: The grid shortcut in a dev session
- **WHEN** a developer presses G on a page in a dev session
- **THEN** only the 8px alignment overlay appears over the page
- **AND** the page ground stays plain Quiet Sand under it, with no notebook grid

### Requirement: The heading line stands on the sand without a masthead bar
A page's heading line (its h1 and page acts) SHALL stand directly on the sand with 24px above and below the 32px line, with no bar and no hairline under it. The first card starts directly under that space.

#### Scenario: Access opens
- **WHEN** an operator opens System › Access
- **THEN** "Access" stands 24px under the top bar on the sand with no hairline under the heading line
- **AND** the first card's top edge is 24px under the heading line

#### Scenario: Page acts
- **WHEN** a page has acts (Packages: Refresh index, Update packages, Install)
- **THEN** they stand on the heading line at its right end, the primary act last

### Requirement: Each ruled section of a page is a card
Every top-level section of a page that has a title SHALL render as a card: Paper, a single Strong Hairline frame with 2px corners, a header holding the section's glyph when it has one, its title and its lede, and its content behind a hairline. Cards stand 24px apart in one column at the page's measure, and the blocks inside a card stand 24px apart; a titled part inside a card starts 40px on.

#### Scenario: Firewall settings
- **WHEN** an operator opens Firewall settings
- **THEN** "When no zone applies", "Protection", "Speed", "TCP and helpers" and "Custom rules" each stand in their own card, in that order, 24px apart

#### Scenario: A section's lede
- **WHEN** a section carries a lede ("Restoring replaces the saved files and reboots the router.")
- **THEN** the lede stands in the card's header on the line under the title, at weight 400
- **AND** the header's hairline stands under the lede

#### Scenario: Blocks in a card
- **WHEN** an operator opens Maintenance
- **THEN** the Firmware card's ledger, its upload act, the packages notice and the automatic-check switch stand 24px apart

#### Scenario: A part inside a section
- **WHEN** a section holds a part (SSH's "Authorized keys")
- **THEN** the part stands inside the section's card on its ledger line, not as a card of its own

### Requirement: A page form's untitled fields stand in an untitled card
The fields a page form opens with outside any titled section SHALL stand in a card of the same frame with no header, so no field stands loose on the sand.

#### Scenario: System › General
- **WHEN** an operator opens System › General
- **THEN** Router name, Private IPv6 prefix and "Spread packet handling over all cores" stand in one card with no title, 24px over the Time card

#### Scenario: Network › DNS
- **WHEN** an operator opens Network › DNS
- **THEN** Upstream servers and "Use the servers the ISP hands out" stand in one card with no title, above the Local names card

### Requirement: A laned listing is one card with its lanes as bands
A page-level listing split into lanes SHALL render as one card. Each lane opens on a Quiet Sand band among the rows, holding the lane's label and its detail, with no count. The listing's column heads stand once, at the top of the card.

#### Scenario: Devices by network
- **WHEN** an operator opens Devices with devices on "Local network", "mgmt" and "Elsewhere"
- **THEN** one card holds every device, under one row of column heads
- **AND** each network opens on a Quiet Sand band naming it, with the subnet beside the name and no count
- **AND** each network's devices stand under its band

### Requirement: The control band is the top strip of its listing's card
The controls that narrow a listing (search, counted cuts) SHALL stand as the first strip inside that listing's card, on the card's Paper, behind a hairline: 16px over the 32px controls and 16px under them, the hairline included. No Mid Sand box is drawn on the page.

#### Scenario: Packages' search and cut
- **WHEN** an operator opens System › Packages
- **THEN** "Find a package" and "Installed · N" stand inside the card's top strip
- **AND** the column-head row stands directly under the strip's hairline

#### Scenario: A titled listing with controls
- **WHEN** a listing with controls is in a titled card
- **THEN** the control strip stands directly under the card's header

### Requirement: Cards keep their content usable on a phone
On a phone a card SHALL take the column's full width: a wide table scrolls sideways inside its card, a card header drops secondary counts before its title truncates, and a control strip stacks its controls.

#### Scenario: Devices on a phone
- **WHEN** an operator opens Devices at 390px wide
- **THEN** the devices table scrolls sideways inside its card
- **AND** the page itself does not scroll sideways
- **AND** the page keeps a 16px gutter at each side
