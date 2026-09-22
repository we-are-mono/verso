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
	"git-merge":  `<circle cx="18" cy="18" r="3"></circle><circle cx="6" cy="6" r="3"></circle><path d="M6 21V9a9 9 0 0 0 9 9"></path>`,
	"tag":        `<path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"></path><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"></circle>`,
	"git-branch": `<line x1="6" x2="6" y1="3" y2="15"></line><circle cx="18" cy="6" r="3"></circle><circle cx="6" cy="18" r="3"></circle><path d="M18 9a9 9 0 0 1-9 9"></path>`,
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

	// Device-type glyphs for the connected-devices roster, chosen by MAC OUI or
	// hostname (see internal/deviceicon). "device" above is the fallback.
	"speaker":     `<rect width="16" height="20" x="4" y="2" rx="2" /><path d="M12 6h.01" /><circle cx="12" cy="14" r="4" /><path d="M12 14h.01" />`,                                                                                                                                                                                                                                                                                                                                           // lucide: speaker
	"server":      `<rect width="20" height="8" x="2" y="2" rx="2" ry="2" /><rect width="20" height="8" x="2" y="14" rx="2" ry="2" /><line x1="6" x2="6.01" y1="6" y2="6" /><line x1="6" x2="6.01" y1="18" y2="18" />`,                                                                                                                                                                                                                                                                         // lucide: server
	"printer":     `<path d="M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2" /><path d="M6 9V3a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v6" /><rect x="6" y="14" width="12" height="8" rx="1" />`,                                                                                                                                                                                                                                                                             // lucide: printer
	"camera":      `<path d="M16.75 12h3.632a1 1 0 0 1 .894 1.447l-2.034 4.069a1 1 0 0 1-1.708.134l-2.124-2.97" /><path d="M17.106 9.053a1 1 0 0 1 .447 1.341l-3.106 6.211a1 1 0 0 1-1.342.447L3.61 12.3a2.92 2.92 0 0 1-1.3-3.91L3.69 5.6a2.92 2.92 0 0 1 3.92-1.3z" /><path d="M2 19h3.76a2 2 0 0 0 1.8-1.1L9 15" /><path d="M2 21v-4" /><path d="M7 9h.01" />`,                                                                                                                              // lucide: cctv
	"lightbulb":   `<path d="M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5" /><path d="M9 18h6" /><path d="M10 22h4" />`,                                                                                                                                                                                                                                                                                                                // lucide: lightbulb
	"gamepad":     `<line x1="6" x2="10" y1="11" y2="11" /><line x1="8" x2="8" y1="9" y2="13" /><line x1="15" x2="15.01" y1="12" y2="12" /><line x1="18" x2="18.01" y1="10" y2="10" /><path d="M17.32 5H6.68a4 4 0 0 0-3.978 3.59c-.006.052-.01.101-.017.152C2.604 9.416 2 14.456 2 16a3 3 0 0 0 3 3c1 0 1.5-.5 2-1l1.414-1.414A2 2 0 0 1 9.828 16h4.344a2 2 0 0 1 1.414.586L17 18c.5.5 1 1 2 1a3 3 0 0 0 3-3c0-1.545-.604-6.584-.685-7.258-.007-.05-.011-.1-.017-.151A4 4 0 0 0 17.32 5z" />`, // lucide: gamepad-2
	"tablet":      `<rect width="16" height="20" x="4" y="2" rx="2" ry="2" /><line x1="12" x2="12.01" y1="18" y2="18" />`,                                                                                                                                                                                                                                                                                                                                                                      // lucide: tablet
	"watch":       `<path d="M12 10v2.2l1.6 1" /><path d="m16.13 7.66-.81-4.05a2 2 0 0 0-2-1.61h-2.68a2 2 0 0 0-2 1.61l-.78 4.05" /><path d="m7.88 16.36.8 4a2 2 0 0 0 2 1.61h2.72a2 2 0 0 0 2-1.61l.81-4.05" /><circle cx="12" cy="12" r="6" />`,                                                                                                                                                                                                                                              // lucide: watch
	"thermometer": `<path d="M14 4v10.54a4 4 0 1 1-4 0V4a2 2 0 0 1 4 0Z" />`,                                                                                                                                                                                                                                                                                                                                                                                                                   // lucide: thermometer

	// Sensor-kind glyphs for the Hardware page's readings table (the running-fan
	// mark is the shell-owned spinning glyph "fan-spin"; "fan" here is the static
	// Lucide fan a stalled header and the table use).
	"zap":   `<path d="M15.914 4a1.5 1.5 0 0 0-2.474-1.561l-9 9A1.5 1.5 0 0 0 5.5 14h4.002a.5.5 0 0 1 .471.666L8.086 20a1.5 1.5 0 0 0 2.475 1.56l9-9A1.5 1.5 0 0 0 18.5 10h-3.997a.5.5 0 0 1-.472-.667z" />`,                             // lucide: zap
	"fan":   `<path d="M10.827 16.379a6.082 6.082 0 0 1-8.618-7.002l5.412 1.45a6.082 6.082 0 0 1 7.002-8.618l-1.45 5.412a6.082 6.082 0 0 1 8.618 7.002l-5.412-1.45a6.082 6.082 0 0 1-7.002 8.618l1.45-5.412Z" /><path d="M12 12v.01" />`, // lucide: fan
	"radio": `<path d="M16.247 7.761a6 6 0 0 1 0 8.478" /><path d="M19.075 4.933a10 10 0 0 1 0 14.134" /><path d="M4.925 19.067a10 10 0 0 1 0-14.134" /><path d="M7.753 16.239a6 6 0 0 1 0-8.478" /><circle cx="12" cy="12" r="2" />`,    // lucide: radio

	// Structural glyphs (chrome + widget affordances), keyed by their Lucide name.
	"chevron-right": `<path d="m9 18 6-6-6-6" />`,
	"chevron-down":  `<path d="m6 9 6 6 6-6" />`,
	"chevron-left":  `<path d="m15 18-6-6 6-6" />`,
	"x":             `<path d="M18 6 6 18" /><path d="m6 6 12 12" />`,
	"pencil":        `<path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" /><path d="m15 5 4 4" />`,
	"square-pen":    `<path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" /><path d="M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z" />`,
	"copy":          `<rect width="14" height="14" x="8" y="8" rx="2" ry="2" /><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />`,
	"bookmark-plus": `<path d="m19 21-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" /><path d="M9 10h6" /><path d="M12 7v6" />`,
	"trash-2":       `<path d="M3 6h18" /><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6" /><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" /><path d="M10 11v6" /><path d="M14 11v6" />`,
	// The pair a row's own state flips between: a thing that is on offers to go
	// off, and a thing that is off offers to come back.
	"power":          `<path d="M12 2v10" /><path d="M18.4 6.6a9 9 0 1 1-12.77.04" />`,
	"power-off":      `<path d="M18.36 6.64A9 9 0 0 1 20.77 15" /><path d="M6.16 6.16a9 9 0 1 0 12.68 12.68" /><path d="M12 2v4" /><path d="m2 2 20 20" />`,
	"check":          `<path d="M20 6 9 17l-5-5" />`,
	"circle-check":   `<circle cx="12" cy="12" r="10" /><path d="m9 12 2 2 4-4" />`,
	"info":           `<circle cx="12" cy="12" r="10" /><path d="M12 16v-4" /><path d="M12 8h.01" />`,
	"triangle-alert": `<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3" /><path d="M12 9v4" /><path d="M12 17h.01" />`,
	"circle-alert":   `<circle cx="12" cy="12" r="10" /><line x1="12" x2="12" y1="8" y2="12" /><line x1="12" x2="12.01" y1="16" y2="16" />`,
	"circle-x":       `<circle cx="12" cy="12" r="10" /><path d="m15 9-6 6" /><path d="m9 9 6 6" />`,
	"activity":       `<path d="M22 12h-2.48a2 2 0 0 0-1.93 1.46l-2.35 8.36a.25.25 0 0 1-.48 0L9.24 2.18a.25.25 0 0 0-.48 0l-2.35 8.36A2 2 0 0 1 4.49 12H2" />`,
	"clock":          `<circle cx="12" cy="12" r="10" /><polyline points="12 6 12 12 16 14" />`,
	"arrow-left":     `<path d="m12 19-7-7 7-7" /><path d="M19 12H5" />`,
	"arrow-right":    `<path d="M5 12h14" /><path d="m12 5 7 7-7 7" />`,
	"plus":           `<path d="M5 12h14" /><path d="M12 5v14" />`,
	"wifi":           `<path d="M12 20h.01" /><path d="M2 8.82a15 15 0 0 1 20 0" /><path d="M5 12.859a10 10 0 0 1 14 0" /><path d="M8.5 16.429a5 5 0 0 1 7 0" />`,
	"tv":             `<path d="m17 2-5 5-5-5" /><rect width="20" height="15" x="2" y="7" rx="2" />`,
	"key":            `<path d="m15.5 7.5 2.3 2.3a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L19 4" /><path d="m21 2-9.6 9.6" /><circle cx="7.5" cy="15.5" r="5.5" />`,
	"lock":           `<rect width="18" height="11" x="3" y="11" rx="2" ry="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" />`,
	"eye":            `<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0" /><circle cx="12" cy="12" r="3" />`,
	"eye-off":        `<path d="M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49" /><path d="M14.084 14.158a3 3 0 0 1-4.242-4.242" /><path d="M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143" /><path d="m2 2 20 20" />`,
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
	"route":                 `<circle cx="6" cy="19" r="3" /><path d="M9 19h8.5a3.5 3.5 0 0 0 0-7h-11a3.5 3.5 0 0 1 0-7H15" /><circle cx="18" cy="5" r="3" />`,
	"house":                 `<path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8" /><path d="M3 10a2 2 0 0 1 .709-1.528l7-5.999a2 2 0 0 1 2.582 0l7 5.999A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />`,
	"user":                  `<path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2" /><circle cx="12" cy="7" r="4" />`,
	"users":                 `<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><path d="M16 3.128a4 4 0 0 1 0 7.744" /><path d="M22 21v-2a4 4 0 0 0-3-3.87" /><circle cx="9" cy="7" r="4" />`,
	"sliders-horizontal":    `<line x1="21" x2="14" y1="4" y2="4" /><line x1="10" x2="3" y1="4" y2="4" /><line x1="21" x2="12" y1="12" y2="12" /><line x1="8" x2="3" y1="12" y2="12" /><line x1="21" x2="16" y1="20" y2="20" /><line x1="12" x2="3" y1="20" y2="20" /><line x1="14" x2="14" y1="2" y2="6" /><line x1="8" x2="8" y1="10" y2="14" /><line x1="16" x2="16" y1="18" y2="22" />`,
	"settings":              `<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" /><circle cx="12" cy="12" r="3" />`,
	"menu":                  `<path d="M4 12h16" /><path d="M4 6h16" /><path d="M4 18h16" />`,
	"grip-vertical":         `<circle cx="9" cy="5" r="1" /><circle cx="9" cy="12" r="1" /><circle cx="9" cy="19" r="1" /><circle cx="15" cy="5" r="1" /><circle cx="15" cy="12" r="1" /><circle cx="15" cy="19" r="1" />`,
	"list-chevrons-up-down": `<path d="M3 5h8" /><path d="M3 12h8" /><path d="M3 19h8" /><path d="m15 8 3-3 3 3" /><path d="m15 16 3 3 3-3" />`,
	"moon":                  `<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />`,
	"sun":                   `<circle cx="12" cy="12" r="4" /><path d="M12 2v2" /><path d="M12 20v2" /><path d="m4.93 4.93 1.41 1.41" /><path d="m17.66 17.66 1.41 1.41" /><path d="M2 12h2" /><path d="M20 12h2" /><path d="m6.34 17.66-1.41 1.41" /><path d="m19.07 4.93-1.41 1.41" />`,
}

// Icon renders a Lucide glyph by name at the shared stroke width, with the given
// utility classes for size and colour. An unknown name falls back to a generic device
// glyph rather than rendering nothing, so a typo shows up instead of silently
// vanishing. It is registered as the "icon" template function on both the widget
// renderer and the server's page templates. Plugin-supplied names reach it only as a
// map key (never emitted), and the class is escaped, so it stays XSS-safe.
func Icon(name, class string) template.HTML {
	if glyph, ok := customGlyphs[name]; ok {
		return customIcon(name, glyph, class)
	}
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

// customGlyph is a shell-owned mark that does not fit the Lucide stroke model — a
// filled figure with its own geometry (ADR-005 §5, the same footing as the port
// connectors). name + viewBox + inner elements.
type customGlyph struct {
	viewBox string
	inner   string
}

// customGlyphs hold the non-Lucide art the shell draws itself. The running fan is
// the one icon the Hardware page carves out of the Lucide set: a multi-blade
// impeller that rotates while the fan runs (the CSS class verso-glyph-fan-spin spins
// it, honouring prefers-reduced-motion) — a stalled or absent header falls back to
// the static Lucide "fan" instead. Its blades take currentColor; its ring is the
// same colour at low opacity, so one ink drives the whole mark in light and dark.
var customGlyphs = map[string]customGlyph{
	"fan-spin": {
		viewBox: "60 60 492 492",
		inner: `<circle cx="306" cy="306" r="242" fill="none" stroke="currentColor" stroke-opacity="0.28" stroke-width="12"/>` +
			`<path fill="currentColor" d="M524.976,260.924c-1.238-6.015-3.819-10.307-8.554-14.218c-37.84-31.272-90.317-29.17-157.433,6.305c-4.38-4.38-9.121-8.082-14.434-11.27c46.257-41.499,88.423-58.921,126.495-52.269c11.225,1.961,18.998-10.901,12.043-19.926c-15.437-20.035-33.006-36.347-54.128-50.259c-5.13-3.378-9.989-4.586-16.103-4.005c-48.869,4.645-84.49,43.238-106.863,115.78c-6.195,0-12.165,0.734-18.175,2.237c3.365-62.052,20.86-104.187,52.486-126.405c9.324-6.55,5.725-21.141-5.574-22.607c-25.083-3.251-49.04-2.363-73.813,2.737c-6.015,1.238-10.307,3.819-14.218,8.554c-31.272,37.84-29.17,90.317,6.305,157.433c-4.38,4.38-8.082,9.121-11.27,14.434c-41.499-46.257-58.921-88.423-52.269-126.495c1.961-11.225-10.901-18.998-19.926-12.044c-20.035,15.437-36.347,33.006-50.259,54.128c-3.378,5.13-4.586,9.989-4.005,16.103c4.645,48.869,43.238,84.49,115.78,106.863c0,6.195,0.734,12.165,2.237,18.175c-62.052-3.365-104.187-20.86-126.405-52.486c-6.55-9.324-21.141-5.725-22.607,5.574c-3.251,25.083-2.363,49.04,2.737,73.813c1.238,6.015,3.819,10.307,8.554,14.218c37.84,31.272,90.317,29.17,157.433-6.305c4.38,4.38,9.121,8.082,14.434,11.27c-46.257,41.499-88.423,58.921-126.495,52.269c-11.225-1.961-18.998,10.901-12.043,19.926c15.437,20.035,33.006,36.347,54.128,50.259c5.13,3.378,9.989,4.586,16.103,4.005c48.869-4.645,84.49-43.238,106.863-115.78c6.195,0,12.165-0.734,18.175-2.238c-3.365,62.052-20.86,104.187-52.486,126.405c-9.324,6.55-5.725,21.141,5.574,22.607c25.083,3.251,49.04,2.363,73.813-2.737c6.015-1.238,10.307-3.819,14.218-8.554c31.272-37.84,29.17-90.317-6.305-157.433c4.38-4.38,8.082-9.121,11.27-14.434c41.499,46.257,58.921,88.423,52.269,126.495c-1.961,11.225,10.901,18.998,19.926,12.043c20.035-15.437,36.347-33.006,50.259-54.128c3.378-5.13,4.586-9.989,4.005-16.103c-4.645-48.869-43.238-84.49-115.78-106.863c0-6.195-0.734-12.165-2.237-18.175c62.052,3.365,104.187,20.86,126.405,52.486c6.55,9.324,21.141,5.725,22.607-5.574C530.964,309.653,530.076,285.698,524.976,260.924z M306,368.449c-34.489,0-62.449-27.96-62.449-62.449s27.96-62.449,62.449-62.449s62.449,27.96,62.449,62.449S340.489,368.449,306,368.449z"/>`,
	},
}

// customIcon wraps a shell-owned glyph in its own viewBox (no Lucide stroke
// frame), tagging it verso-glyph-<name> so the stylesheet can animate it.
func customIcon(name string, g customGlyph, class string) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg class="verso-glyph-`)
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(template.HTMLEscapeString(class))
	b.WriteString(`" viewBox="`)
	b.WriteString(g.viewBox)
	b.WriteString(`" aria-hidden="true">`)
	b.WriteString(g.inner)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
