# Proposal

## Why

Every page now measures its layout on an 8px grid, with 4px allowed inside a component. The 20px cell offered only 10, 20 and 40, so values like 34px controls, 26px heading air and 21px chips sat between its lines. On the 8px grid every layout measure is one of 8, 16, 24, 32, 40 or 48. The owner reviewed the 8px grid side by side with the 20px cell (the Verso grid preview artifact) and confirmed it.

## What Changes

- Every fixed measure of the layout is a multiple of 8px:
  - controls are 32px (were 34)
  - a card's inset and the gap between cards are 24px (were 20)
  - the heading line has 24px of air above and below (was 26)
  - a card's header is 48px (was 50)
  - a listing's control strip is 64px (was 60): 16px over and under the 32px controls
  - a card's column-head row is 48px (was 49)
  - the air over a field is 24px (was 20)
  - the page measures are 1136, 1016, 880 and 760px (were 1140, 1020, 880 and 760), and a squeezed column rounds down to 8px (was 20)
- Inside a component, a measure may step by 4px:
  - chips are 20px (were 21)
  - checkboxes are 16px (were 18)
  - a radio row is 28px (was 30)
  - a rail's "On this page" link is 28px (was 30)
  - row acts stay 28px
- Text keeps its 20px lines. A box that grows with its text (a 40px row, a 60px stacked row, a warning band) lands on 4px; a fixed box lands on 8px.
- Charts rule their plots every 16px. A plot is 192px tall (was 160), so its four scale labels each land on a rule.
- On a phone the side gutter is 16px (was 20).
- The page frame (40px), the drawer (760px wide, 40px inset, 56px header and tab strip), the top bar and the colophon (56px), and table rows (40px) keep their sizes: each is already a multiple of 8.
- In a dev session (`make dev`), pressing G draws the 8px grid over the page and over an open drawer, each measured from its own origin. Pressing G again removes it. The choice is kept for the tab across reloads and hot reloads. Outside a dev session, no grid is ever drawn.
- The page-cards spec is brought in line with what the pages draw since commit 71ab775: a laned listing is one card with its lanes as Quiet Sand bands. Lanes carry no counts.

## Capabilities

### New Capabilities
- `layout-grid`: the measuring system every page, card and drawer is laid out on: the 8px grid, the 4px half-step inside components, 20px text lines, and the dev-session grid overlay that checks it.

### Modified Capabilities
- `page-cards`: the heading line, the card gap, the control strip and the dev-session grid scenario take the 8px measures, and a laned listing is one card with its lanes as bands.

## Impact

- **Shell CSS:** `internal/server/assets/input.css` (`@theme` tokens: `--spacing-control`, the container measures, new layout role tokens), `sections.css`, `forms.css`, and the rebuilt `verso.css`.
- **Templates:** about 400 off-grid spacing utilities across `internal/widget/templates/*.tmpl` and `internal/server/templates/*.tmpl`, most of them in `table`, `shared`, `table-interfaces`, `overview`, `properties`, `link`, `login`, `restarting`, `page`, `meter`, `form` and `field`.
- **Client scripts:** `verso-dev.js` (the G overlay), `verso-chart.js` (16px plot rules), `verso-overview.js` and `verso-tables.js` (measures read in script), and `verso-listing.js` and `verso-commit.js` (class strings).
- **Tests:** Go widget and server tests that assert class strings (`table_test.go`, `renderer_test.go`, `collection_test.go`, `form_test.go`, `lane_test.go` and others) change with the templates. A new guard test keeps fractional spacing out of templates and script class strings.
- **Plugins:** none. Plugins send a widget schema and no classes, so installed plugins render on the 8px grid unchanged. The Rust SDK and its conformance fixtures are untouched.
- **On-device state:** none. No UCI option, file or cookie changes, so no migration ships.
- **Docs:** DESIGN.md (the "20px cell" rules become "The 8px grid", the component measures, and the frontmatter tokens), ADR-023 (amended in place where it says the 20px cell sets every measure inside a card), and docs/BACKLOG.md.
- **ADRs touched:** ADR-023 (pages are cards). ADR-022 (Overview cards) is unaffected: it pins no measure.
