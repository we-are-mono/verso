// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// loginStatus is the complete public refresh payload. It intentionally cannot
// carry the WAN addresses, DNS servers, traffic, or inventories in WANState.
// A nil Internet means the read failed, rather than claiming the uplink is down.
type loginStatus struct {
	Internet *bool  `json:"internet"`
	Uptime   string `json:"uptime"`
	Clock    string `json:"clock"`
	Zone     string `json:"zone"`
	Unix     int64  `json:"unix"`
	Offset   int    `json:"offset"`
}

func (s loginStatus) InternetUp() bool { return s.Internet != nil && *s.Internet }

func (s *Server) readLoginStatus(tr func(string) string) loginStatus {
	now := time.Now()
	zone, offset := now.Zone()
	status := loginStatus{
		Uptime: loginDuration(tr, loginUptime()),
		Clock:  now.Format("15:04:05"), Zone: zone, Unix: now.Unix(), Offset: offset,
	}
	if up, err := s.loginInternet(); err == nil {
		status.Internet = &up
	}
	return status
}

// Compact duration patterns are translated as a whole, leaving each language
// free to order its units without singular/dual/plural sentence fragments.
func loginDuration(tr func(string) string, seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days, hours, minutes := seconds/86400, seconds%86400/3600, seconds%3600/60
	if days > 0 {
		return fmt.Sprintf(tr("%d d %02d:%02d"), days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf(tr("%d h %02d min"), hours, minutes)
	}
	return fmt.Sprintf(tr("%d min"), minutes)
}

func (s *Server) handleLoginStatus(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(s.readLoginStatus(translatorOrIdentity(t)))
}

func visitorNetwork(ip string) string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	return matchingNetwork(ip, addresses)
}

// matchingNetwork names only the visitor's directly connected subnet. A routed
// visitor gets no guessed network name; overlapping subnets use the longest
// prefix. These are kernel addresses, not a pre-auth read of UCI configuration.
func matchingNetwork(address string, addresses []net.Addr) string {
	ip := net.ParseIP(address)
	if ip == nil || ip.IsLoopback() {
		return ""
	}
	best, bits := "", -1
	for _, addr := range addresses {
		_, network, err := net.ParseCIDR(addr.String())
		if err != nil || !network.Contains(ip) {
			continue
		}
		if ones, _ := network.Mask.Size(); ones > bits && ones > 0 {
			best, bits = network.String(), ones
		}
	}
	return best
}
