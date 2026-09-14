#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# apk post-install / post-upgrade hook for the device-limits plugin. The shell
# package owns the verso user and group; this one only brings its own service up.
# The shell discovers the manifest on disk, so nothing else needs restarting.
#
# rpcd reads its acl.d at login, so the grant this package ships is in force at
# the operator's next sign-in without a restart.
/etc/init.d/verso-plugin-qos enable 2>/dev/null
/etc/init.d/verso-plugin-qos restart 2>/dev/null
exit 0
