# Tasks

Every task ends green under `make lint` and `make test`; a task that touches a template or stylesheet stages the rebuilt `internal/server/assets/verso.css` with it.

## 1. Everything a theme changes is a colour

- [ ] 1.1 Write the tests first: `internal/palette` names every `--color-*` token `palette.css` declares in `@theme` and in its top-level `:root` block (including `var()` and `color-mix()` defaults, excluding `[data-verso-off]`), while `Colour` still answers only hex values; then implement `Names()` and verify the tests pass
- [ ] 1.2 Write the test first: `palette.css` declares `--color-shadow-field`, `-float`, `-menu`, `-dialog`, `-drawer`, `-edge`, `-lifted`, `-lifted-near` and `-port-tip` with today's colours, and each `--shadow-*` is its shape over `var(--color-shadow-…)`; then split the shadows and verify the compiled stylesheet computes the same shadows (the existing shadow test updated to the split)
- [ ] 1.3 Write the test first: `palette.css` declares `--color-chart-{denim,amethyst,violet,emerald}-{from,to}` as `color-mix()` of the series hue at 16%/4% and 24%/0, no `--chart-wash-*` remains, and every `.verso-chart--*` stop reads `stop-color: var(--color-chart-…)` at `stop-opacity: 1`; then move the stops and verify the Traffic card's washes look unchanged in the dev container
- [ ] 1.4 Update the `palette` main-spec-bound docs: DESIGN.md's Shadow Vocabulary names each shadow's shape and colour token, and the Traffic card's wash names its colour tokens; verify `impeccable context` still loads DESIGN.md

## 2. Theme files

- [ ] 2.1 Write `internal/theme` tests first: a valid file loads with its id, name, family, scheme and colours; a file with invalid JSON, no name, no or unknown scheme, an unknown key, a non-hex value (`red;}body{display:none`, `rgb(0,0,0)`, `#12345`) or the id `sand`/`sand-light` is skipped with one problem naming file and cause; values of 3, 4, 6 and 8 digits load; then implement `Load(fs.FS, names)` and verify
- [ ] 2.2 Write the tests first: `CSS` renders `sand` as a `prefers-color-scheme: dark` block of Sand dark with `color-scheme: dark`, `sand-light` as nothing, a fixed theme as one `:root` block with its scheme and only the colours it sets, and an unknown id as `sand` with the id reported once; `BarColours` gives Canvas/Paper per scheme (two for `sand`); then implement and verify
- [ ] 2.3 Write the tests first: `Contrast` reports each word pair under 4.5:1, glyph pair under 3:1 and meta on mid under 4.4:1 over Sand light overlaid with the theme, and a dark theme whose shadow colours all equal Sand light's; Sand light reports nothing; then implement and verify
- [ ] 2.4 Add `themes/sand-dark.json` and `themes/catppuccin-mocha.json` with the palettes in design.md §10 (shadow colours and washes included); write the test first that loads `themes/*.json` and fails on any contrast report; verify it passes

## 3. The theme on every page

- [ ] 3.1 Write the tests first: the server, given a theme set and a choice, renders `<style id="verso-theme">` straight after `<style id="verso-css">` on a page, the sign-in page and the three takeovers, and the takeovers declare the theme's bar colours (two media metas under `sand`); then hold the theme state behind an atomic pointer in the server and render it in the four templates; verify
- [ ] 3.2 Write the test first for `cmd/verso`: `-theme` sets the choice and `VERSO_THEMES_DIR` (default `/usr/share/verso/themes`) the directory, a missing directory loads no themes and logs it; then wire the flag, load at start, log each problem and contrast report, and re-read the directory in `Rescan` on SIGHUP; verify
- [ ] 3.3 Pass `-theme` from `verso.ui.theme` in the init script (docker rootfs and package), install `themes/*.json` into `/usr/share/verso/themes/` in the `verso` apk payload and the dev container (`scripts/dev.sh` deploys and signals on change); verify with `make apk APK_GOARCH=arm64` listing the files and the dev container serving Sand dark to a dark-preferring browser

## 4. Picking a theme

- [ ] 4.1 Write the Rust tests first: `setTheme` refuses an id outside `[a-z0-9-]{1,64}`, writes `verso.ui.theme` (creating `verso.ui=ui` when missing), deletes it for `sand`, and commits through root's own save directory without reloading; `EVERY_METHOD` and `verso-helper.json` grant it; then implement in `verso-rpcd` and verify with `cargo test`
- [ ] 4.2 Write the test first: the Go backend's `SetTheme(ctx, sid, id)` calls `verso.setTheme` with the session; then add it to `openwrt.Backend` and the fake; verify
- [ ] 4.3 Write the tests first: `POST /system/theme` refuses an id no loaded theme or built-in has, calls `SetTheme`, swaps the in-memory choice and answers with the new `<style id="verso-theme">` as an out-of-band swap; on a backend error it keeps the theme and answers `HX-Reswap: none` with the outcome template ("The theme wasn't changed."); it reads only `theme` from the request; then implement and verify

## 5. The Appearance card

- [ ] 5.1 Write the tests first: `{"type":"slot","id":"appearance"}` decodes to `widget.Slot`; the server fills a known slot before rendering and renders nothing for an unknown one; then implement `Slot`, the fill walk and the catalogue entry; verify
- [ ] 5.2 Write the Rust tests first: `Widget::slot("appearance")` serialises as the shell decodes it, and the system plugin's General page places it between its untitled section and Time; then implement in the SDK and the system plugin; verify with `cargo test` and a shell render of General
- [ ] 5.3 Write the tests first: the Appearance block renders as a ruled section titled "Appearance" with a select keyed `theme`, its options Sand following the browser, Sand always light, Sand always dark and each other theme as "<family> <name>", the theme in force selected, the takes-effect description under it, `form="verso-theme"`, `hx-post="/system/theme"`, `hx-trigger="change"`, `hx-params="theme"`, `hx-swap="none"`, and the page carries `<form id="verso-theme">` with its CSRF token outside the plugin's form; then implement the shell-only select properties and the block; verify
- [ ] 5.4 Make `verso-forms.js` count a change toward a form only when `event.target.form` is that form; verify in the dev container that picking a theme with a half-typed router name keeps the text, raises no unsaved-changes prompt and leaves Save as it was

## 6. Docs

- [ ] 6.1 Write ADR-024 (themes are packaged data): a theme is a file, the format and its validation, `verso-theme-<name>` beside `verso-plugin-<name>` and `verso-board-<name>`, everything a theme changes is a colour, Sand built in and following the browser; verify it reads as what is
- [ ] 6.2 Amend in place ADR-005 §6 (the theme mechanism in place of "one light theme"; the closed widget set gains `slot`), ADR-010 and ADR-013 (a setting that changes only how the pages look takes effect when picked, outside the stage); verify each reads as what is
- [ ] 6.3 Update DESIGN.md: a Themes section (Sand light and dark, fixed themes, the scheme), Sand dark's role table, and the Appearance card under Components; verify `impeccable context` loads it
- [ ] 6.4 Keep `docs/BACKLOG.md` current: add the release-notes line (a dark-preferring browser gets Sand dark; System › General forces light) and remove anything this change closes; verify by reading the diff

## 7. Integration

- [ ] 7.1 Walk the dev container: Sand follows a light and a dark browser; forcing Sand light and Sand dark; Mocha on a light browser; picking each with an alerts change staged leaves it staged and `uci show verso` holds the theme; a broken theme file in the directory is logged and not offered; installing a theme file and sending SIGHUP offers it; the three takeovers in the theme; then `make lint` and `make test` once more
