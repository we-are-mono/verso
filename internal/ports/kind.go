// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ports

import (
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The connector a NIC's driver reports (ethtool's PORT_* values).
const (
	portTP    uint8 = 0x00
	portFibre uint8 = 0x03
	portDA    uint8 = 0x05
)

// LinkPort answers which connector a NIC's driver says it has, and whether it
// answered at all.
type LinkPort interface {
	Port(iface string) (uint8, bool)
}

// Kinds reads what connector each port is: "sfp" for an SFP+ cage, "rj45"
// otherwise.
type Kinds struct {
	NetClass string   // the sysfs net class, /sys/class/net
	Link     LinkPort // the driver's own answer; nil asks nothing
}

// NewKinds reads the running system.
func NewKinds() Kinds {
	return Kinds{NetClass: "/sys/class/net", Link: EthtoolLink{}}
}

// Kind is "sfp" when the device tree wires the port's MAC to an SFP cage
// (true whether or not a module sits in it) or its driver reports a fibre or
// direct-attach port, and "rj45" otherwise.
func (k Kinds) Kind(iface string) string {
	if _, err := os.Stat(filepath.Join(k.NetClass, iface, "device", "of_node", "sfp")); err == nil {
		return "sfp"
	}
	if k.Link != nil {
		if port, ok := k.Link.Port(iface); ok && (port == portFibre || port == portDA) {
			return "sfp"
		}
	}
	return "rj45"
}

// EthtoolLink asks the kernel through SIOCETHTOOL's ETHTOOL_GLINKSETTINGS, the
// read the ethtool tool itself falls back to. It needs no privilege and no
// generic-netlink family, so it answers on every kernel build.
type EthtoolLink struct{}

// linkSettings is struct ethtool_link_settings (linux/ethtool.h) with room
// for its three link-mode masks (supported, advertising, peer) at the most
// words the field can ask for.
type linkSettings struct {
	cmd                 uint32
	speed               uint32
	duplex              uint8
	port                uint8
	phyAddress          uint8
	autoneg             uint8
	mdioSupport         uint8
	ethTPMDIX           uint8
	ethTPMDIXCtrl       uint8
	linkModeMasksNwords int8
	transceiver         uint8
	masterSlaveCfg      uint8
	masterSlaveState    uint8
	rateMatching        uint8
	reserved            [7]uint32
	linkModeMasks       [3 * 127]uint32
}

// Port runs the two-call handshake: the first call learns how many link-mode
// words the kernel uses, the second, with room for them, returns the port.
func (EthtoolLink) Port(iface string) (uint8, bool) {
	if len(iface) >= unix.IFNAMSIZ {
		return 0, false
	}
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return 0, false
	}
	defer func() { _ = unix.Close(fd) }()

	settings := &linkSettings{cmd: unix.ETHTOOL_GLINKSETTINGS}
	if !ethtoolCall(fd, iface, settings) || settings.linkModeMasksNwords >= 0 {
		return 0, false
	}
	*settings = linkSettings{cmd: unix.ETHTOOL_GLINKSETTINGS, linkModeMasksNwords: -settings.linkModeMasksNwords}
	if !ethtoolCall(fd, iface, settings) {
		return 0, false
	}
	return settings.port, true
}

// ifreqData is struct ifreq with its union holding ifr_data, a pointer to the
// ethtool command.
type ifreqData struct {
	name [unix.IFNAMSIZ]byte
	data unsafe.Pointer
	_    [24 - unsafe.Sizeof(uintptr(0))]byte
}

func ethtoolCall(fd int, iface string, settings *linkSettings) bool {
	var req ifreqData
	copy(req.name[:], iface)
	req.data = unsafe.Pointer(settings)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.SIOCETHTOOL, uintptr(unsafe.Pointer(&req)))
	runtime.KeepAlive(settings)
	return errno == 0
}
