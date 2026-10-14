# Spec Delta

## Purpose

The measuring system every page, card and drawer is laid out on: an 8px grid for every fixed measure, a 4px step inside a component, 20px text lines, and the dev-session overlay that checks alignment.

## ADDED Requirements

### Requirement: Fixed measures are multiples of 8px
Every fixed measure of the layout SHALL be a multiple of 8px. This covers a box's height or width, the air between boxes, a card's or a drawer's inset, and a page measure. The measure is taken from the box's outer edge, hairlines included.

#### Scenario: A control
- **WHEN** a field, a select or a button stands at its control height on any page or in any drawer
- **THEN** it is 32px tall

#### Scenario: Cards on a page
- **WHEN** an operator opens Firewall settings
- **THEN** each card's words start 24px in from its frame
- **AND** the cards stand 24px apart

#### Scenario: A page measure
- **WHEN** a default-measure page stands in a window wide enough for its full measure
- **THEN** its content column is 1016px wide

### Requirement: A squeezed column rounds down to the grid
A content column the window squeezes SHALL round its width down to a multiple of 8px, so its end and the rules that run to it land on the grid.

#### Scenario: A narrow window
- **WHEN** the room for a page's content column is 1013px
- **THEN** the column is 1008px wide

### Requirement: Inside a component a measure steps by 4px
A component's own box (a chip, a checkbox, a radio row, a row act) SHALL measure a multiple of 4px. Inside that box, padding SHALL centre the component's text on its line. Marks and glyphs keep their own sizes: the 6px state square, a 14px or 16px icon, a 1px hairline.

#### Scenario: A chip in a row
- **WHEN** a device row cites its interface as a chip
- **THEN** the chip is 20px tall and stands within the row's 20px line

#### Scenario: A checkbox
- **WHEN** a form shows a checkbox
- **THEN** its box is 16px square, at the top of its label's 20px line

#### Scenario: A radio set
- **WHEN** a form shows three radio choices
- **THEN** each choice is a 28px row

### Requirement: Text writes on 20px lines
Body text, labels, cells and ledes SHALL write on 20px lines. A box that grows with its text SHALL end on a 4px step, and a fixed box SHALL end on an 8px step.

#### Scenario: A one-line row
- **WHEN** a table row holds one line of text
- **THEN** the row is 40px tall, its hairline included

#### Scenario: A stacked row
- **WHEN** a table row holds a value with a detail line under it
- **THEN** the row is 60px tall

### Requirement: Chart plots are ruled every 16px
A chart's plot SHALL draw its horizontal rules 16px apart, and its height SHALL be a multiple of 64px so each of its four scale labels stands on a rule. The plot is 192px tall in a card or as a short chart, and 256px tall otherwise.

#### Scenario: A full-size graph
- **WHEN** a page shows a graph that is neither in a card nor short
- **THEN** the plot is 256px tall with a rule every 16px

#### Scenario: The Overview's traffic graph
- **WHEN** an operator opens the Overview with the Traffic card open
- **THEN** the plot is 192px tall with a rule every 16px
- **AND** the scale labels stand on the rules at 48, 96 and 144px down and at the top

### Requirement: A dev session can draw the grid
In a dev session, pressing G outside a text field SHALL draw the 8px grid over the page, from the top bar's hairline and the rail's border, and over an open drawer, from its chrome's hairline and its left edge. Pressing G again SHALL remove it. A real deployment SHALL never draw the grid.

#### Scenario: Turning the grid on
- **WHEN** a developer presses G on the Devices page in a dev session
- **THEN** 8px grid lines draw over the page's content area
- **AND** the page's controls stay usable under the lines

#### Scenario: Typing a G
- **WHEN** a developer types "G" into a field in a dev session
- **THEN** the letter goes into the field and the grid does not change

#### Scenario: A drawer open
- **WHEN** a developer has the grid on and opens a rule's drawer
- **THEN** the drawer shows the grid measured from its own chrome's hairline

#### Scenario: A reload keeps the choice
- **WHEN** a developer has the grid on and the dev session reloads the page after a rebuild
- **THEN** the grid is still on

#### Scenario: A real deployment
- **WHEN** an operator presses G on a router running a released build
- **THEN** no grid appears
