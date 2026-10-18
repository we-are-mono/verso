// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"database/sql"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// TestAChangeRecordedBeforeCausesKeepsItsPlace: a file a shell wrote at
// version 9 steps to 10 with every change it held, each read back with no
// cause (ADR-021).
func TestAChangeRecordedBeforeCausesKeepsItsPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, 0, steps[:9]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO changes (time, status, user, address, note, relation, old_read) VALUES (60, 'applied', 'root', '10.0.0.12', 'kept', '', 1)`); err != nil {
		t.Fatal(err)
	}
	if got := versionOf(t, db); got != 9 {
		t.Fatalf("the file is at version %d, want 9", got)
	}
	_ = db.Close()

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if got := versionOf(t, store.db); got != latest || latest < 10 {
		t.Fatalf("the file is at version %d, want %d (at least 10)", got, latest)
	}
	list, err := store.Changes(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].User != "root" || list[0].Note != "kept" || list[0].Cause != "" || list[0].CausePlugin != "" {
		t.Fatalf("changes after the step: %+v", list)
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

// beforeKernelNames is a file at version 8, before temperatures were recorded
// under the kernel's names: one series per name, minutes 0 to 2 of each
// valued at the series' id, and any extra rows setup adds.
func beforeKernelNames(t *testing.T, names []string, extra string) string {
	t.Helper()
	// A file as a shell at version 8 wrote it: every table the first eight
	// steps make, so the steps after 9 find what they change.
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := migrate(db, 0, steps[:8]); err != nil {
		t.Fatal(err)
	}
	setup := ""
	for i, name := range names {
		setup += fmt.Sprintf("INSERT INTO series (id, name) VALUES (%d, '%s');\n", i+1, name)
		for m := range 3 {
			setup += fmt.Sprintf("INSERT INTO points VALUES (%d, 60, %d, %d, %d);\n", i+1, minute(m).Unix(), i+1, i+1)
		}
	}
	if _, err := db.Exec(setup + extra); err != nil {
		t.Fatal(err)
	}
	return path
}

func averages(t *testing.T, store *Store, series string) []float64 {
	t.Helper()
	points, err := store.Points(ctx, series, Minute, minute(0), minute(5))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]float64, 0, len(points))
	for _, p := range points {
		out = append(out, p.Average)
	}
	return out
}

// TestTheDKsTemperaturesMoveToTheKernelsNames: a DK recorded its temperatures
// under the names its profile gave them; step 9 moves each, every point with
// it, to the name the kernel gives its sensor, and leaves every other series
// as it was.
func TestTheDKsTemperaturesMoveToTheKernelsNames(t *testing.T) {
	const near, sfpArea = "soc/i2c@2180000/i2c-mux@70/i2c@1/temp-sensor@4c", "soc/i2c@2180000/i2c-mux@70/i2c@2/temp-sensor@4c"
	moves := map[string]string{
		"temp.cluster":              "temp.cluster-thermal",
		"temp.Memory":               "temp.ddr-thermal",
		"temp.Network":              "temp.fman-thermal",
		"temp.SerDes":               "temp.serdes-thermal",
		"temp.Crypto":               "temp.sec-thermal",
		"temp.PCB near CPU":         "temp.temp1." + near,
		"temp.CPU (external diode)": "temp.temp2." + near,
		"temp.SFP+ modules":         "temp.temp1." + sfpArea,
	}
	kept := []string{"cpu", "temp.temperature.sfp · xfi0"}
	names := append(slices.Sorted(maps.Keys(moves)), kept...)
	store, err := Open(beforeKernelNames(t, names, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	for i, old := range names {
		want := []float64{float64(i + 1), float64(i + 1), float64(i + 1)}
		name := old
		if moved, ok := moves[old]; ok {
			name = moved
			if got := averages(t, store, old); len(got) != 0 {
				t.Errorf("%s still holds %v", old, got)
			}
		}
		if got := averages(t, store, name); !slices.Equal(got, want) {
			t.Errorf("%s holds %v, want %v", name, got, want)
		}
	}
	if got := countOf(t, store.db, "series"); got != len(names) {
		t.Errorf("the file holds %d series, want %d", got, len(names))
	}
}

// TestAMovedSeriesJoinsOneAlreadyThere: a DK whose shell already recorded
// under the kernel's name keeps those points, and gains the older ones.
func TestAMovedSeriesJoinsOneAlreadyThere(t *testing.T) {
	path := beforeKernelNames(t, []string{"temp.cluster"},
		fmt.Sprintf("INSERT INTO series (id, name) VALUES (9, 'temp.cluster-thermal');\nINSERT INTO points VALUES (9, 60, %d, 50, 50);\nINSERT INTO points VALUES (9, 60, %d, 51, 51);\n", minute(2).Unix(), minute(3).Unix()))
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if got, want := averages(t, store, "temp.cluster-thermal"), []float64{1, 1, 50, 51}; !slices.Equal(got, want) {
		t.Errorf("temp.cluster-thermal holds %v, want %v", got, want)
	}
	if got := countOf(t, store.db, "series"); got != 1 {
		t.Errorf("the file holds %d series, want 1", got)
	}
}

// TestAnotherBoardsTemperaturesStay: only a DK's file holds temp.cluster, the
// name its profile gave the CPU; a box that happens to name a sensor "Memory"
// keeps its series under that name.
func TestAnotherBoardsTemperaturesStay(t *testing.T) {
	store, err := Open(beforeKernelNames(t, []string{"temp.Memory", "temp.Tctl"}, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	for _, name := range []string{"temp.Memory", "temp.Tctl"} {
		if got := averages(t, store, name); len(got) != 3 {
			t.Errorf("%s holds %v, want its three points", name, got)
		}
	}
	if got := averages(t, store, "temp.ddr-thermal"); len(got) != 0 {
		t.Errorf("another board's Memory moved to ddr-thermal: %v", got)
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
