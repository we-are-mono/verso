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
names. These are the readings the profile flags `main`, the ones
that lead the Hardware page's Temperatures, Power and Cooling cards, and the
card's verdict is the page heading's. Every other reading is the Hardware
page's, a glance's worth on the card and the detail one click away. A board
with no profile has guesses, not a curation, so it has no Hardware card; its
layout drops the card wherever a saved value names it.

The firewall has no card. A card that said "Active" and counted configured
rules told the operator nothing they could act on, so the page keeps only the
fact that matters: a ruleset the kernel has not fully loaded is a cause the
verdict names ("The firewall is not running"). The rule count, and the UCI read
behind it every second, are gone.

A card is built by its own functions in `internal/widget/overview_cards.go`:
its header (`head`), its body (`body`) and the page it opens. A card's header
carries no control: a choice of what a card shows (the Traffic card's window)
stands in its body over the graph it chooses, and folds away with the card,
which a choice in the header would outlive.
The registry is the shell's. A plugin adds a card by contributing to the slot
`shell.overview.cards` (ADR-026). It answers with the card's header in the
shell card's anatomy (title, Lucide icon, tone, status, summary, cause, the page
its door opens) and a body in the closed vocabulary, with no form, since a card
reads and never asks. The shell draws the header, the folds, the door and
Arrange's grip and hide exactly as for its own cards. A plugin card's cause
counts in the heading like any card's, hidden or not, so every installed
plugin's card is asked once each time the page is drawn. The stream never asks a
plugin: the session keeps the headers the page read, and every frame says them
again. A card that answers nothing (204), fails, or holds a form is not drawn,
and its place in the layout holds while its plugin is installed.

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
strip and the verdict's kicker are gone.

### The heading names what is wrong

The verdict is the most important true sentence on the page. An uplink that is
down outranks everything: "Your network is offline". Otherwise one cause is the
heading in its own words and tone ("Storage is high", "Tunnel wg0 is down",
"cluster is hot"), several are counted ("2 things need attention", in danger
when one of them is), and nothing wrong on a working uplink is "Your network is
online". A heading that only said something needed attention made the reader
hunt the cards for it, and named a full disk or a hot CPU a network problem.

Each card that can be out of band says its cause beside its header status, as
a sentence that stands without the card (`cardHead.Cause`). Where the header's
words stand alone, the cause is the same; Tunnels' "1 of 2 down" names no
tunnel, so its cause names the one down or counts several. The firewall, which
has no card, is a cause of its own.

Every card counts whether or not it is hidden, so hiding a card never quiets
the heading, and Hardware counts like the rest: the heading names what is
wrong, not only what is wrong with the network, and Hardware grades only
against the board's own limits. A hidden Hardware card's sensors are still
read for it; only its frames stop. A card stays closed when its status turns;
its header shows the change.

### Arrange is a mode

Opening and closing a card is a click on its header. Moving and hiding are
behind Arrange, an act on the heading line that becomes Done: at rest nothing
on the page moves or disappears by accident. Arranging shows a grip and a hide
on every card and a shelf of the hidden cards, each wearing its card's tone.
Cards move by pointer drag or by the arrow keys on a grip, and each move is
announced with its column and position.

### The layout is kept per browser, in a cookie

The layout is one first-party cookie, `verso_cards`, valued `2.` and the
base64url JSON `{"cols":[[…],[…]],"closed":[…],"hidden":[…]}`, at most 3 KB
(`Path=/`, `SameSite=Lax`, kept 400 days). A card's id is a shell card's
(`traffic`) or a plugin card's full contribution id (`acme.card`); JSON carries
the plugin's id, whose hyphens version 1's separator could not. Version 1,
`1.<left>.<right>.<closed>.<hidden>` with shell ids joined by `-`, is still read
and is never written. The page keeps the place of an installed plugin's card
that did not draw this time (a hidden `data-overview-kept` mark where it stood),
so its next write keeps it; a card of a plugin no longer installed is unknown and
dropped. It is never `Secure`: a router
answers over HTTP and HTTPS alike, and a `Secure` cookie written over HTTPS is
one an HTTP page of the same router can neither read nor overwrite, so its
layout would stop saving. The layout was kept as `verso_overview`, `Secure`
over HTTPS; the shell reads that name while a browser carries no
`verso_cards`, and the page drops it with every write. The page's script
writes it and never reads it; the shell reads it before it renders, so
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
and takes the readings the verdict counts. Hiding a card takes it off the
page in place and reconnects the stream on the new layout; restoring or
resetting reloads the page, which renders the card and reconnects it too.

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
- The page reloads when a card comes back, since the shell renders it; Arrange
  carries over the reload in `sessionStorage`. A hidden card leaves in place.
- A layout cookie rides every request to the router; it is under 200 bytes.
- A change of interface topology reaches the stream's card heads at the next
  connection, which an apply's reload makes.
