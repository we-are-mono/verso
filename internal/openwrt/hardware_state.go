// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HardwareState is what the board says of itself that only root may read
// (verso-rpcd's hardwareState): the clock that set the system time at boot,
// and the variables of U-Boot's environment that say which slot it boots.
type HardwareState struct {
	Clock *Clock            `json:"clock"` // nil on a board with no clock that keeps time
	Boot  map[string]string `json:"boot"`
}

// Clock is the RTC the kernel set the system time from at boot.
type Clock struct {
	Part    string `json:"part"`    // "pcf2131", as its device tree names it
	Battery string `json:"battery"` // "good" | "low" | "empty"; "" where the clock does not say
}

// BootSlot is the A/B slot the box runs from, as U-Boot keeps it.
type BootSlot struct {
	Running string // "b": the slot whose root the kernel booted
	Next    string // the slot U-Boot boots next; Running unless a switch is staged
	Trial   bool   // a new image is on trial, not yet kept
	Count   int    // its boots so far on trial
	Limit   int    // the boots U-Boot allows before it falls back to the other slot
}

// rootPart is the root partition a slot's set_slot_ script names.
var rootPart = regexp.MustCompile(`\brootpart\s+(\d+)`)

// BootSlot reads the slot that runs from the kernel's root (root= of its
// command line, "/dev/mmcblk0p4") against the root partition each slot's
// script names; a root no slot names runs the slot U-Boot boots. nil where
// the environment names no slot.
func (h HardwareState) BootSlot(root string) *BootSlot {
	next := h.Boot["slot"]
	if next == "" {
		return nil
	}
	s := &BootSlot{Running: next, Next: next, Trial: h.Boot["upgrade_available"] == "1"}
	s.Count, _ = strconv.Atoi(h.Boot["bootcount"])
	s.Limit, _ = strconv.Atoi(h.Boot["bootlimit"])
	if part := devicePartition(root); part != "" {
		for key, script := range h.Boot {
			slot, ok := strings.CutPrefix(key, "set_slot_")
			if m := rootPart.FindStringSubmatch(script); ok && m != nil && m[1] == part {
				s.Running = slot
			}
		}
	}
	return s
}

// devicePartition is the partition number of a root named by its device
// node ("/dev/mmcblk0p4" is "4"); a root named otherwise has none.
func devicePartition(root string) string {
	if !strings.HasPrefix(root, "/dev/") {
		return ""
	}
	trimmed := strings.TrimRight(root, "0123456789")
	if n := root[len(trimmed):]; n != "" {
		return strings.TrimLeft(n, "0")
	}
	return ""
}

// HardwareState asks the root helper what the board says of itself; it reads
// a few kilobytes of flash and one RTC register, so it answers fast.
func (b *NativeBackend) HardwareState(ctx context.Context, sid string) (HardwareState, error) {
	var state HardwareState
	err := callHelperWithin(ctx, b.helperSocket, "hardwareState", sid, nil, &state, 10*time.Second)
	return state, err
}
