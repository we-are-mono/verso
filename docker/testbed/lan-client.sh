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
ip address flush dev "$interface"
ip address add "$ip/${subnet:-255.255.255.0}" dev "$interface"
[ -n "$router" ] && ip route replace default via "$router" dev "$interface"
if [ -n "$dns" ]; then
	: > /etc/resolv.conf
	for d in $dns; do echo "nameserver $d" >> /etc/resolv.conf; done
fi
EOF
chmod 0755 /tmp/udhcpc.sh

ip address flush dev lan0
exec udhcpc -i lan0 -f -S -s /tmp/udhcpc.sh
