// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const (
	defaultHelperSocket = "/var/run/verso/verso-rpcd.sock"
	helperCallTimeout   = 90 * time.Second
	// firmwareUpgradeTimeout bounds the one verb whose work is not this
	// device's: an attended-sysupgrade server queues and compiles an image for
	// this exact board and package set, then the router downloads it. That is
	// minutes of someone else's machine, so the wait a browser never sees is
	// measured in them — a ninety-second cap would report a build in progress
	// as a failed upgrade.
	firmwareUpgradeTimeout = 30 * time.Minute
	helperPermissionDenied = 6
)

type helperOperationError struct {
	method  string
	status  int
	message string
}

func (e helperOperationError) Error() string {
	return fmt.Sprintf("verso-rpcd: %s: status %d: %s", e.method, e.status, e.message)
}
func (e helperOperationError) ValidationMessage() string {
	if e.status == 2 {
		return e.message
	}
	return ""
}

type helperRequest struct {
	Method string            `json:"method"`
	SID    string            `json:"sid"`
	Args   map[string]string `json:"args,omitempty"`
}

type helperResponse struct {
	Status int             `json:"status"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

// callHelper sends one request to the resident Rust privileged companion. The
// helper independently verifies SID access over ubus before acting; filesystem
// permissions on the socket are an additional boundary, not the authorization
// decision. One request owns one short socket connection, while the root daemon
// and its package-operation lock remain shared across every browser session.
func callHelper(ctx context.Context, socket, method, sid string, args map[string]string, result any) error {
	return callHelperWithin(ctx, socket, method, sid, args, result, helperCallTimeout)
}

// callHelperWithin is callHelper with the wait stated by the caller, for the
// verbs whose work is genuinely long. The timeout is a floor, not a promise: a
// context deadline that lands sooner still wins, so a request-scoped call can
// never outlive its request.
func callHelperWithin(ctx context.Context, socket, method, sid string, args map[string]string, result any, timeout time.Duration) error {
	if socket == "" {
		socket = defaultHelperSocket
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("verso-rpcd: dial %s: %w", socket, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("verso-rpcd: deadline: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(helperRequest{Method: method, SID: sid, Args: args}); err != nil {
		return fmt.Errorf("verso-rpcd: %s request: %w", method, err)
	}

	var response helperResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return fmt.Errorf("verso-rpcd: %s response: %w", method, err)
	}
	if response.Status != 0 {
		if response.Status == helperPermissionDenied {
			return ErrAccessDenied
		}
		if response.Error == "" {
			response.Error = "request failed"
		}
		return helperOperationError{method: method, status: response.Status, message: response.Error}
	}
	if result != nil && len(response.Result) != 0 {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("verso-rpcd: %s result: %w", method, err)
		}
	}
	return nil
}
