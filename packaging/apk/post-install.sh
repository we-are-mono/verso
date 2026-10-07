#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# apk post-install / post-upgrade hook. Creates the de-privileged verso user,
# opts the device into the daily update check, schedules it, reloads the ubus
# ACLs, and (re)starts the services.
grep -q '^verso:' /etc/group  2>/dev/null || echo 'verso:x:6000:' >> /etc/group
grep -q '^verso:' /etc/passwd 2>/dev/null || echo 'verso:x:6000:6000:verso:/var/run/verso:/bin/false' >> /etc/passwd
# The shipped product opts into unattended update checking visibly: the option is
# written here rather than into /etc/config/verso, which a raw package upgrade
# would re-extract over an owner's edits. Only an absent option is seeded, so an
# owner's explicit 0 survives every upgrade (ADR-014 §2).
uci -q get verso.updates.autocheck >/dev/null || {
	uci -q set verso.updates=updates
	uci -q set verso.updates.autocheck='1'
	uci -q commit verso
}
# The schedule is one constant line the package owns. Appending it only when it is
# absent keeps a reinstall or upgrade from stacking duplicates, and crond starts
# once a crontab exists.
mkdir -p /etc/crontabs
if ! grep -qsF '/usr/libexec/verso/update-check' /etc/crontabs/root; then
	# A hand-edited crontab may lack a trailing newline; appending then would
	# splice our line onto the owner's last one. Add the newline first when the
	# file's final byte is not one (an empty or absent file needs none).
	[ -s /etc/crontabs/root ] && [ -n "$(tail -c1 /etc/crontabs/root)" ] && echo >> /etc/crontabs/root
	echo '0 2 * * * /usr/libexec/verso/update-check' >> /etc/crontabs/root
fi
/etc/init.d/cron enable 2>/dev/null
/etc/init.d/cron start 2>/dev/null
# Usage history is nlbwmon's, kept a day a period on storage that survives a
# reboot (ADR-018); it restarts to take the periods up.
/usr/libexec/verso/usage-setup
/etc/init.d/nlbwmon enable 2>/dev/null
/etc/init.d/nlbwmon restart 2>/dev/null
[ -x /etc/init.d/rpcd ] && /etc/init.d/rpcd reload 2>/dev/null
killall -HUP ubusd 2>/dev/null
/etc/init.d/verso enable 2>/dev/null
/etc/init.d/verso-rpcd enable 2>/dev/null
/etc/init.d/verso-plugin-interfaces enable 2>/dev/null
/etc/init.d/verso-plugin-system enable 2>/dev/null
/etc/init.d/verso-plugin-firewall enable 2>/dev/null
/etc/init.d/verso-rpcd restart 2>/dev/null
/etc/init.d/verso-plugin-interfaces restart 2>/dev/null
/etc/init.d/verso-plugin-system restart 2>/dev/null
/etc/init.d/verso-plugin-firewall restart 2>/dev/null
/etc/init.d/verso restart 2>/dev/null
exit 0
