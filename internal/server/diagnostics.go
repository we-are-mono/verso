// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// Diagnostics: ping, traceroute and a DNS lookup, run from the router rather
// than the browser, so a failure read here is the router's. The helper runs
// them (verso-rpcd's diagnostics) and this page reads a run back while it
// runs: the row on the heading line says what to run, and the output fills
// the sand under it, line by line, as a log does.

// diagnosticTools are the runs the helper knows, by the name it knows them.
var diagnosticTools = map[string]bool{"ping": true, "traceroute": true, "nslookup": true}

// diagnosticLabel is one DNS label: letters, digits and underscores, with
// hyphens inside but never at either end, so a target can never read as a
// flag. It is the helper's own rule, said here so a refusal comes back in
// words before the router is asked.
var diagnosticLabel = regexp.MustCompile(`^[A-Za-z0-9_](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9_])?$`)

func validDiagnosticTarget(target string) bool {
	if net.ParseIP(target) != nil {
		return true
	}
	name := strings.TrimSuffix(target, ".")
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if !diagnosticLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// diagnosticChoice is one option of the run's row: the value it posts and the
// words it shows. A way out of the router is a kernel device, named by the
// interfaces netifd gives it ("wan, wan6" share wan0).
type diagnosticChoice struct {
	Value string
	Label string
}

// diagnosticChoices are the devices a run may leave by: every device a
// logical interface holds, once, in netifd's order. The loopback is not a way
// out, and an interface with no device has nothing to bind to.
func (s *Server) diagnosticChoices(r *http.Request) []diagnosticChoice {
	ifaces, err := s.backend.NetworkInterfaces(r.Context(), s.sessionSID(r))
	if err != nil {
		return nil
	}
	var out []diagnosticChoice
	at := map[string]int{}
	for _, iface := range ifaces {
		if iface.Device == "" || iface.Device == "lo" {
			continue
		}
		if i, ok := at[iface.Device]; ok {
			out[i].Label += ", " + iface.Name
			continue
		}
		at[iface.Device] = len(out)
		out = append(out, diagnosticChoice{Value: iface.Device, Label: iface.Name})
	}
	return out
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	var body, acts strings.Builder
	set := s.pageSet(lang)
	if err := set.ExecuteTemplate(&body, "diagnostics.html.tmpl", nil); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// The family strip's choices, the first the one in force: "any" lets the
	// name decide, as the helper does with no family.
	row := struct {
		Interfaces []diagnosticChoice
		Families   []diagnosticChoice
	}{s.diagnosticChoices(r), []diagnosticChoice{{"", tr("any")}, {"4", "IPv4"}, {"6", "IPv6"}}}
	if err := set.ExecuteTemplate(&acts, "diagnostics.acts", row); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	hdr := pageHeader{Heading: "Diagnostics", Tone: "neutral", Light: true, HeadingAct: template.HTML(acts.String())} //nolint:gosec // rendered by the shell's own templates
	s.renderPage(w, r, http.StatusOK, hdr, "full", s.sectionPages("System", r.URL.Path), template.HTML(body.String()))
}

// diagnosticRun reads the row's choice and says, in words, why it cannot run.
func (s *Server) diagnosticRun(r *http.Request, tr func(string) string) (openwrt.DiagnosticRun, string) {
	run := openwrt.DiagnosticRun{
		Tool:      r.PostForm.Get("tool"),
		Target:    strings.TrimSpace(r.PostForm.Get("target")),
		Interface: r.PostForm.Get("interface"),
		Family:    r.PostForm.Get("family"),
	}
	switch {
	case !diagnosticTools[run.Tool]:
		return run, tr("Choose a tool.")
	case !validDiagnosticTarget(run.Target):
		return run, tr("Enter a hostname or an IP address.")
	case run.Family != "" && run.Family != "4" && run.Family != "6":
		return run, tr("Choose an IP family.")
	}
	if ip := net.ParseIP(run.Target); ip != nil && run.Family != "" {
		v4 := ip.To4() != nil
		if v4 && run.Family == "6" {
			return run, tr("That is an IPv4 address. Choose IPv4 or any.")
		}
		if !v4 && run.Family == "4" {
			return run, tr("That is an IPv6 address. Choose IPv6 or any.")
		}
	}
	if run.Interface != "" {
		if run.Tool == "nslookup" {
			return run, tr("A DNS lookup is not bound to an interface.")
		}
		known := false
		for _, iface := range s.diagnosticChoices(r) {
			known = known || iface.Value == run.Interface
		}
		if !known {
			return run, tr("Choose an interface from the list.")
		}
	}
	return run, ""
}

// handleDiagnosticsRun starts a run and names it; the page then reads it from
// handleDiagnosticsStream.
func (s *Server) handleDiagnosticsRun(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	run, refusal := s.diagnosticRun(r, tr)
	if refusal != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": refusal})
		return
	}
	job, err := s.backend.DiagStart(r.Context(), s.sessionSID(r), run)
	if err != nil {
		message := tr("The router could not start the run.")
		var refused interface{ ValidationMessage() string }
		if errors.As(err, &refused) && refused.ValidationMessage() != "" {
			message = refused.ValidationMessage()
		}
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"job": job})
}

func (s *Server) handleDiagnosticsStop(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if err := s.backend.DiagStop(r.Context(), s.sessionSID(r), r.PostForm.Get("job")); err != nil {
		http.Error(w, "stop failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// diagToken is one run of a line's text and the ink it is read in, named as
// the code block names its inks (code.html.tmpl): "value" green, "keyword"
// denim, "meta" quiet, "" the line's own ink.
type diagToken [2]string

// diagnosticReading picks out what a diagnostic line is about: ping's
// statistics rule, whole; an address (v4 with its port, or v6); a measurement
// (a time, a run of them, a share); a hop that did not answer.
var diagnosticReading = regexp.MustCompile(`(^-{3}.*-{3}$)` +
	`|(\b(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?\b|(?:[0-9A-Fa-f]{0,4}:){2,7}[0-9A-Fa-f]{0,4})` +
	`|(\b\d+(?:\.\d+)?(?:/\d+(?:\.\d+)?)*\s?(?:ms|%))` +
	`|(?:^|\s)(\*)(?:\s|$)`)

var diagnosticInks = [...]string{"meta", "value", "keyword", "meta"}

// diagnosticTokens reads one line of a run in the code block's inks, so the
// output reads as the rest of Verso's code does. What it does not pick out
// keeps the line's own ink.
func diagnosticTokens(line string) []diagToken {
	tokens := []diagToken{}
	at := 0
	for _, match := range diagnosticReading.FindAllStringSubmatchIndex(line, -1) {
		for group, ink := range diagnosticInks {
			start, end := match[2+2*group], match[3+2*group]
			if start < 0 {
				continue
			}
			if start > at {
				tokens = append(tokens, diagToken{"", line[at:start]})
			}
			tokens = append(tokens, diagToken{ink, line[start:end]})
			at = end
			break
		}
	}
	if at < len(line) {
		tokens = append(tokens, diagToken{"", line[at:]})
	}
	return tokens
}

// diagFrame is one step of a run on the wire: the lines it wrote since the
// last, each read into its tokens, and once it is over, how it ended in words
// and the tone to say them in ("" for a run that went as asked, "warning" for
// one cut short, "danger" for one that failed).
type diagFrame struct {
	Lines   [][]diagToken `json:"lines"`
	Done    bool          `json:"done"`
	Summary string        `json:"summary,omitempty"`
	Tone    string        `json:"tone,omitempty"`
}

func diagnosticSummary(out openwrt.DiagnosticOutput, tr func(string) string) (string, string) {
	switch {
	case out.Ended == "timeout":
		return tr("Stopped after 90 seconds."), "warning"
	case out.Ended == "stopped" || out.Code == nil:
		return tr("Stopped."), "warning"
	case *out.Code == 0:
		return tr("Done."), ""
	default:
		return fmt.Sprintf(tr("Ended with status %d."), *out.Code), "danger"
	}
}

// handleDiagnosticsStream reads one run to its end: each tick it asks the
// helper what the run wrote past the cursor, and once the run is over it says
// how it ended and closes, rather than poll a run that will write no more. The
// cursor is the event id, so a reconnect picks up where it left off.
func (s *Server) handleDiagnosticsStream(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	job := r.URL.Query().Get("job")
	cursor, err := strconv.Atoi(r.Header.Get("Last-Event-ID"))
	if err != nil || cursor < 0 {
		cursor = 0
	}
	s.serveSSEEvery(w, r, s.eventInterval/4, func() bool {
		out, err := s.backend.DiagRead(r.Context(), s.sessionSID(r), job, cursor)
		if err != nil {
			_ = writeEvent(w, "", "output", diagFrame{Lines: [][]diagToken{}, Done: true, Summary: tr("This run is no longer on the router."), Tone: "warning"})
			return false
		}
		cursor = out.Next
		frame := diagFrame{Lines: make([][]diagToken, len(out.Lines)), Done: out.Done}
		for i, line := range out.Lines {
			frame.Lines[i] = diagnosticTokens(line)
		}
		if out.Done {
			frame.Summary, frame.Tone = diagnosticSummary(out, tr)
		}
		return writeEvent(w, strconv.Itoa(cursor), "output", frame) == nil && !out.Done
	})
}
