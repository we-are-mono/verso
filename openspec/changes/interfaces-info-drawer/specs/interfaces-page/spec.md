# Spec Delta

## Purpose

The Interfaces page lists the router's devices with the networks they carry, and is where a row is read, brought up or down, restarted, deleted, and opened into the parts behind it: its networks, their DHCP servers and its device.

## ADDED Requirements

### Requirement: A row is one line that opens its drawer from the info act
Each Interfaces row SHALL stand on one line and SHALL NOT unfold. Its acts SHALL stand in the order power, restart, info, delete, each where the row has it; the info act opens the row's drawer over the listing, and no row carries the edit pencil.

#### Scenario: A network row's acts
- **WHEN** the listing shows `br-lan`, which carries the live network `lan`
- **THEN** its acts read Bring down, Restart, Details (the info glyph) and Delete, and nothing under the row unfolds

#### Scenario: A port with no network
- **WHEN** the listing shows the unused port `eth2`
- **THEN** its info act opens its drawer, and it has no power, restart or delete act

### Requirement: The listing's columns
The Interfaces table SHALL show the columns Device · network, Type, Address, State and the acts, in that order. The MAC address SHALL NOT be a column of the table.

#### Scenario: Reading the listing
- **WHEN** the operator opens Interfaces
- **THEN** each row reads its device with its network chips, its type, its first IPv4 address, its state and its acts, and no MAC address

### Requirement: The drawer's address
A row's drawer SHALL have an address of its own naming the row's device (`/plugins/interfaces/details?device=<name>`), which opens the listing with that drawer in front of it; a name the listing does not hold SHALL say the interface no longer exists.

#### Scenario: Opening by address
- **WHEN** the operator follows `/plugins/interfaces/details?device=br-lan`
- **THEN** the listing stands behind the open drawer titled `br-lan`

#### Scenario: A stale address
- **WHEN** the address names a device the router no longer has
- **THEN** the page says the interface no longer exists and leads back to Interfaces

### Requirement: The drawer holds one card per part
The drawer SHALL hold, in this order, a card for each network the row carries, each followed by that network's DHCP card when it has one, and then a card for the device. Each card's header SHALL carry the part's glyph, its kind and its name as the config writes it.

#### Scenario: A bridge carrying the LAN
- **WHEN** the operator opens `br-lan`'s drawer
- **THEN** it holds a Network card for `lan`, a DHCP server card for `lan`, then a Device card for `br-lan`

#### Scenario: A port on its own
- **WHEN** the operator opens `eth2`'s drawer
- **THEN** it holds one Device card for `eth2`

### Requirement: A card states its part's facts
Each card SHALL state its part's facts as a fact list. A network card SHALL state its IPv4, IPv6 and ULA addresses, protocol, uptime, firewall zone, and netifd's errors when it has any. A device card SHALL state its MAC address, link or ports, driver or spanning tree, MTU and error counts, and its addresses when the device carries no network.

#### Scenario: The LAN and its bridge
- **WHEN** the operator opens `br-lan`'s drawer
- **THEN** the `lan` card reads `192.168.1.1/24` and the firewall zone `lan`, and the `br-lan` card reads its MAC address with a copy act, its ports `eth0` and its MTU

### Requirement: A card's header carries the act that edits its part
Each card SHALL carry, on its header, the act that edits its part where the part can be edited: Edit network for a network, Edit device for a device, and Configure DHCP for a DHCP server. Each act SHALL lead to the address that edits that part.

#### Scenario: Editing the network from its card
- **WHEN** the operator presses Edit network on the `lan` card
- **THEN** lan's editor opens over the listing, at the address `/plugins/interfaces/edit?network=lan&device=`

#### Scenario: Configuring the DHCP server
- **WHEN** the operator presses Configure DHCP on lan's DHCP server card
- **THEN** the DHCP page opens with lan's server drawer in front of it

### Requirement: A network's DHCP server has its card
A network that has a DHCP server SHALL have a DHCP card stating the server's state and, while it serves, its pool, lease time and active leases, carrying Configure DHCP. The row's DHCP chip SHALL stand for that network, and for no network without a server.

#### Scenario: The LAN's server
- **WHEN** the operator opens `br-lan`'s drawer
- **THEN** lan's DHCP card reads its state, pool, lease time `12h` and active leases, and `br-lan`'s row wears lan's DHCP chip

### Requirement: A network that could take a server offers one
A network with an address of its own and no DHCP server SHALL have a DHCP card saying it has no DHCP server, carrying Add DHCP server, which opens the DHCP page's blank server drawer with that network chosen. An uplink whose section says not to serve there SHALL have no DHCP card and no DHCP chip.

#### Scenario: A network that could take a server
- **WHEN** `guest` has a static address and no `dhcp` section
- **THEN** guest's DHCP card says it has no DHCP server and its Add DHCP server opens `/plugins/dnsdhcp/?open=guest`, and guest's row carries no DHCP chip

#### Scenario: The uplink
- **WHEN** the operator opens the drawer of `eth1`, which carries `uplink` by DHCP
- **THEN** it holds the Network card for `uplink` and the Device card, and no DHCP card

### Requirement: A row has the same meaning of a DHCP server as the DHCP page
Whether a network has a DHCP server, and whether it could take one, SHALL be the same on the Interfaces page and the DHCP page: a network has a server when it has a `dhcp` section and either an address of its own or a section set to hand out addresses; it could take one when it has an address of its own and no section.

#### Scenario: Both pages agree
- **WHEN** a network appears in the DHCP page's Servers card
- **THEN** its Interfaces row wears the DHCP chip and its drawer's DHCP card carries Configure DHCP, and when a network is offered by Add DHCP server its drawer's DHCP card carries Add DHCP server
