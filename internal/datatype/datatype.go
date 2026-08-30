// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package datatype provides tier-1 declarative field validators, reusing LuCI's
// datatype names (hostname, ipaddr, host, port, …) so the vocabulary is familiar
// to OpenWrt plugin authors. These are the *declarative* checks a widget schema
// names; a plugin's own POST handler still does authoritative tier-3 validation
// (ADR-006 §5).
package datatype

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Validator checks one value and returns a human-readable error, or nil if the
// value is acceptable.
type Validator func(value string) error

var registry = map[string]Validator{
	"hostname": hostname,
	"fqdn":     fqdn,
	"ip4addr":  ip4addr,
	"ip6addr":  ip6addr,
	"ipaddr":   ipaddr,
	"host":     host,
	"port":     port,
}

// Validate checks value against the named datatype. An unknown name is an error
// (fail closed) so a typo cannot wave a value through unvalidated.
func Validate(name, value string) error {
	v, ok := registry[name]
	if !ok {
		return fmt.Errorf("unknown datatype %q", name)
	}
	return v(value)
}

func hostname(v string) error {
	if v == "" || len(v) > 253 {
		return fmt.Errorf("must be a valid hostname")
	}
	for _, label := range strings.Split(v, ".") {
		if !validLabel(label) {
			return fmt.Errorf("must be a valid hostname")
		}
	}
	return nil
}

// fqdn accepts a fully-qualified domain name: a valid hostname with at least one
// dot, so a bare single label (e.g. "router") is rejected.
func fqdn(v string) error {
	if hostname(v) != nil || !strings.Contains(v, ".") {
		return fmt.Errorf("must be a fully-qualified domain name (e.g. host.example.com)")
	}
	return nil
}

// validLabel reports whether a single dot-separated hostname label is valid:
// 1–63 chars, ASCII alphanumerics and hyphens, no leading/trailing hyphen.
func validLabel(l string) bool {
	if len(l) == 0 || len(l) > 63 {
		return false
	}
	if l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}

func ip4addr(v string) error {
	a, err := netip.ParseAddr(v)
	if err != nil || !a.Is4() {
		return fmt.Errorf("must be a valid IPv4 address")
	}
	return nil
}

func ip6addr(v string) error {
	a, err := netip.ParseAddr(v)
	if err != nil || !a.Is6() || a.Is4In6() {
		return fmt.Errorf("must be a valid IPv6 address")
	}
	return nil
}

func ipaddr(v string) error {
	if ip4addr(v) == nil || ip6addr(v) == nil {
		return nil
	}
	return fmt.Errorf("must be a valid IP address")
}

func host(v string) error {
	if hostname(v) == nil || ipaddr(v) == nil {
		return nil
	}
	return fmt.Errorf("must be a hostname or IP address")
}

func port(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("must be a port number (1–65535)")
	}
	return nil
}
