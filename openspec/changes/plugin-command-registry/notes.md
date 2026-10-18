# Notes

## 1.1 apk's versioned conflict

Tested 2026-10-18 in the lab container: OpenWrt 25.12.4 (r32933-4ccb782af7) with
apk-tools 3.0.5. That is the apk-tools version of the OpenWrt tree the DK's image is
built from (`~/Mono/Gateway/openwrt/source`, host apk 3.0.5). The test needs no
router: the resolver is all it exercises.

The throwaway packages were built with the buildroot's `apk mkpkg`, because the target
apk has no `mkpkg`:
- `vtest-core` 1-r0, with no dependencies;
- `vtest-core` 2-r0, with `depends: !vtest-plugin<2-r0`;
- `vtest-plugin` 1-r0, with `depends: vtest-core`;
- `vtest-plugin` 2-r0, with `depends: vtest-core>=2-r0`.

Each scenario started from core 1-r0 and plugin 1-r0 installed, with an unpinned world
(`vtest-core vtest-plugin`). Two feeds were used: one with all four packages, and one
without plugin 2-r0.

```
== B: upgrade verso alone, feed has no new plugin
ERROR: unable to select packages:
  vtest-plugin-1-r0:
    breaks: vtest-core-2-r0[!vtest-plugin<2-r0]
    satisfies: world[vtest-plugin]
vtest-core-1-r0 vtest-plugin-1-r0
== A: upgrade verso alone, feed has the new plugin
(1/2) Upgrading vtest-plugin (1-r0 -> 2-r0)
(2/2) Upgrading vtest-core (1-r0 -> 2-r0)
vtest-core-2-r0 vtest-plugin-2-r0
== C: apk upgrade, feed has both
(1/2) Upgrading vtest-plugin (1-r0 -> 2-r0)
(2/2) Upgrading vtest-core (1-r0 -> 2-r0)
vtest-core-2-r0 vtest-plugin-2-r0
== D: apk upgrade, feed has new verso but no new plugin
ERROR: unable to select packages:
  vtest-plugin-1-r0:
    breaks: vtest-core-2-r0[!vtest-plugin<2-r0]
    satisfies: world[vtest-plugin]
vtest-core-1-r0 vtest-plugin-1-r0
```

Result: the syntax for task 10.2 is `!<plugin><<first registry version>`. apk moves the
two packages together, or refuses both and leaves the router as it was.

Consequence for the release: in scenario D, the whole `apk upgrade` refuses, including
unrelated packages. So the four plugins' new versions must be in the feed before, or
together with, the `verso` that conflicts with their old ones. The release notes and
the deploy order follow from this.

The test packages were removed afterwards: `apk info` and `/etc/apk/world` name no
`vtest` package.

## 3.4 `pluginCommand` against the real rpcd

Tested 2026-10-18 in the lab container, with the helper `make dev` deployed. The
throwaway plugin `regtest` had:
- a root-owned `commands.json` declaring a read `state`, an act `poke` (with argument
  `section: uci-name`) and a transform `files`;
- `/usr/libexec/verso-regtest` (0755), which echoes its request, and refuses `poke`
  with exit 3;
- an rpcd ACL granting `verso-plugin regtest` `state` under `read`, and `poke` and
  `files` under `write`.

The session was root's login after an `rpcd reload`. rpcd accepts the free-form
`verso-plugin` scope in `acl.d` and answers `session.access` for it.

```
== read
{"result":{"dir":"/","path":"/usr/sbin:/usr/bin:/sbin:/bin","request":{"args":{},"command":"state"},"uid":"0"},"status":0}
== act refused by its root half
{"error":"Nothing to poke.","status":2}
== act with a bad argument
{"error":"argument \"section\" is not a valid uci name","status":2}
== transform through the verb
{"error":"regtest registers no command \"files\"","status":3}
== ungranted command
{"error":"permission denied","status":6}
== zero session
{"error":"permission denied","status":6}
== world-writable executable
{"error":"regtest's commands are unavailable","status":9}
```

The container's `/usr` and `/usr/share` are `0775 root:root`: its image was built
with umask 002. The ownership walk therefore treats a group write by group root as
root's alone, and refuses any world write and any group write by another group.

Everything was removed afterwards: the registry, the executable, the ACL and the
session, followed by an rpcd reload.

## 11.1 The pages on `make dev`

Checked 2026-10-19 in the lab container at v0.9.0-60, with headless Chromium
(playwright-core 1.56.1). Screenshots are in `~/Share/screenshots/registry`.
The page emulates reduced motion: the cross-document view transition holds
headless Chromium's frames, so no screenshot completes without it.

For the run, the lab got a DDNS name (router.example.com, Cloudflare, a dummy
token), OpenVPN 2.7.7 with a `lab` profile holding a `<ca>` and a `<tls-crypt>`
block, and a rule file. All were removed afterwards.

- **Reads.** Each registered read answered through `pluginCommand` from its
  plugin's root half: `vpn.state`, `ddns.state`, `dnsdhcp.dhcp-state`,
  `dnsdhcp.dns-state`, `firewall.state` and `acme.state`.
  - The DHCP page says lan is Serving with one lease.
  - Interfaces' DHCP chip reads "DHCP server is running."
  - The firewall rules carry their hit counts.
  - DNS shows the resolver's settings and `/etc/dnsmasq.conf`.
  - DDNS says "private address": the lab's WAN is private.
  - VPN lists the `lab` profile.
  - The `files` transform cannot be reached through the verb.
- **The container hides socket owners.** Docker withholds `CAP_SYS_PTRACE`, so
  even root cannot read another process's socket links there, and `dns-state`
  lists no listeners (`netstat -p` shows `-` too). The router's root has the
  capability.
- **DDNS Update.** Update now answered "Sending the address now.", and the
  updater restarted (pid 1732911, then 1733489).
- **A rule file.**
  - Saving `20-lab-registry.nft` staged it.
  - Applying it with no check-in rolled it back after 30 s: the file was gone
    and fw4 dropped its chain.
  - The rollback kept the stage, as before this change.
  - Applying again with the page open kept it ("The router kept 1 change").
- **A profile.**
  - The profile tab showed `<ca> … 3 lines … </ca>` and
    `<tls-crypt> … 2 lines … </tls-crypt>`, and no key line reached the page.
  - Saving it with `verb 3` added staged the real blocks again (journal
    `0600 root`, file mode 0600).
  - Applying with no check-in rolled it back to the original.
  - Applying again kept it, with both key lines intact.
  - Config history holds the edit folded and no key line.
- **Removing ddns.** The package's own steps were followed: pre-deinstall,
  removing its files, then post-deinstall's SIGHUP to the shell.
  - `/plugins/ddns/` answered 404, and the nav lost Dynamic DNS.
  - `pluginCommand ddns state` answered "ddns registers no command" (status 3).
  - Redeploying brought all three back.

Found and fixed during the check:
- The firewall's Files band listed `/etc/dnsmasq.conf`. The firewall reads
  `dhcp`, so it is handed dnsmasq's family too. `FileSet` now names its family,
  and VPN keeps only `openvpn` files as profiles and taken names.
- The Interfaces DHCP chip read "could not be verified" until the shell
  restarted. The shell reads manifests at start and on SIGHUP, and the dev
  deploy of a changed manifest sends neither. The packages' install hooks do.
- `acme.state` was denied once for a session opened before acme's ACL gained the
  `verso-plugin` grant. Sessions take grants at login, and a fresh one was
  granted.

## 11.2 The upgrade on a router in service

On 2026-10-19 a Mono Gateway in daily use (Mono 25.12.5, layerscape/armv8_64b)
was upgraded with the local apks in one `apk add`, from verso 0.9.0-r39 and the
plugins before the move. apk upgraded acme, ddns, dnsdhcp and vpn first and verso
last, and refused nothing.

It then wrote `/etc/config/verso.apk-new`. That file is the stock default, which
is unchanged since r39, written beside the router's edited config. There was
nothing to merge.

A read-only tour afterwards: every page was read by GET, and the browser refused
any other request.
- **VPN.** The OpenVPN tunnel read connected for 5 d 16 h, with its traffic.
  The connection record survived its hook moving from verso to verso-plugin-vpn.
- **Interfaces.** Every DHCP chip read "DHCP server is running.", from
  `dnsdhcp.dhcp-state`.
- **DHCP.** Every server read Serving, with its lease count.
- **DNS.** The page named AdGuard Home on port 53. The root half reads the socket
  owners the lab container hides.
- **DDNS.** The name read "points here".
- **Firewall.** The rules carried their hits, and the rule files listed fw4's
  alone.
- **Access.** The certificate card read acme's state.

Not checked on this router: a staged file across the upgrade (none was staged),
and apk refusing verso alone (all six moved together). The refusal is the one
1.1 tested and 10.2 read back from the package.

## 10.2 verso's conflicts, as packaged

`make apk APK_GOARCH=arm64` at v0.9.0-59 built `verso-0.9.0-r59.apk`. Its
`apk adbdump` lists these under `depends`:

```
- ca-bundle
- firewall4
- kmod-nfnetlink-log
- nlbwmon
- '!verso-plugin-acme<0.1.0-r11'
- '!verso-plugin-ddns<0.2.0-r2'
- '!verso-plugin-dnsdhcp<0.3.1-r2'
- '!verso-plugin-vpn<0.3.0-r2'
```

The package has no `replaces` and no `provides`, and no path under it is a
plugin's (no hotplug script, no OpenVPN, acme, ddns or dnsmasq file). Each
floor is the version of the plugin's own commit that took its root work over:
vpn c7de0a5, acme r11, ddns r2, dnsdhcp 9a3da55. Each plugin depends on the
verso that no longer does that work itself (r53, r56, r57, r58).

## 10.1 The helper names no plugin

`main.rs`'s `no_verb_does_a_plugins_root_work` holds the verb table (`answer`,
split out of `dispatch` so the table answers without an rpcd to ask) and
`verso-helper.json` to none of the nine moved verbs.

The grep for `acme`, `openvpn` and `ddns` runs over the helper's code above each
file's tests, and finds nothing. The tests do name them: a registry, a family or
a grant needs a plugin to belong to, and the plugins that exist are the plainest
samples. `arrival.rs` names https-dns-proxy and dnsmasq. That is core's own
package handling (a package installed through Verso has its takeover of dnsmasq
undone), and no plugin's root work.
