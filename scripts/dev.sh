#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# Dev loop: watch the shell, helper, bundled plugins, and ACLs independently;
# rebuild and hot-swap only the component that changed. No image rebuild or
# OpenWrt reboot is needed, and a shell/UI edit cannot interrupt an in-flight
# verso-rpcd request.
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

# Two watchers targeting the same container race over the same staging names
# and can restart a component underneath an in-flight request. Keep one dev loop
# per container; flock releases this automatically when the script exits.
LOCK_FILE="${TMPDIR:-/tmp}/verso-dev-$CONTAINER.lock"
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
	log "another make dev process is already targeting $CONTAINER"
	exit 1
fi

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

# deploy_rpcd_acls updates session.access policy without touching verso-rpcd.
# rpcd reload preserves live login sessions.
deploy_rpcd_acls() {
	local f names=()
	for f in "$RPCD_ACL_SRC"/*.json; do
		[ -e "$f" ] || continue
		docker cp "$f" "$CONTAINER":/usr/share/rpcd/acl.d/"$(basename "$f")"
		names+=("/usr/share/rpcd/acl.d/$(basename "$f")")
	done
	[ ${#names[@]} -gt 0 ] || return 0
	docker exec "$CONTAINER" sh -c "chown root:root ${names[*]}; chmod 0644 ${names[*]}"
	docker exec "$CONTAINER" /etc/init.d/rpcd reload >/dev/null 2>&1 || true
}

# deploy_helper builds and atomically replaces the persistent Rust companion.
# It is called only when the helper itself changes, so shell, plugin, and ACL
# edits cannot sever a long-running package operation.
deploy_helper() {
	local cargo_bin="${CARGO:-$HOME/.cargo/bin/cargo}"
	# Build from the crate directory so rustup honors its pinned toolchain file;
	# the host's distro cargo otherwise emits a glibc binary OpenWrt cannot run.
	if (cd verso-rpcd && "$cargo_bin" build --locked --release --target x86_64-unknown-linux-musl) 2>&1; then
		docker exec "$CONTAINER" /etc/init.d/verso-rpcd stop >/dev/null 2>&1 || true
		docker cp verso-rpcd/target/x86_64-unknown-linux-musl/release/verso-rpcd "$CONTAINER":/usr/sbin/.verso-rpcd.new
		docker cp docker/rootfs/etc/init.d/verso-rpcd "$CONTAINER":/etc/init.d/.verso-rpcd.new
		docker exec "$CONTAINER" sh -c 'chown root:root /usr/sbin/.verso-rpcd.new /etc/init.d/.verso-rpcd.new; chmod 0755 /usr/sbin/.verso-rpcd.new /etc/init.d/.verso-rpcd.new; mv /usr/sbin/.verso-rpcd.new /usr/sbin/verso-rpcd; mv /etc/init.d/.verso-rpcd.new /etc/init.d/verso-rpcd; /etc/init.d/verso-rpcd enable; /etc/init.d/verso-rpcd start'
		log "verso-rpcd reloaded"
	else
		log "verso-rpcd build failed — keeping the running helper"
	fi
}

# deploy_i18n lands the localization catalogs — ADR-012 data files the shell
# reads from disk, never part of the binary — under the shell's i18n dir and
# restarts the shell so it reloads them. On a real device these arrive as
# verso-i18n-* packages; the dev loop syncs the repo's catalogs directly.
deploy_i18n() {
	local d code
	for d in i18n/*/; do
		[ -d "$d" ] || continue
		code="$(basename "$d")"
		docker exec "$CONTAINER" mkdir -p "/usr/share/verso/i18n/$code"
		docker cp "$d/." "$CONTAINER":"/usr/share/verso/i18n/$code/"
	done
	docker exec "$CONTAINER" sh -c 'chown -R root:root /usr/share/verso/i18n; find /usr/share/verso/i18n -type f -exec chmod 0644 {} +' 2>/dev/null || true
	docker exec "$CONTAINER" /etc/init.d/verso restart >/dev/null 2>&1 || true
	log "i18n catalogs reloaded"
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
			# The plugin's travelling catalogs (i18n/<code>.json, ADR-012) ride
			# beside the manifest; the shell re-reads them on its next start.
			if [ -d "$dir/i18n" ]; then
				docker exec "$CONTAINER" mkdir -p "/usr/share/verso/plugins/$id/i18n"
				docker cp "$dir/i18n/." "$CONTAINER:/usr/share/verso/plugins/$id/i18n/"
				docker exec "$CONTAINER" sh -c "chown -R root:root /usr/share/verso/plugins/$id/i18n; find /usr/share/verso/plugins/$id/i18n -type f -exec chmod 0644 {} +"
			fi
			docker exec "$CONTAINER" sh -c "chown root:root /usr/bin/.$name.new /etc/init.d/.$name.new /usr/share/verso/plugins/$id/.manifest.json.new; chmod 0755 /usr/bin/.$name.new /etc/init.d/.$name.new; chmod 0644 /usr/share/verso/plugins/$id/.manifest.json.new; mv /usr/bin/.$name.new /usr/bin/$name; mv /etc/init.d/.$name.new /etc/init.d/$name; mv /usr/share/verso/plugins/$id/.manifest.json.new /usr/share/verso/plugins/$id/manifest.json; /etc/init.d/$name enable; /etc/init.d/$name start"
			log "$name reloaded"
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

# compile_css regenerates the embedded stylesheet via `make css` — the one compile
# definition, whose tool rule also downloads Tailwind on a fresh clone. A failure
# is reported, not swallowed: the caller keeps the last-good stylesheet, matching
# the helper/plugin "keep the running one" behavior.
compile_css() {
	local out
	if ! out="$(make -s css 2>&1)"; then
		log "css compile failed — keeping last stylesheet"
		[ -z "$out" ] || printf '%s\n' "$out"
		return 1
	fi
}

# push_css drops the freshly-compiled stylesheet into the container at $DEV_CSS, where
# the shell reads it live (no rebuild). The copy is staged and atomically renamed so a
# render never reads a half-written file. This is what makes a CSS edit a ~1s hot-swap.
push_css() {
	[ -f "$CSS_OUT" ] || return 0
	docker cp "$CSS_OUT" "$CONTAINER":"$DEV_CSS.tmp" >/dev/null 2>&1 || return 0
	docker exec "$CONTAINER" sh -c "chmod 0644 '$DEV_CSS.tmp' && mv '$DEV_CSS.tmp' '$DEV_CSS'" >/dev/null 2>&1 || true
}

# deploy_shell rebuilds and replaces only the Go shell. Keeping this separate is
# important: this is by far the most frequent edit path and must not restart the
# helper or any plugin.
deploy_shell() {
	log "building shell…"
	compile_css || true # embedded CSS stays last-good; the failure is already logged
	if ! CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$BIN" "$CMD" 2>&1; then
		log "build failed — keeping the running binary"
		return 0
	fi
	deploy_shell_init
	docker exec "$CONTAINER" /etc/init.d/verso stop >/dev/null 2>&1 || true
	docker cp "$BIN" "$CONTAINER":/usr/bin/verso
	push_css # land the live stylesheet before start, so the shell boots in CSS hot-reload mode
	docker exec "$CONTAINER" /etc/init.d/verso start >/dev/null 2>&1 || true
	log "reloaded → $URL"
}

# Bring a newly started container completely in sync once. Subsequent edits use
# the component-specific paths below.
sync_all() {
	deploy_acls
	deploy_rpcd_acls
	deploy_i18n
	deploy_helper
	deploy_bundled_plugins
	deploy_shell
}

# Signatures deliberately exclude build output directories. input.css also has
# its own fast path, while a shell rebuild recompiles it before embedding assets.
shell_sig() {
	{
		find cmd internal -type f ! -path "$CSS_IN" ! -path "$CSS_OUT" -printf '%T@ %p\n'
		find docker/rootfs/etc/init.d/verso -printf '%T@ %p\n'
	} 2>/dev/null | sha1sum
}
helper_sig() {
	{
		find verso-rpcd/src -type f -printf '%T@ %p\n'
		find verso-rpcd/Cargo.toml verso-rpcd/Cargo.lock verso-rpcd/rust-toolchain.toml -printf '%T@ %p\n'
		find docker/rootfs/etc/init.d/verso-rpcd -printf '%T@ %p\n'
	} 2>/dev/null | sha1sum
}
plugins_sig() {
	{
		find plugins/verso-plugin-sdk -type f -printf '%T@ %p\n'
		for marker in $BUNDLED_PLUGIN_GLOB; do
			[ -e "$marker" ] || continue
			find "$(dirname "$marker")" -path '*/target' -prune -o -type f -printf '%T@ %p\n'
		done
	} 2>/dev/null | sha1sum
}
acl_sig() { find "$ACL_SRC" "$RPCD_ACL_SRC" -name '*.json' -printf '%T@ %p\n' 2>/dev/null | sha1sum; }
i18n_sig() { find i18n -name '*.json' -printf '%T@ %p\n' 2>/dev/null | sha1sum; }
css_sig() { find "$CSS_IN" -printf '%T@\n' 2>/dev/null | sha1sum; }

ensure_container
# Capture the baseline before the initial synchronization. If a source changes
# while that longer operation is running, the first watch tick deploys it again
# rather than silently treating an undeployed edit as current.
last_shell="$(shell_sig)"
last_helper="$(helper_sig)"
last_plugins="$(plugins_sig)"
last_acl="$(acl_sig)"
last_i18n="$(i18n_sig)"
last_css="$(css_sig)"
sync_all

log "watching for changes (Ctrl-C to stop)… components reload independently; CSS hot-swaps in ~1s"
while sleep 1; do
	cur_shell="$(shell_sig)"
	cur_helper="$(helper_sig)"
	cur_plugins="$(plugins_sig)"
	cur_acl="$(acl_sig)"
	cur_i18n="$(i18n_sig)"
	cur_css="$(css_sig)"

	if [ "$cur_acl" != "$last_acl" ]; then
		last_acl="$cur_acl"
		deploy_acls
		deploy_rpcd_acls
		log "ACLs reloaded (services uninterrupted)"
	fi
	if [ "$cur_i18n" != "$last_i18n" ]; then
		last_i18n="$cur_i18n"
		deploy_i18n
	fi
	if [ "$cur_helper" != "$last_helper" ]; then
		last_helper="$cur_helper"
		deploy_helper
	fi
	if [ "$cur_plugins" != "$last_plugins" ]; then
		last_plugins="$cur_plugins"
		deploy_bundled_plugins
	fi
	if [ "$cur_shell" != "$last_shell" ]; then
		last_shell="$cur_shell"
		last_css="$cur_css" # deploy_shell recompiles and embeds the current CSS
		deploy_shell
	elif [ "$cur_css" != "$last_css" ]; then
		last_css="$cur_css"
		if compile_css; then
			push_css
			log "css hot-swapped (no rebuild)"
		fi
	fi
done
