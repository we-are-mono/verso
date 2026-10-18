// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"io/fs"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/ports"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// portKinds says which connector a port is: "sfp" or "rj45" (ports.Kinds).
type portKinds interface {
	Kind(iface string) string
}

// readPanel loads a profile folder's embedded panel artwork, empty when the folder
// ships none. The art is first-party content compiled into the binary, so the
// Overview renders it as trusted core content (the runtime-contribution
// sanitizer the profile README specifies is only needed once profiles arrive at
// runtime). dir is the profile's own folder (LoadProfile records it), so the art
// travels with the profile, not with the raw board_name.
func readPanel(dir, file string) string {
	if dir == "" {
		return ""
	}
	data, err := fs.ReadFile(profiles.FS, dir+"/"+file)
	if err != nil {
		return ""
	}
	return string(data)
}

// devicePanel is the router's panel, as the Overview's Ports card and the
// Hardware page draw it: the profile's own art when it ships one, else the
// panel the widget generates, with every port in its place on the case and
// carrying its live link, activity, speed and WAN role. It reads the page's
// own snapshot and WAN state, so the panel agrees with the rest of the page.
// A box with no ports gets none.
func (s *Server) devicePanel(snapshot telemetry.Snapshot, wan openwrt.WANState, profile *sensors.Profile) *widget.Ports {
	var profileOrder []string
	if profile != nil {
		profileOrder = profile.Ports
	}
	order := ports.Order(snapshot.Interfaces, profileOrder, ports.BoardOrder(s.boardJSON()))
	if len(order) == 0 {
		return nil
	}
	wanPorts := wanPorts(snapshot, wan)
	items := make([]widget.PortItem, 0, len(order))
	for _, name := range order {
		item := widget.PortItem{Kind: s.portKinds.Kind(name), Label: name, Verbatim: true, Iface: name, Speed: ports.SpeedLabel(-1)}
		if iface, ok := snapshot.Interface(name); ok {
			item.Linked = iface.Operstate == "up"
			if n := len(iface.History); n > 0 {
				point := iface.History[n-1]
				item.Activity = ports.Activity(point.RxPPS + point.TxPPS)
			}
			if item.Linked {
				item.Speed = ports.SpeedLabel(iface.SpeedMbps)
			}
		}
		if wanPorts[name] {
			item.Role = "wan"
		}
		items = append(items, item)
	}
	panel := &widget.Ports{Items: items}
	if profile != nil {
		panel.Back = readPanel(profile.Dir, "back.svg")
		panel.Front = readPanel(profile.Dir, "front.svg")
	}
	return panel
}

// wanPorts are the ports the WAN rides on: each routed WAN device and the
// devices under it, down the same parents the Interfaces topology draws
// (deviceParents: a VLAN's port, a PPPoE session's transport).
func wanPorts(snapshot telemetry.Snapshot, wan openwrt.WANState) map[string]bool {
	parents := deviceParents(snapshot, wan)
	marked := map[string]bool{}
	for _, d := range wan.Devices {
		for name, depth := d.Device, 0; name != "" && !marked[name] && depth < 8; name, depth = parents[name], depth+1 {
			marked[name] = true
		}
	}
	return marked
}
