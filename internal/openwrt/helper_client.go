// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/we-are-mono/verso/internal/peercred"
)

const (
	// defaultHelperSocket sits in a directory of the helper's own, root:verso
	// 0750: the shell may reach it, but no group member can create a name there.
	defaultHelperSocket = "/var/run/verso-rpcd/verso-rpcd.sock"
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

// helperOwnerUID is the account the kernel must report listening behind the
// helper socket: root, which verso-rpcd runs as. Tests aim it at a fake.
var helperOwnerUID uint32

// errUntrustedHelper marks a helper socket whose listener is another account.
var errUntrustedHelper = errors.New("the socket's listener is not the helper's account")

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
// decision. The shell in turn sends nothing until the kernel reports root as the
// listener, so a request can reach no one but the helper. One request owns one
// short socket connection, while the root daemon and its package-operation lock
// remain shared across every browser session.
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
	if err := requireHelperOwner(conn); err != nil {
		return fmt.Errorf("verso-rpcd: %s: %w", socket, err)
	}

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

// requireHelperOwner refuses a connection unless the kernel reports the
// helper's account listening behind it. The path proves nothing on its own:
// whoever binds it while it is free answers there, and every request carries
// the operator's session and, for a password change, the new password.
func requireHelperOwner(conn net.Conn) error {
	uid, err := peercred.UID(conn)
	if err != nil {
		return fmt.Errorf("peer credentials: %w", err)
	}
	if uid != helperOwnerUID {
		return fmt.Errorf("%w (uid %d)", errUntrustedHelper, uid)
	}
	return nil
}
