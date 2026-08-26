#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# wan-sim: the fake ISP. Hands the router a DHCP address on wan-net (wan0) and
# NATs whatever the router sends up through this container's Docker uplink
# (eth0), so the testbed LAN reaches the real internet through an honest
# upstream hop. DNS rides along: dnsmasq forwards to Docker's resolver.
set -e

apk add -q --no-cache dnsmasq iptables

iptables -t nat -C POSTROUTING -o eth0 -j MASQUERADE 2>/dev/null ||
	iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE

# Serve only the ISP-facing interface; Docker's embedded DNS (resolv.conf)
# is the upstream. Range stays clear of .2 (this box), .10 (the router's
# flushed placeholder), and .254 (Docker's bridge).
exec dnsmasq --no-daemon --interface=wan0 --bind-interfaces \
	--dhcp-range=172.30.1.100,172.30.1.199,255.255.255.0,12h \
	--dhcp-option=3,172.30.1.2 --dhcp-option=6,172.30.1.2 \
	--dhcp-authoritative
