# Tasks

## 1. Gate: apk's versioned conflict

- [x] 1.1 On the DK, build two throwaway local apks: `verso-test-core` with `depends: !verso-test-plugin<2`, and `verso-test-plugin` 1 and 2. Check that apk refuses core with plugin 1 installed, and that `apk upgrade` moves both together. Verify by pasting the apk output into the change's notes. If apk does not honour it, stop and take the result to the owner before any other task (design: Risks).

## 2. SDK: the registry, the root dispatcher and the shared fixtures

- [ ] 2.1 Test first in `plugins/verso-plugin-sdk/tests/conformance.rs`: `Registry { commands, families, certificates }` serializes to `testdata/commands.json`. A read with `args` fails to build, and so do a read without a positive `version` and a command with a relative `exec`. Then add the types and writer. Verify with `cargo test` in the SDK.
- [ ] 2.2 Test first: `testdata/argument-kinds.json` lists accepted and refused samples for every kind (`uci-name`, `hostname`, `ipv4`, `ipv6`, `ip`, `port`, `int`, `enum`, `bool`, `text`), and the SDK's validators agree with every sample. Then implement the validators. Verify with `cargo test`.
- [ ] 2.3 Test first: `root::run` refuses a non-zero euid with exit 1 and no handler called. `Ok` exits 0 with the JSON, `Refusal` exits 3 with `{"refusal"}`, and a panic or error exits 1. An unknown command name exits 1. Then implement it with an injectable uid and stdio. Verify with `cargo test`.
- [ ] 2.4 Test first: the brokered-read accessor finds `vpn.state` under its `<plugin>.<name>` key and returns its result when the caller knows its version. It returns nothing, logging once, for an unknown version. It still finds core functions such as `dhcpLeases` by name. Then implement it. Verify with `cargo test`.
- [ ] 2.5 Document `commands.json`, the root dispatcher, exit codes and argument kinds in `docs/plugins.md` ("Root work: the command registry"). Verify that the doc's JSON example decodes with the SDK's `Registry` type in a doc test.

## 3. Helper: registry, runner and the generic verb

- [ ] 3.1 Test first in `verso-rpcd`, with a fake `Owner` table: the registry loader refuses a file or executable that is not uid 0, that has a group or other write bit (on it or on any ancestor), that is a link, or that is not a regular file. Each refusal logs the failing path, and other plugins load. Fold `config_files::check_path` into the same walk. Verify with `cargo test` in `verso-rpcd`.
- [ ] 3.2 Test first: the argument checker passes and refuses exactly as `testdata/argument-kinds.json` says, refuses unknown and missing arguments, and refuses any argument to a read. Then implement it. Verify with `cargo test`.
- [ ] 3.3 Test first, with a fake `Runner`:
  - the child gets `exec` plus `argv`, `PATH` alone, cwd `/`, and `{"command","args"}` on stdin;
  - a hang is killed with its group at the declared timeout (default 10 s, capped at 120 s);
  - stdout over 1 MiB fails;
  - a non-object output fails;
  - exit 3 with `refusal` is a validation failure carrying the text;
  - any other end is an unknown failure with stderr logged.

  Then generalize the existing child runner. Verify with `cargo test`.
- [ ] 3.4 Test first in `dispatch`:
  - `pluginCommand` asks `session.access` for `{verso-plugin, <plugin>, <name>}`, not `{ubus, verso, pluginCommand}`;
  - the zero session is denied before any lookup;
  - a `transform` command is refused through the verb;
  - an unknown plugin or command answers not found.

  Then implement it. Verify with `cargo test`, then on the lab container with `make dev`: one fixture plugin's read answers through `ubus call`.
- [ ] 3.5 Test first in `grants.rs`: a group with `"verso-plugin": {"vpn": ["state"]}` under `read` grants `(verso-plugin, vpn, state)` on refresh. Verify with `cargo test`.
- [ ] 3.6 Write ADR-027 (plugin root halves and the command registry) from design decisions 1–8. Amend ADR-007 §5 and its implementation notes in place: one generic registry-bound path, and the closed broker set becomes the registry. Amend ADR-014 §3: the registry is unreachable with the zero session. Verify that every cross-reference resolves (`grep -n 'ADR-027' docs/ADR/*.md`).

## 4. Shell: generic reads and acts

- [ ] 4.1 Test first in `internal/server` with a fake backend:
  - a manifest `acl.read` entry with scope `verso-plugin` is read through `PluginCommand` and lands in `X-Verso-Ubus` under `<plugin>.<name>` as `{"version", "result"}`, with the version taken from the registry;
  - a refused, failed or unknown read gives no entry, and the page still renders;
  - three 200 ms reads finish in under 400 ms.

  Then implement it beside the core functions in `brokeredUbusRead`, and add `PluginCommand` to the `Backend` seam and `internal/openwrt`. Verify with `make test`.
- [ ] 4.2 Test first: a read whose registry entry names `certificates` has those fields replaced by `certificateFacts`, and an unparsable one is dropped. No PEM reaches the plugin. Then generalize `readACMEState` into this transform, reading the registry from `/usr/share/verso/plugins/<id>/commands.json`. Verify with `make test`.
- [ ] 4.3 Test first in `plugin_commands_test.go`:
  - an act declared as `verso-plugin` in `acl.write` runs through `PluginCommand` with its arguments;
  - an undeclared act is refused without a backend call;
  - a refusal's text reaches the notice through the plugin's catalogue;
  - an act beside a uci commit is refused as not permitted, with nothing staged.

  Then implement it, keeping core's own acts (`config-file-stage`, SSH keys, certificate generate and install, `set-system-time`, interface up/down/restart). Verify with `make test`.
- [ ] 4.4 Measure on the DK: render the VPN and Interfaces pages 20 times each, and compare the time added per read against the current build. Record the numbers in the change's notes. Verify that the notes hold both builds' numbers.
- [ ] 4.5 Rewrite `docs/plugins.md`'s "Privileged post-apply actions" as "Acts": one per POST, run at once, never beside staged changes, the file stage the one exception. Replace the brokered-read table's plugin rows with the registry. Amend ADR-006 in place to match. Verify by checking that `grep -n '"apply"' docs/plugins.md docs/ADR/006-plugin-contract.md` finds nothing.

## 5. File families from the registries

- [ ] 5.1 Test first in `config_files.rs`, with registries in a temp dir:
  - the family table builds from every registry;
  - a family under `/etc/config`, `/etc/shadow` or another protected path is dropped and logged;
  - two families sharing a name or a place are both dropped;
  - a path outside every family is refused.

  Then replace the `Family` enum with the table, keeping the journal format. Verify with `cargo test`.
- [ ] 5.2 Test first, with a fake transform:
  - `list` folds content and withholds by path;
  - `stage` restores held material or refuses;
  - a failing transform lists that family by path alone;
  - check and reload run their fixed argv.

  Then implement the transform protocol (design decision 9). Verify with `cargo test`.
- [ ] 5.3 Test first: a journal written by the current build (families `dnsmasq`, `fw4`, `openvpn`) still lists, applies and discards under the registry-built table. Verify with `cargo test` against a recorded journal fixture.
- [ ] 5.4 Test first in `internal/server`:
  - `configFiles` carries `families` (`name`, `config`, `plugin`);
  - a plugin's file read is narrowed to families of configs it reads;
  - `config-file-stage` checks the owning config from that list.

  Then remove `fileConfig`. Verify with `make test`.
- [ ] 5.5 Test first: a config history entry for an applied OpenVPN profile edit holds the folded text, and the history database holds no line of the key block. Then fix the recording path if it does not already use the listing. Amend ADR-021's files row in place. Verify with `make test` and `cargo test`.
- [ ] 5.6 Document families, their bounds and the transform protocol in `docs/plugins.md`. Verify that the doc's family example decodes with the SDK's `Registry`.

## 6. In-tree plugins: firewall, interfaces, system

- [ ] 6.1 Test first in `plugins/verso-plugin-firewall`: `root` answers `state` with the counters (moved from `firewall.rs` `counters`) and whether `/sbin/fw4` exists. Its registry declares `state` and the `fw4` family with `fw4 check` and `fw4 -q restart`. Then implement it and give firewall its own `rootfs/usr/share/rpcd/acl.d/verso-plugin-firewall.json`. Verify with `cargo test`.
- [ ] 6.2 Switch the firewall pages to `firewall.state` and `configFiles`. Remove `firewallCounters` and `firewallFiles` from the helper, `verso-helper.json`, the shell's broker and `internal/openwrt`. Keep `firewallStatus` and `firewallLog`. Package `commands.json` in `package_plugin`. Verify with `make test`, `make lint`, and the firewall pages on `make dev`.
- [ ] 6.3 Test first in `plugins/verso-plugin-interfaces`: tunnel rows come from `vpn.state` version 1, and DHCP state from `dnsdhcp.dhcp-state` version 1. With either absent, or at a version interfaces does not know, the page renders without it. Then change its manifest and rpcd ACL to the `verso-plugin` grants. Verify with `cargo test`.
- [ ] 6.4 Test first in `verso-rpcd`: after `setWebCertificate` or `makeWebCertificate` writes the pair, the fake runner sees `hotplug-call verso-certificate` with `ACTION=installed`, and no `uci` call. Then replace `acme::hand_over`. Verify with `cargo test`.

## 7. verso-plugin-acme (its own repository)

- [ ] 7.1 Test first in the acme repo: move `acme.rs`'s tests with it (deliver, record, `forget_stale`, `lan_answer`, state, hand-over) behind the root dispatcher, with the same `Router` seam. Then move the code as `root deliver <event>`, `root hand-over`, and the `state` read. Verify with `cargo test`.
- [ ] 7.2 Ship `commands.json` (`state`, with `certificates: {"state": ["certificate"]}`), `/etc/hotplug.d/acme/50-verso` and `/etc/hotplug.d/verso-certificate/50-acme` calling the plugin binary, and the init's `catch_up` calling `root deliver sync`. Change the manifest and ACL to `verso-plugin acme state`, and depend on `verso>=R`. Verify with `cargo test` and the shell's manifest test against acme's manifest.
- [ ] 7.3 Remove `acme.rs`, the `acmeState` verb, the `acme` command line and `ACMEState` from core. Amend ADR-017 §6 in place (delivery, catch-up, state and hand-over from the plugin's root half). Verify with `make test`, `make lint`, and `verso-rpcd acme deliver sync` exiting non-zero on the lab.

## 8. verso-plugin-vpn (its own repository)

- [ ] 8.1 Test first in the vpn repo: move `openvpn.rs`'s tests (state, records, PID and owner checks, fold, unfold, profile withholding, the hotplug script's tests) with it. Then move the code as the `state` read and the `files` transform. Verify with `cargo test`.
- [ ] 8.2 Ship `commands.json` (`state`, `files`, and the `openvpn` family: `/etc/openvpn`, mode `0600`, reload `/etc/init.d/openvpn reload`) and `/etc/hotplug.d/openvpn/50-verso` with `replaces:verso`. Change the manifest and ACL to `verso-plugin vpn state` and `configFiles`, and depend on `verso>=R`. Verify with `cargo test`.
- [ ] 8.3 Remove `openvpn.rs`, `vpnState`, `openvpnFiles`, the hotplug file and `replaces:verso-plugin-vpn` from core. Verify with `make test` and `make lint`.

## 9. verso-plugin-ddns and verso-plugin-dnsdhcp (their own repositories)

- [ ] 9.1 Test first in the ddns repo: move `ddns.rs`'s tests. `update` refuses a section that is not a uci name before running anything (helper-side), and refuses a missing section with a refusal text (root half). Then ship `commands.json` (`state` and `update`, with `section: uci-name` and a 30 s timeout), the manifest, the ACL, and `depends: verso>=R`. Verify with `cargo test`.
- [ ] 9.2 Test first in the dnsdhcp repo: move `dhcp.rs`, `dns_state`'s feature probe and `validate_dnsmasq` as the `dhcp-state` and `dns-state` reads and the `files` transform's `stage`. Ship the `dnsmasq` family (`/etc/dnsmasq.conf`, `/etc/dnsmasq.d`, check `dnsmasq --test`, reload `/etc/init.d/dnsmasq reload`). Then switch its pages to `dnsdhcp.*` and `configFiles`. Verify with `cargo test`.
- [ ] 9.3 Remove `ddns.rs`, `dhcp.rs`, `ddnsState`, `ddnsUpdate`, `dhcpState`, `dnsState`, the `ddns-update` command and the matching backend methods from core. Verify with `make test` and `make lint`.

## 10. Core packaging and the empty verb list

- [ ] 10.1 Test first in `verso-rpcd`: the dispatch table names none of `vpnState`, `openvpnFiles`, `ddnsState`, `ddnsUpdate`, `dhcpState`, `dnsState`, `firewallCounters`, `firewallFiles` or `acmeState`, and `grep -rn 'acme\|openvpn\|ddns' verso-rpcd/src` finds nothing. Verify with `cargo test`.
- [ ] 10.2 Add the versioned conflicts on the pre-registry acme, vpn, ddns and dnsdhcp versions to `verso`'s `depends` in the `Makefile` (the syntax found in 1.1), and drop `replaces:verso-plugin-vpn`. Verify by building with `make apk APK_GOARCH=arm64` and reading the apk's `.PKGINFO`.

## 11. Integration on the lab and the DK

- [ ] 11.1 On `make dev`, check with Playwright: VPN, DDNS, DNS & DHCP, Firewall and Interfaces render their state; a DDNS Update runs; a rule file and a profile save, apply and roll back; removing ddns leaves its pages and reads gone. Save screenshots to ~/Share/screenshots.
- [ ] 11.2 On the DK, with local apks (`deploy-verso` "Local release"), keep an OpenVPN tunnel up and a rule file staged, then upgrade verso and the four plugins together. Check that:
  - the VPN page shows the tunnel connected with its connected time;
  - the staged file still applies;
  - apk refuses an upgrade of verso alone.
- [ ] 11.3 On the DK (router.example.com in every public artifact): install a certificate by hand while `acme.verso` is on, reboot, and check that the hand-installed certificate is served and `acme.verso.enabled` is 0. Then check that a forced acme renew is served with no one signed in.
- [ ] 11.4 Update `docs/BACKLOG.md`: the certificate-source root half and the registry are done, and the events theme is next. Note `wirelessState` and the interface acts as the remaining plugin-specific code in the shell. Write release notes asking to upgrade verso, verso-plugin-acme, verso-plugin-vpn, verso-plugin-ddns and verso-plugin-dnsdhcp together. Verify that `make lint` and `make test` are green.

## Workflow follow-up

- Archive the change once the DK checks pass, after confirming ADR-027, the ADR-006, ADR-007, ADR-014, ADR-017 and ADR-021 amendments, and docs/BACKLOG.md reflect what landed.
