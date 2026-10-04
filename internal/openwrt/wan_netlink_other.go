// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build !linux

package openwrt

import "errors"

func kernelDefaultRoutes() ([]kernelRoute, error) {
	return nil, errors.New("openwrt: route netlink is only available on Linux")
}

func liveRoutes() ([]liveRoute, error) {
	return nil, errors.New("openwrt: route netlink is only available on Linux")
}
