// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"strings"
)

// overviewCount translates the entire phrase, including Slovenian's dual and
// paucal forms. Values remain arguments; no sentence is assembled from suffixes.
func overviewCount(tr func(string) string, n int, one, two, few, many string) string {
	key := many
	switch n % 100 {
	case 1:
		key = one
	case 2:
		key = two
	case 3, 4:
		key = few
	}
	pattern := tr(key)
	// Contextual plural keys have an ordinary English fallback; their context
	// markers are never displayed when a catalog is missing.
	if pattern == key {
		pattern = many
		if n == 1 {
			pattern = one
		}
	}
	return fmt.Sprintf(pattern, n)
}

func interfaceCount(tr func(string) string, n int) string {
	return overviewCount(tr, n, "%d interface", "%d interfaces (two)", "%d interfaces (few)", "%d interfaces")
}

func deviceCount(tr func(string) string, n int) string {
	return overviewCount(tr, n, "%d device connected", "%d devices connected (two)", "%d devices connected (few)", "%d devices connected")
}

func (o *Overview) firewallTile(tr func(string) string) ohTile {
	tile := ohTile{ID: "firewall", Label: tr("Firewall"), Icon: "shield", Variant: "neutral", Status: tr("Status unavailable"), Href: o.SecurityHref}
	if tile.Href != "" {
		tile.Caption = tr("Review firewall settings")
	}
	switch o.FirewallState {
	case "active":
		tile.Status, tile.Variant = tr("Active"), "success"
		tile.Caption = tr("Ruleset loaded")
		if o.FirewallRulesKnown {
			tile.Caption = overviewCount(tr, o.FirewallRules, "%d configured rule", "%d configured rules (two)", "%d configured rules (few)", "%d configured rules")
		}
	case "inactive":
		tile.Status, tile.Variant = tr("Inactive"), "danger"
		tile.Caption = tr("Firewall is not running")
	case "partial":
		tile.Status, tile.Variant = tr("Incomplete"), "warning"
		tile.Caption = tr("Some filter chains are missing")
	}
	return tile
}

func (o *Overview) interfacesTile(tr func(string) string) ohTile {
	tile := ohTile{ID: "interfaces", Label: tr("Interfaces"), Icon: "ethernet-port", Variant: "neutral", Status: tr("Unavailable"), Href: o.InterfacesHref}
	if !o.InterfacesKnown {
		return tile
	}
	tile.Caption = interfaceCount(tr, len(o.Interfaces))
	if o.DevicesKnown {
		tile.Caption += " · " + deviceCount(tr, o.DevicesOnline)
	}
	ports, down, unknown := 0, 0, 0
	for _, n := range o.Interfaces {
		if !n.Physical {
			continue
		}
		ports++
		if n.State != "up" && n.State != "down" && n.State != "lowerlayerdown" {
			unknown++
		}
		if n.State == "down" || n.State == "lowerlayerdown" {
			down++
			if tile.Identity == "" {
				tile.Identity = n.Name
			}
		}
	}
	switch {
	case down > 0:
		tile.Variant = "warning"
		tile.Status = overviewCount(tr, down, "%d port without link", "%d ports without link (two)", "%d ports without link (few)", "%d ports without link")
	case unknown > 0:
		tile.Status = tr("State unknown")
	case ports > 0:
		tile.Status, tile.Variant = tr("All ports linked"), "success"
	default:
		tile.Status = tr("Virtual interfaces")
	}
	return tile
}

type overviewTunnel struct{ Name, Tone string }

func (o *Overview) tunnels() []overviewTunnel {
	var rows []overviewTunnel
	for _, n := range o.Interfaces {
		if n.Kind != "tunnel" {
			continue
		}
		row := overviewTunnel{Name: n.Name, Tone: "neutral"}
		// A netdev's state says nothing about a remote peer or a handshake.
		switch n.State {
		case "up":
			row.Tone = "success"
		case "down", "lowerlayerdown":
			row.Tone = "danger"
		}
		rows = append(rows, row)
	}
	return rows
}

func (o *Overview) tunnelTile(tr func(string) string) ohTile {
	tile := ohTile{ID: "tunnels", Label: tr("Tunnels"), Icon: "lock", Variant: "neutral", Status: tr("Unavailable"), Href: o.TunnelsHref}
	if tile.Href == "" {
		tile.Href = o.InterfacesHref
	}
	if !o.InterfacesKnown {
		return tile
	}
	rows := o.tunnels()
	if len(rows) == 0 {
		tile.Status = tr("None observed")
		return tile
	}
	tile.Caption = interfaceCount(tr, len(rows))
	up, down := 0, 0
	for _, row := range rows {
		if row.Tone == "success" {
			up++
		}
		if row.Tone == "danger" {
			down++
			if tile.Identity == "" {
				tile.Identity = row.Name
			}
		}
	}
	switch {
	case down > 0:
		tile.Status, tile.Variant = fmt.Sprintf(tr("%d of %d down"), down, len(rows)), "danger"
	case up == len(rows):
		tile.Status, tile.Variant = tr("All interfaces up"), "success"
	default:
		tile.Status = tr("State unknown")
	}
	return tile
}

func (o *Overview) masthead(tr func(string) string) overviewMastheadView {
	v := overviewMastheadView{Tone: "neutral", Kicker: tr("Live overview"), Clock: o.Clock, Zone: o.Zone, Uptime: o.Uptime, Unix: o.Unix, Offset: o.Offset}
	v.Lead = tr("Your network at a glance")
	if !o.WANKnown {
		v.Kicker = tr("Status unavailable")
		return v
	}
	if !o.WANUp {
		v.Tone, v.Kicker = "danger", tr("No internet connection")
		v.Lead = tr("Your network is offline")
		return v
	}
	issues := 0
	if o.FirewallState == "inactive" || o.FirewallState == "partial" {
		issues++
	}
	for _, n := range o.Interfaces {
		if (n.Physical || n.Kind == "tunnel") && (n.State == "down" || n.State == "lowerlayerdown") {
			issues++
		}
	}
	for _, meter := range o.SysMetrics {
		if meter.Band == "warning" || meter.Band == "danger" {
			issues++
		}
	}
	if issues > 0 {
		v.Tone = "warning"
		v.Kicker = overviewCount(tr, issues, "%d needs attention", "%d need attention (two)", "%d need attention (few)", "%d need attention")
		v.Lead = tr("Your network needs attention")
	} else {
		v.Tone, v.Kicker = "success", tr("Internet is working")
		// Keep a full sentence so translations can move the accent with grammar.
		before, after, found := strings.Cut(tr("Your network is %s"), "%s")
		v.Lead = before
		if found {
			v.Accent, v.Tail = tr("online"), after
		}
	}
	return v
}

// OverviewStatus is the live part of the masthead. The first paint and the SSE
// stream use the same reading; the browser changes text and tone, not policy.
type OverviewStatus struct {
	Kicker string               `json:"kicker"`
	Lead   string               `json:"lead"`
	Accent string               `json:"accent"`
	Tail   string               `json:"tail"`
	Tone   string               `json:"tone"`
	Tiles  []overviewTileStatus `json:"tiles"`
}

type overviewTileStatus struct {
	ID       string `json:"id"`
	Tone     string `json:"tone"`
	Status   string `json:"status"`
	Caption  string `json:"caption"`
	Identity string `json:"identity"`
}

func (o *Overview) LiveStatus(tr func(string) string) OverviewStatus {
	v := o.masthead(tr)
	status := OverviewStatus{Kicker: v.Kicker, Lead: v.Lead, Accent: v.Accent, Tail: v.Tail, Tone: v.Tone}
	for _, tile := range []ohTile{o.internetTile(tr), o.firewallTile(tr), o.tunnelTile(tr), o.interfacesTile(tr)} {
		status.Tiles = append(status.Tiles, overviewTileStatus{ID: tile.ID, Tone: tile.Variant, Status: tile.Status, Caption: tile.Caption, Identity: tile.Identity})
	}
	return status
}
