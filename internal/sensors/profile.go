// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package sensors reads the box's hardware sensors — CPU temperature, fan speed,
// power draw — and resolves them through the matching board profile. A profile
// (from the profiles/ tree, keyed by board_name) names sensors by their
// device-tree node-path tail; without one, the reader auto-detects generically.
// See profiles/README.md for the profile format and resolution rules.
package sensors

import (
	"encoding/json"
	"io/fs"
	"path"
)

// Profile is one board's sensor map: ordered arrays of location→name entries per
// kind. The keys are device-tree of_node path tails; the values are the chosen
// display names. main/cpu flags lift one entry each onto the home dashboard.
//
// ID is the device's raw board_name this profile claims (e.g. "mono,gateway-dk"),
// and the match key — the folder name is only storage, so a board_name may carry a
// comma or any character a path segment should not. Aliases cover a board that
// reports different names across revisions. Dir is the folder the profile was
// loaded from, recorded so its panel art (back.svg/front.svg) loads from the same
// place; it never comes from JSON.
type Profile struct {
	ID      string   `json:"id"`
	Aliases []string `json:"aliases"`
	Name    string   `json:"name"`
	Ports   []string `json:"ports"`
	Fans    []Entry  `json:"fans"`
	Power   []Entry  `json:"power"`
	Thermal []Entry  `json:"thermal"`

	Dir string `json:"-"`
}

// Entry is one sensor: its of_node path tail (Path), the display Name, and the
// optional headline flags. It marshals from the object form
// `{ "<path>": "<name>", "main": true }` — the single non-flag key is the path.
type Entry struct {
	Path string
	Name string
	Main bool // fan: the primary channel; power: the headline draw (input rail)
	CPU  bool // thermal: the CPU temperature
}

// UnmarshalJSON reads the `{ "<path>": "<name>", "main"|"cpu": true }` shape: the
// one key that isn't a flag is the path, its string value the name.
func (e *Entry) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		switch k {
		case "main":
			if err := json.Unmarshal(v, &e.Main); err != nil {
				return err
			}
		case "cpu":
			if err := json.Unmarshal(v, &e.CPU); err != nil {
				return err
			}
		default:
			if err := json.Unmarshal(v, &e.Name); err != nil {
				return err
			}
			e.Path = k
		}
	}
	return nil
}

// LoadProfile finds the profile in fsys (the embedded profiles tree) whose id — or
// one of its aliases — equals the device's raw board_name, and returns it with the
// folder it lives in recorded (its panel art loads from there). The folder name is
// not the match key, so a board_name like "mono,gateway-dk" needs no folder to be
// named for it. A board with no matching profile reports ok=false, and the caller
// falls back to generic auto-detection.
func LoadProfile(fsys fs.FS, boardName string) (*Profile, bool) {
	if boardName == "" {
		return nil, false
	}
	paths, _ := fs.Glob(fsys, "*/profile.json")
	for _, file := range paths {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			continue
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if p.matches(boardName) {
			p.Dir = path.Dir(file)
			return &p, true
		}
	}
	return nil, false
}

// matches reports whether the profile claims a board_name as its id or an alias.
func (p *Profile) matches(boardName string) bool {
	if p.ID == boardName {
		return true
	}
	for _, alias := range p.Aliases {
		if alias == boardName {
			return true
		}
	}
	return false
}
