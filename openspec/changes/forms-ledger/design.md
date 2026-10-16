# Design

## Context

Every labelled row goes through one Go builder, `fieldFrame` (`internal/widget/field_frame.go`), and one partial, `verso-field` (`shared.html.tmpl`). Each widget's frame builder decides today what becomes a tooltip and what a visible line: `Field` raises its `help` (or its `tip`, which then drops the help) onto the label as `verso-tip-wide`; only `note` is drawn, inside the control column; a grouped switch draws its help; `SettingsItem` draws its `desc`. The partial has two branches, a stacked one (label over control) and a toggle one (checkbox before label), and two competing grids in `forms.css` (`.verso-field-toggle`, `.verso-field-with-remove`). Read-only settings rows (`data-verso-settings-row`) bypass the frame and already lay out title left, value right.

Rows are capped by `--verso-form-width` (761px on a page, 641px in a dialog/drawer); a page card insets 23px (`--verso-side`). The 8px grid's band rule puts text on whole 24px bands with air after a drawn edge a pixel short (`py-row` 7/8). `grid_guard_test.go` refuses off-grid air outside its named exceptions.

The canvas the owner approved (https://claude.ai/artifact/YPqMsKwyRg1VzF9qGgZaaQ) is the visual reference: 272px answer column, 24px gap, rows 15/24/16, hairlines border to border, header seam in the frame's ink, a quiet "Hide descriptions" act on the heading line.

## Goals / Non-Goals

**Goals:**
- One partial, one grid, for every ledger row; every widget that frames a row gets the ledger by rendering through it.
- Descriptions are data the shell owns end to end: what is a description, where it stands, how it folds.
- The fold paints right on first load and follows rows that htmx swaps in later (reshapes, drawers, refusals).

**Non-Goals:**
- No new widget and no schema field; no plugin rebuild (ADR-005 §1).
- Listing control bands, the sign-in page, inline/search forms, choosers without a Save, the diagnostics tool row and dialog file drops keep their layout.
- Table toggle cells keep their bare box; the On/Off word belongs to a form row only.
- No router-side preference: the fold is per browser.

## Decisions

### 1. One frame: a question and an answer

`verso-field` renders one shape for every row:

```
.verso-field-row            container (inline-size), padding, hairline, change hooks
  .verso-field-ledger       the grid: [question] 1fr  [answer] 17rem  [lane] auto
    .verso-field-question   label line (name · chip · staged), then the description
    .verso-field-answer     data-verso-control: control, refusal band, note
    lane                    the remove glyph, when a row is removable
```

The toggle branch disappears: a switch is an answer like any other (its box and word in the answer column), so `.verso-field-toggle` and `.verso-field-with-remove` collapse into the one grid. The row stays the container so its children can query its width; the grid is the inner element because a container cannot query itself. Class names a script reads (`.verso-field-row`, `.verso-field-label`, `[data-verso-control]`) keep their meaning, so `verso-commit.js` (staged mark), the refusal navigator in `verso-forms.js` and the reshape wait need no change beyond the answer wrapper carrying `data-verso-control`.

*Alternative considered:* keep two branches and restyle each. Rejected: the second branch is the source of the two left edges the change exists to remove.

### 2. Geometry

- Tokens in `input.css` `@theme`: `--spacing-answer: 17rem` (272px, 34 × 8). The 24px gap is `--spacing-inset`.
- A row's air is 15px after its hairline and 16px before the next edge, a `py-field` utility beside `py-row` and named in `grid_guard_test.go`'s exceptions the same way, so a one-line row pitches at 56px.
- The answer centres on the name's band: the answer wrapper pulls 4px up and down (`margin-block: -4px`), so a 32px control and its value share the name's 24px line. A checkbox, a radio row (32px) and the On/Off word land on the same line by the same rule.
- In a card the rows bleed to the frame: `margin-inline: calc(-1 * var(--verso-side))` with the side inset as their own padding, so hairlines touch both borders and words keep the gutter. A card body whose first or last block is a run of rows keeps 8px over and under the run, so the run's first and last words stand 24px from the frame; `sections.css` owns this as it owns every other first/last give-back. In a drawer rows run the content column, with no bleed.
- The header seam of every card (page cards and Overview cards) is `border-rule-strong`.

### 3. When an answer is wide, and when a row stacks

A frame carries `Wide bool`, and each widget decides it, because each knows its control (open for extension, closed for modification):

| Widget / kind | Wide when |
|---|---|
| `Field` | style `code`, kind `textarea`, measure `endpoint`, `checks` segmented |
| `List` | always (rows, tokens) |
| fused / joined `Grid` | the parts' fixed measures plus 40px per joiner exceed 272px, or a part has no fixed measure |
| everything else | never |

A wide row puts its answer under the question across both columns (`grid-column: 1 / -1`). Below 36rem of row width (`@container verso-field (width < 36rem)`), every row stacks the same way, the answer straight under the question with no gap, and selects and free-text fields fill the row. In the answer column a select and a free-text field fill the column; a field with its own measure (number, short, prefix, unit box, time) keeps it; `host` and `secret` cap at the column.

*Alternative considered:* let CSS decide width from the control's intrinsic size. Rejected: no CSS lets a row change its own grid from a child's min-content width without a reflow loop, and the widgets already know their measures (`ControlMeasure`, part measures).

### 4. Descriptions

- `Field.description()` is the help, then the tip, joined by a space; a fused group's description is its own help, then each part's explanation led by the part's label. A switch's, a list's, a gate's and a settings item's help or desc become the description the same way. The description renders in the question as `<p id="…-desc" data-verso-field-desc>` and is the control's `aria-describedby` target in place of `-tip` (`list.html.tmpl`'s hard-coded `-tip` included).
- `source` joins the key chip: `mac · dhcp host`, the middot form fused chips already use.
- The field label's tooltip partial (`verso-field-label-text`'s `verso-tip-wide`, `data-verso-tip`, `cursor-help`) is removed from the label. The tooltip machinery stays for its other users.
- A grouped item's help stays an item detail (`data-verso-field-detail`), not a description: it identifies the item and never folds.
- `note` renders in the answer under the control as `data-verso-field-note`; it never folds.

### 5. The fold

- The shell reads a cookie, `verso_descriptions`, whose only honoured value is `folded`, and renders `<html data-verso-descriptions="folded">` (`page.html.tmpl`), so the first paint is already folded and a drawer htmx swaps in inherits it. The page writes the cookie; the shell only reads it, as ADR-022's Overview layout cookie does (ADR-015 §1 named the same mechanism). `Path=/`, a year, `SameSite=Strict`.
- CSS folds: `[data-verso-field-desc]` is a one-track grid (`1fr`) whose text wraps in an inner span with `min-height: 0; overflow: hidden`; under the root attribute the track is `0fr` and the text `opacity: 0`, both on a 200ms transition, none under `prefers-reduced-motion`. A folded description stays in the accessibility tree, so `aria-describedby` still reads it.
- The act is shell markup on every page heading line and in every drawer header: a quiet button holding both states, each its Lucide glyph (`fold-vertical` before "Hide descriptions", `unfold-vertical` before "Show descriptions") and its words, the root attribute showing one. It is the one quiet heading act that wears a glyph (the owner's call); the eye stays the password reveal's alone. CSS shows the act only where it has something to fold (`:has([data-verso-field-desc])` scoped to the page body or the drawer), so a reshape or a swapped drawer is right without a server flag. `verso-page.js` flips the root attribute and writes the cookie on click; nothing posts.

*Alternatives considered:* `localStorage` set by `verso-boot.js` before paint (works, but the shell could not render the right state for a request, and a second mechanism beside the cookie precedent); a server-side "page has descriptions" flag (cannot follow htmx swaps; a render handle is copied per section, so the flag would need a shared pointer for no gain over `:has`).

### 6. Switches say On or Off

`switch.row` (the form row's switch, not `switch.control`, which table cells share) renders the box, then both words translated server-side, `.verso-switch-on` and `.verso-switch-off`; `label:has(input:checked)` shows one. The label wraps box and words, and the row's name points at the same input, so all three flip it with no script.

### 7. Groups, gates and related fields

- A gate's revealed rows and a switch group's items draw no hairline and no top air of their own (`.verso-conditional-body > .verso-field-row`, `.verso-switch-group > .verso-field-row`); their question indents 24px per level by nesting selectors (two levels exist).
- A labelled set of typed fields already fuses into one `verso-box` (`Grid.fusable`), DNS's "Port and ranking" among them; until now its parts were named only in the label's tooltip (`Tip.Parts`) and by `aria-label`. With the tooltip gone, a named part (`boxPart.Named`) shows its name in the slot a unit uses (`verso-box-unit`), and both are set sans, since a unit and a part's name are words. A named part's measure holds its longest value and its name, so the box's width is known in Go and decides answer column or full width (§3).
- A labelled plain form grid that cannot fuse (it holds a dropdown that is not a unit, a list or a switch) stops laying fields in columns: its label is a group head and each cell is a ledger row. `data-verso-columns` keeps working for non-field cells.
- The fuse rule widens to a select part. Today an unlabelled grid of typed fields with no join fuses into one `verso-box` (`grid.go`); a grid of a typed field and a select (DDNS's `check_interval` and `check_unit`) fuses the same way, the select drawn as a box part with its chevron, split by the box's inner hairline as the new-password pair is. The first field's label is the question, the chip names both keys, and the select part takes its own label ("Unit") as its accessible name. The value part keeps the number measure (96px) and the unit part its longest unit, so the box stays inside the answer column. This is the shell reading the schema it already gets: the DDNS plugin's own comment asks for exactly this pairing, and no plugin changes.
- Read-only settings rows render through the same grid: title and description in the question, value or pills in the answer.
- A table among a form's fields (the rule-file and dnsmasq option-file listings) renders as a ledger row: its group label is the question, its add act stands at the row's right end on the question's line (the row's grid is `1fr auto` at every width, so the act ends where the rows' edit glyphs end and never drops under the label on a phone), its rows a wide answer. The render handle carries whether it stands inside a form (set by `Form` and kept by `nested()`, as `depth` is), and the table chooses its in-form shape from it, so a page listing outside a form is untouched. Its lines (under the head, between rows) are inset to the gutter and its band fill is dropped: a line that touches a card's borders always separates questions, and nothing else does. The head's line stands 8px under the head's text, so each 40px table row keeps its own line on the grid.

  *Alternative considered:* a table and the fields beside it in separate cards. Rejected: the shell would invent an untitled card the plugin never asked for, and the files and the switches that load them are one subject.

### 8. What stays outside

Compact forms (`data-verso-compact-form`: inline, search), forms with neither a submit nor actions (choosers such as Hardware's "Show chart for"), the sign-in template, the diagnostics tool row and `file` fields keep their markup; the frame is not involved for them or a `.verso-form-plain` scope resets the ledger grid. The staged-review note and the logs filter checkbox stay hand-drawn.

## Risks / Trade-offs

- [Forms grow taller with descriptions shown] → the fold, kept per browser from the first paint; 16px air keeps a one-line row at 56px, the old field's 80px less.
- [Help copy written for a tooltip reads long on the page, notably in the plugins in their own repos] → BACKLOG item to review help strings in ddns, dnsdhcp, qos, vpn and wireless; nothing breaks meanwhile.
- [A long URL or hostname in a 272px column scrolls inside its field] → accepted for free text; a field that declares the `endpoint` measure stands wide.
- [A dropdown's longest option truncates in the closed control] → the open list shows every option in full; the column width is the trade for one answer edge.
- [Controls beside their names stand 4px off the 8px lines] → deliberate: words share one baseline; `layout-grid` states the exception and the audit checks text bands, not those borders.
- [Snapshot-style tests pin exact class strings (`verso-field-row max-w-form verso-field-toggle`, `-tip` ids)] → rewritten test-first per task; the frame's public hooks are kept so scripts do not churn.
- [`:has()` in the act's visibility] → Verso already relies on `:has()` for conditionals; every supported browser has it.

## Migration Plan

No router state changes: no UCI option, file or flag is added, renamed or moved, so there is no migration and a downgrade restores the stacked forms. The cookie is per browser and ignored by older shells. Release notes: forms are ledgers, descriptions are on the page, "Hide descriptions" folds them, and installed plugins need no upgrade.
