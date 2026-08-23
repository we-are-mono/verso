#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# Dev loop: watch Go sources + templates, rebuild the static binary, hot-swap it
# into the running OpenWrt container and restart the verso service. No image
# rebuild, no OpenWrt reboot — refresh the browser to see changes (~3s).
set -euo pipefail

cd "$(dirname "$0")/.."

CONTAINER=verso
CMD=./cmd/verso
BIN=build/verso
URL="http://localhost:8080"

log() { printf '\033[36m[dev]\033[0m %s\n' "$*"; }

ensure_container() {
	if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null)" != "true" ]; then
		log "container not running — starting it (docker compose up -d --build)"
		docker compose up -d --build
	fi
}

reload() {
	log "building…"
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
	find cmd internal \( -name '*.go' -o -name '*.tmpl' \) -printf '%T@ %p\n' 2>/dev/null | sha1sum
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
