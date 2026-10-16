# layout-grid Specification

## Purpose
The measuring system every page, card and drawer is laid out on: an 8px grid for every fixed measure, a 4px step inside a component, 24px text lines, and the dev-session overlay that checks alignment.

## Requirements

### Requirement: Fixed measures are multiples of 8px
Every fixed measure of the layout SHALL be a multiple of 8px. This covers a box's height or width, the air between boxes, a card's or a drawer's inset, and a page measure. A framed box is measured between the lines its borders stand on, so it is a multiple of 8px and one pixel more from outer edge to outer edge.

#### Scenario: A control
- **WHEN** a field, a select or a button stands at its control height on any page or in any drawer
- **THEN** its top and bottom borders stand 32px apart, on grid lines unless it stands beside its field's name
- **AND** whatever holds it measures it as 32px

#### Scenario: Cards on a page
- **WHEN** an operator opens Firewall settings
- **THEN** each card's words start 24px from its frame's outer edge
- **AND** the cards stand 24px apart

#### Scenario: A page measure
- **WHEN** a default-measure page stands in a window wide enough for its full measure
- **THEN** its content column is 1017px wide, 1016px and the pixel its cards' right border lands on

#### Scenario: The answer column
- **WHEN** a form shows its fields as rows on a page or in a drawer
- **THEN** the answer column is 272px wide, 24px from the question column
- **AND** a control that fills it has its left and right borders on grid lines

### Requirement: A squeezed column rounds down to the grid
A content column the window squeezes SHALL round its width down to a multiple of 8px and one pixel more, so its end and the rules and frames that run to it land on the grid. A drawer's content column SHALL round the same way, whatever the drawer's width.

#### Scenario: A narrow window
- **WHEN** the room for a page's content column is 1013px
- **THEN** the column is 1009px wide

#### Scenario: A drawer on a phone
- **WHEN** an operator opens a rule's drawer on a 390px-wide phone
- **THEN** a field running the drawer's width ends with its right border on a grid line

### Requirement: Every drawn edge stands on a grid line
Every border, hairline and frame the layout draws SHALL stand on a grid line, top, bottom, left and right. A box gives its bottom border's pixel back to what follows it, so the next box starts on a line too. A control standing beside its field's name SHALL centre on the name's 24px line instead, its top and bottom borders 4px inside the lines over and under that line, its left and right borders on lines.

#### Scenario: A notice
- **WHEN** a page shows a notice
- **THEN** both its top and bottom borders stand on grid lines
- **AND** its words start 24px from its frame's outer edge, with its state square in the middle 8px of that 24px

#### Scenario: A field of a fixed width
- **WHEN** a form shows an address field at its own measure
- **THEN** its left and right borders both stand on grid lines

#### Scenario: A control beside its name
- **WHEN** a form shows "When refusing a connection, say" with its dropdown in the answer column
- **THEN** the dropdown's value and the field's name write on the same 24px line
- **AND** the dropdown's top border stands 4px above that line and its bottom border 4px below it

#### Scenario: The colophon on a short page
- **WHEN** a page is shorter than the window, at any window height
- **THEN** the colophon stands at the window's foot with its hairline on a grid line

### Requirement: Inside a component a measure steps by 4px
A component's own box (a chip, a checkbox, a radio row, a row act) SHALL measure a multiple of 4px. Inside that box, padding SHALL centre the component's text on its line, and the 6px between a glyph and its words is the component's own. Marks and glyphs keep their own sizes: the 6px state square, a 14px or 16px icon, a 1px hairline. The option chip beside a setting's name is the one box measured by eye, so its word reads centred.

#### Scenario: A chip in a row
- **WHEN** a device row cites its interface as a chip
- **THEN** the chip is 18px tall and stands 3px inside the row's 24px line at either end

#### Scenario: The option chip beside a name
- **WHEN** a form shows "Traffic to this router" with its `input` chip
- **THEN** the chip is 21px tall, 2px under the top of the name's 24px line and 1px over its bottom

#### Scenario: A checkbox
- **WHEN** a form shows a checkbox
- **THEN** its box is 16px square, centred on its field's 24px name line

#### Scenario: A radio set
- **WHEN** a form shows three radio choices
- **THEN** each choice is 32px tall, the three standing side by side on one line

### Requirement: Text writes on 24px lines
Body text, labels, cells and ledes SHALL write on 24px lines, so a box that grows with its text ends on the 8px grid as a fixed box does.

#### Scenario: A field
- **WHEN** a page form shows a field as a row
- **THEN** the field's name writes on a 24px line 15px under the hairline above its row, and the row ends 16px under its last line
- **AND** a row with one line is 56px, its hairline included

#### Scenario: A field with its description
- **WHEN** a field row shows a description of three lines
- **THEN** each line is 24px and the row is 128px, its hairline included

#### Scenario: A stacked field
- **WHEN** a field row stands narrower than 576px
- **THEN** the name's line is 24px and the 32px control stands straight under it

#### Scenario: A one-line row
- **WHEN** a table row holds one line of text
- **THEN** the row is 40px tall, its hairline included

#### Scenario: A stacked row
- **WHEN** a table row holds a value with a detail line under it
- **THEN** the row is 64px tall

### Requirement: Chart plots are ruled in squares
A chart's plot SHALL be ruled in square cells, four cells tall, so each of its four scale labels stands on a rule, and its width SHALL be whole cells cut into whole steps of its window, with a bottom border on the grid line under it. The cell SHALL be 32px on a page's card (a 128px plot) and 24px in an Overview card or a drawer (a 96px plot). Every rule stands on an 8px grid line.

#### Scenario: A full-size graph
- **WHEN** a page shows a graph in one of its cards
- **THEN** the plot is 128px tall with a rule every 32px across and down

#### Scenario: The Overview's traffic graph
- **WHEN** an operator opens the Overview with the Traffic card open
- **THEN** each lane's plot is 96px tall with a rule every 24px across and down
- **AND** the scale labels stand on the rules at 24, 48 and 72px down and at the top

#### Scenario: A drawer's graph
- **WHEN** an operator opens a device's drawer with its Usage graph
- **THEN** each lane's plot is 96px tall with a rule every 24px across and down

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
