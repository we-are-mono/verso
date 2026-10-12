# Design

## Context

- `make build` already cross-compiles the Go shell, `verso-rpcd` and the core
  plugins for every arch in `ARCHES ?= amd64 arm64`. `make apk` already maps
  `APK_GOARCH=amd64` to the apk arch `x86_64`.
- `APK_ARCH` is resolved once, when make parses the Makefile, from
  `APK_GOARCH`. One make invocation therefore packages exactly one arch.
- `APK_OUT` is `build/apk/verso-<ver>.apk` for every arch, and the payload
  tree `build/apk/pkg` is shared. The four non-core plugin repositories
  repeat the same layout, `build/apk/<name>-<ver>.apk`.
- `apk-publish` clears `$(VERSO_REPO_DIR)/$(APK_ARCH)/*.apk` before copying.
  The non-core plugins and `apk-i18n-publish` add to that dir and re-index.
- Release publishing (`asu-publish`, `asu-push`) lives in the git-ignored
  `local.mk`, because it names the deployment host and keys. It runs once per
  `APK_GOARCH`.
- The x86_64 runtime is already exercised. The docker testbed is x86_64
  OpenWrt and runs the amd64 binaries, and `scripts/dev.sh` hot-swaps
  `x86_64-unknown-linux-musl` builds into it.

## Goals / Non-Goals

**Goals:**
- A single `make asu-push` publishes complete, consistent feeds for both
  arches.
- Per-arch build outputs, mirroring the feed layout.
- A feed-wide architecture check before anything is pushed.

**Non-Goals:**
- Attended-sysupgrade images for x86 targets. The ASU server builds only
  Mono Gateway firmware.
- Any change to Go or Rust source. Runtime behaviour on x86_64 hardware
  without Mono's offload (ASK/CMM) or its TMU sensors is already what the
  testbed shows.
- More arches (armv7, mips). `ARCHES` stays the one list to extend.

## Decisions

### 1. Per-arch output directories: `build/apk/<apk-arch>/`

`APK_DIR` becomes `$(BUILDDIR)/apk/$(APK_ARCH)`, which carries the payload
tree and the `.apk` along with it. The i18n catalog builds into
`build/apk/noarch/`. Each non-core plugin repository makes the same
one-line change.

*Alternative:* put the arch in the filename (`verso-<ver>-x86_64.apk`).
Rejected. apk names packages by `name-version` inside the feed, so the
filename would have to be renamed back when published. The directory form is
byte-for-byte the feed layout.

### 2. The release loops over `ARCHES` with one sub-make per arch

`asu-publish` becomes a loop:
`for a in $(ARCHES); do $(MAKE) asu-publish-arch APK_GOARCH=$$a; done`.
Here `asu-publish-arch` is the current single-arch body, which runs
`apk-publish`, `apk-i18n-publish`, each non-core plugin, the arch check and
`index.json`. `asu-push` loops the same way over the staged, Verso-key
re-signed indexes, and uploads `verso.pem` once.

*Alternative:* pattern rules (`apk-arm64`, `apk-amd64`) with target-specific
variables. Rejected. Every recipe would have to be rewritten to use `$*`
lookups, and the plugin repositories would have to follow, all to save one
sub-make per arch. The recursive form keeps each arch's build exactly the
single-arch build that is already proven.

`ARCHES` is reused rather than a second `RELEASE_ARCHES` list. The arches
Verso compiles for are the arches it ships, and two lists would drift.

### 3. `scripts/check-feed-arch.sh <feed-dir> <apk-arch>`

The script reads every `.apk` in the dir with `apk adbdump` and fails at the
first package whose `arch` is neither the feed's nor `noarch`. It prints the
package file, its recorded arch and the feed's.

It runs in two places:
- inside `apk-publish` and `apk-i18n-publish`, before `mkndx`, which guards
  the public Makefile's own path;
- in `asu-publish-arch`, over the complete feed after every plugin has added
  its packages and before `index.json` is written. That covers the non-core
  plugins without copying the script into four repositories.

`asu-push` runs only after `asu-publish` succeeds, so a mismatched feed is
never pushed.

*Alternative:* a check in each plugin repository. Rejected as four copies of
one rule. The plugins' arch comes from the same `APK_GOARCH`, so the only
failure the check exists for is a mistake in the release loop, and the
feed-wide check catches exactly that.

The script is a plain POSIX shell over `apk adbdump` output. Its test,
`scripts/check-feed-arch.test.cjs`, uses the repository's existing script-test
idiom: `node --test scripts/*.test.cjs`, which `make test` already runs. The
test builds small unsigned fixture packages into a temp dir with the
buildroot's `apk mkpkg` (one `x86_64`, one `aarch64_generic`, one `noarch`)
and runs the script against mixed dirs. It marks itself skipped, with the
reason, when the apk tool is absent, so `make test` stays green on a machine
without a buildroot.

### 4. The ASU release-feed step stays `aarch64_generic`

The step that places Verso into each Mono firmware release dir as its
`verso/` subdir copies only the `aarch64_generic` feed. This step is part of
the local deploy skill, not the repository. `x86_64` goes to the public feed
alone, the spec's "Mono firmware feed stays aarch64" requirement.

### 5. Acceptance on a real x86_64 OpenWrt, in a QEMU VM

The acceptance check boots the official apk-era OpenWrt x86-64 image in
QEMU with **user-mode networking only**. There is no bridge or tap onto the
dev host's LAN, and the VM has no `/dev/watchdog`. The VM reaches the dev
box's local feed at `10.0.2.2:8079`. The buildroot's `public-key.pem` goes
into the VM's `/etc/apk/keys/`, then `apk add verso`, and the UI is reached
through a QEMU host-forward of port 8080.

*Alternative:* a fresh docker container from the testbed image. Rejected as
the acceptance check. A container shares the host kernel, so the
`kmod-nfnetlink-log` dependency resolves against nothing real and procd's
jail is absent. A VM proves the package against a stock x86_64 install.

## Risks / Trade-offs

- [Release takes about twice as long, because the Rust crates and the Go
  shell compile once per arch] → Accepted. Cargo's per-target dirs keep
  incremental builds, and a release is not the inner dev loop (`make dev`
  stays x86_64-only).
- [x86_64 followers see Mono-specific surfaces with nothing behind them
  (offload state, Gateway sensors)] → The testbed already renders these
  without Mono hardware. The QEMU check walks every top-level page, and any
  page that breaks becomes its own change.
- [`kmod-nfnetlink-log` must come from the follower's own kmods feed] →
  Official x86_64 images ship that feed. docs/building.md already explains
  the custom-firmware case.
- [The buildroot key signs both arches' packages] → No new trust. The public
  feed's index is re-signed with the Verso key exactly as it is for arm64.
