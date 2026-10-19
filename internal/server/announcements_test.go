// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/events"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// announcing is a shell with acme installed and history kept, taking from a
// spool whose announcements the test's account writes, standing in for root.
func announcing(t *testing.T) (*Server, *history.Store, string) {
	t.Helper()
	s := newServerWith(t, fakeBackend{access: true}, &fakeTransport{}, []plugin.Manifest{acmeEvents()})
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore = store
	s.announcerUID = os.Getuid()
	return s, store, t.TempDir()
}

// leave puts an announcement in the spool as root's writer names it.
func leave(t *testing.T, dir string, at int64, body string) string {
	t.Helper()
	name := fmt.Sprintf("%020d-1-0000abcd.json", at)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

func announced(kind string, at int64, args ...string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	return fmt.Sprintf(`{"v":1,"time":%d,"kind":%q,"args":[%s]}`, at, kind, strings.Join(quoted, ","))
}

func recordedEvents(t *testing.T, store *history.Store) []history.Event {
	t.Helper()
	got, _, err := store.EventsAfter(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// logged is what the shell logs while fn runs.
func logged(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	fn()
	log.SetOutput(prev)
	return buf.String()
}

// TestAnnouncementsAreRecordedOldestFirst: the shell records what waits in
// the spool in the order it was announced, at the time it was announced, and
// lets each go.
func TestAnnouncementsAreRecordedOldestFirst(t *testing.T) {
	s, store, dir := announcing(t)
	later := time.Now().Add(-time.Minute).UnixNano()
	earlier := later - int64(time.Second)
	leave(t, dir, later, announced("acme.renewed", later, "second.example.com"))
	leave(t, dir, earlier, announced("acme.renewed", earlier, "first.example.com"))
	s.takeAnnouncements(t.Context(), store, dir)
	got := recordedEvents(t, store)
	if len(got) != 2 || !strings.Contains(got[0].Detail, "first.example.com") || !strings.Contains(got[1].Detail, "second.example.com") {
		t.Fatalf("recorded %+v", got)
	}
	if !got[0].Time.Equal(time.Unix(0, earlier).Truncate(time.Second)) {
		t.Errorf("time = %v, want the announcement's", got[0].Time)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left %v", entries)
	}
}

// TestAnAnnouncementThatIsNotRootsIsLetGo: one not written by root, a link,
// one too large, one of another format, or of a kind no plugin declares, is
// let go unrecorded, with the reason logged.
func TestAnAnnouncementThatIsNotRootsIsLetGo(t *testing.T) {
	at := time.Now().UnixNano()
	for name, c := range map[string]struct {
		body, reason string
		owner        int
		link         bool
	}{
		"another account's":  {announced("acme.renewed", at, "x"), "not root", os.Getuid() + 1, false},
		"a link":             {announced("acme.renewed", at, "x"), "", os.Getuid(), true},
		"too large":          {`{"v":1,"time":1,"kind":"acme.renewed","args":["` + strings.Repeat("a", 9000) + `"]}`, "over", os.Getuid(), false},
		"another format":     {`{"v":2,"time":1,"kind":"acme.renewed"}`, "format", os.Getuid(), false},
		"an undeclared kind": {announced("acme.expired", at), "acme.expired", os.Getuid(), false},
		"a core kind":        {announced("wan.down", at), "wan.down", os.Getuid(), false},
	} {
		s, store, dir := announcing(t)
		s.announcerUID = c.owner
		spooled := leave(t, dir, at, c.body)
		if c.link {
			target := filepath.Join(t.TempDir(), "target.json")
			if err := os.Rename(filepath.Join(dir, spooled), target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, spooled)); err != nil {
				t.Fatal(err)
			}
		}
		out := logged(t, func() { s.takeAnnouncements(t.Context(), store, dir) })
		if got := recordedEvents(t, store); len(got) != 0 {
			t.Errorf("%s: recorded %+v", name, got)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("%s: left %v", name, entries)
		}
		if !strings.Contains(out, spooled) || !strings.Contains(out, c.reason) {
			t.Errorf("%s: logged %q, want its name and %q", name, out, c.reason)
		}
	}
}

// TestAnAnnouncementWaitsWhileHistoryCannotTakeIt: an announcement the store
// cannot record stays for the next pass.
func TestAnAnnouncementWaitsWhileHistoryCannotTakeIt(t *testing.T) {
	s, store, dir := announcing(t)
	at := time.Now().UnixNano()
	leave(t, dir, at, announced("acme.renewed", at, "x"))
	_ = store.Close()
	s.takeAnnouncements(t.Context(), store, dir)
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("spool = %v, want the announcement kept", entries)
	}
}

// TestAnAnnouncementLeftBehindIsRecordedOnce: a shell that recorded an
// announcement and stopped before letting it go records it once.
func TestAnAnnouncementLeftBehindIsRecordedOnce(t *testing.T) {
	s, store, dir := announcing(t)
	at := time.Now().UnixNano()
	name := leave(t, dir, at, announced("acme.renewed", at, "x"))
	s.takeAnnouncements(t.Context(), store, dir)
	leave(t, dir, at, announced("acme.renewed", at, "x"))
	s.takeAnnouncements(t.Context(), store, dir)
	if got := recordedEvents(t, store); len(got) != 1 || !strings.Contains(got[0].Detail, name) {
		t.Errorf("recorded %+v, want one under %s", got, name)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left %v", entries)
	}
}

// TestAnUpgradesNewEventIsRecordedBeforeTheRescan: an event a plugin's upgrade
// declares is recorded from its registry on disk before the shell reads its
// plugins again.
func TestAnUpgradesNewEventIsRecordedBeforeTheRescan(t *testing.T) {
	s, store, dir := announcing(t)
	plugins := t.TempDir()
	if err := os.MkdirAll(filepath.Join(plugins, "acme"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry := `{"schema_version":1,"commands":[{"name":"state","kind":"read","version":1,"exec":"/x"}],` +
		`"events":[{"name":"issued","sentence":"A certificate for %s was issued.","tone":"success","read":"state"}]}`
	if err := os.WriteFile(filepath.Join(plugins, "acme", "commands.json"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
	s.SetPluginsDir(plugins)
	at := time.Now().UnixNano()
	leave(t, dir, at, announced("acme.issued", at, "x"))
	s.takeAnnouncements(t.Context(), store, dir)
	if got := recordedEvents(t, store); len(got) != 1 || got[0].Kind != "acme.issued" {
		t.Errorf("recorded %+v", got)
	}
}

// TestAnnouncementsAreTakenWhileAnApplyWaits: an event is no reading of
// config, so an apply's rollback window, which holds config readings back,
// does not hold it.
func TestAnnouncementsAreTakenWhileAnApplyWaits(t *testing.T) {
	s, store, dir := announcing(t)
	s.applies.by = map[string]pendingApply{"sid": {started: time.Now()}}
	if !s.applyPending() {
		t.Fatal("no apply is pending")
	}
	at := time.Now().UnixNano()
	leave(t, dir, at, announced("acme.renewed", at, "x"))
	s.takeAnnouncements(t.Context(), store, dir)
	if got := recordedEvents(t, store); len(got) != 1 {
		t.Errorf("recorded %+v while an apply waits", got)
	}
}

// TestAPluginsSaidEventIsKeptForItsReaders: a recorded event declared said is
// kept for each signed-in session rpcd grants its read; the router's own, and
// one declared quiet, for none.
func TestAPluginsSaidEventIsKeptForItsReaders(t *testing.T) {
	s, store, dir := announcing(t)
	quiet := acmeEvents()
	quiet.Commands.Events = append(quiet.Commands.Events, plugin.EventDeclaration{Name: "checked", Sentence: "Checked.", Tone: "info", Read: "state"})
	s.kinds = events.NewTable([]plugin.Manifest{quiet})
	reader := s.sessions.CreateWithMetadata("sid-reader", "root", "", "")
	other := s.sessions.CreateWithMetadata("sid-other", "guest", "", "")
	s.backend = fakeBackend{grantSID: func(sid, scope, object, function string) bool {
		return sid == "sid-reader" || scope != "verso-plugin"
	}}
	at := time.Now().UnixNano()
	leave(t, dir, at, announced("acme.renewed", at, "router.example.com"))
	leave(t, dir, at+1, announced("acme.checked", at+1))
	leave(t, dir, at+2, announced(events.CertificateReplaced, at+2, "router.example.com", "2027-01-17"))
	s.takeAnnouncements(t.Context(), store, dir)
	if got := s.sessions.told(reader, 0); len(got) != 1 || got[0].Event.Kind != "acme.renewed" || got[0].ID == 0 {
		t.Errorf("reader was told %+v, want acme's renewal alone", got)
	}
	if got := s.sessions.told(other, 0); len(got) != 0 {
		t.Errorf("a session without acme's read was told %+v", got)
	}
	if got := recordedEvents(t, store); len(got) != 3 {
		t.Errorf("recorded %+v, want all three", got)
	}
}

// TestTheSpoolsNamesAreTheWritersNames: the taker reads exactly the names the
// writer gives.
func TestTheSpoolsNamesAreTheWritersNames(t *testing.T) {
	if !events.IsSpoolName(fmt.Sprintf("%020d-1-0000abcd.json", 5)) || events.IsSpoolName(".x.json") {
		t.Error("the spool's names are not the writer's")
	}
}
