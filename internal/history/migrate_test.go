// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// The files in testdata were written by the shells routers ran, each by its
// own code: r11 keyed points by the series' name (version 1), r17 stored
// series once (version 2), r25 added the alerts outbox and holds one alert.
// Each holds three minutes of cpu and iface.eth0.rx and one boot event, in WAL
// mode with auto_vacuum off, as on the routers.
var shellFiles = []struct {
	name   string
	alerts int
}{
	{"history-r11.db", 0},
	{"history-r17.db", 0},
	{"history-r25.db", 1},
}

// copied puts a copy of a testdata file where a test may change it.
func copied(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "history.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// fileAt writes a history file at the version it claims, holding what setup
// creates.
func fileAt(t *testing.T, version int, setup string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(setup); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
		t.Fatal(err)
	}
	return path
}

func versionOf(t *testing.T, db *sql.DB) int {
	t.Helper()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func countOf(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestANewFileTakesEveryStep: a fresh history is at the newest version and
// holds every table the steps make.
func TestANewFileTakesEveryStep(t *testing.T) {
	store, _ := open(t)
	if got := versionOf(t, store.db); got != latest {
		t.Fatalf("a new file is at version %d, want %d", got, latest)
	}
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("no points table: %v", err)
	}
	if _, err := store.Enqueue(ctx, Alert{Created: minute(0), Title: "t"}); err != nil {
		t.Fatalf("no outbox: %v", err)
	}
}

// TestEveryFileAShellWroteReachesTheLatestWithAllItHeld: whichever shell a
// router ran, its file arrives at the latest version with every point, event
// and alert it held, and takes new ones.
func TestEveryFileAShellWroteReachesTheLatestWithAllItHeld(t *testing.T) {
	for _, file := range shellFiles {
		t.Run(file.name, func(t *testing.T) {
			store, err := Open(copied(t, file.name))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()

			if got := versionOf(t, store.db); got != latest {
				t.Fatalf("the file is at version %d, want %d", got, latest)
			}
			for _, series := range []struct {
				name  string
				scale float64
			}{{"cpu", 1}, {"iface.eth0.rx", 100}} {
				points, err := store.Points(ctx, series.name, Minute, minute(0), minute(3))
				if err != nil {
					t.Fatal(err)
				}
				if len(points) != 3 {
					t.Fatalf("%s kept %d points, want 3", series.name, len(points))
				}
				for i, p := range points {
					if p.Average != series.scale*float64(i) || p.Peak != series.scale*float64(i)+series.scale/2 {
						t.Fatalf("%s point %d = %+v", series.name, i, p)
					}
				}
			}
			if got := countOf(t, store.db, "events"); got != 1 {
				t.Fatalf("the file kept %d events, want 1", got)
			}
			if got := countOf(t, store.db, "alerts"); got != file.alerts {
				t.Fatalf("the file kept %d alerts, want %d", got, file.alerts)
			}
			if _, err := store.RecordChange(ctx, applied(minute(3))); err != nil {
				t.Fatalf("a migrated file takes no changes: %v", err)
			}

			store.Add(Point{Series: "cpu", Time: minute(3), Average: 3, Peak: 3},
				Point{Series: "temp.soc", Time: minute(3), Average: 40, Peak: 41})
			if err := store.Flush(ctx); err != nil {
				t.Fatalf("a migrated file takes no new points: %v", err)
			}
			if _, err := store.Enqueue(ctx, Alert{Created: minute(3), Title: "t"}); err != nil {
				t.Fatalf("a migrated file takes no alerts: %v", err)
			}
		})
	}
}

// TestOpeningAMigratedFileAgainChangesNothing: the steps run once; a file
// already at the latest version is opened as it is.
func TestOpeningAMigratedFileAgainChangesNothing(t *testing.T) {
	path := copied(t, "history-r11.db")
	for range 2 {
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := countOf(t, store.db, "points"); got != 6 {
			t.Fatalf("the file holds %d points, want 6", got)
		}
		if got := countOf(t, store.db, "series"); got != 2 {
			t.Fatalf("the file holds %d series, want 2", got)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestOpenLeavesAFileNewerThanItReads: a downgraded shell does not know the
// newer shape, and rebuilding would throw away what a later upgrade reads
// again; it fails, so the history is kept in memory, and the file stays.
func TestOpenLeavesAFileNewerThanItReads(t *testing.T) {
	path := fileAt(t, latest+1, `CREATE TABLE kept (x); INSERT INTO kept VALUES (1);`)

	if _, err := Open(path); err == nil {
		t.Fatal("Open succeeded on a file newer than it reads")
	}
	db, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if got := countOf(t, db, "kept"); got != 1 {
		t.Fatalf("the newer file was touched: %d rows", got)
	}
	if got := versionOf(t, db); got != latest+1 {
		t.Fatalf("the newer file's version is %d, want %d", got, latest+1)
	}
}

// TestTheShippedStepsLoad: the embedded migrations are numbered one by one
// from 1, which the shell relies on to know a file's version.
func TestTheShippedStepsLoad(t *testing.T) {
	loaded, err := loadSteps(migrations)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != latest {
		t.Fatalf("%d steps loaded, want %d", len(loaded), latest)
	}
}

// TestStepsMustBeNumberedOneByOne: a gap, a repeat or a stray name would make
// a version mean different shapes on different routers, so it never loads.
func TestStepsMustBeNumberedOneByOne(t *testing.T) {
	step := &fstest.MapFile{Data: []byte(`CREATE TABLE t (x);`)}
	for _, tc := range []struct {
		name  string
		files fstest.MapFS
		ok    bool
	}{
		{"in order", fstest.MapFS{"migrations/001_a.sql": step, "migrations/002_b.sql": step}, true},
		{"a gap", fstest.MapFS{"migrations/001_a.sql": step, "migrations/003_c.sql": step}, false},
		{"not from one", fstest.MapFS{"migrations/002_b.sql": step}, false},
		{"a repeat", fstest.MapFS{"migrations/001_a.sql": step, "migrations/001_b.sql": step}, false},
		{"no number", fstest.MapFS{"migrations/001_a.sql": step, "migrations/b.sql": step}, false},
		{"not sql", fstest.MapFS{"migrations/001_a.sql": step, "migrations/002_b.txt": step}, false},
		{"empty", fstest.MapFS{"migrations/001_a.sql": &fstest.MapFile{Data: []byte("\n")}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded, err := loadSteps(tc.files)
			if tc.ok && (err != nil || len(loaded) != 2) {
				t.Fatalf("loadSteps = %d steps, %v", len(loaded), err)
			}
			if !tc.ok && err == nil {
				t.Fatal("loadSteps accepted it")
			}
		})
	}
}

// TestAFailedStepLeavesTheFileAtTheStepBefore: a step and the version it
// reaches commit together, so a step that fails (or a power cut during it) is
// taken again on the next open rather than skipped or half-done.
func TestAFailedStepLeavesTheFileAtTheStepBefore(t *testing.T) {
	db, err := sql.Open(driver, filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	err = migrate(db, 0, []string{
		`CREATE TABLE first (x);`,
		`CREATE TABLE second (x); SELECT * FROM absent;`,
	})
	if err == nil {
		t.Fatal("a failing step reported success")
	}
	if got := versionOf(t, db); got != firstStep {
		t.Fatalf("the file is at version %d, want %d", got, firstStep)
	}
	if _, err := db.Exec(`INSERT INTO second VALUES (1)`); err == nil {
		t.Fatal("the failed step's table outlived its rollback")
	}
}
