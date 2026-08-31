#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# apk post-install / post-upgrade hook for the DNS/DHCP plugin. The shell package
# owns the verso user and group; this one only brings its own service up. The
# shell discovers the manifest on disk, so nothing else needs restarting.
/etc/init.d/verso-plugin-dnsdhcp enable 2>/dev/null
/etc/init.d/verso-plugin-dnsdhcp restart 2>/dev/null
exit 0
