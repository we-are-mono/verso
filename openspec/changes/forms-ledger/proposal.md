# Proposal

## Why

A settings card today mixes two shapes of row: a field puts its name over its control, a checkbox puts its box before its name, and a radio set draws its options exactly as checkbox rows are drawn, so "accept" reads as a setting of its own. The names start on two different edges, every control hugs the card's left third, and what a setting does is hidden in a tooltip on its name. After this change every form reads as a ledger: the question on the left with what it does written under it, the answer in one column on the right, a hairline between rows. Someone who knows the settings folds every description away with one act, and the browser remembers it.

## What Changes

- Every form field row (page forms, drawers, a settings face, a modal wide enough) becomes a ledger row: on the left the field's name, its option chip and its staged mark, then its description; on the right its control, in one 272px answer column. A row's name writes on a 24px line 15px under the hairline above it and the row ends 16px under its last line, so a one-line row is 56px with its hairline; a card keeps 8px over its first row and under its last, so they stand 24px from its frame. A 32px control centres on the name's line, so every word on the row shares one baseline.
- Rows are separated by hairlines running the full width of what holds them: a card from border to border, a drawer from content edge to content edge. Words keep the 24px gutter. A revealed field (a switch's gate) indents its name 24px and draws no hairline above it; a group of switches (which files load) stands under its group's name with no hairline between its items.
- In the answer column a select fills the column, a short field keeps its own measure, radio choices run across one line, and a switch is its box with the word "On" or "Off" beside it. An answer wider than the column (tokens, rows of values, a code or text area, a weekday strip, a fused or joined value wider than the column, an address of the endpoint measure) stands under its question at the row's full width. A row narrower than 576px stacks: the name, its description, then the control straight under them.
- Help is shown, not raised: a field's `help`, then its `tip`, write as its description under its name; what reads the option (`source`) joins the key chip after a middot. The tooltip on a field's name is gone, and help is no longer dropped when a tip is set. A fused box names each of its parts inside the box, where a unit stands, so "5060 port │ 10 priority │ 5 weight" reads without a tooltip; a value and its unit dropdown fuse the same way; a part's name and a unit are words, set sans. A note stays under the control, in the answer column; a refusal band stands under the control it refuses.
- One act, "Hide descriptions" / "Show descriptions", stands on the heading line of every page and in the header of every drawer that shows a description. It folds every description on the surface away together, and back, and the browser keeps the choice for every page and drawer it opens. Notes, refusals, staged marks and the detail of an item in a group never fold.
- The hairline under every card's header, on every page and the Overview, takes the card frame's ink (Strong Hairline), so a card's frame reads as one ink and the rows inside it as the lighter one.
- Forms that are not a list of settings keep their own layout: sign-in, inline and search forms, a chooser with no Save (a graph's own choices), listing control bands, the diagnostics tool row and the file drop of a dialog.
- No plugin rebuild and no schema change: the layout comes from the shell's rendering of the existing widget schema (ADR-005 §1). Plugins in their own repos render as ledgers on upgrade.
- **BREAKING (visible):** every form looks different, and help that was only on hover is now written on the page, so forms are taller until an operator hides descriptions. Nothing on the router changes, so there is no migration.

## Capabilities

### New Capabilities

- `form-fields`: how a form's fields are laid out and what each row shows, its question, its description, its answer, its note, its refusal and its staged mark; which answers stand at full width; how revealed fields and groups nest; when a row stacks; and the per-browser act that folds descriptions.

### Modified Capabilities

- `layout-grid`: a field's control centres on its name's 24px line beside it, so its top and bottom borders stand 4px inside the grid lines rather than on them; a field row is 56px from hairline to hairline, and a stacked field keeps its label's line with the control straight under it.
- `page-cards`: the hairline under a card's header is a Strong Hairline, the frame's own ink.

## Impact

- **No ADR is written or amended**: this is a design decision, and DESIGN.md is its contract. It rests on **ADR-005 §1** (props are semantic, so the layout is the shell's alone and no plugin changes) and follows **ADR-015 §1** and **ADR-022** for a per-browser cookie the shell reads before rendering. Folding descriptions hides prose, never a setting, so it is not a reader mode and ADR-015 stays deferred.
- **DESIGN.md**: the Inputs and Fields anatomy, layout, spacing, related fields and width-by-content entries rewritten for the ledger, with the reasons the ledger was chosen; the 760px measure note; card anatomy's header hairline.
- Shell: `internal/widget` field frame (`field_frame.go`, `field.go`, `switch.go`, `list.go`, `conditional.go`, `grid.go`, `settings.go`) and their templates (`shared.html.tmpl`, `field.html.tmpl`, `form_switch.html.tmpl`, `conditional.html.tmpl`, `settings.html.tmpl`, `grid.html.tmpl`, `list.html.tmpl`); `forms.css`, `sections.css`, `input.css`; the page and drawer chrome for the fold act; a cookie read in `internal/server`; `verso-forms.js` / `verso-page.js` for the fold, and the tooltip placement code losing its field case.
- SDK crate (`plugins/verso-plugin-sdk`): doc comments on `help`, `tip`, `note`, `source` and the conditional's help say what is shown; no API change. `docs/plugins.md` agrees.
- Tests that pin the stacked frame and the field tooltip (`field_frame_test.go`, `field_test.go`, `field_a11y_test.go`, `switch_test.go`, `switch_group_test.go`, `settings_test.go`, `assets_test.go`, `grid_guard_test.go` and others) are rewritten to the ledger.
- i18n: "Hide descriptions", "Show descriptions", "On" and "Off" in the Slovenian catalog.
- Plugins in their own repos (ddns, dnsdhcp, qos, vpn, wireless) need no change; their help strings, written for a tooltip, now read on the page.
