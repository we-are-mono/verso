// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package events

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// acmeRegistry is acme's commands.json, declaring a renewal of one argument.
const acmeRegistry = `{"schema_version":1,"commands":[{"name":"state","kind":"read","version":1,"exec":"/usr/bin/verso-plugin-acme"}],` +
	`"events":[{"name":"renewed","sentence":"The certificate for %s was renewed.","tone":"success","read":"state","notify":true}]}`

// spoolWriter is a writer as root, into a sound spool owned by the test's
// account standing in for the shell's, with acme installed.
func spoolWriter(t *testing.T) Writer {
	t.Helper()
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	if err := os.MkdirAll(filepath.Join(plugins, "acme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugins, "acme", "commands.json"), []byte(acmeRegistry), 0o644); err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(root, "events")
	if err := os.Mkdir(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	return Writer{
		Dir: spool, PluginsDir: plugins, ShellUID: os.Getuid(),
		EUID: func() int { return 0 },
		Now:  func() time.Time { return time.Unix(1_800_000_000, 5) },
	}
}

// waiting is the announcements in the spool, by name.
func waiting(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestAnAnnouncementIsWrittenForTheShell: an accepted announcement is one
// file in the spool, readable by the shell's account, under a name that
// sorts by when it was announced, and reads back field for field.
func TestAnAnnouncementIsWrittenForTheShell(t *testing.T) {
	w := spoolWriter(t)
	a, err := w.Announce("acme.renewed", "", []string{"router.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	names := waiting(t, w.Dir)
	if len(names) != 1 || !regexp.MustCompile(`^01800000000000000005-\d+-[0-9a-f]{8}\.json$`).MatchString(names[0]) {
		t.Fatalf("spool = %v", names)
	}
	info, err := os.Stat(filepath.Join(w.Dir, names[0]))
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, %v; the shell's account reads what root wrote", info.Mode(), err)
	}
	data, err := os.ReadFile(filepath.Join(w.Dir, names[0]))
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseAnnouncement(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.Kind != "acme.renewed" || back.Time != 1_800_000_000_000_000_005 || back.Subject != "" ||
		len(back.Args) != 1 || back.Args[0] != "router.example.com" || !reflect.DeepEqual(back, a) {
		t.Errorf("read back %+v, want %+v", back, a)
	}
	if _, err := w.Announce("acme.renewed", "", []string{"router.example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := waiting(t, w.Dir); len(got) != 2 {
		t.Errorf("two announcements in one nanosecond = %v, want both", got)
	}
}

// TestAnAnnouncementIsRefusedAndLeavesNothing: each announcement that breaks
// a rule is refused with its reason, and nothing is written.
func TestAnAnnouncementIsRefusedAndLeavesNothing(t *testing.T) {
	for name, c := range map[string]struct {
		kind, subject string
		args          []string
		euid          int
		reason        string
	}{
		"not root":               {"acme.renewed", "", []string{"x"}, 6000, "root"},
		"undeclared":             {"acme.expired", "", nil, 0, "declare"},
		"plugin not installed":   {"ddns.updated", "", []string{"x"}, 0, "declare"},
		"no owner":               {"renewed", "", nil, 0, "kind"},
		"a core kind":            {"wan.down", "", nil, 0, "announce"},
		"too few arguments":      {"acme.renewed", "", nil, 0, "argument"},
		"too many arguments":     {"acme.renewed", "", []string{"a", "b"}, 0, "argument"},
		"a ninth argument":       {"acme.renewed", "", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, 0, "argument"},
		"a long argument":        {"acme.renewed", "", []string{strings.Repeat("a", 257)}, 0, "256"},
		"a newline":              {"acme.renewed", "", []string{"a\nb"}, 0, "control"},
		"a tab":                  {"acme.renewed", "", []string{"a\tb"}, 0, "control"},
		"a subject with a slash": {"acme.renewed", "eth0/1", []string{"x"}, 0, "device"},
		"a subject too long":     {"acme.renewed", "abcdefghijklmnop", []string{"x"}, 0, "device"},
	} {
		w := spoolWriter(t)
		w.EUID = func() int { return c.euid }
		_, err := w.Announce(c.kind, c.subject, c.args)
		if err == nil || !strings.Contains(err.Error(), c.reason) {
			t.Errorf("%s: err = %v, want one naming %q", name, err, c.reason)
		}
		if got := waiting(t, w.Dir); len(got) != 0 {
			t.Errorf("%s: left %v", name, got)
		}
	}
}

// TestTheCertificatesReplacementIsAnnouncedAsCore: the one core kind root
// announces is held to its own sentence.
func TestTheCertificatesReplacementIsAnnouncedAsCore(t *testing.T) {
	w := spoolWriter(t)
	if _, err := w.Announce(CertificateReplaced, "", []string{"router.example.com", "2027-01-17"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Announce(CertificateReplaced, "", []string{"router.example.com"}); err == nil {
		t.Error("the replacement with one argument of two was accepted")
	}
}

// TestASpoolThatIsNotTheShellsIsRefused: root writes only into the shell's own
// directory, reached through a descriptor it checked, so a link or another
// directory at the spool's name leads it nowhere.
func TestASpoolThatIsNotTheShellsIsRefused(t *testing.T) {
	w := spoolWriter(t)
	elsewhere := t.TempDir()
	if err := os.Remove(w.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, w.Dir); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Announce("acme.renewed", "", []string{"x"}); err == nil {
		t.Error("a link at the spool's name was written through")
	}
	if got := waiting(t, elsewhere); len(got) != 0 {
		t.Errorf("the link's target holds %v", got)
	}

	w = spoolWriter(t)
	w.ShellUID = os.Getuid() + 1
	if _, err := w.Announce("acme.renewed", "", []string{"x"}); err == nil || !strings.Contains(err.Error(), "shell") {
		t.Errorf("a spool another account owns: err = %v", err)
	}

	w = spoolWriter(t)
	if err := os.Chmod(w.Dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Announce("acme.renewed", "", []string{"x"}); err == nil {
		t.Error("a spool its group may enter was written into")
	}

	w = spoolWriter(t)
	if err := os.Remove(w.Dir); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Announce("acme.renewed", "", []string{"x"}); err == nil {
		t.Error("a missing spool was made or written")
	}
}

// TestAFullSpoolRefusesAndKeepsWhatWaits: 256 waiting is the most; the next
// is refused and those waiting stay.
func TestAFullSpoolRefusesAndKeepsWhatWaits(t *testing.T) {
	w := spoolWriter(t)
	for i := range SpoolLimit {
		if err := os.WriteFile(filepath.Join(w.Dir, fmt.Sprintf("%020d-1-00000000.json", i)), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Announce("acme.renewed", "", []string{"x"}); err == nil || !strings.Contains(err.Error(), "256") {
		t.Errorf("err = %v, want the spool full", err)
	}
	if got := waiting(t, w.Dir); len(got) != SpoolLimit {
		t.Errorf("%d wait, want %d", len(got), SpoolLimit)
	}
}

// TestTheServiceEventCarriesTheKindSubjectAndArguments: procd hears of an
// event as verso.<owner>.<event>, its subject and arguments as its data,
// arguments an empty list rather than null.
func TestTheServiceEventCarriesTheKindSubjectAndArguments(t *testing.T) {
	argv := ServiceEvent(Announcement{Kind: "acme.renewed", Args: []string{"router.example.com"}})
	want := `{"type":"verso.acme.renewed","data":{"subject":"","args":["router.example.com"]}}`
	if strings.Join(argv[:4], " ") != "/bin/ubus call service event" || argv[4] != want {
		t.Errorf("argv = %q", argv)
	}
	if got := ServiceEvent(Announcement{Kind: "x.y", Subject: "eth0"})[4]; got != `{"type":"verso.x.y","data":{"subject":"eth0","args":[]}}` {
		t.Errorf("no arguments = %s", got)
	}
}

// TestParseAnnouncementRefusesAnotherFormat: the shell reads only the format
// it knows.
func TestParseAnnouncementRefusesAnotherFormat(t *testing.T) {
	for _, body := range []string{`{`, `{"v":2,"time":1,"kind":"acme.renewed"}`, `{"v":1,"time":1}`} {
		if _, err := ParseAnnouncement([]byte(body)); err == nil {
			t.Errorf("%s parsed", body)
		}
	}
}
