// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package secrets is the options OpenWrt's own packages keep a secret in, as
// config.option (ADR-021). secrets.json is the one list: the shell embeds it
// and verso-rpcd compiles it in, so config history keeps no secret's value
// whichever of them reads the config.
package secrets

import (
	_ "embed"
	"encoding/json"
)

//go:embed secrets.json
var list []byte

// Base is every option in the list.
func Base() []string {
	var names []string
	if err := json.Unmarshal(list, &names); err != nil {
		panic("secrets: secrets.json is not a list of names: " + err.Error())
	}
	return names
}
