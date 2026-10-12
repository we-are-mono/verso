# Design

## Context

The Overview is shell-owned page chrome, not a plugin widget (ADR-009). `widget.Overview` composes it at render time:

- `overview_masthead.html.tmpl` draws the verdict, the clock with uptime, the four `ohTile`s and the IPv4/IPv6 fact columns.
- `overview.html.tmpl` adds the Internet `Traffic` chart, four `Meter`s and two `Properties` sheets.
- `LiveStatus` gives the verdict and the four tiles' status to the stream.

`handleOverviewEvents` (`internal/server/events.go`) holds one SSE connection per page. Each tick it writes five frames in order: `meters`, `interfaces`, `wan`, `sensors` and `overview`, with `clock` on its own beat. `verso-stream.js` writes each frame into elements found by `data-overview-*` hooks.

The Overview's data is already in hand: `Interfaces` carries every port and tunnel with its rates, and `WANConn` carries the IPv4/IPv6 facts. No new router reading is needed.

The mock is the visual reference: https://claude.ai/artifact/DRTQcksAMhEJRRg7LtUT8Y.

## Goals / Non-Goals

**Goals:**
- Each card is built by its own builder from the readings `Overview` already holds, so adding, removing or changing a card touches one builder.
- First paint is final: the server renders the saved layout, and JS never reorders cards on load.
- The live stream's per-tick work stays at or below today's.
- The layout logic (move, hide, restore, reset, encode) is pure and runs under `node --test` without a DOM.

**Non-Goals:**
- Plugin-contributed cards. The card list is the shell's; the builder registry is the seam a plugin contract would use.
- New readings, such as a Devices card, update status or zones.
- Storing the layout on the router.

## Decisions

### 1. A card is a value; a builder per card makes it

`widget` gains `overviewCard{ID, Title, Icon, Tone, Status, Summary, Body template.HTML, Go string}` and one builder per card, for example `(o *Overview) internetCard(r *Renderer, csrf string) (overviewCard, error)`. A fixed ordered registry `overviewCards = []cardBuilder{…}` names the eight cards and their default column.

- **Why:** SRP and OCP. Today one method knows every tile and every section; a card's content, status and summary belong together. The existing tile methods (`internetTile`, `firewallTile`, `interfacesTile`, `tunnelTile`) become the header half of their card's builder. Their tone and plural logic, and their tests, are kept.
- **Alternative rejected:** a generic "card widget" in the plugin vocabulary. Plugins don't get cards in this change, and a vocabulary widget needs approval and a contract. The card frame is Overview chrome, like the tiles were.

Bodies compose existing widgets, never hand-built markup:

| Card | Body |
|---|---|
| Internet | `Properties` (`overview-connection`, mono, copy) per family; the empty slot when a family isn't configured |
| Traffic | `InternetTraffic` |
| Resources | the four `Meter`s |
| Hardware, Firmware | `Properties` (`overview-system`) |
| Ports, Tunnels | the `Table` widget: an `entity` network chip (Ports) or the tunnel's kind as a `keyword` (Tunnels), `Receive` and `Transmit` as `rate` cells, status last |

The rows key themselves `interface:<name>` with `rx-rate`, `tx-rate` and `state` cells, the hooks the interface stream already writes, so the cards' rates and states stay live with no new client code and no new cell kind. A spare port's row carries no key: "Not in use" is the Overview's own reading, which the stream's generic state words would overwrite.

### 2. The card frame is one template

`overview_card.html.tmpl` draws the header (icon, title, summary, status, the Go link, chevron, grip, hide) and the collapsible body. `overview.html.tmpl` becomes the verdict line plus two column `range`s over the resolved layout. `overview_masthead.html.tmpl` and its tile strip are removed. Every live hook keeps one naming scheme on the card: `data-overview-card="<id>"`, `data-overview-card-status`, `data-overview-card-summary`.

### 3. The layout is a cookie the browser writes and the shell reads

- **Name:** `verso_overview`.
- **Value:** `1.<left>.<right>.<closed>.<hidden>`. Each field is card ids joined by `-`, so `1.internet-ports-tunnels.traffic-resources-hardware-firmware.firmware.` is a layout with Firmware closed. Ids are `[a-z]+`. The value uses only cookie-safe characters and stays under 200 bytes.
- **Attributes:** `Path=/; SameSite=Lax; Max-Age=34560000` (400 days, the browser cap), plus `Secure` when the page is served over HTTPS. It isn't `HttpOnly`, because the page's JS writes it. It holds nothing secret.
- **Writer:** `verso-overview.js` writes it on every open/close and at each move, hide, restore or reset. There is no server endpoint, no CSRF and no router write.
- **Reader:** `overviewLayout(r *http.Request) layout` parses and normalizes, behind a small interface:

  ```go
  type layoutSource interface{ Layout(*http.Request) overviewLayout }
  ```

  `cookieLayout` is its one implementation. Normalizing follows the spec's fallback rules: a wrong version or bad syntax gives the default layout, unknown ids are dropped, duplicates keep their first place, and missing cards are appended open to the shorter column. The shorter column is counted in cards, because the server can't measure heights.
- **Why a cookie:** chosen by the owner. The server sees it at render time, so the page arrives arranged and in-app HTMX swaps carry it with no extra request.
- **Alternatives:** `localStorage` would reorder after load. A per-user file on `/data` next to `history.db` follows the operator across browsers, but adds a write endpoint and on-device state.
- **This is an interim:** the cookie is the layout's home until it moves to the router per user. That move replaces `cookieLayout` with a store-backed `layoutSource`. The renderer, the stream and the JS model stay as they are, and the JS writes through a single `persist(layout)` call that a POST can replace.

### 4. The stream skips what only hidden cards read

`handleOverviewEvents` resolves the layout once per connection and builds its frame list from it:

| Frame | Read by | Sent when |
|---|---|---|
| `meters` | Resources | Resources is not hidden. The readings are still taken, because the verdict counts meter bands. |
| `sensors` | Hardware | Hardware is not hidden. Otherwise `resolveSensors` isn't called. |
| `wan` | Traffic, and the Internet summary's uptime | Either card is not hidden. |
| `interfaces` | Ports, Tunnels | Either card is not hidden. The telemetry snapshot is still read, because the verdict counts port and tunnel state. |
| `overview` | the verdict, and every card's status and summary | Always. |
| `clock` | the verdict line | Always. |

`LiveStatus` grows from four tiles to one entry per card: `{id, tone, status, summary}`. It covers hidden cards too, so the Arrange shelf can show a hidden card's packet.

Hiding, restoring or resetting changes what the page holds. The JS writes the cookie and reloads the page, as the shell's pages navigate by full loads; the stream reconnects with the page, so the frame list is rebuilt. Arrange carries over the reload in `sessionStorage`, so hiding a card doesn't end the mode. Moving and opening/closing change neither, so they need no round trip.

The stream reads the interfaces' topology (`interfaceList`) once per connection and lays each second's states and rates onto it. Before this, the stream's interfaces had no networks, so a port in use read as spare one second after load, and a lost link never reached the verdict.

### 5. `verso-overview.js`: a pure model and a thin DOM layer

- **The model** has pure functions over a `{cols:[[ids],[ids]], closed:[ids], hidden:[ids]}` value: `move(layout, id, col, index)`, `nudge(layout, id, key)`, `hide`, `restore` (to the shorter column by rendered height, which the DOM layer passes in), `toggle`, `encode`, and the cookie strings. It never decodes: the page reads the layout off the rendered cards, so the shell's parser is the cookie's one reader. Reset drops the cookie. `node --test` covers them in `scripts/overview-layout.test.cjs`, loaded with `vm` the way `takeover.test.cjs` loads its module.
- **The DOM layer** uses event delegation on `[data-overview]`. It never uses an Alpine expression, so it is CSP-safe (CSP-mode Alpine silently rejects expressions in bindings):
  - Header click toggles the card.
  - Arrange flips a `data-arranging` attribute on the page root; CSS shows and hides grips, hide controls and the shelf.
  - Pointer drag lifts the card (fixed position, the dialog shadow) and leaves a dashed slot in Strong Hairline on Quiet Sand.
  - Arrow keys on a grip nudge the card.
  - An `aria-live` region announces each move.
- **Open/close motion:** a `grid-template-rows` 0fr↔1fr transition (300ms ease-out). Under reduced motion there is no transition.

### 6. Visual contract

DESIGN.md gains an **Overview cards** section, the source for the CSS in `verso.css`:

- **Page and cards:** a Quiet Sand page ground. Cards are Paper with a Strong Hairline frame and 2px corners, flat at rest. Only the card in flight casts the dialog shadow. The gap is 20px between cards and between columns.
- **Header:** 52px tall. A 16px Glyph icon, the title at 16px/600 Ink, the summary in Meta (shown only when closed), the status packet and word in the tone's Deep step, then 28px icon buttons: Go (arrow-up-right), the chevron, and in Arrange mode the grip and hide.
- **Body:** behind a Hairline, 20px inset.
- **Scope:** the frame is Overview-only, an exception to the flat direction stated where the flat direction is.

### 7. ADR-022 records the decisions

ADR-022, "Overview cards", records the card model, header-as-status, Arrange as a mode, the cookie and why it is the interim home, and the stream's frame selection. ADR-009's Overview paragraph gains a pointer to it.

## Risks / Trade-offs

- **[Layout doesn't follow the operator to another browser, or survives in a private window only until it closes]** → The default layout is complete and sensible; this is the cost of the interim store, and decision 3 keeps the move to a router-side store small.
- **[A hidden card's problem is out of sight]** → The verdict still counts it, and the Arrange shelf shows its packet. A hidden card's problem can't make the verdict read "online".
- **[Tables in half-width cards overflow on narrow screens]** → Each table scrolls inside its card. Below a card width of about 26rem the `Receive` and `Transmit` columns drop.
- **[Hide or restore mid-session leaves the stream sending stale frames until the swap]** → The swap follows immediately and reconnects the stream; frames for a card that isn't present find no hooks and are ignored.

## Migration Plan

No on-device state changes, so routers upgrade with nothing to migrate. Browsers without the cookie get the default layout. Rollback is a downgrade: an older shell ignores the cookie.
