// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ports

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// fakeLink answers what each NIC's driver says its connector is.
type fakeLink map[string]uint8

func (f fakeLink) Port(iface string) (uint8, bool) {
	port, ok := f[iface]
	return port, ok
}

// TestKind: a port is an SFP+ cage when the device tree wires its MAC to an
// SFP cage, which holds whether or not a module sits in it, or when its driver
// reports a fibre or direct-attach port; anything else, including a driver
// that answers nothing, is drawn as an RJ45 jack.
func TestKind(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "eth3", "device", "of_node"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "eth3", "device", "of_node", "sfp"), []byte{0, 0, 0, 1}, 0o644); err != nil {
		t.Fatal(err)
	}
	kinds := Kinds{NetClass: root, Link: fakeLink{
		"eth0": portTP, "eth5": portFibre, "eth6": portDA, "eth3": portTP,
	}}
	for iface, want := range map[string]string{
		"eth3": "sfp", "eth5": "sfp", "eth6": "sfp", "eth0": "rj45", "eth9": "rj45",
	} {
		if got := kinds.Kind(iface); got != want {
			t.Errorf("Kind(%s) = %q, want %q", iface, got, want)
		}
	}
	if got := (Kinds{NetClass: root}).Kind("eth5"); got != "rj45" {
		t.Errorf("Kind without a link prober = %q, want rj45", got)
	}
}

// TestLinkSettingsMatchesTheKernelsLayout pins the mirror of struct
// ethtool_link_settings to the offsets linux/ethtool.h gives it: the port
// byte at 9, the mask word count at 15, the masks at 48, and struct ifreq at
// 40 bytes with ifr_data at 16.
func TestLinkSettingsMatchesTheKernelsLayout(t *testing.T) {
	var s linkSettings
	var r ifreqData
	for name, got := range map[string][2]uintptr{
		"port":          {unsafe.Offsetof(s.port), 9},
		"nwords":        {unsafe.Offsetof(s.linkModeMasksNwords), 15},
		"reserved":      {unsafe.Offsetof(s.reserved), 20},
		"masks":         {unsafe.Offsetof(s.linkModeMasks), 48},
		"ifreq size":    {unsafe.Sizeof(r), 40},
		"ifreq data at": {unsafe.Offsetof(r.data), 16},
	} {
		if got[0] != got[1] {
			t.Errorf("%s = %d, want %d", name, got[0], got[1])
		}
	}
}

// TestEthtoolLinkAnswersForLoopback: the real prober runs its handshake
// against the kernel without privilege. Loopback has no link settings, so it
// answers nothing, and an interface that does not exist answers nothing too.
func TestEthtoolLinkAnswersForLoopback(t *testing.T) {
	for _, iface := range []string{"lo", "verso-no-such-nic"} {
		if port, ok := (EthtoolLink{}).Port(iface); ok {
			t.Errorf("Port(%s) = %d, true; want no answer", iface, port)
		}
	}
}
