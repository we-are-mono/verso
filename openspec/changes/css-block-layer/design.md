# Design

## Context

The motivation and the measurements are in `proposal.md`. This section covers what shapes the approach.

- **The stylesheet today.**
  - `input.css` (1,667 lines) is the Tailwind v4 entry. It imports `palette.css`, `forms.css` (583 lines) and `sections.css` (694 lines).
  - Rules are spread across `@layer base`, `components` and `utilities`, plus unlayered rules.
  - The section kinds live in `@layer utilities` so they outrank the page stack's rhythm and Tailwind's padding utilities, and the card rules are unlayered so they outrank the section kinds. Placement in a layer is being used to win specificity fights.
- **About 150 `verso-*` classes in roughly 40 families**, in three naming habits:
  - single-dash parts: `verso-chart-line`, `verso-field-label`;
  - `--` modifiers: `verso-chart--denim`, `verso-ports--sky`;
  - numbered series: `verso-narrow-2` … `-16`, and `verso-nav-sub-1` … `-11`, which are view-transition class names, not element classes.
  - Alongside them, 77 `data-verso-*` attributes are selected in CSS. Some are behaviour hooks the scripts also use, some are states, and some are style hooks standing in for a missing name.
- **The renderer already knows roles.** `sectionView.Kind()` (`internal/widget/section.go`) computes a section's kind, the renderer handle carries `depth`, `headed` and `inForm`, and `widget.OnCards` wraps a drawer's loose widgets in a headless ruled section. The stylesheet re-derives the same facts from structure.
- **Rendering surfaces.** Section trees render from 17 `RenderWithToken` call sites, and the output lands in a page body, a drawer body, or a fragment swapped into one.
  - The choices drawer (`shared.html.tmpl:685`) renders sections as list groups and styles them with `[&>section]…` utilities.
  - The Overview card body renders inside its own card.
  - Login and the takeovers (restarting, upgrading, firmware-started) are full pages with no dev route.
- **Client scripts build markup.** `verso-tables.js`, `verso-stream.js` and others write rows "in exactly the treatments the templates use". Tailwind scans `verso*.js` so those utilities compile.
- **Typography.** DESIGN.md › Typography › Hierarchy defines ten roles, and other sections define more inline: row label 14/400 Meta on 24, field label 14/500, chip 14/400 on 16 (mono 14/500), button 14/600, page-card column head 14/600 on 16, editor path mono 14/500.
  - The markup writes these as loose utilities: 74 distinct combinations over 252 elements, 43 used once.
  - `@theme` overrides only `--text-sm--line-height` and the two families.
  - The renderer already carries what the face depends on: `Mono`, `Duration` and `Chip` on property and cell views, which templates turn into `font-mono text-base font-medium` and `tabular-nums`.
- **Plugins.** Plugins send the widget schema and never classes. The raw bridge's Markdown output is sanitised and carries no shell classes. `fieldFrame.Class` is a shell-internal hook, not a plugin field.

## Goals / Non-Goals

**Goals:**
- One contract for every look, now and later: three layers, one naming scheme, one file per block, utilities outranking blocks, blocks adjusting each other only through custom properties.
- Every look in the shell converted to that contract, and every rule that infers a role from structure removed.
- Guards that fail the build on each way the contract can be broken, so future work follows it without anyone having to remember it.
- Type from a closed set of roles that DESIGN.md and the code name the same way, with a value's face decided by its content.
- Geometry identical to the pixel on every surface, proven per conversion, except where drifted copies of a look converge on their block or role. Each convergence is recorded in `drift.md`.

**Non-Goals:**
- **Any visual change beyond convergence.** Copies of a look that drifted apart become identical once they share a block or a role, and that is intended. When a block's copies differ, the block takes the form DESIGN.md specifies, or the most widely used form where DESIGN.md is silent. Every convergence is recorded in `drift.md` (elements, difference, before/after screenshot). Any other geometry difference is a defect of this change. Visual improvements found along the way go to `docs/BACKLOG.md`.
- **Parent-owned spacing (`gap`) in place of the rhythm and row air.** A field row carries its own 15/16px air and the `form-fields` contract measures from it. Re-deriving the rhythm as `gap` changes the measuring model, not where it is defined.
- **Renaming behaviour hooks.** A `data-verso-*` attribute that scripts use stays as it is. It may also serve as a block's state attribute where it is one.
- **Plugins, the SDK and the widget schema.** Untouched.

## Decisions

### 1. Three layers (CUBE CSS), and only the entry outside `blocks/`

```
assets/
  palette.css           tokens              --color-*, --shadow-*
  type.css              tokens              @utility type-<role>, and each role's
                                            verbatim and figures forms
  input.css             entry               @import, @source, @font-face, @theme,
                                            @utility, bare-element base rules
  blocks/<block>.css    @layer components   every rule naming verso-* or data-verso-*
  (Tailwind)            @layer utilities    in markup, outranks every block
```

- Tailwind v4 declares `@layer theme, base, components, utilities`. Blocks sit in `components`, so a utility in markup wins by layer order and a one-off never needs a stronger selector.
- `forms.css` and `sections.css` dissolve into blocks.
- The `@layer utilities` blocks of hand-written rules in `input.css` (the press treatment, the rail mark, …) become blocks too. The layer is left to Tailwind and `@utility` definitions.
- *Alternative: BEM only, no utilities.* Rejected. Utilities stay the right tool for layout glue and breakpoints, and banning them adds ceremony without making anything more consistent.
- *Alternative: more class-string templates.* Rejected. They name a string, not a thing, and leave the stylesheet inferring structure.

### 2. Naming: BEM blocks and elements; variants and states as `data-verso-*`

| Kind | Form | Example |
|---|---|---|
| block | `verso-<block>` | `verso-card`, `verso-field` |
| element | `verso-<block>__<element>` | `verso-card__head`, `verso-field__answer` |
| variant / state | `data-verso-<name>[="value"]` on the root | `data-verso-headless`, `data-verso-tone="denim"` |
| mix | two blocks on one element | `class="verso-card verso-overview-card"` |

- **Why attributes for variants:** the client toggles attributes already, CSP Alpine binds a getter to an attribute without a ternary, and a variant with a value (`tone="denim"`, `narrow="2 5"`) has no clean class form.
- **Numbered series** become attribute values: `verso-narrow-N` becomes `data-verso-narrow~="N"` on the table. View-transition names (`verso-nav-sub-N`) are not element classes and are outside the naming rule.
- **The `verso-` prefix stays.** Tailwind owns `table`, `grid`, `container`, `hidden`, `contents`, `flex`.

### 3. Block inventory

From the families in the stylesheets, the 21 class-string templates and the 47 repeated bundles. Each row is one block file. The conversion may split or merge rows when the markup shows two looks or one, and records that in this table.

| Group | Blocks (today's families) |
|---|---|
| Compositions | `rhythm` (verso-rhythm), `stack` (verso-stack), `cluster` (the repeated `flex flex-wrap items-center gap-2 min-w-0`), `page` (page-body, -full, -ground, -heading), `section` (data-verso-section kinds other than card, section band, ledger, kicker, lede), `drawer` (drawer, -motion, -open, -scrollbar, drawer ground), `beside` (data-verso-beside), `cards-row` (data-verso-cards) |
| Card | `card`, `overview-card` (overview-card, -body, -content, -chevron, -summary, -slot), `overview` (overview, -cols, -col, -meters, -dragging) |
| Forms | `form` (form, form-grid), `field` (field-row, -label, -question, -answer, -ledger, -control, -act, -wide, -plain, -set; verso-field-*-class templates), `input` (verso-box family, verso-input-class, time-input), `select` (box-select, box-chevron, the repeated chevron overlay), `choice` (switch, inline-switch, checkbox and radio rows), `conditional` (conditional, -gate, -body, -otherwise), `condition` (condition, -list, -name, -single), `joined` (joined, -part, -word), `key-chip` |
| Controls | `button` (button, button-waiting, press, menu-button, verso-btn-icon), `link` (the repeated underline link), `chip` (chip, verso-chip-class), `tip` (tip-look, tip-class, copy-tip), `menu` (suggest, token and condition menus), `tabs` (drawer and entity-panel tab strips), `disclosure` (disclosure, fold, unfold, reveal, chevron), `copy` (copy-inline) |
| Data display | `table` (table, -seam-*, narrow, row-name), `list`, `facts` (properties), `meter` (meter-bar, threshold marks), `chart` (chart family), `ports` (ports and port families), `panel` (panel art, chassis, face), `code` (config preview, preview-busy), `prose`, `empty` (empty, empty-icon, empty-slot), `notice` (callout, flash, staged-bar, outcome), `mark` (the 6px state square), `waiting` (the four-square waiting mark), `live` (live-dot), `step` (step, step-mark) |
| Chrome | `nav` (nav-row, -branch, -sub), `rail` (rail-link), `actionbar`, `colophon`, `modal` (modal, -motion, dialog box), `pending` (pending-link), `stream` (stream-release, -shelf, pluck), `console` (console, -notice, vt), `scroll-edge`, `takeover` (login, restarting, upgrading, firmware-started), `glyph` (glyph-fan-spin), `arrival` (arrived, landed) |

### 4. Guards, and a ratchet so every commit is green

The guards live in `internal/server/assets_test.go`. They read the sources: `assets/*.css`, `assets/blocks/*.css`, `widget/templates/*.tmpl`, `server/templates/*.tmpl`, `assets/verso*.js`, plus the Go files that write class strings.

| Guard | Fails when |
|---|---|
| `TestRulesLiveInBlocks` | a rule outside `blocks/` and `type.css` selects a `verso-*` class or `data-verso-*` attribute (`type.css` may select only `data-verso-verbatim` and `data-verso-figures`) |
| `TestABlockStartsFromItself` | a selector's first compound in `blocks/<b>.css` is not `.verso-<b>` or `.verso-<b>__*` |
| `TestCrossBlockRulesSetOnlyProperties` | a rule reaching `.verso-<other>` declares anything but `--*` |
| `TestBlockNames` | a `verso-*` class has a form other than block, `__`element, or listed view-transition name |
| `TestNoRepeatedBundles` | the same set of ≥4 utilities (template actions stripped, order ignored) is on two or more elements |
| `TestShortClassLists` | an element carries more than 8 utilities |
| `TestNoRulesInMarkup` | markup writes a `[&…]` arbitrary variant or a `!` utility |
| `TestNoClassStringTemplates` | a `{{define}}`'s whole body is a class list |
| `TestBlockClassesDefinedAndUsed` | a written `verso-*` class has no rule, or a ruled one is never written |
| `TestTypeComesFromRoles` | markup writes `text-{xs…3xl,[…]}`, `font-{weight}`, `font-mono/sans`, `leading-*`, `tracking-*`, or a file other than `type.css` declares `font-size`, `font-weight`, `font-family`, `line-height`, `letter-spacing` |
| `TestTypeRolesMatchDesign` | a `type-*` utility in `type.css` is not listed by class in DESIGN.md › Typography › Hierarchy, or a listed one is not defined |

- **The ratchet.** `internal/server/testdata/stylesheet-ratchet.txt` lists today's violations, one per line, as `guard path selector-or-element`, generated once in task 2.2.
  - A guard fails on any violation not on the list, and also on a listed line that no longer occurs, so the list can only shrink and never goes stale.
  - Each block's conversion deletes its lines.
  - The last group asserts the list is empty and deletes the file and the code that reads it.
- **Thresholds.** The ≥4 bundle size and the 8-utility cap come from today's distribution:
  - 789 of 1,075 class attributes carry five or fewer utilities;
  - 127 carry nine or more, and every one of those is a look without a name.
  - A template action (`{{if}}…{{end}}`) inside a class attribute counts as one token, so conditional classes don't evade the cap.
- *Alternative: a near-duplicate (similarity) detector.* Rejected. It is noisy, and the 8-cap combined with the exact-bundle guard already pushes every substantial look into a block.

### 5. The renderer marks roles; a card is a section kind

- The renderer handle gains `cards bool`, the surface it renders onto stands its top-level sections on cards.
  - It is true from `RenderWithToken`, since every call site's output lands in a page or drawer body (today's stylesheet scope).
  - It is carried by `nested` and `amongFields`, and turned off for the choices drawer's and the Overview card body's contents.
- `sectionView.Kind()` returns `card` when `cards && depth == 0` and the section has a band or is headless and ruled. The template writes `data-verso-section="card" class="verso-card"`, `verso-card__head` on the band, and `verso-card__body` on the rhythm.
- **Why a kind and not only a class:** a card stops matching every non-card section rule, so no override or `:not(.verso-card)` exception exists anywhere.
- **The same principle applies across the board.** Wherever a block rule today tests ancestors or children to learn a role, the template or renderer writes the role:
  - a part is `data-verso-section="part"`;
  - a listing that opens a card body carries an attribute on that body;
  - a stacked row carries its block element.
  - Only browser-side state stays read from the document: `:checked`, `[open]`, `[aria-expanded]`, `:hover`, `:focus-visible`.
- *Alternative: a short contextual selector (`.verso-page-body .verso-card`).* Rejected. It is the same inference, shorter, and it breaks `TestABlockStartsFromItself`.

### 6. Blocks adjust each other through custom properties

- A block declares each measure a container may turn as a custom property read with its default. For example, the field row:
  - `padding-inline: var(--verso-field-bleed, 0)`;
  - `margin-inline: calc(-1 * var(--verso-field-bleed, 0))`;
  - `padding-top: var(--verso-field-lead, calc(var(--spacing) * 3.75))`.
- A container's rule sets the property only (`.verso-card__body > … > .verso-field { --verso-field-lead: … }`).
- Exactly one rule sets each property on a given element, so layer and specificity stop mattering.
- **Properties don't inherit.** Each property is registered with `@property --verso-field-lead { syntax: "<length>"; inherits: false; initial-value: … }`. A value set on one row never leaks into a row nested inside it.
  - Where a container hands its edge down to the content that meets it, the handoff is explicit: the card sets `--verso-edge-trail` on its last visible child, and that child's block (a form, a field set, a conditional) forwards it to its own last visible child, down to the row that reads it.
  - `@property` is within Tailwind v4's own browser floor (Chrome 111, Safari 16.4, Firefox 128), which Tailwind already uses for its own variables, so the baseline does not move.
- **Precedent:** `--verso-reach-start` / `--verso-reach-end` and `--verso-rhythm` already work this way.
- Each block file's header lists the properties it reads, with defaults, and the properties it sets on other blocks.

### 7. Visible-edge selection with `:nth-child(1 of S)`

- **The patterns today:**
  - first visible: `:not(X):not(:not(X) ~ *)`
  - last visible: `:not(X):not(:has(~ :not(X)))`
- **The replacement:** with `X` = `input[type="hidden"], [hidden], template`, they become `:nth-child(1 of :not(X))` and `:nth-last-child(1 of :not(X))`.
- **Browser support.** Selectors Level 4 `of S` (Chrome 111, Safari 9, Firefox 113) is inside Tailwind v4's floor, so the baseline does not move.
- **Conditionals.** Whether a conditional's body or its `otherwise` branch shows is browser-side state (`:checked`), so the conditional block reads it. The card never tests the switch. The conditional block forwards the edge it receives (Decision 6) to the last row of whichever branch shows.
- **Lightning CSS.** Task 1.3 confirms Lightning CSS emits the `of S` form unchanged before anything relies on it. If it does not, the fallback is today's form inside the block file, which keeps every other decision intact.

### 8. `@apply` policy

- A block is written in plain CSS over tokens (`var(--color-rule-strong)`, `var(--spacing-inset)`), as the existing rules are.
- `@apply` is used where it is shorter than the plain rule, which in practice means variant bundles (`hover:`, `focus-visible:`, `group-data-*`).
- Tailwind compiles the imported block files in the same build, so `@apply` works without `@reference`.

### 9. Client scripts name blocks

- A script that builds markup writes block and element classes and sets `data-verso-*` state. It never writes a look's utilities.
- Tailwind keeps scanning `verso*.js` for the layout utilities scripts still write.
- `TestNoRepeatedBundles` and `TestShortClassLists` read the scripts' class strings (`className`, `class="…"`, `classList.add`) along with the templates, so a script and a template can't drift apart.

### 10. Geometry parity

- `scripts/layout-snapshot.cjs`, run by `make layout-snapshot LAYOUT_OUT=<dir>`, `make layout-diff FROM= TO=` and `make layout-noise FROM= TO=`.
  - It drives the pinned `playwright-core` (fetched into `build/tools` by SHA-256, as Tailwind is) and its Chromium headless shell. That takes about two minutes for the whole reference set, where driving the chat browser would take thousands of calls.
  - Before measuring, every text node holding a figure is written as a single `0`, and motion is turned off, so live readings and the table columns that reflow around them don't read as moves.
  - It reuses one saved session, so runs don't pile sessions up on Access.
  - `layout-noise` records the elements two runs of the unchanged app disagree on (a device going idle, the live log), and diffs leave them out.
  - A diff lists the elements whose own style changed and counts the ones that only moved or resized as a result.
  - It records, for every element in the document's body: a stable path (tag, `verso-*` classes, `data-verso-*` names, sibling index), the border box, the computed margins, paddings and border widths, the computed colour, background and border colour, and the computed font family, size, weight, line height, letter spacing and `font-variant-numeric`.
  - Output is sorted JSON.
  - Classes are left out of the comparison key so a rename does not count as a change. The path uses tag, index and behaviour attributes.
- **The reference set is discovered, not listed:**
  - every page the rail links to, and every page those link to;
  - the first drawer of each kind a page opens through its `?open=` links (new and existing);
  - the interactions `scripts/layout-reference.json` names: a device's three tabs, a confirm, Arrange, a closed Overview card;
  - all at 1440px and 390px in the dev container (`make dev`, localhost:8080, root / 12345678).
  - That comes to 108 surfaces today.
- **Login** is recorded signed out. **The takeovers** are written as self-contained HTML (the page inlines its stylesheet) by a test-only helper behind a build tag into `build/layout-fixtures/`. The snapshot serves them itself, passing `/assets/` through to the dev container so fonts and icons are the app's own.
- **Gate:** each conversion task ends with a diff against the previous task's snapshot in which every difference belongs to a convergence that task records in `drift.md`. That covers the converging element's own properties and the box shifts they cause after it. An unrecorded difference fails the task. Diffing against the previous task's snapshot, rather than the first baseline, keeps each task's diff to its own convergences. Screenshots are kept beside the snapshots for a human look, but they are not the gate: antialiasing makes pixel diffs noisy, and the dev host has no ImageMagick.

### 11. Type roles: a closed set, one class each, the face following the content

- **A role is one `@utility type-<role>` in `assets/type.css`.** It sets `font-family`, `font-size`, `line-height`, `font-weight`, `letter-spacing`, `text-transform` and `font-variant-numeric` together.
  - *Alternative: Tailwind's `--text-<name>` theme tokens* with `--line-height`, `--font-weight` and `--letter-spacing` sub-values. Rejected: they can't carry the family or numerals, so the Verbatim Rule would still be retyped at each site.
- **No ink in a role.** The same role appears in Ink, Meta and a tone's Deep step. The block that applies the role sets `color`.
- **Blocks apply roles with `@apply`.** For example: `.verso-facts__label { @apply type-row-label; color: var(--color-meta); }`. Markup may also write a role class on a one-off element: the role is a utility, so it outranks a block like any other.
- **The role set, approved by the owner on 2026-10-17,** from DESIGN.md's hierarchy and the roles it defines inline. Sizes are px at weight, on the line height given.

  | Role | Spec | Verbatim form |
  |---|---|---|
  | `type-headline` | sans 24/600, 1.1, -0.025em | — |
  | `type-title` | sans 18/600, -0.025em | — |
  | `type-section-title` | sans 18/600 on 24, -0.025em | — |
  | `type-card-title` | sans 16/600 on 24, -0.025em | — |
  | `type-subheading` | sans 14/600 on 24 | — |
  | `type-body` | sans 14/400, 1.5 | inline `code`: mono 16/400 |
  | `type-lede` | sans 14/300 on 24 (a takeover's 16/300) | — |
  | `type-label` | sans 12/500 on 16, 0.08em, uppercase | — |
  | `type-column-head` | sans 14/600 on 16 | — |
  | `type-row-label` | sans 14/400 on 24 | — |
  | `type-field-label` | sans 14/500 on 24 | — |
  | `type-value` | sans 14/500 on 24 | mono 16/500 |
  | `type-chip` | sans 14/400 on 16 | mono 14/500 |
  | `type-button` | sans 14/600 | — |
  | `type-input` | sans 14/400 | mono 16/400 |
  | `type-code` | mono 16/400 on 24 (the editor) | — |
  | `type-figure` | mono 36/700, -0.05em, tabular | — |

  Task 3.1 measures the live combinations against this table. A combination with no row is a drift move, unless the owner approves it as a new row and it is added to DESIGN.md with its reason.
- **The face follows the content.**
  - The renderer writes `data-verso-verbatim` on an element holding a machine-written string, and `data-verso-figures` on a count, duration, rate or size, from the flags it already has (`Mono`, `Duration`, a rate or size datatype).
  - Each role defines both forms inside its utility (`&[data-verso-verbatim] { … }`, `&[data-verso-figures] { font-variant-numeric: tabular-nums }`), so the attribute works wherever the role is applied and no block repeats it.
  - A role with no verbatim form ignores the attribute. The face then follows the role, which is right for a heading that happens to name a device.
  - A count or duration never carries `data-verso-verbatim`; a renderer test asserts this for each value kind.
- **DESIGN.md names the roles by class.** Typography › Hierarchy lists each role as `**Card title** (`type-card-title`) …`. `TestTypeRolesMatchDesign` parses that list against `type.css` in both directions. Adding a role is a DESIGN.md change with its reason, as adding a widget is.
- **Drift converges; distinctions are asked about.**
  - Task 3.1 maps every live combination to a role. Each element that moves is recorded in `drift.md` as it converts.
  - A combination that looks like a deliberate distinction rather than drift is proposed to the owner as a new role, with its case, before it is applied. Otherwise it converges on its role.

## Risks / Trade-offs

- **[A convergence shifts layout below it (a 21px line becoming 24px)]** → `drift.md` records the box effect along with the change itself, the gate accepts only recorded differences, and the owner sees a before/after screenshot of each.
- **[Seventeen roles is more than DESIGN.md's ten]** → The seven extra roles are already specified in DESIGN.md's own sections; this only names them. The set stays closed after task 3.1, and growing it takes a DESIGN.md change.

- **[The conversion is large, and a long-running half-converted state invites mixing]** → The ratchet keeps every commit green and forbids new violations from the first commit, so nothing new is built the old way while the rest converts. Groups follow dependency order (compositions, then forms, then the card, …), so a block never waits on a converted dependency.
- **[A renamed class breaks a script or test selector]** → Each block task greps `verso*.js` and `*_test.go` for the old names, converts them in the same commit, and runs `make test`. Scripts select by behaviour attributes wherever one exists.
- **[A surface renders a top-level titled section that is not a card today]** → Task 6.1 has a test per surface, and the parity run covers every surface. A surface that changes turns `cards` off and gains a test.
- **[`:nth-child(1 of S)` mis-compiled]** → Checked first (task 1.3), with the fallback recorded in Decision 7.
- **[Custom properties are invisible coupling]** → Block headers list what each block reads and sets, and `TestCrossBlockRulesSetOnlyProperties` keeps the coupling in that one form.
- **[The 8-utility cap pushes a genuinely one-off layout into a block]** → That's intended. A one-off with nine utilities is still a named thing, and a one-element block costs a few lines. No allowlist exists, so the guard stays unconditional.
- **[Snapshot misses state-only looks: hover, focus, busy, drag]** → Each block's markup test asserts its state attributes, and the reference set includes the open menu, the confirm dialog, Arrange mode and a closed Overview card. Hover and focus rules move verbatim with their selectors re-rooted, and the block guard checks the re-rooting.

## Migration Plan

- **Server-rendered.** No router state changes. Templates and the stylesheet ship together in the one binary, so an upgrade swaps both at once.
- **Plugins** are unaffected, since they send schema, not classes.
- **Rollback.** Revert the commits. Every commit is green and pixel-identical, so any prefix of the change is a valid state to stop at.
