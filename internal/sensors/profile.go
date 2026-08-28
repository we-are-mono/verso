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
)

// Profile is one board's sensor map: ordered arrays of location→name entries per
// kind. The keys are device-tree of_node path tails; the values are the chosen
// display names. main/cpu flags lift one entry each onto the home dashboard.
type Profile struct {
	Name    string   `json:"name"`
	Ports   []string `json:"ports"`
	Fans    []Entry  `json:"fans"`
	Power   []Entry  `json:"power"`
	Thermal []Entry  `json:"thermal"`
}

// Entry is one sensor: its of_node path tail (Path), the display Name, and the
// optional headline flags. It marshals from the object form
// `{ "<path>": "<name>", "main": true }` — the single non-flag key is the path.
type Entry struct {
	Path string
	Name string
	Main bool // power: the headline draw (the input rail)
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

// LoadProfile reads the profile for boardName from fsys (the embedded profiles
// tree), addressed as "<boardName>/profile.json". It reports ok=false when the
// board has no folder — the caller then falls back to generic auto-detection.
func LoadProfile(fsys fs.FS, boardName string) (*Profile, bool) {
	if boardName == "" {
		return nil, false
	}
	data, err := fs.ReadFile(fsys, boardName+"/profile.json")
	if err != nil {
		return nil, false
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, false
	}
	return &p, true
}
