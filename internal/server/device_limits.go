// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/deviceicon"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// deviceLimits reads all configured devices once from the plugin that owns
// their limits. The shell never interprets that plugin's UCI configuration.
// An unavailable read is distinct from a successful, empty configuration.
func (s *Server) deviceLimits(r *http.Request) (devices []widget.Device, offered, available bool) {
	for _, m := range s.manifestList() {
		for _, tab := range m.EntityTabs {
			if tab.Entity != "device" || tab.Slot != "shape" || !tab.Summaries {
				continue
			}
			if !s.probe(m.Socket) {
				return nil, true, false
			}
			env, err := s.fetchEntity(r.Context(), r, entityClaim{EntityTab: tab, PluginID: m.ID, Socket: m.Socket}, "device", "")
			if err != nil || env == nil || env.SchemaVersion != supportedSchemaVersion || env.Entities == nil {
				return nil, true, false
			}
			seen := map[string]bool{}
			tr := s.pluginTranslators(r)(m.ID)
			for _, summary := range *env.Entities {
				mac, err := net.ParseMAC(summary.ID)
				if err != nil || len(mac) != 6 || summary.State == "" || seen[mac.String()] {
					continue
				}
				id := mac.String()
				seen[id] = true
				name := summary.Name
				if name == "" {
					name = id
				}
				devices = append(devices, widget.Device{
					MAC: id, Name: name, Limit: tr(summary.State), LimitDetails: entitySummaryDetails(summary.Details, tr), LimitTip: deviceLimitTip(summary.Details, tr), Presence: "offline",
					Maker: deviceicon.Maker(id), Icon: deviceicon.Resolve(id, name),
					Lease: "No DHCP lease",
				})
			}
			return devices, true, true
		}
	}
	return nil, false, false
}

func entitySummaryDetails(details []plugin.EntitySummaryDetail, tr func(string) string) string {
	var lines []string
	for _, detail := range details {
		values := make([]string, len(detail.Values))
		for i, value := range detail.Values {
			values[i] = tr(value)
		}
		if len(values) > 0 {
			lines = append(lines, fmt.Sprintf(tr("%s: %s"), tr(detail.Label), strings.Join(values, ", ")))
		}
	}
	return strings.Join(lines, "\n")
}

func mergeDeviceLimits(roster, configured []widget.Device) []widget.Device {
	index := make(map[string]int, len(roster))
	for i, d := range roster {
		index[strings.ToLower(d.MAC)] = i
	}
	for _, d := range configured {
		if i, ok := index[d.MAC]; ok {
			roster[i].Limit = d.Limit
			roster[i].LimitDetails = d.LimitDetails
			roster[i].LimitTip = d.LimitTip
		} else {
			index[d.MAC] = len(roster)
			roster = append(roster, d)
		}
	}
	sortDevices(roster)
	return roster
}

// deviceLimitTip turns typed facts into the shell's schedule and rate readout.
// The words can translate independently of the kinds that choose the layout.
func deviceLimitTip(details []plugin.EntitySummaryDetail, tr func(string) string) *widget.DeviceLimitTip {
	tip := &widget.DeviceLimitTip{}
	for _, detail := range details {
		if len(detail.Values) == 0 {
			continue
		}
		value := detail.Values[0]
		switch detail.Kind {
		case "block":
			tip.Title, tip.Hours = tr("Internet block"), tr(value)
		case "weekdays":
			tip.Title = tr("Internet block")
			for _, day := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
				active := false
				for _, selected := range detail.Values {
					active = active || selected == day
				}
				tip.Days = append(tip.Days, widget.DeviceLimitDay{Label: tr(day), Active: active})
			}
		case "hours":
			tip.Clock = tr("Router time")
			if from, until, both := strings.Cut(value, "–"); both {
				tip.From, tip.Until = from, until
			} else {
				tip.Hours = tr(value)
			}
		case "from":
			tip.From, tip.Qualifier, tip.Clock = value, tr("From"), tr("Router time")
		case "until":
			tip.Until, tip.Qualifier, tip.Clock = value, tr("Until"), tr("Router time")
		case "download", "upload":
			rate := widget.DeviceLimitRate{Label: tr(detail.Label), Value: tr(value), Icon: detail.Kind}
			if number, ok := strings.CutSuffix(value, " Mbit/s"); ok {
				rate.Value, rate.Unit = number, "Mbit/s"
			}
			tip.Rates = append(tip.Rates, rate)
		}
	}
	if tip.Title == "" && len(tip.Rates) == 0 {
		return nil
	}
	return tip
}
