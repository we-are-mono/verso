# Proposal

## Why

Operators see nothing new. The difference is what happens when they install or remove a
plugin. Today the root work for five plugins lives in core:
- **VPN:** tunnel state, profile folding, and the OpenVPN hotplug recorder.
- **DDNS:** state, and the update trigger.
- **DNS and DHCP:** state.
- **Firewall:** counters and rule files.
- **The trusted certificate:** delivery, state, and turning acme off.

So a router without these plugins still carries their code as root. A plugin can't fix
its own root half without a core release, and a third-party plugin can't have a root
half at all.

Separately, the shell brokers each plugin's helper read by name and runs plugin acts from
a closed list. So adding a plugin means editing core. The owner's goal for plugins is
that Verso knows nothing about them. Slots (ADR-026) made that true on screen; this
change makes it true for root work.

## What Changes

- **A command registry in `verso-rpcd`.** A plugin package installs
  `/usr/share/verso/plugins/<id>/commands.json`. It names the plugin's root commands,
  each a **read** (no arguments, run for a page render) or an **act** (structured
  arguments checked against a declared schema).
  - `verso-rpcd` runs a command only for a session that holds the grant
    `verso-plugin <id> <name>`.
  - It runs only a registry file and an executable that root owns and only root can
    write.
  - It runs the executable as root with JSON on stdin: no shell, a clean environment,
    capped output, and a timeout.
- **A plugin's root half is its own binary run by root.** `verso-plugin-<id> root …` is
  started by the helper, by the plugin's hotplug scripts and by its init. The SDK gives
  the dispatcher. No plugin ships a second binary.
- **BREAKING: the plugin-named helper verbs leave core.** These move to their plugins,
  together with `acme.rs`, `openvpn.rs`, `ddns.rs`, `dhcp.rs` and the openvpn hotplug:
  - vpn: `vpnState`, `openvpnFiles`;
  - ddns: `ddnsState`, `ddnsUpdate`;
  - dnsdhcp: `dhcpState`, `dnsState`;
  - firewall: `firewallCounters`, `firewallFiles`;
  - acme: `acmeState` and `verso-rpcd acme deliver`.

  The shell's by-name broker becomes generic. Its closed command list keeps only the
  acts core owns.
- **Config-file families are declared by plugins.** Staging, versions, apply, rollback
  and config history stay core machinery. Each family is declared by its plugin in the
  registry:
  - its places;
  - the uci config that owns it;
  - its size limit and file mode;
  - its check and reload programs;
  - optional fold and unfold commands.

  dnsmasq's family moves to dnsdhcp, fw4's to firewall, and OpenVPN's to vpn.
- **A hand-installed certificate is announced, not handled by core.** Installing or
  making a certificate by hand raises `hotplug-call verso-certificate` with
  `ACTION=installed`. acme's hotplug script turns `acme.verso` off. Core no longer
  names acme.
- **One plugin can read another's state.** interfaces holds grants on vpn's tunnel
  state and dnsdhcp's DHCP state. When the other plugin is absent, interfaces gets no
  data and draws without it.
- **Every read declares a version.** It rises only on an incompatible change. A plugin
  uses a result only at a version it knows, so a producer and a reader from different
  repositories can upgrade apart without a page misreading state.
- **Plugin acts are documented as what they are.** `docs/plugins.md` says the `apply`
  array runs after the UCI apply. The code reads `commands` and runs an act at once,
  never in a save together with staged changes. The docs and ADR-006 now say what the
  code does. A file stage is the one command that joins the stage.
- **What core keeps:**
  - what the shell itself renders: `firewallStatus`, `firewallLog`, `dhcpLeases`,
    `networkState`;
  - what guards access to the router: `accessCredentials`, `setAuthorizedKeys`,
    certificate install and make, the clock, the password;
  - packages, firmware, backups, diagnostics, usage and hardware.

Not in this change:
- `wirelessState`, which the shell computes in Go for the wireless plugin.
- The `interface-up/down/restart` acts, which call rpcd's `network.interface`.
- Core events beyond `verso-certificate`, and the shell's hard-coded firewall links,
  Overview nav-label lookups and `device`/`shape` slot. These belong to the events
  theme and the slots theme.

## Capabilities

### New Capabilities
- `plugin-commands`: the root side of the plugin contract:
  - the registry file and its ownership checks;
  - reads and acts, argument schemas, grants and cross-plugin grants;
  - how the helper runs a command, and how the shell brokers reads and forwards acts;
  - what core keeps;
  - how a removed plugin leaves no trace.
- `config-file-families`: hand-edited daemon files as families declared by plugins:
  - places and their limits;
  - check, reload, fold and unfold;
  - staging beside uci, apply and rollback, and their record in config history.

### Modified Capabilities
- `trusted-certificate`:
  - the certificate's root work ships with verso-plugin-acme, and core carries no ACME
    code;
  - a hand install is announced by an event that acme answers, instead of core turning
    acme off.

## Impact

- **ADRs:**
  - New ADR-027 (plugin root halves and the command registry), giving the reasons.
  - Amends ADR-007 §5 in place: the helper gains one generic, registry-bound execution
    path. It is root-authored, schema-checked and grant-gated, never a command string.
    The closed broker set in its implementation notes becomes the registry.
  - Amends ADR-006: `commands.json`, reads and acts, and the documented `apply` array
    replaced by what runs.
  - Amends ADR-017 §6: delivery and catch-up run from acme's own root half, and
    `verso-certificate` replaces `hand_over`.
  - Amends ADR-014 §3 to state that the registry is never reachable with the zero
    session.
  - Amends ADR-021: a family's files are recorded as its fold gives them.
- **Helper (`verso-rpcd`):**
  - a new registry module and the `pluginCommand` verb;
  - family definitions read from the registries in place of the `Family` enum;
  - removal of `acme.rs`, `openvpn.rs`, `ddns.rs`, `dhcp.rs`, the dnsmasq feature
    probe, the per-family reads, and the `acme` command line.
- **Shell (Go):**
  - `brokeredUbusRead` (`internal/server/gateway.go`) becomes a generic registry read;
  - `runPluginCommands` (`internal/server/plugin_commands.go`) forwards registered acts
    and keeps core's own acts;
  - `fileConfig` comes from family declarations;
  - `ACMEState`, `VPNState`, `DDNSState`, `DDNSUpdate`, `DHCPState`, `DNSState`,
    `FirewallFiles`, `FirewallCounters` and `OpenVPNFiles` leave `internal/openwrt`;
  - `readACMEState` keeps the PEM-to-facts step as a generic certificate-facts
    transform the read's registry entry asks for.
- **SDK (`plugins/verso-plugin-sdk`):** the `commands.json` types and writer, the `root`
  dispatcher, argument kinds, conformance fixtures (`testdata/commands.json`), and the
  brokered-read accessor keyed `<plugin>.<name>`.
- **Plugins:**
  - in this tree (bundled in the `verso` package): firewall gains its root half and its
    own rpcd ACL; interfaces reads vpn's and dnsdhcp's state through registry grants;
    system raises no acme work.
  - own repositories: acme, vpn, ddns and dnsdhcp each gain a root half,
    `commands.json` and grants. vpn takes back `/etc/hotplug.d/openvpn/50-verso`.
- **Packaging and compatibility:**
  - `verso` declares a conflict with the acme, vpn, ddns and dnsdhcp versions built
    before the move. Each of those plugins depends on the `verso` that has the
    registry, and vpn replaces `verso` for the openvpn hotplug file.
  - Runtime paths stay as they are (`/var/run/verso-acme`, `/var/run/verso-openvpn`,
    the helper's version key in `/etc/verso`), so an upgrade carries live tunnel
    records and file versions over without migration.
  - The release notes say to upgrade verso and these four plugins together.
- **Docs:**
  - `docs/plugins.md`: the brokered-read table becomes the registry, the `apply`
    section is rewritten, and file families are covered;
  - `docs/BACKLOG.md`.
