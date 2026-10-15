// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package machine reads what the box is built from: its processor's cores and
// top clock, the memory the kernel manages, and the disk the firmware boots
// from. These are facts of the build, read once per page from procfs and
// sysfs; the readings that change (temperatures, rails, fans) are the sensors
// package's.
package machine

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Facts is the box's build. A fact the kernel does not answer is its zero
// value (Disk nil), and the page leaves it out.
type Facts struct {
	Cores       int    // CPUs online
	MaxKHz      int    // the highest top clock any CPU's cpufreq reports, 0 with none
	MemoryBytes uint64 // MemTotal: what the kernel manages, less than what is fitted
	Disk        *Disk  // the disk the firmware boots from
}

// Disk is the whole disk under the firmware's own filesystem.
type Disk struct {
	Bytes uint64
	Kind  string // "eMMC" | "SD card" | "MMC" | "NVMe" | "disk"
}

// Reader reads the facts under Root, "/" when empty; tests point it at a tree.
type Reader struct {
	Root string
}

func (r *Reader) path(p string) string {
	root := r.Root
	if root == "" {
		root = "/"
	}
	return filepath.Join(root, p)
}

// Read gathers every fact it can find.
func (r *Reader) Read() Facts {
	return Facts{
		Cores:       countCPUs(r.readLine("sys/devices/system/cpu/online")),
		MaxKHz:      r.maxKHz(),
		MemoryBytes: r.memTotal(),
		Disk:        r.bootDisk(),
	}
}

func (r *Reader) readLine(p string) string {
	b, err := os.ReadFile(r.path(p))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// countCPUs counts a kernel CPU list ("0-3,6"); a list it cannot read is none.
func countCPUs(list string) int {
	n := 0
	for part := range strings.SplitSeq(list, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			return 0
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil || b < a {
				return 0
			}
		}
		n += b - a + 1
	}
	return n
}

func (r *Reader) maxKHz() int {
	paths, _ := filepath.Glob(r.path("sys/devices/system/cpu/cpu[0-9]*/cpufreq/cpuinfo_max_freq"))
	top := 0
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if khz, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			top = max(top, khz)
		}
	}
	return top
}

func (r *Reader) memTotal() uint64 {
	f, err := os.Open(r.path("proc/meminfo"))
	if err != nil {
		return 0
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0
			}
			return kb * 1024
		}
	}
	return 0
}

// bootDisk is the disk under the root filesystem, or under /rom when the root
// is an overlay (a squashfs image's own filesystem is mounted there). A root
// on no block device, or on one this does not know as a disk, is none.
func (r *Reader) bootDisk() *Disk {
	devs := r.mountDevices()
	for _, mount := range []string{"/", "/rom"} {
		dev, ok := devs[mount]
		if !ok || strings.HasPrefix(dev, "0:") {
			continue
		}
		return r.disk(dev)
	}
	return nil
}

// mountDevices maps each mount point to its device's major:minor, the last
// mount at a point winning, as it shadows those under it.
func (r *Reader) mountDevices() map[string]string {
	f, err := os.Open(r.path("proc/self/mountinfo"))
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 5 {
			continue
		}
		out[fields[4]] = fields[2]
	}
	return out
}

// disk follows a device's major:minor to its whole disk: a partition's disk is
// the directory above it.
func (r *Reader) disk(majMin string) *Disk {
	dir, err := filepath.EvalSymlinks(r.path(filepath.Join("sys/dev/block", majMin)))
	if err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "partition")); err == nil {
		dir = filepath.Dir(dir)
	}
	kind := diskKind(filepath.Base(dir), func(p string) string { return readTrim(filepath.Join(dir, p)) })
	if kind == "" {
		return nil
	}
	sectors, err := strconv.ParseUint(readTrim(filepath.Join(dir, "size")), 10, 64)
	if err != nil || sectors == 0 {
		return nil
	}
	return &Disk{Bytes: sectors * 512, Kind: kind}
}

// diskKind names a disk by the kernel's name for it; an MMC device says
// whether it is soldered eMMC or a card. A name it does not know (a UBI
// volume, a RAM disk) is no disk.
func diskKind(name string, read func(string) string) string {
	switch {
	case strings.HasPrefix(name, "mmcblk"):
		switch read("device/type") {
		case "MMC":
			return "eMMC"
		case "SD":
			return "SD card"
		}
		return "MMC"
	case strings.HasPrefix(name, "nvme"):
		return "NVMe"
	case strings.HasPrefix(name, "sd"), strings.HasPrefix(name, "vd"), strings.HasPrefix(name, "xvd"):
		return "disk"
	}
	return ""
}

func readTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
