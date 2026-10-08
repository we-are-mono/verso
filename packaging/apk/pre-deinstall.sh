#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# apk pre-deinstall hook, while Verso's files are still on disk: it undoes what
# installing and running Verso changed outside its own files, so the router is
# left as it was before, but for what is the owner's (Verso's settings file and
# the usage history nlbwmon wrote). post-deinstall finishes once the files are
# gone. An upgrade runs neither.
export PATH=/usr/sbin:/usr/bin:/sbin:/bin

# Packet logging returns to OpenWrt's standard kernel transport; upstream and
# local template edits stay.
/usr/libexec/verso/firewall-logging-setup remove || exit 1

# LuCI's uhttpd gets the router's web ports back if Verso took them (ADR-017).
if [ "$(uci -q get uhttpd.@uhttpd[0].listen_http)" = "0.0.0.0:8080 [::]:8080" ] &&
	[ "$(uci -q get uhttpd.@uhttpd[0].listen_https)" = "0.0.0.0:8443 [::]:8443" ]; then
	/usr/libexec/verso/web-owner luci ||
		echo 'verso: uhttpd stays on 8080 and 8443; set its listen_http and listen_https to give it 80 and 443' >&2
fi

for service in verso verso-plugin-interfaces verso-plugin-system verso-plugin-firewall verso-rpcd; do
	[ -x "/etc/init.d/$service" ] || continue
	"/etc/init.d/$service" stop
	"/etc/init.d/$service" disable
done

# The daily update check is the package's one crontab line.
[ -f /etc/crontabs/root ] && sed -i '\#/usr/libexec/verso/update-check#d' /etc/crontabs/root

/usr/libexec/verso/usage-setup remove
/etc/init.d/nlbwmon running 2>/dev/null && /etc/init.d/nlbwmon restart
exit 0
