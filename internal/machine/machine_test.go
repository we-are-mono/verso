// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package machine

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// box writes the files every tree shares: the CPUs online, a top clock per
// CPU (none when khz is 0), and the memory the kernel manages.
func box(t *testing.T, cpus string, ncpu, khz int, memKB string) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "sys/devices/system/cpu/online"), cpus+"\n")
	for i := range ncpu {
		cpu := filepath.Join(root, "sys/devices/system/cpu", "cpu"+strconv.Itoa(i))
		write(t, filepath.Join(cpu, "online"), "1\n")
		if khz > 0 {
			write(t, filepath.Join(cpu, "cpufreq/cpuinfo_max_freq"), strconv.Itoa(khz-i*100000)+"\n")
		}
	}
	write(t, filepath.Join(root, "proc/meminfo"), "MemTotal:        "+memKB+" kB\nMemFree:          123456 kB\n")
	return root
}

// disk writes a whole disk under a device path and its partition, with the
// sys/dev/block link the kernel gives the partition's major:minor, the way
// sysfs links it (relative, up from sys/dev/block).
func disk(t *testing.T, root, devPath, name, part, majMin string, sectors string) string {
	t.Helper()
	d := filepath.Join(root, "sys/devices", devPath, "block", name)
	write(t, filepath.Join(d, "size"), sectors+"\n")
	target := d
	if part != "" {
		target = filepath.Join(d, part)
		write(t, filepath.Join(target, "partition"), "2\n")
		write(t, filepath.Join(target, "size"), "1024\n")
	}
	rel, err := filepath.Rel(filepath.Join(root, "sys/dev/block"), target)
	if err != nil {
		t.Fatal(err)
	}
	link(t, rel, filepath.Join(root, "sys/dev/block", majMin))
	return d
}

func mounts(t *testing.T, root string, lines ...string) {
	t.Helper()
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	write(t, filepath.Join(root, "proc/self/mountinfo"), body)
}

// The DK: four Cortex-A72 cores clocked up to 1.6 GHz, 7.7 GiB the kernel
// manages, and an ext4 root on the eMMC's second partition.
func TestTheDK(t *testing.T) {
	root := box(t, "0-3", 4, 1600000, "8070316")
	d := disk(t, root, "platform/soc/1560000.mmc/mmc_host/mmc0/mmc0:0001", "mmcblk0", "mmcblk0p2", "179:2", "61071360")
	write(t, filepath.Join(d, "device/type"), "MMC\n")
	mounts(t, root,
		"20 1 179:2 / / rw,noatime shared:1 - ext4 /dev/root rw",
		"21 20 0:5 / /proc rw,nosuid shared:2 - proc proc rw",
	)

	f := (&Reader{Root: root}).Read()
	if f.Cores != 4 || f.MaxKHz != 1600000 {
		t.Errorf("cores %d at %d kHz, want 4 at 1600000", f.Cores, f.MaxKHz)
	}
	if f.MemoryBytes != 8070316*1024 {
		t.Errorf("memory %d, want %d", f.MemoryBytes, 8070316*1024)
	}
	if f.Disk == nil || f.Disk.Bytes != 61071360*512 || f.Disk.Kind != "eMMC" {
		t.Errorf("disk %+v, want 61071360 sectors of eMMC", f.Disk)
	}
	if f.Disk.Wear != nil {
		t.Errorf("an eMMC that reports no estimate has no wear: %+v", f.Disk.Wear)
	}

	write(t, filepath.Join(d, "device/life_time"), "0x01 0x02\n")
	write(t, filepath.Join(d, "device/pre_eol_info"), "0x01\n")
	if w := (&Reader{Root: root}).Read().Disk.Wear; w == nil || w.Band != 2 || w.PreEOL != 1 {
		t.Errorf("wear %+v, want the worse estimate (10–20%%) and a normal reserve", w)
	}
}

// An eMMC's wear grades by the worse of what it estimates: its life used in
// tenths, and its reserve blocks spent.
func TestWearLevels(t *testing.T) {
	for _, c := range []struct {
		wear Wear
		want string
	}{
		{Wear{Band: 1, PreEOL: 1}, ""},
		{Wear{Band: 8, PreEOL: 1}, ""},
		{Wear{Band: 9, PreEOL: 1}, "warn"},      // 80–90% of its life used
		{Wear{Band: 2, PreEOL: 2}, "warn"},      // 80% of its reserve spent
		{Wear{Band: 11, PreEOL: 1}, "critical"}, // past its rated life
		{Wear{Band: 3, PreEOL: 3}, "critical"},  // its reserve all but gone
		{Wear{Band: 0, PreEOL: 0}, ""},          // nothing estimated
	} {
		if got := c.wear.Level(); got != c.want {
			t.Errorf("%+v: level %q, want %q", c.wear, got, c.want)
		}
	}
}

// An estimate the device leaves undefined is no estimate.
func TestAnUndefinedWearIsNone(t *testing.T) {
	root := box(t, "0-3", 4, 0, "1000000")
	d := disk(t, root, "platform/soc/1560000.mmc/mmc_host/mmc0/mmc0:0001", "mmcblk0", "mmcblk0p2", "179:2", "61071360")
	write(t, filepath.Join(d, "device/type"), "MMC\n")
	write(t, filepath.Join(d, "device/life_time"), "0x00 0x00\n")
	write(t, filepath.Join(d, "device/pre_eol_info"), "0x00\n")
	mounts(t, root, "20 1 179:2 / / rw - ext4 /dev/root rw")
	if w := (&Reader{Root: root}).Read().Disk.Wear; w != nil {
		t.Errorf("wear %+v, want none", w)
	}
}

// An x86 box: sixteen CPUs with no cpufreq, the root on an NVMe partition.
func TestAnNVMeBox(t *testing.T) {
	root := box(t, "0-15", 16, 0, "32768000")
	disk(t, root, "pci0000:00/0000:00:01.2/0000:01:00.0/nvme/nvme0", "nvme0n1", "nvme0n1p2", "259:2", "1000215216")
	mounts(t, root, "25 1 259:2 / / rw,relatime - ext4 /dev/nvme0n1p2 rw")

	f := (&Reader{Root: root}).Read()
	if f.Cores != 16 || f.MaxKHz != 0 {
		t.Errorf("cores %d at %d kHz, want 16 and no clock", f.Cores, f.MaxKHz)
	}
	if f.Disk == nil || f.Disk.Kind != "NVMe" || f.Disk.Bytes != 1000215216*512 {
		t.Errorf("disk %+v, want the NVMe drive", f.Disk)
	}
}

// A virtual machine: the root on a virtio disk with no partition table.
func TestAVirtioDisk(t *testing.T) {
	root := box(t, "0-1", 2, 0, "1004000")
	disk(t, root, "pci0000:00/0000:00:04.0/virtio1", "vda", "", "254:0", "2097152")
	mounts(t, root, "18 1 254:0 / / rw - ext4 /dev/vda rw")

	f := (&Reader{Root: root}).Read()
	if f.Disk == nil || f.Disk.Kind != "disk" || f.Disk.Bytes != 2097152*512 {
		t.Errorf("disk %+v, want the virtio disk", f.Disk)
	}
}

// A squashfs image: the root is an overlay, and the firmware's own
// filesystem, mounted at /rom, is on the SD card's third partition.
func TestAnOverlayRootReadsROM(t *testing.T) {
	root := box(t, "0-3", 4, 1500000, "1000000")
	d := disk(t, root, "platform/soc/fe340000.mmc/mmc_host/mmc1/mmc1:aaaa", "mmcblk1", "mmcblk1p3", "179:35", "62333952")
	write(t, filepath.Join(d, "device/type"), "SD\n")
	mounts(t, root,
		"16 1 179:35 / /rom ro,relatime - squashfs /dev/root ro",
		"22 1 0:22 / / rw,noatime - overlay overlayfs:/overlay rw,lowerdir=/,upperdir=/overlay/upper",
	)

	f := (&Reader{Root: root}).Read()
	if f.Disk == nil || f.Disk.Kind != "SD card" {
		t.Errorf("disk %+v, want the SD card under /rom", f.Disk)
	}
}

// A container: the root is an overlay with nothing at /rom on a block device,
// so the boot disk is unknown and left out.
func TestAnOverlayWithNoDisk(t *testing.T) {
	root := box(t, "0-7", 8, 0, "16000000")
	mounts(t, root, "600 500 0:52 / / rw,relatime - overlay overlay rw,lowerdir=/var/lib/docker")

	f := (&Reader{Root: root}).Read()
	if f.Disk != nil {
		t.Errorf("disk %+v, want none", f.Disk)
	}
	if f.Cores != 8 || f.MemoryBytes == 0 {
		t.Errorf("the other facts should stand: %+v", f)
	}
}

// A root on a device the reader does not know (a UBI volume on NAND) is left
// out rather than given a volume's size as the disk's.
func TestAnUnknownDiskIsLeftOut(t *testing.T) {
	root := box(t, "0", 1, 0, "250000")
	disk(t, root, "virtual/ubi/ubi0/ubiblock0_1", "ubiblock0_1", "", "254:0", "40000")
	mounts(t, root, "15 1 254:0 / /rom ro - squashfs /dev/root ro", "20 1 0:21 / / rw - overlay overlayfs:/overlay rw")

	if f := (&Reader{Root: root}).Read(); f.Disk != nil {
		t.Errorf("disk %+v, want none for a UBI volume", f.Disk)
	}
}

// mtd writes one MTD device as sysfs lists it: its type, its size and its
// offset in what holds it, linked to the device it is on. A read-only twin
// (mtd0ro) has no type.
func mtd(t *testing.T, root, name, typ, device string, offset, size uint64) {
	t.Helper()
	d := filepath.Join(root, "sys/class/mtd", name)
	if typ != "" {
		write(t, filepath.Join(d, "type"), typ+"\n")
		write(t, filepath.Join(d, "size"), strconv.FormatUint(size, 10)+"\n")
		write(t, filepath.Join(d, "offset"), strconv.FormatUint(offset, 10)+"\n")
	}
	if err := os.MkdirAll(device, 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, device, filepath.Join(d, "device"))
}

const mib = 1 << 20

// The DK's QSPI NOR: the SPI NOR framework names the part, and the partition
// spanning the whole chip says its size, over the parts mapped inside it.
func TestTheDKsNOR(t *testing.T) {
	root := box(t, "0-3", 4, 1600000, "8070316")
	spi := filepath.Join(root, "sys/devices/platform/soc/1550000.spi/spi_master/spi1/spi1.0")
	write(t, filepath.Join(spi, "spi-nor/partname"), "n25q512a\n")
	mtd(t, root, "mtd0", "nor", spi, 0, 64*mib)
	mtd(t, root, "mtd0ro", "", spi, 0, 0)
	mtd(t, root, "mtd1", "nor", spi, 0, 1*mib)
	mtd(t, root, "mtd6", "nor", spi, 8*mib, 24*mib)
	mtd(t, root, "mtd7", "nor", spi, 32*mib, 32*mib)

	f := (&Reader{Root: root}).Read()
	if f.Flash == nil || f.Flash.Part != "n25q512a" || f.Flash.Bytes != 64*mib {
		t.Errorf("flash %+v, want the 64 MiB n25q512a", f.Flash)
	}
}

// A chip mapped only in parts is as large as the furthest part reaches; a
// part split inside another (a firmware's rootfs) is measured in its parent,
// never taken for the chip.
func TestANORMappedInParts(t *testing.T) {
	root := box(t, "0", 1, 0, "250000")
	spi := filepath.Join(root, "sys/devices/platform/10000000.spi/spi_master/spi0/spi0.0")
	write(t, filepath.Join(spi, "spi-nor/partname"), "w25q128\n")
	mtd(t, root, "mtd0", "nor", spi, 0, mib/4)
	mtd(t, root, "mtd1", "nor", spi, mib/4, 16*mib-mib/4)
	mtd(t, root, "mtd2", "nor", filepath.Join(root, "sys/class/mtd/mtd1"), 2*mib, 13*mib)

	f := (&Reader{Root: root}).Read()
	if f.Flash == nil || f.Flash.Part != "w25q128" || f.Flash.Bytes != 16*mib {
		t.Errorf("flash %+v, want the 16 MiB w25q128", f.Flash)
	}
}

// A parallel NOR has no SPI NOR name: its size alone. NAND is not NOR.
func TestAParallelNORAndNAND(t *testing.T) {
	root := box(t, "0", 1, 0, "250000")
	cfi := filepath.Join(root, "sys/devices/platform/1e000000.flash")
	mtd(t, root, "mtd0", "nor", cfi, 0, 8*mib)
	mtd(t, root, "mtd1", "nand", filepath.Join(root, "sys/devices/platform/1e100000.nand"), 0, 128*mib)

	f := (&Reader{Root: root}).Read()
	if f.Flash == nil || f.Flash.Part != "" || f.Flash.Bytes != 8*mib {
		t.Errorf("flash %+v, want the 8 MiB parallel NOR", f.Flash)
	}
}

// Nothing to read is no facts, never a crash.
func TestAnEmptyRoot(t *testing.T) {
	f := (&Reader{Root: t.TempDir()}).Read()
	if f.Cores != 0 || f.MaxKHz != 0 || f.MemoryBytes != 0 || f.Disk != nil || f.Flash != nil {
		t.Errorf("an empty root gave facts: %+v", f)
	}
}

func TestCPUListCounts(t *testing.T) {
	for list, want := range map[string]int{"0": 1, "0-3": 4, "0-3,6": 5, "0,2,4-7": 6, "": 0, "x": 0} {
		if got := countCPUs(list); got != want {
			t.Errorf("countCPUs(%q) = %d, want %d", list, got, want)
		}
	}
}
