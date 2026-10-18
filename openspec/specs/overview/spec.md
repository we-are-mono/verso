# overview Specification

## Purpose

The Overview is the landing page: a verdict on the network over cards that each hold one kind of router reading, which an operator opens, closes, arranges and hides to suit what they check.

## Requirements

### Requirement: The Overview shows seven cards in two columns
The Overview SHALL show seven cards, each holding one kind of reading: Internet, Ports, Tunnels, Traffic, Resources, System and Hardware. Hardware stands only on a board that has a profile. Cards stand in two columns, each column stacking its cards at their own heights. The default layout puts Internet, Ports and Tunnels on the left, and Traffic, Resources, System and Hardware on the right. Each card a running plugin contributes to `shell.overview.cards` SHALL stand among them, in the default layout where its position puts it.

#### Scenario: First visit
- **WHEN** a signed-in operator opens the Overview with no layout cookie on a board that has a profile
- **THEN** the left column shows Internet, Ports and Tunnels in that order
- **AND** the right column shows Traffic, Resources, System and Hardware in that order
- **AND** every card is open

#### Scenario: First visit on a board with no profile
- **WHEN** a signed-in operator opens the Overview with no layout cookie on a board that has no profile
- **THEN** the right column shows Traffic, Resources and System in that order, and no Hardware card
- **AND** Arrange mode's shelf never lists Hardware

#### Scenario: A layout from before System
- **WHEN** the layout cookie names Firmware
- **THEN** System stands where Firmware stood, closed or hidden as that card was

#### Scenario: A layout that names Hardware
- **WHEN** the layout cookie names Hardware on a board that has a profile
- **THEN** Hardware stands where the cookie puts it, closed or hidden as the cookie says

#### Scenario: A layout that names Hardware on a board with no profile
- **WHEN** the layout cookie names Hardware on a board that has no profile
- **THEN** Hardware is dropped as a card the Overview does not have, and every other card keeps its place

#### Scenario: Narrow screen
- **WHEN** the page is too narrow for two columns
- **THEN** the cards stand in one column, the left column's cards first, then the right column's

#### Scenario: A plugin card in the default layout
- **WHEN** verso-plugin-acme contributes its Certificate card with position `after:system` and there is no layout cookie
- **THEN** the right column shows Traffic, Resources, System, Certificate and Hardware in that order

### Requirement: Each card's header carries its status
Every card header SHALL show the card's icon and title. Cards with a verdict (Internet, Tunnels) show the verdict at the header's end as a status word with its packet in the verdict's tone, whether the card is open or closed. The Ports card shows a status only when a port a network uses has lost link or its state is unknown, since its panel and list already show every port linked. Cards of readings (Traffic, Resources, System, Hardware) show a status only when one of their readings is in a warning or danger band.

#### Scenario: Internet is up
- **WHEN** the uplink is up
- **THEN** the Internet header reads "Connected" in the success tone

#### Scenario: A port loses link
- **WHEN** a physical port that a network uses loses link
- **THEN** the Ports header reads "1 port without link" in the warning tone

#### Scenario: Every port in use linked
- **WHEN** every physical port a network uses has link, and two spare ports have none
- **THEN** the Ports header shows no status word

#### Scenario: Readings are healthy
- **WHEN** every Resources reading is in its normal band
- **THEN** the Resources header shows no status word

#### Scenario: A board runs hot
- **WHEN** the profile's CPU temperature, "cluster", passes its own warn limit
- **THEN** the Hardware header reads "cluster is hot" in the warning tone

### Requirement: A closed card says one reading after its title
A closed card SHALL show only its header, with one reading after the title: the one most worth a glance, never its status again, and never a reading that changes which it is from one second to the next. An open card hides the line, because its body carries the detail.

#### Scenario: Every card closed
- **WHEN** the operator closes every card
- **THEN** Internet shows the uplink's IPv4 address, Ports how many devices are connected, and Tunnels how many tunnels there are, or the down ones by name
- **AND** Traffic shows the current download and upload, Resources memory, and System the firmware the router runs
- **AND** Hardware shows the CPU temperature

#### Scenario: Internet is down while closed
- **WHEN** the Internet card is closed and the uplink is down
- **THEN** its header shows no line after the title, since "Not connected" says it

#### Scenario: Closing Traffic
- **WHEN** the operator closes the Traffic card
- **THEN** its header shows the current download and upload rates in one line
- **AND** those rates keep updating while it stays closed

### Requirement: Clicking a header opens or closes the card
Clicking a card's header, or its chevron control, SHALL toggle the card between open and closed, outside Arrange mode. The chevron is a button whose accessible name says what it does ("Close Internet", "Open Internet") and whose expanded state matches the card's.

#### Scenario: Keyboard toggle
- **WHEN** keyboard focus is on a card's chevron and the operator presses Enter
- **THEN** the card closes and the chevron reports itself collapsed

### Requirement: A closed card stays closed when its status changes
A card's open or closed state SHALL change only by the operator's action. A closed card whose status turns to warning or danger stays closed and shows the new status in its header.

#### Scenario: Internet drops while closed
- **WHEN** the Internet card is closed and the uplink goes down
- **THEN** the card stays closed
- **AND** its header reads "Not connected" in the danger tone

### Requirement: One verdict heads the page
The Overview SHALL open with one verdict sentence tinted by tone, the most important true sentence on the page. With the uplink known and down it SHALL read "Your network is offline" (danger), whatever else is wrong. Otherwise, with exactly one cause it SHALL read that cause as a sentence that stands on its own, in the cause's tone; with two or more it SHALL read "N things need attention", in danger when any cause is danger and warning otherwise; with none and the uplink up it SHALL read "Your network is online" (success); and with nothing known it SHALL read "Your network at a glance" (neutral). A cause is a firewall not fully loaded, a port a network uses or a tunnel without link, a Resources reading out of its normal band, a Hardware reading past the board's own limit, or the cause a plugin card states; every card counts, hidden or not. The router's live clock and time zone stand in the System card as Local time, not beside the verdict. The page shows no kicker over the verdict and no status-tile strip.

#### Scenario: The firewall is not running
- **WHEN** the uplink is up, fw4's ruleset is not loaded in the kernel, and nothing else is wrong
- **THEN** the verdict reads "The firewall is not running" in the danger tone, though no card shows the firewall

#### Scenario: A problem anywhere
- **WHEN** the uplink is up, one port a network uses has lost link, and nothing else is wrong
- **THEN** the verdict reads "1 port without link" in the warning tone

#### Scenario: One problem names itself
- **WHEN** the uplink is up and storage enters its warning band, and nothing else is wrong
- **THEN** the verdict reads "Storage is high" in the warning tone

#### Scenario: A tunnel down reads without its card
- **WHEN** the uplink is up, one of two tunnels, wg0, has lost link, and nothing else is wrong
- **THEN** the verdict reads "Tunnel wg0 is down" in the danger tone, while the Tunnels header reads "1 of 2 down"

#### Scenario: Several tunnels down
- **WHEN** the uplink is up, two of three tunnels have lost link, and nothing else is wrong
- **THEN** the verdict reads "2 of 3 tunnels down" in the danger tone

#### Scenario: Several problems are counted
- **WHEN** the uplink is up, one port a network uses has lost link, and storage enters its warning band
- **THEN** the verdict reads "2 things need attention" in the warning tone

#### Scenario: Offline outranks every cause
- **WHEN** the uplink is down and storage is high
- **THEN** the verdict reads "Your network is offline"

#### Scenario: A hot board on a healthy network
- **WHEN** the uplink is up, nothing else needs attention, and the Hardware header reads "cluster is hot"
- **THEN** the verdict reads "cluster is hot" in the warning tone

#### Scenario: A plugin card's cause
- **WHEN** the uplink is up, nothing else needs attention, and acme's Certificate card states the cause "The certificate could not be renewed" in the warning tone
- **THEN** the verdict reads "The certificate could not be renewed" in the warning tone, whether the card is open, closed or hidden

### Requirement: Arrange mode moves and hides cards
The Overview SHALL offer an Arrange act on its heading line. In Arrange mode every card shows a drag handle and a hide control, and a shelf lists the hidden cards. Outside Arrange mode no card can be moved or hidden. Done ends Arrange mode.

#### Scenario: Entering and leaving Arrange
- **WHEN** the operator presses Arrange
- **THEN** each card shows a drag handle and a hide control, and the shelf of hidden cards appears
- **AND** the act, a glyph named "Arrange cards", is a filled check whose tip says Done until pressed again, and reports itself pressed

### Requirement: Cards move by pointer or keyboard
In Arrange mode the operator SHALL move a card by dragging it to any place in either column, or by focusing its drag handle and pressing an arrow key. Up and Down move it within its column; Left and Right move it to the same position in the other column. Each move is announced to assistive technology with the card's column and position.

#### Scenario: Drag across columns
- **WHEN** the operator drags System from the right column and drops it above Ports in the left column
- **THEN** System stands above Ports in the left column

#### Scenario: Keyboard move
- **WHEN** focus is on the Internet card's drag handle and the operator presses ArrowDown
- **THEN** Internet moves one place down in its column
- **AND** a screen reader hears "Internet: left column, position 2."

### Requirement: Hidden cards come back from the shelf
Hiding a card SHALL remove it from its column and list it on the shelf. Restoring a card from the shelf puts it at the end of the shorter column. Reset layout restores the default layout, with every card shown and open.

#### Scenario: Hide and restore
- **WHEN** the operator hides System and then restores it from the shelf
- **THEN** System leaves the page and is listed on the shelf
- **AND** after restoring it stands at the end of the shorter column

### Requirement: The layout is kept per browser in a cookie
The browser SHALL keep each card's column, order, and open, closed or hidden state in one first-party cookie, scoped to the router's origin, sent to every Overview request and kept until the browser's own cookie limit. Nothing about the layout is stored on the router.

#### Scenario: Layout survives a reload
- **WHEN** the operator moves Traffic to the left column, closes System and reloads the page
- **THEN** Traffic is in the left column and System is closed

#### Scenario: Another browser
- **WHEN** the same operator opens the Overview in another browser
- **THEN** that browser shows the default layout

### Requirement: The page is rendered already in the saved layout
The shell SHALL read the layout cookie when it renders the Overview and render every card in its saved column, position and state, so no card moves after the page loads. This holds for a full load and for an in-app page swap.

#### Scenario: No reorder after load
- **WHEN** the browser has a layout cookie that puts Traffic first in the left column
- **THEN** the Overview's HTML already has Traffic first in the left column

### Requirement: A damaged or stale layout falls back safely
The shell SHALL treat the layout cookie as untrusted input. A cookie it can't parse gives the default layout. Card ids it doesn't know are dropped, and cards it has that the cookie doesn't name are added open at the end of the shorter column. A card named twice keeps its first place. The card of a plugin that is installed but not answering SHALL keep its place in the layout, and the card of a plugin that is not installed SHALL be dropped.

#### Scenario: Garbled cookie
- **WHEN** the layout cookie holds text that isn't a layout
- **THEN** the Overview renders the default layout without an error

#### Scenario: A card the cookie doesn't know
- **WHEN** the cookie names six of the seven cards, or names a card the Overview no longer has
- **THEN** the card it leaves out stands open at the end of the shorter column, and the card it names that isn't there is dropped

#### Scenario: A plugin restarting
- **WHEN** the operator has moved acme's Certificate card to the top of the left column, and acme's service is restarting while the Overview loads
- **THEN** the Overview draws without the card, the operator's next arrangement keeps its place, and once acme answers again the card stands at the top of the left column

#### Scenario: A plugin removed
- **WHEN** the cookie names acme's Certificate card and verso-plugin-acme is not installed
- **THEN** the card is dropped from the layout, and the next layout the page writes does not name it

### Requirement: Hidden cards are neither rendered nor streamed
The shell SHALL NOT render a hidden card, and the Overview's live stream SHALL NOT send a frame that only hidden cards would read, nor take a reading that only such a frame needs. Readings the verdict needs are taken whatever is hidden. Closed cards still receive their frames, because their summaries are live.

#### Scenario: Resources hidden
- **WHEN** Resources is hidden
- **THEN** the live stream sends no system-meter frames

#### Scenario: A hidden card's problem still reaches the verdict
- **WHEN** Resources is hidden and memory enters its warning band, and nothing else is wrong
- **THEN** the verdict reads "Memory is high"
- **AND** in Arrange mode the shelf shows Resources with the warning packet

#### Scenario: Resources closed
- **WHEN** Resources is closed but not hidden
- **THEN** the live stream keeps sending system-meter frames, and the closed header's summary updates

#### Scenario: Hardware hidden
- **WHEN** Hardware is hidden
- **THEN** the live stream sends no sensor frames, and the shell still reads the sensors for the verdict
- **AND** in Arrange mode the shelf shows Hardware in its header's tone

### Requirement: Cards carry the Overview's readings
Every reading the Overview takes SHALL appear in exactly one card, and each card SHALL hold only its own kind of reading. Each address in the Internet card has a copy control.

#### Scenario: What each card holds
- **WHEN** the Overview renders with every card open
- **THEN** Internet holds the uplink's state, its device and how long it has been connected, and the IPv4 and IPv6 facts with their protocol
- **AND** Ports holds the device panel, then each physical port's network, current rate and link
- **AND** Tunnels holds each tunnel someone brought up, with its kind, current rate and state
- **AND** Traffic holds the Internet graph, download and upload in two lanes, with its window choice in the card's header
- **AND** Resources holds load, CPU, memory and storage
- **AND** System holds the board, its architecture and target platform, the firmware, kernel, uptime and the local time with its zone
- **AND** Hardware, on a board that has a profile, holds the CPU temperature, the main power rail and the main fan

#### Scenario: No IPv6
- **WHEN** no IPv6 prefix was delegated
- **THEN** the Internet card's IPv6 block shows the hollow packet and "No prefix was delegated." in place of its rows

#### Scenario: A spare port
- **WHEN** a physical port carries no network
- **THEN** the Ports card lists it with its link state, live, and no rate
- **AND** its lost link does not count against the Ports card's status

#### Scenario: No tunnels
- **WHEN** no tunnel has been brought up
- **THEN** the Tunnels card's header reads "None observed" and its body says that no tunnel is up

#### Scenario: Uptime is shown once
- **WHEN** the Overview renders
- **THEN** the router's uptime appears in the System card and nowhere else on the page

### Requirement: The Traffic card's window rides its header
The Traffic card's header SHALL carry the Internet graph's window as a compact dropdown before the card's open-close control: Last minute, Last 24 hours, Last 7 days, Last 30 days and Last year, the live minute chosen when the page opens. Choosing a window SHALL redraw the card's graph alone and SHALL NOT open or close the card.

#### Scenario: Choosing a day
- **WHEN** the operator picks Last 24 hours in the Traffic card's header
- **THEN** both lanes redraw over the last 24 hours with their averages and peaks
- **AND** the card stays open

#### Scenario: A closed card
- **WHEN** the Traffic card is closed
- **THEN** its header still shows the window, and choosing one leaves the card closed

#### Scenario: Arranging
- **WHEN** the operator is in Arrange mode
- **THEN** the Traffic card's window is hidden, as the header's other controls are

### Requirement: The Ports card's table follows the panel
The Ports card's table SHALL list the same ports as the device panel above it, in the panel's left-to-right order, so a DSA conduit is neither drawn nor listed and does not count toward the card's status.

#### Scenario: The DK's order
- **WHEN** the Ports card renders on the Mono Gateway DK
- **THEN** its table rows read eth1, eth2, eth0, eth3, eth4, matching the panel above

#### Scenario: A DSA conduit
- **WHEN** a board's switch chip is linked through the conduit `eth0`
- **THEN** the Ports card's table has no `eth0` row
- **AND** `eth0` does not count toward "ports without link"

#### Scenario: Ports card closed
- **WHEN** the Ports card is closed
- **THEN** neither the panel nor the table is shown, and the header shows its one reading

### Requirement: The Hardware card shows the board's headline readings
The Hardware card SHALL show at most three rows, in this order, under the names the profile gives them: the CPU temperature (the thermal entry flagged `main`), the main power rail (flagged `main`) with its draw, voltage and current, and the main fan (flagged `main`). A reading the board does not have has no row. Each value stands at the row's right edge, so a long name keeps one line. Every other reading stays on the Hardware page.

#### Scenario: The Mono Gateway DK
- **WHEN** the Overview renders on the DK with Hardware open
- **THEN** the card shows cluster with its temperature, USB Power Delivery as "9.8 W · 20.10 V · 0.49 A", and System Fan 1 with its speed, in that order
- **AND** USB Power Delivery stands on one line

#### Scenario: A board with no main fan
- **WHEN** the board's profile flags no fan as main and none is running
- **THEN** the card shows the CPU and the power rail, and no fan row

#### Scenario: No entry answers
- **WHEN** the board has a profile but none of its headline readings answers
- **THEN** the Hardware card's body says that no sensor answered, and its header reads "Unavailable" in the neutral tone

### Requirement: Each Hardware reading shows its value or why it has none
Each Hardware row SHALL show its reading in its unit: the CPU in °C, the power rail in W followed by V and A where the chip reports them, the fan in rpm. A CPU reading the chip marks faulted shows "—". A fan reading zero shows "—"; with its fault or alarm set, it is a fan fault.

#### Scenario: A fan stops
- **WHEN** System Fan 1 reads 0 rpm and its chip reports a fault
- **THEN** its row shows "—" and the Hardware header reads "System Fan 1 stopped" in the warning tone

#### Scenario: A rail with no current reading
- **WHEN** the main rail's chip reports power and voltage but no current
- **THEN** its row shows the power and the voltage only

### Requirement: The Hardware header names a hot CPU, then a stopped fan
The Hardware header SHALL grade the CPU temperature against its own limits only: its thermal zone's trip points, or its chip's own limits. Past the warn limit is the warning tone, past the critical limit the danger tone; a CPU with no limits is never out of band. A hot CPU is named before a stopped fan.

#### Scenario: A critical CPU
- **WHEN** cluster passes its critical trip point while the main fan has stopped
- **THEN** the Hardware header reads "cluster is hot" in the danger tone

#### Scenario: No limits
- **WHEN** a CPU with no limits of its own reads 95 °C
- **THEN** the Hardware header shows no status for it

### Requirement: Hardware readings refresh every five seconds
While the Hardware card is shown, open or closed, the Overview SHALL refresh its readings and its header every five seconds. Every open Overview reads the same reading for a given five seconds, however many browsers have it open.

#### Scenario: The CPU warms
- **WHEN** the Hardware card is open and the CPU temperature rises
- **THEN** its row shows the new value within five seconds, without a reload

#### Scenario: Two browsers
- **WHEN** two browsers show the Overview with Hardware open
- **THEN** the router reads its sensors at most once every five seconds for both

### Requirement: The Hardware card opens the Hardware page
The Hardware card SHALL link to the Hardware page, which shows every reading the board exposes, those the profile lists and those it does not.

#### Scenario: Following the card
- **WHEN** the operator follows the Hardware card's link
- **THEN** the Hardware page opens

### Requirement: A plugin card has the shell card's anatomy
A contribution to `shell.overview.cards` SHALL answer with a card's title, glyph by the shell's name for it, tone, status, summary, optional cause, optional page it opens on the plugin, and a body in the closed widget vocabulary. The shell SHALL draw its header, its open and closed states, its Go link and Arrange's grip and hide exactly as for its own cards.

#### Scenario: Closed plugin card
- **WHEN** the operator closes acme's Certificate card, whose summary is `router.example.com`
- **THEN** the closed header reads Certificate followed by `router.example.com`, as a closed shell card reads its summary

#### Scenario: A body outside the vocabulary
- **WHEN** a plugin card's body holds a form
- **THEN** the Overview draws without the card, the card keeps its place in the layout, and the shell logs a compatibility warning naming the plugin and the contribution

### Requirement: A plugin card reads as of the page's load
The shell SHALL fetch each plugin card once each time it draws the Overview, hidden cards included, so their causes reach the verdict. The Overview's live stream SHALL NOT dial plugins.

#### Scenario: The stream runs
- **WHEN** the Overview has been open for ten minutes with acme's card on it
- **THEN** acme was asked for its card once, when the page was drawn

### Requirement: The layout cookie names plugin cards
The layout cookie SHALL be able to name a plugin card by its contribution's full id, `<plugin id>.<contribution id>`. The shell SHALL read a cookie written in the earlier format as before. The page SHALL write the version that can name plugin cards, keeping the ids of installed plugins' cards that did not draw.

#### Scenario: An existing layout after the upgrade
- **WHEN** a browser holds a layout written before plugin cards, with Traffic first on the left and System closed
- **THEN** after the upgrade the Overview draws Traffic first on the left and System closed

#### Scenario: Saving a layout with a plugin card
- **WHEN** the operator moves acme's Certificate card to the left column and reloads
- **THEN** the card stands in the left column
