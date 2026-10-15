// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func writePasswd(t *testing.T, body string) PasswdAccounts {
	t.Helper()
	path := filepath.Join(t.TempDir(), "passwd")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return PasswdAccounts{Path: path}
}

const routerPasswd = `root:x:0:0:root:/root:/bin/ash
verso:x:6000:6000:verso:/var/run/verso:/bin/false
verso-plugin-systemd:x:7001:6000:not the system plugin:/var/run/verso:/bin/false
verso-plugin-system:x:6005:6000:Verso system plugin:/var/run/verso:/bin/false
verso-plugin-dnsdhcp:x:6011:6000:Verso DNS and DHCP plugin:/var/run/verso:/bin/false
verso-plugin-broken:x:root:6000::/var/run/verso:/bin/false
verso-plugin-short:x
`

// TestPasswdAccountsNameEachPluginsOwnAccount: a plugin runs as
// verso-plugin-<id>, and only that exact name answers for it — not a longer
// name that starts the same, and not the shell's own verso account.
func TestPasswdAccountsNameEachPluginsOwnAccount(t *testing.T) {
	accounts := writePasswd(t, routerPasswd)
	for id, want := range map[string]uint32{"system": 6005, "dnsdhcp": 6011} {
		got, err := accounts.UID(id)
		if err != nil || got != want {
			t.Errorf("UID(%q) = %d, %v; want %d", id, got, err, want)
		}
	}
}

// TestPasswdAccountsTakeTheFirstOfADuplicatedName: when the file names a
// plugin's account twice, the first entry is its account, as musl's getpwnam
// answers, so the uid checked is the one the plugin's process is started as.
func TestPasswdAccountsTakeTheFirstOfADuplicatedName(t *testing.T) {
	accounts := writePasswd(t, routerPasswd+
		"verso-plugin-system:x:7005:6000:a second entry:/var/run/verso:/bin/false\n")
	if got, err := accounts.UID("system"); err != nil || got != 6005 {
		t.Errorf("UID(system) = %d, %v; want the first entry's 6005", got, err)
	}
}

// TestPasswdAccountsRefuseWhatNamesNoUID: a plugin with no entry, or one whose
// entry carries no number where the uid belongs, has no account the shell
// could check a listener against.
func TestPasswdAccountsRefuseWhatNamesNoUID(t *testing.T) {
	accounts := writePasswd(t, routerPasswd)
	for _, id := range []string{"wireless", "broken", "short", ""} {
		if uid, err := accounts.UID(id); err == nil {
			t.Errorf("UID(%q) = %d, want an error", id, uid)
		}
	}
	if uid, err := (PasswdAccounts{Path: filepath.Join(t.TempDir(), "absent")}).UID("system"); err == nil {
		t.Errorf("UID from a missing file = %d, want an error", uid)
	}
}
