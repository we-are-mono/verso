#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# apk post-deinstall hook, once Verso's files are gone: the users the shell and
# its bundled plugins ran as, the web certificate only Verso served, and the
# runtime directory go, and rpcd and ubusd forget Verso's access lists. Each
# plugin package removes its own user; apk removes them before this one.
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

sed -i -e '/^verso:/d' -e '/^verso-plugin-system:/d' -e '/^verso-plugin-firewall:/d' \
	-e '/^verso-plugin-interfaces:/d' /etc/passwd
sed -i '/^verso:/d' /etc/group
rm -rf /etc/verso /var/run/verso
[ -x /etc/init.d/rpcd ] && /etc/init.d/rpcd reload
killall -HUP ubusd 2>/dev/null
exit 0
