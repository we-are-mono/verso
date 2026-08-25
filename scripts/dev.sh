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
CSS_IN=internal/server/assets/input.css
CSS_OUT=internal/server/assets/verso.css
DEV_CSS=/usr/share/verso/verso-dev.css # in-container drop file the shell reads live (dev only; not /tmp — that's a tmpfs docker cp can't write)
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
	deploy_helper
	log "building…"
	compile_css
	if ! CGO_ENABLED=0 go build -trimpath -o "$BIN" "$CMD" 2>&1; then
		log "build failed — keeping the running binary"
		return 0
	fi
	docker exec "$CONTAINER" /etc/init.d/verso stop >/dev/null 2>&1 || true
	docker cp "$BIN" "$CONTAINER":/usr/bin/verso
	push_css # land the live stylesheet before start, so the shell boots in CSS hot-reload mode
	docker exec "$CONTAINER" /etc/init.d/verso start >/dev/null 2>&1 || true
	log "reloaded → $URL"
}

# code_sig hashes the sources compiled into the binary (Go, templates, embedded JS)
# plus the ACLs — a change here needs a full rebuild. input.css is deliberately excluded:
# it takes the fast CSS path below. css_sig tracks input.css alone.
code_sig() {
	{
		find cmd internal \( -name '*.go' -o -name '*.tmpl' -o -name '*.js' \) -printf '%T@ %p\n'
		find "$ACL_SRC" "$RPCD_ACL_SRC" -name '*.json' -printf '%T@ %p\n'
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
