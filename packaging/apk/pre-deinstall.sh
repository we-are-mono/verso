#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
# Undo only Verso's log transport addition, retaining upstream/local template
# edits. Removal returns packet logging to OpenWrt's standard kernel transport.
/usr/libexec/verso/firewall-logging-setup remove || exit 1
/etc/init.d/verso-rpcd stop
/etc/init.d/verso-rpcd disable
