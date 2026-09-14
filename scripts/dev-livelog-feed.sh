#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# Dev testbed only: put firewall verdicts into the container's log ring so
# Firewall → Activity has something to show.
#
# Why this exists. On a real router the kernel writes fw4's verdicts to
# /proc/kmsg, logd picks them up, and `ubus call log read` hands them to the
# shell. In the Docker testbed neither half of that works: the kernel refuses
# netfilter logging from a network namespace that is not the initial one
# (net.netfilter.nf_log_all_netns is 0 on the host and only the host can change
# it), and the container has no real /proc/kmsg for logd to read even if it did.
# So the container's own firewall can be hit all day and its log stays empty.
#
# What this does. It writes the *same lines the kernel would have written* into
# the same ring, through logd's syslog socket, using the fw4 prefix as the
# syslog tag — which is byte-for-byte the shape `log read` returns for a real
# kernel verdict, minus the optional uptime stamp. Everything downstream is the
# product: the shell's ubus read, its netfilter parse, its zone and rule
# resolution, the event stream, and the browser's rows.
#
# The rule and forward it names are the ones scripts/dev.sh seeds, so the rows
# resolve to real sections with real editors behind them.
#
#   scripts/dev-livelog-feed.sh          # a steady trickle until Ctrl-C
#   scripts/dev-livelog-feed.sh 20       # twenty verdicts, then stop
#
set -euo pipefail

CONTAINER=${CONTAINER:-verso}
COUNT=${1:-0} # 0 = keep going

# One verdict, written the way the kernel writes it: the fw4 prefix, then the
# interface pair, then the packet.
emit() {
	docker exec "$CONTAINER" logger -t "$1" -p kern.warn "$2"
}

# The scanners: the internet's background noise arriving on wan and dying at the
# seeded logging rule. Repeats are deliberate — consecutive identical verdicts
# collapse into one row with a climbing counter.
probe() {
	emit "Log-WAN-probes" "IN=wan0 OUT= MAC=02:42:ac:1e:01:ab:02:42:ac:1e:01:02:08:00 SRC=$1 DST=172.30.1.171 LEN=60 TOS=0x00 PREC=0x00 TTL=52 ID=$((RANDOM % 60000)) DF PROTO=TCP SPT=$((RANDOM % 20000 + 40000)) DPT=$2 WINDOW=64240 RES=0x00 SYN URGP=0"
}

# The zone's own policy deciding, with no rule behind it — the row whose Rule
# cell is a quiet dash.
policy() {
	emit "reject wan in" "IN=wan0 OUT= MAC=02:42:ac:1e:01:ab SRC=$1 DST=172.30.1.171 LEN=44 TOS=0x00 PREC=0x00 TTL=44 ID=$((RANDOM % 60000)) PROTO=UDP SPT=$((RANDOM % 20000 + 40000)) DPT=$2 LEN=24"
}

# The seeded port forward letting someone in to a device on the network.
forward() {
	emit "Minecraft server" "IN=wan0 OUT=br-lan MAC=02:42:ac:1e:01:ab SRC=$1 DST=192.168.77.30 LEN=52 TOS=0x00 PREC=0x00 TTL=51 ID=$((RANDOM % 60000)) DF PROTO=TCP SPT=$((RANDOM % 20000 + 40000)) DPT=25565 WINDOW=65535 RES=0x00 SYN URGP=0"
}

# A ping the router answered — an accept, so all three verdict tones appear.
ping() {
	emit "Allow-Ping" "IN=wan0 OUT= MAC=02:42:ac:1e:01:ab SRC=$1 DST=172.30.1.171 LEN=84 TOS=0x00 PREC=0x00 TTL=54 ID=$((RANDOM % 60000)) DF PROTO=ICMP TYPE=8 CODE=0 ID=4242 SEQ=1"
}

sent=0
while [ "$COUNT" -eq 0 ] || [ "$sent" -lt "$COUNT" ]; do
	case $((sent % 8)) in
	0 | 1 | 2) probe "193.32.162.85" "445" ;; # the hammering scanner: one row, rising count
	3) probe "45.148.10.77" "22" ;;
	4) policy "162.142.125.9" "5060" ;;
	5) forward "84.255.202.10" ;;
	6) ping "89.248.165.201" ;;
	7) probe "89.248.165.201" "3389" ;;
	esac
	sent=$((sent + 1))
	sleep 1
done
