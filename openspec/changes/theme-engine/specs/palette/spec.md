# Spec Delta

## MODIFIED Requirements

### Requirement: Every shadow is a palette token
Every shadow the shell casts SHALL keep its shape (offset, blur, spread) in the shell and take its colour from a named palette token, one per job: a field's inset, a tooltip, a menu, a dialog, a drawer, a scroll edge, a lifted row and a port's tip. A template, stylesheet or client script SHALL name the shadow, never write its colour.

#### Scenario: A drawer opens
- **WHEN** an operator opens a rule's drawer
- **THEN** the drawer casts the Drawer shadow in the Drawer shadow colour, the same shadow as before under Sand light

#### Scenario: A field with an error
- **WHEN** a field holds an invalid value and has focus
- **THEN** its crimson ring and its inset read tokens, and the inset is the same Field shadow every field casts

#### Scenario: A theme darkens a shadow
- **WHEN** the theme in force sets the Dialog shadow colour to black at half strength
- **THEN** a dialog casts its shadow at the same offset and blur, in that colour

### Requirement: Takeover pages take their bar colour from the palette
The upgrading, restarting and firmware-started takeovers SHALL declare the browser bar colour from the theme in force's value for their ground (Canvas for upgrading and firmware-started, Paper for restarting), never typed into the page. Under Sand following the browser they SHALL declare one bar colour for each preference, light and dark.

#### Scenario: Restarting on a phone
- **WHEN** a phone shows the restarting takeover under Sand, always light
- **THEN** the page declares Sand light's Paper value as its bar colour

#### Scenario: Restarting under Sand following a dark phone
- **WHEN** a phone that prefers dark shows the restarting takeover with no theme chosen
- **THEN** the page declares Sand dark's Paper value as its bar colour for a dark preference

## REMOVED Requirements

### Requirement: A chart's wash strength is a palette token
**Reason**: A wash is a colour like every other thing a theme changes; a strength alone could not be set by a theme file of colours.
**Migration**: The wash is a pair of palette colour tokens per series ("A chart's wash is a pair of palette colours"), whose defaults are the series' hue at the strengths this requirement held.

## ADDED Requirements

### Requirement: A chart's wash is a pair of palette colours
The wash under a chart series SHALL run between two palette colour tokens, where it starts at the line and where it fades to, for each series hue. By default each SHALL be the series' own hue at its strength (16% to 4% for the live traffic series, 24% to 0 for a role series), so a theme that changes the hue changes its wash.

#### Scenario: The Traffic card under Sand light
- **WHEN** the Overview's Traffic card draws download and upload under Sand light
- **THEN** each series' wash runs from its hue at 16% to its hue at 4%

#### Scenario: A theme sets the wash
- **WHEN** the theme in force sets download's wash to start at `#6b9bd640`
- **THEN** download's wash starts at that colour, and the grid rules still show through it
