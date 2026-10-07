// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
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
		rates, _ := s.usageSampler.Latest()
		body = append(body, deviceUsageSection(d, usage.Devices(rates, addressOwners([]widget.Device{d})), s.usageHistory.Periods()))
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

// deviceUsageSection is what the device moves: its rate now, down and up,
// named so the roster's stream keeps it current, and its calendar periods.
func deviceUsageSection(d widget.Device, rates map[string]usage.Rate, p usage.Periods) widget.Widget {
	mac := strings.ToLower(d.MAC)
	// No rate yet is the dash; an idle device reads as nothing moving, as the
	// stream that keeps these current says it.
	down, up := "—", "—"
	if r, ok := rates[mac]; ok {
		down, up = "0", "0"
		if usage.Busy(r) {
			down, up = usage.Mbits(r.Down), usage.Mbits(r.Up)
		}
	}
	now := &widget.Grid{Columns: 2, Children: widget.Widgets{
		&widget.Stat{Style: "bare", Label: "Download", Value: down, Unit: "Mbit/s", Verbatim: true, Name: "usage-down:" + mac},
		&widget.Stat{Style: "bare", Label: "Upload", Value: up, Unit: "Mbit/s", Verbatim: true, Name: "usage-up:" + mac},
	}}
	period := func(label string, totals map[string]usage.Totals) widget.TableRow {
		t := totals[mac]
		return widget.TableRow{Cells: []widget.TableCell{{Text: label}, {Text: usage.Bytes(t.Down)}, {Text: usage.Bytes(t.Up)}}}
	}
	periods := &widget.Table{
		Style: "flat",
		Columns: []widget.TableColumn{
			{Label: "Period"},
			{Label: "Download", Kind: "num", Width: widget.MeasureWord},
			{Label: "Upload", Kind: "num", Width: widget.MeasureWord},
		},
		Rows: []widget.TableRow{
			period("Today", p.Today),
			period("Last 7 days", p.Last7),
			period("This month", p.ThisMonth),
			period("Last month", p.LastMonth),
		},
	}
	return &widget.Section{Title: "Usage", Children: widget.Widgets{now, periods}}
}
