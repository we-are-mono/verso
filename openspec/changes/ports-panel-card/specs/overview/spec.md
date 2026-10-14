# Spec Delta

## MODIFIED Requirements

### Requirement: Cards carry the Overview's readings
Every reading the Overview takes SHALL appear in exactly one card, and each card SHALL hold only its own kind of reading. Each address in the Internet card has a copy control.

#### Scenario: What each card holds
- **WHEN** the Overview renders with every card open
- **THEN** Internet holds the uplink's state, its device and how long it has been connected, and the IPv4 and IPv6 facts with their protocol
- **AND** Ports holds the device panel, then each physical port's network, current rate and link
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

## ADDED Requirements

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
