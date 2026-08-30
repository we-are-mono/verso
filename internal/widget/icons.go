// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"strings"
)

// Icons are Lucide (https://lucide.dev), ISC-licensed — see lucide-icons.LICENSE in
// this directory. This map is the single source of every icon in the UI: never
// hand-draw one or inline an <svg> in a template. To add an icon, take the Lucide
// glyph's inner SVG verbatim and register it here by a Verso-semantic name; the shell
// renders it through Icon() at one stroke width, so the whole set stays consistent
// (no per-template drift). Plugins only ever reference an icon by name (ADR-005/006).
//
// Values are the inner elements of the Lucide <svg> (the outer tag, stroke width, and
// colour are applied uniformly by Icon). The comment names the Lucide source glyph
// where it differs from the key.
const iconStrokeWidth = "1.75"

var lucideIcons = map[string]string{
	// Device / status glyphs referenced by widgets' `icon` fields.
	"phone":         `<rect width="14" height="20" x="5" y="2" rx="2" ry="2" /><path d="M12 18h.01" />`, // lucide: smartphone
	"laptop":        `<path d="M18 5a2 2 0 0 1 2 2v8.526a2 2 0 0 0 .212.897l1.068 2.127a1 1 0 0 1-.9 1.45H3.62a1 1 0 0 1-.9-1.45l1.068-2.127A2 2 0 0 0 4 15.526V7a2 2 0 0 1 2-2z" /><path d="M20.054 15.987H3.946" />`,
	"router":        `<rect width="20" height="8" x="2" y="14" rx="2" /><path d="M6.01 18H6" /><path d="M10.01 18H10" /><path d="M15 10v4" /><path d="M17.84 7.17a4 4 0 0 0-5.66 0" /><path d="M20.66 4.34a8 8 0 0 0-11.31 0" />`,
	"device":        `<rect width="20" height="14" x="2" y="3" rx="2" /><line x1="8" x2="16" y1="21" y2="21" /><line x1="12" x2="12" y1="17" y2="21" />`,                                         // lucide: monitor (generic single device)
	"devices":       `<path d="M18 8V6a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h8" /><path d="M10 19v-3.96 3.15" /><path d="M7 19h5" /><rect width="6" height="10" x="16" y="12" rx="2" />`, // lucide: monitor-smartphone
	"globe":         `<circle cx="12" cy="12" r="10" /><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" /><path d="M2 12h20" />`,
	"network":       `<rect x="16" y="16" width="6" height="6" rx="1" /><rect x="2" y="16" width="6" height="6" rx="1" /><rect x="9" y="2" width="6" height="6" rx="1" /><path d="M5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3" /><path d="M12 12V8" />`,                                       // lucide: network (a logical interface)
	"ethernet-port": `<path d="M10 8v1" /><path d="M14 8v1" /><path d="M18 8v1" /><path d="M19 17a2 2 0 0 0-1.765 1.059l-.47.882A2 2 0 0 1 15 20H9a2 2 0 0 1-1.765-1.059l-.47-.882A2 2 0 0 0 5 17H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2z" /><path d="M6 8v1" />`, // lucide: ethernet-port (a physical port)
	"ban":           `<circle cx="12" cy="12" r="10" /><path d="M4.929 4.929 19.07 19.071" />`,                                                                                                                                                                                              // lucide: ban (a reject verdict)
	"circle-slash":  `<circle cx="12" cy="12" r="10" /><line x1="9" x2="15" y1="15" y2="9" />`,                                                                                                                                                                                              // lucide: circle-slash (a drop verdict)
	"shield":        `<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" /><path d="m9 12 2 2 4-4" />`,                                                            // lucide: shield-check
	"zone":          `<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" />`,                                                                                      // lucide: shield (a firewall zone endpoint)
	"shield-alert":  `<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" /><path d="M12 8v4" /><path d="M12 16h.01" />`,
	"speed":         `<path d="M15.914 4a1.5 1.5 0 0 0-2.474-1.561l-9 9A1.5 1.5 0 0 0 5.5 14h4.002a.5.5 0 0 1 .471.666L8.086 20a1.5 1.5 0 0 0 2.475 1.56l9-9A1.5 1.5 0 0 0 18.5 10h-3.997a.5.5 0 0 1-.472-.667z" />`, // lucide: zap

	// Structural glyphs (chrome + widget affordances), keyed by their Lucide name.
	"chevron-right":  `<path d="m9 18 6-6-6-6" />`,
	"chevron-down":   `<path d="m6 9 6 6 6-6" />`,
	"chevron-left":   `<path d="m15 18-6-6 6-6" />`,
	"x":              `<path d="M18 6 6 18" /><path d="m6 6 12 12" />`,
	"pencil":         `<path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" /><path d="m15 5 4 4" />`,
	"copy":           `<rect width="14" height="14" x="8" y="8" rx="2" ry="2" /><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />`,
	"check":          `<path d="M20 6 9 17l-5-5" />`,
	"circle-check":   `<circle cx="12" cy="12" r="10" /><path d="m9 12 2 2 4-4" />`,
	"info":           `<circle cx="12" cy="12" r="10" /><path d="M12 16v-4" /><path d="M12 8h.01" />`,
	"triangle-alert": `<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3" /><path d="M12 9v4" /><path d="M12 17h.01" />`,
	"circle-alert":   `<circle cx="12" cy="12" r="10" /><line x1="12" x2="12" y1="8" y2="12" /><line x1="12" x2="12.01" y1="16" y2="16" />`,
	"circle-x":       `<circle cx="12" cy="12" r="10" /><path d="m15 9-6 6" /><path d="m9 9 6 6" />`,
	"activity":       `<path d="M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2" />`,
	"clock":          `<circle cx="12" cy="12" r="10" /><polyline points="12 6 12 12 16 14" />`,
	"arrow-right":    `<path d="M5 12h14" /><path d="m12 5 7 7-7 7" />`,
	"plus":           `<path d="M5 12h14" /><path d="M12 5v14" />`,
	"wifi":           `<path d="M12 20h.01" /><path d="M2 8.82a15 15 0 0 1 20 0" /><path d="M5 12.859a10 10 0 0 1 14 0" /><path d="M8.5 16.429a5 5 0 0 1 7 0" />`,
	"tv":             `<path d="m17 2-5 5-5-5" /><rect width="20" height="15" x="2" y="7" rx="2" />`,
	"key":            `<path d="m15.5 7.5 2.3 2.3a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L19 4" /><path d="m21 2-9.6 9.6" /><circle cx="7.5" cy="15.5" r="5.5" />`,
	"lock":           `<rect width="18" height="11" x="3" y="11" rx="2" ry="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" />`,
	"search":         `<circle cx="11" cy="11" r="8" /><path d="m21 21-4.3-4.3" />`,
	"log-out":        `<path d="m16 17 5-5-5-5" /><path d="M21 12H9" /><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />`,
	"refresh-cw":     `<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" /><path d="M21 3v5h-5" /><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" /><path d="M8 16H3v5" />`,
	"loader-circle":  `<path d="M21 12a9 9 0 1 1-6.219-8.56" />`,
	"download":       `<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" x2="12" y1="15" y2="3" />`,
	"upload":         `<path d="M12 3v12" /><path d="m17 8-5-5-5 5" /><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />`,
	"cpu":            `<path d="M12 20v2" /><path d="M12 2v2" /><path d="M17 20v2" /><path d="M17 2v2" /><path d="M2 12h2" /><path d="M2 17h2" /><path d="M2 7h2" /><path d="M20 12h2" /><path d="M20 17h2" /><path d="M20 7h2" /><path d="M7 20v2" /><path d="M7 2v2" /><rect x="4" y="4" width="16" height="16" rx="2" /><rect x="8" y="8" width="8" height="8" rx="1" />`,
	"memory-stick":   `<path d="M12 12v-2" /><path d="M12 18v-2" /><path d="M16 12v-2" /><path d="M16 18v-2" /><path d="M2 11h1.5" /><path d="M20 18v-2" /><path d="M20.5 11H22" /><path d="M4 18v-2" /><path d="M8 12v-2" /><path d="M8 18v-2" /><rect x="2" y="6" width="20" height="10" rx="2" />`,
	"hard-drive":     `<path d="M10 16h.01" /><path d="M2.212 11.577a2 2 0 0 0-.212.896V18a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-5.527a2 2 0 0 0-.212-.896L18.55 5.11A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" /><path d="M21.946 12.013H2.054" /><path d="M6 16h.01" />`,

	// Sidebar (device-first navigation).
	"house":              `<path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8" /><path d="M3 10a2 2 0 0 1 .709-1.528l7-5.999a2 2 0 0 1 2.582 0l7 5.999A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />`,
	"users":              `<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><path d="M16 3.128a4 4 0 0 1 0 7.744" /><path d="M22 21v-2a4 4 0 0 0-3-3.87" /><circle cx="9" cy="7" r="4" />`,
	"sliders-horizontal": `<line x1="21" x2="14" y1="4" y2="4" /><line x1="10" x2="3" y1="4" y2="4" /><line x1="21" x2="12" y1="12" y2="12" /><line x1="8" x2="3" y1="12" y2="12" /><line x1="21" x2="16" y1="20" y2="20" /><line x1="12" x2="3" y1="20" y2="20" /><line x1="14" x2="14" y1="2" y2="6" /><line x1="8" x2="8" y1="10" y2="14" /><line x1="16" x2="16" y1="18" y2="22" />`,
	"settings":           `<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" /><circle cx="12" cy="12" r="3" />`,
	"menu":               `<path d="M4 12h16" /><path d="M4 6h16" /><path d="M4 18h16" />`,
	"moon":               `<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />`,
	"sun":                `<circle cx="12" cy="12" r="4" /><path d="M12 2v2" /><path d="M12 20v2" /><path d="m4.93 4.93 1.41 1.41" /><path d="m17.66 17.66 1.41 1.41" /><path d="M2 12h2" /><path d="M20 12h2" /><path d="m6.34 17.66-1.41 1.41" /><path d="m19.07 4.93-1.41 1.41" />`,
}

// Icon renders a Lucide glyph by name at the shared stroke width, with the given
// utility classes for size and colour. An unknown name falls back to a generic device
// glyph rather than rendering nothing, so a typo shows up instead of silently
// vanishing. It is registered as the "icon" template function on both the widget
// renderer and the server's page templates. Plugin-supplied names reach it only as a
// map key (never emitted), and the class is escaped, so it stays XSS-safe.
func Icon(name, class string) template.HTML {
	inner, ok := lucideIcons[name]
	if !ok {
		inner = lucideIcons["device"]
	}
	var b strings.Builder
	b.WriteString(`<svg class="`)
	b.WriteString(template.HTMLEscapeString(class))
	b.WriteString(`" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="`)
	b.WriteString(iconStrokeWidth)
	b.WriteString(`" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">`)
	b.WriteString(inner)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
