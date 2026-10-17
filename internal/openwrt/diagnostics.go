// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"strconv"
	"time"
)

// DiagnosticRun is one network diagnostic, run on the router by the helper:
// the tool ("ping", "traceroute" or "nslookup"), what it reaches for, the
// kernel device it leaves by ("" for whichever the routes choose) and the IP
// family ("4", "6", or "" for whichever the name resolves to). The helper
// builds the command from these parts; none of them is ever a flag.
type DiagnosticRun struct {
	Tool      string
	Target    string
	Interface string
	Family    string
}

// DiagnosticOutput is what a run has written past a cursor. Next is the cursor
// to read from after these lines. A finished run is Done, and Ended says how
// it finished: "exited" (with the program's Code), "stopped" or "timeout".
type DiagnosticOutput struct {
	Lines []string `json:"lines"`
	Next  int      `json:"next"`
	Done  bool     `json:"done"`
	Code  *int     `json:"code"`
	Ended string   `json:"ended"`
}

// Starting a run, reading it and stopping it are each one short request: the
// run itself goes on in the helper, between them.
const diagCallTimeout = 5 * time.Second

func (b *NativeBackend) DiagStart(ctx context.Context, sid string, run DiagnosticRun) (string, error) {
	var result struct {
		Job string `json:"job"`
	}
	err := callHelperWithin(ctx, b.helperSocket, "diagStart", sid, map[string]string{
		"tool": run.Tool, "target": run.Target, "interface": run.Interface, "family": run.Family,
	}, &result, diagCallTimeout)
	return result.Job, err
}

func (b *NativeBackend) DiagRead(ctx context.Context, sid, job string, after int) (DiagnosticOutput, error) {
	var result DiagnosticOutput
	err := callHelperWithin(ctx, b.helperSocket, "diagRead", sid, map[string]string{
		"job": job, "after": strconv.Itoa(after),
	}, &result, diagCallTimeout)
	return result, err
}

func (b *NativeBackend) DiagStop(ctx context.Context, sid, job string) error {
	return callHelperWithin(ctx, b.helperSocket, "diagStop", sid, map[string]string{"job": job}, nil, diagCallTimeout)
}
