# interfaces-page Specification

## Purpose

The Interfaces page lists the router's devices with the networks they carry, and is where a row is read, brought up or down, restarted, deleted, and opened into the parts behind it: its networks and its device.

## Requirements

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
The drawer SHALL hold, in this order, a card for each network the row carries, and then a card for the device. Each card's header SHALL carry the part's glyph, its kind and its name as the config writes it.

#### Scenario: A bridge carrying the LAN
- **WHEN** the operator opens `br-lan`'s drawer
- **THEN** it holds a Network card for `lan`, then a Device card for `br-lan`

#### Scenario: A port on its own
- **WHEN** the operator opens `eth2`'s drawer
- **THEN** it holds one Device card for `eth2`

### Requirement: A card states its part's facts
Each card SHALL state its part's facts as a fact list. A network card SHALL state its IPv4, IPv6 and ULA addresses, protocol, uptime, firewall zone, and netifd's errors when it has any. A device card SHALL state its MAC address, link or ports, driver or spanning tree, MTU and error counts, and its addresses when the device carries no network.

#### Scenario: The LAN and its bridge
- **WHEN** the operator opens `br-lan`'s drawer
- **THEN** the `lan` card reads `192.168.1.1/24` and the firewall zone `lan`, and the `br-lan` card reads its MAC address with a copy act, its ports `eth0` and its MTU

### Requirement: A card's header carries the act that edits its part
Each card SHALL carry, on its header, the act that edits its part where the part can be edited: Edit network for a network and Edit device for a device. Each act SHALL lead to the address that edits that part.

#### Scenario: Editing the network from its card
- **WHEN** the operator presses Edit network on the `lan` card
- **THEN** lan's editor opens over the listing, at the address `/plugins/interfaces/edit?network=lan&device=`

### Requirement: A network's DHCP server stays on the DHCP page
Neither the drawer nor the interface's editor SHALL state or edit a network's DHCP server; the DHCP page alone does. The row's DHCP chip SHALL stand for a network that has a DHCP server, and for no other network.

#### Scenario: The LAN's server
- **WHEN** the operator opens `br-lan`'s drawer or lan's editor
- **THEN** neither names a DHCP server nor leads to the DHCP page, and `br-lan`'s row wears lan's DHCP chip

#### Scenario: The uplink
- **WHEN** `uplink` takes its address by DHCP and its `dhcp` section says not to serve there
- **THEN** `eth1`'s row carries no DHCP chip

### Requirement: A row has the same meaning of a DHCP server as the DHCP page
Whether a network has a DHCP server, and whether it could take one, SHALL be the same on the Interfaces page and the DHCP page: a network has a server when it has a `dhcp` section and either an address of its own or a section set to hand out addresses; it could take one when it has an address of its own and no section.

#### Scenario: Both pages agree
- **WHEN** a network appears in the DHCP page's Servers card
- **THEN** its Interfaces row wears the DHCP chip, and a network the DHCP page offers to Add DHCP server wears none
