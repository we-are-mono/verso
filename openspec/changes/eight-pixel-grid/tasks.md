# Tasks

Every task ends green under `make lint` and `make test`. Each step is reviewed in the browser through `make dev`, with G on once group 1 lands.

## 1. The dev grid overlay

- [x] 1.1 Write the server test first: a dev page's HTML carries `verso-dev.js`, and the stylesheet carries the `[data-verso-grid]` overlay rule for `main` and for the drawer body, drawn every 8px with `pointer-events: none`. A non-dev page carries no dev script. Watch the test fail.
- [x] 1.2 Add the overlay rules to `input.css` (from `main`'s top-left, and from the drawer body's chrome hairline) and rebuild `verso.css`. Verify 1.1 passes.
- [x] 1.3 Add the G handler to `verso-dev.js`: toggle `data-verso-grid` on `<html>`, ignore the key in fields and with modifiers, and keep the choice in `sessionStorage` behind try/catch. Add a `node --test` case under `scripts/` that loads the handler on a fake document and covers toggling on and off, typing G in an input, a modifier held, and storage throwing.
- [x] 1.4 In the browser: press G on Devices and in an open rule drawer, and confirm the lines start at the top bar's hairline, the rail's border and the drawer chrome. Reload and confirm the grid stays on. Commit.

## 2. Tokens and the guard

- [x] 2.1 Write the guard test in `internal/widget` first. It walks every embedded widget and server template and every `assets/*.js`, and fails on fractional spacing utilities and on `*-5` air (padding, margin, gap, inset) outside a named allowlist. Run it and record the current failures as the sweep's checklist.
- [x] 2.2 Add the role tokens to `@theme` in `input.css`: `--spacing-control` 2rem, `--spacing-inset` 1.5rem, `--spacing-frame` 2.5rem, `--spacing-act` 1.75rem. Add the row's optical padding as one `@utility py-row`. Change the container measures to 1136, 1016, 880 and 760px, and every `round(down, …)` step to 0.5rem. Update the server tests that pin the measures first, then rebuild. Verify `make test` is green except for the guard.

## 3. Shell frame

- [x] 3.1 Page frame: heading line 24 / 32 / 24, phone gutter 16px, measure rounding to 8px. Update `page.html.tmpl` and the frame rules in `input.css`, changing the server tests that pin the heading air first. With G on, verify the h1 line and the first card's top edge land on lines on Access and Firewall settings. Commit.
- [x] 3.2 Rail: rows keep 40px, the "On this page" links become 28px, and the nav row's spacing comes off fractions (`nav-row.html.tmpl`). Update the guard checklist entries for these templates. Verify with G on. Commit.

## 4. Cards and listings

- [x] 4.1 Cards (`sections.css`, `section.html.tmpl`, `card.html.tmpl`): inset and gap 24, header 48, body 24 over and under, lede and part steps on the new roles. Update the section, heading-level and lane tests first. Verify with G on: Firewall settings and System › General. Commit.
- [x] 4.2 Listings (`table.html.tmpl`, `table-interfaces.html.tmpl`, `actionbar.html.tmpl`, `verso-tables.js`): strip 16 / 32 / 16, column-head row 48, rows via `py-row`, lane bands 40, words 24px in, acts and grip re-centred off the old 20px crossings. Chips (`shared.html.tmpl`) become 20px. Update `table_test.go`, `table_interfaces_test.go`, `lane_test.go` and `renderer_test.go` first. Verify with G on: Devices, Firewall › Rules (lanes and reorder grip) and Network › Interfaces (expanded row). Commit.

## 5. Forms

- [x] 5.1 Fields (`field.html.tmpl`, `forms.css`, `form.html.tmpl`, `form_switch.html.tmpl`, `settings.html.tmpl`): 24px air, a 4px gap from the label line to a 32px control, 16px checkboxes, 28px radio rows, value chips in a 32px box, refusal bands, fused and joined rows, and the ledger line. Update `form_test.go`, `field_frame_test.go` and `conditions_test.go` first. Verify with G on: Firewall › Rules' new-rule drawer (conditions), System › Access and Network › DNS. Commit.
- [x] 5.2 The remaining form widgets (`list.html.tmpl`, `collection.html.tmpl`, `conditions.html.tmpl`, `conditional.html.tmpl`, `code.html.tmpl`, `callout.html.tmpl`, `confirm.html.tmpl`, `properties.html.tmpl`, `link.html.tmpl`, `button.html.tmpl`, `disclosure.html.tmpl`, `divider.html.tmpl`, `empty.html.tmpl`, `grid.html.tmpl`, `stack.html.tmpl`). Update `collection_test.go`, `properties_test.go` and `maintenance_widgets_test.go` first. Verify with G on: SSH keys, System › Maintenance and a configuration card. Commit.

## 6. Overview and charts

- [x] 6.1 Overview (`overview.html.tmpl`, `meter.html.tmpl`, `stat.html.tmpl`, `ports.html.tmpl`, `verso-overview.js`, the Overview rules in `input.css`): header 48, body 24, card and column gaps 24, column rounding to 8px, and shelf and Arrange spacing. Update the overview widget tests first. Verify with G on, both closed and open cards, and in Arrange mode. Commit.
- [x] 6.2 Charts (`chart.html.tmpl`, `traffic.html.tmpl`, `chart.go`, `verso-chart.js`): rules every 16px, plots 192px in a card or short and 256px otherwise, scale labels on every fourth rule, and the glide keeping the 16px grid travelling with the curve. Update the chart tests that pin `W`/`H` and the plot class first. Verify on the Overview Traffic card and a device's Usage tab, with live data gliding. Commit.

## 7. Drawers and the remaining surfaces

- [x] 7.1 Drawers (`modal.html.tmpl`, `entity-panel.html.tmpl`, `system-panel.html.tmpl`, `staged.html.tmpl`, the drawer rules in `sections.css`): section rules 40 under content, titles 40 under the rule, content 24 on, drawer column heads 24, and tabs and header kept at 56. Update `staged_test.go` and the entity panel tests first. Verify with G on: a device panel, a rule drawer, and the staged review drawer. Commit.
- [x] 7.2 Live pages and the rest (`logs.html.tmpl`, `diagnostics.html.tmpl`, `login.html.tmpl`, `restarting.html.tmpl`, `upgrading.html.tmpl`, `firmware-started.html.tmpl`, `notice.html.tmpl`, `package-files.html.tmpl`, `package-navigation.html.tmpl`, `badge.html.tmpl`, `filter.html.tmpl`, `switch.html.tmpl`, `when.html.tmpl`, `repeater.html.tmpl`, `text.html.tmpl`, `raw.html.tmpl`, `device_limit_tip.html.tmpl`, `verso-listing.js`, `verso-commit.js`, the colophon): every remaining guard failure. Update `server_test.go` and `hardware_test.go` first. Verify that the guard test passes with an empty checklist, that `make test` is fully green, and with G on: Logs, Diagnostics, sign-in and an upgrade takeover. Commit.

## 8. Docs

- [x] 8.1 DESIGN.md: "The 20px cell" becomes "The 8px grid" (fixed measures on 8, 4 inside a component, 24px text lines, the dev overlay). Update every component measure and the frontmatter tokens (heights 32, spacing group 24, chip 20, checkbox 16, radio 28, plots 192/256). In the same pass, correct the stale lane, column-head and ledger-count passages to match commit 71ab775. Verify with `grep -n "20px cell\|34px\|21px\|a cell" DESIGN.md`: only the intended text lines remain.
- [x] 8.2 docs/BACKLOG.md: remove anything this change settles and add any off-grid spot the sweep surfaced but did not fix. Verify against the guard's allowlist. Commit the docs.

## 9. Integration

- [x] 9.1 Walk every rail page and its drawers in a private window with G on, at 1440px and 390px wide, and fix anything off a line. Then run `make lint` and `make test`, and confirm `openspec validate eight-pixel-grid --strict` passes.
- [x] 9.2 Text writes on 24px lines (design decision 9): `--text-sm--line-height`, `leading-6` in every template and script, `py-row` 8/7, card header 12/11, field label straight over its control, radio rows 32, marks 9px down, callout acts 23px under the words. Update DESIGN.md and the layout-grid spec. Walk all 23 rail pages at 1440px and 390px requiring every frame's edges on lines; commit.

## Workflow follow-up

- The owner uses the build on the Mono Gateway DK and confirms the look.
- Archive the change once DESIGN.md reflects what landed.
- After archive, update the memory notes that pin the 20px cell (notebook grid rules, row heights, page rhythm).
