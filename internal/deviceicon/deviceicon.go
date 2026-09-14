// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package deviceicon picks a device-type Lucide icon for a connected device,
// deterministically, from data it already has: the device's hostname and its
// MAC. It is a best-effort classifier, not an identity — the returned name is a
// widget icon (internal/widget/icons.go), and an unknown device resolves to the
// generic Fallback rather than an error.
//
// Two curated tables drive it, both embedded so a build is self-contained and a
// contributor edits data, not code:
//
//   - hostname-icons: keyword → icon. The primary signal, because modern phones
//     and laptops present a per-network RANDOMIZED MAC whose OUI identifies
//     nothing — but their DHCP hostname still names them.
//   - mac-oui-icons: OUI → icon. A tight set of single-purpose vendors whose
//     devices use a real (non-randomized) MAC and map cleanly to one type.
package deviceicon

import (
	_ "embed"
	"strings"
)

// Fallback is the icon for a device that matches neither table — an honest
// "some device" rather than a wrong guess. It must exist in the widget icon set.
const Fallback = "device"

//go:embed mac-oui-icons
var ouiData string

//go:embed hostname-icons
var hostnameData string

var (
	ouiIcons  = parseOUI(ouiData)           // OUI (6 upper hex) → icon
	ouiMakers = parseMakers(ouiData)        // OUI (6 upper hex) → the vendor's own name
	hostRules = parseHostname(hostnameData) // ordered; first substring match wins
)

type hostRule struct{ keyword, icon string }

// Resolve returns the icon name for a device. The order is deliberate:
// hostname first (it survives MAC randomization and is the more specific
// signal), then the OUI table, then Fallback. It never returns "".
func Resolve(mac, hostname string) string {
	if h := strings.ToLower(strings.TrimSpace(hostname)); h != "" {
		for _, r := range hostRules {
			if strings.Contains(h, r.keyword) {
				return r.icon
			}
		}
	}
	if oui, ok := ouiOf(mac); ok {
		if icon, ok := ouiIcons[oui]; ok {
			return icon
		}
	}
	return Fallback
}

// ouiOf normalizes a MAC to its uppercase 6-hex OUI. It reports ok=false for a
// malformed MAC and — crucially — for a locally-administered (randomized)
// address: the second-least-significant bit of the first octet marks a MAC that
// is not a real vendor assignment, so its OUI classifies nothing.
func ouiOf(mac string) (string, bool) {
	var hex strings.Builder
	for _, c := range mac {
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'F':
			hex.WriteRune(c)
		case c >= 'a' && c <= 'f':
			hex.WriteRune(c - 32) // to upper
		case c == ':' || c == '-' || c == '.' || c == ' ':
			// separator, skip
		default:
			return "", false // an unexpected character — not a MAC
		}
	}
	s := hex.String()
	if len(s) < 6 {
		return "", false
	}
	if hexNibble(s[1])&0x2 != 0 { // locally-administered bit of the first octet
		return "", false
	}
	return s[:6], true
}

// hexNibble is the value of a single upper-hex digit (the caller guarantees the
// character is one of 0-9A-F).
func hexNibble(c byte) byte {
	if c <= '9' {
		return c - '0'
	}
	return c - 'A' + 10
}

// parseOUI reads the "<OUI> <icon> <vendor…>" table into an OUI→icon map. Blank
// lines and '#' comments are skipped; a malformed line is skipped, not fatal,
// so one bad contribution never breaks the classifier.
func parseOUI(data string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		oui := strings.ToUpper(fields[0])
		if len(oui) != 6 {
			continue
		}
		out[oui] = fields[1]
	}
	return out
}

// Maker names who built a device, from the vendor label the OUI table carries
// alongside each icon. Empty whenever the table cannot say — a randomized MAC,
// an OUI the curated seed does not list — and a listing that shows the maker
// simply omits it for that device rather than guessing.
func Maker(mac string) string {
	oui, ok := ouiOf(mac)
	if !ok {
		return ""
	}
	return ouiMakers[oui]
}

// parseMakers reads the same table's trailing vendor label, which parseOUI
// ignores: the rest of the line after the OUI and the icon, verbatim.
func parseMakers(data string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		oui := strings.ToUpper(fields[0])
		if len(oui) != 6 {
			continue
		}
		out[oui] = strings.Join(fields[2:], " ")
	}
	return out
}

// parseHostname reads the "<keyword> <icon>" table into an ordered slice — order
// is preserved from the file so the more specific keywords, listed first, win.
func parseHostname(data string) []hostRule {
	var out []hostRule
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		out = append(out, hostRule{keyword: strings.ToLower(fields[0]), icon: fields[1]})
	}
	return out
}
