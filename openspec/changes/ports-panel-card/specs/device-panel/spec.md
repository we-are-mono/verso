# Spec Delta

## Purpose

Draws the router's physical ports as a panel the operator recognises, from the board's own art or generated from the ports Verso finds, with link and traffic LEDs that follow the ports' live state.

## ADDED Requirements

### Requirement: The panel stands where ports are read
The device panel SHALL open the Overview's Ports card and System › Hardware, live on both. The Hardware page's panel stays live whatever the Overview's layout hides.

#### Scenario: Ports hidden on the Overview
- **WHEN** the operator has hidden the Ports card and opens System › Hardware
- **THEN** the Hardware page's panel lights and blinks from live state

### Requirement: A board's own art is its panel
When the board's profile ships panel art, the panel SHALL be that art, with each port lit from live state through the art's tagged port groups. A board without art SHALL get a generated panel of its ports.

#### Scenario: The Mono Gateway DK
- **WHEN** the panel renders on a board whose profile id is `mono,gateway-dk`
- **THEN** it shows the DK's rear art with eth1, eth2, eth0, eth3 and eth4 from left to right

#### Scenario: A board with no profile
- **WHEN** the panel renders on a board no profile describes
- **THEN** it shows a generated panel with one connector per port Verso finds

### Requirement: Only physical ports appear on the panel
The panel SHALL show each wired physical network port and nothing else: no wireless, bridge, VLAN, tunnel, loopback or other virtual device, and no DSA conduit (the CPU port that links a switch chip to the SoC).

#### Scenario: A DSA router
- **WHEN** a board's switch chip exposes `lan1` to `lan4` and `wan` through the conduit `eth0`
- **THEN** the panel shows `wan` and `lan1` to `lan4`
- **AND** it does not show `eth0`

#### Scenario: A virtual machine
- **WHEN** an x86 virtual machine has the virtio NICs `eth0` and `eth1` and the bridge `br-lan`
- **THEN** the panel shows `eth0` and `eth1` as RJ45 connectors and does not show `br-lan`

### Requirement: Ports stand in the board's physical order
The panel SHALL order ports by the board profile's left-to-right list when one exists. Without a profile it SHALL take the order of OpenWrt's board description (the WAN device, then the LAN ports in the order listed), then place every other port after them in natural name order.

#### Scenario: Profile order
- **WHEN** the DK's profile lists `eth1, eth2, eth0, eth3, eth4`
- **THEN** the panel draws them in that order, not in kernel numbering

#### Scenario: Board description order
- **WHEN** a board has no profile and its board description names `wan` as the WAN device and `lan1, lan2, lan3, lan4` as LAN ports
- **THEN** the panel draws `wan, lan1, lan2, lan3, lan4`

#### Scenario: Natural order
- **WHEN** a board has neither a profile nor a board description that names its ports, and its ports are `eth10`, `eth2` and `eth1`
- **THEN** the panel draws `eth1, eth2, eth10`

### Requirement: Each port is drawn as the connector it is
A generated panel SHALL draw a port as an SFP+ cage when the device tree wires its MAC to an SFP cage or its driver reports a fibre or direct-attach port, and as an RJ45 jack otherwise.

#### Scenario: An empty SFP+ cage
- **WHEN** a port's MAC is wired to an SFP cage that holds no module
- **THEN** the port is drawn as an SFP+ cage

#### Scenario: A copper port
- **WHEN** a port's driver reports twisted pair, or reports nothing
- **THEN** the port is drawn as an RJ45 jack

### Requirement: Connector art is one family
Both connector kinds SHALL share one outline, lip and pair of LEDs under the mouth. An RJ45 jack has a dark mouth with eight contact pins at its top. An SFP+ cage is shorter, its top lower than a jack's, with a dark mouth holding the connector socket. A generated panel is a chassis holding its ports in one row, drawn in the shell's palette in light and dark themes; board art keeps its own finish.

#### Scenario: A mixed panel
- **WHEN** a generated panel holds RJ45 and SFP+ ports
- **THEN** all their lips and LEDs stand on one line
- **AND** the SFP+ cages' tops stand lower than the jacks'

### Requirement: Each port says its name and speed
Each port on a generated panel SHALL carry its interface name and its negotiated link speed under the connector, the speed showing a dash while the port has no link. On board art and generated panels alike, each port SHALL be reachable by keyboard, and hovering or focusing it SHALL show its detail: name, role, address and link.

#### Scenario: A gigabit link
- **WHEN** `lan1` links at 1000 Mb/s
- **THEN** its label reads `lan1` with `1G` under it

#### Scenario: No link
- **WHEN** `lan2` has no cable
- **THEN** its speed reads as a dash

#### Scenario: Keyboard on the DK's art
- **WHEN** the operator tabs to `eth0` on the DK's panel
- **THEN** `eth0`'s detail shows above it, naming its role, address and link
- **AND** Escape hides it

### Requirement: The router's panel carries no legend
The router's panel SHALL draw no key to its LEDs or connector kinds: the LEDs read as LEDs, and each port's detail on hover or focus says what they show.

#### Scenario: The Ports card
- **WHEN** the Overview's Ports card shows the panel
- **THEN** no legend stands between the panel and the list of ports

### Requirement: The link LED follows the cable live
A port's link LED SHALL be lit while the port has link and dark while it has none, on board art and generated panels alike, and SHALL change within one second of a cable being plugged in or pulled out, with no reload.

#### Scenario: Pulling a cable
- **WHEN** the panel is on screen and the operator pulls the cable from `eth2`
- **THEN** within one second `eth2`'s link LED goes dark and its speed reads as a dash

### Requirement: The traffic LED blinks with the port's packets
A port's traffic LED SHALL show one of five activity levels from its packets per second in both directions: dark at none; an occasional blip below 10; a steady blink below 1,000; a fast flicker below 100,000; and a near-solid flicker at 100,000 or more. Board art and generated panels blink by the same rule. Ports blinking at one level SHALL NOT blink in step.

#### Scenario: An idle port
- **WHEN** a linked port moves no packets
- **THEN** its traffic LED is dark

#### Scenario: A trickle
- **WHEN** a port moves 3 packets a second
- **THEN** its traffic LED blips occasionally

#### Scenario: A download
- **WHEN** a port moves 50,000 packets a second
- **THEN** its traffic LED flickers fast

#### Scenario: Two busy ports
- **WHEN** two ports are at the same activity level
- **THEN** their traffic LEDs do not light at the same instants

### Requirement: Reduced motion stills the traffic LED
When the reader prefers reduced motion, a port's traffic LED SHALL stay lit while the port moves any packets and dark while it moves none.

#### Scenario: Reduced motion during a download
- **WHEN** the reader prefers reduced motion and a port moves 50,000 packets a second
- **THEN** its traffic LED is lit and does not blink

### Requirement: The WAN port is marked
The port carrying the WAN SHALL be marked on the panel, on board art and generated panels alike; the role comes from the live network configuration, never from the art.

#### Scenario: WAN on eth0
- **WHEN** the WAN interface's device is `eth0`
- **THEN** `eth0`'s connector carries the WAN mark and no other port does

### Requirement: The panel fits its card in one row
The panel SHALL keep every port in one row at the width of the card holding it, drawing the whole panel smaller as ports grow in number or the card narrows. Labels shrink with the panel; the port's hover and focus detail always names it at full size.

#### Scenario: An eight-port box in a narrow card
- **WHEN** a board with eight ports renders in a card narrower than eight full-size connectors
- **THEN** the eight connectors stand in one row, smaller, with no horizontal scroll

### Requirement: The panel's height stands on the 8px grid
The panel's drawn art SHALL be a multiple of 8px tall at every width, rounding down to the next 8px step as it scales, with its width following from its proportions. Its labels write on 24px lines, so the whole panel starts and ends on grid lines. A board's art is drawn edge to edge in its box, so its top and bottom edges are the box's.

#### Scenario: Board art at any width
- **WHEN** the DK's art renders in a card of any width
- **THEN** the art's drawn height is a multiple of 8px
- **AND** its chassis's top and bottom edges stand on the box's top and bottom

#### Scenario: Room for the art's own size
- **WHEN** the card has room for the art at its native size
- **THEN** the art is drawn at exactly that size, every edge it draws on a grid line, and never larger

#### Scenario: Narrowing the window
- **WHEN** the operator narrows the window so the panel shrinks
- **THEN** the panel's height steps down by multiples of 8px and never lands between grid lines

### Requirement: The panel stands at the card's left
The panel SHALL start at the left edge of its card's content, as the card's text and tables do, never centred; any width the art leaves over stays on its right.

#### Scenario: A five-port board in a wide card
- **WHEN** a generated five-port panel renders in a card wider than it needs
- **THEN** the first port's left edge stands on the card content's left edge
- **AND** the spare width lies to the right of the last port

### Requirement: A plugin's ports widget keeps its boolean activity
The `ports` widget SHALL accept a port's activity either as a level from 0 to 4 or as the boolean `active`; `active: true` with no level reads as the steady blink, and a level wins when both are sent.

#### Scenario: An installed plugin sends the boolean
- **WHEN** a plugin's view sends a port with `"active": true` and no level
- **THEN** the port's traffic LED shows the steady blink
