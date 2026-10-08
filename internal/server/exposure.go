// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"cmp"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/widget"
)

// The firewall decides who reaches the shell's listeners; Access says when the
// internet can (ADR-017 §1). What follows reads that from the firewall's config,
// so a rule written in raw nftables is not seen.

type exposedKind int

const (
	exposedZone    exposedKind = iota // a WAN zone accepting input
	exposedRule                       // a rule accepting a shell port from a WAN zone
	exposedForward                    // a port forward from a WAN zone to the router itself
)

// exposed is one reason the internet reaches the shell: its kind, the zone,
// rule or forward by the name it goes by, and the shell's port it reaches
// (empty for every port).
type exposed struct {
	kind exposedKind
	name string
	port string
}

// exposure is every reason in the firewall config fw that the internet reaches
// one of the shell's ports. A WAN zone is the one named "wan" or one holding a
// network the router routes its default traffic over (wanNetworks).
func exposure(fw map[string]any, wanNetworks []string, ports []string) []exposed {
	defaults := ""
	wanZones := map[string]bool{}
	for _, v := range fw {
		s := asSection(v)
		switch s[".type"] {
		case "defaults":
			defaults = option(s, "input")
		case "zone":
			name := option(s, "name")
			nets := words(s["network"])
			if name == "wan" || slices.ContainsFunc(nets, func(n string) bool { return slices.Contains(wanNetworks, n) }) {
				wanZones[name] = true
			}
		}
	}
	var out []exposed
	for key, v := range fw {
		s := asSection(v)
		if off(s) {
			continue
		}
		label := cmp.Or(option(s, "name"), key)
		switch s[".type"] {
		case "zone":
			if wanZones[option(s, "name")] && strings.EqualFold(cmp.Or(option(s, "input"), defaults), "ACCEPT") {
				out = append(out, exposed{exposedZone, option(s, "name"), ""})
			}
		case "rule":
			// A rule with a dest forwards through the router; one without is
			// traffic to the router itself. A rule accepts only when it says so.
			src := option(s, "src")
			if (src != "*" && !wanZones[src]) || option(s, "dest") != "" ||
				!strings.EqualFold(option(s, "target"), "ACCEPT") || !carriesTCP(s) {
				continue
			}
			if port, ok := reaches(option(s, "dest_port"), ports); ok {
				out = append(out, exposed{exposedRule, label, port})
			}
		case "redirect":
			// A DNAT with no dest_ip lands on the router itself, on dest_port,
			// which is src_dport when it names none.
			if !wanZones[option(s, "src")] || option(s, "dest_ip") != "" ||
				!strings.EqualFold(cmp.Or(option(s, "target"), "DNAT"), "DNAT") || !carriesTCP(s) {
				continue
			}
			if port, ok := reaches(cmp.Or(option(s, "dest_port"), option(s, "src_dport")), ports); ok && port != "" {
				out = append(out, exposed{exposedForward, label, port})
			}
		}
	}
	slices.SortFunc(out, func(a, b exposed) int {
		return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.name, b.name))
	})
	return out
}

func option(s map[string]any, key string) string {
	v, _ := s[key].(string)
	return strings.TrimSpace(v)
}

// words is a list option, or a single option holding several words.
func words(v any) []string {
	var out []string
	for _, item := range stringList(v) {
		out = append(out, strings.Fields(item)...)
	}
	return out
}

// off is a section switched off, as fw4 reads `enabled`.
func off(s map[string]any) bool {
	switch strings.ToLower(option(s, "enabled")) {
	case "0", "no", "off", "false", "disabled":
		return true
	}
	return false
}

// carriesTCP is whether a rule or forward matches TCP, the shell's protocol:
// fw4 reads no proto as TCP and UDP both.
func carriesTCP(s map[string]any) bool {
	protos := words(s["proto"])
	if len(protos) == 0 {
		return true
	}
	return slices.ContainsFunc(protos, func(p string) bool {
		switch strings.ToLower(p) {
		case "tcp", "tcpudp", "all", "any", "6":
			return true
		}
		return false
	})
}

// reaches is which of ports a fw4 port match covers: the first it covers, or
// "" for a match naming no port at all, which covers every one.
func reaches(match string, ports []string) (string, bool) {
	if match == "" {
		return "", len(ports) > 0
	}
	for _, port := range ports {
		p, err := strconv.Atoi(port)
		if err != nil {
			continue
		}
		for _, item := range strings.Fields(match) {
			lo, hi, isRange := strings.Cut(strings.ReplaceAll(item, ":", "-"), "-")
			a, errA := strconv.Atoi(lo)
			b, errB := strconv.Atoi(hi)
			if errA == nil && ((!isRange && a == p) || (isRange && errB == nil && a <= p && p <= b)) {
				return port, true
			}
		}
	}
	return "", false
}

// exposureCallout says, on Access, that the internet can reach the shell and
// why, or nothing when the firewall keeps it off the WAN or cannot be read.
func (s *Server) exposureCallout(r *http.Request, tr func(string) string) widget.Widget {
	if s.listeners == nil {
		return nil
	}
	c := s.listeners.Current()
	var ports []string
	for _, addr := range append(slices.Clone(c.HTTPS), c.HTTP...) {
		if _, port, err := net.SplitHostPort(addr); err == nil && !slices.Contains(ports, port) {
			ports = append(ports, port)
		}
	}
	fw, err := s.backend.UCIConfig(r.Context(), s.sessionSID(r), "firewall")
	if err != nil {
		return nil
	}
	var networks []string
	if wan, err := s.wanStatus(r); err == nil {
		for _, d := range wan.Devices {
			networks = append(networks, d.Networks...)
		}
	}
	found := exposure(fw, networks, ports)
	if len(found) == 0 {
		return nil
	}
	var why []string
	for _, e := range found {
		switch {
		case e.kind == exposedZone:
			why = append(why, fmt.Sprintf(tr("The firewall zone “%s” accepts every connection from the internet."), e.name))
		case e.kind == exposedRule && e.port == "":
			why = append(why, fmt.Sprintf(tr("The rule “%s” lets the internet reach every port on the router."), e.name))
		case e.kind == exposedRule:
			why = append(why, fmt.Sprintf(tr("The rule “%s” lets the internet reach port %s."), e.name, e.port))
		default:
			why = append(why, fmt.Sprintf(tr("The port forward “%s” sends the internet to port %s on the router."), e.name, e.port))
		}
	}
	return &widget.Callout{
		Variant: "warning",
		Title:   "The internet can reach this web interface",
		Body:    strings.Join(why, " "),
		Link:    &widget.Link{Label: "Open Firewall", Href: "/plugins/firewall/"},
	}
}
