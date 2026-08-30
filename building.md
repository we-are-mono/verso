# Building & deploying Verso to a real OpenWrt router

How we package Verso as an OpenWrt **apk**, serve it from a tiny signed repo on
the dev box, and install/upgrade it on the production router — with **no SSH into
the router** (it pulls). This is the manual runbook we verified end to end; the
`make deploy-prod` + pull-watcher automation is still TODO (see the end).

---

## The environment (specific to this setup)

| Thing | Value |
|---|---|
| Router arch (`CONFIG_TARGET_ARCH_PACKAGES`) | `aarch64_generic` |
| Go cross-build | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` (pure-Go static, runs on musl) |
| apk tool (apk-tools 3.0.5, from the OpenWrt tree) | `~/Mono/Gateway/openwrt/source/staging_dir/host/bin/apk` |
| Signing keys (RSA) | `~/Mono/Gateway/openwrt/source/{private,public}-key.pem` |
| Dev box repo URL (LAN, `br0` = `10.0.0.232`) | `http://10.0.0.232:8079/` |
| Repo root on disk | `/srv/verso/` |

**Why the router trusts our packages with zero setup:** the router was flashed
from this same buildroot, so its `/etc/apk/keys/public-key.pem` is a byte-for-byte
match with `source/public-key.pem`. Anything signed with `private-key.pem`
verifies on the router — no key install, no `--allow-untrusted` on the router.

Handy shell vars used below:

```sh
APK=~/Mono/Gateway/openwrt/source/staging_dir/host/bin/apk
KEY=~/Mono/Gateway/openwrt/source/private-key.pem
```

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
curl -sf http://10.0.0.232:8079/                    # verify it serves
```

---

## 2. Build & sign the package (on the dev box)

### 2a. Cross-build the Go shell and persistent Rust helper

```sh
P=/tmp/verso-pkg
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o $P/usr/bin/verso ./cmd/verso
cargo build --locked --release --manifest-path verso-rpcd/Cargo.toml --target aarch64-unknown-linux-musl
install -Dm755 verso-rpcd/target/aarch64-unknown-linux-musl/release/verso-rpcd $P/usr/sbin/verso-rpcd
```

`verso-rpcd` is one long-running root daemon, not an rpcd exec plugin. The Go
shell reaches it over `/var/run/verso/verso-rpcd.sock`; every request still
carries the operator SID, which the Rust helper validates through native ubus
`session.access` before acting. Verso itself samples standard per-interface
counters from `/sys/class/net` once per second and retains the latest 60 samples
in memory. It does not enumerate conntrack or query ASK/FCI.

### 2b. Assemble a CLEAN payload

Include only Verso's own files. **Do NOT ship `/etc/config/*` or `netfix`** —
those are docker-harness defaults and would clobber the router's network/firewall
config.

```sh
install -Dm755 docker/rootfs/etc/init.d/verso                       $P/etc/init.d/verso
install -Dm755 docker/rootfs/etc/init.d/verso-rpcd                  $P/etc/init.d/verso-rpcd
install -Dm644 docker/rootfs/etc/capabilities/verso.json           $P/etc/capabilities/verso.json
install -Dm644 docker/rootfs/usr/share/acl.d/verso.json            $P/usr/share/acl.d/verso.json
install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-shell.json  $P/usr/share/rpcd/acl.d/verso-shell.json
install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-helper.json $P/usr/share/rpcd/acl.d/verso-helper.json
```

Final payload:

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

### 2c. Own the payload as root:root — CRITICAL

`apk mkpkg` records file ownership **by name** from the build tree. ubusd
**rejects any `acl.d` file not owned by root** ("has wrong owner") and silently
drops the ACL → Verso can't see the `session` object → every login fails with
`ubus: object "session" not found`. So:

```sh
sudo chown -R 0:0 $P
```

### 2d. The post-install / post-upgrade script

`/tmp/verso-postinst.sh` — creates the de-privileged user, reloads the ACLs, and
(re)starts the service:

```sh
#!/bin/sh
grep -q '^verso:' /etc/group  2>/dev/null || echo 'verso:x:6000:' >> /etc/group
grep -q '^verso:' /etc/passwd 2>/dev/null || echo 'verso:x:6000:6000:verso:/var/run/verso:/bin/false' >> /etc/passwd
[ -x /etc/init.d/rpcd ] && /etc/init.d/rpcd reload 2>/dev/null
killall -HUP ubusd 2>/dev/null
/etc/init.d/verso enable 2>/dev/null
/etc/init.d/verso-rpcd enable 2>/dev/null
/etc/init.d/verso-rpcd restart 2>/dev/null
/etc/init.d/verso restart 2>/dev/null
exit 0
```

### 2e. Package + sign

Bump `VER` every build so `apk` sees an upgrade.

```sh
VER=0.0.3-r1
$APK mkpkg \
  --info name:verso --info version:$VER --info arch:aarch64_generic \
  --info "description:Verso — a modern web UI for OpenWrt" \
  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
  --info origin:verso \
  --files "$P" \
  --script post-install:/tmp/verso-postinst.sh \
  --script post-upgrade:/tmp/verso-postinst.sh \
  --sign-key "$KEY" \
  --output /tmp/verso-$VER.apk
```

Sanity-check (arch, version, and that ownership is `root`):

```sh
$APK adbdump /tmp/verso-$VER.apk | grep -iE "name: verso$|version:|arch:"
$APK adbdump /tmp/verso-$VER.apk | grep -A3 "name: verso.json" | grep user:   # must say root
```

---

## 3. Publish to the repo

The v3 index file **must be named `packages.adb`** (OpenWrt convention). Rebuild
it over whatever `.apk`s are present, signed:

```sh
rm -f /srv/verso/aarch64_generic/*.apk
cp /tmp/verso-$VER.apk /srv/verso/aarch64_generic/
( cd /srv/verso/aarch64_generic && $APK mkndx --allow-untrusted --sign-key "$KEY" --output packages.adb *.apk )
chmod -R a+rX /srv/verso
```

`--allow-untrusted` here only means "don't verify on the build host" (the host
doesn't trust our own key) — the index is still **signed** with `--sign-key`, and
the router verifies it against its trusted `public-key.pem`.

Verify it serves:

```sh
curl -sf -o /dev/null -w "%{http_code}\n" http://10.0.0.232:8079/aarch64_generic/packages.adb
```

---

## 4. Install on the router (first time, on the router console)

The repo line must be the **full URL to `packages.adb`**, not a directory — a bare
dir makes apk fall back to the legacy `<arch>/APKINDEX.tar.gz` layout and it won't
find anything.

```sh
echo 'http://10.0.0.232:8079/aarch64_generic/packages.adb' > /etc/apk/repositories.d/verso-dev.list
apk update
apk add verso
```

(Any `sysupgrade.mono.si` errors during `apk update` are the router's *own* feed,
unrelated — ignore them.) Verso listens on **`:8080`** (coexists with LuCI on 80).
Browse `http://<router-ip>:8080`.

---

## 5. The fast iteration loop (re-deploy)

Repeat §2e (bump `VER`) → §3, then on the router:

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
   every login. `sudo chown -R 0:0 $P` before packaging.
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

- **`make deploy-prod`** on the dev box: cross-build → `apk mkpkg` (bumped version,
  `chown 0:0`, sign) → `mkndx` → publish to `/srv/verso`. Fold in every gotcha
  above.
- **Pull-watcher on the router** (no SSH): a tiny procd service that polls a
  version stamp on the repo and runs the §5 upgrade line when it changes — so the
  loop is one command on the dev box and the router self-updates. Dev-only; ship
  disabled.
