# Building & deploying Verso to a real OpenWrt router

How we package Verso as an OpenWrt **apk**, serve it from a tiny signed repo on
the dev box, and install/upgrade it on the production router — with **no SSH into
the router** (it pulls).

The build-and-publish half is `make apk` / `make apk-publish` (§2–3); this
runbook explains what those do and why, plus the one-time server setup (§1) and
the install and upgrade, which are run on the router's console (§4–5).

---

## The variables this runbook uses

Substitute your own values; the `make` targets read the same ones (`OPENWRT_DIR`,
`VERSO_REPO_DIR`).

| Variable | What it is |
|---|---|
| Router arch (`CONFIG_TARGET_ARCH_PACKAGES`) | `aarch64_generic` (arm64) or `x86_64` (amd64); every release ships both |
| Go cross-build | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` or `GOARCH=amd64` (pure-Go static, runs on musl) |
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
make apk                    # -> build/apk/aarch64_generic/verso-<version>.apk, cross-built for arm64 and signed
make apk APK_GOARCH=amd64   # -> build/apk/x86_64/verso-<version>.apk
```

One command per arch; each arch builds under a directory of its own, so neither
overwrites the other. What it does, and why each part matters:

- **Cross-builds both binaries** — the Go shell (`CGO_ENABLED=0 GOOS=linux
  GOARCH=arm64` or `amd64`, pure-Go static) and the Rust `verso-rpcd` helper
  (`aarch64-unknown-linux-musl` or `x86_64-unknown-linux-musl`). `verso-rpcd`
  is one long-running root daemon, not an rpcd exec plugin: the Go shell reaches
  it over
  `/var/run/verso-rpcd/verso-rpcd.sock`, only to a listener the kernel reports
  as root, and every request carries the operator SID, which the helper
  validates through native ubus `session.access` before acting.
  Verso samples per-interface counters from `/sys/class/net` once per second
  (latest 60 samples, in memory); it does not enumerate conntrack or query ASK/FCI.

- **Compiles the stylesheet with a digest-pinned Tailwind.** The build
  regenerates `internal/server/assets/verso.css`, which is embedded in the
  signed binary, with the Tailwind standalone CLI from its GitHub release. A
  release asset can be replaced under the same tag and name, so the Makefile
  records its SHA-256 (`TAILWIND_SHA256`) beside the version: the download
  lands in a temporary directory of its own beside `build/tools/tailwindcss`
  (so two builds fetching at once never share a file), is checked with
  `sha256sum -c`, and only then is made executable and moved into place; the
  temporary directory is removed however the fetch ends. The cached file is
  checked again in the same shell command that runs it. On a mismatch the
  file is deleted and the build stops, naming the file and the expected
  digest; the next run fetches it afresh. golangci-lint, which `make lint`
  and the pre-commit hook run, is fetched the same way: `GOLANGCI_TARBALL_SHA256` checks the release tarball
  (its line in `golangci-lint-<version>-checksums.txt`) and `GOLANGCI_SHA256`
  the `golangci-lint` binary extracted from it, on download and before every
  run. The pinned builds are Linux x86_64; on any other build host (an Intel
  Mac included), or one without `sha256sum` on `PATH`, the download stops
  before fetching and says so. To bump either tool, change its version and
  every digest of it together, taking the digests from the release's checksum
  file.
  `deadcode` installs with `go install` under the same checksum-database
  enforcement as the [vulnerability scanners](#vulnerability-checks).

- **Assembles a CLEAN payload** — only Verso's own files. It never ships
  `/etc/config/*` or `netfix`; those are docker-harness defaults that would
  clobber the router's live network/firewall config. The nine files:

  ```
  /usr/bin/verso
  /usr/sbin/verso-rpcd
  /etc/init.d/verso
  /etc/init.d/verso-rpcd
  /etc/capabilities/verso.json
  /usr/share/acl.d/verso.json
  /usr/share/rpcd/acl.d/verso-shell.json
  /usr/share/rpcd/acl.d/verso-helper.json
  /usr/share/rpcd/acl.d/verso-admin.json
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

- **Depends on OpenWrt's `ca-bundle` package.** Available refreshes the official
  HTTPS feed indexes through `apk update`; declaring the trust bundle prevents a
  minimal image from failing every refresh with TLS verification errors.

- **Requires `firewall4` and `kmod-nfnetlink-log`.** Firewall packet logs use
  NFLOG to bypass the kernel/system logs. The kernel module must come from the
  feed built for the router's exact kernel ABI; it does not belong in Verso's
  architecture-only feed. For custom firmware, select
  `CONFIG_PACKAGE_kmod-nfnetlink-log=m` before building the firmware and its
  module feed (`=y` also installs it in the image). If the existing build omitted
  it, adding the option changes OpenWrt's kernel ABI identifier: rebuild and
  deploy matching firmware and modules together. Docker tests use the host's
  kernel and cannot verify that the router's module feed is complete.

- **Versions from the release tag.** A build is `<X.Y.Z>-r<N>`: the latest
  `vX.Y.Z` tag and how many commits stand on it (`scripts/version.sh`; `make
  version` prints it). The tagged commit builds `0.1.0-r0`, the third commit
  after it `0.1.0-r3`, so every commit is an upgrade `apk` sees. A new version
  is a new annotated tag (`git tag -a v0.2.0 -m "Verso 0.2.0"`). The binary,
  the login page, the colophon and every package state the same string.

Sanity-check the result (arch, version, and that ownership is `root`). The apk
tool lives under your OpenWrt buildroot — the same one `make apk` uses:

```sh
APK="$OPENWRT_DIR/staging_dir/host/bin/apk"   # OPENWRT_DIR = your OpenWrt buildroot
$APK adbdump build/apk/aarch64_generic/verso-*.apk | grep -iE "name: verso$|version:|arch:"
$APK adbdump build/apk/aarch64_generic/verso-*.apk | grep -A3 "name: verso.json" | grep user:   # must say root
```

**Machine-agnostic:** the only per-host input is the OpenWrt buildroot, from which
`make apk` derives the apk tool and the signing key. It auto-detects a default
location (see `OPENWRT_DIR` in the Makefile); point it elsewhere with
`OPENWRT_DIR=/path/...` on the command line or in a gitignored `local.mk`.
A release packages every arch in the Makefile's `ARCHES` (arm64 and amd64), once
per arch.

---

## 3. Publish to the repo

```sh
make apk-publish                    # runs `make apk`, then copies + rebuilds the signed index
make apk-publish APK_GOARCH=amd64   # the same for the x86_64 feed
```

It runs `make vuln` first ([Vulnerability checks](#vulnerability-checks)): an
untriaged advisory against anything the package ships stops the publish before
the build. Then it copies the fresh `.apk` into `VERSO_REPO_DIR` (default `/srv/verso`, the only
dev-box-specific path) under its arch subdir (`aarch64_generic/` or `x86_64/`),
checks that every package there is built for that arch or for none
(`scripts/check-feed-arch.sh`, which names any that is not and stops), and
rebuilds the v3 index — which
**must be named `packages.adb`** (OpenWrt convention) — signed. `mkndx` runs with
`--allow-untrusted`, which only means "don't verify on the build host" (the host
doesn't trust our own key); the index is still **signed** with `--sign-key`, and
the router verifies it against its trusted `public-key.pem`.

Verify it serves:

```sh
curl -sf -o /dev/null -w "%{http_code}\n" "http://$DEV_BOX/aarch64_generic/packages.adb"
curl -sf -o /dev/null -w "%{http_code}\n" "http://$DEV_BOX/x86_64/packages.adb"
```

---

## 4. Install on the router (first time, on the router console)

The repo line must be the **full URL to `packages.adb`**, not a directory — a bare
dir makes apk fall back to the legacy `<arch>/APKINDEX.tar.gz` layout and it won't
find anything. Use the router's own arch: `aarch64_generic` on arm64, `x86_64` on
an x86_64 router.

```sh
ARCH=aarch64_generic   # or x86_64
echo "http://$DEV_BOX/$ARCH/packages.adb" > /etc/apk/repositories.d/verso-dev.list
apk update
apk add verso
```

The router's normal userspace and matching kernel-module feeds must also be
enabled and reachable so apk can resolve Verso's dependencies.
Verso serves HTTPS on **`:443`**, or on **`:8443`** when LuCI's uhttpd holds
443 ([ADR-017](ADR/017-tls-and-the-web-certificate.md)). Browse
`https://<router-ip>/` or `https://<router-ip>:8443/`.

---

## 5. The fast iteration loop (re-deploy)

On the dev box, commit, then `make apk-publish` (each commit builds a higher
revision, so `apk` sees an upgrade). Then on the router:

```sh
apk upgrade verso -U
```

- `-U` (`--update-cache`) refreshes the indexes, so a separate `apk update` is
  unnecessary. Naming `verso` targets its upgrade and dependency resolution.
- Keep the normal feeds enabled: a new Verso version can introduce a dependency
  that is not installed yet. Restricting `--repositories-file` to Verso's feed
  prevents apk from finding those dependencies.

If apk reports `kmod-nfnetlink-log (no such package)`, first check `apk update`
for feed errors and run `apk search -x kmod-nfnetlink-log`. On custom firmware,
check that the matching module was actually built and published. Removing the
dependency or forcing a module from another kernel does not supply NFLOG support.

The `post-upgrade` hook does the rest: it reloads the ubusd + rpcd ACLs
(`killall -HUP ubusd`, `rpcd reload`) and restarts verso + verso-rpcd. The login
rate-limiter is in-memory, so the restart also clears any lockout. **An ACL or
manifest change lands here** — through a real reinstall — not via `make dev`'s
binary hot-swap.

---

## Vulnerability checks

```sh
make vuln
```

`make vuln` checks everything a release ships against the published advisories
and fails on any it has not been told to accept. It needs the network: the
advisory databases are fetched on every run.

| What | Scanner | Fails on |
|---|---|---|
| The Go module and the standard library of the toolchain that builds it, for each release target (`linux/arm64`, `linux/amd64`, `CGO_ENABLED=0`) | `govulncheck ./...` (Go vulnerability database) | an advisory whose vulnerable function Verso's code calls on any target |
| `verso-rpcd`, the plugin SDK and the System, Firewall and Interfaces plugins: every committed `Cargo.lock` | `osv-scanner` (OSV.dev, which carries the RustSec advisories) | any advisory against a pinned crate version |
| The vendored scripts, htmx and Alpine (CSP build) | `osv-scanner`, reading the vendored-scripts manifest | any advisory against the recorded npm version |

Go advisories the code imports but never calls, or only requires, are listed
for the record and do not fail the run; a Rust or npm advisory has no such
analysis, so a pinned affected version counts as reached.
`cmd/vulngate` reads both scanners' JSON results, sets them against the
ignore list, prints one table, and decides. It refuses a result that cannot
carry that decision: a govulncheck scan below symbol level (which follows no
calls) or against any database but `https://vuln.go.dev`, and an osv-scanner
result missing any file it was asked to read (osv-scanner leaves out a
lockfile that lists no package). osv-scanner runs with `--config /dev/null`,
so an `osv-scanner.toml` beside a lockfile cannot drop findings past the
ignore list.

**One scanner per ecosystem it does best.** govulncheck is the Go team's
scanner and the only one that follows calls into the standard library and
modules. osv-scanner reads Cargo lockfiles and the scripts' manifest against
one database, OSV.dev, which mirrors RustSec and the GitHub advisories for npm,
so the Rust crates and the vendored scripts share one check instead of
cargo-audit plus a second tool for npm. Both are Go programs, so both install
the way the linters do.

**Pinned and verified tools.** The Makefile pins each scanner's exact version
(`GOVULNCHECK_VERSION`, `OSV_SCANNER_VERSION`) and builds it with `go install
<module>@<version>` into `build/tools/<tool>-<version>/`. The module and every
dependency are checked against the Go checksum database: the install forces
`GOSUMDB=sum.golang.org`, clears `GONOSUMDB`, `GOPRIVATE` and `GOFLAGS`, and
sets `GOENV=off`, so neither the environment nor a `go env -w` setting can
switch the check off (go reads an empty variable as unset and falls back to
that file). Bumping a version installs afresh.

**The ignore list.** `vuln-ignore.txt` at the repository root holds the
advisories triaged as not stopping a release, one per line:

```
GO-2026-0000 2027-01-31 the vulnerable parser only reads the build host's own files
```

The advisory may be named by any identifier the report prints for it (GO-,
RUSTSEC-, GHSA-, CVE-). The date is required, at most 180 days out, and the
entry lapses the day after it, so every triage is revisited; the reason is
required. `make vuln` names lapsed entries and entries that match no finding,
so the list stays short. Fixing the dependency is preferred over an entry.

**The vendored scripts.** `internal/server/assets/vendored-js.json` records,
for each third-party script the shell embeds, its npm package and version, the
upstream URL of the exact file, and its SHA-256. `go test ./internal/server`
hashes the embedded files against it (`TestVendoredScriptsMatchTheirRecord`)
and fails on any embedded script outside the shell's own `verso*.js` that has
no record (`TestEveryVendoredScriptIsRecorded`). The manifest is in
osv-scanner's custom-lockfile shape, so `make vuln` checks the same versions
the test pins. To upgrade a script: fetch the file from its npm release (the
`url` with the new version), replace it under `internal/server/assets/`, and
update `version`, `url` and `sha256` together.

**When it runs.** `make apk-publish` runs it before building, so a package
reaches no feed past an untriaged advisory. The GitHub Actions workflow
`.github/workflows/vuln.yml` runs it on every push and pull request, and daily
at 05:17 UTC, so an advisory published against an unchanged tree still
surfaces. It is not in the pre-commit hook, which runs offline.

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
   decoy is a fresh random string with no NUL. Reproduce locally by setting a
   root password in the dev container (`passwd root`) — it's passwordless by
   default, which is why this hid.
