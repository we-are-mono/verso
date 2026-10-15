# Proposal

## Why

Verso has one look, Sand on a light page, and an operator working at night or on a dark desktop gets a bright page whatever their system says. After this change Verso follows the browser into a dark Sand by default, an operator can force Sand light or Sand dark, and they can pick another theme, Catppuccin Mocha first, from System › General. The choice takes effect the moment it is picked, for everyone who signs in to the router. A theme is a small JSON file of colours, so a theme someone else writes is a package with one file and no code.

## What Changes

- A theme is one palette in one file, `/usr/share/verso/themes/<id>.json`: a name, a family, a scheme (light or dark) and colours keyed by palette token name, each a hex value of 3, 4, 6 or 8 digits. A key the palette does not have rejects the file; a key the file leaves out keeps Sand light's value. A file that fails is logged once and skipped, never fatal.
- Sand light stays `palette.css` itself. Sand dark (the inverted sand palette) and Catppuccin Mocha ship as files inside the `verso` package. Another theme ships as `verso-theme-<name>`, a package that installs one file.
- One UCI option, `verso.ui.theme`. Unset or `sand`: Sand follows the browser's light or dark preference. `sand-light`, `sand-dark`: Sand, forced. Any other theme's id: that theme, whatever the browser says. An id with no theme behind it falls back to Sand following the browser, logged once.
- Every page the shell serves, the sign-in page and the upgrading, restarting and firmware-started takeovers included, carries the active theme as an inline block of custom properties after the stylesheet, so the first paint is already themed. The takeovers' browser bar colour follows the theme.
- Everything a theme changes is a colour. Shadows keep their shape in the shell and take their colour from a palette token per job; a chart series' wash is a pair of colour tokens (where it starts at the line, where it fades to), defaulting to the series' own hue at today's strengths. An 8-digit hex is how a theme writes a translucent one.
- System › General gains an Appearance card, above Time, with one dropdown: the theme. The system plugin places the card with a new generic widget, `slot`, and the shell fills it: the plugin decides where, the shell owns the setting. Picking a theme writes `verso.ui.theme` at once through the privileged helper (`verso-rpcd`), outside the operator's staged changes, which stay staged; the page changes colour in place, with no reload. The card says: "Takes effect immediately, for everyone who signs in to this router."
- The shell reads the theme once at start (an init-script flag, as its other settings are) and holds it in memory; a write swaps it, and SIGHUP re-reads the themes directory, so a theme package appears in the dropdown when it is installed.
- At load, a theme whose words fall under the contrast floors, or a dark theme that leaves its shadow colours at Sand light's, is logged with the pair that falls short. First-party themes are held to the floors by a test.
- **BREAKING (visible):** with `verso.ui.theme` unset, a router whose operator's system prefers dark shows Sand dark after the upgrade. No on-device state changes, so there is no migration.

## Capabilities

### New Capabilities

- `themes`: what a theme file is and how it is loaded, which theme is in force and how it is chosen, how it reaches every page, and the Appearance card that picks it.

### Modified Capabilities

- `palette`: a shadow's shape is the shell's and its colour a palette token; a chart series' wash is a pair of colour tokens in place of a pair of strengths; the takeovers' bar colour comes from the theme in force.

## Impact

- **A new ADR, ADR-024 (themes are packaged data)**: a theme is a file the shell reads, not a plugin it asks; `verso-theme-<name>` beside ADR-006's `verso-plugin-<name>` and ADR-023's `verso-board-<name>`; why everything a theme changes is a colour.
- **ADR-005 (UI consistency contract)**: §6's "one light theme; there is no dark palette" becomes the theme mechanism; the closed widget set gains `slot`.
- **ADR-010 (staged changes)** and **ADR-013 (Verso's own settings live in uci)**: a setting that changes only how the pages look takes effect when it is picked, outside the stage.
- **DESIGN.md**: the themes, Sand dark's role table, the Appearance card, and the shadow and wash colour tokens.
- Shell: a new `internal/theme` package (load, validate, contrast, CSS); `internal/palette` names every token a theme may set; the four page templates carry the theme block; a `-theme` flag and themes directory in `cmd/verso`; SIGHUP re-reads themes; a POST route for the Appearance card; `widget.Slot` and the shell filling it.
- `palette.css`: shadow colour tokens and chart wash colour tokens; `input.css` chart stops read them.
- `verso-rpcd`: a `setTheme` method, granted in `verso-helper.json`; the Go backend gains `SetTheme`.
- SDK crate: `Widget::slot`; the system plugin places the Appearance slot. The system plugin ships inside the `verso` package, so shell and plugin always upgrade together.
- Init script: `-theme` from `verso.ui.theme`. Packaging: `themes/*.json` into `/usr/share/verso/themes/` in the `verso` apk and the dev container.
- `verso-forms.js`: a page form's change tracking ignores a control that belongs to another form.
