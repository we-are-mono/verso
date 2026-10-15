// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Accounts names the uid each plugin runs as: the one account the shell talks
// to behind that plugin's socket (ADR-007). It is the transport's injected seam
// (ADR-003), so the peer check is testable against any listener.
type Accounts interface {
	UID(pluginID string) (uint32, error)
}

// PasswdAccounts resolves a plugin's account from a passwd(5) file. Every
// plugin runs as its own system account, verso-plugin-<id>, which its init
// script, as root, writes there; nothing a plugin runs as can change the file.
// The file is read on every lookup, so an account added or renumbered after
// the shell started is the one checked.
type PasswdAccounts struct {
	Path string
}

// accountName is the system account a plugin runs as.
func accountName(pluginID string) string { return "verso-plugin-" + pluginID }

// UID is the uid of verso-plugin-<pluginID> in the file. A plugin with no
// entry, or an entry with no number for its uid, has no account to check a
// listener against, which is an error.
func (p PasswdAccounts) UID(pluginID string) (uint32, error) {
	file, err := os.Open(p.Path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	name := accountName(pluginID)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if fields[0] != name {
			continue
		}
		if len(fields) < 3 {
			return 0, fmt.Errorf("account %s in %s has no uid", name, p.Path)
		}
		uid, err := strconv.ParseUint(fields[2], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("account %s in %s: uid %q: %w", name, p.Path, fields[2], err)
		}
		return uint32(uid), nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("no account %s in %s", name, p.Path)
}
