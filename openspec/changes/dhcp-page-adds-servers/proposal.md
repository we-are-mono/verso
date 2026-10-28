# Proposal

## Why

The DHCP page's one heading act is "Add reservation", but the page is about servers: its first card lists them, and its settings apply to every one of them. An act on the heading line acts on the page (DESIGN.md: acts sit at the level they act on), so the page should make servers there, and the Reservations card should make reservations on its own header. Today a server cannot be made at all: the Servers card lists every network with a static address, and a network with no server stands there as an "Off" row, so making one and switching one on read as the same thing.

## What Changes

- The DHCP page's heading act is **Add DHCP server**. It opens a blank server drawer whose first field is the network the server serves, offering the networks with a static address that have no server yet. The act stands only while such a network exists.
- The Servers card lists the networks that have a DHCP server. A server switched off stays listed as a switched-off row (sunk, with the "disabled" chip). A network that has no server is no longer drawn as an "Off" row, and the uplink's "do not serve here" section (OpenWrt's stock `config dhcp 'wan'` with `ignore 1`) is not a server and is not listed.
- A server's row can remove the server after an inline crimson confirm; the save stages the section's deletion.
- An address naming a network that can serve but has no server (`?open=<network>`, which the Interfaces page links to) opens the blank server drawer with that network chosen.
- **Add reservation** moves from the page's heading line to the Reservations card's header. A lease's Reserve still opens the blank reservation drawer filled from the lease.

## Capabilities

### New Capabilities
- `dhcp-page`: the DHCP page's listings and acts: which servers it lists and how, how a server is made and removed, and where a reservation is made from.

### Modified Capabilities

## Impact

- `plugins/verso-plugin-dnsdhcp/src/dhcp.rs` (page act, servers table, server drawer, opened/save routing, removal), `page.rs` (the new-server key), `hosts.rs` (the blank reservation drawer's host and the card's add).
- Shell: a page card's section `control` carries the Reservations add as a panel link. The blank reservation drawer rides the Reservations listing instead of the heading act, so the shell must draw a cell-less row that only carries an open drawer as just that drawer (the SDK Files editor already answers with one).
- No UCI renames: a new server writes `config dhcp '<network>'` with `option interface`, as OpenWrt names its own, so no migration ships.
- The Interfaces page and `verso_plugin::dhcp::Server::href` are unchanged; their link lands on the drawer it means whether or not the server exists.
- ADR-005 §8 (an object lives in its drawer; New opens it blank) is followed, not amended.
