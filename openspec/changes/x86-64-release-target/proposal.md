# Proposal

## Why

Anyone running apk-era OpenWrt on an x86_64 machine (a VM, a mini-PC, a spare
PC with two NICs) can install Verso from the feed with `apk add verso` and
follow its releases with `apk upgrade verso`, as an arm64 follower does today.
Verso already runs on x86_64: the docker testbed is x86_64 OpenWrt running the
amd64 binaries, and ADR-001 sets the hardware floor at "aarch64 or x86_64". What
falls short is the release: every packaging and publishing step ships one
architecture per run, defaults to arm64, and two architectures built in turn
overwrite each other's package file.

## What Changes

- Every release builds, signs and publishes Verso for both `aarch64_generic`
  and `x86_64`, all from one command: the `verso` package, the shell's i18n
  catalogs and every non-core plugin.
- Built packages land in per-architecture output paths, so building one
  architecture never overwrites another's package.
- A package whose recorded architecture differs from the feed directory it is
  published into is refused at publish time, naming both.
- The architecture-independent i18n catalogs are published into every
  architecture's feed, so each feed is complete on its own.
- The README, CONTRIBUTING and docs/building.md describe both architectures as
  release targets, and the "arm64 only" project guidance goes, along with the
  stale `VERSION`-file / `REVISION=` text in CONTRIBUTING.
- The ASU release feed for Mono firmware stays `aarch64_generic` only, because
  the attended-sysupgrade server builds images only for the Mono Gateway.
  x86_64 followers get their packages from the public feed.

## Capabilities

### New Capabilities

- `release-packaging`: which architectures a Verso release ships for, what
  each architecture's feed contains, and the guards that keep a feed
  consistent (one version across all packages, every package's architecture
  matching its feed).

### Modified Capabilities

None. No spec exists yet.

## Impact

- **ADRs:** rests on ADR-001 (one static artifact per arch; floor "aarch64 or
  x86_64"), which already states this and needs no amendment. ADR-011's plugin
  feeds and ADR-012's per-language catalog packages apply per architecture
  unchanged.
- **Build:** `Makefile` (`apk`, `apk-publish`, `apk-i18n-publish`, the
  per-arch output layout, the arm64-only comments). The git-ignored `local.mk`
  publishing targets loop over the release architectures.
- **Non-core plugin repositories** (dnsdhcp, qos, vpn, ddns) already build
  amd64 and take `APK_GOARCH`. Each gets the same per-arch output path; the
  feed-wide architecture check covers their packages.
- **Docs:** README.md, CONTRIBUTING.md, docs/building.md,
  openspec/config.yaml.
- **Runtime code:** none. No Go or Rust source changes.
- **Release time:** each release builds roughly twice as much, because the
  Rust crates and the Go shell compile once per architecture.
