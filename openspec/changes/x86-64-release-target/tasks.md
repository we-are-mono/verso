# Tasks

## 1. Per-arch build outputs

- [x] 1.1 Write the failing check first. From a clean `build/`, run `make apk APK_GOARCH=arm64 && make apk APK_GOARCH=amd64` and confirm that today only one `verso-<ver>.apk` survives, overwritten in place.
- [x] 1.2 Set `APK_DIR := $(BUILDDIR)/apk/$(APK_ARCH)` in the Makefile, with the i18n catalog under `build/apk/noarch/`. Verify by running the same two commands: `build/apk/aarch64_generic/verso-<ver>.apk` and `build/apk/x86_64/verso-<ver>.apk` both exist, and `apk adbdump` reports `arch: aarch64_generic` and `arch: x86_64` respectively.
- [x] 1.3 Reword the Makefile's arm64-only comments to state both release arches: the header's "Prototype build target is arm64" and `APK_GOARCH`'s "the router is arm64". `make lint` and `make test` stay green.
- [x] 1.4 Make the same `APK_DIR` change in each non-core plugin repository (dnsdhcp, qos, vpn, ddns), one commit per repository. Verify in each repo that `make apk APK_GOARCH=amd64` writes `build/apk/x86_64/<name>-<ver>.apk`.

## 2. Feed architecture check

- [x] 2.1 Write `scripts/check-feed-arch.test.cjs` first. Using fixture packages built with the buildroot's `apk mkpkg`, cover four cases: an all-`x86_64` dir passes for `x86_64`; a dir mixing `x86_64` and `noarch` passes; an `aarch64_generic` package in an `x86_64` dir fails, with output naming the file, `aarch64_generic` and `x86_64`; and the test skips with a reason when the apk tool is absent. Confirm it fails because the script is missing.
- [x] 2.2 Implement `scripts/check-feed-arch.sh <feed-dir> <apk-arch>` in POSIX shell over `apk adbdump`. Verify that `node --test scripts/check-feed-arch.test.cjs` passes, and that `make test` passes.
- [x] 2.3 Call the check in `apk-publish` and `apk-i18n-publish` before `mkndx`. Verify that `make apk-publish APK_GOARCH=amd64` passes, then that copying an `aarch64_generic` apk into the `x86_64` repo dir makes the next `make apk-i18n-publish APK_GOARCH=amd64` stop with the named mismatch.

## 3. Release loop (git-ignored local.mk)

- [x] 3.1 Split `asu-publish` into `asu-publish-arch` (today's body, plus `check-feed-arch.sh` over the full feed before `index.json`) and an `asu-publish` that loops `$(ARCHES)` with one sub-make each. Verify that after `make asu-publish`, `/srv/verso/aarch64_generic/` and `/srv/verso/x86_64/` hold the same package names, each with a `packages.adb` and an `index.json`. For `make version` = V, `verso` must be version V in both.
- [ ] 3.2 Loop `asu-push` over `$(ARCHES)` (stage, re-sign with the Verso key, rsync per arch) and upload `verso.pem` once. Verify that the public `x86_64/packages.adb` fetches and `apk adbdump --keys-dir <dir-with-verso.pem>` reports its signature OK.

## 4. Documentation

- [x] 4.1 docs/building.md: give the variables table, §2, §3 and §4 both arches (`aarch64_generic` / `x86_64` repo lines, `GOARCH=arm64|amd64`), and replace "Package another arch with `APK_GOARCH=amd64`" with the both-arch release. Verify that the documented commands run as written on the dev box.
- [x] 4.2 CONTRIBUTING.md: state both release arches, and replace the stale `VERSION`-file / `REVISION=2` sentence with the tag-derived version (`make version`). README.md: name x86_64 beside arm64 where it lists build outputs. Verify with `grep -n 'VERSION\|REVISION\|arm64 only' README.md CONTRIBUTING.md docs/building.md`, which must return no stale claims.
- [x] 4.3 openspec/config.yaml: replace "build and test for arm64 only" with the two release arches. Verify that `openspec instructions proposal --change x86-64-release-target` shows the new context.
- [ ] 4.4 Update docs/BACKLOG.md: remove any x86 or arch item this change closes, and add any follow-up the QEMU check in 5.1 turns up.

## 5. Integration check on x86_64 OpenWrt

- [ ] 5.1 Boot the official apk-era OpenWrt x86-64 image in QEMU with user-mode networking only (no bridge or tap onto the LAN, no watchdog device) and a host-forward for port 8080. Install the buildroot `public-key.pem`, point apk at `http://10.0.2.2:8079/x86_64/packages.adb`, and run `apk add verso verso-i18n-base-sl`. Verify a sign-in at the forwarded port and that every top-level page renders. File any page that breaks as a BACKLOG item, not in this change.
- [ ] 5.2 Confirm the arm64 path still holds: on the bench DUT, run `apk upgrade verso -U` from the `aarch64_generic` feed and check that it upgrades to V and signs in.

## Workflow follow-up

- Update the local deploy-verso skill: `make asu-push` publishes both arches; the ASU release-feed step (each Mono release's `verso/` subdir) copies `aarch64_generic` only; the follower onboarding shows both arches' repo lines.
- Update the project memory "Prototype target: arm64" to state both release arches.
- Archive the change with `/opsx:archive` once the tasks are done.
