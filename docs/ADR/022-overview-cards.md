# ADR-022 — Overview cards

- **Status:** Accepted
- **Date:** 2026-10-12
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-004 (the overview stream), ADR-005 (the closed widget
  set), ADR-009 (the shell owns the read-only status baseline), ADR-012
  (translated strings), ADR-019 (the Internet graph and its past windows)

## Context

The Overview was one fixed sheet: a verdict with a kicker, a strip of four
status tiles, the IPv4/IPv6 facts, the Internet graph and the System readings,
in that order for everyone. An operator could not put first what they check
first, or fold away what they never read. Each tile also said in brief what
the sheet said again further down.

## Decision

### One kind of reading per card

The Overview is eight cards in two columns: Internet, Firewall, Ports and
Tunnels on the left by default; Traffic, Resources, Hardware and Firmware on
the right. Each card holds one kind of reading, and every reading the page
takes stands in exactly one card. Uptime moves from beside the clock into
Firmware.

A card is built by its own functions in `internal/widget/overview_cards.go`:
its header (`head`), its body (`body`) and the page it opens. The registry is
the shell's. A plugin contributes no card: a plugin card would need a contract
of its own, as the entity panel's tabs have, and is a decision of its own.

Bodies compose the existing widgets (`Properties`, `Meter`, `Traffic`,
`Table`). The Ports and Tunnels listings key their rows `interface:<name>` with
the `rx-rate`, `tx-rate` and `state` cells the interface stream already
writes, so no new cell kind or client code was needed for them to stay live.

### The header is the card's status

A card's header carries its verdict: the status word and packet at its end,
open or closed, in the tone's Deep step. A card of readings (Traffic,
Resources, Hardware, Firmware) speaks only when a reading is out of band. A
closed card adds the old tile's caption as one line after its title. The tile
strip and the verdict's kicker are gone: the page says once that something
needs attention, and the card with the problem says which.

The verdict counts what the cards read whether or not a card is hidden, so
hiding a card never makes the page read "online". A card stays closed when its
status turns; its header shows the change.

### Arrange is a mode

Opening and closing a card is a click on its header. Moving and hiding are
behind Arrange, an act on the heading line that becomes Done: at rest nothing
on the page moves or disappears by accident. Arranging shows a grip and a hide
on every card and a shelf of the hidden cards, each wearing its card's tone.
Cards move by pointer drag or by the arrow keys on a grip, and each move is
announced with its column and position.

### The layout is kept per browser, in a cookie

The layout is one first-party cookie, `verso_overview`, valued
`1.<left>.<right>.<closed>.<hidden>`, each field card ids joined by `-`
(`Path=/`, `SameSite=Lax`, `Secure` over HTTPS, kept 400 days). The page's
script writes it and never reads it; the shell reads it before it renders, so
the page arrives arranged and nothing moves after load. The shell treats it as
untrusted: a value it cannot read is the default layout, unknown cards are
dropped, a card named twice keeps its first place, and a card the value leaves
out comes back open at the end of the column with fewer cards.

The cookie is the layout's interim home. Kept on the router per user (next to
`history.db`), the layout would follow an operator to another browser, at the
cost of a write endpoint and state on the device. The move replaces one
`layoutSource` implementation (`cookieLayout`); the renderer, the stream and
the page's script write and read the layout through it unchanged.

### The stream follows the layout

The overview stream reads the layout once per connection. It sends no frame
only hidden cards would read (`meters` for Resources, `sensors` for Hardware,
`wan` for Traffic and Internet, `interfaces` for Ports and Tunnels) and takes
no reading only such a frame needs; it always sends `clock` and `overview`,
and takes the readings the verdict counts. Hiding, restoring or resetting
reloads the page, which reconnects the stream on the new layout.

The stream reads the interfaces' topology once per connection and lays each
second's states and rates onto it, so its card heads are the first paint's.

## Consequences

- The Overview is the one page with cards, and the one exception to the flat
  direction's frame rule; DESIGN.md's "Overview cards" states it.
- A layout does not follow an operator to another browser, and a private
  window keeps it only while it is open.
- The page reloads when a card is hidden or comes back; Arrange carries over
  the reload in `sessionStorage`.
- A layout cookie rides every request to the router; it is under 200 bytes.
- A change of interface topology reaches the stream's card heads at the next
  connection, which an apply's reload makes.
