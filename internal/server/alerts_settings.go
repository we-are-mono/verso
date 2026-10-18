// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/alerts"
	"github.com/we-are-mono/verso/internal/widget"
)

const (
	alertsPath     = eventsPath + "/alerts"
	alertsTestPath = alertsPath + "/test"
	alertsSection  = "alerts"
)

// alertsConfig is verso.alerts as the alerter takes it (ADR-020).
func alertsConfig(verso map[string]any) alerts.Config {
	section, _ := verso[alertsSection].(map[string]any)
	return alerts.Config{
		URL:       uciString(section["url"]),
		Heartbeat: uciString(section["heartbeat"]),
		Kinds:     uciList(section["kind"]),
	}
}

// refreshAlerts hands the running alerter verso.alerts as the confirmed
// apply left it, read under the confirming operator's session.
func (s *Server) refreshAlerts(ctx context.Context, sid string) {
	if s.alerter == nil {
		return
	}
	config, err := s.backend.UCIConfig(ctx, sid, versoConfig)
	if err != nil {
		log.Printf("verso: alerts: settings not reread: %v", err)
		return
	}
	s.alerter.Configure(alertsConfig(config))
}

// webAddress is whether an alert can be posted to raw.
func webAddress(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// handleAlerts is where alerts go, the heartbeat, and which kinds alert: a
// panel over System → Events, staged into verso.alerts like any setting.
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if s.alerter == nil {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	sid := s.sessionSID(r)
	verso, err := s.backend.UCIConfig(r.Context(), sid, versoConfig)
	status, message := http.StatusOK, ""
	if err != nil {
		status, message = http.StatusBadGateway, "Alert settings could not be read."
	}
	cfg := alertsConfig(verso)
	if r.Method == http.MethodPost && status == http.StatusOK {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		next := alerts.Config{
			URL:       strings.TrimSpace(r.PostForm.Get("url")),
			Heartbeat: strings.TrimSpace(r.PostForm.Get("heartbeat")),
			Kinds:     r.PostForm["kind"],
		}
		switch err := s.stageAlerts(r.Context(), sid, verso, next); {
		case errors.Is(err, errAlertsAddress):
			status, message, cfg = http.StatusUnprocessableEntity, "Enter a web address starting with https:// or http://.", next
		case err != nil:
			log.Printf("verso: alerts: %v", err)
			status, message, cfg = http.StatusBadGateway, "Alert settings could not be saved.", next
		default:
			// Staged, so not yet done: the chip says a change waits.
			panelRedirect(w, r, eventsPath)
			return
		}
	}
	kinds := cfg.Kinds
	if kinds == nil {
		kinds = alerts.DefaultKinds
	}
	children := []widget.Widget{}
	if message != "" {
		children = append(children, &widget.Callout{Variant: "danger", Compact: true, Body: message})
	}
	g := s.grantsFor(sid)
	children = append(children, &widget.Form{Action: alertsPath, Submit: "Save alerts", Fields: []widget.Widget{
		&widget.Field{Name: "url", Label: "Alert address", Value: cfg.URL, Placeholder: "https://ntfy.sh/your-topic",
			Help: s.alertsHelp(r, g, tr)},
		&widget.Field{Name: "heartbeat", Label: "Heartbeat address", Value: cfg.Heartbeat, Placeholder: "Optional",
			Help: "The router calls this every five minutes, and a monitor there (Healthchecks.io, Uptime Kuma) tells you when the calls stop: the one way to hear of an outage while it lasts."},
		&widget.Field{Name: "kind", Label: "Alert when", Kind: "checks", Values: kinds, Options: []widget.Option{
			{Value: alerts.KindWAN, Label: "The internet goes down and comes back"},
			{Value: alerts.KindBoot, Label: "The router restarts after losing power or crashing"},
			{Value: alerts.KindRollback, Label: "Applied changes are rolled back because nobody could confirm them"},
			{Value: alerts.KindStorage, Label: "History cannot be saved"},
		}},
	}})
	if s.alerter.Configured() && mayTestAlerts(r.Context(), g) {
		children = append(children, &widget.Form{Action: alertsTestPath, NoSubmit: true, Fields: []widget.Widget{
			&widget.Button{Label: "Send a test alert", Busy: "Sending…", Style: "act", Icon: "send"},
		}})
	}
	if panelRequest(r) {
		s.writeSystemPanel(w, r, status, "Alerts", children)
		return
	}
	var content strings.Builder
	if err := s.widgets.RenderWithToken(&content, &widget.Stack{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, status, pageHeader{Heading: "Alerts", Tone: "neutral"}, "narrow",
		s.sectionPages("System", eventsPath), template.HTML(content.String())) //nolint:gosec // rendered by the shell's own templates
}

var errAlertsAddress = errors.New("not a web address")

// stageAlerts writes next into verso.alerts: an empty address is removed,
// absent meaning off; the default kinds leave no list, absent meaning them;
// no kind chosen is said as "none".
func (s *Server) stageAlerts(ctx context.Context, sid string, verso map[string]any, next alerts.Config) error {
	for _, address := range []string{next.URL, next.Heartbeat} {
		if address != "" && !webAddress(address) {
			return errAlertsAddress
		}
	}
	section, present := verso[alertsSection].(map[string]any)
	if !present {
		if _, err := s.backend.UCIAdd(ctx, sid, versoConfig, alertsSection, alertsSection); err != nil {
			return err
		}
	}
	set := map[string]any{}
	var drop []string
	for option, value := range map[string]string{"url": next.URL, "heartbeat": next.Heartbeat} {
		switch {
		case value != "":
			set[option] = value
		case section[option] != nil:
			drop = append(drop, option)
		}
	}
	chosen := slices.Clone(next.Kinds)
	slices.Sort(chosen)
	defaults := slices.Sorted(slices.Values(alerts.DefaultKinds))
	switch {
	case slices.Equal(chosen, defaults):
		if section["kind"] != nil {
			drop = append(drop, "kind")
		}
	case len(chosen) == 0:
		set["kind"] = []string{"none"}
	default:
		set["kind"] = chosen
	}
	if len(set) > 0 {
		if err := s.backend.UCISet(ctx, sid, versoConfig, alertsSection, set); err != nil {
			return err
		}
	}
	for _, option := range drop {
		if err := s.backend.UCIDelete(ctx, sid, versoConfig, alertsSection, option); err != nil {
			return err
		}
	}
	return nil
}

// The alert settings are verso.alerts, read and staged through rpcd; how the
// last alert went and a test alert are the shell's own, from the outbox in
// history.db and the running alerter, so they reach a session only under the
// grants of the settings they concern (ADR-007): the last alert under the
// read, a test under the read and the write.
var (
	alertsReadGrant  = configGrant(versoConfig, "read")
	alertsWriteGrant = configGrant(versoConfig, "write")
)

// mayTestAlerts reports whether the session may send a test alert.
func mayTestAlerts(ctx context.Context, g *sessionGrants) bool {
	return g.may(ctx, alertsReadGrant) && g.may(ctx, alertsWriteGrant)
}

// alertsHelp is the alert address's description: what it is, and how the
// last alert went to a session that may read the settings.
func (s *Server) alertsHelp(r *http.Request, g *sessionGrants, tr func(string) string) string {
	help := tr("Each alert is posted here with ntfy's headers; any webhook reads it. While the internet is down, alerts wait and arrive when it returns, unless this address is on your network.")
	if !g.may(r.Context(), alertsReadGrant) {
		return help
	}
	last, found, err := s.historyStore.LastAlert(r.Context())
	if err != nil || !found {
		return help
	}
	when := eventTime(last.LastTry, time.Now())
	switch last.State {
	case "sent":
		return help + " " + fmt.Sprintf(tr("The last alert arrived at %s."), when)
	case "pending":
		return help + " " + fmt.Sprintf(tr("The last alert has not arrived yet: %s at %s. It is tried again."), last.Result, when)
	}
	return help + " " + fmt.Sprintf(tr("The last alert did not arrive: %s at %s."), last.Result, when)
}

// handleAlertsTest posts a test alert to the address in force, and says how
// it went, for a session that may read and change the alert settings.
func (s *Server) handleAlertsTest(w http.ResponseWriter, r *http.Request) {
	if s.alerter == nil {
		http.NotFound(w, r)
		return
	}
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if g := s.grantsFor(s.sessionSID(r)); !mayTestAlerts(r.Context(), g) {
		s.flash(r, "danger", tr(g.refusal("This account may not send a test alert.")))
		http.Redirect(w, r, eventsPath, http.StatusSeeOther)
		return
	}
	if err := s.alerter.Test(r.Context()); err != nil {
		s.flash(r, "danger", fmt.Sprintf(tr("The test alert did not arrive: %s."), err))
	} else {
		s.flash(r, "success", tr("The test alert was sent."))
	}
	http.Redirect(w, r, eventsPath, http.StatusSeeOther)
}
