#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# wan-sim: the fake ISP, both address families.
#
# IPv4 — dnsmasq hands the router a DHCP address on wan-net (wan0) and this
# box NATs whatever the router sends up through its Docker uplink (eth0), so
# the testbed LAN reaches the real internet. DNS rides along: dnsmasq
# forwards to Docker's resolver.
#
# IPv6 — the real fiber-line flow: radvd advertises this box as the default
# router (M+O flags pointing at DHCPv6), Kea answers DHCPv6 with a WAN
# address (IA_NA) and a delegated /56 (IA_PD) out of fd42:7ea:aa00::/48. The
# router's reservation keys on its pinned MAC (docker-compose.yml), which
# also makes the route back to the delegated space static and known here. No
# NAT66 — v6 is routed, and being ULA it ends at this box.
set -e

apk add -q --no-cache dnsmasq iptables radvd kea-dhcp6

iptables -t nat -C POSTROUTING -o eth0 -j MASQUERADE 2>/dev/null ||
	iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE

cat > /etc/radvd.conf <<"EOF"
interface wan0 {
	AdvSendAdvert on;
	AdvManagedFlag on;
	AdvOtherConfigFlag on;
	AdvDefaultLifetime 1800;
};
EOF

cat > /etc/kea-dhcp6.conf <<"EOF"
{ "Dhcp6": {
	"interfaces-config": { "interfaces": [ "wan0" ] },
	"lease-database": { "type": "memfile", "persist": false },
	"mac-sources": [ "duid" ],
	"host-reservation-identifiers": [ "hw-address" ],
	"subnet6": [ {
		"id": 1,
		"subnet": "fd42:7ea:1::/64",
		"interface": "wan0",
		"pools": [ { "pool": "fd42:7ea:1::100 - fd42:7ea:1::1ff" } ],
		"pd-pools": [ { "prefix": "fd42:7ea:aa00::", "prefix-len": 48, "delegated-len": 56 } ],
		"reservations": [ {
			"hw-address": "02:42:7e:a0:00:01",
			"ip-addresses": [ "fd42:7ea:1::10" ],
			"prefixes": [ "fd42:7ea:aa00::/56" ]
		} ]
	} ]
} }
EOF

# The route back into the delegated space, via the router's reserved address.
ip -6 route replace fd42:7ea:aa00::/56 via fd42:7ea:1::10 dev wan0

mkdir -p /run/radvd /run/kea # both daemons refuse to start without their run dirs

# Kea binds wan0's link-local, which sits in duplicate-address detection for
# the first moments after the interface appears — wait until it settles.
until ip -6 addr show wan0 | grep -q "fe80" && ! ip -6 addr show wan0 | grep -q tentative; do
	sleep 1
done

radvd -m stderr
kea-dhcp6 -c /etc/kea-dhcp6.conf &

exec dnsmasq --no-daemon --interface=wan0 --bind-interfaces \
	--dhcp-range=172.30.1.100,172.30.1.199,255.255.255.0,12h \
	--dhcp-option=3,172.30.1.2 --dhcp-option=6,172.30.1.2 \
	--dhcp-authoritative
