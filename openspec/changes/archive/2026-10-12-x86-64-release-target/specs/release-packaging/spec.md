# Spec Delta

## Purpose

Defines which CPU architectures a Verso release ships for, what each
architecture's apk feed contains, and the guards that keep every feed
installable and consistent.

## ADDED Requirements

### Requirement: Release architectures
A Verso release SHALL publish its packages for the OpenWrt package
architectures `aarch64_generic` and `x86_64`, each into a feed of its own
named after that architecture.

#### Scenario: One release command publishes both architectures
- **WHEN** the maintainer runs the release publishing command once
- **THEN** both the `aarch64_generic` and the `x86_64` feed hold the release's packages

#### Scenario: An x86_64 router installs Verso
- **WHEN** an apk-era OpenWrt x86_64 router that trusts the Verso feed key and lists the `x86_64` feed runs `apk add verso`
- **THEN** `verso` and its dependencies install, and the web UI accepts a sign-in over HTTPS, on 443, or on 8443 beside LuCI (ADR-017)

### Requirement: Complete feed per architecture
Each architecture's feed SHALL hold the `verso` package, every published
shell i18n catalog package, and every non-core plugin package, together with
a signed `packages.adb` index and an `index.json` for an attended-sysupgrade
server.

#### Scenario: Feed contents match across architectures
- **WHEN** a release has been published
- **THEN** the set of package names in the `x86_64` feed equals the set in the `aarch64_generic` feed

#### Scenario: Catalog packages appear in every feed
- **WHEN** a release publishes `verso-i18n-base-sl`
- **THEN** the package, recorded with architecture `noarch`, is listed in the index of both architectures' feeds

### Requirement: One version across architectures
Every package that comes from the Verso repository SHALL carry the same
version in every architecture's feed of one release. Each non-core plugin
SHALL carry its own repository's version, the same in every architecture's
feed.

#### Scenario: verso versions agree
- **WHEN** a release built from a commit that `make version` reports as `0.1.0-r7` is published
- **THEN** `verso` is version `0.1.0-r7` in both the `aarch64_generic` and the `x86_64` feed

### Requirement: Package architecture matches its feed
A package recorded with an architecture other than its feed's SHALL NOT be
published into that feed; `noarch` packages are accepted in every feed. The
refusal SHALL name the package, its recorded architecture and the feed's.

#### Scenario: Mismatched package refused
- **WHEN** a package recorded as `aarch64_generic` is about to be published into the `x86_64` feed
- **THEN** publishing stops before the feed is pushed to the public server and reports the package, `aarch64_generic` and `x86_64`

#### Scenario: Architecture-independent package accepted
- **WHEN** a package recorded as `noarch` is published into either feed
- **THEN** it is accepted

### Requirement: Architectures build without overwriting each other
Building packages for one architecture SHALL leave every package already
built for another architecture in place and unchanged.

#### Scenario: Building x86_64 after arm64
- **WHEN** the `verso` package is built for arm64 and then for amd64 from the same commit
- **THEN** both packages exist side by side, each recorded with its own architecture

### Requirement: Mono firmware feed stays aarch64
Verso's packages in the attended-sysupgrade release feed for Mono firmware
SHALL be the `aarch64_generic` packages only, because that server builds
images only for the Mono Gateway.

#### Scenario: Release feed for Mono firmware
- **WHEN** a release is published to the attended-sysupgrade release feed
- **THEN** only the `aarch64_generic` feed's packages are placed there, and the `x86_64` feed is published to the public feed alone
