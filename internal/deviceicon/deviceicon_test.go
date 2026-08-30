// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package deviceicon

import "testing"

func TestResolve(t *testing.T) {
	cases := []struct {
		name     string
		mac      string
		hostname string
		want     string
	}{
		// OUI table — real prefixes from the embedded data.
		{"sonos oui", "00:04:3C:11:22:33", "", "speaker"},
		{"espressif oui", "08:3A:8D:aa:bb:cc", "", "cpu"},
		{"nintendo oui", "00:09:BF:00:00:01", "", "gamepad"},
		{"oui lowercase + no separators", "00043c112233", "", "speaker"},
		{"oui dash separators", "00-04-3C-11-22-33", "", "speaker"},

		// Hostname keywords.
		{"iphone hostname", "", "Toms-iPhone", "phone"},
		{"sonos hostname", "", "living-room-sonos", "speaker"},
		{"case-insensitive hostname", "", "KITCHEN-ECOBEE", "thermometer"},

		// Hostname wins over OUI (the more specific signal, survives randomization).
		{"hostname beats oui", "00:04:3C:11:22:33", "someones-ipad", "tablet"},

		// The crux: a locally-administered (randomized) MAC must NOT be matched
		// against the OUI table, even when the first three octets look like a real
		// vendor. 00->02 in the first octet sets the locally-administered bit.
		{"randomized mac, real-looking oui, no hostname", "02:04:3C:11:22:33", "", Fallback},
		{"randomized mac da:a1:19", "DA:A1:19:00:00:00", "", Fallback},
		{"randomized mac but hostname still classifies", "DA:A1:19:00:00:00", "Pixel-8", "phone"},

		// Unknown / malformed → fallback, never "".
		{"unknown oui", "AA:BB:CC:DD:EE:FF", "", Fallback},
		{"empty everything", "", "", Fallback},
		{"garbage mac", "not-a-mac", "", Fallback},
		{"short mac", "00:04", "", Fallback},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve(c.mac, c.hostname); got != c.want {
				t.Errorf("Resolve(%q, %q) = %q, want %q", c.mac, c.hostname, got, c.want)
			}
		})
	}
}

// TestLocallyAdministeredBit pins the bit test directly: every first octet whose
// value & 0x02 is set is a randomized/local MAC and yields no OUI.
func TestLocallyAdministeredBit(t *testing.T) {
	universal := []string{"00", "AC", "08", "0C", "F0"} // bit clear → real OUI
	local := []string{"02", "06", "0A", "0E", "DA", "FE"} // bit set → randomized
	for _, oct := range universal {
		if _, ok := ouiOf(oct + ":00:00:00:00:00"); !ok {
			t.Errorf("first octet %s should be a universal (vendor) MAC", oct)
		}
	}
	for _, oct := range local {
		if _, ok := ouiOf(oct + ":00:00:00:00:00"); ok {
			t.Errorf("first octet %s should be locally-administered (randomized)", oct)
		}
	}
}

// TestTablesParsed guards against an empty embed or a broken parser.
func TestTablesParsed(t *testing.T) {
	if len(ouiIcons) == 0 {
		t.Fatal("OUI table parsed empty")
	}
	if len(hostRules) == 0 {
		t.Fatal("hostname table parsed empty")
	}
}
