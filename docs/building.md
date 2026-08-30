# Building & deploying Verso to a real OpenWrt router

How we package Verso as an OpenWrt **apk**, serve it from a tiny signed repo on
the dev box, and install/upgrade it on the production router — with **no SSH into
the router** (it pulls).

The build-and-publish half is now `make apk` / `make apk-publish` (§2–3); this
runbook explains what those do and why, plus the one-time server setup (§1) and
the router-side install (§4–5), which stay manual. The router pull-watcher is
still TODO (see the end).

---

## The variables this runbook uses

Substitute your own values; the `make` targets read the same ones (`OPENWRT_DIR`,
`VERSO_REPO_DIR`).

| Variable | What it is |
|---|---|
| Router arch (`CONFIG_TARGET_ARCH_PACKAGES`) | `aarch64_generic` (arm64) |
| Go cross-build | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` (pure-Go static, runs on musl) |
| `$OPENWRT_DIR` | your OpenWrt buildroot — holds the `apk` tool and the signing keys |
| apk tool (apk-tools 3.0.5) | `$OPENWRT_DIR/staging_dir/host/bin/apk` |
| Signing keys (RSA) | `$OPENWRT_DIR/{private,public}-key.pem` |
| `$VERSO_REPO_DIR` | the apk repo root on the dev box (this runbook's default: `/srv/verso`) |
| `$DEV_BOX` | the dev box's LAN address the router pulls from, e.g. `<ip>:8079` |

**Why the router trusts our packages with zero setup:** the router was flashed
from this same buildroot, so its `/etc/apk/keys/public-key.pem` is a byte-for-byte
match with `$OPENWRT_DIR/public-key.pem`. Anything signed with `private-key.pem`
verifies on the router — no key install, no `--allow-untrusted` on the router.

`make apk` derives both the apk tool and the signing key from `$OPENWRT_DIR`
(auto-detected) — you don't set these by hand.

---

## 1. The apk repo server (one-time, on the dev box)

A static HTTP server over `/srv/verso`, as a hardened systemd unit
(`/etc/systemd/system/verso-apk.service`):

```ini
[Unit]
Description=Verso apk repository (dev) — static HTTP server for /srv/verso
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/bin/python3 -m http.server 8079 --directory /srv/verso
Restart=on-failure
RestartSec=2
DynamicUser=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
```

```sh
sudo mkdir -p /srv/verso
sudo chown -R "$(id -un)":"$(id -gn)" /srv/verso   # so the deploy loop needs no sudo
sudo systemctl daemon-reload
sudo systemctl enable --now verso-apk.service
curl -sf "http://$DEV_BOX/"                          # verify it serves
```

---

## 2. Build & sign the package (on the dev box)

```sh
make apk       # -> build/apk/verso-<version>.apk, cross-built for arm64 and signed
```

One command. What it does, and why each part matters:

- **Cross-builds both binaries** — the Go shell (`CGO_ENABLED=0 GOOS=linux
  GOARCH=arm64`, pure-Go static) and the Rust `verso-rpcd` helper
  (`aarch64-unknown-linux-musl`). `verso-rpcd` is one long-running root daemon,
  not an rpcd exec plugin: the Go shell reaches it over
  `/var/run/verso/verso-rpcd.sock`, and every request carries the operator SID,
  which the helper validates through native ubus `session.access` before acting.
  Verso samples per-interface counters from `/sys/class/net` once per second
  (latest 60 samples, in memory); it does not enumerate conntrack or query ASK/FCI.

- **Assembles a CLEAN payload** — only Verso's own files. It never ships
  `/etc/config/*` or `netfix`; those are docker-harness defaults that would
  clobber the router's live network/firewall config. The eight files:

  ```
  /usr/bin/verso
  /usr/sbin/verso-rpcd
  /etc/init.d/verso
  /etc/init.d/verso-rpcd
  /etc/capabilities/verso.json
  /usr/share/acl.d/verso.json
  /usr/share/rpcd/acl.d/verso-shell.json
  /usr/share/rpcd/acl.d/verso-helper.json
  ```

- **Records root:root ownership — via `fakeroot`, no `sudo`.** `apk mkpkg` stamps
  ownership **by name** from the build tree, and ubusd **rejects any `acl.d` file
  not owned by root** ("has wrong owner") → it silently drops the ACL → Verso
  can't see the `session` object → every login fails with `ubus: object "session"
  not found`. The `chown -R 0:0` + `mkpkg` run inside one `fakeroot` session, so
  the package records root ownership while nothing on disk is actually root-owned.

- **Runs the post-install / post-upgrade hook** from `packaging/apk/post-install.sh`
  (committed, not `/tmp`): it creates the de-privileged `verso` user, reloads the
  ACLs (`-HUP ubusd`), and (re)starts the services.

- **Versions from the `VERSION` file** + a revision. `VER = <VERSION>-r<REVISION>`
  (default `REVISION=1`). Bump `VERSION` in a commit for a new version so `apk`
  sees an upgrade; pass `REVISION=2` to repackage the same version.

Sanity-check the result (arch, version, and that ownership is `root`). The apk
tool lives under your OpenWrt buildroot — the same one `make apk` uses:

```sh
APK="$OPENWRT_DIR/staging_dir/host/bin/apk"   # OPENWRT_DIR = your OpenWrt buildroot
$APK adbdump build/apk/verso-*.apk | grep -iE "name: verso$|version:|arch:"
$APK adbdump build/apk/verso-*.apk | grep -A3 "name: verso.json" | grep user:   # must say root
```

**Machine-agnostic:** the only per-host input is the OpenWrt buildroot, from which
`make apk` derives the apk tool and the signing key. It auto-detects a default
location (see `OPENWRT_DIR` in the Makefile); point it elsewhere with
`OPENWRT_DIR=/path/...` on the command line or in a gitignored `local.mk`.
Package another arch with `APK_GOARCH=amd64`.

---

## 3. Publish to the repo

```sh
make apk-publish   # runs `make apk`, then copies + rebuilds the signed index
```

This copies the fresh `.apk` into `VERSO_REPO_DIR` (default `/srv/verso`, the only
dev-box-specific path) under its arch subdir, and rebuilds the v3 index — which
**must be named `packages.adb`** (OpenWrt convention) — signed. `mkndx` runs with
`--allow-untrusted`, which only means "don't verify on the build host" (the host
doesn't trust our own key); the index is still **signed** with `--sign-key`, and
the router verifies it against its trusted `public-key.pem`.

Verify it serves:

```sh
curl -sf -o /dev/null -w "%{http_code}\n" "http://$DEV_BOX/aarch64_generic/packages.adb"
```

---

## 4. Install on the router (first time, on the router console)

The repo line must be the **full URL to `packages.adb`**, not a directory — a bare
dir makes apk fall back to the legacy `<arch>/APKINDEX.tar.gz` layout and it won't
find anything.

```sh
echo "http://$DEV_BOX/aarch64_generic/packages.adb" > /etc/apk/repositories.d/verso-dev.list
apk update
apk add verso
```

(Any `sysupgrade.mono.si` errors during `apk update` are the router's *own* feed,
unrelated — ignore them.) Verso listens on **`:8080`** (coexists with LuCI on 80).
Browse `http://<router-ip>:8080`.

---

## 5. The fast iteration loop (re-deploy)

On the dev box, `make apk-publish REVISION=<n>` (bump the revision, or `VERSION`,
so `apk` sees an upgrade). Then on the router:

```sh
apk update && apk add --upgrade verso && killall -HUP ubusd && /etc/init.d/verso-rpcd restart && /etc/init.d/verso restart
```

The `-HUP ubusd` reloads the ACLs; the restart runs the new binary. The login
rate-limiter is in-memory, so a restart also clears any lockout.

---

## Gotchas we hit (so we don't again)

1. **Repo line = full `…/packages.adb` URL**, not a directory. A bare dir →
   apk tries the legacy `…/<arch>/APKINDEX.tar.gz` and fails.
2. **Payload must be `root:root`.** `apk mkpkg` preserves the build-tree owner by
   name; ubusd drops any non-root `acl.d` file → `object "session" not found` on
   every login. `make apk` handles this with `fakeroot` (chown + mkpkg in one
   fakeroot session) — no `sudo`, and re-runs can still clean the payload tree.
3. **Index filename is `packages.adb`** (apk v3), not `APKINDEX.adb`/`.tar.gz`.
4. **Never ship `/etc/config/*` or `netfix`** — they'd overwrite the router's live
   network/firewall config.
5. **Auth null-byte decoy bug** (code, now fixed): the passwordless-login guard
   probed with `password + "\x00decoy-…"`; rpcd/crypt truncates at the NUL, so the
   decoy collapsed onto the real password and every real login was rejected. The
   decoy is now a fresh random string with no NUL. Reproduce locally by setting a
   root password in the dev container (`passwd root`) — it's passwordless by
   default, which is why this hid.

---

## TODO — automation (not built yet)

The dev-box half — cross-build → `mkpkg` (versioned, `fakeroot`-owned, signed) →
`mkndx` → publish — is now `make apk` / `make apk-publish`. What's left:

- **Pull-watcher on the router** (no SSH): a tiny procd service that polls a
  version stamp on the repo and runs the §5 upgrade line when it changes — so the
  loop is one command on the dev box and the router self-updates. Dev-only; ship
  disabled.
