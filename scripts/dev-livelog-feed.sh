#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
# Send real probes from the isolated WAN simulator. NFLOG works inside network
# namespaces, so Activity no longer needs synthetic messages injected into logd.
# Usage: scripts/dev-livelog-feed.sh [count] (zero/default keeps running).
set -euo pipefail
CONTAINER=${CONTAINER:-verso}
WAN_CONTAINER=${WAN_CONTAINER:-verso-wan}
COUNT=${1:-0}
case "$COUNT" in ''|*[!0-9]*) echo 'count must be a nonnegative integer' >&2; exit 2 ;; esac
router_ip=$(docker exec "$CONTAINER" sh -c 'ubus call network.interface.wan status | jsonfilter -e '\''@["ipv4-address"][0].address'\''')
[ -n "$router_ip" ] || { echo 'The test router has no WAN IPv4 address.' >&2; exit 1; }
sent=0
ports=(445 3389 23 8088)
while [ "$COUNT" -eq 0 ] || [ "$sent" -lt "$COUNT" ]; do
	docker exec "$WAN_CONTAINER" nc -z -w 1 "$router_ip" "${ports[$((sent % 4))]}" >/dev/null 2>&1 || true
	sent=$((sent + 1))
	sleep 1
done
