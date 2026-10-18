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
