#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# check-feed-arch.sh <feed-dir> <apk-arch>
#
# Every package in a feed directory is built for the feed's architecture or for
# none (noarch). Run before a feed is indexed or published: it names each package
# that does not belong, with the architecture it was built for, and fails. The apk
# tool is $APK (the OpenWrt buildroot's), or apk on PATH.
set -eu

[ $# -eq 2 ] || { echo "usage: $0 <feed-dir> <apk-arch>" >&2; exit 2; }
dir=$1
feed_arch=$2
apk=${APK:-apk}
[ -d "$dir" ] || { echo "$dir: no such feed directory" >&2; exit 2; }

status=0
for pkg in "$dir"/*.apk; do
	[ -e "$pkg" ] || continue
	if ! info=$("$apk" adbdump "$pkg"); then
		echo "$pkg: unreadable by $apk adbdump" >&2
		status=1
		continue
	fi
	arch=$(printf '%s\n' "$info" | sed -n 's/^  arch: //p' | head -n 1)
	case $arch in
	"$feed_arch" | noarch) ;;
	*)
		echo "$pkg: built for ${arch:-no architecture}, not for the $feed_arch feed" >&2
		status=1
		;;
	esac
done
exit "$status"
