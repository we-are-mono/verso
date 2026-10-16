# ADR-024 — Themes are packaged data: files of colours over the palette

- **Status:** Accepted
- **Date:** 2026-10-15
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the shell owns all appearance, through the palette),
  ADR-006 (`verso-plugin-<name>`), ADR-023 (`verso-board-<name>`, packaged
  data), ADR-010 and ADR-013 (a setting that changes only how the pages look
  takes effect when picked), ADR-007 (the helper writes it).

## Context

Every colour and shadow the shell paints is a token of `palette.css`
(ADR-005 §6), so one file decides the whole look. That look is light, and an
operator on a dark desktop, or working at night, gets a bright page whatever
their system prefers. A dark palette alone answers one person; a way to lay
any palette over the shell's answers the dark desktop and the operator who
wants their own colours, and lets someone other than Verso write one.

## Decision

**A theme is one file of colours, read by the shell, laid over the palette.
Sand follows the browser between its light and dark palettes; any other theme
is fixed. The operator picks the router's theme on System › General, and the
pick takes effect at once.**

1. **A theme is a file.** `/usr/share/verso/themes/<id>.json`: a name, a
   family, a scheme (`light` or `dark`), and colours keyed by palette token
   name, each a hex of 3, 4, 6 or 8 digits. The id is the file's name. A key
   the palette does not declare, a value that is not such a hex, a missing
   name or scheme, or a built-in's id (`sand`, `sand-light`) leaves the file
   out, logged once with the cause; the rest load. The hex rule is the
   injection guard: a value is written into the page's style block, so it can
   be nothing but a colour. A colour a theme leaves out keeps Sand light's.

2. **Everything a theme changes is a colour.** A shadow's shape (offset,
   blur, spread) is the shell's and its colour a palette token; a chart
   series' wash is a pair of colour tokens, the series' hue mixed toward
   transparent by default. An 8-digit hex is how a theme writes a translucent
   one. Type, measure, geometry and the grid are never a theme's.

3. **Sand is built in.** Sand light is `palette.css` itself; Sand dark ships
   as `sand-dark.json` inside the `verso` package, as do Catppuccin Mocha and
   Tokyo Night Storm.
   `verso.ui.theme` unset, or `sand`, follows the browser's preference through
   a `prefers-color-scheme` block; `sand-light` and `sand-dark` force one; any
   other id is that theme, whatever the browser prefers, with its scheme
   declared so the browser's own controls follow. An id with no theme behind
   it draws as Sand following the browser, logged.

4. **A theme package installs one file.** `verso-theme-<name>`, beside
   `verso-plugin-<name>` (ADR-006) and `verso-board-<name>` (ADR-023): no code,
   no process. Its install and remove scripts signal the shell (SIGHUP), which
   reads the themes directory again and keeps the choice.

5. **The theme reaches every page in its first response.** The shell renders
   the theme in force once, when it or the themes change, and every page it
   serves (sign-in and the takeovers included) carries it in
   `<style id="verso-theme">` after the stylesheet, so the first paint is in
   it. The takeovers' browser bar colour follows it.

6. **The pick is the router's and takes effect at once.** System › General
   holds an Appearance card the system plugin places with a `slot` widget
   (ADR-005) and the shell fills. A pick goes to the helper's `setTheme`,
   which writes `verso.ui.theme` and commits `verso` through root's own save
   directory, so what the operator has staged stays staged; the shell swaps
   the theme in memory and the page repaints in place. The init script passes
   the option as `-theme` at start, so a hand edit takes effect on
   `/etc/init.d/verso restart`.

7. **Contrast is checked, not refused.** At load the shell logs each pairing
   of a theme's words under its floor (4.5 : 1, 3 : 1 for glyphs) and a dark
   theme that keeps the light palette's shadow colours. The themes the `verso`
   package ships are held to every floor by a test.

## Consequences

- A theme cannot break a page: a broken file is left out, a missing theme
  falls back to Sand, and no value but a colour reaches the style block.
- Someone else's palette is a file and a package, written against the token
  names `palette.css` declares; a renamed token is a theme's missing key, not
  a failure.
- The theme is the router's, not the browser's: everyone who signs in sees
  the same one. A per-person preference would be a different decision.
- An operator whose system prefers dark sees Sand dark after the upgrade that
  carries this; the release notes say so.
