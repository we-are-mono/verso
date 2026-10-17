// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"reflect"
	"testing"
)

// The DK's environment, as fw_printenv prints it on zamanifi.
func dkBootEnv() map[string]string {
	return map[string]string{
		"slot": "b", "bootcount": "0", "bootlimit": "3", "upgrade_available": "0",
		"set_slot_a": "setenv bootpart 1; setenv rootpart 2",
		"set_slot_b": "setenv bootpart 3; setenv rootpart 4",
	}
}

// The slot that runs is the one whose root partition the kernel booted; the
// next is what U-Boot boots, and a new image on trial counts its boots.
func TestBootSlot(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]string)
		root string
		want *BootSlot
	}{
		{"running B", func(map[string]string) {}, "/dev/mmcblk0p4", &BootSlot{Running: "b", Next: "b", Limit: 3}},
		{"a switch staged", func(env map[string]string) { env["slot"] = "a" }, "/dev/mmcblk0p4", &BootSlot{Running: "b", Next: "a", Limit: 3}},
		{"on trial", func(env map[string]string) { env["upgrade_available"], env["bootcount"] = "1", "2" }, "/dev/mmcblk0p4",
			&BootSlot{Running: "b", Next: "b", Trial: true, Count: 2, Limit: 3}},
		// A root no slot names (by PARTUUID, or none) runs the slot U-Boot boots.
		{"a root by PARTUUID", func(map[string]string) {}, "PARTUUID=5452574f-04", &BootSlot{Running: "b", Next: "b", Limit: 3}},
		{"no root", func(map[string]string) {}, "", &BootSlot{Running: "b", Next: "b", Limit: 3}},
		{"no slots", func(env map[string]string) { clear(env) }, "/dev/mmcblk0p4", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := dkBootEnv()
			c.edit(env)
			if got := (HardwareState{Boot: env}).BootSlot(c.root); !reflect.DeepEqual(got, c.want) {
				t.Errorf("BootSlot(%q) = %+v, want %+v", c.root, got, c.want)
			}
		})
	}
}
