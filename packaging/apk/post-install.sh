#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# apk post-install / post-upgrade hook. Creates the de-privileged verso user,
# reloads the ubus ACLs, and (re)starts the services.
grep -q '^verso:' /etc/group  2>/dev/null || echo 'verso:x:6000:' >> /etc/group
grep -q '^verso:' /etc/passwd 2>/dev/null || echo 'verso:x:6000:6000:verso:/var/run/verso:/bin/false' >> /etc/passwd
[ -x /etc/init.d/rpcd ] && /etc/init.d/rpcd reload 2>/dev/null
killall -HUP ubusd 2>/dev/null
/etc/init.d/verso enable 2>/dev/null
/etc/init.d/verso-rpcd enable 2>/dev/null
/etc/init.d/verso-plugin-system enable 2>/dev/null
/etc/init.d/verso-plugin-firewall enable 2>/dev/null
/etc/init.d/verso-rpcd restart 2>/dev/null
/etc/init.d/verso-plugin-system restart 2>/dev/null
/etc/init.d/verso-plugin-firewall restart 2>/dev/null
/etc/init.d/verso restart 2>/dev/null
exit 0
