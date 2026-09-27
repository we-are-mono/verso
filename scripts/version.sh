#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
#
# version.sh prints the build's version, <X.Y.Z>-r<N>: the latest vX.Y.Z tag and
# how many commits stand on it. Every commit builds its own version, in order —
# the tagged commit is 0.1.0-r0, the third after it 0.1.0-r3 — so apk sees each
# one as an upgrade, and a new version is a new tag. A tree with no release tag
# builds 0.0.0-r0. The Makefile stamps this into every binary and package, and
# `make dev` adds "-dev".
set -eu

cd "$(dirname "$0")/.."

if ! tag=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null); then
	echo "0.0.0-r0"
	exit 0
fi
echo "${tag#v}-r$(git rev-list --count "$tag"..HEAD)"
