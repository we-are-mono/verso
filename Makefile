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
SYSTEM_PLUGIN_MANIFEST := plugins/verso-plugin-system/Cargo.toml

# `make build` cross-compiles every architecture in ARCHES; each maps to a Go
# GOARCH and the matching Rust musl target triple below. Override to build one:
#   make build ARCHES=arm64      (or just `make build-arm64`)
ARCHES ?= amd64 arm64
rust_target_amd64 := x86_64-unknown-linux-musl
rust_target_arm64 := aarch64-unknown-linux-musl

# The release string, from the committed VERSION file, stamped into the binary
# at link time (below) and shown on the login page. An un-stamped build (go run,
# make dev) falls back to "dev". Defined here so LDFLAGS can reference it; the
# apk section adds the -r<REVISION> suffix for the package name.
VERSION  := $(file < VERSION)
VERSION_PKG := github.com/we-are-mono/verso/internal/version.Version

# Strip the symbol table (-s) and DWARF debug info (-w): a shipped runtime binary
# needs neither, and dropping them cuts ~25-30% off its size. -X stamps the
# version into the binary without a source edit.
LDFLAGS  := -s -w -X $(VERSION_PKG)=$(VERSION)

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

# VERSION (defined up top, stamped into the binary) + a rebuild revision. Bump
# VERSION in a commit for a new version; override REVISION=2 to repackage the same.
REVISION ?= 1
VER      := $(VERSION)-r$(REVISION)

APK_DIR     := $(BUILDDIR)/apk
APK_PAYLOAD := $(APK_DIR)/pkg
APK_OUT     := $(APK_DIR)/verso-$(VER).apk
POSTINST    := packaging/apk/post-install.sh

# Local dev apk repo — only `make apk-publish` touches it, so its default lives
# here rather than in the portable build path.
VERSO_REPO_DIR ?= /srv/verso

# build-<arch> is intentionally NOT phony: make skips pattern rules for phony
# targets, and no file of that name is ever produced, so the rule fires each run.
.PHONY: all build run dev css test lint deadcode hooks tidy rpcd clean apk apk-publish apk-preflight apk-i18n apk-i18n-publish i18n-pot

all: lint test build

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
	$(CARGO) build --locked --release --manifest-path $(SYSTEM_PLUGIN_MANIFEST) --target $(rust_target_$*)
	cp plugins/verso-plugin-system/target/$(rust_target_$*)/release/verso-plugin-system $(BUILDDIR)/verso-plugin-system-$*

run:
	go run $(CMD)

# dev: hot-reload loop — rebuild + swap the binary into the running container on
# every source change. Requires the container up (it will start one if needed).
dev:
	./scripts/dev.sh

test:
	go test ./...
	$(CARGO) test --locked --manifest-path $(RPCD_MANIFEST)
	$(CARGO) test --locked --manifest-path $(SYSTEM_PLUGIN_MANIFEST)

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
	$(GOLANGCI) run ./...
	$(CARGO) clippy --locked --manifest-path $(RPCD_MANIFEST) --all-targets -- -D warnings
	$(CARGO) clippy --locked --manifest-path $(SYSTEM_PLUGIN_MANIFEST) --all-targets -- -D warnings

# hooks points git at the tracked pre-commit hook so commits are gated on lint.
hooks:
	git config core.hooksPath scripts/hooks

tidy:
	go mod tidy

# Native helper build for the Docker/dev loop (host arch, no cross-target).
rpcd:
	$(CARGO) build --locked --release --manifest-path $(RPCD_MANIFEST)

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
	install -Dm755 docker/rootfs/etc/init.d/verso                       $(APK_PAYLOAD)/etc/init.d/verso
	install -Dm755 docker/rootfs/etc/init.d/verso-rpcd                  $(APK_PAYLOAD)/etc/init.d/verso-rpcd
	install -Dm755 plugins/verso-plugin-system/rootfs/etc/init.d/verso-plugin-system $(APK_PAYLOAD)/etc/init.d/verso-plugin-system
	install -Dm644 plugins/verso-plugin-system/manifest.json             $(APK_PAYLOAD)/usr/share/verso/plugins/system/manifest.json
	install -Dm644 docker/rootfs/etc/capabilities/verso.json           $(APK_PAYLOAD)/etc/capabilities/verso.json
	install -Dm644 docker/rootfs/usr/share/acl.d/verso.json            $(APK_PAYLOAD)/usr/share/acl.d/verso.json
	install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-shell.json  $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-shell.json
	install -Dm644 docker/rootfs/usr/share/rpcd/acl.d/verso-helper.json $(APK_PAYLOAD)/usr/share/rpcd/acl.d/verso-helper.json
	fakeroot -- sh -c 'chown -R 0:0 "$(APK_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:verso --info version:$(VER) --info arch:$(APK_ARCH) \
	  --info "description:Verso — a modern web UI for OpenWrt" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info depends:ca-bundle \
	  --files "$(APK_PAYLOAD)" \
	  --script post-install:$(POSTINST) \
	  --script post-upgrade:$(POSTINST) \
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

# ── i18n catalog packaging ────────────────────────────────────────────────────
# A language is a data package, discovered on disk at runtime (ADR-012): the same
# shape LuCI ships (luci-i18n-base-<code>). `make apk-i18n` builds + signs a
# data-only, architecture-independent apk (verso-i18n-base-<code>) that drops one
# <code>.json catalog into /usr/share/verso/i18n/. No post-install script — nothing
# to restart, the shell rescans on the same trigger as a plugin install. Override
# I18N_CODE to package another language (its <code>.json must exist at the root).
I18N_CODE ?= sl
I18N_SRC     := $(I18N_CODE).json
I18N_PKG     := verso-i18n-base-$(I18N_CODE)
I18N_PAYLOAD := $(APK_DIR)/i18n-$(I18N_CODE)
I18N_OUT     := $(APK_DIR)/$(I18N_PKG)-$(VER).apk

# apk-i18n needs no cross-build (data only), only the buildroot's apk + signing key.
apk-i18n: apk-preflight
	@test -f "$(I18N_SRC)" || { echo "catalog $(I18N_SRC) not found — author it at the repo root, or pass I18N_CODE=<code>."; exit 1; }
	rm -rf $(I18N_PAYLOAD)
	install -Dm644 $(I18N_SRC) $(I18N_PAYLOAD)/usr/share/verso/i18n/$(I18N_SRC)
	fakeroot -- sh -c 'chown -R 0:0 "$(I18N_PAYLOAD)" && "$(APK)" mkpkg \
	  --info name:$(I18N_PKG) --info version:$(VER) --info arch:all \
	  --info "description:Verso localization catalog ($(I18N_CODE))" \
	  --info license:GPL-2.0-only --info url:https://github.com/we-are-mono/verso \
	  --info origin:verso \
	  --info "depends:verso" \
	  --files "$(I18N_PAYLOAD)" \
	  --sign-key "$(KEY)" \
	  --output "$(I18N_OUT)"'
	@echo "built and signed: $(I18N_OUT)  (arch all, version $(VER))"

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

# i18n-pot emits the source strings wrapped by the explicit localization seams —
# every {{ t "…" }} in a template and every t("…")/tr("…") STRING LITERAL in Go —
# sorted and de-duplicated, so a catalog can be diffed against those (ADR-012).
#
# It is a partial extractor, not the authoritative source-string set. Two large
# classes of translated strings are NOT listed, because they are not literal
# arguments to a t()/{{ t }} call:
#   1. Strings the render walk (translateSchema) translates in place — authored
#      widget struct fields (Section titles/subs, Callout bodies, Form submits,
#      table column labels, …). These are the bulk of a page's prose.
#   2. Strings translated centrally on a VARIABLE — renderPage's tr(hdr.Subheading)
#      / tr(flashMessage) / tr(p.Label), buildSidebar's tr(sec.Title), renderLogin's
#      tr(errMsg), services' tr(lifecycleWord(...)) — where the English lives in a
#      struct field or a bare argument elsewhere, not inside the tr(...) call.
# Author the catalog from the code and the shipped sl.json, using this only to spot
# drift in the explicit-seam subset.
i18n-pot:
	@{ grep -rhoE '\{\{[ ]*t "([^"]+)"' internal --include='*.tmpl' | sed -E 's/^\{\{[ ]*t "//; s/"$$//'; \
	   grep -rhoE '\btr?\("([^"]+)"' internal cmd --include='*.go' | grep -v '_test.go' | sed -E 's/^\btr?\("//; s/"$$//'; \
	 } | sort -u
