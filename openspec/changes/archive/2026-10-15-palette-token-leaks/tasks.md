# Tasks

Every task ends green under `make lint` and `make test`, and a task that touches a template or stylesheet stages the rebuilt `internal/server/assets/verso.css` with it.

## 1. The palette as Go reads it

- [x] 1.1 Write `internal/palette` tests first: parsing the embedded `palette.css` returns every `--color-*` token of its `@theme` with its value (`canvas` → `#ffffff`, `ground` → `#faf9f7`, `hw-tip` → `#0b0f16`), ignores comments, `var()` references and the `[data-verso-off]` block, and errors on an empty source; then implement the parser and verify the tests pass
- [x] 1.2 Embed `assets/palette.css` in the server and load it once at startup through `internal/palette`; verify with a server test that the loaded palette carries `canvas` and `ground`

## 2. White's two jobs

- [x] 2.1 Write the test first: the compiled `verso.css` defines `--color-on-fill` as `#fff`/`#ffffff` and no `.text-white`, `.bg-white` or `--color-white`; then add `--color-on-fill` to `palette.css` (Neutral family, with its job in the comment) and unset `--color-white` and `--color-black`; verify the test fails until 2.2 lands and passes after
- [x] 2.2 Move every `text-white` to `text-on-fill` and every `bg-white` / `bg-white/50` to `bg-canvas` / `bg-canvas/50` in the widget templates (button, actionbar, shared, switch, field, callout, code, traffic, overview, table-interfaces), the shell templates (login, logs, staged, entity-panel) and `verso.js`; point `.verso-box`'s fill in `forms.css` at `--color-canvas`; update tests that assert the old class strings; verify `make test` and that the Overview, a primary button, a checked radio, a picked log filter and a field look unchanged in the dev container
- [x] 2.3 Start `TestNothingPaintsALiteralColour` in `internal/widget/palette_test.go` over the stock-ramp guard's roots plus the shell's non-test Go sources, failing on a `white`/`black` colour utility (not `white-space`); verify it passes, and fails when a template gains `text-white`

## 3. Shadows

- [x] 3.1 Write the test first: the compiled `verso.css` defines `--shadow-field`, `--shadow-float`, `--shadow-menu`, `--shadow-dialog`, `--shadow-drawer`, `--shadow-edge`, `--shadow-lifted` and `--shadow-port-tip` with today's values; then add them to a `:root` block of `palette.css` (a Shadow family, each with its job)
- [x] 3.2 Move every arbitrary `shadow-[…rgba(27,25,23,…)…]` in the templates to its token (`shadow-(--shadow-dialog)`, `shadow-(--shadow-float)`, …), the invalid-field composites to `var(--shadow-field)`, `forms.css`'s two field insets, `verso-tables.js`'s lifted row, the arranged Overview card and `input.css`'s port tip to their tokens; update `logs_test.go`, `field_a11y_test.go`, `heading_act_test.go`, `renderer_test.go` and `switch_test.go` to the token classes; verify `make test` and that a drawer, a dialog, a tooltip, an invalid field, a reordered row and a port's tip cast the same shadows as before
- [x] 3.3 Extend the guard to fail on `rgb(`, `rgba(` and `hsl(` with a number; verify it passes, and fails when a template gains `shadow-[0_1px_2px_rgba(0,0,0,.1)]`

## 4. Chart washes, the stray background and the takeovers

- [x] 4.1 Write the test first: the compiled `verso.css` defines `--chart-wash-from: .16`, `--chart-wash-to: .04`, `--chart-role-wash-from: .24`, `--chart-role-wash-to: 0`; then add them in a `:root` block of `palette.css` and point the `.verso-chart--*` stops in `input.css` at them; verify the Traffic card's washes look unchanged
- [x] 4.2 Move `table-interfaces.html.tmpl`'s `bg-[#f9f7f3]` to `bg-ground`; verify the expanded interface panel in the dev container
- [x] 4.3 Write the test first: the upgrading and firmware-started takeovers declare `theme-color` as the palette's `canvas` value and restarting as its `ground` value, read from the loaded palette; then render the meta from it in the three templates; verify the tests pass
- [x] 4.4 Extend the guard to fail on a hex value (not an `&#…;` entity, a `#fragment` or `url(#id)`) and an arbitrary `[#…]` class, with an exceptions map giving each kept literal its reason (the scroll-edge mask's `#000` alpha, the Safari pinned-tab brand colour); verify it passes, and fails when a template gains `bg-[#f9f7f3]`, naming file, literal and line

## 5. Docs

- [x] 5.1 Amend ADR-005 §6 in place: every colour and shadow the shell paints is a `palette.css` token, and the literal-colour guard enforces it; verify the ADR reads as what is
- [x] 5.2 Update DESIGN.md: the Neutral list names Canvas (a surface a step above the page) and On-fill (a label on a filled control) in place of White, the Shadow Vocabulary names each shadow's token, and the chart's wash names its tokens; verify `/home/tzaman/.claude/skills/impeccable/scripts/impeccable context` still loads DESIGN.md without errors
- [x] 5.3 Keep `docs/BACKLOG.md` current: remove what this change closes and add nothing for it; verify by reading the diff

## 6. Integration

- [x] 6.1 Walk the dev container (`make dev`) through the Overview, a list page with a drawer open, a dialog, an invalid field, Logs, the Interfaces table with a panel open, and each takeover, and confirm nothing changed visibly; then run `make lint` and `make test` once more
