# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

# Verso shell — build/test tooling.
# A release ships for every arch in ARCHES: arm64 (aarch64_generic) and amd64 (x86_64).

# Optional per-machine overrides (gitignored). Anything set here wins over the
# `?=` defaults below — e.g. OPENWRT_DIR, KEY, VERSO_REPO_DIR — so nobody edits
# this file to package on their own box.
-include local.mk

BINARY   := verso
CMD      := ./cmd/verso
GOOS     ?= linux
BUILDDIR := build
CARGO    ?= $(if $(wildcard $(HOME)/.cargo/bin/cargo),$(HOME)/.cargo/bin/cargo,cargo)
RPCD_MANIFEST := verso-rpcd/Cargo.toml
# The core plugins, built and shipped inside the verso package; each is
# plugins/verso-plugin-<name>, a crate of its own. Every other plugin is a
# repository of its own that builds and packages itself.
PLUGINS := interfaces system firewall
plugin_manifest = plugins/verso-plugin-$(1)/Cargo.toml
# Every Rust crate the tree tests and lints: the helper, the SDK, the plugins.
CRATES := $(RPCD_MANIFEST) plugins/verso-plugin-sdk/Cargo.toml $(foreach p,$(PLUGINS),$(call plugin_manifest,$(p)))

# cargo_plugin builds one plugin for one arch and copies it beside the shell. A
# canned recipe: each line runs as its own recipe line when expanded in a loop.
define cargo_plugin
$(CARGO) build --locked --release --manifest-path $(call plugin_manifest,$(1)) --target $(rust_target_$(2))
cp plugins/verso-plugin-$(1)/target/$(rust_target_$(2))/release/verso-plugin-$(1) $(BUILDDIR)/verso-plugin-$(1)-$(2)

endef

# `make build` cross-compiles every architecture in ARCHES; each maps to a Go
# GOARCH and the matching Rust musl target triple below. Override to build one:
#   make build ARCHES=arm64      (or just `make build-arm64`)
ARCHES ?= amd64 arm64
rust_target_amd64 := x86_64-unknown-linux-musl
rust_target_arm64 := aarch64-unknown-linux-musl

# The build's version, <X.Y.Z>-r<N>: the latest vX.Y.Z tag and how many commits
# stand on it (scripts/version.sh), so every commit builds its own version, in
# order, and a new version is a new tag. Stamped into the binary at link time
# (below), where the login page and the colophon state it, and into every
# package's version. An un-stamped build (go run, go test) reports "dev".
VER      := $(shell scripts/version.sh)
VERSION_PKG := github.com/we-are-mono/verso/internal/server.Version

# Strip the symbol table (-s) and DWARF debug info (-w): a shipped runtime binary
# needs neither, and dropping them cuts ~25-30% off its size. -X stamps the
# version into the binary without a source edit.
LDFLAGS  := -s -w -X $(VERSION_PKG)=$(VER)

# The two tools fetched as release binaries, Tailwind and golangci-lint, are
# pinned by SHA-256 as well as by version: a release asset can be replaced
# under the same tag and name, and both tools run on the host that signs the
# packages. A download lands beside its target, runs only once its bytes match
# the digest recorded here, and the cached file is checked again before every
# use. A bump updates the version and every digest of that tool together.
# Both are the Linux x86_64 builds; any other build host fails naming that.
TOOLS_HOST_OS   := Linux
TOOLS_HOST_ARCH := x86_64

# Tailwind v4 standalone CLI (no Node); runs on the build host, pinned + cached.
# TAILWIND_SHA256 is tailwindcss-linux-x64 in the release's sha256sums.txt.
TAILWIND         := $(BUILDDIR)/tools/tailwindcss
TAILWIND_VERSION := v4.3.3
TAILWIND_SHA256  := dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a
TAILWIND_URL     := https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64
CSS_IN           := internal/server/assets/input.css
CSS_OUT          := internal/server/assets/verso.css

# golangci-lint, pinned + cached on the build host (a dev tool, not a runtime dep).
# Each version caches in its own directory, so bumping one fetches it afresh.
# GOLANGCI_TARBALL_SHA256 is the tarball's line in the release's
# golangci-lint-<version>-checksums.txt; GOLANGCI_SHA256 is the golangci-lint
# binary inside that tarball, the file the cache keeps.
GOLANGCI_VERSION        := v2.14.0
GOLANGCI                := $(BUILDDIR)/tools/golangci-lint-$(GOLANGCI_VERSION)/golangci-lint
# GOLANGCI_DIST names both the release tarball (plus .tar.gz) and the directory
# inside it that holds the binary.
GOLANGCI_DIST           := golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-amd64
GOLANGCI_TARBALL_SHA256 := ab90aeb7b066f92a33415b638a50fe5344bbb75a0d32ad30cc248d88f81032ab
GOLANGCI_SHA256         := 9f44104281628a17a0dc5aea2429ef51e256d405fd1af38342939578b2e2c83d
GOLANGCI_URL            := https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_VERSION)/$(GOLANGCI_DIST).tar.gz

# require_sha256sum fails before any check when sha256sum is missing, so a
# missing tool never reads as a mismatch that deletes a good file.
require_sha256sum = command -v sha256sum >/dev/null \
	|| { echo "sha256sum is not on PATH; it checks the pinned Tailwind and golangci-lint builds"; exit 1; }

# verify_sha256 checks file $(1) against SHA-256 $(2). On a mismatch it deletes
# the file and fails, so neither a substituted download nor a cached tool
# altered since it was fetched ever runs; the next make fetches it afresh.
# Output is discarded rather than passed --status, which BusyBox's sha256sum
# rejects. The braces keep it one command, so `&&` after it runs only on a match.
verify_sha256 = { $(require_sha256sum); echo "$(2)  $(1)" | sha256sum -c - >/dev/null \
	|| { rm -f "$(1)"; echo "$(1) does not match its recorded SHA-256 $(2); removed it"; exit 1; }; }

# require_tools_host fails a download on a host the pinned builds do not run
# on, or that cannot check them, before anything is fetched.
require_tools_host = test "$$(uname -s)/$$(uname -m)" = $(TOOLS_HOST_OS)/$(TOOLS_HOST_ARCH) \
	|| { echo "the pinned Tailwind and golangci-lint builds are $(TOOLS_HOST_OS)/$(TOOLS_HOST_ARCH); this host is $$(uname -s)/$$(uname -m)"; exit 1; }; \
	$(require_sha256sum)

# tool_tmp opens a fetch recipe line: it binds "$$tmp" to a fresh directory
# beside target $@, so concurrent fetches never share a partial download, and
# removes it when the line's shell exits, on success, failure or interrupt.
tool_tmp = tmp=$$(mktemp -d "$@.XXXXXX") || exit 1; \
	trap 'rm -rf "$$tmp"' EXIT; trap 'exit 1' HUP INT TERM

# Whole-program Go dead-code analysis, also pinned + cached on the build host.
DEADCODE_VERSION := v0.51.0
DEADCODE         := $(BUILDDIR)/tools/deadcode-$(DEADCODE_VERSION)/deadcode

# The vulnerability scanners, pinned + cached like the linters. govulncheck
# checks the Go module and the toolchain's standard library, following calls
# so only reachable advisories count; osv-scanner checks every committed Rust
# lockfile and the vendored scripts' manifest against OSV.dev, which carries
# the RustSec, GitHub and npm advisories. Both install with `go install` at an
# exact version, verified against the Go checksum database (forced on below:
# go reads an empty variable as unset and falls back to the `go env -w` file,
# so GOENV=off drops that file and the empty values then mean the defaults).
GOVULNCHECK_VERSION := v1.8.0
GOVULNCHECK         := $(BUILDDIR)/tools/govulncheck-$(GOVULNCHECK_VERSION)/govulncheck
OSV_SCANNER_VERSION := v2.6.0
OSV_SCANNER         := $(BUILDDIR)/tools/osv-scanner-$(OSV_SCANNER_VERSION)/osv-scanner
VERIFIED_INSTALL    := GOENV=off GOSUMDB=sum.golang.org GONOSUMDB= GOPRIVATE= GOFLAGS=
VENDORED_JS         := internal/server/assets/vendored-js.json
CARGO_LOCKS         := $(foreach m,$(CRATES),$(dir $(m))Cargo.lock)
OSV_LOCKFILES       := $(CARGO_LOCKS) $(VENDORED_JS)
VULN_IGNORE         := vuln-ignore.txt
VULN_DIR            := $(BUILDDIR)/vuln

# ── apk packaging ────────────────────────────────────────────────────────────
# `make apk` cross-builds one arch, assembles a clean payload, and produces a
# signed .apk under build/apk/<apk-arch>/. It is portable: the only machine-specific input
# is the OpenWrt buildroot (OPENWRT_DIR), auto-detected below; the apk tool and
# the signing key both live under it. `make apk-publish` copies the result into
# the local dev repo and rebuilds the signed index — that step alone is tied to
# this dev box (VERSO_REPO_DIR). See building.md for the router-side install.

# The apk tool and signing key both live under the OpenWrt buildroot, so it is
# the one path that varies per machine. Auto-detect it; override in local.mk or
# on the command line (OPENWRT_DIR=/path/...) if the tree lives elsewhere.
OPENWRT_DIR ?= $(firstword $(wildcard $(HOME)/Mono/Gateway/openwrt/source))
APK         ?= $(OPENWRT_DIR)/staging_dir/host/bin/apk
KEY         ?= $(OPENWRT_DIR)/private-key.pem

# apk uses OpenWrt's package-arch names, not Go's GOARCH — map between them.
apk_arch_arm64 := aarch64_generic
apk_arch_amd64 := x86_64

# `make apk` packages one arch per run, arm64 unless APK_GOARCH names another;
# a release runs it once for each arch in ARCHES.
APK_GOARCH ?= arm64
APK_ARCH   := $(apk_arch_$(APK_GOARCH))

# Each arch builds under a directory of its own, laid out as its feed is, so
# packaging one arch never overwrites another's package.
APK_ROOT    := $(BUILDDIR)/apk
APK_DIR     := $(APK_ROOT)/$(APK_ARCH)
APK_PAYLOAD := $(APK_DIR)/pkg
APK_OUT     := $(APK_DIR)/verso-$(VER).apk
POSTINST    := packaging/apk/post-install.sh

# package_plugin lays one core plugin into the payload: its binary, init script
# and manifest, its travelling catalogs (ADR-012), and the rpcd acl.d file a
# plugin with uci scopes ships (ADR-007). A canned recipe, like cargo_plugin.
define package_plugin
install -Dm755 $(BUILDDIR)/verso-plugin-$(1)-$(APK_GOARCH) $(APK_PAYLOAD)/usr/bin/verso-plugin-$(1)
install -Dm755 plugins/verso-plugin-$(1)/rootfs/etc/init.d/verso-plugin-$(1) $(APK_PAYLOAD)/etc/init.d/verso-plugin-$(1)
install -Dm644 plugins/verso-plugin-$(1)/manifest.json $(APK_PAYLOAD)/usr/share/verso/plugins/$(1)/manifest.json
$(foreach f,$(wildcard plugins/verso-plugin-$(1)/i18n/*.json),$(call install_data,$(f),$(APK_PAYLOAD)/usr/share/verso/plugins/$(1)/i18n/$(notdir $(f))))
$(foreach f,$(wildcard plugins/verso-plugin-$(1)/rootfs/usr/share/rpcd/acl.d/*.json),$(call install_data,$(f),$(APK_PAYLOAD)/usr/share/rpcd/acl.d/$(notdir $(f))))

endef

# install_data is one data file's install line, file $(1) to path $(2).
define install_data
install -Dm644 $(1) $(2)

endef

# Local dev apk repo — only `make apk-publish` touches it, so its default lives
# here rather than in the portable build path.
VERSO_REPO_DIR ?= /srv/verso

# index_feed rebuilds the signed v3 index (packages.adb) over every package in
# this arch's dev feed and opens the feed to the web server's reads.
# --allow-untrusted only skips verification on THIS host (it doesn't trust our
# own key); the index is still signed and the router verifies it.
define index_feed
cd $(VERSO_REPO_DIR)/$(APK_ARCH) && "$(APK)" mkndx --allow-untrusted --sign-key "$(KEY)" --output packages.adb *.apk
chmod -R a+rX $(VERSO_REPO_DIR)
endef

# build-<arch> is intentionally NOT phony: make skips pattern rules for phony
# targets, and no file of that name is ever produced, so the rule fires each run.
.PHONY: all build dev css test lint deadcode vuln hooks clean apk apk-publish apk-preflight apk-i18n apk-i18n-publish i18n-audit version

all: lint test build

# version prints what this tree builds as, the string every binary and package
# is stamped with.
version:
	@echo $(VER)

$(TAILWIND):
	@$(require_tools_host)
	@mkdir -p $(dir $@)
	@$(tool_tmp); \
	echo "curl -fsSL $(TAILWIND_URL)"; \
	curl -fsSL $(TAILWIND_URL) -o "$$tmp/tailwindcss" \
	&& $(call verify_sha256,$$tmp/tailwindcss,$(TAILWIND_SHA256)) \
	&& chmod +x "$$tmp/tailwindcss" && mv "$$tmp/tailwindcss" $@

# css compiles the embedded stylesheet. The generated CSS_OUT is committed so a
# bare `go build`/`go test` stays self-contained; regenerate it here on change.
# The cached tool is checked and run in one shell line.
css: $(TAILWIND)
	@$(call verify_sha256,$(TAILWIND),$(TAILWIND_SHA256)) \
	&& $(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --minify

# build cross-compiles verso (Go) and verso-rpcd (Rust) for every arch in ARCHES.
# Outputs are arch-suffixed: build/verso-<arch>, build/verso-rpcd-<arch>. The Rust
# toolchain + musl targets are provisioned by verso-rpcd/rust-toolchain.toml, and
# rust-lld (bundled with rustc) is the cross-linker — no host cross-gcc needed
# (.cargo/config.toml). A fresh checkout builds with only Go, rustup, and make.
build: lint css $(addprefix build-,$(ARCHES))

# build-<arch>: one architecture's runtime binaries. Runnable on its own, e.g.
# `make build-arm64` for that arch alone.
build-%: css
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$* go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARY)-$* $(CMD)
	$(CARGO) build --locked --release --manifest-path $(RPCD_MANIFEST) --target $(rust_target_$*)
	cp verso-rpcd/target/$(rust_target_$*)/release/verso-rpcd $(BUILDDIR)/verso-rpcd-$*
	$(foreach p,$(PLUGINS),$(call cargo_plugin,$(p),$*))

# dev: component-aware hot-reload loop — update only the shell, helper, bundled
# plugin, ACL, or CSS that changed. Starts the container when needed.
dev:
	./scripts/dev.sh

test:
	go test ./...
	set -e; for m in $(CRATES); do $(CARGO) test --locked --manifest-path $$m; done
	APK="$(APK)" node --test scripts/*.test.cjs

# lint replaces plain `go vet` (govet is one of the linters it runs), with
# golangci-lint's default linter set and its gofmt check as .golangci.yml tunes them.
$(GOLANGCI):
	@$(require_tools_host)
	@mkdir -p $(dir $@)
	@$(tool_tmp); \
	echo "curl -fsSL $(GOLANGCI_URL)"; \
	curl -fsSL $(GOLANGCI_URL) -o "$$tmp/dist.tar.gz" \
	&& $(call verify_sha256,$$tmp/dist.tar.gz,$(GOLANGCI_TARBALL_SHA256)) \
	&& tar -xzf "$$tmp/dist.tar.gz" -O $(GOLANGCI_DIST)/golangci-lint > "$$tmp/golangci-lint" \
	&& $(call verify_sha256,$$tmp/golangci-lint,$(GOLANGCI_SHA256)) \
	&& chmod +x "$$tmp/golangci-lint" && mv "$$tmp/golangci-lint" $@

$(DEADCODE):
	@mkdir -p $(dir $@)
	# Built with the project's toolchain: deadcode cannot load code newer than itself.
	$(VERIFIED_INSTALL) GOTOOLCHAIN=$$(go env GOVERSION) GOBIN=$(abspath $(dir $@)) go install golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)

# Analyze the production executables for every architecture we ship. deadcode
# reports findings on stdout without failing, so turn non-empty output into a
# lint failure while preserving genuine tool errors and their exit status.
deadcode: $(DEADCODE)
	@status=0; for arch in $(ARCHES); do \
		output="$$(CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$$arch $(DEADCODE) ./...)"; tool_status=$$?; \
		if [ $$tool_status -ne 0 ]; then printf '%s\n' "$$output"; exit $$tool_status; fi; \
		if [ -n "$$output" ]; then \
			printf 'dead code found for %s/%s:\n%s\n' "$(GOOS)" "$$arch" "$$output"; \
			status=1; \
		fi; \
	done; exit $$status

lint: deadcode $(GOLANGCI)
	@$(call verify_sha256,$(GOLANGCI),$(GOLANGCI_SHA256)) \
	&& $(GOLANGCI) run ./...
	set -e; for m in $(CRATES); do $(CARGO) clippy --locked --manifest-path $$m --all-targets -- -D warnings; done

$(GOVULNCHECK):
	@mkdir -p $(dir $@)
	# Built with the project's toolchain: govulncheck cannot load code newer than itself.
	$(VERIFIED_INSTALL) GOTOOLCHAIN=$$(go env GOVERSION) GOBIN=$(abspath $(dir $@)) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(OSV_SCANNER):
	@mkdir -p $(dir $@)
	$(VERIFIED_INSTALL) GOBIN=$(abspath $(dir $@)) go install github.com/google/osv-scanner/v2/cmd/osv-scanner@$(OSV_SCANNER_VERSION)

# vuln checks what a release ships against the published advisories: the Go
# module and standard library (govulncheck), every Rust crate's lockfile, and
# the vendored scripts (osv-scanner). cmd/vulngate sets both results against
# vuln-ignore.txt and fails on any advisory that is reached (Go) or pinned
# (Rust, scripts) and not triaged there. It needs the network, so it gates
# publishing and CI, not commits. govulncheck scans each release target, as
# the binaries build, since a call path can exist on one architecture alone.
# osv-scanner exits 1 when it finds an advisory; vulngate decides on those, so
# only other exit codes stop here. --all-packages lists every scanned package
# and vulngate checks each requested file is in the result, so a clean result
# still shows each file was read. --config /dev/null keeps an osv-scanner.toml
# beside a lockfile from dropping findings past the dated ignore list.
vuln: $(GOVULNCHECK) $(OSV_SCANNER)
	@mkdir -p $(VULN_DIR)
	set -e; for arch in $(ARCHES); do \
		CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$$arch $(GOVULNCHECK) -format json ./... > $(VULN_DIR)/govulncheck-$$arch.json; \
	done
	$(OSV_SCANNER) scan source --config /dev/null --format json --all-packages --output-file $(VULN_DIR)/osv-scanner.json \
		$(addprefix -L ,$(CARGO_LOCKS)) -L osv-scanner:$(VENDORED_JS) \
		|| [ $$? -eq 1 ]
	go run ./cmd/vulngate $(foreach arch,$(ARCHES),-govulncheck $(VULN_DIR)/govulncheck-$(arch).json) \
		-osv $(VULN_DIR)/osv-scanner.json $(addprefix -lockfile ,$(OSV_LOCKFILES)) -ignore $(VULN_IGNORE)

# hooks points git at the tracked pre-commit hook so commits are gated on lint.
hooks:
	git config core.hooksPath scripts/hooks

clean:
	rm -rf $(BUILDDIR)

# apk-preflight fails early, with an actionable message, when the buildroot
# tools aren't where we expect — clearer than a mid-recipe "command not found".
apk-preflight:
	@test -n "$(OPENWRT_DIR)" || { echo "OPENWRT_DIR is unset and no buildroot was auto-detected. Pass OPENWRT_DIR=/path/to/openwrt/source (or set it in local.mk)."; exit 1; }
	@test -x "$(APK)" || { echo "apk tool not found or not executable at: $(APK). Set OPENWRT_DIR or APK=... ."; exit 1; }
	@test -f "$(KEY)" || { echo "signing key not found at: $(KEY). Set OPENWRT_DIR or KEY=... ."; exit 1; }
	@command -v fakeroot >/dev/null || { echo "fakeroot not found — needed to record root:root ownership in the package without sudo. Install it (e.g. apt-get install fakeroot)."; exit 1; }

# apk builds + signs the package. The payload files must be recorded as root:root
# (ubusd rejects any non-root acl.d file), so the chown + mkpkg run inside a
# single fakeroot session: the package records root ownership while nothing on
# disk is actually root-owned — no sudo, and re-runs can still clean the tree.
apk: apk-preflight build-$(APK_GOARCH)
	rm -rf $(APK_PAYLOAD)
	install -Dm755 $(BUILDDIR)/$(BINARY)-$(APK_GOARCH)                   $(APK_PAYLOAD)/usr/bin/verso
	install -Dm755 $(BUILDDIR)/verso-rpcd-$(APK_GOARCH)                  $(APK_PAYLOAD)/usr/sbin/verso-rpcd
	install -Dm755 docker/rootfs/etc/init.d/verso                      $(APK_PAYLOAD)/etc/init.d/verso
	install -Dm755 docker/rootfs/usr/libexec/verso/firewall-logging $(APK_PAYLOAD)/usr/libexec/verso/firewall-logging
	install -Dm755 docker/rootfs/usr/libexec/verso/firewall-logging-setup $(APK_PAYLOAD)/usr/libexec/verso/firewall-logging-setup
	install -Dm755 docker/rootfs/etc/init.d/verso-rpcd                  $(APK_PAYLOAD)/etc/init.d/verso-rpcd
	install -Dm755 docker/rootfs/usr/libexec/verso/update-check         $(APK_PAYLOAD)/usr/libexec/verso/update-check
	install -Dm755 docker/rootfs/usr/libexec/verso/web-owner            $(APK_PAYLOAD)/usr/libexec/verso/web-owner
	install -Dm755 docker/rootfs/usr/libexec/verso/usage-setup          $(APK_PAYLOAD)/usr/libexec/verso/usage-setup
	install -Dm644 docker/rootfs/etc/hotplug.d/openvpn/50-verso         $(APK_PAYLOAD)/etc/hotplug.d/openvpn/50-verso
	install -Dm644 docker/rootfs/etc/capabilities/verso.json          $(APK_PAYLOAD)/etc/capabilities/verso.json
	install -Dm644 docker/rootfs/etc/config/verso                      $(APK_PAYLOAD)/etc/config/verso
	install -Dm644 docker/rootfs/lib/upgrade/keep.d/verso              $(APK_PAYLOAD)/lib/upgrade/keep.d/verso
	install -Dm644 docker/rootfs/etc/sysctl.d/05-verso-ping.conf       $(APK_PAYLOAD)/etc/sysctl.d/05-verso-ping.conf
	install -Dm644 docker/rootfs/usr/share/acl.d/verso.json            $(APK_PAYLOAD)/usr/share/acl.d/verso.json
	install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-shell.json  $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-shell.json
	install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-helper.json $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-helper.json
	install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-admin.json  $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-admin.json
	$(foreach p,$(PLUGINS),$(call package_plugin,$(p)))
	for theme in themes/*.json; do install -Dm644 "$$theme" "$(APK_PAYLOAD)/usr/share/verso/$$theme"; done
	fakeroot -- sh -c 'chown -R 0:0 "$(APK_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:verso --info version:$(VER) --info arch:$(APK_ARCH) \
	  --info "description:Verso — a modern web UI for OpenWrt" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info "depends:ca-bundle firewall4 kmod-nfnetlink-log nlbwmon" \
	  --info "replaces:verso-plugin-vpn" \
	  --files "$(APK_PAYLOAD)" \
	  --script post-install:$(POSTINST) \
	  --script post-upgrade:$(POSTINST) \
	  --script pre-deinstall:packaging/apk/pre-deinstall.sh \
	  --script post-deinstall:packaging/apk/post-deinstall.sh \
	  --sign-key "$(KEY)" \
	  --output "$(APK_OUT)"'
	@echo "built and signed: $(APK_OUT)  (arch $(APK_ARCH), version $(VER))"

# apk-publish copies the built package into the local dev repo, in place of
# every package there, and rebuilds the index. Dev-box-specific: it writes to
# VERSO_REPO_DIR. Publishing runs `make vuln` first, so no package reaches a
# feed past an untriaged advisory; `make apk` alone builds without it.
apk-publish: vuln apk
	mkdir -p $(VERSO_REPO_DIR)/$(APK_ARCH)
	rm -f $(VERSO_REPO_DIR)/$(APK_ARCH)/*.apk
	cp $(APK_OUT) $(VERSO_REPO_DIR)/$(APK_ARCH)/
	$(index_feed)
	@echo "published to: $(VERSO_REPO_DIR)/$(APK_ARCH)/  (index: packages.adb)"

# ── i18n catalog packaging ────────────────────────────────────────────────────
# The shell's own strings ship one language per data package (ADR-012), the same
# shape LuCI ships (luci-i18n-base-<code>); a plugin's catalogs travel inside the
# plugin's own package. `make apk-i18n` builds + signs a data-only,
# architecture-independent apk of one language of the shell (verso-i18n-base-<code>).
# It drops i18n/<code>/base.json onto disk at /usr/share/verso/i18n/<code>/base.json.
# No post-install script — nothing to restart, the shell rescans on the same trigger
# as a plugin install. Override I18N_CODE (its source must exist at
# i18n/<code>/base.json):
#   make apk-i18n                              # verso-i18n-base-sl
#   make apk-i18n I18N_CODE=de                 # verso-i18n-base-de
I18N_CODE      ?= sl
I18N_SRC       := i18n/$(I18N_CODE)/base.json
I18N_PKG       := verso-i18n-base-$(I18N_CODE)
I18N_PAYLOAD   := $(APK_ROOT)/noarch/i18n-base-$(I18N_CODE)
I18N_OUT       := $(APK_ROOT)/noarch/$(I18N_PKG)-$(VER).apk

# apk-i18n needs no cross-build (data only), only the buildroot's apk + signing key.
apk-i18n: apk-preflight
	@test -f "$(I18N_SRC)" || { echo "catalog $(I18N_SRC) not found — author it, or pass I18N_CODE=<code>."; exit 1; }
	rm -rf $(I18N_PAYLOAD)
	install -Dm644 $(I18N_SRC) $(I18N_PAYLOAD)/usr/share/verso/i18n/$(I18N_CODE)/base.json
	fakeroot -- sh -c 'chown -R 0:0 "$(I18N_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:$(I18N_PKG) --info version:$(VER) --info arch:noarch \
	  --info "description:Verso localization catalog ($(I18N_CODE))" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info "depends:verso" \
	  --files "$(I18N_PAYLOAD)" \
	  --sign-key "$(KEY)" \
	  --output "$(I18N_OUT)"'
	@echo "built and signed: $(I18N_OUT)  (arch noarch, version $(VER))"

# apk-i18n-publish drops the catalog package alongside verso in the per-arch dev
# repo and re-indexes what is present (a noarch package installs on every router's
# arch, so each arch's feed carries it). It does NOT clear the dir, so run it after
# `apk-publish` to keep both the shell and the language in one index.
apk-i18n-publish: apk-i18n
	mkdir -p $(VERSO_REPO_DIR)/$(APK_ARCH)
	cp $(I18N_OUT) $(VERSO_REPO_DIR)/$(APK_ARCH)/
	APK="$(APK)" scripts/check-feed-arch.sh $(VERSO_REPO_DIR)/$(APK_ARCH) $(APK_ARCH)
	$(index_feed)
	@echo "published: $(VERSO_REPO_DIR)/$(APK_ARCH)/$(notdir $(I18N_OUT))  (index rebuilt)"

# i18n-audit renders every page reachable from / and /login in each installed
# language and reports the source strings that fell back to English plus the
# catalog keys no render requested. The translators record their own misses
# (i18n.Bundle.Recorded), so the report is exact for everything the crawl renders.
i18n-audit:
	@VERSO_I18N_AUDIT=1 go test ./internal/server -run TestI18nAudit -count=1 -v
