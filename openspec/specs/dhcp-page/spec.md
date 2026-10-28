# dhcp-page Specification

## Purpose

The DHCP page lists the router's DHCP servers, the reservations they honour and the leases they hand out, and is where a server and a reservation are made, edited and removed.

## Requirements

### Requirement: The page's act makes a DHCP server
The DHCP page's heading line SHALL carry one act, "Add DHCP server", which opens a blank server drawer over the page. The act SHALL stand only while at least one network with a static address has no DHCP server; when every such network has one, the heading line SHALL carry no act.

#### Scenario: A network can take a server
- **WHEN** the network `guest` has a static address and no `dhcp` section serves it
- **THEN** the heading line shows "Add DHCP server", and pressing it opens the "New DHCP server" drawer with the page behind it

#### Scenario: Every network already has a server
- **WHEN** every network with a static address has a `dhcp` section
- **THEN** the heading line shows no act

### Requirement: A new server starts from the network it serves
The blank server drawer SHALL open with a Network choice as its first field, offering only the networks with a static address that have no DHCP server, followed by the same fields an existing server's drawer shows. Saving it SHALL stage a new `dhcp` section named after the network, with `option interface` set to it, and SHALL refuse a network that is not offered.

#### Scenario: Saving a new server
- **WHEN** the operator chooses `guest`, keeps the default pool and saves
- **THEN** the change staged is `config dhcp 'guest'` with `option interface 'guest'` and the pool's options, and the page lists `guest` among its servers

#### Scenario: A network that already has a server is refused
- **WHEN** a submission names `lan`, which already has a server
- **THEN** nothing is staged and the drawer stays open with the refusal on the Network field

### Requirement: An address naming a network without a server opens its blank server
An address that names a network with a static address and no DHCP server (`?open=<network>`) SHALL open the blank server drawer with that network chosen, so a link to a network's DHCP server always lands on a drawer for it.

#### Scenario: A link to a network with no server
- **WHEN** the operator follows `/plugins/dnsdhcp/?open=guest` and `guest` has no server
- **THEN** the "New DHCP server" drawer opens with `guest` chosen

#### Scenario: A stale name opens nothing
- **WHEN** the address names a network that does not exist
- **THEN** the page opens with no drawer

### Requirement: The Servers card lists the networks that have a server
The Servers card SHALL list one row per network that has a DHCP server: a `dhcp` section whose network has a static address, or one set to hand out addresses. A server that is switched off SHALL stay listed as a switched-off row. A network with no `dhcp` section, and an uplink's section that is set not to serve, SHALL NOT be listed.

#### Scenario: A switched-off server stays on the list
- **WHEN** `lan`'s section sets `ignore 1`
- **THEN** `lan` is listed as a switched-off row, reading as off, with its values intact

#### Scenario: The stock uplink section is not a server
- **WHEN** `wan` takes its address by DHCP and its `dhcp` section sets `ignore 1`
- **THEN** `wan` is not listed

#### Scenario: A network without a section is not listed
- **WHEN** `guest` has a static address and no `dhcp` section
- **THEN** `guest` is not listed, and "Add DHCP server" offers it

### Requirement: A server is removed from its row
Each server's row SHALL offer "Remove DHCP server", which asks an inline danger confirm naming the network and saying that its devices keep their addresses until their leases run out. Confirming SHALL stage the deletion of that `dhcp` section; the drawer and the page's settings are not touched.

#### Scenario: Removing a server
- **WHEN** the operator confirms "Remove DHCP server" on `guest`
- **THEN** the deletion of `dhcp.guest` is staged, and `guest` leaves the Servers card and returns to the networks "Add DHCP server" offers

#### Scenario: Keeping a server
- **WHEN** the operator answers the confirm with "Keep DHCP server"
- **THEN** nothing is staged

### Requirement: A reservation is made from the Reservations card
The Reservations card's header SHALL carry "Add reservation", which opens the blank reservation drawer over the page. A lease's Reserve act SHALL open the same drawer filled from that lease. An address naming a blank reservation SHALL open that drawer whether or not the page carries a heading act.

#### Scenario: Adding from the card
- **WHEN** the operator presses "Add reservation" on the Reservations card
- **THEN** the "New reservation" drawer opens with the page behind it

#### Scenario: A blank reservation opens with no heading act
- **WHEN** every network has a server, so the heading line has no act, and the operator opens `/plugins/dnsdhcp/?open=new`
- **THEN** the "New reservation" drawer opens

#### Scenario: Reserving a lease
- **WHEN** the operator presses Reserve on a lease for `aa:bb:cc:dd:ee:ff`
- **THEN** the "New reservation" drawer opens with that MAC and the lease's address filled in
