# Tasks

**Every task ends** green under `make lint` and `make test`. Run `make css` and stage the rebuilt `internal/server/assets/verso.css` with any template or stylesheet change. Commit each task on its own.

**Parity** means the geometry and type snapshot (design Decision 10) of the whole reference set, at 1440px and 390px, diffs against the previous task's snapshot such that every difference belongs to a convergence this task records in `drift.md`: drifted copies of a look becoming the block's or role's one form, and the boxes that shift after them.

**A block is done** when all of these hold:
1. A markup test written first in the widget's or page's `_test.go` asserts the block's root, elements and variant attributes, and fails before the conversion.
2. The block's rules live in `internal/server/assets/blocks/<block>.css`, with a header listing its elements, variants, the custom properties it reads (with defaults), and those it sets on other blocks.
3. Each of its parts takes its type from a role with `@apply type-<role>` and sets its own ink. No type utility or declaration of its own remains.
4. Templates and client scripts name the block, and the old classes, class-string templates and `[&…]` utilities for that look are gone.
5. Its lines are deleted from the ratchet list.
6. Parity holds.

## 1. Baseline and toolchain checks

- [x] 1.1 Write `scripts/layout-snapshot.js` (Decision 10), recording boxes, spacing, colours and computed type. Verify: two snapshots of Firewall settings taken back to back in the dev container diff empty; a `mt-1` added by hand in the browser to one card makes the diff name exactly that card and the boxes below it; a `font-weight` changed by hand on one label makes it name that label.
- [x] 1.2 Write the test-only fixture writer, behind a build tag, that renders the login page and the restarting, upgrading and firmware-started takeovers as self-contained HTML into `build/layout-fixtures/`. Serve them from the Playwright scratch server (bind 0.0.0.0, open via 10.0.0.232). Verify: each fixture opens and looks like the live page in a screenshot.
- [x] 1.3 Confirm that Tailwind's compile keeps each of these unchanged:
  - `:nth-child(1 of S)` and `:nth-last-child(1 of S)`;
  - `@property` with `inherits: false`;
  - an `@utility` with a nested `&[data-…]` rule;
  - `@apply` of that utility inside an `@layer components` rule, carrying its nested rule along.

  Add throwaway rules, run `make css`, then remove them. Verify: the compiled `verso.css` held all four as written. If not, record the fallback in design.md Decisions 7 and 11.
- [x] 1.4 Capture the baseline: snapshots and one screenshot of every reference-set surface at both widths, under `build/layout-baseline/` (gitignored). Verify: one snapshot per surface and width, and a second full run diffs empty.

## 2. Contract and guards

- [x] 2.1 Write ADR-025 "Stylesheet blocks", with the reasons from design.md, covering:
  - the three layers and the type roles;
  - naming and one file per block;
  - the role-marking rule;
  - the custom-property rule with `@property`;
  - the `@apply` policy;
  - the guards and their thresholds.

  Amend ADR-005 Decision 6 in place to point at it. Add a "Styling" section to CONTRIBUTING.md saying when a look becomes a block, when text needs a role, and where each goes. Verify: ADR-025 states the contract with no process wording, ADR-005 §6 and CONTRIBUTING link it, and `docs/ADR` numbering has no clash.
- [x] 2.2 Test first: write the ten guards in `internal/server/stylesheet_guards_test.go` (Decision 4), with their fixture tests in `stylesheet_guards_fixtures_test.go`. Each guard first gets a table test over in-memory fixture sources that shows it failing on its violation and passing on the conforming form. Then generate `internal/server/testdata/stylesheet-ratchet.txt` from the real sources, and make each guard fail on an unlisted violation and on a listed line that no longer occurs. Verify: the fixture tests cover every guard; `make test` is green; adding `[&_td]:pt-0` to any template fails `TestNoRulesInMarkup` with the template's name; adding `text-xs` fails `TestTypeComesFromRoles`.
- [x] 2.3 Import blocks from `input.css` inside `@layer components`, after the current imports (`@import "./blocks/<b>.css" layer(components);`, one line per block, since an import takes no glob), and guard that every block file is imported (`TestEveryBlockIsImported`). Verify: `make css` builds and the compiled stylesheet is unchanged.

## 3. Type roles

- [x] 3.1 Map type to roles:
  - Map every live type combination (template, script and stylesheet) to a role in Decision 11's table.
  - Write the mapping into `openspec/changes/css-block-layer/drift.md` (combination → role, and the elements carrying it). Each block task then records the convergences it makes.
  - List separately any combination that might be a deliberate distinction rather than drift, with the case for a role of its own.

  Verify: every combination the snapshot recorded is mapped. **If any distinction is proposed, stop for the owner's decision on it; otherwise continue.** The owner settled the role set at 21 (drift.md).
- [x] 3.2 Test first: `TestTypeRolesMatchDesign` fails until DESIGN.md › Typography › Hierarchy lists every approved role by class with its full spec. Update DESIGN.md: name the ten existing roles by class, add the approved roles from other sections with their reasons, and state that a new role goes through DESIGN.md. Verify: the test passes against `type.css`.
- [x] 3.3 Write `internal/server/assets/type.css`, imported from `input.css` after `palette.css`. It holds every approved role as an `@utility type-<role>` with its verbatim and figures forms (Decision 11). Verify: `TestTypeRolesMatchDesign` passes; `TestEveryRoleIsWhole` asserts each role sets face, size, line, weight, tracking and figures; the compiled stylesheet is unchanged, since no role is applied yet.
- [x] 3.4 Test first, in the property, table-cell and chip tests (`internal/widget/value_marks_test.go`); a machine string elsewhere (the nameplate, a part's meta, a list field's values, the sign-in facts) takes its mark with its role when its block converts:
  - a verbatim value renders `data-verso-verbatim` and no `font-mono`;
  - a duration, rate, size or count renders `data-verso-figures` and no `tabular-nums`;
  - no count or duration renders `data-verso-verbatim`.

  Then switch the renderer and templates from the `Mono`/`Duration` utilities to the attributes. Verify: the tests pass; parity holds, because the attributes have no effect until a role is applied, and the elements keep today's utilities, which come off as each block converts.

## 4. Compositions

- [ ] 4.1 `rhythm`: the block rhythm and its `--verso-rhythm` property, registered with `@property`; first-visible via `:nth-child(1 of S)`. Verify: block done.
- [ ] 4.2 `stack` and `cluster`: the page stack, and the repeated `flex flex-wrap items-center gap-2 min-w-0` row. Verify: block done; `TestNoRepeatedBundles` no longer lists that bundle.
- [ ] 4.3 `page`: page body, full and ground variants, heading (`type-headline`), the reach properties. Verify: block done.
- [ ] 4.4 `section`: the kinds other than card (ruled, part, continued, plain, bare), band, ledger, kicker (`type-label`) and lede (`type-lede`). Every rule keyed on `data-verso-headless` or `data-verso-ruled` names its kind. Verify: block done; `section.html.tmpl` holds no class-string template call.
- [ ] 4.5 `drawer`, `beside`, `cards-row`: drawer frame, motion, open state, scrollbar, ground, gutter and title; the figure-beside row; the cards-of-one-kind row. Verify: block done, with a drawer of each kind in the parity run.

## 5. Forms

- [ ] 5.1 `form`: the form and its grid. Verify: block done.
- [ ] 5.2 `field`: row, question, label (`type-field-label`), answer, ledger, control, act, wide, plain and set. Register `--verso-field-bleed`, `--verso-field-lead`, `--verso-field-trail` and `--verso-edge-trail` (Decision 6), defaulting to today's values. Retire the `verso-field-*-class` templates. Verify: block done; `form-fields` scenarios pass in their tests.
- [ ] 5.3 `input` and `select` (`type-input`, with its verbatim form for identifiers): the `verso-box` family, `verso-input-class`, time input, the select chevron and the repeated chevron overlay. Verify: block done.
- [ ] 5.4 `choice`: switch, inline switch, checkbox and radio rows. Verify: block done.
- [ ] 5.5 `conditional`: gate, body, otherwise. Read `:checked` and forward the edge (Decision 7) to the last row of the branch that shows. Verify: block done; a test with the body open and one with it closed both assert which row carries the trail property.
- [ ] 5.6 `condition`, `joined`, `key-chip`. Verify: block done.

## 6. The card

- [ ] 6.1 Test first, in `internal/widget/section_card_test.go`:
  - a titled top-level section renders `class="verso-card"` and `data-verso-section="card"`, with `verso-card__head` on its band and `verso-card__body` on its rhythm;
  - a headless ruled top-level section renders `verso-card` and `data-verso-headless`;
  - a nested part does not render `verso-card`;
  - `OnCards`' untitled card does;
  - a section in the choices drawer does not;
  - a section in an Overview card body does not;
  - one test per remaining `RenderWithToken` surface.

  Verify: the tests fail.
- [ ] 6.2 Add `cards` to the `Renderer` handle (Decision 5) and return `card` from `sectionView.Kind()`. Emit the card classes in `section.html.tmpl`, and turn `cards` off for the choices drawer and the Overview card body. Verify: the 6.1 tests pass, and parity holds with the card rules still in `sections.css` re-keyed to `[data-verso-section="card"]`.
- [ ] 6.3 `card`: frame, inset, head seam, title (`type-card-title`), body. The row bleed and edge handoff are property settings only, and visible edges use `:nth-child(1 of S)`. Delete every `:is(.verso-page-body, [data-verso-drawer-body]) section…` rule. Verify: block done; that pattern no longer appears in any stylesheet (search is empty).
- [ ] 6.4 `overview-card` and `overview`: the article takes `verso-card verso-overview-card`, the header `verso-card__head`, the title `verso-card__title`, the content `verso-card__body`. The Overview card's own frame and type utilities come off, and collapse, arrange, tone, summary and slot go to its block. Verify: block done; Arrange and collapse work in the dev container; the reference set's closed card and Arrange states hold parity.
- [ ] 6.5 Spec scenario checks in the dev container, never committed:
  - swap the border token in `card.css`;
  - swap `type-card-title`'s weight in `type.css`.

  Verify: a page card, a drawer card and an Overview card all show each change. Then revert.

## 7. Controls

- [ ] 7.1 `button` (`type-button`): button styles, waiting, press, menu button, icon button (`verso-btn-icon`), the actionbar buttons' repeated bundle. Verify: block done; the busy-button test asserts `data-verso-*` busy state.
- [ ] 7.2 `link` and `chip` (`type-chip`, with its verbatim form): the repeated underline link; the chip and `verso-chip-class`. Verify: block done; the Overview's "IPv4 · dhcp" chip renders `data-verso-verbatim` (test).
- [ ] 7.3 `tip` and `menu`: tip look, tip class, copy tip; the suggest, token and condition menus. Verify: block done, with an open menu in the parity run.
- [ ] 7.4 `tabs`: the drawer and entity-panel tab strips. Verify: block done.
- [ ] 7.5 `disclosure` and `copy`: disclosure, fold, unfold, reveal, chevron; copy inline. Verify: block done.

## 8. Data display

- [ ] 8.1 `table` (`type-column-head`, `type-label`, `type-value`): table, seams, row name. Narrow columns become `data-verso-narrow~="N"`. Rename `verso-tables.js` and `verso-stream.js` row-building in the same commit. Verify: block done; a live listing's streamed row carries the same classes and type attributes as a rendered one (test).
- [ ] 8.2 `list` and `facts` (`type-row-label`, `type-value`, `type-subheading` for the head): the identity, system and ruled styles. Verify: block done; the `page-cards` "Facts read as one list" scenarios and the spec's Internet-card scenarios pass in their tests.
- [ ] 8.3 `meter` and `chart` (`type-figure` where a figure stands): the chart's `--` colour modifiers become a `data-verso-tone` attribute and its parts become `__` elements, in `verso-chart.js` too. Verify: block done; the history-charts tests pass.
- [ ] 8.4 `ports` and `panel`. Verify: block done.
- [ ] 8.5 `code` (`type-code`), `prose` (`type-body`, inline `code` verbatim), `empty`. Verify: block done.
- [ ] 8.6 `notice`: callout, flash, staged bar, outcome. Verify: block done.
- [ ] 8.7 `mark`, `waiting`, `live`, `step`: the 6px state square (the six-site bundle), the four-square waiting mark, the live dot, steps. Verify: block done; `TestNoRepeatedBundles` lists none of them.

## 9. Shell chrome and takeovers

- [ ] 9.1 `nav`, `rail`, `actionbar`, `colophon`. View-transition names stay as they are. Verify: block done.
- [ ] 9.2 `modal` and `pending`: dialog and drawer boxes, motion, the confirm frame, the dialog title (`type-title`). Verify: block done, with an open confirm dialog in the parity run.
- [ ] 9.3 `stream` and `console`: stream release, shelf and pluck; console, notice and vt, in `verso-stream.js` too. Verify: block done on Logs, Firewall › Activity and Diagnostics.
- [ ] 9.4 `takeover`: login, restarting, upgrading, firmware-started (`type-headline`, the takeover `type-lede`), and the repeated step and heading bundles they share. Verify: block done against the 1.2 fixtures.
- [ ] 9.5 `scroll-edge`, `glyph`, `arrival`, and any family the ratchet still lists. Verify: block done; the ratchet holds no `TestClassesAreBlocks`, `TestRulesLiveInBlocks` or `TestTypeComesFromRoles` line.

## 10. Close the ratchet

- [ ] 10.1 Delete `forms.css` and `sections.css`, reduce `input.css` to its entry role (Decision 1), and remove the `@theme` text overrides the roles supersede (`--text-sm--line-height`). Verify: `TestRulesLiveInBlocks` passes with no ratchet lines, and parity holds.
- [ ] 10.2 Confirm that the ratchet list is empty, then delete it and the guards' ratchet-reading code, so every guard is unconditional. Verify: the file is gone; `make test` green; every guard passes with no exceptions.

## 11. Integration

- [ ] 11.1 Full parity over the whole reference set at both widths. Look at the screenshots beside the baseline's and confirm that every difference is a convergence `drift.md` records, then run `make lint` and `make test`. Verify: the diff holds only recorded convergences; the screenshots match otherwise.
- [ ] 11.2 Update `docs/BACKLOG.md` with the block layer and the type roles as they stand, and record any visual improvement noticed during the conversion as its own item. Run `graphify update .`. Verify: BACKLOG reflects it; graphify reports the update.

## Workflow follow-up

- Archive with `/opsx:archive` once ADR-025, ADR-005, DESIGN.md and docs/BACKLOG.md reflect what landed.
