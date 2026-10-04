// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/we-are-mono/verso/internal/plugin"
)

// fileConfig names the uci config whose daemon reads a hand-edited file: the
// dnsmasq files are dhcp's, the nftables rule files fw4 loads are the
// firewall's. The helper bounds the paths themselves; this binds each one to
// the plugin allowed to change what that daemon does.
func fileConfig(path string) (string, bool) {
	switch {
	case path == "/etc/dnsmasq.conf" || strings.HasPrefix(path, "/etc/dnsmasq.d/"):
		return "dhcp", true
	case strings.HasPrefix(path, "/etc/nftables.d/"):
		return "firewall", true
	}
	return "", false
}

// Commands have a closed vocabulary and structured arguments. They run only
// after validation on a CSRF-protected POST, with the operator's own session.
func (s *Server) runPluginCommands(ctx context.Context, m plugin.Manifest, sid string, commands []plugin.ApplyAction) error {
	if len(commands) != 1 {
		return fmt.Errorf("expected one command")
	}
	cmd := commands[0]
	switch cmd.Name {
	case "config-file-stage":
		// A file belongs to the daemon that reads it, and is staged only by a
		// plugin that may write that daemon's config.
		config, known := fileConfig(cmd.Args["path"])
		configAllowed := known && slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: config, Function: "write"})
		allowed := slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "verso", Function: "stageConfigFile"})
		if !allowed || !configAllowed || len(cmd.Args) != 3 {
			return fmt.Errorf("invalid file staging command")
		}
		err := s.backend.StageConfigFile(ctx, sid, cmd.Args["path"], cmd.Args["expected"], cmd.Args["content"])
		var validation interface{ ValidationMessage() string }
		if errors.As(err, &validation) && validation.ValidationMessage() != "" {
			return commandValidationError(validation.ValidationMessage())
		}
		return err
	case "ssh-key-add", "ssh-key-remove", "certificate-generate", "certificate-install":
		return s.credentialCommand(ctx, m, sid, cmd)
	case "set-system-time":
		if err := validateApplyActions(m, commands); err != nil {
			return err
		}
		return s.backend.SetSystemTime(ctx, sid, cmd.Args["datetime"], cmd.Args["timezone"])
	case "interface-restart", "interface-up", "interface-down":
		method := strings.TrimPrefix(cmd.Name, "interface-")
		allowed := slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "network.interface", Function: method})
		name := cmd.Args["interface"]
		if !allowed || len(cmd.Args) != 1 || name == "" || name == "loopback" || len(name) > 63 || strings.IndexFunc(name, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
		}) >= 0 {
			return fmt.Errorf("invalid interface command")
		}
		if method != "restart" {
			return s.backend.NetworkSetUp(ctx, sid, name, method == "up")
		}
		return s.backend.NetworkRestart(ctx, sid, name)
	default:
		return fmt.Errorf("unknown command")
	}
}
