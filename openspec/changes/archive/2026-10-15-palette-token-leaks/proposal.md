# Proposal

## Why

Nothing an operator sees changes: every page keeps today's look, pixel for pixel. What changes is that every colour and shadow the shell paints comes from a named token in `palette.css`, so one file decides the whole look. Today some of them are written in place: white does two jobs (a surface and the label on a coloured fill), shadows carry Ink as a literal, a chart's wash strength is a bare number, one panel has a hand-written background, and three takeover pages set the browser's bar colour by hex. Each of those is a colour that a palette change silently misses, and a theme loaded over the palette would leave them in their light values.

## What Changes

- White's two jobs become two tokens. A surface a step above the page (a field's fill, the top bar) reads `canvas`, which already exists. A label set on a coloured fill (the primary, danger and caution buttons, a checked choice, Arrange while arranging) reads a new `on-fill` token. Both hold today's white.
- Every shadow becomes a named token in `palette.css`, one per job the Shadow Vocabulary names (field inset, tooltip, menu, dialog, drawer, scroll edge, a lifted row, a port's tip), each holding today's value. Templates, stylesheets and the client's built markup reference the token, never an `rgba(…)`.
- A chart series' wash strength (16% to 4% for the live traffic series, 24% to 0 for a role series) becomes a token pair.
- The Interfaces table's literal `#f9f7f3` panel background reads a palette token.
- The `theme-color` of the upgrading, restarting and firmware-started takeovers is rendered from the palette's own `canvas` and `ground`, read from `palette.css` at startup, instead of being typed into each template.
- A guard test fails on any colour literal (hex, `rgb()`/`rgba()`/`hsl()`, a `white`/`black` utility, an arbitrary `[#…]`) in the shell's templates, stylesheets and client scripts outside `palette.css`, beside the existing stock-ramp guard. Each kept literal is listed with its reason.

## Capabilities

### New Capabilities

- `palette`: where every colour and shadow the shell paints comes from (the named tokens of `palette.css`), the jobs the surface and on-fill tokens split between them, and the guard that keeps literals out of the shell's sources.

### Modified Capabilities

None.

## Impact

- **ADR-005 (UI consistency contract)** §6: amended in place to say that every colour and shadow is a palette token, with the guard that enforces it. Its "one light theme" sentence is unchanged by this change.
- **DESIGN.md**: the Neutral list names Canvas and On-fill in place of White; the Shadow Vocabulary names each shadow's token.
- `internal/server/assets/palette.css`: the new `on-fill`, shadow and wash tokens.
- `internal/server/assets/forms.css`, `input.css`: the field box, the chart washes and the port tip read tokens.
- Widget templates (`internal/widget/templates/`: button, actionbar, shared, switch, field, callout, code, traffic, overview, table-interfaces, list, conditions, modal, table) and shell templates (`internal/server/templates/`: page, login, logs, diagnostics, staged, entity-panel, upgrading, restarting, firmware-started): `text-white`, `bg-white` and the shadow arbitrary values move to tokens.
- `internal/server/assets/verso.js`, `verso-tables.js`: classes they build move to tokens.
- A small Go reader of `palette.css`'s tokens, embedded in the binary, used for the takeovers' `theme-color`.
- Tests: a new literal-colour guard in `internal/widget/palette_test.go`; tests that assert on today's arbitrary shadow strings (`logs_test.go`, `field_a11y_test.go`, `heading_act_test.go`, `renderer_test.go`, `switch_test.go`) follow the tokens.
- No on-device state changes, so no migration. Plugins are untouched: they never express appearance (ADR-005).
