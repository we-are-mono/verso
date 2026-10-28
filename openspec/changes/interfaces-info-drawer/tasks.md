# Tasks

## 1. SDK: one meaning of a DHCP server

- [x] 1.1 Write tests in `plugins/verso-plugin-sdk/src/dhcp.rs`: a static network with an address and a section `exists()`; one with an address and no section `can_be_added()`; the stock uplink section (`proto dhcp`, `ignore 1`) neither; a section set to serve on a network with no address `exists()`. Make them pass by giving `Server` the section, address and serving facts and the two methods, and verify with `cargo test` for the SDK and `make lint`
- [x] 1.2 Write a test in `plugins/verso-plugin-dnsdhcp/src/dhcp.rs` that the Servers card and Add DHCP server agree with `Server::exists` and `Server::can_be_added` on the fixture plus a free network. Make `State::has_server` and `State::free` read the SDK rule, keep every existing DHCP page test green, and verify with `make test` and `make lint` in the plugin repo

## 2. Interfaces: the row

- [ ] 2.1 Write tests in `plugins/verso-plugin-interfaces/src/tests.rs`: `br-lan`'s acts are Bring down, Restart, Details (info glyph, href `/plugins/interfaces/details?device=br-lan`) and Delete; no row carries the pencil or `expanded`; `eth2` carries only Details; the row's `panel` is the details address. Replace `listing_uses_live_addresses_and_expandable_details` with the one-line row's expectations, make them pass in `page.rs`, and verify with `cargo test`
- [ ] 2.2 Write a test that the table's columns are Device · network, Type, Address, State and acts, with no MAC column. Make it pass, and verify with `cargo test`
- [ ] 2.3 Write a test that a row's DHCP chip stands only where `Server::exists()` holds: `lan` wears it, `uplink` does not. Make it pass, and verify with `cargo test`

## 3. Interfaces: the drawer

- [ ] 3.1 Write tests: `/details?device=br-lan` answers the listing with an open drawer titled `br-lan` closed on `/plugins/interfaces/`; a device the router lacks answers "This interface no longer exists". Add the route in `main.rs` and `page::details`, and verify with `cargo test`
- [ ] 3.2 Write tests: `br-lan`'s drawer holds, as top-level sections, Network `lan` (IPv4 `192.168.1.1/24`, protocol, uptime, zone `lan`), DHCP server `lan`, then Device `br-lan` (MAC address with copy, ports `eth0`, MTU), each a fact list; `eth2`'s holds one Device card; `eth1`'s holds Network `uplink` and Device `eth1` and no DHCP card. Replace `an_expanded_row_reads_as_the_uci_sections_behind_it` with these, make them pass by turning `details` into the drawer's cards, and verify with `cargo test`
- [ ] 3.3 Write tests: the Network card carries Edit network to `/plugins/interfaces/edit?network=lan&device=`, the Device card Edit device to `/plugins/interfaces/edit?network=&device=br-lan`, lan's DHCP card Configure DHCP to `/plugins/dnsdhcp/?open=lan`, and a static network with no section (added to the fixture as `guest`) carries Add DHCP server to `/plugins/dnsdhcp/?open=guest` and says it has no DHCP server. Rework `a_dhcp_server_reads_as_its_state_and_how_full_its_pool_is` and `a_disabled_dhcp_server_says_so_once` to read the drawer's cards, make them pass, and verify with `cargo test` and `make lint`
- [ ] 3.4 Add the drawer's new words ("Details", "MAC address", "No DHCP server", "Add DHCP server", the stale-address notice if new) to `plugins/verso-plugin-interfaces/i18n/` catalogs, and verify with the shell's i18n audit in `make test`

## 4. Shell and SDK: no unfolding rows

- [ ] 4.1 Remove `TableRow.expanded` from the SDK and the three `expanded: Vec::new()` in `plugins/verso-plugin-firewall`, and verify with `make lint` and `make test`
- [ ] 4.2 Write a shell test that a table row decoded from JSON carrying `expanded` renders as a plain row with no unfold control and no sheet. Remove `Expanded`/`Expandable` from `internal/widget/table.go`, the unfold markup from `table.html.tmpl` and `table-interfaces.html.tmpl`, `versoExpandRow` and its hooks from `verso-tables.js`, and `__expand`/`__sheet` from `blocks/table.css`; rework `TestInterfaceTreeKeepsBranchesThroughExpandedDescendants` and the other tests that drew unfolding rows, rebuild `verso.css`, and verify with `make lint` and `make test`
- [ ] 4.3 Update DESIGN.md: the "Expanded row" entry gives way to the Interfaces drawer (one card per part, each with its edit act on its header), the row-opener rule lists Interfaces among the info rows, and the SDK's `SectionWidget.icon` doc stops naming expanded rows. Update `docs/plugins.md` where it names an expanded row, remove the backlog's "Interfaces rows open an info drawer" item, and verify with `rg -i "expanded row|expand-row|unfold" DESIGN.md docs plugins/verso-plugin-sdk/src` finding nothing stale

## 5. Integration

- [ ] 5.1 With `make dev` running, open Interfaces, check that no row unfolds, open `br-lan`'s drawer by its info act and by address, follow each card's act (Edit network, Edit device, Configure DHCP, and Add DHCP server on a network without one), and save screenshots to ~/Share/screenshots

## Workflow follow-up

- Archive once the owner has tried the drawer in the browser.
- The Interfaces listing moving into the shell stays its own backlog item.
