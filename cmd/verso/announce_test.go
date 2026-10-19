// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/events"
)

// announceWriter is a writer as root into a sound spool, acme installed with
// its renewal declared.
func announceWriter(t *testing.T) events.Writer {
	t.Helper()
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins", "acme")
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := `{"schema_version":1,"commands":[{"name":"state","kind":"read","version":1,"exec":"/usr/bin/verso-plugin-acme"}],` +
		`"events":[{"name":"renewed","sentence":"The certificate for %s was renewed.","tone":"success","read":"state","notify":true}]}`
	if err := os.WriteFile(filepath.Join(plugins, "commands.json"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(root, "events")
	if err := os.Mkdir(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	return events.Writer{Dir: spool, PluginsDir: filepath.Join(root, "plugins"), ShellUID: os.Getuid(),
		EUID: func() int { return 0 }, Now: time.Now}
}

// ran is what a fake runner was asked to run.
type ran struct{ argv [][]string }

func (r *ran) run(err error) func(context.Context, []string) error {
	return func(_ context.Context, argv []string) error {
		r.argv = append(r.argv, argv)
		return err
	}
}

// TestAnnounceLeavesTheEventAndTellsProcd: `verso announce` writes the event
// for the shell, then sends it to procd as the service event other plugins'
// triggers answer, its data the subject and the arguments.
func TestAnnounceLeavesTheEventAndTellsProcd(t *testing.T) {
	w := announceWriter(t)
	var r ran
	var stderr bytes.Buffer
	if err := announce([]string{"acme.renewed", "--subject", "eth0", "router.example.com"}, w, r.run(nil), &stderr); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(w.Dir); len(entries) != 1 {
		t.Fatalf("spool = %v", entries)
	}
	if len(r.argv) != 1 || strings.Join(r.argv[0][:4], " ") != "/bin/ubus call service event" {
		t.Fatalf("ran %q", r.argv)
	}
	var msg struct {
		Type string
		Data struct {
			Subject string
			Args    []string
		}
	}
	if err := json.Unmarshal([]byte(r.argv[0][4]), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "verso.acme.renewed" || msg.Data.Subject != "eth0" || len(msg.Data.Args) != 1 || msg.Data.Args[0] != "router.example.com" {
		t.Errorf("service event = %+v", msg)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// TestAnnounceTakesTheSDKsArguments: the SDK ends the flags with "--", so an
// argument beginning with a dash is an argument.
func TestAnnounceTakesTheSDKsArguments(t *testing.T) {
	w := announceWriter(t)
	var r ran
	if err := announce([]string{"acme.renewed", "--subject", "eth0", "--", "-x"}, w, r.run(nil), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(r.argv) != 1 || !strings.Contains(r.argv[0][4], `"subject":"eth0","args":["-x"]`) {
		t.Errorf("ran %q", r.argv)
	}
}

// TestAnnounceRefusesWithItsReason: a refused announcement is an error naming
// why, procd is not told, and nothing waits.
func TestAnnounceRefusesWithItsReason(t *testing.T) {
	w := announceWriter(t)
	var r ran
	err := announce([]string{"acme.expired"}, w, r.run(nil), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "declare") {
		t.Errorf("err = %v", err)
	}
	if len(r.argv) != 0 {
		t.Errorf("procd was told of a refused event: %q", r.argv)
	}
	if err := announce(nil, w, r.run(nil), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("no kind: err = %v", err)
	}
}

// TestAnnounceStandsWhenProcdIsNotTold: the event is recorded whether or not
// procd answers, so a failed send is said and the announcement stands.
func TestAnnounceStandsWhenProcdIsNotTold(t *testing.T) {
	w := announceWriter(t)
	var r ran
	var stderr bytes.Buffer
	if err := announce([]string{"acme.renewed", "router.example.com"}, w, r.run(errors.New("ubus: not found")), &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "procd") || !strings.Contains(stderr.String(), "not found") {
		t.Errorf("stderr = %q, want the failed send said", stderr.String())
	}
	if entries, _ := os.ReadDir(w.Dir); len(entries) != 1 {
		t.Errorf("spool = %v, want the announcement kept", entries)
	}
}
