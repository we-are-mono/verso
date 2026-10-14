# Design

## Context

Grid measures live in three places:

- **Theme tokens** in `input.css`: `--spacing-control` (34px, the `h-control` utility) and the container measures (`--container-3xl` … `--container-6xl`). The measure rounding is `round(down, …, 1.25rem)` in `input.css`, and the Overview's columns round with `round(down, …, 20px)`.
- **Section and form sheets**: `sections.css` and `forms.css` state the cell in `calc(var(--spacing) * N)`, where N = 5 is the cell.
- **Templates and scripts**: Tailwind utilities in `internal/widget/templates`, `internal/server/templates` and a few class strings in scripts. About 400 of them are fractional (`pt-2.5`, `pb-2.25`, `px-3.75`, …), the 20px cell's optical half-steps. Many more are whole multiples of 5 (`mt-5`, `gap-5`, `p-5`): a cell, as 20px.

Plugins send a widget schema and never a class (checked: no Rust plugin emits a spacing class), so the shell owns every measure.

The dev session injects `verso-dev.js` only when `page.Dev` is set. Commit `adc5625` removed its G grid switch, together with the drawn notebook grid.

The confirmed target is the Verso grid preview artifact (see proposal.md).

## Goals / Non-Goals

**Goals:**
- Every value from the proposal's list is in place, app-wide: pages, cards, drawers, the Overview, live pages, sign-in, takeovers and the staged review.
- The grid is stated once, as named role tokens, so the next change to it is a token edit and not another sweep.
- A guard test keeps off-grid spacing from creeping back in.
- The dev overlay makes alignment checkable on any page at any time.

**Non-Goals:**
- No change to type sizes, colours, the widget schema or any plugin.
- No drawn grid for operators. The overlay is dev-only.
- No change to the 20px text line or the 40px row: both already land on the grid.

## Decisions

### 1. Name the layout roles as theme tokens

Add Tailwind v4 spacing tokens in `@theme`, so the utilities read by role. Tailwind derives `p-inset`, `gap-inset`, `mt-inset` and so on from each token.

| Token | Value | Role |
|---|---|---|
| `--spacing-control` | 2rem (32) | every field and button (`h-control`, existing name) |
| `--spacing-inset` | 1.5rem (24) | card inset, card gap, field air, a group's step |
| `--spacing-frame` | 2.5rem (40) | page frame, drawer inset, a block's step in a drawer |
| `--spacing-act` | 1.75rem (28) | row and subsection acts |

`sections.css` and `forms.css` switch from `calc(var(--spacing) * 5)` to `var(--spacing-inset)`. Templates switch from `mt-5`/`p-5` to `mt-inset`/`p-inset` where the value means the role, and to plain 8px multiples (`gap-4`, `mt-6`) where it doesn't.

**Alternative considered:** a bare value sweep (`5` → `6` everywhere). It is cheaper, but the next grid change would mean a third sweep, and nothing would say why a 24 is a 24. Rejected for durability.

### 2. Optical padding inside a component lives in named utilities

A row's 10px/9px around its 20px line, a header's 14px/13px, and a strip's 16px/15px are component internals that centre text, so they are exempt from the 4px step. Each gets one `@utility` in `input.css` (`py-row`, `py-card-head`, `py-strip`, `py-colhead`), and templates use the name. This also removes most of the 400 fractional utilities from the templates.

### 3. Guard test against fractional spacing

A Go test in `internal/widget` walks every embedded template (widget and server) and every `assets/*.js`. It fails on two kinds of utility outside an explicit allowlist of component internals, which names each exception with its reason:

- **Fractional spacing** (`-N.25`, `-N.5`, `-N.75` on padding, margin, gap, inset, size, height or width). These are off the 4px step.
- **The retired cell as air** (`*-5`, 20px, on padding, margin, gap or inset). Air between boxes is a role token now.

A 20px height or size (`h-5`, `size-5`) stays legal: it is a text line or an icon box.

The test is source-level. A layout-level check would need a browser, and the dev overlay covers that by eye.

**Alternative considered:** a Playwright pass measuring boxes. It is heavier, slower and flaky against live data. Rejected; the overlay plus the source guard cover it.

### 4. Measures and rounding

- **Page measures:** containers become 1136, 1016, 880 and 760px. `--container-form-split` stays 1000.
- **Rounding:** every `round(down, …, 1.25rem | 20px)` becomes `round(down, …, 0.5rem)`.
- **Phone gutter:** 20px → 16px.
- **Overview:** columns, cards and the shelf round to 8px. Its gutter stays 40px (two 20px cells today, which is also a multiple of 8).

### 5. Charts

`verso-chart.js` swaps its 20px `GRID` for 16px. `chart.html.tmpl`'s plot heights become `h-48!` (192: card and short) and `h-64!` (256: full). The chart's `H` and the scale's four steps follow from those heights, so the labels land on every fourth rule. The four-step scale (DESIGN.md's One Rate Rule) is unchanged.

### 6. The dev overlay

`verso-dev.js` gets the G handler back:

- It ignores the key in inputs, selects, textareas and contenteditable, and with any modifier.
- It toggles `data-verso-grid` on `<html>` and keeps the choice in `sessionStorage` (inside try/catch), so a hot reload or a redeploy reload keeps it.

`input.css` gets one rule set keyed on that attribute:

- a `::after` overlay on `main`, positioned from its top-left (the top bar's hairline and the rail's border), and one on the drawer body, positioned from the chrome's hairline
- a 1px Denim line at 22% every 8px, `pointer-events: none`

The rule ships in the stylesheet: about 300 bytes, inert without the attribute. Only the dev script ever sets the attribute, so a real deployment never draws it (spec: layout-grid).

**Alternative considered:** serving the overlay CSS only in dev. That means a second stylesheet path for a few hundred bytes, and LAN bytes are free. Rejected.

### 7. Order of work

The overlay comes first, so every later step can be reviewed with G on. Then tokens and shell frame, then cards and listings, forms, the Overview and charts, drawers, and the remaining surfaces (live pages, sign-in, takeovers, staged review, colophon, rail). Each step is its own commit, reviewed in the browser through `make dev`.

### 8. Docs follow what is drawn

- **DESIGN.md:** "The 20px cell" becomes "The 8px grid"; the component measures and the frontmatter tokens change with it. The stale passages left by commit 71ab775 are corrected in the same pass: lanes as bands, column heads on Paper, no ledger count.
- **page-cards spec:** follows through this change's delta.

## Risks / Trade-offs

- [The sweep touches about 50 templates, and a missed spot lands off the grid] → The guard test catches fractional and odd spacing in source. The G overlay on each page catches the rest by eye, and the task list walks every surface.
- [Go tests assert class strings, so about 200 test lines change with the templates] → Tests change first (test-first), in the same commit as their templates, so the commit's diff shows both sides.
- [Smaller checkboxes (16px) and controls (32px) shrink hit areas on touch] → 32px controls stay above WCAG 2.2's 24px target minimum. The checkbox's label stays part of its hit area, as it is now.
- [Charts grow from 160/180/240 to 192/192/256 and push content down] → This was accepted with the preview artifact. The Overview Traffic card grows 32px.
- [A plugin page that relies on a shell spacing quirk shifts] → Plugins carry no classes. The shell draws every widget, so a plugin page shifts exactly as a shell page does.

## Migration Plan

No on-device state changes, so no migration ships. The change ships in the next Verso release as a shell build. Rolling back means installing the previous Verso package. Nothing persists that the older build cannot read.
