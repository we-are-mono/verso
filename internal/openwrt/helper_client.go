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
	defaultHelperSocket    = "/var/run/verso/verso-rpcd.sock"
	helperCallTimeout      = 90 * time.Second
	helperPermissionDenied = 6
)

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
	if socket == "" {
		socket = defaultHelperSocket
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("verso-rpcd: dial %s: %w", socket, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(helperCallTimeout)
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
		return fmt.Errorf("verso-rpcd: %s: status %d: %s", method, response.Status, response.Error)
	}
	if result != nil && len(response.Result) != 0 {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("verso-rpcd: %s result: %w", method, err)
		}
	}
	return nil
}
