// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package datatype

import "testing"

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		// hostname
		{"hostname", "router", true},
		{"hostname", "my-router", true},
		{"hostname", "OpenWrt-1", true},
		{"hostname", "a", true},
		{"hostname", "", false},
		{"hostname", "-router", false},
		{"hostname", "router-", false},
		{"hostname", "has space", false},
		{"hostname", "under_score", false},
		{"hostname", "trailing.", false},
		// fqdn — a hostname that is fully qualified (at least one dot)
		{"fqdn", "host.example.com", true},
		{"fqdn", "router.lan", true},
		{"fqdn", "a.b", true},
		{"fqdn", "OpenWrt", false}, // single label is not fully qualified
		{"fqdn", "router", false},
		{"fqdn", "", false},
		{"fqdn", "-bad.example.com", false}, // invalid first label
		{"fqdn", "under_score.com", false},
		{"fqdn", "has space.com", false},
		{"fqdn", "trailing.", false},
		// ip4addr
		{"ip4addr", "192.168.1.1", true},
		{"ip4addr", "0.0.0.0", true},
		{"ip4addr", "255.255.255.255", true},
		{"ip4addr", "256.1.1.1", false},
		{"ip4addr", "1.2.3", false},
		{"ip4addr", "192.168.001.1", false}, // leading zeros rejected (strict)
		{"ip4addr", "::1", false},
		// ip6addr
		{"ip6addr", "::1", true},
		{"ip6addr", "2001:db8::1", true},
		{"ip6addr", "fe80::1", true},
		{"ip6addr", "192.168.1.1", false},
		{"ip6addr", "::ffff:1.2.3.4", false}, // v4-mapped is not a v6 host
		// ipaddr (v4 or v6)
		{"ipaddr", "10.0.0.1", true},
		{"ipaddr", "2001:db8::1", true},
		{"ipaddr", "nope", false},
		// host (hostname or ip) — the NTP-server datatype
		{"host", "0.openwrt.pool.ntp.org", true},
		{"host", "192.168.1.1", true},
		{"host", "time.example.com", true},
		{"host", "bad host", false},
		{"host", "", false},
		// port
		{"port", "1", true},
		{"port", "65535", true},
		{"port", "0", false},
		{"port", "65536", false},
		{"port", "-1", false},
		{"port", "abc", false},
	}

	for _, c := range cases {
		err := Validate(c.name, c.value)
		if c.ok && err != nil {
			t.Errorf("Validate(%q, %q) = %v, want ok", c.name, c.value, err)
		}
		if !c.ok && err == nil {
			t.Errorf("Validate(%q, %q) = nil, want error", c.name, c.value)
		}
	}
}

func TestValidateHostnameLabelLength(t *testing.T) {
	label63 := ""
	for i := 0; i < 63; i++ {
		label63 += "a"
	}
	if err := Validate("hostname", label63); err != nil {
		t.Errorf("63-char label should be valid: %v", err)
	}
	if err := Validate("hostname", label63+"a"); err == nil {
		t.Error("64-char label should be invalid")
	}
}

// TestValidateUnknownDatatypeFailsClosed: a datatype name the shell doesn't know
// must error, never silently pass — a typo in a plugin schema can't wave data
// through unvalidated.
func TestValidateUnknownDatatypeFailsClosed(t *testing.T) {
	if err := Validate("definitely-not-a-type", "anything"); err == nil {
		t.Error("unknown datatype must return an error")
	}
}

func TestKnown(t *testing.T) {
	if !Known("hostname") {
		t.Error("hostname should be known")
	}
	if Known("bogus") {
		t.Error("bogus should not be known")
	}
}
