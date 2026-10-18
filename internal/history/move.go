// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// fileName is the history's file in its directory.
const fileName = "history.db"

// MovingSuffix names, after the history's file name, the copy a move is
// writing. It becomes the history only once whole, so a move a stop cuts short
// never leaves half a history to open; a factory reset removes it by this name
// with the history's own files (ADR-021).
const MovingSuffix = "-moving"

// sideFiles are the files SQLite keeps beside the history's file.
var sideFiles = []string{"-wal", "-shm", "-journal"}

// Move carries the history kept in the directory from into the directory to,
// where the shell opens it next (ADR-019): VACUUM INTO copies it whole,
// WAL and all, into a private file that takes the history's name once it is
// complete, and the files it was kept in are then removed, so the router keeps
// one history and a reset that reaches where it is reaches all of it. It
// answers whether a history moved.
//
// Nothing moves when from is empty, names to however it is spelled, or holds
// no history. A history in to already (one left there earlier, a stick from
// another router) is never written over: Move refuses, and both stay where
// they are. A move that fails leaves the history whole where it was.
func Move(from, to string) (bool, error) {
	if from == "" || filepath.Clean(from) == filepath.Clean(to) {
		return false, nil
	}
	source, target := filepath.Join(from, fileName), filepath.Join(to, fileName)
	if _, err := os.Stat(source); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	// An empty file is one an open made and then failed on: no history.
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return false, fmt.Errorf("%s holds a history already, so the one in %s stays there", to, from)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	moving := target + MovingSuffix
	if err := copyInto(source, moving); err != nil {
		_ = os.Remove(moving)
		return false, err
	}
	if err := os.Rename(moving, target); err != nil {
		_ = os.Remove(moving)
		return false, err
	}
	if err := syncDir(to); err != nil {
		return true, err
	}
	var removed []error
	for _, name := range append([]string{""}, sideFiles...) {
		if err := os.Remove(source + name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			removed = append(removed, err)
		}
	}
	return true, errors.Join(removed...)
}

// copyInto writes the history at source into a new file at copy, private to
// the shell from the start (it holds configs only root may read) and on disk
// before it returns. VACUUM INTO writes only into a file that is absent or
// empty, so one a cut-short move left is removed first.
func copyInto(source, copy string) error {
	if err := os.Remove(copy); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(copy, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	db, err := sql.Open(driver, "file:"+uriPath.Replace(source)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`VACUUM INTO ?`, copy)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("copy %s: %w", source, err)
	}
	f, err = os.OpenFile(copy, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// syncDir puts a rename in dir on disk.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return err
}
