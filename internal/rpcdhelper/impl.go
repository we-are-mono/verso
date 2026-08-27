// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package rpcdhelper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/we-are-mono/verso/internal/sysstat"
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

// ApkPackageManager wraps the apk CLI (the 25.12 target's package manager,
// ADR-011 §4). Callers validated names/queries already; everything here still
// passes single argv words — never a shell — so nothing can be smuggled.
type ApkPackageManager struct{}

// apkIndexDir holds the fetched feed indexes; the freshest mtime is the
// honest "checked N ago" for the Discover face.
const apkIndexDir = "/var/cache/apk"

// Update refreshes the feed indexes.
func (ApkPackageManager) Update() error {
	if out, err := exec.Command("apk", "update").CombinedOutput(); err != nil {
		return fmt.Errorf("apk update: %w: %s", err, tail(out))
	}
	return nil
}

// apkListLine reads one `apk list` row: name-version, arch, {origin},
// (license), and the optional [installed] marker. The name/version split is at
// the last dash that starts a digit — the apk convention.
var apkListLine = regexp.MustCompile(`^(\S+)-([0-9][^ ]*) \S+ \{([^}]*)\}.*?(\[installed\])?$`)

// Search lists packages whose name contains query, capped at limit, with
// descriptions filled from apk info. The full match count comes back so the
// caller can say "showing 30 of 214".
func (ApkPackageManager) Search(query string, limit int) ([]Package, int, error) {
	out, err := exec.Command("apk", "list", "*"+query+"*").Output()
	if err != nil {
		return nil, 0, fmt.Errorf("apk list: %w", err)
	}
	var found []Package
	total := 0
	for _, line := range strings.Split(string(out), "\n") {
		m := apkListLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		total++
		if len(found) >= limit {
			continue
		}
		found = append(found, Package{
			Name:      m[1],
			Version:   m[2],
			Feed:      feedOf(m[3]),
			Installed: m[4] != "",
		})
	}
	for i := range found {
		found[i].Description = apkDescription(found[i].Name)
	}
	return found, total, nil
}

// Installed lists every installed package with its detail — one structured
// exec (`apk query --format json`), so the full set with descriptions,
// licenses, and sizes costs the same as a bare listing.
func (ApkPackageManager) Installed() ([]Package, error) {
	out, err := exec.Command("apk", "query", "--installed", "--format", "json", "*").Output()
	if err != nil {
		return nil, fmt.Errorf("apk query --installed: %w", err)
	}
	var raw []struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		License     string `json:"license"`
		Origin      string `json:"origin"`
		URL         string `json:"url"`
		FileSize    int64  `json:"file-size"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("apk query: %w", err)
	}
	found := make([]Package, 0, len(raw))
	for _, p := range raw {
		found = append(found, Package{
			Name: p.Name, Version: p.Version, Feed: feedOf(p.Origin),
			Description: p.Description, License: p.License, Webpage: p.URL,
			Size: p.FileSize, Installed: true,
		})
	}
	return found, nil
}

// Install adds one package; Remove deletes one. Both return the tool's output
// tail so the surface can show what actually happened.
func (ApkPackageManager) Install(name string) (string, error) {
	out, err := exec.Command("apk", "add", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("apk add %s: %w: %s", name, err, tail(out))
	}
	return tail(out), nil
}

func (ApkPackageManager) Remove(name string) (string, error) {
	out, err := exec.Command("apk", "del", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("apk del %s: %w: %s", name, err, tail(out))
	}
	return tail(out), nil
}

// CheckedAt is the freshest feed index's mtime; zero when none exists yet.
func (ApkPackageManager) CheckedAt() int64 {
	entries, err := os.ReadDir(apkIndexDir)
	if err != nil {
		return 0
	}
	var newest int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Unix() > newest {
			newest = info.ModTime().Unix()
		}
	}
	return newest
}

// apkDescription reads one package's description (`apk info -d`, whose output
// is a "name description:" header line then the text); a read failure is an
// empty description, never a failed search.
func apkDescription(name string) string {
	out, err := exec.Command("apk", "info", "-d", name).Output()
	if err != nil {
		return ""
	}
	lines := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimSpace(lines[1])
}

// feedOf reads the feed name out of apk's origin path ("feeds/packages/…" →
// "packages"); the base target feed reads as "base".
func feedOf(origin string) string {
	parts := strings.Split(origin, "/")
	if len(parts) >= 2 && parts[0] == "feeds" {
		return parts[1]
	}
	return "base"
}

// tail is the last few lines of a tool's output — enough to be honest,
// bounded enough to travel over the bus.
func tail(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > 400 {
		s = "…" + s[len(s)-400:]
	}
	return s
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

// SysTrafficReader reads the kernel's conntrack table — a root-only file —
// and hands back per-address totals (sysstat owns the parsing and the fold).
type SysTrafficReader struct{}

func (SysTrafficReader) ConnStats() (map[string]sysstat.DeviceTraffic, error) {
	raw, err := sysstat.ConntrackDump()
	if err != nil {
		return nil, err
	}
	return sysstat.AggregateByAddress(sysstat.ParseConntrack(raw)), nil
}
