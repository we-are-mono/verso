// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ConfigFile struct {
	Path    string `json:"path"`
	Family  string `json:"family"`
	Content string `json:"content"`
	Version string `json:"version"`
	Pending bool   `json:"pending"`
}

// fileFamilies binds each family of hand-edited files to the uci config whose
// daemon reads them: a staged file waits under that config, and discarding
// that config discards the family's files with it.
var fileFamilies = map[string]string{"dnsmasq": "dhcp", "fw4": "firewall"}

// familyOf is the family whose files a config owns, if any.
func familyOf(config string) (string, bool) {
	for family, owner := range fileFamilies {
		if owner == config {
			return family, true
		}
	}
	return "", false
}

type configFileState struct {
	Files        []ConfigFile `json:"files"`
	Active       bool         `json:"active"`
	UCI          bool         `json:"uci"`
	UCIConfirmed bool         `json:"uci_confirmed"`
}
type configFileCall func(context.Context, string, string, map[string]string, any) error

func nativeConfigFileCall(ctx context.Context, sid, method string, args map[string]string, result any) error {
	return callHelper(ctx, "", method, sid, args, result)
}
func (*NativeBackend) DNSState(ctx context.Context, sid string) (json.RawMessage, error) {
	var state json.RawMessage
	err := callHelper(ctx, "", "dnsState", sid, nil, &state)
	return state, err
}

// FirewallFiles is the rule files fw4 reads from its own folder, as they will
// read once staged changes are applied.
func (*NativeBackend) FirewallFiles(ctx context.Context, sid string) (json.RawMessage, error) {
	var state json.RawMessage
	err := callHelper(ctx, "", "firewallFiles", sid, nil, &state)
	return state, err
}
func (b *NativeBackend) configFileState(ctx context.Context, sid string) (configFileState, error) {
	var state configFileState
	if b.configFileCall == nil {
		return state, nil
	}
	err := b.configFileCall(ctx, sid, "configFiles", nil, &state)
	return state, err
}
func (b *NativeBackend) fileAction(ctx context.Context, sid, action string, uci bool, timeout int) error {
	if b.configFileCall == nil {
		return nil
	}
	enabled := "0"
	if uci {
		enabled = "1"
	}
	return b.configFileCall(ctx, sid, "applyConfigFiles", map[string]string{"action": action, "uci": enabled, "timeout": fmt.Sprint(timeout)}, nil)
}

// discardFiles drops one family's staged files, leaving the others staged.
func (b *NativeBackend) discardFiles(ctx context.Context, sid, family string) error {
	if b.configFileCall == nil {
		return nil
	}
	return b.configFileCall(ctx, sid, "applyConfigFiles", map[string]string{"action": "discard", "family": family, "uci": "0", "timeout": "30"}, nil)
}
func (b *NativeBackend) StageConfigFile(ctx context.Context, sid, path, expected, content string) error {
	if b.configFileCall == nil {
		return fmt.Errorf("file staging unavailable")
	}
	var directories []string
	// dnsmasq's jail only mounts its configured include directory. Stage that
	// setting with the first include file so it also works on bare metal.
	if strings.HasPrefix(path, "/etc/dnsmasq.d/") {
		cfg, err := b.UCIConfig(ctx, sid, "dhcp")
		if err != nil {
			return err
		}
		found := false
		for section, raw := range cfg {
			values, ok := raw.(map[string]any)
			if !ok || values[".type"] != "dnsmasq" {
				continue
			}
			found = true
			dir, _ := values["confdir"].(string)
			if dir != "" && dir != "/etc/dnsmasq.d" {
				return fmt.Errorf("the resolver uses a different custom options directory")
			}
			if dir == "" {
				directories = append(directories, section)
			}
		}
		if !found {
			return fmt.Errorf("local resolver configuration unavailable")
		}
	}
	// Validate and stage the file before changing UCI. A stale editor, invalid
	// filename, or dnsmasq syntax error must not leave a confdir change behind.
	if err := b.configFileCall(ctx, sid, "stageConfigFile", map[string]string{"path": path, "expected": expected, "content": content}, nil); err != nil {
		return err
	}
	for _, section := range directories {
		if err := b.UCISet(ctx, sid, "dhcp", section, map[string]any{"confdir": "/etc/dnsmasq.d"}); err != nil {
			return err
		}
	}
	return nil
}
func (b *NativeBackend) applyWithFiles(ctx context.Context, sid string, timeout int) error {
	files, err := b.configFileState(ctx, sid)
	if err != nil {
		return err
	}
	pending := false
	for _, f := range files.Files {
		pending = pending || f.Pending
	}
	if !pending {
		return b.uciApply(ctx, sid, timeout)
	}
	changes, err := b.uciChanges(ctx, sid)
	if err != nil {
		return err
	}
	uci := false
	for _, list := range changes {
		uci = uci || len(list) > 0
	}
	if err := b.fileAction(ctx, sid, "apply", uci, timeout); err != nil {
		return err
	}
	if uci {
		if err := b.uciApply(ctx, sid, timeout); err != nil {
			// A cancelled HTTP request must not cancel restoration of files already written.
			restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), helperCallTimeout)
			defer cancel()
			if restoreErr := b.fileAction(restoreCtx, sid, "abort", false, timeout); restoreErr != nil {
				return fmt.Errorf("uci apply: %w; file rollback: %v", err, restoreErr)
			}
			return err
		}
	}
	return nil
}
func (b *NativeBackend) confirmWithFiles(ctx context.Context, sid string) error {
	files, err := b.configFileState(ctx, sid)
	if err != nil {
		return err
	}
	if !files.Active {
		return b.uciConfirm(ctx, sid)
	}
	if files.UCI && !files.UCIConfirmed {
		if err := b.uciConfirm(ctx, sid); err != nil {
			return err
		}
		if err := b.fileAction(ctx, sid, "uci-confirmed", true, 30); err != nil {
			return err
		}
	}
	return b.fileAction(ctx, sid, "confirm", files.UCI, 30)
}
