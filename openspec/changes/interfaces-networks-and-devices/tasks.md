# Tasks

Every task is test-first: write the named test, see it fail, then the code.
Every task ends green under `make lint` and `make test`.

## 1. Companion resolution in the model

- [x] 1.1 Test `a_dhcpv6_network_on_the_same_device_is_its_networks_companion` (same-device `wan`/`wan6`, `@wan` under PPPoE), then add `Model::companion` and `Model::companion_of` in `model.rs`; verify the test passes
- [x] 1.2 Test `contended_dhcpv6_networks_stay_their_own` (two DHCPv6 on one device; one DHCPv6 beside two candidates), then cover contention in the resolver; verify the test passes

## 2. Networks and Devices cards

- [x] 2.1 Test `the_page_holds_networks_then_devices` (lab fixture: Networks `lan`, `mgmt`, `wan`; Devices `br-lan`, `eth0`, `lan0`, `wan0`; no `wan6`, no loopback), then split `page.rs`'s inventory into a networks table and a devices table in a stack of two sections; verify the test passes
- [x] 2.2 Test `networks_columns_read_name_protocol_address_device_zone_state` (lan with DHCP chip, `static`, `192.168.77.1/24`, `br-lan`, `lan`, `up`), then build the network rows; replace `the_listing_reads_device_type_address_and_state`; verify the test passes
- [x] 2.3 Test `devices_columns_read_device_type_used_by_state` (`br-lan` used by `lan`, `lan0` a port with speed used by `br-lan`, a VLAN used by its network), then build the device rows; verify the test passes
- [x] 2.4 Test `kernel_made_devices_without_a_use_have_no_row` (`sit0`, `ip6tnl0` hidden; unused physical `eth2` listed `unused`; `sit0` listed once a network names it), then filter the device rows; verify the test passes
- [x] 2.5 Test `rows_open_their_drawer_from_the_pencil` (network acts power, restart, edit, delete; `lan0` only edit; unused configured VLAN edit and delete; no info act anywhere), then set the acts and each row's `panel` to its `/edit` address; replace `a_row_is_one_line_that_opens_its_drawer_from_the_info_act`; verify the test passes
- [x] 2.6 Test `a_networks_row_reads_and_acts_on_its_companion` (`partial` when `wan6` is down; restart posts commands for `wan` and `wan6`; delete stages both and its confirmation names `wan6`), then extend `state`, `editor::action` and `editor::delete`; verify the test passes
- [x] 2.7 Retire the shell's `interfaces` table style: test in `internal/widget` that a table with style `interfaces` renders as the default flat table, then remove the branch in `table.go`, the `table.interfaces` template and `table_interfaces_test.go`; verify `go test ./internal/widget/...`
- [x] 2.8 Update `docs/plugins.md`'s Interfaces inventory section (two cards, columns, row acts, no `interfaces` style) and `DESIGN.md`'s "An interface's drawer" paragraph for the rows; verify both read as what the code does

## 3. One drawer per object, Status on top

- [x] 3.1 Test `status_card_states_live_facts_only` (lan: state, IPv4 with copy, uptime; no protocol, zone or MTU; wan's card carries `wan6`'s global IPv6; br-lan: MAC, live ports, errors), then move the fact helpers from `page.rs` into a new `status.rs` and build both Status cards; verify the test passes
- [x] 3.2 Test `a_network_drawer_reads_status_general_ipv4_ipv6_advanced` (card order; Bring up at boot first in General; no Status in a new drawer; refused save keeps the drawer over what was typed), then reorder `editor::drawer` into those cards and prepend Status for an existing object; verify the test passes
- [x] 3.3 Test `a_device_drawer_reads_status_kind_advanced` (br-lan: Ports, Spanning tree, IGMP snooping; eth0: Status and Advanced only; an OpenVPN device keeps its hand-off), then build the device drawer; verify the test passes
- [x] 3.4 Test `status_acts_run_and_return_to_the_listing` (Restart on lan's Status card posts `restart=lan` with the editor's form to `/edit?network=lan`, stages nothing, answers with the notice, the `interface-restart` command and Back to the listing; `wan`'s adds `wan6`'s), then add the header buttons and the command check in `editor::save`; verify the test passes
- [x] 3.5 Check the two-button Status header in the dev container (`make dev`, open `lan`'s drawer at 1440 and 390 px); if the buttons do not sit on the header line, fix the header's CSS for a stacked control, rebuild and commit `internal/server/assets/verso.css`; verify with a screenshot in `~/Share/screenshots`
- [x] 3.6 Test `drawer_addresses` (`/edit?network=lan` opens lan; `/details?device=br-lan` opens br-lan's device drawer; `/edit?network=wan6` opens wan's; an unknown name says the interface no longer exists), then route them and remove `page::details`; verify the test passes
- [x] 3.7 Update `DESIGN.md`'s interface drawer paragraph for one drawer per object with Status first, and `docs/plugins.md`'s drawer sentence; verify neither names the details drawer or Edit network

## 4. The IPv6 card and the companion's save

- [ ] 4.1 Test `wans_ipv6_card_edits_wan6` (header meta `wan6`; Request address, Request prefix, Use DNS from peer filled from `wan6`; lan's IPv6 card holds Delegated prefix), then build the IPv6 card from the companion; verify the test passes
- [ ] 4.2 Test `one_save_stages_the_network_and_its_companion` (saving `wan` stages `wan` and `wan6`; the preview shows both blocks; a refused `wan6` field lands on its own field), then extend `Values::posted`, `validate` and `operations`; verify the test passes
- [ ] 4.3 Test `zone_and_rename_carry_the_companion` (zone `wan2` stages both into it and out of `wan`; renaming `wan` to `isp` stages `wan6`'s device `@isp`), then extend `relationships`; verify the test passes

## 5. Add acts by kind

- [ ] 5.1 Test `add_interface_offers_network_internet_and_tunnel` (Type options; `/new?kind=bridge` offers Bridge and VLAN; reshaping never crosses from one list to the other), then split `KINDS` into network and device kinds; update `adding_an_interface_opens_a_new_network_with_its_type_to_change`; verify the test passes
- [ ] 5.2 Test `the_devices_card_carries_add_device` (the Devices section's control is a panel link to `/new?kind=bridge`; saving a new VLAN stages a `device` section), then add the control; verify the test passes
- [ ] 5.3 Update `docs/plugins.md`'s new-drawer sentence (two choosers) and `DESIGN.md`; verify they list the same kinds as the code

## 6. Integration

- [ ] 6.1 Walk the lab router in the dev container: open each row, restart `wan` from its Status card, change `lan`'s zone and Apply, add and delete a VLAN; verify each outcome reaches the top-right notification and the listing follows, with screenshots in `~/Share/screenshots`
- [ ] 6.2 Update `docs/BACKLOG.md`: the DK check of the Interfaces page (a PPPoE `wan` with `wan6` on `@wan`, restart from the Status card, a real port's link speed); keep the owner's uncommitted edits in that file intact; verify `git diff docs/BACKLOG.md` shows only the added entry beside them
- [ ] 6.3 Run `graphify update .`, `make lint` and `make test`; verify all green

## Workflow follow-up

- Before archiving, update the Purpose of `openspec/specs/interfaces-page/spec.md` (it still says a row is a device with the networks it carries) and confirm `DESIGN.md`, `docs/plugins.md` and `docs/BACKLOG.md` reflect what landed.
- Archive with `/opsx:archive interfaces-networks-and-devices` after the DK check.
