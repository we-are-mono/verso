#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# Dev loop: watch the shell, helper, and bundled plugins; rebuild and hot-swap
# them into the OpenWrt container; restart their procd services; and redeploy the
# ubusd + rpcd ACLs. No image rebuild or OpenWrt reboot is needed.
set -euo pipefail

cd "$(dirname "$0")/.."

CONTAINER=verso
CMD=./cmd/verso
BIN=build/verso
CSS_IN=internal/server/assets/input.css
CSS_OUT=internal/server/assets/verso.css
DEV_CSS=/usr/share/verso/verso-dev.css # in-container drop file the shell reads live (dev only; not /tmp — that's a tmpfs docker cp can't write)
ACL_SRC=docker/rootfs/usr/share/acl.d
RPCD_ACL_SRC=docker/rootfs/usr/share/rpcd/acl.d
URL="http://localhost:8080"
BUNDLED_PLUGIN_GLOB=plugins/verso-plugin-*/bundled

log() { printf '\033[36m[dev]\033[0m %s\n' "$*"; }

# deploy_acls copies the shell's ubusd ACLs into the running container so a grant
# edit never goes stale against the hot-swapped binary (e.g. luci.setPassword for
# the password page). Two gotchas are handled: ubusd skips any acl.d file that is
# not root-owned or is group/world-writable (docker cp lands it as the host uid),
# and ubusd only re-reads ACLs on SIGHUP — never on its own. So normalize
# ownership/mode and HUP ubusd; no container restart needed.
deploy_acls() {
	local f names=()
	for f in "$ACL_SRC"/*.json; do
		[ -e "$f" ] || continue
		docker cp "$f" "$CONTAINER":/usr/share/acl.d/"$(basename "$f")"
		names+=("/usr/share/acl.d/$(basename "$f")")
	done
	[ ${#names[@]} -gt 0 ] || return 0
	docker exec "$CONTAINER" sh -c \
		"chown root:root ${names[*]}; chmod 0644 ${names[*]}; kill -HUP \$(pidof ubusd) 2>/dev/null || true"
}

# Keep the shell's procd definition in sync too. Runtime environment changes
# (such as its private multipart scratch directory) are part of the service,
# not the Go binary, and must be exercised by the dev container before release.
deploy_shell_init() {
	docker cp docker/rootfs/etc/init.d/verso "$CONTAINER":/etc/init.d/.verso.new
	docker exec "$CONTAINER" sh -c 'chown root:root /etc/init.d/.verso.new; chmod 0755 /etc/init.d/.verso.new; mv /etc/init.d/.verso.new /etc/init.d/verso'
}

# deploy_helper builds and atomically replaces the persistent Rust companion.
# Its rpcd ACL remains the source of session.access policy, so ACL edits still
# reload rpcd without discarding live login sessions.
deploy_helper() {
	local cargo_bin="${CARGO:-$HOME/.cargo/bin/cargo}"
	# Build from the crate directory so rustup honors its pinned toolchain file;
	# the host's distro cargo otherwise emits a glibc binary OpenWrt cannot run.
	if (cd verso-rpcd && "$cargo_bin" build --locked --release --target x86_64-unknown-linux-musl) 2>&1; then
		docker exec "$CONTAINER" /etc/init.d/verso-rpcd stop >/dev/null 2>&1 || true
		docker cp verso-rpcd/target/x86_64-unknown-linux-musl/release/verso-rpcd "$CONTAINER":/usr/sbin/.verso-rpcd.new
		docker cp docker/rootfs/etc/init.d/verso-rpcd "$CONTAINER":/etc/init.d/.verso-rpcd.new
		docker exec "$CONTAINER" sh -c 'chown root:root /usr/sbin/.verso-rpcd.new /etc/init.d/.verso-rpcd.new; chmod 0755 /usr/sbin/.verso-rpcd.new /etc/init.d/.verso-rpcd.new; mv /usr/sbin/.verso-rpcd.new /usr/sbin/verso-rpcd; mv /etc/init.d/.verso-rpcd.new /etc/init.d/verso-rpcd; /etc/init.d/verso-rpcd enable; /etc/init.d/verso-rpcd start'
	else
		log "verso-rpcd build failed — keeping the running helper"
	fi
	local f names=()
	for f in "$RPCD_ACL_SRC"/*.json; do
		[ -e "$f" ] || continue
		docker cp "$f" "$CONTAINER":/usr/share/rpcd/acl.d/"$(basename "$f")"
		names+=("/usr/share/rpcd/acl.d/$(basename "$f")")
	done
	[ ${#names[@]} -gt 0 ] && docker exec "$CONTAINER" sh -c "chown root:root ${names[*]}; chmod 0644 ${names[*]}"
	docker exec "$CONTAINER" /etc/init.d/rpcd reload >/dev/null 2>&1 || true
}

# deploy_bundled_plugins discovers plugins by a checked-in `bundled` marker.
# Adding another bundled Rust plugin therefore makes `make dev` install and
# activate it without teaching this script its name. Independent/reference
# plugins have no marker and remain under their own deploy flow.
deploy_bundled_plugins() {
	local marker dir name id cargo_bin="${CARGO:-$HOME/.cargo/bin/cargo}"
	for marker in $BUNDLED_PLUGIN_GLOB; do
		[ -e "$marker" ] || continue
		dir="$(dirname "$marker")"
		name="$(basename "$dir")"
		id="${name#verso-plugin-}"
		if (cd "$dir" && "$cargo_bin" build --locked --release --target x86_64-unknown-linux-musl) 2>&1; then
			docker exec "$CONTAINER" sh -c "mkdir -p /usr/share/verso/plugins/$id; /etc/init.d/$name stop >/dev/null 2>&1 || true"
			docker cp "$dir/target/x86_64-unknown-linux-musl/release/$name" "$CONTAINER:/usr/bin/.$name.new"
			docker cp "$dir/rootfs/etc/init.d/$name" "$CONTAINER:/etc/init.d/.$name.new"
			docker cp "$dir/manifest.json" "$CONTAINER:/usr/share/verso/plugins/$id/.manifest.json.new"
			docker exec "$CONTAINER" sh -c "chown root:root /usr/bin/.$name.new /etc/init.d/.$name.new /usr/share/verso/plugins/$id/.manifest.json.new; chmod 0755 /usr/bin/.$name.new /etc/init.d/.$name.new; chmod 0644 /usr/share/verso/plugins/$id/.manifest.json.new; mv /usr/bin/.$name.new /usr/bin/$name; mv /etc/init.d/.$name.new /etc/init.d/$name; mv /usr/share/verso/plugins/$id/.manifest.json.new /usr/share/verso/plugins/$id/manifest.json; /etc/init.d/$name enable; /etc/init.d/$name start"
		else
			log "$name build failed — keeping the running plugin"
		fi
	done
}

ensure_container() {
	if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null)" != "true" ]; then
		log "container not running — starting it (docker compose up -d --build)"
		docker compose up -d --build
	fi
}

# compile_css regenerates the embedded stylesheet from input.css (Tailwind, no Node).
compile_css() {
	[ -x build/tools/tailwindcss ] || return 0
	build/tools/tailwindcss -i "$CSS_IN" -o "$CSS_OUT" --minify >/dev/null 2>&1 || true
}

# push_css drops the freshly-compiled stylesheet into the container at $DEV_CSS, where
# the shell reads it live (no rebuild). The copy is staged and atomically renamed so a
# render never reads a half-written file. This is what makes a CSS edit a ~1s hot-swap.
push_css() {
	[ -f "$CSS_OUT" ] || return 0
	docker cp "$CSS_OUT" "$CONTAINER":"$DEV_CSS.tmp" >/dev/null 2>&1 || return 0
	docker exec "$CONTAINER" sh -c "chmod 0644 '$DEV_CSS.tmp' && mv '$DEV_CSS.tmp' '$DEV_CSS'" >/dev/null 2>&1 || true
}

reload() {
	# Privileged surface deploys first, unconditionally: an ACL or helper edit
	# should land even when the shell build below fails and we keep the old binary.
	deploy_acls
	deploy_shell_init
	deploy_helper
	deploy_bundled_plugins
	log "building…"
	compile_css
	if ! CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$BIN" "$CMD" 2>&1; then
		log "build failed — keeping the running binary"
		return 0
	fi
	docker exec "$CONTAINER" /etc/init.d/verso stop >/dev/null 2>&1 || true
	docker cp "$BIN" "$CONTAINER":/usr/bin/verso
	push_css # land the live stylesheet before start, so the shell boots in CSS hot-reload mode
	docker exec "$CONTAINER" /etc/init.d/verso start >/dev/null 2>&1 || true
	log "reloaded → $URL"
}

# code_sig hashes the sources compiled into the binary (Go, templates, embedded JS
# and fonts) plus the ACLs — a change here needs a full rebuild. input.css is
# deliberately excluded: it takes the fast CSS path below. css_sig tracks input.css
# alone.
code_sig() {
	{
		find cmd internal \( -name '*.go' -o -name '*.tmpl' -o -name '*.js' \) -printf '%T@ %p\n'
		find internal/server/assets/fonts -type f -printf '%T@ %p\n'
		find "$ACL_SRC" "$RPCD_ACL_SRC" -name '*.json' -printf '%T@ %p\n'
		find docker/rootfs/etc/init.d/verso -printf '%T@ %p\n'
		find plugins/verso-plugin-sdk -type f -printf '%T@ %p\n'
		for marker in $BUNDLED_PLUGIN_GLOB; do
			[ -e "$marker" ] || continue
			find "$(dirname "$marker")" -path '*/target' -prune -o -type f -printf '%T@ %p\n'
		done
	} 2>/dev/null | sha1sum
}
css_sig() { find "$CSS_IN" -printf '%T@\n' 2>/dev/null | sha1sum; }

ensure_container
reload

last_code="$(code_sig)"; last_css="$(css_sig)" # baseline before announcing, so no edit is missed
log "watching for changes (Ctrl-C to stop)… CSS edits hot-swap in ~1s; code edits rebuild"
while sleep 1; do
	cur_code="$(code_sig)"
	if [ "$cur_code" != "$last_code" ]; then
		last_code="$cur_code"
		last_css="$(css_sig)" # a full reload re-embeds the css too, so track it here
		reload
	else
		cur_css="$(css_sig)"
		if [ "$cur_css" != "$last_css" ]; then
			last_css="$cur_css"
			compile_css
			push_css
			log "css hot-swapped (no rebuild)"
		fi
	fi
done
