# Release notes (draft)

Drafts for the GitHub releases of the tags that carry this change: Verso's
section first, then each plugin repository's for its own tag.

## Verso

> **Verso is highly experimental.** Back up your router's configuration before you upgrade.

This release moves each plugin's root work into the plugin itself. Verso's root helper now runs only what a plugin's package registers, so Verso no longer carries code for ACME, OpenVPN, Dynamic DNS or DNS and DHCP.

### Changes to your router

- **Upgrade Verso and its plugins together.** Verso, verso-plugin-acme, verso-plugin-vpn, verso-plugin-ddns and verso-plugin-dnsdhcp move as one: this Verso refuses to install beside an older one of these plugins, and each new plugin needs this Verso. `apk upgrade` moves them all at once.
- **A certificate you install by hand stays.** Installing or making the web certificate on Access tells any certificate source on the router. The ACME plugin then switches its own renewal off, so no renewal replaces your certificate, and this holds across a reboot.
- **The OpenVPN hook moved.** `/etc/hotplug.d/openvpn/50-verso` now ships with verso-plugin-vpn, not Verso.

### Fixed

- The firewall's custom rule files listed dnsmasq's configuration file beside them, with an editor link that led nowhere.
- A plugin's action ran as soon as its page was saved, though the plugin guide said it ran after the changes were applied. The guide now says what happens: an action runs at once, and never in the same save as changes that wait to be applied.

### For plugin authors

- **Root work is registered.** A plugin's package installs `/usr/share/verso/plugins/<id>/commands.json`, naming its reads (versioned), its acts (with typed arguments) and its file families. The root helper runs the plugin's own binary as root for them, granted by the session as `verso-plugin <plugin> <command>`. See "Root work: the command registry" in docs/plugins.md, and ADR-027.
- **Reads arrive by name and version.** A registered read reaches a page as `<plugin>.<name>`; `Ubus::read(key, &[versions])` returns it only at a version the plugin knows.
- **Hand-edited files are declared.** A plugin declares its file families (folder, suffixes, check, reload, transform) in its registry instead of in Verso. `configFiles` hands a plugin the families of the configs it reads, and `FileSet` names the family it lists.
- **The helper verbs that did a plugin's work are gone:** `vpnState`, `openvpnFiles`, `ddnsState`, `ddnsUpdate`, `dhcpState`, `dnsState`, `firewallCounters`, `firewallFiles` and `acmeState`.
- The SDK gains `verso_plugin::root` (the dispatcher, `own_write`, `signal_shell`, `write_new`, `uci::committed`), `verso_plugin::netlink`, `Registry` and `ArgKind`.

### Upgrade

```sh
apk update && apk upgrade
```

Upgrade verso, verso-plugin-acme, verso-plugin-vpn, verso-plugin-ddns and verso-plugin-dnsdhcp together.

## verso-plugin-acme

The plugin now carries its own root half: delivering the trusted certificate, recording what acme.sh did, and switching renewal off when a certificate is installed by hand. It needs the Verso released beside it; upgrade them together.

## verso-plugin-vpn

The plugin now carries its own root half and OpenVPN's hotplug hook: what each tunnel is doing, and the profiles read out with their key blocks folded. A file the firewall owns no longer counts as a profile name. It needs the Verso released beside it; upgrade them together.

## verso-plugin-ddns

The plugin now carries its own root half: what each updater last did, and Update now. It needs the Verso released beside it; upgrade them together.

## verso-plugin-dnsdhcp

The plugin now carries its own root half: each network's DHCP server as dnsmasq loaded it, what the resolver can do, and checking custom options with dnsmasq before they are staged. It needs the Verso released beside it; upgrade them together.
