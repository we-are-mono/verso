# interfaces-page Specification

## Purpose

The Interfaces page lists the router's networks and devices in two cards, and is where a network or a device is read, brought up or down, restarted, deleted, and opened into its one drawer: its editor, with what the router reports first.

## Requirements

### Requirement: The page holds a Networks card and a Devices card
The Interfaces page SHALL hold two cards, Networks first and Devices second. Networks SHALL list one row per network the config or netifd holds, loopback and IPv6 companions left out. Devices SHALL list one row per device the config names or the router reports, `lo` left out.

#### Scenario: The lab router
- **WHEN** the config holds networks `lan` on `br-lan`, `mgmt` on `eth0`, `wan` and `wan6` on `wan0`, and a bridge `br-lan` with the port `lan0`
- **THEN** Networks lists `lan`, `mgmt` and `wan`, and Devices lists `br-lan`, `eth0`, `lan0` and `wan0`

#### Scenario: A network that is down
- **WHEN** `guest` is in the config and netifd reports it not active
- **THEN** Networks lists `guest` with the state `not active`

### Requirement: A device the router made on its own has no row
A software device that no config section names, that carries no network, and that is no bridge's port and no VLAN's parent SHALL have no row. A physical port SHALL always have a row.

#### Scenario: Kernel tunnel bases
- **WHEN** the kernel reports `sit0` and `ip6tnl0` and nothing in the config names or uses them
- **THEN** Devices lists neither

#### Scenario: An unused port
- **WHEN** the physical port `eth2` carries nothing
- **THEN** Devices lists `eth2` with the state `unused`

#### Scenario: A tunnel base put to use
- **WHEN** a network names `sit0` as its device
- **THEN** Devices lists `sit0`

### Requirement: The Networks card's columns
The Networks table SHALL show the columns Network, Protocol, Address, Device, Zone, State and the acts, in that order. Network SHALL read the network's name with its DHCP chip where it has a server; Address its first observed IPv4 address; Device the device it runs on; Zone its firewall zone.

#### Scenario: Reading the LAN
- **WHEN** the operator opens Interfaces
- **THEN** `lan`'s row reads `lan` with its DHCP chip, `static`, `192.168.77.1/24`, `br-lan`, `lan` and `up`

### Requirement: The Devices card's columns
The Devices table SHALL show the columns Device, Type, Used by, State and the acts, in that order. Used by SHALL name, in words as the config writes them, the networks the device carries, the bridge it is a port of, and the devices made on it, or a dash where nothing rests on it; a port SHALL carry its link speed after its type.

#### Scenario: A bridge and its port
- **WHEN** the operator reads `br-lan` and `lan0`
- **THEN** `br-lan` reads `bridge`, used by `lan`, and `lan0` reads `port` with its speed, used by `br-lan`

### Requirement: A row is one line that opens its drawer from its pencil
Each row SHALL stand on one line and SHALL NOT unfold. Its acts SHALL stand in the order power, restart, edit, delete, each where the row has it. The edit act (the pencil) SHALL open the row's drawer over the listing; no row carries an info act.

#### Scenario: A network row's acts
- **WHEN** the listing shows `lan`, which netifd reports up
- **THEN** its acts read Bring down, Restart, Edit (the pencil) and Delete

#### Scenario: A device row's acts
- **WHEN** the listing shows the port `lan0`, which belongs to `br-lan`
- **THEN** its only act is Edit, since a device in use is not deleted

#### Scenario: A configured device nothing uses
- **WHEN** the listing shows a VLAN device `eth1.20` that no network carries
- **THEN** its acts read Edit and Delete

### Requirement: An IPv6 companion folds into its network
A network whose protocol is DHCPv6 and whose device is `@<network>` or the same device as exactly one network of another protocol SHALL be that network's IPv6 companion. A companion SHALL have no row and no drawer of its own. A DHCPv6 network that could belong to more than one network, or that more than one DHCPv6 network contends for, SHALL keep its own row.

#### Scenario: The default internet line
- **WHEN** `wan` (DHCP) and `wan6` (DHCPv6) both run on `wan0`
- **THEN** Networks lists `wan` and no row for `wan6`

#### Scenario: A PPPoE line
- **WHEN** `wan` is PPPoE on `wan0` and `wan6` names `@wan` as its device
- **THEN** `wan6` is `wan`'s companion

#### Scenario: Two DHCPv6 networks on one device
- **WHEN** `wan6` and `wan6b` are both DHCPv6 on `wan0` beside `wan`
- **THEN** both keep their own rows and `wan`'s drawer has no companion

### Requirement: A network's row acts on its companion too
A network's row, and its device's, SHALL read the network's own state; its companion's state SHALL stand in the network's Status card. Bring down, Bring up and Restart SHALL act on the network and its companion together, and Delete SHALL remove both, its confirmation saying so.

#### Scenario: Restarting the internet line
- **WHEN** the operator presses Restart on `wan`'s row
- **THEN** netifd is asked to restart both `wan` and `wan6`

#### Scenario: A provider with no IPv6
- **WHEN** `wan` is up and `wan6` waits pending because the provider hands out no IPv6
- **THEN** `wan`'s row and `wan0`'s read `up`, and `wan`'s Status card reads its IPv6 state `pending`

#### Scenario: Deleting the line
- **WHEN** the operator deletes `wan`
- **THEN** the confirmation names `wan6` as removed with it, and both sections are staged for removal

### Requirement: The drawer's address
A network's drawer SHALL have the address `/plugins/interfaces/edit?network=<name>` and a device's `/plugins/interfaces/edit?device=<name>`, each opening the listing with that drawer in front of it. The address `/plugins/interfaces/details?device=<name>` SHALL open that device's drawer. A name the listing does not hold SHALL say the interface no longer exists. A companion's address SHALL open its network's drawer.

#### Scenario: Opening by address
- **WHEN** the operator follows `/plugins/interfaces/edit?network=lan`
- **THEN** the listing stands behind the open drawer titled `lan`

#### Scenario: An old details address
- **WHEN** the operator follows `/plugins/interfaces/details?device=br-lan`
- **THEN** the listing stands behind `br-lan`'s device drawer

#### Scenario: A companion's address
- **WHEN** the operator follows `/plugins/interfaces/edit?network=wan6`
- **THEN** `wan`'s drawer opens

#### Scenario: A stale address
- **WHEN** the address names a network or device the router no longer has
- **THEN** the page says the interface no longer exists and leads back to Interfaces

### Requirement: A network's drawer
A network's drawer SHALL hold, in order, a Status card, then cards General, IPv4, IPv6 and Advanced, then the `/etc/config/network` preview and Save. General SHALL stand untitled, as the drawer's title names the object, and hold Bring up at boot first, then Name, Runs on and Firewall zone. IPv4 SHALL hold the protocol and that protocol's fields.

#### Scenario: Opening the LAN
- **WHEN** the operator presses Edit on `lan`'s row
- **THEN** one drawer titled `lan` opens, its Status card first and the General card's first control Bring up at boot

#### Scenario: A refused save
- **WHEN** a save is refused for an address outside its netmask
- **THEN** the drawer stays open over what was typed, the IPv4 card's address carrying the refusal

### Requirement: A network's IPv6 card
The IPv6 card SHALL edit the network's companion where it has one: its header SHALL name the companion as the config writes it, and the card SHALL hold the companion's address request, prefix request and DNS-from-peer settings. A network with no companion SHALL hold its delegated prefix length there.

#### Scenario: The internet line's IPv6
- **WHEN** the operator opens `wan`'s drawer
- **THEN** its IPv6 card's header names `wan6`, and the card holds Request address, Request prefix and Use DNS from peer

#### Scenario: The LAN's IPv6
- **WHEN** the operator opens `lan`'s drawer
- **THEN** its IPv6 card holds Delegated prefix

### Requirement: One Save stages a network and its companion
Saving a network's drawer SHALL stage its own section and its companion's in one save, and the preview SHALL show both. Its firewall zone SHALL be written for both. A rename SHALL carry the companion's `@<network>` device reference to the new name.

#### Scenario: Moving the line to another zone
- **WHEN** the operator sets `wan`'s zone to `wan2` and saves
- **THEN** both `wan` and `wan6` are staged into the `wan2` zone and out of `wan`

#### Scenario: Renaming a PPPoE line
- **WHEN** the operator renames `wan` to `isp` and `wan6`'s device is `@wan`
- **THEN** `wan6`'s device is staged as `@isp`

### Requirement: The Status card states what the router reports
The Status card SHALL state live facts only, none of which the drawer's form sets, and SHALL be absent from a new object's drawer. A network's card SHALL state its state, its companion's state where it has one, IPv4, IPv6 and ULA addresses (its companion's among them), uptime, and netifd's errors when it has any; its header SHALL carry Restart and Bring down or Bring up. A device's card SHALL state its state, MAC address, link speed or a bridge's live ports, driver, and error counts.

#### Scenario: The LAN's status
- **WHEN** the operator opens `lan`'s drawer
- **THEN** its Status card reads `up`, `192.168.77.1/24` with a copy act and the uptime, and its header carries Restart and Bring down

#### Scenario: The internet line's addresses
- **WHEN** the operator opens `wan`'s drawer and `wan6` holds a global IPv6 address
- **THEN** that address stands in `wan`'s Status card

#### Scenario: A new network
- **WHEN** the operator presses Add network
- **THEN** the drawer has no Status card

### Requirement: A Status act returns to the listing
An act on a Status card SHALL run as the row's act does, SHALL stage nothing the form holds, and SHALL return to the listing, whose state column reads the change.

#### Scenario: Restarting from the drawer
- **WHEN** the operator has typed an MTU in `lan`'s drawer and presses Restart on its Status card
- **THEN** netifd is asked to restart `lan`, the MTU is not staged, and the listing stands with the notice

### Requirement: A device's drawer
A device's drawer SHALL hold a Status card, then its kind's settings card (a bridge's ports, spanning tree and IGMP snooping; a VLAN's parent device, VLAN ID and tag protocol), then Advanced (MTU and MAC override), the preview and Save. A device OpenVPN made SHALL keep its hand-off to that tunnel's panel in place of a form.

#### Scenario: Opening the bridge
- **WHEN** the operator presses Edit on `br-lan`'s row
- **THEN** one drawer titled `br-lan` opens with its Status card, then Ports, Spanning tree and IGMP snooping, then MTU and MAC override

#### Scenario: A plain port
- **WHEN** the operator opens `eth0`'s drawer
- **THEN** it holds its Status card and Advanced, and no kind's settings card

### Requirement: Each card adds its own kind
The page's heading act Add network SHALL open a new network's drawer whose Type offers Network, Internet connection and Tunnel. The Devices card's header SHALL carry Add device, which SHALL open a new device's drawer whose Type offers Bridge and VLAN.

#### Scenario: Adding a VLAN
- **WHEN** the operator presses Add device and keeps VLAN
- **THEN** the drawer asks for the parent device, VLAN ID and tag protocol, and its save stages a `device` section

#### Scenario: The network chooser
- **WHEN** the operator presses Add network
- **THEN** Type offers Network, Internet connection and Tunnel, and neither Bridge nor VLAN

### Requirement: A network's DHCP server stays on the DHCP page
A network's drawer SHALL NOT state or edit its DHCP server; the DHCP page alone does. A network row's DHCP chip SHALL stand for a network that has a DHCP server, and for no other network.

#### Scenario: The LAN's server
- **WHEN** the operator opens `lan`'s drawer
- **THEN** it names no DHCP server and leads nowhere on the DHCP page, and `lan`'s row wears its DHCP chip

#### Scenario: The uplink
- **WHEN** `uplink` takes its address by DHCP and its `dhcp` section says not to serve there
- **THEN** `uplink`'s row carries no DHCP chip

### Requirement: A row has the same meaning of a DHCP server as the DHCP page
Whether a network has a DHCP server, and whether it could take one, SHALL be the same on the Interfaces page and the DHCP page: a network has a server when it has a `dhcp` section and either an address of its own or a section set to hand out addresses; it could take one when it has an address of its own and no section.

#### Scenario: Both pages agree
- **WHEN** a network appears in the DHCP page's Servers card
- **THEN** its Interfaces row wears the DHCP chip, and a network the DHCP page offers to Add DHCP server wears none
