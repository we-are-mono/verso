# Tasks

Every task is test first: write the named test, see it fail, then write the code. Every task ends green under `make lint` and `make test`.

## 1. The layout and its cookie

- [ ] 1.1 Write `internal/widget/overview_layout_test.go`. It needs table cases for: the default layout; a valid `1.…` value; a wrong version; garbage; unknown ids; a duplicate id; missing cards appended open to the shorter column; and a closed or hidden id that isn't in either column. Then add `overviewLayout`, its default and its normalizer. Verify: `go test ./internal/widget -run Layout`.
- [ ] 1.2 Write `internal/server/overview_layout_test.go`. It checks that `cookieLayout` returns the default with no cookie and the normalized layout with one, and that it never fails a request. Then add the `layoutSource` interface and `cookieLayout`, and have the Server hold a `layoutSource`. Verify: `go test ./internal/server -run Layout`.

## 2. Cards built from the Overview's readings

- [ ] 2.1 Extend `internal/widget/overview_test.go`. The existing tile tests become header tests for the Internet, Firewall, Ports and Tunnels builders, keeping every tone, status and plural case. Add a case for the Tunnels "None observed" body. Then add `overviewCard`, the builder registry, and those four builders, reusing the tile logic. Verify: `go test ./internal/widget -run Overview`.
- [ ] 2.2 Test then build the Traffic, Resources, Hardware and Firmware builders. Cover: a reading card shows a status only in a warning or danger band; every card's closed summary; Firmware carrying uptime. Verify: `go test ./internal/widget -run Overview`.
- [ ] 2.3 Test then build the Internet body: the IPv4/IPv6 `Properties` with copy controls, and the empty slot when a family isn't configured. Verify: a render test asserts the slot's text and its hollow packet.
- [ ] 2.4 Test then build the Ports and Tunnels bodies on the `Table` widget: an `entity` network chip, `Now` as inked figures, status last, and a spare port as "Not in use". If a cell kind is missing, stop and take the widget extension to the owner with its case before building it. Verify: render tests for an up port, a lost-link port, a spare port and a down tunnel.

## 3. The page renders the layout

- [ ] 3.1 Write render tests. They assert that the HTML puts cards in the layout's columns and order, renders closed cards with `aria-expanded="false"` and their summary shown, omits hidden cards entirely, and shows neither kicker nor tile strip. Then add `overview_card.html.tmpl`, rewrite `overview.html.tmpl` as the verdict line plus two columns, and delete `overview_masthead.html.tmpl`. Verify: `go test ./internal/widget`.
- [ ] 3.2 Write a server test for the Overview handler. A request with a layout cookie renders that layout, and one without renders the default. Then wire `layoutSource` into the handler. Verify: `go test ./internal/server -run Overview`.
- [ ] 3.3 Add the card, header, column, slot and shelf styles to `verso.css` from DESIGN.md's tokens (Quiet Sand page ground, Paper cards, Strong Hairline frame, 2px corners, flat at rest). Verify: `make dev` hot-reloads, and the page at localhost:8080 matches the mock at desktop and phone widths, with no horizontal page scroll.
- [ ] 3.4 Write the **Overview cards** section in DESIGN.md (frame, header anatomy, Arrange mode, the Overview-only exception to the flat direction). Verify: every value the CSS uses is named there.

## 4. The live stream follows the layout

- [ ] 4.1 Extend the `LiveStatus` tests: one entry per card, hidden cards included, each with id, tone, status and summary. Then grow `LiveStatus` from tiles to cards. Verify: `go test ./internal/widget -run LiveStatus`.
- [ ] 4.2 Write stream tests on `handleOverviewEvents` with layout cookies:
  - Resources hidden: no `meters` frame.
  - Hardware hidden: no `sensors` frame, and the sensor reader isn't called.
  - Traffic and Internet both hidden: no `wan` frame.
  - Ports and Tunnels both hidden: no `interfaces` frame.
  - Everything closed: every frame still sent.
  - `overview` and `clock`: always sent.

  Then build the frame list from the layout. Verify: `go test ./internal/server -run OverviewEvents`.
- [ ] 4.3 Update `verso-stream.js` to write card statuses, summaries and tones through the `data-overview-card*` hooks, and remove the tile hooks. Verify: a status change pushed in the dev container recolours a closed card's header without a reload.

## 5. Open, close and arrange in the browser

- [ ] 5.1 Write `scripts/overview-layout.test.cjs`, loaded with `vm` as `takeover.test.cjs` loads its module. Cover: move within and across columns; nudge in all four directions, including at column edges; hide; restore to the shorter column; reset; toggle; and `encode`/`decode` round-trips matching the Go parser's cases. Then write the pure model in `internal/server/assets/verso-overview.js`. Verify: `node --test scripts/overview-layout.test.cjs`.
- [ ] 5.2 Build the DOM layer: header and chevron toggle with `aria-expanded` and the name "Open/Close <card>"; Arrange/Done; pointer drag with the slot and lift; arrow keys on the grip; the `aria-live` announcement; and cookie writes through `persist`. Embed the file and load it in `page.html.tmpl`. Verify, in the dev container, the spec's scenarios: drag Hardware above Firewall; ArrowDown on Internet's grip announces "Internet: left column, position 2."; the layout survives a reload.
- [ ] 5.3 Hide, restore and reset re-request the Overview through the HTMX swap and reconnect the stream. Verify: hiding Resources stops `meters` frames in the browser's network panel, and restoring brings the card back at the end of the shorter column.
- [ ] 5.4 Reduced motion and keyboard: the open/close transition and the lift are off under `prefers-reduced-motion`; every control is reachable by Tab with a visible focus ring. Verify in the browser with reduced motion emulated.

## 6. Words, records and docs

- [ ] 6.1 Add every new string to the source catalog and `i18n/sl`, with Slovenian dual and paucal forms for counts through `overviewCount`: card titles; Arrange; Done; Hidden; No hidden cards; Reset layout; "Open/Close/Move/Hide <card>"; the move announcement; and the summaries. Verify: the i18n audit stays at zero and the Slovenian Overview survives its longer strings in two columns.
- [ ] 6.2 Write `docs/ADR/022-overview-cards.md` (decisions 1–7 of the design, with the cookie named the interim home of the layout), and point ADR-009's Overview paragraph to it. Verify: ADR-009, ADR-019 and ADR-022 agree on who owns the Overview and where the graph lives.
- [ ] 6.3 Update `docs/BACKLOG.md`. Add "Overview layout on the router, per user" (the move off the cookie) and "Plugin-contributed Overview cards"; reword any Overview entry the cards make stale. Verify: no BACKLOG entry describes the tile strip.

## 7. Integration

- [ ] 7.1 Run the whole Overview in the dev container in each of these states: healthy; a port without link; the uplink down; no IPv6; no tunnels; Resources hidden; and every card closed. Verify: each matches its spec scenario, and `make lint` and `make test` are green.
- [ ] 7.2 Verify on the Mono Gateway DK bench unit, not the production router: the Overview renders the real ports and tunnels, and the stream's CPU use with every card hidden is no higher than today's.

## Workflow follow-up

- Archive the change once ADR-022 and docs/BACKLOG.md reflect what landed.
