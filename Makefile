# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

# Verso shell — build/test tooling.
# Prototype build target is arm64 (project memo); override GOARCH to change.

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
# The bundled plugins; each is plugins/verso-plugin-<name>, a crate of its own.
PLUGINS := interfaces system firewall dnsdhcp qos
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
VERSION_PKG := github.com/we-are-mono/verso/internal/version.Version

# Strip the symbol table (-s) and DWARF debug info (-w): a shipped runtime binary
# needs neither, and dropping them cuts ~25-30% off its size. -X stamps the
# version into the binary without a source edit.
LDFLAGS  := -s -w -X $(VERSION_PKG)=$(VER)

# Tailwind v4 standalone CLI (no Node); runs on the build host, pinned + cached.
TAILWIND         := $(BUILDDIR)/tools/tailwindcss
TAILWIND_VERSION := v4.3.3
CSS_IN           := internal/server/assets/input.css
CSS_OUT          := internal/server/assets/verso.css

# golangci-lint, pinned + cached on the build host (a dev tool, not a runtime dep).
GOLANGCI         := $(BUILDDIR)/tools/golangci-lint
GOLANGCI_VERSION := v1.64.8

# Whole-program Go dead-code analysis, also pinned + cached on the build host.
DEADCODE         := $(BUILDDIR)/tools/deadcode
DEADCODE_VERSION := v0.39.0

# ── apk packaging ────────────────────────────────────────────────────────────
# `make apk` cross-builds one arch, assembles a clean payload, and produces a
# signed .apk under build/apk/. It is portable: the only machine-specific input
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

# `make apk` ships one arch (the router is arm64). Override to package another.
APK_GOARCH ?= arm64
APK_ARCH   := $(apk_arch_$(APK_GOARCH))

APK_DIR     := $(BUILDDIR)/apk
APK_PAYLOAD := $(APK_DIR)/pkg
APK_OUT     := $(APK_DIR)/verso-$(VER).apk
POSTINST    := packaging/apk/post-install.sh

# Local dev apk repo — only `make apk-publish` touches it, so its default lives
# here rather than in the portable build path.
VERSO_REPO_DIR ?= /srv/verso

# build-<arch> is intentionally NOT phony: make skips pattern rules for phony
# targets, and no file of that name is ever produced, so the rule fires each run.
.PHONY: all build dev css test lint deadcode hooks clean apk apk-publish apk-preflight apk-dnsdhcp apk-dnsdhcp-publish apk-qos apk-qos-publish apk-i18n apk-i18n-publish i18n-audit version

all: lint test build

# version prints what this tree builds as, the string every binary and package
# is stamped with.
version:
	@echo $(VER)

$(TAILWIND):
	@mkdir -p $(dir $@)
	curl -fsSL https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64 -o $@
	@chmod +x $@

# css compiles the embedded stylesheet. The generated CSS_OUT is committed so a
# bare `go build`/`go test` stays self-contained; regenerate it here on change.
css: $(TAILWIND)
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --minify

# build cross-compiles verso (Go) and verso-rpcd (Rust) for every arch in ARCHES.
# Outputs are arch-suffixed: build/verso-<arch>, build/verso-rpcd-<arch>. The Rust
# toolchain + musl targets are provisioned by verso-rpcd/rust-toolchain.toml, and
# rust-lld (bundled with rustc) is the cross-linker — no host cross-gcc needed
# (.cargo/config.toml). A fresh checkout builds with only Go, rustup, and make.
build: lint css $(addprefix build-,$(ARCHES))

# build-<arch>: one architecture's runtime binaries. Runnable on its own, e.g.
# `make build-arm64` for just the device target.
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
	node --test scripts/*.test.cjs

# lint replaces plain `go vet` (govet is one of the linters it runs). Sensible
# defaults: no custom config, golangci-lint's default linter set.
$(GOLANGCI):
	@mkdir -p $(dir $@)
	curl -fsSL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_VERSION)/golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-amd64.tar.gz \
		| tar -xz -C $(dir $@) --strip-components=1 golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-amd64/golangci-lint
	@chmod +x $@

$(DEADCODE):
	@mkdir -p $(dir $@)
	GOBIN=$(abspath $(dir $@)) go install golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)

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
	@fmt_drift=$$(gofmt -l cmd internal); if [ -n "$$fmt_drift" ]; then echo "gofmt drift:"; echo "$$fmt_drift"; exit 1; fi
	$(GOLANGCI) run ./...
	set -e; for m in $(CRATES); do $(CARGO) clippy --locked --manifest-path $$m --all-targets -- -D warnings; done

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
	install -Dm755 $(BUILDDIR)/verso-plugin-system-$(APK_GOARCH)         $(APK_PAYLOAD)/usr/bin/verso-plugin-system
	install -Dm755 $(BUILDDIR)/verso-plugin-firewall-$(APK_GOARCH)       $(APK_PAYLOAD)/usr/bin/verso-plugin-firewall
	install -Dm755 docker/rootfs/etc/init.d/verso                       $(APK_PAYLOAD)/etc/init.d/verso
	install -Dm755 docker/rootfs/usr/libexec/verso/firewall-logging $(APK_PAYLOAD)/usr/libexec/verso/firewall-logging
	install -Dm755 docker/rootfs/usr/libexec/verso/firewall-logging-setup $(APK_PAYLOAD)/usr/libexec/verso/firewall-logging-setup
	install -Dm755 docker/rootfs/etc/init.d/verso-rpcd                  $(APK_PAYLOAD)/etc/init.d/verso-rpcd
	install -Dm755 docker/rootfs/usr/libexec/verso/update-check         $(APK_PAYLOAD)/usr/libexec/verso/update-check
	install -Dm755 plugins/verso-plugin-system/rootfs/etc/init.d/verso-plugin-system $(APK_PAYLOAD)/etc/init.d/verso-plugin-system
	install -Dm755 plugins/verso-plugin-firewall/rootfs/etc/init.d/verso-plugin-firewall $(APK_PAYLOAD)/etc/init.d/verso-plugin-firewall
	install -Dm644 plugins/verso-plugin-system/manifest.json             $(APK_PAYLOAD)/usr/share/verso/plugins/system/manifest.json
	install -Dm644 plugins/verso-plugin-firewall/manifest.json           $(APK_PAYLOAD)/usr/share/verso/plugins/firewall/manifest.json
	install -Dm644 plugins/verso-plugin-firewall/i18n/sl.json $(APK_PAYLOAD)/usr/share/verso/plugins/firewall/i18n/sl.json
	install -Dm644 docker/rootfs/etc/capabilities/verso.json           $(APK_PAYLOAD)/etc/capabilities/verso.json
	install -Dm644 docker/rootfs/etc/config/verso                      $(APK_PAYLOAD)/etc/config/verso
	install -Dm644 docker/rootfs/usr/share/acl.d/verso.json            $(APK_PAYLOAD)/usr/share/acl.d/verso.json
	install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-shell.json  $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-shell.json
	install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-helper.json $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-helper.json
	install -Dm755 $(BUILDDIR)/verso-plugin-interfaces-$(APK_GOARCH) $(APK_PAYLOAD)/usr/bin/verso-plugin-interfaces
	install -Dm755 plugins/verso-plugin-interfaces/rootfs/etc/init.d/verso-plugin-interfaces $(APK_PAYLOAD)/etc/init.d/verso-plugin-interfaces
	install -Dm644 plugins/verso-plugin-interfaces/manifest.json $(APK_PAYLOAD)/usr/share/verso/plugins/interfaces/manifest.json
	install -Dm644 plugins/verso-plugin-interfaces/rootfs/usr/share/rpcd/acl.d/verso-plugin-interfaces.json $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-plugin-interfaces.json
	install -Dm644 plugins/verso-plugin-system/rootfs/usr/share/rpcd/acl.d/verso-plugin-system.json $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-plugin-system.json
	install -Dm644 plugins/verso-plugin-interfaces/i18n/sl.json $(APK_PAYLOAD)/usr/share/verso/plugins/interfaces/i18n/sl.json
	install -Dm644 plugins/verso-plugin-system/i18n/sl.json $(APK_PAYLOAD)/usr/share/verso/plugins/system/i18n/sl.json
	fakeroot -- sh -c 'chown -R 0:0 "$(APK_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:verso --info version:$(VER) --info arch:$(APK_ARCH) \
	  --info "description:Verso — a modern web UI for OpenWrt" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info "depends:ca-bundle firewall4 kmod-nfnetlink-log" \
	  --files "$(APK_PAYLOAD)" \
	  --script post-install:$(POSTINST) \
	  --script post-upgrade:$(POSTINST) \
	  --script pre-deinstall:packaging/apk/pre-deinstall.sh \
	  --sign-key "$(KEY)" \
	  --output "$(APK_OUT)"'
	@echo "built and signed: $(APK_OUT)  (arch $(APK_ARCH), version $(VER))"

# apk-publish copies the built package into the local dev repo and rebuilds the
# signed v3 index (packages.adb). Dev-box-specific: it writes to VERSO_REPO_DIR.
# --allow-untrusted only skips verification on THIS host (it doesn't trust our
# own key); the index is still signed and the router verifies it.
apk-publish: apk
	mkdir -p $(VERSO_REPO_DIR)/$(APK_ARCH)
	rm -f $(VERSO_REPO_DIR)/$(APK_ARCH)/*.apk
	cp $(APK_OUT) $(VERSO_REPO_DIR)/$(APK_ARCH)/
	cd $(VERSO_REPO_DIR)/$(APK_ARCH) && "$(APK)" mkndx --allow-untrusted --sign-key "$(KEY)" --output packages.adb *.apk
	chmod -R a+rX $(VERSO_REPO_DIR)
	@echo "published to: $(VERSO_REPO_DIR)/$(APK_ARCH)/  (index: packages.adb)"

# ── DNS/DHCP plugin packaging ─────────────────────────────────────────────────
# The DNS/DHCP plugin ships as its own package rather than inside the verso
# payload: it is only useful where dnsmasq serves the network, and apk is what
# states that — `depends:dnsmasq` alongside the shell it plugs into. Its layout
# is the shell package's, narrowed to one plugin: the binary, the init script,
# and the manifest the shell discovers on disk.
DNSDHCP_PKG     := verso-plugin-dnsdhcp
DNSDHCP_PAYLOAD := $(APK_DIR)/pkg-dnsdhcp
DNSDHCP_OUT     := $(APK_DIR)/$(DNSDHCP_PKG)-$(VER).apk
DNSDHCP_POSTINST := packaging/apk/post-install-dnsdhcp.sh

apk-dnsdhcp: apk-preflight build-$(APK_GOARCH)
	rm -rf $(DNSDHCP_PAYLOAD)
	install -Dm755 $(BUILDDIR)/verso-plugin-dnsdhcp-$(APK_GOARCH)                       $(DNSDHCP_PAYLOAD)/usr/bin/verso-plugin-dnsdhcp
	install -Dm755 plugins/verso-plugin-dnsdhcp/rootfs/etc/init.d/verso-plugin-dnsdhcp  $(DNSDHCP_PAYLOAD)/etc/init.d/verso-plugin-dnsdhcp
	install -Dm644 plugins/verso-plugin-dnsdhcp/rootfs/usr/share/rpcd/acl.d/verso-plugin-dnsdhcp.json $(DNSDHCP_PAYLOAD)/usr/share/rpcd/acl.d/verso-plugin-dnsdhcp.json
	install -Dm644 plugins/verso-plugin-dnsdhcp/i18n/sl.json $(DNSDHCP_PAYLOAD)/usr/share/verso/plugins/dnsdhcp/i18n/sl.json
	install -Dm644 plugins/verso-plugin-dnsdhcp/manifest.json                           $(DNSDHCP_PAYLOAD)/usr/share/verso/plugins/dnsdhcp/manifest.json
	fakeroot -- sh -c 'chown -R 0:0 "$(DNSDHCP_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:$(DNSDHCP_PKG) --info version:$(VER) --info arch:$(APK_ARCH) \
	  --info "description:Verso DNS and DHCP — dnsmasq and odhcpd, as pages" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info "depends:verso dnsmasq" \
	  --files "$(DNSDHCP_PAYLOAD)" \
	  --script post-install:$(DNSDHCP_POSTINST) \
	  --script post-upgrade:$(DNSDHCP_POSTINST) \
	  --sign-key "$(KEY)" \
	  --output "$(DNSDHCP_OUT)"'
	@echo "built and signed: $(DNSDHCP_OUT)  (arch $(APK_ARCH), version $(VER))"

# apk-dnsdhcp-publish drops the plugin package beside verso in the per-arch dev
# repo and re-indexes what is present. Like the catalog publisher, it does not
# clear the dir — run it after `apk-publish` to keep both in one index.
apk-dnsdhcp-publish: apk-dnsdhcp
	mkdir -p $(VERSO_REPO_DIR)/$(APK_ARCH)
	cp $(DNSDHCP_OUT) $(VERSO_REPO_DIR)/$(APK_ARCH)/
	cd $(VERSO_REPO_DIR)/$(APK_ARCH) && "$(APK)" mkndx --allow-untrusted --sign-key "$(KEY)" --output packages.adb *.apk
	chmod -R a+rX $(VERSO_REPO_DIR)
	@echo "published: $(VERSO_REPO_DIR)/$(APK_ARCH)/$(notdir $(DNSDHCP_OUT))  (index rebuilt)"

# The device-limits plugin ships as its own package: it is only useful where fw4
# enforces the rules it writes, and apk is what states that — `depends:firewall4`
# alongside the shell it plugs into. Besides the usual three files it ships the
# rpcd acl.d grant that makes its declared uci scopes grantable (ADR-007), and a
# default /etc/config/qos, because uci will not write into a file that is absent.
QOS_PKG      := verso-plugin-qos
QOS_PAYLOAD  := $(APK_DIR)/pkg-qos
QOS_OUT      := $(APK_DIR)/$(QOS_PKG)-$(VER).apk
QOS_POSTINST := packaging/apk/post-install-qos.sh

apk-qos: apk-preflight build-$(APK_GOARCH)
	rm -rf $(QOS_PAYLOAD)
	install -Dm755 $(BUILDDIR)/verso-plugin-qos-$(APK_GOARCH)                                      $(QOS_PAYLOAD)/usr/bin/verso-plugin-qos
	install -Dm755 plugins/verso-plugin-qos/rootfs/etc/init.d/verso-plugin-qos                     $(QOS_PAYLOAD)/etc/init.d/verso-plugin-qos
	install -Dm644 plugins/verso-plugin-qos/rootfs/etc/config/qos                                  $(QOS_PAYLOAD)/etc/config/qos
	install -Dm644 plugins/verso-plugin-qos/rootfs/usr/share/rpcd/acl.d/verso-plugin-qos.json      $(QOS_PAYLOAD)/usr/share/rpcd/acl.d/verso-plugin-qos.json
	install -Dm644 plugins/verso-plugin-qos/manifest.json                                          $(QOS_PAYLOAD)/usr/share/verso/plugins/qos/manifest.json
	fakeroot -- sh -c 'chown -R 0:0 "$(QOS_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:$(QOS_PKG) --info version:$(VER) --info arch:$(APK_ARCH) \
	  --info "description:Verso device limits — what a device may reach, when, and how fast" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info "depends:verso firewall4" \
	  --files "$(QOS_PAYLOAD)" \
	  --script post-install:$(QOS_POSTINST) \
	  --script post-upgrade:$(QOS_POSTINST) \
	  --sign-key "$(KEY)" \
	  --output "$(QOS_OUT)"'
	@echo "built and signed: $(QOS_OUT)  (arch $(APK_ARCH), version $(VER))"

# apk-qos-publish drops the plugin package beside verso in the per-arch dev repo
# and re-indexes what is present, like the DNS/DHCP publisher above.
apk-qos-publish: apk-qos
	mkdir -p $(VERSO_REPO_DIR)/$(APK_ARCH)
	cp $(QOS_OUT) $(VERSO_REPO_DIR)/$(APK_ARCH)/
	cd $(VERSO_REPO_DIR)/$(APK_ARCH) && "$(APK)" mkndx --allow-untrusted --sign-key "$(KEY)" --output packages.adb *.apk
	chmod -R a+rX $(VERSO_REPO_DIR)
	@echo "published: $(VERSO_REPO_DIR)/$(APK_ARCH)/$(notdir $(QOS_OUT))  (index rebuilt)"

# ── i18n catalog packaging ────────────────────────────────────────────────────
# A catalog is a per-component data package, discovered on disk at runtime
# (ADR-012), the same shape LuCI ships (luci-i18n-<app>-<code>). `make apk-i18n`
# builds + signs a data-only, architecture-independent apk for one component of one
# language: the shell (verso-i18n-base-<code>) or a plugin (verso-i18n-<plugin>-<code>).
# It drops i18n/<code>/<component>.json onto disk at
# /usr/share/verso/i18n/<code>/<component>.json. No post-install script — nothing to
# restart, the shell rescans on the same trigger as a plugin install. Override
# I18N_CODE and I18N_COMPONENT (its source must exist at i18n/<code>/<component>.json):
#   make apk-i18n                              # verso-i18n-base-sl
#   make apk-i18n I18N_COMPONENT=system        # verso-i18n-system-sl
I18N_CODE      ?= sl
I18N_COMPONENT ?= base
I18N_SRC       := i18n/$(I18N_CODE)/$(I18N_COMPONENT).json
I18N_PKG       := verso-i18n-$(I18N_COMPONENT)-$(I18N_CODE)
I18N_PAYLOAD   := $(APK_DIR)/i18n-$(I18N_COMPONENT)-$(I18N_CODE)
I18N_OUT       := $(APK_DIR)/$(I18N_PKG)-$(VER).apk
# The shell's catalog depends on verso; a plugin's depends on that plugin's package,
# so a plugin catalog is meaningless without the plugin it translates.
I18N_DEPENDS   := $(if $(filter base,$(I18N_COMPONENT)),verso,verso-plugin-$(I18N_COMPONENT))

# apk-i18n needs no cross-build (data only), only the buildroot's apk + signing key.
apk-i18n: apk-preflight
	@test -f "$(I18N_SRC)" || { echo "catalog $(I18N_SRC) not found — author it, or pass I18N_CODE=<code> I18N_COMPONENT=<base|plugin-id>."; exit 1; }
	rm -rf $(I18N_PAYLOAD)
	install -Dm644 $(I18N_SRC) $(I18N_PAYLOAD)/usr/share/verso/i18n/$(I18N_CODE)/$(I18N_COMPONENT).json
	fakeroot -- sh -c 'chown -R 0:0 "$(I18N_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:$(I18N_PKG) --info version:$(VER) --info arch:noarch \
	  --info "description:Verso localization catalog ($(I18N_COMPONENT), $(I18N_CODE))" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info "depends:$(I18N_DEPENDS)" \
	  --files "$(I18N_PAYLOAD)" \
	  --sign-key "$(KEY)" \
	  --output "$(I18N_OUT)"'
	@echo "built and signed: $(I18N_OUT)  (arch noarch, version $(VER))"

# apk-i18n-publish drops the catalog package alongside verso in the per-arch dev
# repo and re-indexes what is present (an arch:all package installs on the router's
# arch just the same). It does NOT clear the dir, so run it after `apk-publish` to
# keep both the shell and the language in one index.
apk-i18n-publish: apk-i18n
	mkdir -p $(VERSO_REPO_DIR)/$(APK_ARCH)
	cp $(I18N_OUT) $(VERSO_REPO_DIR)/$(APK_ARCH)/
	cd $(VERSO_REPO_DIR)/$(APK_ARCH) && "$(APK)" mkndx --allow-untrusted --sign-key "$(KEY)" --output packages.adb *.apk
	chmod -R a+rX $(VERSO_REPO_DIR)
	@echo "published: $(VERSO_REPO_DIR)/$(APK_ARCH)/$(notdir $(I18N_OUT))  (index rebuilt)"

# i18n-audit renders every page reachable from / and /login in each installed
# language and reports the source strings that fell back to English plus the
# catalog keys no render requested. The translators record their own misses
# (i18n.Bundle.Recorded), so the report is exact for everything the crawl renders.
i18n-audit:
	@VERSO_I18N_AUDIT=1 go test ./internal/server -run TestI18nAudit -count=1 -v
