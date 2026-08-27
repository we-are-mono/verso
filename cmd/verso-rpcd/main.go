// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Command verso-rpcd is Verso's privileged rpcd helper: the small root-owned
// surface the unprivileged shell cannot perform itself (ADR-007). rpcd runs it as
// root and exposes it as the ubus object "verso" (named after the install path
// /usr/libexec/rpcd/verso). It has a single responsibility — perform ACL-gated
// root actions — kept apart from the shell binary so the root process carries the
// minimum code and dependencies.
package main

import (
	"os"

	"github.com/we-are-mono/verso/internal/rpcdhelper"
)

func main() {
	h := rpcdhelper.New(rpcdhelper.UbusAuthorizer{}, rpcdhelper.SystemPasswordSetter{}, rpcdhelper.ApkPackageManager{}, rpcdhelper.SysTrafficReader{})
	os.Exit(h.Run(os.Args[1:], os.Stdin, os.Stdout))
}
