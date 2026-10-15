# palette Specification

## Purpose
Where every colour and shadow the shell paints comes from: the named tokens of one palette, each with one job, so changing the palette changes the whole look and nothing keeps an old colour behind.

## Requirements

### Requirement: Every colour the shell paints is a palette token
Every colour the shell paints SHALL come from a named token of the palette: text, fills, borders, hairlines, marks, a chart's lines and washes, and the browser bar colour a page declares. A template, stylesheet or client script outside the palette SHALL name a token, never write a colour value.

#### Scenario: A palette value changes
- **WHEN** the palette's Hairline token is given a new value and the stylesheet is rebuilt
- **THEN** every hairline on every page, card and drawer shows the new value
- **AND** no hairline keeps the old one

#### Scenario: Plugins stay out of appearance
- **WHEN** a plugin's page renders
- **THEN** every colour on it comes from the palette through the widgets the shell renders, and none from the plugin

### Requirement: A surface and a label on a fill are separate tokens
A surface a step above the page (a field's fill, a select's fill, the top bar) SHALL read the Canvas token. A label set on a filled control (the primary, danger and caution buttons, a checked choice, a picked log filter, Arrange while arranging) SHALL read the On-fill token. Both SHALL hold white.

#### Scenario: A primary button
- **WHEN** a page shows its primary act on Denim
- **THEN** the act's words read the On-fill token

#### Scenario: A field
- **WHEN** a form shows an address field
- **THEN** the field's fill reads the Canvas token

#### Scenario: Today's look holds
- **WHEN** any page renders
- **THEN** fields, the top bar and labels on fills show the same white as before the split

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

### Requirement: A chart's wash is a pair of palette colours
The wash under a chart series SHALL run between two palette colour tokens, where it starts at the line and where it fades to, for each series hue. By default the start SHALL be the series' own hue at its strength (16% for the traffic and temperature series, 24% for a role series) and the end SHALL be transparent at the plot's bottom border, so a theme that changes the hue changes its wash and a quiet line rides clean paper.

#### Scenario: The Traffic card under Sand light
- **WHEN** the Overview's Traffic card draws download and upload under Sand light
- **THEN** each series' wash runs from its hue at 16% to nothing at the bottom border

#### Scenario: A theme sets the wash
- **WHEN** the theme in force sets download's wash to start at `#6b9bd640`
- **THEN** download's wash starts at that colour, and the grid rules still show through it

### Requirement: Takeover pages take their bar colour from the palette
The upgrading, restarting and firmware-started takeovers SHALL declare the browser bar colour from the theme in force's value for their ground (Canvas for upgrading and firmware-started, Paper for restarting), never typed into the page. Under Sand following the browser they SHALL declare one bar colour for each preference, light and dark.

#### Scenario: Restarting on a phone
- **WHEN** a phone shows the restarting takeover under Sand, always light
- **THEN** the page declares Sand light's Paper value as its bar colour

#### Scenario: Restarting under Sand following a dark phone
- **WHEN** a phone that prefers dark shows the restarting takeover with no theme chosen
- **THEN** the page declares Sand dark's Paper value as its bar colour for a dark preference

### Requirement: A guard keeps colour literals out of the shell's sources
A test SHALL fail when a template, stylesheet or client script outside the palette writes a colour literal: a hex value, an `rgb()`, `rgba()` or `hsl()` value, a white or black utility, or an arbitrary hex class. A literal kept on purpose SHALL be listed with its reason.

#### Scenario: A new template writes a hex colour
- **WHEN** a template is added with `bg-[#f9f7f3]`
- **THEN** the guard fails and names the file, the literal and the line

#### Scenario: A literal kept on purpose
- **WHEN** a stylesheet uses `#000` as a mask's alpha
- **THEN** the guard passes, because the literal is listed with its reason (a mask's alpha, not a painted colour)
