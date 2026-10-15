// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

var ctx = context.Background()

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

// minute is the n-th minute after a day boundary, so rollups land on whole
// quarters, hours and days.
func minute(n int) time.Time { return time.Unix(1_800_000_000-1_800_000_000%86400+int64(n)*60, 0) }

// TestAddedPointsAreReadOnlyOnceFlushed: points collect in memory and reach the
// file in one transaction, so nothing is written per minute.
func TestAddedPointsAreReadOnlyOnceFlushed(t *testing.T) {
	store, path := open(t)
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 10, Peak: 30})
	store.Add(Point{Series: "cpu", Time: minute(1), Average: 20, Peak: 40})

	other, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	var rows int
	if err := other.QueryRow(`SELECT count(*) FROM points`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("before a flush the file holds %d points (%v), want 0", rows, err)
	}

	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.Points(ctx, "cpu", Minute, minute(0), minute(2))
	if err != nil {
		t.Fatal(err)
	}
	want := []Point{
		{Series: "cpu", Time: minute(0), Average: 10, Peak: 30},
		{Series: "cpu", Time: minute(1), Average: 20, Peak: 40},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("points = %+v, want %+v", got, want)
	}
}

// TestASeriesNameIsStoredOnce: points carry a number, not the name, so a long
// sensor name costs its bytes once rather than on every point; a name seen
// after a reopen keeps its number.
func TestASeriesNameIsStoredOnce(t *testing.T) {
	store, path := open(t)
	for n := range 3 {
		store.Add(Point{Series: "temp.Composite.nvme · 0e:00", Time: minute(n), Average: 1, Peak: 1})
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	reopened.Add(Point{Series: "temp.Composite.nvme · 0e:00", Time: minute(3), Average: 1, Peak: 1})
	if err := reopened.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	var names int
	if err := reopened.db.QueryRow(`SELECT count(*) FROM series`).Scan(&names); err != nil || names != 1 {
		t.Fatalf("series rows = %d (%v), want 1", names, err)
	}
	got, err := reopened.Points(ctx, "temp.Composite.nvme · 0e:00", Minute, minute(0), minute(4))
	if err != nil || len(got) != 4 {
		t.Fatalf("points = %+v (%v), want all four under one name", got, err)
	}
}

// TestPointsIncludesMinutesNotYetFlushed: a chart of the last day shows the
// last quarter of an hour too, though it is still in memory.
func TestPointsIncludesMinutesNotYetFlushed(t *testing.T) {
	store, _ := open(t)
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	store.Add(Point{Series: "cpu", Time: minute(1), Average: 2, Peak: 2})
	store.Add(Point{Series: "memory", Time: minute(1), Average: 9, Peak: 9})
	store.Add(Point{Series: "cpu", Time: minute(5), Average: 3, Peak: 3})
	got, err := store.Points(ctx, "cpu", Minute, minute(0), minute(5))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Average != 1 || got[1].Average != 2 {
		t.Fatalf("points = %+v, want the flushed minute and the one in memory", got)
	}
}

// TestSlotsPutsEachPointAtItsTimeAndLeavesGaps: a chart's x is time, so a
// minute nobody recorded is NaN at its own place, not the next point moved
// left.
func TestSlotsPutsEachPointAtItsTimeAndLeavesGaps(t *testing.T) {
	points := []Point{
		{Time: minute(0), Average: 4},
		{Time: minute(2), Average: 6},
		{Time: minute(9), Average: 99}, // past the window
	}
	got := Slots(points, func(p Point) float64 { return p.Average / 2 }, minute(0), Minute, 4)
	if len(got) != 4 || got[0] != 2 || !math.IsNaN(got[1]) || got[2] != 3 || !math.IsNaN(got[3]) {
		t.Fatalf("slots = %v, want [2 NaN 3 NaN]", got)
	}
}

// TestPointsReadsOneSeriesInRange: the range is half open, and another series
// never leaks in.
func TestPointsReadsOneSeriesInRange(t *testing.T) {
	store, _ := open(t)
	for n := range 5 {
		store.Add(Point{Series: "cpu", Time: minute(n), Average: float64(n), Peak: float64(n)})
		store.Add(Point{Series: "memory", Time: minute(n), Average: 99, Peak: 99})
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.Points(ctx, "cpu", Minute, minute(1), minute(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Average != 1 || got[1].Average != 2 {
		t.Fatalf("points = %+v, want minutes 1 and 2 of cpu", got)
	}
}

// TestMaintainRollsFinishedPeriodsUp: a finished quarter holds its minutes'
// average and their highest peak, so a burst survives the rollup; the quarter
// still running is not rolled, since its average would change.
func TestMaintainRollsFinishedPeriodsUp(t *testing.T) {
	store, _ := open(t)
	for n := range 20 {
		peak := 10.0
		if n == 7 {
			peak = 900
		}
		store.Add(Point{Series: "cpu", Time: minute(n), Average: float64(n % 15), Peak: peak})
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx, minute(20)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Points(ctx, "cpu", Quarter, minute(0), minute(30))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("quarters = %+v, want only the finished one", got)
	}
	if want := (Point{Series: "cpu", Time: minute(0), Average: 7, Peak: 900}); got[0] != want {
		t.Fatalf("quarter = %+v, want %+v", got[0], want)
	}
}

// TestMaintainRollsEveryResolution: minutes become quarters, quarters hours,
// hours days, each once its period has finished.
func TestMaintainRollsEveryResolution(t *testing.T) {
	store, _ := open(t)
	for n := range 24 * 60 {
		store.Add(Point{Series: "cpu", Time: minute(n), Average: 50, Peak: float64(n)})
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx, minute(24*60)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		res  Resolution
		rows int
	}{{Quarter, 96}, {Hour, 24}, {Day, 1}} {
		got, err := store.Points(ctx, "cpu", c.res, minute(0), minute(24*60))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.rows {
			t.Fatalf("%v: %d points, want %d", c.res, len(got), c.rows)
		}
		if last := got[len(got)-1]; last.Average != 50 || last.Peak != 24*60-1 {
			t.Fatalf("%v: last point = %+v, want average 50 and the day's last peak", c.res, last)
		}
	}
}

// TestMaintainKeepsAPeriodWhoseMinutesHaveExpired: a quarter rolled up while
// its minutes were whole is not rewritten from the few that remain at the edge
// of their retention.
func TestMaintainKeepsAPeriodWhoseMinutesHaveExpired(t *testing.T) {
	store, _ := open(t)
	for n := range 15 {
		store.Add(Point{Series: "cpu", Time: minute(n), Average: float64(n), Peak: float64(n)})
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx, minute(15)); err != nil {
		t.Fatal(err)
	}
	// 48 hours and five minutes later, minutes 0 to 4 have expired; 5 to 14
	// remain. The quarter must still read the average of all fifteen.
	if err := store.Maintain(ctx, minute(48*60+5)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Points(ctx, "cpu", Quarter, minute(0), minute(15))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Average != 7 {
		t.Fatalf("quarter = %+v, want the average of all fifteen minutes (7)", got)
	}
}

// TestMaintainDeletesWhatHasExpired: each resolution keeps its own window.
func TestMaintainDeletesWhatHasExpired(t *testing.T) {
	store, _ := open(t)
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
	store.Add(Point{Series: "cpu", Time: minute(1), Average: 1, Peak: 1})
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx, minute(48*60+1)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Points(ctx, "cpu", Minute, minute(0), minute(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Time != minute(1) {
		t.Fatalf("minutes = %+v, want only the one inside 48 hours", got)
	}
}

// TestEventsAreRecordedAndReadNewestFirst: two events of one kind on one
// subject in the same second are both kept (a flapping WAN).
func TestEventsAreRecordedAndReadNewestFirst(t *testing.T) {
	store, _ := open(t)
	events := []Event{
		{Time: minute(0), Kind: "wan.down", Subject: "wan"},
		{Time: minute(0), Kind: "wan.down", Subject: "wan"},
		{Time: minute(3), Kind: "wan.up", Subject: "wan", Detail: `{"after":180}`},
	}
	for _, e := range events {
		if err := store.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Events(ctx, minute(0), minute(10))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != events[2] || got[1] != events[0] {
		t.Fatalf("events = %+v, want all three, newest first", got)
	}
}

// TestLatestIsTheNewestEventOfAKind: what a watcher compares the present
// against; none recorded is not an error.
func TestLatestIsTheNewestEventOfAKind(t *testing.T) {
	store, _ := open(t)
	if _, found, err := store.Latest(ctx, "boot"); err != nil || found {
		t.Fatalf("Latest on nothing = found %v, %v", found, err)
	}
	newest := Event{Time: minute(5), Kind: "boot", Detail: `{"id":"b-2"}`}
	for _, e := range []Event{{Time: minute(1), Kind: "boot", Detail: `{"id":"b-1"}`}, newest, {Time: minute(9), Kind: "wan.down", Subject: "wan0"}} {
		if err := store.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, found, err := store.Latest(ctx, "boot")
	if err != nil || !found || got != newest {
		t.Fatalf("Latest = %+v found %v (%v), want %+v", got, found, err, newest)
	}
}

// TestMaintainDeletesExpiredEvents: events are kept as long as daily points.
func TestMaintainDeletesExpiredEvents(t *testing.T) {
	store, _ := open(t)
	old := Event{Time: minute(0), Kind: "boot", Subject: "router"}
	recent := Event{Time: minute(400 * 24 * 60), Kind: "boot", Subject: "router"}
	for _, e := range []Event{old, recent} {
		if err := store.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Maintain(ctx, minute(400*24*60+1)); err != nil {
		t.Fatal(err)
	}
	got, err := store.Events(ctx, minute(0), minute(400*24*60+1))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != recent {
		t.Fatalf("events = %+v, want only the one inside 400 days", got)
	}
}

// TestANewFileVacuumsIncrementally: auto_vacuum is on from the file's first
// write, the only moment SQLite takes it, so Maintain gives pages back.
func TestANewFileVacuumsIncrementally(t *testing.T) {
	store, _ := open(t)
	var mode int
	if err := store.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != 2 {
		t.Fatalf("auto_vacuum = %d, want 2 (incremental)", mode)
	}
}

// TestCloseFlushes: procd stopping the shell writes what is still in memory.
func TestCloseFlushes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err := reopened.Points(ctx, "cpu", Minute, minute(0), minute(1))
	if err != nil || len(got) != 1 {
		t.Fatalf("after reopening: %+v (%v), want the point Close wrote", got, err)
	}
}

// underUmask runs the rest of the test under mask, as procd's jail runs the
// shell under 022.
func underUmask(t *testing.T, mask int) {
	t.Helper()
	was := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(was) })
}

// readableDir is a directory made as root's init script makes one, readable
// to every account under the umask.
func readableDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "history")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("directory before opening: %v (%v), want 0755", info, err)
	}
	return dir
}

// wantPrivate fails unless the directory, the file and the WAL files beside it
// are the shell's account's alone.
func wantPrivate(t *testing.T, path string) {
	t.Helper()
	want := map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600, path + "-wal": 0o600, path + "-shm": 0o600}
	for name, perm := range want {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != perm {
			t.Errorf("%s is %#o, want %#o", filepath.Base(name), mode, perm)
		}
	}
}

// TestOpenMakesTheFileItsAccountsAloneWhateverTheUmask: the file holds copies
// of configs only root may read, so neither it, its WAL files nor its
// directory are left to the umask, which procd's jail sets to 022.
func TestOpenMakesTheFileItsAccountsAloneWhateverTheUmask(t *testing.T) {
	underUmask(t, 0o022)
	path := filepath.Join(readableDir(t), "history.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	wantPrivate(t, path)
}

// TestOpenTightensAFileLeftReadable: a router whose directory, file and the WAL
// files an unclean stop left beside it were made readable to every account has
// them made private on the next start.
func TestOpenTightensAFileLeftReadable(t *testing.T) {
	underUmask(t, 0o022)
	path := filepath.Join(readableDir(t), "history.db")
	earlier, err := sql.Open(driver, "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = earlier.Close() }()
	if _, err := earlier.Exec(`CREATE TABLE kept (x)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if info, err := os.Stat(name); err != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("%s before opening: %v (%v), want 0644", filepath.Base(name), info, err)
		}
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	wantPrivate(t, path)
}

// TestKeepPrivateLeavesASharedDirectoryOpen: history kept in a stick's root or
// in /data itself shares the directory with other daemons, so its mode is
// left as it stands; the files are still the shell's account's alone.
func TestKeepPrivateLeavesASharedDirectoryOpen(t *testing.T) {
	underUmask(t, 0o022)
	dir := readableDir(t)
	path := filepath.Join(dir, "history.db")
	asked := ""
	shared := func(d string) (bool, error) { asked = d; return true, nil }
	if err := keepPrivate(path, shared); err != nil {
		t.Fatal(err)
	}
	if asked != dir {
		t.Errorf("asked whether %q is shared, want %q", asked, dir)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("shared directory is %v (%v), want 0755 as it stood", info, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file is %v (%v), want 0600", info, err)
	}
}

// TestKeepPrivateFailsWhenItCannotTellTheDirectory: a directory that cannot be
// looked at cannot be written either, and the caller reports it.
func TestKeepPrivateFailsWhenItCannotTellTheDirectory(t *testing.T) {
	failed := errors.New("no such directory")
	err := keepPrivate(filepath.Join(t.TempDir(), "history.db"), func(string) (bool, error) { return false, failed })
	if !errors.Is(err, failed) {
		t.Fatalf("keepPrivate = %v, want %v", err, failed)
	}
}

// TestSharedDirectory: /data itself and a mount, whose device differs from its
// parent's, are shared; a directory made for history is not.
func TestSharedDirectory(t *testing.T) {
	if shared, err := sharedDirectory("/data"); err != nil || !shared {
		t.Errorf("sharedDirectory(/data) = %v, %v; want shared", shared, err)
	}
	if shared, err := sharedDirectory(t.TempDir()); err != nil || shared {
		t.Errorf("sharedDirectory(made) = %v, %v; want not shared", shared, err)
	}
	if _, err := sharedDirectory(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("sharedDirectory(absent) succeeded")
	}
	if _, err := os.Stat("/proc/self"); err != nil {
		t.Skip("no /proc mounted to tell a mount by")
	}
	if shared, err := sharedDirectory("/proc"); err != nil || !shared {
		t.Errorf("sharedDirectory(/proc) = %v, %v; want shared", shared, err)
	}
}

// TestOpenInMemoryMakesNoFile: the history kept in memory, where the directory
// cannot be written, has no file to guard and leaves none behind.
func TestOpenInMemoryMakesNoFile(t *testing.T) {
	t.Chdir(t.TempDir())
	store, err := Open(Memory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if entries, err := os.ReadDir("."); err != nil || len(entries) != 0 {
		t.Fatalf("Open(Memory) left %v (%v), want nothing", entries, err)
	}
}

// TestOpenFailsOnAnUnwritableDirectory: the caller learns the cause and keeps
// collecting in memory; Open does not create directories.
func TestOpenFailsOnAnUnwritableDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent", "history.db")
	if _, err := Open(missing); err == nil {
		t.Fatal("Open succeeded in a directory that does not exist")
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatalf("Open created the directory: %v", err)
	}
}

// TestOpenWritesTheFileItNames: SQLite reads its name as a URI, where ? starts
// the parameters, # a fragment and %xx is a byte, so a directory spelled with
// any of them still holds the file, private, and nothing lands elsewhere. The
// base is spelled plainly, so a name cut short at ? or # stays inside it.
func TestOpenWritesTheFileItNames(t *testing.T) {
	underUmask(t, 0o022)
	root := t.TempDir()
	for _, name := range []string{"a?b", "c#d", "h%69st", "e%3fx"} {
		t.Run(name, func(t *testing.T) {
			base, err := os.MkdirTemp(root, "case")
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(base, name)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(base, "hist"), 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "history.db")
			store, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			store.Add(Point{Series: "cpu", Time: minute(0), Average: 1, Peak: 1})
			if err := store.Flush(ctx); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if info, err := os.Stat(path); err != nil || info.Size() == 0 || info.Mode().Perm() != 0o600 {
				t.Errorf("%s: %v (%v), want the history, 0600", path, info, err)
			}
			_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && p != path {
					t.Errorf("Open wrote %s", p)
				}
				return nil
			})
		})
	}
}
