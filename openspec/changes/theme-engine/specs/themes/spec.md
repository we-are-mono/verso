# Spec Delta

## Purpose

Which colours the shell paints with: a theme is one palette in one file, the operator picks the theme in force for the whole router, and Sand follows the browser between its light and dark palettes unless forced.

## ADDED Requirements

### Requirement: A theme is one palette in one file
A theme SHALL be a JSON file in the themes directory, its id the file's name without `.json`. It SHALL carry a name, a family, a scheme (`light` or `dark`) and colours keyed by palette token name, each a hex value of 3, 4, 6 or 8 digits. A colour the file leaves out SHALL keep Sand light's value.

#### Scenario: A partial theme
- **WHEN** a theme file sets only `denim` and `denim-deep`
- **THEN** pages under that theme draw their acts in its denim and every other colour as Sand light draws it

#### Scenario: A translucent colour
- **WHEN** a theme sets `shadow-dialog` to `#00000080`
- **THEN** a dialog under that theme casts its shadow in black at half strength

### Requirement: A theme file that breaks the format is skipped
The shell SHALL skip a theme file that is not valid JSON, lacks a name or scheme, names a key the palette does not have, carries a value that is not a 3, 4, 6 or 8 digit hex colour, or takes the id of a built-in theme. Each skipped file SHALL be logged once, naming the file and the cause, and the shell SHALL go on serving.

#### Scenario: A misspelt key
- **WHEN** a theme file sets `denium`
- **THEN** the theme is not offered, the log names the file and the key, and every other theme loads

#### Scenario: A value that would leave the stylesheet
- **WHEN** a theme file sets `ground` to `red;}body{display:none`
- **THEN** the theme is not offered and no page carries the value

### Requirement: Sand follows the browser unless forced
With no theme chosen, or Sand chosen, every page SHALL draw in Sand light when the browser prefers light and in Sand dark when it prefers dark. Sand, always light and Sand, always dark SHALL each draw in that palette whatever the browser prefers.

#### Scenario: A dark desktop
- **WHEN** an operator whose system prefers dark opens a router with no theme chosen
- **THEN** the page draws in Sand dark

#### Scenario: Forced light
- **WHEN** the theme is Sand, always light and the operator's system prefers dark
- **THEN** the page draws in Sand light

### Requirement: Any other theme is fixed
A theme other than Sand SHALL draw every page in its own palette whatever the browser prefers, and the page SHALL declare the theme's scheme, so the browser's own controls and scrollbars follow it.

#### Scenario: Mocha on a light desktop
- **WHEN** the theme is Catppuccin Mocha and the operator's system prefers light
- **THEN** the page draws in Mocha and its scrollbars are dark

### Requirement: An unknown theme falls back to Sand
When the chosen theme's id names no loaded theme, the shell SHALL draw in Sand following the browser and log the id once.

#### Scenario: A removed theme package
- **WHEN** the theme in force is uninstalled and the shell is restarted
- **THEN** every page draws in Sand following the browser, and the log names the missing theme

### Requirement: The theme reaches every page before it paints
Every page the shell serves (each page, the sign-in page, and the upgrading, restarting and firmware-started takeovers) SHALL carry the theme in force in its first response, so its first paint is in that theme.

#### Scenario: Signing in on a dark desktop
- **WHEN** an operator whose system prefers dark opens the sign-in page with no theme chosen
- **THEN** the page arrives in Sand dark, with no light frame first

### Requirement: The theme is picked on System › General
System › General SHALL show an Appearance card above Time with one dropdown, the theme, offering Sand following the browser, Sand always light, Sand always dark and every other loaded theme by family and name. Under it the card SHALL say "Takes effect immediately, for everyone who signs in to this router."

#### Scenario: The card
- **WHEN** an operator opens System › General
- **THEN** the Appearance card stands between the card of router settings and Time, its dropdown naming the theme in force

### Requirement: A theme takes effect when it is picked
Picking a theme SHALL write it to the router at once, outside the staged changes, and the page SHALL change to it in place without reloading. Changes the operator has staged SHALL stay staged, and fields not yet saved on the page SHALL keep what was typed.

#### Scenario: Picking Mocha with a change staged
- **WHEN** an operator with an alerts change staged picks Catppuccin Mocha on System › General
- **THEN** the page turns to Mocha, `uci show verso` holds the theme, and the alerts change still waits in the staged-changes chip

#### Scenario: Picking a theme with the router name half typed
- **WHEN** an operator types a new router name, does not save, and picks a theme
- **THEN** the page turns to the theme and the router name field still holds what was typed

#### Scenario: The write fails
- **WHEN** the router refuses the theme
- **THEN** the page keeps its theme and a notification says the theme was not changed

### Requirement: A theme package appears when it is installed
The shell SHALL read the themes directory again when it is told to re-read what is on disk (SIGHUP), so a theme a package installs is offered without a restart.

#### Scenario: Installing a theme package
- **WHEN** a theme package is installed and its install script signals the shell
- **THEN** the Appearance dropdown offers the new theme on the next page load

### Requirement: Themes keep their words readable
At load the shell SHALL log, per theme, each pair of text and ground that falls under its floor (4.5:1 for words, 3:1 for glyphs), and SHALL log a dark theme that leaves its shadow colours at Sand light's values. The themes the `verso` package ships SHALL meet every floor.

#### Scenario: A low-contrast theme
- **WHEN** a theme sets `meta` so that it stands at 3.9:1 on `ground`
- **THEN** the theme loads and the log names the theme, the pair and the ratio

#### Scenario: The shipped themes
- **WHEN** the test suite runs
- **THEN** Sand light, Sand dark and Catppuccin Mocha each meet every floor
