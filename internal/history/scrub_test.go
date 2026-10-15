// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

var listed = map[string]bool{"wireless.key": true, "wireless.auth_secret": true, "network.private_key": true}

// masked is a verbatim row's words for a secret, as the shell writes them.
func masked(op ChangeOp) string {
	return op.Config + ": " + op.Section + "." + op.Option + " = ••••••••"
}

// plaintextFile is a history a shell wrote with secrets in it: a Wi-Fi
// network removed whole, key and all; a listed secret changed on an
// undescribed row, its value in the row's words; a secret changed before it
// was listed, its values kept; and the configs' readings holding one.
func plaintextFile(t *testing.T) (*Store, string) {
	t.Helper()
	store, path := open(t)
	db, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`
INSERT INTO changes (id, time, status, old_read) VALUES (1, 60, 'applied', 1);
INSERT INTO change_items (change_id, position, page, verb, words, verbatim) VALUES
	(1, 0, 'Wi-Fi', 'removed', 'Removed the network “Guest”.', 0),
	(1, 1, 'Wi-Fi', 'changed', 'wireless: corp.auth_secret = RADIUS-PLAINTEXT', 1),
	(1, 2, 'Interfaces', 'changed', 'network: wg0.private_key = WG-PLAINTEXT', 1),
	(1, 3, 'Interfaces', 'removed', 'Removed the tunnel “wg1”.', 0);
INSERT INTO change_ops (change_id, item, position, config, section, op, option, value, old, secret) VALUES
	(1, 0, 0, 'wireless', 'guestnet', 'remove-section', '', NULL, '{".type":"wifi-iface","ssid":"Guest","key":"PSK-PLAINTEXT"}', 0),
	(1, 1, 0, 'wireless', 'corp', 'set', 'auth_secret', NULL, NULL, 1),
	(1, 2, 0, 'network', 'wg0', 'set', 'private_key', 'WG-PLAINTEXT', '"WG-BEFORE"', 0),
	(1, 3, 0, 'network', 'wg1', 'remove-section', '', NULL, '{".type":"interface","private_key":{"secret":"0f1e2d3c"}}', 0);
INSERT INTO config_state (config, sections, time) VALUES ('wireless', '{"corp":{".type":"wifi-iface","ssid":"Corp","key":"STATE-PLAINTEXT"}}', 60);
INSERT INTO config_start (config, sections, time) VALUES ('wireless', '{"corp":{".type":"wifi-iface","ssid":"Corp","key":["START-PLAINTEXT"]}}', 0);
`); err != nil {
		t.Fatal(err)
	}
	return store, path
}

// rawText is every text config history keeps, as one string.
func rawText(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var all string
	if err := db.QueryRow(`SELECT
		(SELECT group_concat(words, '|') FROM change_items) || '|' ||
		(SELECT group_concat(coalesce(value, '') || coalesce(old, ''), '|') FROM change_ops) || '|' ||
		(SELECT group_concat(sections, '|') FROM config_state) || '|' ||
		(SELECT group_concat(sections, '|') FROM config_start)`).Scan(&all); err != nil {
		t.Fatal(err)
	}
	return all
}

// TestScrubbingDropsEveryListedSecret: no copy of a listed secret is left,
// a secret's change is marked as one, and what no list names stays.
func TestScrubbingDropsEveryListedSecret(t *testing.T) {
	store, path := plaintextFile(t)
	n, err := store.ScrubSecrets(ctx, listed, masked)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("scrubbed nothing")
	}
	all := rawText(t, path)
	if strings.Contains(all, "PLAINTEXT") || strings.Contains(all, "WG-BEFORE") {
		t.Fatalf("left a secret:\n%s", all)
	}
	for _, kept := range []string{"Guest", "Corp", "Removed the network “Guest”.", `"secret":"0f1e2d3c"`} {
		if !strings.Contains(all, kept) {
			t.Errorf("scrubbed %q, which is no secret's value:\n%s", kept, all)
		}
	}

	c := readBack(t, store, 1)
	var held map[string]any
	if err := json.Unmarshal(c.Items[0].Ops[0].Old, &held); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(held, map[string]any{".type": "wifi-iface", "ssid": "Guest", "key": map[string]any{"secret": true}}) {
		t.Errorf("removed section = %v", held)
	}
	if c.Items[1].Words != "wireless: corp.auth_secret = ••••••••" || c.Items[2].Words != "network: wg0.private_key = ••••••••" {
		t.Errorf("words = %q, %q", c.Items[1].Words, c.Items[2].Words)
	}
	if op := c.Items[2].Ops[0]; !op.Secret || op.Value != nil || op.Old != nil {
		t.Errorf("a secret's change kept as %+v", op)
	}
	state, _, _ := store.ConfigState(ctx, "wireless")
	if string(state) != `{"corp":{".type":"wifi-iface","key":{"secret":true},"ssid":"Corp"}}` {
		t.Errorf("state = %s", state)
	}
}

// onDisk is history.db's bytes as the filesystem holds them, its write-ahead
// log's and the log's index after.
func onDisk(t *testing.T, path string) []byte {
	t.Helper()
	var all []byte
	for _, file := range []string{path, path + "-wal", path + "-shm"} {
		b, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		all = append(all, b...)
	}
	return all
}

// longPlaintextFile is plaintextFile with a long history behind it, as a
// router's is, a Wi-Fi key and a password in every entry, all of it out of
// the log and in the file; scrubbing it frees many pages. It answers the
// secrets to scrub it of.
func longPlaintextFile(t *testing.T) (*Store, string, map[string]bool) {
	t.Helper()
	store, path := plaintextFile(t)
	for i := range 500 {
		value := fmt.Sprintf("10.0.%d.%d", i/250, i%250)
		password := fmt.Sprintf("PASSWORD-PLAINTEXT-%04d", i)
		if _, err := store.RecordChange(ctx, Change{Time: time.Unix(int64(100+i), 0), Status: ChangeApplied, OldRead: true, Items: []ChangeItem{
			{Page: "Interfaces", Verb: "changed", Words: "network: lan.ipaddr = " + value, Verbatim: true, Ops: []ChangeOp{
				{Config: "network", Section: "lan", Op: "set", Option: "ipaddr", Value: &value, Old: json.RawMessage(`"192.168.1.1"`)},
			}},
			{Page: "Wi-Fi", Verb: "removed", Words: "Removed a network.", Ops: []ChangeOp{
				{Config: "wireless", Section: "guestnet", Op: "remove-section",
					Old: json.RawMessage(fmt.Sprintf(`{".type":"wifi-iface","ssid":"Guest","key":"PSK-PLAINTEXT-%04d"}`, i))},
			}},
			{Page: "Interfaces", Verb: "changed", Words: "network: wan.password = " + password, Verbatim: true, Ops: []ChangeOp{
				{Config: "network", Section: "wan", Op: "set", Option: "password", Value: &password},
			}},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	secrets := map[string]bool{"network.password": true}
	maps.Copy(secrets, listed)
	return store, path, secrets
}

// noSecretOnDisk fails t when a byte of history.db or its log holds a secret
// a scrub took out.
func noSecretOnDisk(t *testing.T, path, when string) {
	t.Helper()
	for _, needle := range []string{"PLAINTEXT", "WG-BEFORE"} {
		if n := bytes.Count(onDisk(t, path), []byte(needle)); n > 0 {
			t.Errorf("%s, history.db and its log hold %q %d times", when, needle, n)
		}
	}
}

// TestScrubbingLeavesNoSecretInTheFile: SQLite keeps what a row held in the
// pages it freed, so a scrub that wrote a row rewrites the file, and no byte
// of the file or its log holds a secret it took out.
func TestScrubbingLeavesNoSecretInTheFile(t *testing.T) {
	store, path, secrets := longPlaintextFile(t)
	if _, err := store.ScrubSecrets(ctx, secrets, masked); err != nil {
		t.Fatal(err)
	}
	noSecretOnDisk(t, path, "scrubbed")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	noSecretOnDisk(t, path, "closed")
}

// TestARewriteCutShortIsMadeByTheNextScrub: a scrub's rows commit before the
// file is rewritten, and whatever stops the rewrite (the power, a full disk,
// the scrub's time running out) leaves them scrubbed and their old values in
// the file's freed pages. The rewrite is owed in the file itself, so the next
// scrub makes it, the next start's included, though no row is left to write;
// once made, a scrub writes nothing again.
func TestARewriteCutShortIsMadeByTheNextScrub(t *testing.T) {
	store, path, secrets := longPlaintextFile(t)
	written, owed, err := store.scrubRows(ctx, secrets, masked)
	if err != nil || written == 0 || !owed {
		t.Fatalf("scrubbed %d rows, owing a rewrite %v: %v", written, owed, err)
	}
	if !bytes.Contains(onDisk(t, path), []byte("PLAINTEXT")) {
		t.Fatal("the rows' old values are out of the file before any rewrite")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	next, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	if n, err := next.ScrubSecrets(ctx, secrets, masked); err != nil || n != 0 {
		t.Fatalf("the next start's scrub wrote %d rows: %v", n, err)
	}
	noSecretOnDisk(t, path, "rewritten by the next scrub")

	made := onDisk(t, path)
	if n, err := next.ScrubSecrets(ctx, secrets, masked); err != nil || n != 0 {
		t.Fatalf("a scrub after the rewrite wrote %d rows: %v", n, err)
	}
	if !bytes.Equal(onDisk(t, path), made) {
		t.Error("a scrub after the rewrite was made wrote to the file or its log")
	}
}

// TestARewriteThatFailsSaysSoAndIsMadeAgain: a reader holding the log keeps
// the rewrite from emptying it. The scrub answers the rows it wrote and that
// the file is not rewritten, and the next scrub, with no row to write, makes
// the rewrite.
func TestARewriteThatFailsSaysSoAndIsMadeAgain(t *testing.T) {
	store, path, secrets := longPlaintextFile(t)
	if _, err := store.db.Exec(`PRAGMA busy_timeout = 0`); err != nil {
		t.Fatal(err)
	}
	reader, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	reading, err := reader.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := reading.QueryRow(`SELECT count(*) FROM change_ops`).Scan(&rows); err != nil {
		t.Fatal(err)
	}

	n, err := store.ScrubSecrets(ctx, secrets, masked)
	if n == 0 || !errors.Is(err, ErrNotRewritten) {
		t.Fatalf("a scrub whose rewrite failed answered %d rows, %v", n, err)
	}
	if err := errors.Join(reading.Rollback(), reader.Close()); err != nil {
		t.Fatal(err)
	}

	if n, err := store.ScrubSecrets(ctx, secrets, masked); err != nil || n != 0 {
		t.Fatalf("the scrub after a failed rewrite wrote %d rows: %v", n, err)
	}
	noSecretOnDisk(t, path, "rewritten by the next scrub")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	noSecretOnDisk(t, path, "closed")
}

// TestScrubbingTwiceChangesNothing: what is scrubbed stays as it is, so the
// shell scrubs every time it learns the secrets, and only a secret newly
// named is written.
func TestScrubbingTwiceChangesNothing(t *testing.T) {
	store, path := plaintextFile(t)
	if _, err := store.ScrubSecrets(ctx, listed, masked); err != nil {
		t.Fatal(err)
	}
	once, bytesOnce := rawText(t, path), onDisk(t, path)
	n, err := store.ScrubSecrets(ctx, listed, masked)
	if !bytes.Equal(onDisk(t, path), bytesOnce) {
		t.Error("a second scrub wrote to the file or its log")
	}
	if err != nil || n != 0 || rawText(t, path) != once {
		t.Fatalf("a second scrub changed %d rows, %v", n, err)
	}
}

// TestScrubbingFollowsTheListAsItGrows: a secret named after it was kept is
// scrubbed once it is named.
func TestScrubbingFollowsTheListAsItGrows(t *testing.T) {
	store, path := plaintextFile(t)
	if _, err := store.ScrubSecrets(ctx, map[string]bool{"wireless.key": true}, masked); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rawText(t, path), "WG-PLAINTEXT") {
		t.Fatal("scrubbed a secret not yet named")
	}
	if _, err := store.ScrubSecrets(ctx, listed, masked); err != nil {
		t.Fatal(err)
	}
	if all := rawText(t, path); strings.Contains(all, "PLAINTEXT") {
		t.Fatalf("left a secret:\n%s", all)
	}
}
