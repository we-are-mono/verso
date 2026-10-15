# Spec Delta

## MODIFIED Requirements

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

## ADDED Requirements

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
