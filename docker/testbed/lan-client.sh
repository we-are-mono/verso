#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# lan-client: a LAN host behind the router. Drops the Docker-assigned address
# and asks the router for one, like a laptop joining the network — so it lands
# in the router's DHCP leases and its traffic transits the box. The udhcpc
# handler is written here rather than trusting the image to ship one.
set -e

cat > /tmp/udhcpc.sh <<"EOF"
#!/bin/sh
case "$1" in bound | renew) ;; *) exit 0 ;; esac
# v4 only — flushing both families here would wipe the SLAAC address the
# router's RA just granted.
ip -4 address flush dev "$interface"
ip address add "$ip/${subnet:-255.255.255.0}" dev "$interface"
[ -n "$router" ] && ip route replace default via "$router" dev "$interface"
if [ -n "$dns" ]; then
	: > /etc/resolv.conf
	for d in $dns; do echo "nameserver $d" >> /etc/resolv.conf; done
fi
EOF
chmod 0755 /tmp/udhcpc.sh

# Drop Docker's placeholder addresses (v4, and the global v6) but keep the
# door open for the router's world: the link bounce regenerates the
# link-local and re-runs SLAAC against the router's advertisements.
ip -4 address flush dev lan0
ip -6 address flush dev lan0 scope global
ip link set lan0 down
ip link set lan0 up

# The first router solicitations can all fire while the link is still
# settling, and the next periodic RA is minutes out — re-bounce until SLAAC
# lands a global address.
(
	for _ in 1 2 3 4 5 6; do
		sleep 6
		ip -6 addr show lan0 | grep -q "scope global" && exit 0
		ip link set lan0 down
		ip link set lan0 up
	done
) &
# Introduce ourselves like a real laptop would: the hostname lands in the
# router's leases and is what its device roster shows.
exec udhcpc -i lan0 -f -S -s /tmp/udhcpc.sh -x hostname:family-laptop
