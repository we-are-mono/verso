// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// The restarting takeover is where every act that takes the router down ends: a
// reboot, a restored backup, a factory reset. The act has already happened when
// this paints, so the screen does the one thing left, which is watching the
// router go and come back. Like the firmware takeover it is chrome-less, rendered
// straight through the page-template set.
//
// The client (verso-takeover.js) polls restartStatusPath. While the router is
// still up the poll answers JSON. After the restart the in-memory session is
// gone, so the same request meets the login redirect, and that answer is the
// "back" signal. It needs no down-then-up sequence, which a fast restart could
// slip between two polls. If nothing comes back within the plan's budget, a calm
// surface opens a door instead of holding the person on the waiting mark.

const restartStatusPath = "/system/maintenance/restart/status"

// restartPlan is what one kind of restart says while it waits: its heading, what
// is happening, how long it usually takes, and how long before the screen stops
// waiting. Address is set only when the router comes back somewhere else.
type restartPlan struct {
	Title, Lede, Estimate string
	Budget                time.Duration
	Address               string
}

func rebootPlan(tr func(string) string) restartPlan {
	return restartPlan{
		Title:    tr("Restarting"),
		Lede:     tr("Closing everything down and coming back on the same firmware. Settings are untouched."),
		Estimate: tr("Usually about 45 seconds."),
		Budget:   3 * time.Minute,
	}
}

func restorePlan(tr func(string) string) restartPlan {
	return restartPlan{
		Title:    tr("Restoring your backup"),
		Lede:     tr("Writing the saved settings, then restarting. Everything configured since the backup is replaced."),
		Estimate: tr("Usually about two minutes."),
		Budget:   5 * time.Minute,
	}
}

// factoryResetPlan names where the router answers afterwards, because a reset
// can move it: the address is the board's default, not the one in the address
// bar. A board whose LAN has no fixed default gets no address rather than a guess.
func factoryResetPlan(tr func(string) string, address string) restartPlan {
	return restartPlan{
		Title:    tr("Erasing and restarting"),
		Lede:     tr("Settings, installed plugins and local data are being erased. The router comes back as it left the factory."),
		Estimate: tr("Usually about two minutes."),
		Budget:   5 * time.Minute,
		Address:  address,
	}
}

// restartingView is the takeover template's model. Every word is localized here
// or in the template; the client only moves [hidden] between the surfaces.
type restartingView struct {
	Lang, Title, Lede, Estimate, Status string
	Hostname, Maker, Model              string
	CSS                                 template.CSS
	Budget                              int
	Address                             template.HTML
}

// restartingPage renders the takeover before the act it follows. Every read it
// needs (the hostname, the catalog) happens while the router still answers, and
// a render failure refuses the act instead of stranding the person on an error
// after the router has gone.
func (s *Server) restartingPage(r *http.Request, plan restartPlan) (string, error) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	hw := board()
	view := restartingView{
		Lang: langAttr(lang), CSS: s.currentCSS(), Status: restartStatusPath,
		Title: plan.Title, Lede: plan.Lede, Estimate: plan.Estimate,
		Hostname: s.nameplate(r), Maker: hw.Maker, Model: hw.Model,
		Budget: int(plan.Budget / time.Second),
	}
	if plan.Address != "" {
		view.Address = verbatimIn(tr("It answers at %s afterwards. Reconnect this computer if it doesn't pick up an address on its own."), "http://"+plan.Address)
	}
	var body strings.Builder
	if err := s.pageSet(lang).ExecuteTemplate(&body, "restarting.html.tmpl", view); err != nil {
		return "", err
	}
	return body.String(), nil
}

func writeRestarting(w http.ResponseWriter, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, page)
}

// verbatimIn sets a machine string inside a translated sentence: the sentence
// stays one catalog entry with a %s, and the value rides in mono, escaped.
func verbatimIn(format, value string) template.HTML {
	mono := `<span class="font-mono text-base">` + template.HTMLEscapeString(value) + `</span>`
	return template.HTML(fmt.Sprintf(template.HTMLEscapeString(format), mono))
}

// handleRestartStatus is the poll: an answer at all, as JSON, means this session
// still lives, so the router has not restarted yet.
func (s *Server) handleRestartStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Up bool `json:"up"`
	}{Up: true})
}

// boardFactoryLAN is where this board answers after a reset, read once: the
// board.json that config_generate builds the default network from does not
// change while the system runs.
var boardFactoryLAN = sync.OnceValue(func() string {
	data, err := os.ReadFile("/etc/board.json")
	if err != nil {
		return ""
	}
	return factoryLANAddress(data)
})

// factoryLANAddress follows config_generate: a LAN with a device or ports and a
// static protocol takes the board's ipaddr, or 192.168.1.1. Any other LAN has no
// fixed address to name.
func factoryLANAddress(data []byte) string {
	var b struct {
		Network struct {
			LAN *struct {
				Device   string   `json:"device"`
				Ports    []string `json:"ports"`
				Protocol string   `json:"protocol"`
				IPAddr   string   `json:"ipaddr"`
			} `json:"lan"`
		} `json:"network"`
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return ""
	}
	lan := b.Network.LAN
	if lan == nil || (lan.Device == "" && len(lan.Ports) == 0) || lan.Protocol != "static" {
		return ""
	}
	if lan.IPAddr != "" {
		return lan.IPAddr
	}
	return "192.168.1.1"
}
