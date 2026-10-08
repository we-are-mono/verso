// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/we-are-mono/verso/internal/usage"
	"github.com/we-are-mono/verso/internal/widget"
)

// The shell's own tab on a device's panel, Details, ahead of the tabs plugins
// contribute: the machine facts the roster does not show and, while usage is
// on, what the device moves (ADR-018). It is a tab of its own, never a recap
// above a plugin's form.

// deviceTabs are the shell's tabs for one device.
func (s *Server) deviceTabs(r *http.Request, d widget.Device) []entityTab {
	body := widget.Widgets{deviceFacts(d)}
	if s.readUsageHistory(r.Context(), s.sessionSID(r)) {
		// The graph opens on the minute the roster's stream has watched, and
		// the stream feeds it from there; nothing here reads the router again.
		owners := addressOwners([]widget.Device{d})
		down, up := s.usageSampler.Series(owners, strings.ToLower(d.MAC))
		body = append(body, deviceUsage(d, down, up, s.usageHistory.Periods())...)
	}
	return []entityTab{{
		Slot: "details", Label: "Details",
		Href: entityPath + "device/" + url.PathEscape(d.MAC) + "?tab=details",
		Body: &widget.Stack{Children: body},
	}}
}

// deviceFacts is what the device is, in machine facts: its MAC, who made it,
// every address it holds, its DHCP identity and its lease.
func deviceFacts(d widget.Device) widget.Widget {
	items := []widget.Property{{Label: "MAC address", Value: d.MAC, Mono: true, Copy: true}}
	if d.Maker != "" {
		items = append(items, widget.Property{Label: "Maker", Value: d.Maker, Verbatim: true})
	}
	seen := map[string]bool{}
	for _, a := range d.Addresses {
		if seen[a.Addr] {
			continue
		}
		seen[a.Addr] = true
		items = append(items, widget.Property{Label: a.Family, Value: a.Addr, Mono: true, Copy: true})
	}
	for _, a := range []struct{ family, addr string }{{"IPv4", d.V4}, {"IPv6", d.V6}} {
		if a.addr != "" && !seen[a.addr] {
			items = append(items, widget.Property{Label: a.family, Value: a.addr, Mono: true, Copy: true})
		}
	}
	if d.DUID != "" {
		items = append(items, widget.Property{Label: "DUID", Value: d.DUID, Mono: true, Copy: true})
	}
	if d.Lease != "" {
		items = append(items, widget.Property{Label: "Lease", Value: d.Lease})
	}
	return &widget.Properties{Align: "left", Items: items}
}

// deviceUsage is what the device moves: its traffic graph, as the overview
// draws the Internet's, fed by the roster's stream under the name of the
// device, and its calendar periods. The series are bits per second.
func deviceUsage(d widget.Device, downBPS, upBPS []float64, p usage.Periods) widget.Widgets {
	mac := strings.ToLower(d.MAC)
	down, up := make([]float64, len(downBPS)), make([]float64, len(upBPS))
	for i := range downBPS {
		down[i], up[i] = downBPS[i]/1_000_000, upBPS[i]/1_000_000
	}
	traffic := &widget.Traffic{
		Title: "Usage", Live: "usage:" + mac,
		Label:   "Device traffic — download and upload, last minute",
		DownNow: fmt.Sprintf("%.1f", down[len(down)-1]), UpNow: fmt.Sprintf("%.1f", up[len(up)-1]),
		Down: down, Up: up, Panel: true,
	}
	period := func(label string, totals map[string]usage.Totals) widget.TableRow {
		t := totals[mac]
		return widget.TableRow{Cells: []widget.TableCell{{Text: label}, {Text: usage.Bytes(t.Down)}, {Text: usage.Bytes(t.Up)}}}
	}
	periods := &widget.Table{
		Style: "flat",
		Columns: []widget.TableColumn{
			{Label: "Period"},
			// Volumes, so the words say what was moved: "Download" is the
			// button's verb wherever else it stands, and translates as one.
			{Label: "Downloaded", Kind: "num", Width: widget.MeasureWord},
			{Label: "Uploaded", Kind: "num", Width: widget.MeasureWord},
		},
		Rows: []widget.TableRow{
			period("Today", p.Today),
			period("Last 7 days", p.Last7),
			period("This month", p.ThisMonth),
			period("Last month", p.LastMonth),
		},
	}
	return widget.Widgets{traffic, periods}
}
