# form-fields Specification

## Purpose

How a form's fields are laid out and what each field row shows: its question, its description, its answer, its note, its refusal and its staged mark, as one ledger on every page, drawer and dialog, with descriptions an operator can fold away.

## Requirements

### Requirement: A field is a ledger row
Every field of a form that commits settings SHALL render as one row: on the left its question, the field's name with its option chip and staged mark on one line and its description under it; on the right its answer, the control, in one 272px answer column. Every row of a form SHALL put its answer column at the same place, so the answers read down one column.

#### Scenario: A policy on Firewall settings
- **WHEN** an operator opens Firewall settings
- **THEN** "Traffic to this router" and its `input` chip stand at the left of their row with "Traffic addressed to the router itself — the web interface, SSH, its DNS resolver. Not traffic it merely routes." under them
- **AND** the row's accept, reject and drop choices stand in the answer column on the right

#### Scenario: A field in a drawer
- **WHEN** an operator opens a zone's drawer
- **THEN** the zone's name field stands in the answer column of its row, at the same place as every other answer in the drawer

### Requirement: Rows are separated by hairlines across their surface
Field rows SHALL be separated by hairlines that run the full width of what holds them: a card's from its left border to its right, a drawer's from its content's left edge to its right. A row's words SHALL keep the surface's 24px gutter. The first row of a run of rows, under a card's header or after another block, SHALL draw no hairline of its own, and rows SHALL carry no fill.

#### Scenario: Rows in a card
- **WHEN** an operator opens the Protection card on Firewall settings
- **THEN** a hairline between "Slow down SYN floods" and "Connection rate" touches the card's left and right borders
- **AND** both names start 24px from the card's left border

#### Scenario: A card's first and last rows
- **WHEN** an operator opens the Protection card on Firewall settings
- **THEN** "Slow down SYN floods" stands 15px after the header's seam, as a name stands after the hairline between rows
- **AND** the card's frame closes 16px under its last row's words

#### Scenario: Pointing at a row
- **WHEN** an operator moves the pointer over "Connection rate"
- **THEN** the row keeps the card's Paper, with no tint

#### Scenario: A listing and fields in one card
- **WHEN** an operator opens the Custom rules card
- **THEN** the hairlines between its questions touch the card's borders
- **AND** the rule-file table's own lines stop 24px short of them

### Requirement: A table among fields is a row of the ledger
A table standing among a form's fields SHALL render as one row: its label as the question, its act at the row's right end on the same line, and its rows under them as a wide answer at the row's width, with a hairline under its head and between its rows. The table's lines SHALL stay inside the 24px gutters, so only a line between questions touches a card's borders, and the table SHALL carry no band fill.

#### Scenario: Custom rules' files
- **WHEN** an operator opens the Custom rules card with two rule files
- **THEN** "Files" stands as a question with "Add file" at the row's right end on the same line, ending where the files' edit glyphs end
- **AND** the two files stand under it, a hairline under the head and between them, each stopping 24px short of the card's borders

#### Scenario: A table among fields on a phone
- **WHEN** an operator opens the Custom rules card at 390px wide
- **THEN** "Add file" stays on the "Files" line at its right end

#### Scenario: A table outside a form
- **WHEN** an operator opens Devices
- **THEN** the devices table keeps its column heads and its lines run its card's width

### Requirement: A select fills the answer column
A dropdown in a ledger row SHALL be as wide as the answer column, whatever its longest option.

#### Scenario: Two reject codes
- **WHEN** an operator opens Firewall settings
- **THEN** "Connection refused (TCP reset)" and "Nothing is listening" stand in dropdowns of the same width, the answer column's

### Requirement: A short field keeps its own measure
A field whose value has a measure of its own (a number, a port, a count with its unit, a prefix) SHALL keep that measure in the answer column, starting at the column's left edge. A field without a measure of its own SHALL fill the answer column.

#### Scenario: A burst in packets
- **WHEN** an operator opens the Protection card
- **THEN** the Burst field and its "packets" unit stand at their own measure at the answer column's left edge

### Requirement: Radio choices run across their row
A field offered as radio choices SHALL lay its choices across one line in its answer, in order, and SHALL wrap a choice that does not fit onto the next 32px line.

#### Scenario: Congestion notification
- **WHEN** an operator opens the TCP and helpers card
- **THEN** Off, On and Incoming only stand side by side on one line in the answer column

### Requirement: A switch states its state in a word
A switch in a ledger row SHALL be its box in the answer column with the word "On" or "Off" beside it, the word following the box as it is checked or cleared. The field's name, the box and the word SHALL each flip the switch.

#### Scenario: Flipping a switch
- **WHEN** an operator clicks "Off" beside "Drop packets that belong to no connection"
- **THEN** the box is checked and the word beside it reads "On"

### Requirement: A wide answer stands under its question
An answer that cannot stand in the answer column SHALL stand under its question at the row's full width: a set of value chips, rows of values, a code or text area, a weekday strip, a field of the endpoint measure, and a fused or joined value whose parts together are wider than the column. The question and its description stay on their own lines above it.

#### Scenario: A rule's protocols
- **WHEN** an operator opens a rule's drawer on its Match tab
- **THEN** the protocol chips stand under "Protocol" across the row's full width

#### Scenario: An address with its netmask
- **WHEN** an operator opens a static interface's drawer
- **THEN** the IPv4 address and its netmask stand as one fused box under "IPv4 address" at the row's full width

#### Scenario: A joined value that fits
- **WHEN** an operator opens the Protection card
- **THEN** "Connection rate" keeps its count, "per" and its period together in the answer column

### Requirement: A narrow row stacks
A field row narrower than 576px SHALL stack: the question, its description, then the answer straight under them at the row's width, with no hairline between them. A stacked switch stays its box and its word.

#### Scenario: Firewall settings on a phone
- **WHEN** an operator opens Firewall settings at 390px wide
- **THEN** "Traffic to this router" stands over its description, and the accept, reject and drop choices stand under the description

#### Scenario: A dialog
- **WHEN** an operator opens the firmware upload dialog
- **THEN** its password field stands under its name, not beside it

### Requirement: A revealed field stands under the switch that reveals it
A field a switch reveals SHALL stand under that switch's row with its question indented 24px and no hairline between them, its answer in the same answer column as every other row. A field revealed inside a revealed field SHALL indent 24px more. The hairline after the group SHALL follow its last revealed row.

#### Scenario: Hardware offload
- **WHEN** an operator checks "Shortcut established connections"
- **THEN** "Do it in hardware" appears under it, indented 24px, with no hairline between the two rows
- **AND** its box stands in the answer column under the first row's box

### Requirement: A group of switches stands under its group's name
A labelled group of switches (one setting asked of several things) SHALL stand as its group's name with its chip and description, then one row per item with no hairline between items, each item's name verbatim where the group says so, its detail under it, and its box and word in the answer column.

#### Scenario: Files the config includes
- **WHEN** an operator opens the Custom rules card with an include section for `/usr/libexec/verso/firewall-logging`
- **THEN** "Files the config includes" and its `enabled` chip stand on their own line
- **AND** `/usr/libexec/verso/firewall-logging` stands under it with "Shell script · runs after the ruleset loads" under the path, and its box and "On" in the answer column

### Requirement: Related fields stand as consecutive rows
A labelled set of related fields that cannot fuse into one box (one holding a dropdown that is not a unit, a list or a switch) SHALL stand as the set's name and description, then each field as its own ledger row with no hairline between them, never as fields side by side in columns.

#### Scenario: A set with a dropdown
- **WHEN** a plugin sends a labelled set of a dropdown and two typed fields
- **THEN** each of the three stands on its own row under the set's name, with no hairline between them, its answer in the answer column

### Requirement: An unnamed set joins the form's rows
A set of related fields with neither a name nor a description that cannot fuse SHALL stand as its fields' rows in the form's own run, each under a hairline as any row is. A set holding something that is not a setting (words, a table) SHALL keep its columns.

#### Scenario: An unlabelled set
- **WHEN** a plugin sends an unlabelled set of a typed field and a list
- **THEN** each stands as a row of the form, a hairline between them, with no head over them

### Requirement: A fused value names its parts inside the box
Each typed part of a labelled set fused into one box SHALL show its name inside its own trailing edge, as a unit stands after its value, since the row's name is the set's; a part with a unit SHALL show its unit instead. The field's description SHALL be the set's help followed by each part's help led by the part's name.

#### Scenario: A record's port and ranking
- **WHEN** an operator opens a DNS service record with port 5060, priority 10 and weight 5
- **THEN** "Port and ranking" and the chip `port · priority · weight` stand in the question
- **AND** one box reads 5060 "Port", 10 "Priority" and 5 "Weight", its parts split by inner hairlines
- **AND** the description reads "Port: SRV only. Priority: SRV and MX. Lower wins. Weight: SRV only."

### Requirement: A fused box stands where its parts fit
A fused box SHALL stand in the answer column when its parts fit it, and under its question at the row's full width when they do not. A part's name and a unit inside a box are words, set in the sans face.

#### Scenario: Three numbers and their names
- **WHEN** an operator opens a DNS service record's port and ranking
- **THEN** the box of three numbers with their names, wider than the answer column, stands under the description at the row's full width

#### Scenario: A unit
- **WHEN** an operator opens the Protection card
- **THEN** "packets" after the Burst value is set in the sans face

### Requirement: A value and its unit fuse into one box
An unlabelled set of a value and a dropdown of the units it is counted in SHALL stand as one row: the value's name as the question, a chip naming both options joined by a middot, and one fused box in the answer holding the value and the unit dropdown, split by an inner hairline.

#### Scenario: How often DDNS checks
- **WHEN** an operator opens a dynamic DNS hostname's Timing
- **THEN** "Check every" and the chip `check_interval · check_unit` stand in the question
- **AND** "10" and "minutes" stand in one box in the answer column, "minutes" a dropdown of seconds, minutes, hours and days

### Requirement: A field's help is its description
A field's help, and after it the field's tip when it has one, SHALL write as the field's description under its name, at the question's width. The field's name SHALL carry no tooltip. A field that sets both a help and a tip SHALL show both.

#### Scenario: Help written on the page
- **WHEN** an operator opens Firewall settings
- **THEN** "Rate-limits half-open TCP connections so a flood cannot exhaust the connection table." stands under "Slow down SYN floods"
- **AND** pointing at "Slow down SYN floods" raises nothing

#### Scenario: A help and a tip
- **WHEN** a plugin sends a field with a help and a tip
- **THEN** the field's description is the help followed by the tip

### Requirement: What reads an option joins its chip
A field that names what reads its option SHALL show it in its option chip after the key, joined by a middot.

#### Scenario: A reservation's MAC
- **WHEN** a plugin sends a field keyed `mac` whose option is read by `dhcp host`
- **THEN** the field's chip reads `mac · dhcp host`

### Requirement: A note stands under its answer
A field's note, the sentence that says what its value does now, SHALL stand in the answer column under the control, and SHALL stay on screen when descriptions are folded.

#### Scenario: The theme
- **WHEN** an operator opens System › General
- **THEN** "Takes effect immediately, for everyone who signs in to this router." stands under the theme dropdown in its answer column

### Requirement: A refusal stands under the control it refuses
A field's refusal SHALL stand as its band under the control in the answer column, 8px under the control's line, or under a wide answer at the row's full width.

#### Scenario: A burst that is not a number
- **WHEN** an operator types "5o" as the Burst and saves
- **THEN** "Enter a whole number." stands in a refusal band under the Burst field in the answer column
- **AND** the Burst field's border is the refusal's colour

#### Scenario: A refusal beside a long description
- **WHEN** the Burst field is refused with its two-line description shown
- **THEN** the refusal band stands straight under the Burst field, beside the description, with no gap opened between the field and the band

### Requirement: A staged field is marked down its row's edge
A field whose change waits on the stage SHALL wear a bar in the stage's marigold down its row's left edge, from the line above the row to the line below it, on the card's border or a drawer's edge, and SHALL say "staged" to a screen reader beside its name.

#### Scenario: A staged switch
- **WHEN** an operator checks "Drop packets that belong to no connection" and saves
- **THEN** a marigold bar runs down the left edge of that field's row, inside the card's border
- **AND** a screen reader hears "staged" beside the field's name

### Requirement: Descriptions fold with one act
Every page and drawer that shows at least one description SHALL carry one act, "Hide descriptions", on a page's heading line or in a drawer's header. Pressing it SHALL fold every description on the page and in its drawers together, and the act SHALL then read "Show descriptions" and bring them back. A page or drawer with no description SHALL carry no such act.

#### Scenario: Folding on Firewall settings
- **WHEN** an operator presses "Hide descriptions" on Firewall settings
- **THEN** every description on the page folds away together, each row closing to its name's line
- **AND** the act reads "Show descriptions"

#### Scenario: The act's glyph
- **WHEN** descriptions are shown
- **THEN** "Hide descriptions" is led by a glyph of rows folding together
- **AND** once they are folded, "Show descriptions" is led by a glyph of rows folding apart

#### Scenario: A drawer over a listing
- **WHEN** an operator opens a zone's drawer from the Zones listing
- **THEN** the drawer's header carries "Hide descriptions"

#### Scenario: A page without descriptions
- **WHEN** an operator opens Devices
- **THEN** the heading line carries no descriptions act

#### Scenario: Reduced motion
- **WHEN** an operator whose system asks for reduced motion presses "Hide descriptions"
- **THEN** the descriptions fold at once, without moving

### Requirement: The browser keeps the fold
The fold SHALL be kept per browser and SHALL apply to every page and drawer that browser opens, from the first paint, until it is pressed again. A browser that never pressed it SHALL see descriptions. The fold SHALL NOT be staged and SHALL NOT be written to the router.

#### Scenario: Another page after folding
- **WHEN** an operator who folded descriptions on Firewall settings opens Network › DNS
- **THEN** the DNS page paints with its descriptions folded and its act reads "Show descriptions"

#### Scenario: Another browser
- **WHEN** a second operator signs in from another browser
- **THEN** they see every description

#### Scenario: The stage
- **WHEN** an operator folds descriptions with changes staged
- **THEN** the staged count is unchanged

### Requirement: Folding hides descriptions alone
Folding SHALL hide descriptions and nothing else: names, chips, staged marks, controls, notes, refusal bands and the detail under an item of a group SHALL stay. A folded description SHALL stay the accessible description of its control.

#### Scenario: What stays when folded
- **WHEN** an operator folds descriptions on Firewall settings with a refusal on Burst
- **THEN** the refusal band under Burst stays
- **AND** "Shell script · runs after the ruleset loads" stays under its included file

#### Scenario: A screen reader
- **WHEN** a screen reader user focuses the Burst field with descriptions folded
- **THEN** the field is described by "How many may arrive at once before the rate limit applies. Absorbs ordinary bursts, like a page opening 40 sockets."

### Requirement: A read-only fact in a settings card stands as a ledger row
A settings card's read-only fact SHALL stand as a ledger row: its title and description on the left, its value or pills in the answer column, with the same hairlines and air as a field row.

#### Scenario: A fact beside fields
- **WHEN** a settings card shows a read-only fact over two fields
- **THEN** the fact's value starts at the same place as the fields' controls

### Requirement: Forms that are not lists of settings keep their own layout
The sign-in form, inline and search forms, a chooser with no Save that only picks what a page shows, a listing's control band, the diagnostics tool row and a dialog's file drop SHALL keep their own layout and SHALL NOT render as ledger rows.

#### Scenario: Sign-in
- **WHEN** someone opens the sign-in page
- **THEN** the username and password fields stand under their labels

#### Scenario: A graph's own choices
- **WHEN** an operator opens the Hardware page
- **THEN** "Show chart for" still reads [CPU ▾] over [Last 24 hours ▾] over the graph

#### Scenario: Finding a package
- **WHEN** an operator opens System › Packages
- **THEN** "Find a package" stands in the listing's control band as it did

### Requirement: A plugin renders as a ledger without a change
The ledger SHALL come from the shell's rendering of the existing widget schema, so a plugin built before it renders its forms as ledger rows without being rebuilt and without any new schema field.

#### Scenario: An installed plugin built against an older shell
- **WHEN** the shell runs with a plugin built before the ledger existed
- **THEN** the plugin's form fields render as ledger rows with their help as descriptions
