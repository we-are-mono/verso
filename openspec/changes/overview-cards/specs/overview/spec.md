# Spec Delta

## Purpose

The Overview is the landing page: a verdict on the network over cards that each hold one kind of router reading, which an operator opens, closes, arranges and hides to suit what they check.

## ADDED Requirements

### Requirement: The Overview shows seven cards in two columns
The Overview SHALL show seven cards, each holding one kind of reading: Internet, Ports, Tunnels, Traffic, Resources, Hardware and Firmware. Cards stand in two columns, each column stacking its cards at their own heights. The default layout puts Internet, Ports and Tunnels on the left, and Traffic, Resources, Hardware and Firmware on the right.

#### Scenario: First visit
- **WHEN** a signed-in operator opens the Overview with no layout cookie
- **THEN** the left column shows Internet, Ports and Tunnels in that order
- **AND** the right column shows Traffic, Resources, Hardware and Firmware in that order
- **AND** every card is open

#### Scenario: Narrow screen
- **WHEN** the page is too narrow for two columns
- **THEN** the cards stand in one column, the left column's cards first, then the right column's

### Requirement: Each card's header carries its status
Every card header SHALL show the card's icon and title. Cards with a verdict (Internet, Ports, Tunnels) show the verdict at the header's end as a status word with its packet in the verdict's tone, whether the card is open or closed. Cards of readings (Traffic, Resources, Hardware, Firmware) show a status only when one of their readings is in a warning or danger band.

#### Scenario: Internet is up
- **WHEN** the uplink is up
- **THEN** the Internet header reads "Connected" in the success tone

#### Scenario: A port loses link
- **WHEN** a physical port that a network uses loses link
- **THEN** the Ports header reads "1 port without link" in the warning tone

#### Scenario: Readings are healthy
- **WHEN** every Resources reading is in its normal band
- **THEN** the Resources header shows no status word

### Requirement: A closed card says one reading after its title
A closed card SHALL show only its header, with one reading after the title: the one most worth a glance, never its status again, and never a reading that changes which it is from one second to the next. An open card hides the line, because its body carries the detail.

#### Scenario: Every card closed
- **WHEN** the operator closes every card
- **THEN** Internet shows the uplink's IPv4 address, Ports how many devices are connected, and Tunnels how many tunnels there are, or the down ones by name
- **AND** Traffic shows the current download and upload, Resources memory, Hardware the CPU's temperature, and Firmware the firmware the router runs

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
The Overview SHALL open with one verdict sentence tinted by tone: "Your network is online" (success), "Your network needs attention" (warning) or "Your network is offline" (danger). The page's live clock and time zone stand beside the verdict. The page shows no kicker over the verdict and no status-tile strip.

#### Scenario: The firewall is not running
- **WHEN** the uplink is up and fw4's ruleset is not loaded in the kernel
- **THEN** the verdict reads "Your network needs attention", though no card shows the firewall

#### Scenario: A problem anywhere
- **WHEN** the uplink is up and one port has lost link
- **THEN** the verdict reads "Your network needs attention" in the warning tone

### Requirement: Arrange mode moves and hides cards
The Overview SHALL offer an Arrange act on its heading line. In Arrange mode every card shows a drag handle and a hide control, and a shelf lists the hidden cards. Outside Arrange mode no card can be moved or hidden. Done ends Arrange mode.

#### Scenario: Entering and leaving Arrange
- **WHEN** the operator presses Arrange
- **THEN** each card shows a drag handle and a hide control, and the shelf of hidden cards appears
- **AND** the act reads Done until pressed again

### Requirement: Cards move by pointer or keyboard
In Arrange mode the operator SHALL move a card by dragging it to any place in either column, or by focusing its drag handle and pressing an arrow key. Up and Down move it within its column; Left and Right move it to the same position in the other column. Each move is announced to assistive technology with the card's column and position.

#### Scenario: Drag across columns
- **WHEN** the operator drags Hardware from the right column and drops it above Ports in the left column
- **THEN** Hardware stands above Ports in the left column

#### Scenario: Keyboard move
- **WHEN** focus is on the Internet card's drag handle and the operator presses ArrowDown
- **THEN** Internet moves one place down in its column
- **AND** a screen reader hears "Internet: left column, position 2."

### Requirement: Hidden cards come back from the shelf
Hiding a card SHALL remove it from its column and list it on the shelf. Restoring a card from the shelf puts it at the end of the shorter column. Reset layout restores the default layout, with every card shown and open.

#### Scenario: Hide and restore
- **WHEN** the operator hides Firmware and then restores it from the shelf
- **THEN** Firmware leaves the page and is listed on the shelf
- **AND** after restoring it stands at the end of the shorter column

### Requirement: The layout is kept per browser in a cookie
The browser SHALL keep each card's column, order, and open, closed or hidden state in one first-party cookie, scoped to the router's origin, sent to every Overview request and kept until the browser's own cookie limit. Nothing about the layout is stored on the router.

#### Scenario: Layout survives a reload
- **WHEN** the operator moves Traffic to the left column, closes Firmware and reloads the page
- **THEN** Traffic is in the left column and Firmware is closed

#### Scenario: Another browser
- **WHEN** the same operator opens the Overview in another browser
- **THEN** that browser shows the default layout

### Requirement: The page is rendered already in the saved layout
The shell SHALL read the layout cookie when it renders the Overview and render every card in its saved column, position and state, so no card moves after the page loads. This holds for a full load and for an in-app page swap.

#### Scenario: No reorder after load
- **WHEN** the browser has a layout cookie that puts Traffic first in the left column
- **THEN** the Overview's HTML already has Traffic first in the left column

### Requirement: A damaged or stale layout falls back safely
The shell SHALL treat the layout cookie as untrusted input. A cookie it can't parse gives the default layout. Card ids it doesn't know are dropped, and cards it has that the cookie doesn't name are added open at the end of the shorter column. A card named twice keeps its first place.

#### Scenario: Garbled cookie
- **WHEN** the layout cookie holds text that isn't a layout
- **THEN** the Overview renders the default layout without an error

#### Scenario: A card the cookie doesn't know
- **WHEN** the cookie names six of the seven cards, or names a card the Overview no longer has
- **THEN** the card it leaves out stands open at the end of the shorter column, and the card it names that isn't there is dropped

### Requirement: Hidden cards are neither rendered nor streamed
The shell SHALL NOT render a hidden card, and the Overview's live stream SHALL NOT send a frame that only hidden cards would read, nor take a reading that only such a frame needs. Readings the verdict needs are taken whatever is hidden. Closed cards still receive their frames, because their summaries are live.

#### Scenario: Resources hidden
- **WHEN** Resources is hidden
- **THEN** the live stream sends no system-meter frames

#### Scenario: Hardware hidden
- **WHEN** Hardware is hidden
- **THEN** the live stream sends no sensor frames and reads no hardware sensors

#### Scenario: A hidden card's problem still reaches the verdict
- **WHEN** Resources is hidden and memory enters its warning band
- **THEN** the verdict reads "Your network needs attention"
- **AND** in Arrange mode the shelf shows Resources with the warning packet

#### Scenario: Resources closed
- **WHEN** Resources is closed but not hidden
- **THEN** the live stream keeps sending system-meter frames, and the closed header's summary updates

### Requirement: Cards carry the Overview's readings
Every reading the Overview takes SHALL appear in exactly one card, and each card SHALL hold only its own kind of reading. Each address in the Internet card has a copy control.

#### Scenario: What each card holds
- **WHEN** the Overview renders with every card open
- **THEN** Internet holds the uplink's state, its device and how long it has been connected, and the IPv4 and IPv6 facts with their protocol
- **AND** Ports holds each physical port's network, current rate and link
- **AND** Tunnels holds each tunnel someone brought up, with its kind, current rate and state
- **AND** Traffic holds the Internet graph and its window choice
- **AND** Resources holds load, CPU, memory and storage
- **AND** Hardware holds the board and its CPU temperature, fan speed and power draw
- **AND** Firmware holds firmware, kernel and uptime

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
- **THEN** the router's uptime appears in the Firmware card and nowhere else on the page
