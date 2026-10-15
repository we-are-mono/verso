// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"slices"
	"testing"
)

func seriesPoints(t *testing.T, store *Store, name string, values ...float64) {
	t.Helper()
	for i, v := range values {
		store.Add(Point{Series: name, Time: minute(i), Average: v, Peak: v})
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestRenamingASeriesMovesEveryPoint: a series renamed takes every point it
// held, at every resolution, and the old name goes.
func TestRenamingASeriesMovesEveryPoint(t *testing.T) {
	store, _ := open(t)
	seriesPoints(t, store, "temp.temp1.lm75 · 5-48", 40, 41, 42)
	seriesPoints(t, store, "cpu", 7)
	if _, err := store.db.Exec(`INSERT INTO points SELECT series_id, 900, time, average, peak FROM points WHERE series_id = (SELECT id FROM series WHERE name = 'temp.temp1.lm75 · 5-48')`); err != nil {
		t.Fatal(err)
	}

	if err := store.RenameSeries(ctx, map[string]string{"temp.temp1.lm75 · 5-48": "temp.temp1.soc/i2c@0/lm75@48"}); err != nil {
		t.Fatal(err)
	}
	if got := averages(t, store, "temp.temp1.soc/i2c@0/lm75@48"); !slices.Equal(got, []float64{40, 41, 42}) {
		t.Errorf("the renamed series holds %v", got)
	}
	if got := averages(t, store, "temp.temp1.lm75 · 5-48"); len(got) != 0 {
		t.Errorf("the old name still holds %v", got)
	}
	if got := countOf(t, store.db, "points"); got != 7 {
		t.Errorf("the file holds %d points, want 7 (three a minute, three a quarter, cpu's)", got)
	}
	if got := countOf(t, store.db, "series"); got != 2 {
		t.Errorf("the file holds %d series, want 2", got)
	}
}

// TestARenameJoinsASeriesAlreadyThere: where the new name already holds
// points, they stay, and the old series' others join them.
func TestARenameJoinsASeriesAlreadyThere(t *testing.T) {
	store, _ := open(t)
	seriesPoints(t, store, "old", 1, 2, 3)
	store.Add(Point{Series: "new", Time: minute(2), Average: 50, Peak: 50}, Point{Series: "new", Time: minute(3), Average: 51, Peak: 51})
	if err := store.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameSeries(ctx, map[string]string{"old": "new"}); err != nil {
		t.Fatal(err)
	}
	if got, want := averages(t, store, "new"), []float64{1, 2, 50, 51}; !slices.Equal(got, want) {
		t.Errorf("new holds %v, want %v", got, want)
	}
	if got := countOf(t, store.db, "series"); got != 1 {
		t.Errorf("the file holds %d series, want 1", got)
	}
}

// TestRenamingWhatIsNotThereChangesNothing: a name the file never held makes
// no series under the new name, so a rename asked at every start is free once
// done.
func TestRenamingWhatIsNotThereChangesNothing(t *testing.T) {
	store, _ := open(t)
	seriesPoints(t, store, "cpu", 1)
	if err := store.RenameSeries(ctx, map[string]string{"absent": "elsewhere"}); err != nil {
		t.Fatal(err)
	}
	if got := countOf(t, store.db, "series"); got != 1 {
		t.Errorf("the file holds %d series, want 1", got)
	}
}
