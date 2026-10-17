# Spec Delta

## Purpose

How the shell's stylesheet and markup name and define every look and every piece of text the app draws: one block per look and one role per kind of text, each defined once, named by the markup that draws it, and held to that by tests. A fix to a look then reaches everywhere it is drawn, today and on every screen built later.

## ADDED Requirements

### Requirement: The stylesheet has three layers
The shell's stylesheet SHALL hold three kinds of rule:
- tokens (the palette, the theme and the type roles);
- blocks, one file per block, in the components layer;
- Tailwind utilities.

The entry stylesheet SHALL hold only imports, sources, font faces, the theme, utility definitions and rules on bare elements. No rule outside a block file or the type roles' file SHALL select a `verso-*` class or a `data-verso-*` attribute.

#### Scenario: A component rule outside the block files
- **WHEN** the entry stylesheet holds a rule selecting `.verso-field-row`
- **THEN** the stylesheet guard test fails, naming the file and the selector

#### Scenario: A rule on a bare element
- **WHEN** the entry stylesheet styles `summary::-webkit-details-marker`
- **THEN** the guard test passes

### Requirement: Type comes from a closed set of roles
Every piece of text the shell draws SHALL take its face, size, line height, weight, letter spacing and numerals from one type role. Each role is one class defined once in the type roles' file and listed by that class in DESIGN.md's typography hierarchy with its full spec, and every role listed there SHALL be defined. A role carries no ink: the block that applies it sets the colour.

#### Scenario: Changing a role
- **WHEN** a developer changes the card title role's weight in the type roles' file
- **THEN** every Overview card title, page card title and drawer card title shows the new weight, with no template or block edited

#### Scenario: A role missing from DESIGN.md
- **WHEN** the type roles' file defines `type-caption` and DESIGN.md's hierarchy does not list it
- **THEN** the type guard test fails, naming the role

#### Scenario: One role in two inks
- **WHEN** a fact shows a value that is about to change
- **THEN** the value it was and the value it becomes share the value role, in Meta and in Ink

### Requirement: A value's face follows its content
The shell SHALL mark a string the machine wrote (an address, an interface name, a version, a config key) as verbatim, and a count or duration as figures, wherever it renders one. A role that holds values SHALL define its verbatim form (in the mono face) and its figures form (tabular numerals). A count or duration SHALL never take the verbatim form.

#### Scenario: An address in a fact list
- **WHEN** the Overview's Internet card shows its IPv4 address
- **THEN** the value is marked verbatim and stands in the value role's mono form, 16px at 500

#### Scenario: An uptime in the same list
- **WHEN** the same card shows how long the uplink has been up
- **THEN** the value is marked as figures, stands in the value role's sans face with tabular numerals, and is not mono

#### Scenario: A protocol chip beside a heading
- **WHEN** the IPv4 head shows its protocol chip "dhcp"
- **THEN** the chip is marked verbatim and stands in the chip role's mono form

### Requirement: No type is written outside the roles
Templates, client scripts and block files SHALL NOT write a font size, weight, line height, letter spacing or font family themselves, whether as a Tailwind utility (`text-sm`, `font-semibold`, `leading-6`, `tracking-tight`, `font-mono`) or as a declaration. Only the type roles' file SHALL.

#### Scenario: A size utility in a template
- **WHEN** a template writes `text-sm font-medium` on a label
- **THEN** the type guard test fails, naming the template and the element

#### Scenario: A weight in a block
- **WHEN** a block file declares `font-weight: 600` on its title
- **THEN** the type guard test fails, naming the block file and the rule

### Requirement: A look is named as a block
A look the shell draws SHALL be named as a block:
- the root carries `verso-<block>`;
- each named part carries `verso-<block>__<element>`;
- each variant or state is a `data-verso-*` attribute on the root, never a class.

No `verso-*` class SHALL use another form. Every name carries the `verso-` prefix, so none matches a Tailwind utility.

#### Scenario: A part of a block
- **WHEN** a chart draws its plot line
- **THEN** the line carries `verso-chart__line`, not `verso-chart-line`

#### Scenario: A variant of a block
- **WHEN** a chart draws a series in denim
- **THEN** the chart's root carries a `data-verso-*` attribute naming denim, not the class `verso-chart--denim`

#### Scenario: A name that would collide
- **WHEN** the table's block is defined
- **THEN** its class is `verso-table`, never `table`, which is Tailwind's `display: table` utility

### Requirement: A block's rules live in its file and start from it
Each block's rules SHALL live in one file named for the block. Every selector in that file SHALL start from the block's own class: its root, an element, or the root with a variant attribute. It MAY then reach inside, but SHALL NOT start from an ancestor outside the block.

#### Scenario: A rule that starts outside its block
- **WHEN** the card's block file holds a rule whose selector starts at `.verso-page-body`
- **THEN** the block guard test fails, naming the file and the selector

#### Scenario: Changing a block's look
- **WHEN** a developer changes the card's border token in the card's block file
- **THEN** every page card, drawer card and Overview card shows the change, with no template edited

### Requirement: A block adjusts another block only through its custom properties
A block SHALL expose each measure a container may change as a custom property it reads with its own default. A rule in one block's file that reaches an element of another block SHALL set only custom properties, never another declaration.

#### Scenario: A card bleeds its rows to its borders
- **WHEN** a card holds field rows
- **THEN** the card's rule on those rows sets only the row's bleed property, and the row's own block turns that into margin and padding

#### Scenario: A cross-block rule sets a declaration
- **WHEN** the card's block file sets `margin-inline` on `.verso-field-row`
- **THEN** the block guard test fails, naming the rule

### Requirement: A utility outranks a block
A Tailwind utility on an element SHALL take precedence over the look of any block on that element, whatever the block selector's specificity or order. A one-off adjustment never needs a stronger selector.

#### Scenario: A one-off margin on a card
- **WHEN** a template writes `mt-0` on an element that carries `verso-card`
- **THEN** the element's top margin is 0, whatever the card block sets

### Requirement: Markup names a look rather than repeating it
Templates and client scripts SHALL NOT draw the same look twice from utilities. A set of four or more utilities (in any order, template actions aside) appearing on more than one element across templates and client scripts SHALL be a block.

#### Scenario: A copied bundle
- **WHEN** two templates each write `size-1.5 shrink-0 rounded-[1px] border border-faint`
- **THEN** the markup guard test fails, naming both places

#### Scenario: A client script draws a row a template draws
- **WHEN** the live listing script builds a table row
- **THEN** the row and its cells carry the table block's classes, as the template's do

### Requirement: An element's class list stays short
An element in a template or client script SHALL carry at most eight Tailwind utilities, counting each responsive or state variant as one. An element that needs more has a look, and that look is a block.

#### Scenario: A long class list
- **WHEN** a template writes eleven utilities on a button
- **THEN** the markup guard test fails, naming the template and the element

### Requirement: Markup carries no stylesheet rules
Templates and client scripts SHALL NOT write arbitrary-variant utilities (`[&…]:`) or important utilities (`!`-marked). What either expresses belongs in a block.

#### Scenario: A selector written into a class attribute
- **WHEN** a template writes `[&_td:last-child]:!text-right`
- **THEN** the markup guard test fails, naming the template

### Requirement: No template exists only to print classes
A template definition whose whole output is a class list SHALL NOT exist. The look it names is a block.

#### Scenario: A class-string template
- **WHEN** a template file defines `verso-chip-class` as a run of utilities
- **THEN** the markup guard test fails, naming the definition

### Requirement: Every block class is defined and used
Every `verso-*` class written by a template, a client script or the Go that renders SHALL be defined by a block file. Every class a block file defines SHALL be written somewhere.

#### Scenario: A misspelt block
- **WHEN** a template writes `verso-crad`
- **THEN** the markup guard test fails, naming the template and the class

#### Scenario: A block part nothing draws
- **WHEN** a block file defines `verso-card__foot` and no markup writes it
- **THEN** the guard test fails, naming the block file and the class

### Requirement: The shell names an element's role where it renders it
Where the stylesheet treats an element by its role (a card, a part, a gate's open branch), the shell SHALL mark that role when it renders the element. The stylesheet SHALL find the role only through the mark, never from the element's ancestors or children. Only state that changes in the browser (a checked switch, an open disclosure) MAY be read from the document.

#### Scenario: A titled section on a page
- **WHEN** Firewall settings renders "Protection"
- **THEN** the section carries `verso-card` and is of kind `card`

#### Scenario: A part inside a card
- **WHEN** System › Access renders SSH's "Authorized keys" part
- **THEN** the part does not carry `verso-card`

#### Scenario: A drawer's loose fields
- **WHEN** a port forward's drawer stands "Forward is active" and Name in an untitled card
- **THEN** that card carries `verso-card`

#### Scenario: A switch opens its conditional
- **WHEN** an operator checks a switch that gates a conditional body
- **THEN** the stylesheet reads the checked state from the document to show the body

#### Scenario: A plugin built against an older shell
- **WHEN** an installed plugin's page renders a titled section
- **THEN** the section carries `verso-card` without the plugin being rebuilt

### Requirement: Every card draws one frame
Page cards, drawer cards and the Overview's cards SHALL all take their frame (border, corner, ground) and side inset from the card block. The Overview card SHALL add only what is its own (collapsing, arranging, its tone) through a second block on the same element.

#### Scenario: The Overview's Internet card
- **WHEN** an operator opens the Overview
- **THEN** the Internet card carries `verso-card` and the Overview card's block
- **AND** its frame and side inset match a page card's to the pixel

### Requirement: A container's edges count only visible content
A block that treats its first or last content differently SHALL count only visible content. Hidden carriers (hidden inputs, templates, hidden elements) and a closed conditional's body SHALL NOT count, whether they come first or last.

#### Scenario: A card's form opens with its CSRF carrier
- **WHEN** a card's form begins with a hidden CSRF input followed by the "Router name" row
- **THEN** "Router name" stands 15px under the header's seam, as it would with no hidden input

#### Scenario: A card ends on a closed conditional
- **WHEN** a card's last block is a switch whose conditional body is closed
- **THEN** the card closes under the switch's row as it closes under any last row
