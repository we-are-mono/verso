# Spec Delta

## RENAMED Requirements

- FROM: `### Requirement: The Overview shows six cards in two columns`
- TO: `### Requirement: The Overview shows seven cards in two columns`

## MODIFIED Requirements

### Requirement: The Overview shows seven cards in two columns
The Overview SHALL show seven cards, each holding one kind of reading: Internet, Ports, Tunnels, Traffic, Resources, System and Hardware. Hardware stands only on a board that has a profile. Cards stand in two columns, each column stacking its cards at their own heights. The default layout puts Internet, Ports and Tunnels on the left, and Traffic, Resources, System and Hardware on the right.

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

### Requirement: Each card's header carries its status
Every card header SHALL show the card's icon and title. Cards with a verdict (Internet, Ports, Tunnels) show the verdict at the header's end as a status word with its packet in the verdict's tone, whether the card is open or closed. Cards of readings (Traffic, Resources, System, Hardware) show a status only when one of their readings is in a warning or danger band.

#### Scenario: Internet is up
- **WHEN** the uplink is up
- **THEN** the Internet header reads "Connected" in the success tone

#### Scenario: A port loses link
- **WHEN** a physical port that a network uses loses link
- **THEN** the Ports header reads "1 port without link" in the warning tone

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

### Requirement: Hidden cards are neither rendered nor streamed
The shell SHALL NOT render a hidden card, and the Overview's live stream SHALL NOT send a frame that only hidden cards would read, nor take a reading that only such a frame needs. Readings the verdict needs are taken whatever is hidden. Closed cards still receive their frames, because their summaries are live.

#### Scenario: Resources hidden
- **WHEN** Resources is hidden
- **THEN** the live stream sends no system-meter frames

#### Scenario: A hidden card's problem still reaches the verdict
- **WHEN** Resources is hidden and memory enters its warning band
- **THEN** the verdict reads "Your network needs attention"
- **AND** in Arrange mode the shelf shows Resources with the warning packet

#### Scenario: Resources closed
- **WHEN** Resources is closed but not hidden
- **THEN** the live stream keeps sending system-meter frames, and the closed header's summary updates

#### Scenario: Hardware hidden
- **WHEN** Hardware is hidden
- **THEN** the live stream sends no sensor frames and the shell reads no sensor for the Overview
- **AND** in Arrange mode the shelf shows Hardware in the neutral tone

### Requirement: Cards carry the Overview's readings
Every reading the Overview takes SHALL appear in exactly one card, and each card SHALL hold only its own kind of reading. Each address in the Internet card has a copy control.

#### Scenario: What each card holds
- **WHEN** the Overview renders with every card open
- **THEN** Internet holds the uplink's state, its device and how long it has been connected, and the IPv4 and IPv6 facts with their protocol
- **AND** Ports holds each physical port's network, current rate and link
- **AND** Tunnels holds each tunnel someone brought up, with its kind, current rate and state
- **AND** Traffic holds the Internet graph and its window choice
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

## ADDED Requirements

### Requirement: The Hardware card shows the board's headline readings
The Hardware card SHALL show at most three rows, in this order, under the names the profile gives them: the CPU temperature (the entry flagged `cpu`), the main power rail (flagged `main`) with its draw, voltage and current, and the main fan (flagged `main`). A reading the board does not have has no row. Each value stands at the row's right edge, so a long name keeps one line. Every other reading stays on the Hardware page.

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

### Requirement: The Hardware card stays out of the verdict
The Hardware card's status SHALL NOT change the page's verdict, which speaks for the network.

#### Scenario: A hot board on a healthy network
- **WHEN** the uplink is up, nothing else needs attention, and the Hardware header reads "cluster is hot"
- **THEN** the verdict reads "Your network is online"

### Requirement: The Hardware card opens the Hardware page
The Hardware card SHALL link to the Hardware page, which shows every reading the board exposes, those the profile lists and those it does not.

#### Scenario: Following the card
- **WHEN** the operator follows the Hardware card's link
- **THEN** the Hardware page opens
