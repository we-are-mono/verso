# Notes

## 3.2 — the Wi-Fi read in the lab (2026-10-19)

- The wireless repo's `deploy-dev.sh` installed `commands.json` root-owned, and the helper ran `wireless.state` for a session granted `verso-plugin wireless state`: the grant and the ownership walk pass.
- The container has no radio, so netifd has no `network.wireless` object: the read fails ("ubus call network.wireless status ended with exit status: 4"), the helper logs it, and the page shows what the config says. The Go read failed the same way there.
- With a stand-in `network.wireless` object (an rpcd exec plugin answering radio0 up with `default_radio0` on `phy0-ap0`) and radio0 enabled in the config, the helper answered `{"radios":{"radio0":{"up":true}},"networks":{"default_radio0":{"up":true}}}`. The page drew the network "up" and radio0 "on"; channel numbers stayed dashes, since iwinfo has no such device. The stand-in and the config change were removed afterwards.
- Screenshot: `~/Share/screenshots/shell-names-no-plugin-3.2-wifi-live-read.png`.
- A router with a radio is the check that the numbers (channel, power, busy share, clients) arrive.

## 4.6 — doors in the lab (2026-10-19)

- **Overview.** Internet and Ports: `/plugins/interfaces/` "Open Interfaces"; Tunnels: `/plugins/vpn/` "Open VPN"; System: `/system/services` "Open Services". The container has no board profile, so no Hardware card.
- **Access.** The lab's `mgmt` zone accepts input from the internet. The notice links `/plugins/firewall/zones` "Open Firewall".
- **Firewall log.** Probes from `verso-wan` (`scripts/dev-livelog-feed.sh`) log under Log-WAN-probes; each row's name links `/plugins/firewall/?open=cfg0d92bd`, which opens that rule's drawer.
- **Firewall plugin stopped.** A fresh Access keeps the warning with no link. With Activity open, rows that arrived after the rule mapping refreshed (the newest eight of 29) name the rule as plain text; the earlier rows keep their links.
- **VPN plugin stopped.** Tunnels: `/plugins/interfaces/` "Open Interfaces".
- Both plugins were started again afterwards.
- Screenshots: `~/Share/screenshots/shell-names-no-plugin-4.6-*.png` (overview, access, activity, rule-drawer, activity-plugin-stopped, overview-vpn-stopped).

## 5.5 — the rail in the lab (2026-10-19)

- Every plugin deployed, each manifest declaring its place (acme and qos have no nav entries).
- Rail: Overview, Devices, Network, Wireless, VPN, Firewall, System.
- Network open: Interfaces, DHCP, DNS, Routes, Dynamic DNS.
- System open: General, Hardware, Access, Packages, Services, Maintenance, Logs, Events, Changes, Diagnostics.
- Glyphs: Wireless and VPN wear the ones their entries name (wifi, lock), Firewall its section's (Security's shield).
- Screenshots: `~/Share/screenshots/shell-names-no-plugin-5.5-rail-network.png` and `-rail-system.png`.

## 6.4 — service words in the lab (2026-10-19): not observable here

- procd holds a reload trigger only for a running service with an instance. The container's ddns has both its hostnames disabled, so `/etc/init.d/ddns start` registers nothing, and openvpn is not installed. No staged change there can make the reload step name either.
- Enabling a DDNS hostname would run a real updater against a provider, so it was not done.
- `TestPendingPageNamesThePluginsServicesInTheirWords` renders the Pending page with ddns and openvpn changes staged, procd's triggers for both, and the two plugins' `services`: it reads "Reloads Dynamic DNS and OpenVPN."
- A router with an enabled DDNS hostname, or an OpenVPN profile, is where 6.4 is checked.

## 7.3 — what is left (2026-10-19)

- `graphify query "core Go naming a plugin id, path or nav label"` surfaces core's own vocabulary only: event kinds, theme labels, the helper's reset.
- Core's Go outside tests holds no `/plugins/<id>` path. Its remaining capitalised words that a plugin also uses are OpenWrt's: "Firewall" for the firewall service, "DNS" and "DHCP" for protocols.
- The guard (`internal/boundary`) reported a planted `pluginHref("firewall", …)` and `navRow.Label == "VPN"` at their lines, and passes on the tree.

## 8.2 — lint and tests (2026-10-19)

- `make lint`: 0 issues (golangci-lint, deadcode, clippy over the helper, the SDK and the in-tree plugins).
- `make test`: every Go package passes, the guard among them; the Rust crates pass 722 tests with none failing; node passes 79 of 84. The five that fail are `scripts/dev-grid.test.cjs`'s G-grid tests, another session's, as before this change.
