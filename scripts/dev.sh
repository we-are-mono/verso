#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# Dev loop: watch Go sources + templates + ACLs, rebuild the shell binary and the
# privileged rpcd helper (verso-rpcd), hot-swap both into the running OpenWrt
# container, restart the verso service, and (re)deploy the ubusd + rpcd ACLs. No
# image rebuild, no OpenWrt reboot — refresh the browser to see changes (~3s).
set -euo pipefail

cd "$(dirname "$0")/.."

CONTAINER=verso
CMD=./cmd/verso
BIN=build/verso
ACL_SRC=docker/rootfs/usr/share/acl.d
RPCD_ACL_SRC=docker/rootfs/usr/share/rpcd/acl.d
URL="http://localhost:8080"

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

# deploy_helper builds and deploys the privileged rpcd helper (verso-rpcd) and its
# rpcd sid-ACL, so its behaviour tracks the source like the shell binary does.
# rpcd execs the helper fresh per call, so a logic change needs no restart; the
# method list and ACL are picked up by `rpcd reload`, which preserves live
# sessions (so the dev browser is not logged out).
deploy_helper() {
	if CGO_ENABLED=0 go build -trimpath -o build/verso-rpcd ./cmd/verso-rpcd 2>&1; then
		docker cp build/verso-rpcd "$CONTAINER":/usr/libexec/rpcd/verso
		docker exec "$CONTAINER" sh -c 'chown root:root /usr/libexec/rpcd/verso; chmod 0755 /usr/libexec/rpcd/verso'
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

ensure_container() {
	if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null)" != "true" ]; then
		log "container not running — starting it (docker compose up -d --build)"
		docker compose up -d --build
	fi
}

reload() {
	# Privileged surface deploys first, unconditionally: an ACL or helper edit
	# should land even when the shell build below fails and we keep the old binary.
	deploy_acls
	deploy_helper
	log "building…"
	if [ -x build/tools/tailwindcss ]; then
		build/tools/tailwindcss -i internal/server/assets/input.css \
			-o internal/server/assets/verso.css --minify >/dev/null 2>&1 || true
	fi
	if ! CGO_ENABLED=0 go build -trimpath -o "$BIN" "$CMD" 2>&1; then
		log "build failed — keeping the running binary"
		return 0
	fi
	docker exec "$CONTAINER" /etc/init.d/verso stop >/dev/null 2>&1 || true
	docker cp "$BIN" "$CONTAINER":/usr/bin/verso
	docker exec "$CONTAINER" /etc/init.d/verso start >/dev/null 2>&1 || true
	log "reloaded → $URL"
}

# sig hashes the mtimes of watched sources; embedded templates count because
# they are compiled into the binary.
sig() {
	{
		find cmd internal \( -name '*.go' -o -name '*.tmpl' -o -name '*.css' \) \
			! -name 'verso.css' -printf '%T@ %p\n'
		find "$ACL_SRC" "$RPCD_ACL_SRC" -name '*.json' -printf '%T@ %p\n'
	} 2>/dev/null | sha1sum
}

ensure_container
reload

last="$(sig)" # capture baseline before announcing, so no edit is missed
log "watching for changes (Ctrl-C to stop)…"
while sleep 1; do
	cur="$(sig)"
	if [ "$cur" != "$last" ]; then
		last="$cur"
		reload
	fi
done
