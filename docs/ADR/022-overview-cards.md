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

The Overview is seven cards in two columns: Internet, Ports and Tunnels on the
left by default; Traffic, Resources, System and Hardware on the right. Each
card holds one kind of reading, and every reading a card shows stands in
exactly one card. System is the router itself: its board, architecture, target
platform, firmware, kernel, uptime and local time with its zone (both move
into it from beside the verdict). A browser's layout that names the Firmware
card of an earlier Overview reads it as System, where it stood.

Hardware is the board's headline readings, and stands only on a board whose
profile lists sensors: the CPU temperature, the main power rail's draw,
voltage and current, and the main fan, one row each under the profile's
names. These are the readings the profile flags `cpu` and `main`, the ones
the Hardware page's tiles lead with. Every other reading is the Hardware
page's, a glance's worth on the card and the detail one click away. A board
with no profile has guesses, not a curation, so it has no Hardware card; its
layout drops the card wherever a saved value names it.

The firewall has no card. A card that said "Active" and counted configured
rules told the operator nothing they could act on, so the page keeps only the
fact that matters: a ruleset the kernel has not fully loaded makes the verdict
read that something needs attention. The rule count, and the UCI read behind
it every second, are gone.

A card is built by its own functions in `internal/widget/overview_cards.go`:
its header (`head`), its body (`body`) and the page it opens. The registry is
the shell's. A plugin contributes no card: a plugin card would need a contract
of its own, as the entity panel's tabs have, and is a decision of its own.

Bodies compose the existing widgets (`Properties`, `Meter`, `Traffic`,
`Table`, `Ports`). The Ports and Tunnels listings key their rows `interface:<name>` with
the `rx-rate`, `tx-rate` and `state` cells the interface stream already
writes, so no new cell kind or client code was needed for them to stay live.

The Ports card opens with the router's panel (`Ports`): the board profile's
own art, or the panel the shell generates for a board without art, every
port in its place on the case. The listing under it follows the panel's
left-to-right order, so the card has one order, and a DSA conduit, the
switch's link to the SoC, is neither drawn nor listed nor counted. The panel
carries no legend; each port's detail on hover or focus says what its LEDs
show. It is built only while the card is shown, from the same snapshot and
WAN state as the rest of the page.

### The header is the card's status

A card's header carries its verdict: the status word and packet at its end,
open or closed, in the tone's Deep step. A card of readings (Traffic,
Resources, System, Hardware) speaks only when a reading is out of band, and
Ports only when a port in use lost link: every port linked is what its panel
and list already show, and a header that said so too was fluff. A
closed card adds one reading after its title, the one most worth a glance and
never its status again: the uplink's address, the devices connected, the
tunnels down, the traffic, memory (always memory, so the line never swaps one
reading for another as they pass), the firmware, the CPU temperature. Hardware
grades the CPU only against its own limits (its zone's trip points or its
chip's), never a guessed one, and names a hot CPU before a stopped fan. The tile
strip and the verdict's kicker are gone: the page says once that something
needs attention, and the card with the problem says which.

The verdict counts what the cards read whether or not a card is hidden, so
hiding a card never makes the page read "online". Hardware is the exception:
the verdict speaks for the network, and a warm board is not a network that
needs attention, so Hardware's status never moves it. A card stays closed when
its status turns; its header shows the change.

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
only hidden cards would read (`meters` for Resources, `wan` for Traffic and
Internet, `interfaces` for Ports and Tunnels, `sensors` for Hardware) and takes
no reading only such a frame needs; it always sends `clock` and `overview`,
and takes the readings the verdict counts. Hiding, restoring or resetting
reloads the page, which reconnects the stream on the new layout.

Most of a board's sensors are i2c transactions behind a mux, and the bus is
slow and shared. One read of them serves every page render and every stream
connection for five seconds, and the `sensors` frame carries that read's rows
each second, by key. While Hardware is hidden nothing reads them, since the
verdict does not count them.

The stream reads the interfaces' topology once per connection and lays each
second's states and rates onto it, so its card heads are the first paint's.

## Consequences

- The Overview is the one page whose cards fold and move; every page is made
  of cards, and DESIGN.md's "Overview cards" states what is the
  Overview's own.
- A layout does not follow an operator to another browser, and a private
  window keeps it only while it is open.
- The page reloads when a card is hidden or comes back; Arrange carries over
  the reload in `sessionStorage`.
- A layout cookie rides every request to the router; it is under 200 bytes.
- A change of interface topology reaches the stream's card heads at the next
  connection, which an apply's reload makes.
