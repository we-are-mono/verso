# Tasks

Code follows the ponytail rule: the smallest complete change. Reuse the markup the shell already renders, style before restructuring, and delete what goes (the grid, the masthead fill, the Mid Sand band) rather than leave it unused.

## 1. Page frame: sand ground, no grid, no masthead bar

- [x] 1.1 Write a failing test in `internal/server/server_test.go`: a form page (Access) and a wide page (Devices) render with `bg-quiet`, without `verso-notebook`, and with the masthead's heading line padded 26px; Logs, Firewall › Activity and Diagnostics render byte-for-byte the same page frame as before (golden of their frame markup taken before the change). Then change `page.html.tmpl` until it passes.
- [x] 1.2 Delete `.verso-notebook` and its grid, anchor and mask rules from `input.css`, and the default masthead's fill and hairline, scoped so `[data-verso-masthead="light"]` keeps its Paper, hairline and reach, and the G shortcut with `data-verso-grid-full` from `verso-dev.js`. Update the tests that assert them (`heading_act_test.go`, `document_title_test.go`, the page tests listed by `grep -l verso-notebook`) and check that `make test` is green.
- [x] 1.3 Rewrite DESIGN.md's Layout section (the notebook grid, the masthead, "every page but home and a log stands on the grid") to describe the sand ground and the heading line. Check with `grep -n "notebook grid" DESIGN.md` that only the inside-a-card rhythm is left.

## 2. Sections become cards

- [x] 2.1 Write a failing widget test (`internal/widget/heading_levels_test.go` or a new `section_card_test.go`) that a top-level section with a title and an `Icon` renders the glyph before its title in the band, and a part's ledger line is unchanged. Then add the glyph to `section.html.tmpl`.
- [x] 2.2 In `sections.css`, frame a top-level page section with a band as a card (Paper, Strong Hairline, 2px, the band as a 16px-title header with a hairline under it, 20px body inset, reach variables 20px inside the card, cards 20px apart), scoped to `main` and excluding `dialog`, and a ruled headless section (a page form's untitled fields) as the same card with no header. Verify in the dev container that Firewall settings, Access and the Wi-Fi, DHCP/DNS, VPN and System General pages show one card per section and a drawer (a firewall rule's) is unchanged.
- [x] 2.3 Give the configuration preview (`[data-verso-code-divider]`) the card frame with its file strip as the header. Remove the `artifact` card style's frame and fill inside a page card. Verify on Firewall settings (the `/etc/config/firewall` card above "Save settings" on the sand) and Access (the certificate with no inner frame).
- [x] 2.4 Write a failing test in `internal/server/password_test.go` (or the Access test file) that the exposure callout renders inside the Web interface section. Then move it in `password.go`.
- [x] 2.5 Check the hand-built pages that compose sections (Maintenance, and any `grep -l data-verso-section internal/server/*.go` turns up) in the dev container. Convert any that hand-draw a band to the section markup, with a test asserting the section markup.
- [x] 2.6 Update DESIGN.md: Section headings on a page are card headers, Configuration cards on a page, the flat direction (cards are the page's subjects; drawers keep bands), and "Overview cards" no longer the only cards.

## 3. Listings become cards

- [x] 3.1 Write a failing test in `internal/server/packages_test.go` that the action bar and table render inside one listing wrapper. Then, if the existing markup does not already group them, group them in the page stack. Style the wrapper as an untitled card and the action bar as its top strip (13px over, 12px under, a hairline). Delete the Mid Sand control band rules. Verify Packages in the dev container.
- [ ] 3.2 Write a failing test in `internal/widget/lane_test.go`: a laned table renders one `<table>` per lane, each with the listing's `colgroup` and `thead`, the lane's label, detail, tally and add act in that card's header. Then change `table.html.tmpl`, with `table-layout: fixed` on laned listings.
- [ ] 3.3 Add `scripts/listing-lanes.test.cjs` (in the style of `overview-layout.test.cjs`) covering the filter walking rows across every table of a listing and the live inventory refresh replacing each lane's `tbody`. Then update the four single-table assumptions in `verso-tables.js` (filter walk, scope `querySelector("table")`, inventory refresh, scroll edges) until it passes.
- [ ] 3.4 Set Devices' column widths for fixed layout (Now 14rem, This month 8rem, Status 8rem) in `internal/widget/devices.go`, with `devices_test.go` asserting the colgroup. Check every other laned listing (Firewall rules, Port forwards, Zones, routes) in the dev container for a truncated first column, and adjust its widths the same way.
- [ ] 3.5 Update DESIGN.md's Control Band and Tables sections (lanes as cards, the control strip, rows to the card's edges, the last row's hairline given to the frame).

## 4. Decision records and backlog

- [ ] 4.1 Write `docs/ADR/023-pages-are-cards.md` (context, the four mocks, decisions: no band, no grid, drawers unchanged, no collapse, the control strip, Save placement; consequences). Amend ADR-022's consequence about the one page with cards in place. Verify that `grep -n "one page with cards" docs/ADR` returns nothing.
- [ ] 4.2 Update `docs/BACKLOG.md`: remove items this change closes (anything about the grid or the masthead bar) and add the padding-and-margins fine-tuning spec the owner asked for as an open item.

## 5. Integration check

- [ ] 5.1 With `make dev` running, open every page in the nav at 1600px and 390px. Check that there is no grid, no masthead bar, one card per section or lane, no card inside a card, that tables scroll inside their card on a phone, that drawers are unchanged, and that Logs, Firewall › Activity and Diagnostics look exactly as before (compare with screenshots taken before group 1). `make lint` and `make test` are green.

## Workflow follow-up

- Archive with `/opsx:archive` once every task is done and ADR-023, DESIGN.md and `docs/BACKLOG.md` reflect what landed.
