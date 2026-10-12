# Proposal

## Why

The Overview reads as one long sheet: a verdict, a strip of four status tiles, the IPv4/IPv6 facts, the Internet graph and the System readings, all stacked in a fixed order. An operator can't put what they check first at the top or fold away what they never read. Each reading also shows up twice, once in a tile and once in its detail further down.

## What Changes

The Overview becomes two columns of cards on a Quiet Sand page. Each card holds one kind of reading, and you can open, close, move and hide it. The design is the mock at https://claude.ai/artifact/DRTQcksAMhEJRRg7LtUT8Y.

- **Seven cards** carry what the Overview shows today, less the firewall's rule count, which told the operator nothing to act on; a firewall that isn't fully loaded still turns the verdict:
  - Internet: the uplink's state, its device and how long it has been connected, plus the IPv4 and IPv6 facts with copy controls.
  - Ports: each physical port's network, current rate and link.
  - Tunnels: each tunnel someone brought up, with its kind, rate and state.
  - Traffic: the Internet graph and its window choice.
  - Resources: load, CPU, memory and storage.
  - Hardware: the board and its CPU temperature, fan speed and power draw.
  - Firmware: firmware, kernel and uptime.
- **A card's header is its status.** The status word and its packet stand at the header's end whether the card is open or closed. A closed card adds a one-line summary, which is the old tile caption. The tile strip is removed.
- **The verdict heads the page alone.** "Your network is online" stays, tinted by tone. The kicker above it and the uptime beside the clock are removed: uptime moves into the Firmware card, and the card that has a problem names it.
- **Open and close:** clicking a card's header opens or closes it. A closed card stays closed when its status turns to a warning or failure; its header shows the change in the tone's colour.
- **Arrange mode:** an Arrange act on the heading line turns on drag handles, a hide control on each card, and a shelf of hidden cards with a reset. Done turns it off. Cards move by pointer drag or with the arrow keys on a handle.
- **The layout is kept per browser in a cookie.** The shell reads it when it renders the page, so cards arrive in place, with no reorder after load. A cookie with no layout, or a broken one, gives the default layout. Unknown card ids are dropped, and cards missing from the cookie are added.
- **A hidden card costs the router nothing.** It isn't rendered, and the live stream leaves out any frame that only hidden cards would read. A closed card keeps streaming, because its summary is live.
- **Narrow screens** show one column: the left cards, then the right.

There are no **BREAKING** changes to on-device state. Nothing in UCI, on `/data` or in the init script changes; the layout lives only in the browser's cookie.

## Capabilities

### New Capabilities
- `overview`: the landing page's cards, their content, their open/closed/hidden state and arrangement, how the layout is kept and read, and what the live stream sends for them.

### Modified Capabilities
None. The only existing spec, `release-packaging`, is untouched.

## Impact

- **ADRs:**
  - A new ADR-022, "Overview cards", records the card layout, the header-as-status rule, Arrange as a mode, and why the cookie was chosen over a file on the router.
  - ADR-009 §the read-only status baseline: the Overview stays shell-owned. Plugins don't contribute cards; a plugin card contract would be a change of its own.
  - ADR-019: the Internet graph and its window move into the Traffic card unchanged.
- **DESIGN.md:** a new "Overview cards" section states the Overview-only exception to the flat direction (Paper cards with a Strong Hairline frame on a Quiet Sand page ground), plus the card header anatomy and Arrange mode.
- **Code:**
  - `internal/widget/overview.go`, `overview_status.go`, `templates/overview.html.tmpl`, `templates/overview_masthead.html.tmpl`: the masthead, tiles and fact columns become cards.
  - `internal/server/server.go` (the Overview handler) and `internal/server/events.go` (the overview stream): read the layout cookie.
  - `internal/server/assets/verso-stream.js`: update cards in place.
  - A new shell JS module for open/close/arrange that works under Alpine CSP mode, and new styles in `assets/verso.css` / `input.css`.
- **i18n:** new strings (card titles, Arrange, Done, Hidden, Reset layout, the closed-card summaries, the drag-handle labels) go into the source catalog and `i18n/sl`.
- **Tests:** `internal/widget/overview_test.go` and the server's Overview and stream tests.
