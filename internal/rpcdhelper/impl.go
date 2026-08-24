// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package rpcdhelper

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/we-are-mono/verso/internal/ubus"
)

// SystemPasswordSetter sets a password via busybox passwd, feeding the new
// password twice on stdin — never via argv or the environment, so it is not
// visible in the process table.
type SystemPasswordSetter struct{}

// SetPassword runs passwd for username with the password supplied on stdin.
func (SystemPasswordSetter) SetPassword(username, password string) error {
	cmd := exec.Command("passwd", username)
	cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("passwd %q: %w: %s", username, err, bytes.TrimSpace(out))
	}
	return nil
}

// UbusAuthorizer self-gates helper calls by probing rpcd's session.access over
// the ubus socket, so the operator's ACL — not the helper — authorizes the
// action. Socket "" uses the default ubus socket.
type UbusAuthorizer struct{ Socket string }

// Access reports whether sid may call verso.<function>. It fails closed: an empty
// sid, an unreachable socket, or a probe error all deny.
func (a UbusAuthorizer) Access(sid, function string) (bool, error) {
	if sid == "" {
		return false, nil
	}
	c, err := ubus.Dial(a.Socket)
	if err != nil {
		return false, err
	}
	defer c.Close()
	id, err := c.Lookup("session")
	if err != nil {
		return false, err
	}
	res, err := c.InvokeArgs(id, "access", map[string]string{
		"ubus_rpc_session": sid,
		"scope":            "ubus",
		"object":           "verso",
		"function":         function,
	})
	if err != nil {
		return false, err
	}
	// ubus encodes booleans as INT8/BOOL, which the client decodes to int64 (1/0).
	switch v := res["access"].(type) {
	case bool:
		return v, nil
	case int64:
		return v != 0, nil
	}
	return false, nil
}
