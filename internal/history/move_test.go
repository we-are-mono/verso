// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// kept is a history on disk in a directory of its own, holding one event,
// closed as a stopping shell leaves it.
func kept(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(ctx, Event{Time: time.Unix(1_800_000_000, 0), Kind: "boot", Subject: "kept"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// names are the files dir holds.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// TestMoveCarriesTheHistoryAndLeavesNoneBehind: a history moved to another
// directory opens there whole, private to the shell, and the directory it
// came from keeps none of its files, so there is one history and a reset that
// reaches where it is reaches all of it (ADR-019, ADR-021).
func TestMoveCarriesTheHistoryAndLeavesNoneBehind(t *testing.T) {
	from, to := kept(t), t.TempDir()
	if err := os.WriteFile(filepath.Join(from, "notes.txt"), []byte("the owner's"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := Move(from, to)
	if err != nil || !moved {
		t.Fatalf("Move = %v, %v, want moved", moved, err)
	}
	if got := names(t, from); !slices.Equal(got, []string{"notes.txt"}) {
		t.Errorf("the directory moved from holds %q, want only the owner's file", got)
	}
	if got := names(t, to); !slices.Equal(got, []string{"history.db"}) {
		t.Errorf("the directory moved to holds %q, want the history alone", got)
	}
	if info, err := os.Stat(filepath.Join(to, "history.db")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the moved history is %v (%v), want 0600", info.Mode(), err)
	}
	store, err := Open(filepath.Join(to, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if e, found, err := store.Latest(ctx, "boot"); err != nil || !found || e.Subject != "kept" {
		t.Errorf("the moved history holds %+v (%v, %v), want the kept event", e, found, err)
	}
	// retention gives back what it deletes only to a file kept incremental
	var vacuum int
	if err := store.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&vacuum); err != nil || vacuum != 2 {
		t.Errorf("the moved history's auto_vacuum = %d (%v), want 2 (incremental)", vacuum, err)
	}
}

// TestMoveTakesWhatTheWALHolds: a shell that did not stop cleanly leaves
// writes in the WAL beside the file; the move carries them too.
func TestMoveTakesWhatTheWALHolds(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	store, err := Open(filepath.Join(from, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err := store.Record(ctx, Event{Time: time.Unix(1_800_000_000, 0), Kind: "boot", Subject: "in the wal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(from, "history.db-wal")); err != nil {
		t.Fatalf("the write is not in a WAL: %v", err)
	}
	if moved, err := Move(from, to); err != nil || !moved {
		t.Fatalf("Move = %v, %v", moved, err)
	}
	carried, err := Open(filepath.Join(to, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = carried.Close() }()
	if e, found, err := carried.Latest(ctx, "boot"); err != nil || !found || e.Subject != "in the wal" {
		t.Errorf("the moved history holds %+v (%v, %v), want the WAL's event", e, found, err)
	}
}

// TestMoveLeavesBothWhereTheDirectoryHoldsAHistory: a directory that holds a
// history of its own already (one left there earlier, a stick from another
// router) is not written over, and the history moved from stays where it is.
func TestMoveLeavesBothWhereTheDirectoryHoldsAHistory(t *testing.T) {
	from, to := kept(t), kept(t)
	moved, err := Move(from, to)
	if moved || err == nil {
		t.Fatalf("Move = %v, %v, want a refusal", moved, err)
	}
	for _, dir := range []string{from, to} {
		if got := names(t, dir); !slices.Equal(got, []string{"history.db"}) {
			t.Errorf("%s holds %q, want its history untouched", dir, got)
		}
	}
}

// TestMoveOverAnEmptyFileOrAnInterruptedCopy: an empty history.db (a start
// that made the file and failed) holds no history, and a copy a stop cut
// short is begun again.
func TestMoveOverAnEmptyFileOrAnInterruptedCopy(t *testing.T) {
	from, to := kept(t), t.TempDir()
	if err := os.WriteFile(filepath.Join(to, "history.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(to, "history.db"+MovingSuffix), []byte("half a copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if moved, err := Move(from, to); err != nil || !moved {
		t.Fatalf("Move = %v, %v, want moved", moved, err)
	}
	if got := names(t, to); !slices.Equal(got, []string{"history.db"}) {
		t.Errorf("the directory moved to holds %q, want the history alone", got)
	}
}

// TestMoveWithNothingToMove: no directory named, the same directory however
// it is spelled, or one without a history, moves nothing and refuses nothing.
func TestMoveWithNothingToMove(t *testing.T) {
	dir := kept(t)
	for _, from := range []string{"", dir, dir + "/", dir + "/./", t.TempDir(), filepath.Join(t.TempDir(), "gone")} {
		if moved, err := Move(from, dir); moved || err != nil {
			t.Errorf("Move(%q) = %v, %v, want nothing", from, moved, err)
		}
	}
	if got := names(t, dir); !slices.Equal(got, []string{"history.db"}) {
		t.Errorf("the history moved nowhere is %q", got)
	}
}

// TestMoveThatFailsLeavesTheHistoryWhereItWas: a directory that cannot be
// written takes nothing, and the history stays whole where it was.
func TestMoveThatFailsLeavesTheHistoryWhereItWas(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes a directory whatever its mode")
	}
	from, to := kept(t), t.TempDir()
	if err := os.Chmod(to, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(to, 0o700) })
	moved, err := Move(from, to)
	if moved || err == nil {
		t.Fatalf("Move = %v, %v, want a failure", moved, err)
	}
	if got := names(t, from); !slices.Equal(got, []string{"history.db"}) {
		t.Errorf("the history after a failed move is %q", got)
	}
	if _, err := os.Stat(filepath.Join(to, "history.db"+MovingSuffix)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a failed move left its copy: %v", err)
	}
}
