# Tasks

Every task ends with `make lint` passing, and `make test` passing except the five known `scripts/dev-grid.test.cjs` tests, which belong to another session. A task that touches a plugin repository commits there, in the order design.md's Migration Plan gives.

## 1. The SDK: the clock check and a root half's ubus

- [x] 1.1 Move `plugins/verso-plugin-system/src/datetime.rs` to `plugins/verso-plugin-sdk/src/datetime.rs`, exported as `verso_plugin::datetime`, and move its tests from `verso-plugin-system/src/tests.rs` beside it.
  - Point the System plugin at the SDK, and `verso-rpcd/src/main.rs` at the SDK's file by path; the helper keeps its own boundary test (design decision 8).
  - Verify with `cargo test` in the SDK, the System plugin and `verso-rpcd`, and `make lint`.
- [x] 1.2 Test first in the SDK's `root` module, against a stand-in program:
  - `root::ubus(object, method, args)` returns the parsed JSON object;
  - it fails, with a reason, on a non-zero exit, on output that is not one JSON object, and on output over 1 MiB, which it stops reading;
  - it gives the child no stdin.

  Then implement it (design decision 6). Document it under "Your root half" in `docs/plugins.md`. Verify with `cargo test` in the SDK.

## 2. Rates split at the narrow space

- [x] 2.1 In `plugins/verso-plugin-qos` (its own repo), test first in `summaries.rs`: a capped device's download and upload read `25\u{202f}Mb/s`, and an uncapped one reads `No limit`. Then write the rates with U+202F. Verify with `cargo test` and `cargo clippy` there, and commit there.
- [x] 2.2 Test first in `internal/server/device_limits_test.go`:
  - `25` U+202F `Mb/s` gives figure `25` and unit `Mb/s`;
  - `3` U+202F `kb/s` gives figure `3` and unit `kb/s`;
  - `25 Mb/s` with a plain space is shown whole with no unit;
  - `No limit` is shown whole.

  Then cut at `units.Space` and drop the spelling loop (design decision 7). Verify with `make test`.

## 3. Wi-Fi state is the wireless plugin's read

- [x] 3.1 In `plugins/verso-plugin-wireless` (its own repo), test first in a new `root.rs`: every case of `internal/openwrt/wireless_state_test.go` and `internal/server/wireless_state_test.go`, ported against injected ubus answers. They cover a radio up and down, a network with no interface, absent numbers when iwinfo cannot answer, and busy share from the in-use frequency only. Then:
  - implement the root half on `root::ubus`, and add `rootfs/usr/share/verso/plugins/wireless/commands.json` with the read `state`, version 1;
  - replace `ubus verso wirelessState` with `verso-plugin wireless state` in the manifest's `acl.read`;
  - in `rootfs/usr/share/rpcd/acl.d/verso-plugin-wireless.json`, grant `"verso-plugin": {"wireless": ["state"]}` in place of `verso wirelessState`, `network.wireless` and `iwinfo`;
  - read the state in `live.rs` with `ubus.read("wireless.state", &[1])`, and update `fixture.rs` to the registered read's shape;
  - make `deploy-dev.sh` install `commands.json` as root:root 0644.

  Verify with `cargo test` and `cargo clippy` there, and commit there.
- [x] 3.2 In the lab, with `make dev` running, run the wireless repo's `deploy-dev.sh`. Ask the helper for `pluginCommand {"plugin": "wireless", "name": "state"}` over `socat`, with a session granted the plugin's access group, and open the Wi-Fi page. Verify that the helper answers one JSON object with `radios` and `networks`, and that the page shows the seeded radio's state.
- [x] 3.3 Test first in `internal/server/gateway_test.go`: `coreRead("wirelessState")` is nil, and a manifest declaring `ubus verso wirelessState` gets no entry. Then remove:
  - the `coreRead` case;
  - `NativeBackend.WirelessState` and `wireless_state.go` with their tests;
  - the `Backend` method and every fake's.

  Remove the `wirelessState` row from `docs/plugins.md`, and amend ADR-007 §4 in place to name only the brokered reads the shell draws. Verify with `make lint` (deadcode included) and `make test`.

## 4. Doors are plugins' claims

- [x] 4.1 Test first in `internal/server/entity_test.go`:
  - `network`, `tunnel`, `zone`, `rule` and `port-forward` are door kinds, each with the slot `open`, and `/entity/zone/x` answers 404;
  - `door(kind)` is the live claimant whose id sorts first; a stopped claimant gives no door, and a claim under no nav entry gives none;
  - `To("a b")` escapes the id under the plugin's mount;
  - `Listing()` is the most specific page holding the claim's path, so `zone` → `/plugins/firewall/zones` and `network` → `/plugins/interfaces/`;
  - `Word` is the holding entry's label through the plugin's translator, the Slovenian one in Slovenian;
  - at load, an `entity_acts` claim on an unknown kind or slot, or an `open` claim with no `{id}`, is dropped with a log line naming the plugin, while dnsdhcp's `add` claim with `/?open=new` stands;
  - `entityActs` takes the first claimant by id.

  Then implement them (design decision 1). Verify with `make test`.
- [x] 4.2 In `plugins/verso-plugin-vpn` (its own repo), add to `manifest.json`:
  - the claim `{"entity": "tunnel", "slot": "open", "path": "/?open={id}"}`;
  - `order` 40 on its nav entry;
  - the glyph `lock`;
  - `services` `{"openvpn": "OpenVPN"}`, with `OpenVPN` in its catalogs.

  Verify with `cargo test` there, and that `make dev` loads it with no log line about a dropped claim. Commit there.
- [x] 4.3 Test first in `internal/widget/overview_test.go` and `internal/server/server_test.go`, for every scenario of the `overview` delta:
  - with interfaces and vpn answering, Internet, Ports and Tunnels have their doors and words;
  - with vpn stopped, Tunnels says "Open Interfaces";
  - with neither answering, Internet, Ports and Tunnels draw no Go link, and Hardware still links `/system/hardware`;
  - with the VPN entry labelled Tunnels, the card says "Open Tunnels".

  Then give `overviewCardSpec` its `door`, fill `Overview.Doors` from `s.door`, and claim `network` with `/edit?network={id}` in `plugins/verso-plugin-interfaces/manifest.json`. Remove `InterfacesHref`, `TunnelsHref` and `navLabelHref` (design decision 3). Verify with `make test`.
- [x] 4.4 Test first in `internal/server/exposure_test.go` and `internal/server/fwlog_test.go`:
  - the exposure notice links the listing door of its first cause's kind as "Open Firewall": Zones for a zone, Rules for a rule, Port forwards for a forward. It has no link with no claim;
  - a logged rule addresses `/plugins/firewall/?open=<section>`, and a redirect `/plugins/firewall/port-forwards?open=<section>`, through the doors;
  - with no claim, both are plain text, and a `nat` row stays plain;
  - with a fixture plugin `fw2` claiming `zone`, `rule` and `port-forward` in place of the firewall plugin, the notice and the log lead into `fw2`, matching plugin-boundary's "A plugin is replaced".

  Then claim the three kinds in `plugins/verso-plugin-firewall/manifest.json`, give `fwLogReader` its `door`, and remove `fwLogRuleEditor`, `fwLogRedirectEditor` and the exposure path. Verify with `make test`.
- [x] 4.5 Document door kinds and the `open` claim in `docs/plugins.md` ("Entity panels"): the door's address, page, word and the first-by-id rule. Reword the `entity_acts` row (`docs/plugins.md:92`) and the `EntityAct` doc comment, which say it opens nothing. Amend ADR-006 (`entity_acts`) and ADR-022 (a card's door is a claim) in place. Verify by checking that `grep -n 'navLabel\|/plugins/firewall/' docs/ADR/0{06,22}*.md` finds nothing that says core links there.
- [x] 4.6 In the lab, check each case and save screenshots to `~/Share/screenshots/`:
  - each Overview card opens the right page;
  - the exposure notice on Access opens Firewall's Zones page for a zone cause;
  - a Firewall log row's rule name opens that rule's drawer;
  - with `verso-plugin-firewall` stopped, a fresh Access has no link on the notice;
  - with Activity open, stopping the plugin turns new rows' rule names to plain text after 30 s;
  - with `verso-plugin-vpn` stopped, Tunnels says "Open Interfaces".

## 5. Places in the rail

- [ ] 5.1 Test first in `internal/plugin/manifest_test.go`:
  - a nav entry's `order` decodes as present or absent;
  - `"40"` and `4.5` are logged and read as absent, while the manifest loads;
  - a manifest with no `order` loads as before.

  Then read `order` per entry beside the main decode, as `readContributes` does, into `NavEntry.Order *int`. Verify with `make test`.
- [ ] 5.2 Test first in `internal/server/nav_test.go`, every scenario of the `rail` delta:
  - the bundled plugins' designed rail and both sections' pages;
  - a row at 45 between Wireless and Firewall, and a Network page at 35;
  - ties by English label in two languages;
  - `-5` standing after Overview, and 5 before Devices;
  - a System page at 65;
  - an older VPN with no `order` after System, and an older dnsdhcp after Routes;
  - a glyph from the entry, else from the section;
  - an entry filed under Status still has no row.

  Then implement `placeLess` and the shell's place constants. Remove `railOrder`, `navIcon`'s label cases and `collapsedSections.Order`. Give the in-tree manifests their numbers: Interfaces 10, Routes 40, Firewall 50, General 10 (design decision 4). Rewrite the existing nav tests that depend on label order to declare `order`. Verify with `make test`.
- [ ] 5.3 Declare `order` in each plugin repository's `manifest.json`, then verify with `cargo test` and commit in each repo:
  - `plugins/verso-plugin-dnsdhcp`: DHCP 20, DNS 30;
  - `plugins/verso-plugin-ddns`: Dynamic DNS 50;
  - `plugins/verso-plugin-wireless`: Wireless 30.
- [ ] 5.4 Document nav `order` in `docs/plugins.md` ("The manifest") beside `contributes[].order`, saying that a missing nav `order` falls after the ordered entries while a missing contribution `order` reads as 100. Include:
  - the published numbers of the shell's rows and System pages, and the bundled plugins' numbers;
  - values below 1, ties, and malformed values.

  Amend ADR-009 §2, §3 and §5 and ADR-006 in place. Verify by checking that `grep -n 'designed order' docs/ADR/009*.md` names numbers, not labels.
- [ ] 5.5 In the lab, with every plugin deployed, verify that the rail reads Overview, Devices, Network, Wireless, VPN, Firewall, System, and that Network's and System's pages read as the `rail` delta's first-party scenario says. Save a screenshot to `~/Share/screenshots/`.

## 6. Plugins name their services

- [ ] 6.1 In `plugins/verso-plugin-ddns` (its own repo), add `services` `{"ddns": "Dynamic DNS"}` to `manifest.json`. Verify with `cargo test` there that its catalogs carry the words, and commit there.
- [ ] 6.2 Test first:
  - in `internal/plugin/manifest_test.go`, every scenario of the `staged-changes` delta's malformed entries: an entry with a bad script name or with words that are not non-empty text is dropped with a log line; a `services` that is not an object is dropped whole; the manifest loads either way;
  - in `internal/server/apply_steps_test.go`, every scenario of its reload-step words: ddns and openvpn in their plugins' words; raw `openvpn` with no words; `firewall` keeps the shell's word; a stopped plugin's words still said; two scripts with one word said once.

  Then read `services` per entry into `Manifest.Services`, add the namer, and drop `ddns` from `serviceWords` (design decision 5). Verify with `make test`.
- [ ] 6.3 Document `services` in `docs/plugins.md` ("The manifest"), and amend ADR-006 in place. Verify that the documented example decodes in the manifest test.
- [ ] 6.4 In the lab, stage a change to the `ddns` config and one to the `openvpn` config. Verify that Pending changes reads "Reloads Dynamic DNS and OpenVPN.", or each alone where only one has a trigger in the container.

## 7. The guard

- [ ] 7.1 Rename every core parameter that takes a plugin id to `pluginID`: `pluginHref`, `manifestByID`, the translator `pluginTranslators` returns, and the rest the search finds. Verify with `make lint` and `make test`.
- [ ] 7.2 Test first in `internal/boundary/boundary_test.go`, against fixtures in `internal/boundary/testdata`.
  - **Reported** at its file and line, one fixture each:
    - a `/plugins/<id>` constant;
    - a `Manifest.ID` compared with `"vpn"`;
    - a switch on a `PluginID` field;
    - a `Plugin` field compared with `"acme"`;
    - `pluginHref("firewall", "/zones")`;
    - a nav label compared with `"VPN"`;
    - a nav label as a fixed map's key;
    - a `#[path]`, an `include!` and an `include_str!` into a plugin.
  - **Passes**, in a clean fixture:
    - the `firewall` config;
    - `m.ID == plugin.ShellID`;
    - `== ""`;
    - a sort by label;
    - two labels compared with each other;
    - `"/plugins/"` alone.

  Then implement the scan over `./internal/...` and `./cmd/...` with `go list -export -deps` and `go/types`, and the `verso-rpcd/src` check (design decision 9). Verify with `make test`: the real tree passes.
- [ ] 7.3 Run `graphify update .`, then `graphify query "core Go naming a plugin id, path or nav label"`. Verify that nothing it surfaces outside tests names a plugin.

## 8. The backlog and the last checks

- [ ] 8.1 In `docs/BACKLOG.md`, staging only this change's hunks, since the file holds another session's edits:
  - remove the item "Plugin contracts: the shell names no plugin";
  - add "The Interfaces listing in the shell": the shell lists interfaces from netifd; the interfaces plugin contributes its editor and config writes through `network` slots; up, down and restart are the listing's own acts; the tree table draws it.

  Verify with `git diff --cached docs/BACKLOG.md`, which shows only those hunks.
- [ ] 8.2 Run `make lint` and `make test`. Verify that every check passes except the five known `scripts/dev-grid.test.cjs` tests, and record the result in the change's notes.

## Workflow follow-up

- The owner checks groups 3, 4, 5 and 6's lab items on a router built from a clean HEAD, then archives the change.
- Release notes name the plugin versions that carry `order`, the `tunnel` claim and the service words.
