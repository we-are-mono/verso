// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

const (
	systemLogSource = "system-log"
	systemLogLimit  = 1000
)

type systemLogEvent struct {
	ID       int64  `json:"id"`
	At       int64  `json:"at"`
	Source   string `json:"source"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// procd bounds its process tag, so a long executable name and PID can lose the
// closing bracket. The source is still intact and should remain filterable.
var syslogTag = regexp.MustCompile(`^([^\s:\[\]]+)(?:\[\d+\]?)?:\s*(.*)$`)
var syslogSeverities = [...]string{"emerg", "alert", "crit", "err", "warn", "notice", "info", "debug"}

func systemLogRow(entry openwrt.LogEntry) systemLogEvent {
	row := systemLogEvent{ID: entry.ID, At: entry.Time / 1000, Source: "syslog", Severity: syslogSeverities[entry.Priority&7], Message: entry.Msg}
	if entry.Source == 0 {
		row.Source = "kernel"
	} else if match := syslogTag.FindStringSubmatch(entry.Msg); match != nil {
		row.Source, row.Message = match[1], match[2]
	}
	return row
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	lang, t := s.localize(r)
	var body, acts strings.Builder
	if err := s.pageSet(lang).ExecuteTemplate(&body, "logs.html.tmpl", nil); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// What narrows the log and what acts on it — its search, its live control,
	// the download, the settings — sit on the heading line.
	if err := s.pageSet(lang).ExecuteTemplate(&acts, "logs.acts", struct{ Filter string }{translatorOrIdentity(t)("Find a message or source")}); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	hdr := pageHeader{Heading: "Logs", Tone: "neutral", HeadingAct: template.HTML(acts.String())} //nolint:gosec // rendered by the shell's own templates
	s.renderPage(w, r, http.StatusOK, hdr, "full", s.sectionPages("System", r.URL.Path), template.HTML(body.String()))
}

// Reuse the authenticated listing stream route and sampling clock. IDs are
// logd's cursor, including zero; a restarted ring explicitly replaces history.
func (s *Server) streamSystemLog(w http.ResponseWriter, r *http.Request) {
	cursor := int64(-1)
	systemCursor, firewallCursor, _ := strings.Cut(r.Header.Get("Last-Event-ID"), "|")
	includeFirewall := r.URL.Query().Get("firewall") == "1"
	generation, after, resumed := parseFirewallCursor(firewallCursor)
	if !resumed {
		after = -1
	}
	if last, err := strconv.ParseInt(systemCursor, 10, 64); err == nil && last >= 0 {
		cursor = last
	}
	s.serveSSE(w, r, func() bool {
		entries, err := s.backend.LogRead(r.Context(), s.sessionSID(r), systemLogLimit)
		if err != nil {
			return writeEvent(w, "", "unavailable", struct{}{}) == nil
		}
		reset := len(entries) > 0 && highestID(entries) < cursor
		var packets openwrt.FirewallLogBatch
		firewallUnavailable := false
		if includeFirewall {
			var readErr error
			packets, readErr = s.backend.FirewallLogRead(r.Context(), s.sessionSID(r), generation, after, 500)
			firewallUnavailable = readErr != nil || !packets.Available
			if readErr == nil {
				reset = reset || (packets.Reset && generation != "")
				generation = packets.Generation
				if packets.Reset {
					after = -1
				}
			} else {
				packets = openwrt.FirewallLogBatch{}
			}
		}
		if reset {
			cursor = -1
		}
		rows := make([]systemLogEvent, 0, len(entries))
		for _, entry := range entries {
			if entry.ID > cursor {
				_, packet := parseFWLogLine(entry.Msg)
				packet = packet && entry.Source == 0
				if packet && !includeFirewall {
					continue
				}
				row := systemLogRow(entry)
				if packet {
					row.Source = "firewall"
				}
				rows = append(rows, row)
			}
		}
		if len(entries) > 0 {
			cursor = highestID(entries)
		}
		for _, entry := range packets.Entries {
			if entry.ID <= after {
				continue
			}
			after = entry.ID
			row := systemLogRow(entry)
			row.ID, row.Source = -entry.ID, "firewall"
			rows = append(rows, row)
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].At < rows[j].At })
		id := strconv.FormatInt(cursor, 10)
		if includeFirewall && generation != "" {
			id += "|" + generation + ":" + strconv.FormatInt(after, 10)
		}
		return writeEvent(w, id, "stream", struct {
			Rows                []systemLogEvent `json:"rows"`
			Reset               bool             `json:"reset"`
			FirewallUnavailable bool             `json:"firewall_unavailable"`
		}{rows, reset, firewallUnavailable}) == nil
	})
}

func systemLogSection(config map[string]any) (string, map[string]any) {
	for _, id := range sectionOrder(config) {
		if section, ok := config[id].(map[string]any); ok && uciString(section[".type"]) == "system" {
			return id, section
		}
	}
	return "", nil
}

func (s *Server) handleLogSettings(w http.ResponseWriter, r *http.Request) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	config, err := s.backend.UCIConfig(r.Context(), s.sessionSID(r), "system")
	section, values := systemLogSection(config)
	status, message := http.StatusOK, ""
	if err != nil || section == "" {
		status, message = http.StatusBadGateway, "Log settings could not be read."
	}
	if r.Method == http.MethodPost && status == http.StatusOK {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		input := map[string]any{}
		for _, key := range []string{"log_ip", "log_port", "log_proto", "log_buffer_size", "log_file"} {
			input[key] = strings.TrimSpace(r.PostForm.Get(key))
		}
		input["log_remote"] = "0"
		if r.PostForm.Get("log_remote") == "1" {
			input["log_remote"] = "1"
		}
		if err := validateLogSettings(input); err != nil {
			status, message, values = http.StatusUnprocessableEntity, err.Error(), input
		} else if err := s.backend.UCISet(r.Context(), s.sessionSID(r), "system", section, input); err != nil {
			status, message, values = http.StatusBadGateway, "Log settings could not be saved.", input
		} else {
			// Staged, so not yet done: the chip says a change waits.
			if panelRequest(r) {
				w.Header().Set("HX-Redirect", "/system/logs")
				return
			}
			http.Redirect(w, r, "/system/logs", http.StatusSeeOther)
			return
		}
	}
	children := []widget.Widget{}
	if message != "" {
		children = append(children, &widget.Callout{Variant: "danger", Compact: true, Body: message})
	}
	if values != nil {
		value := func(key, fallback string) string {
			if v := uciString(values[key]); v != "" {
				return v
			}
			return fallback
		}
		buffer := value("log_buffer_size", value("log_size", "64"))
		if buffer == "0" {
			buffer = "64"
		}
		children = append(children, &widget.Form{Action: "/system/logs/settings", Submit: "Save log settings", Fields: []widget.Widget{
			&widget.Switch{Name: "log_remote", Label: "Send logs to a remote server", On: value("log_remote", "1") == "1"},
			&widget.Field{Name: "log_ip", Label: "Remote log server", Value: value("log_ip", ""), Placeholder: "Optional", Help: "Leave empty to keep logs on this router."},
			&widget.Field{Name: "log_port", Label: "Port", Value: value("log_port", "514"), Datatype: "port", Required: true},
			&widget.Field{Name: "log_proto", Label: "Transport", Kind: "select", Value: value("log_proto", "udp"), Options: []widget.Option{{Label: "UDP", Value: "udp"}, {Label: "TCP", Value: "tcp"}}},
			&widget.Field{Name: "log_buffer_size", Label: "Log buffer (KiB)", Value: buffer, Datatype: "uinteger", Required: true, Help: "Old entries are replaced when the buffer is full."},
			&widget.Field{Name: "log_file", Label: "Log file", Value: value("log_file", ""), Placeholder: "Optional", Help: "An absolute path. Writing logs to persistent storage uses flash write cycles."},
		}})
	}
	var content, view, panel strings.Builder
	if err := s.widgets.RenderWithToken(&content, &widget.Stack{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// The drawer over the log opens on how the log is read (logs.view), above
	// the settings and outside their form.
	if err := s.pageSet(lang).ExecuteTemplate(&view, "logs.view", nil); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	data := struct {
		Title string
		Body  template.HTML
	}{tr("Log settings"), template.HTML(view.String() + content.String())} //nolint:gosec // rendered by the shell's own templates
	if err := s.pageSet(lang).ExecuteTemplate(&panel, "system-panel.html.tmpl", data); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if panelRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(panel.String()))
		return
	}
	// The settings alone, as a page: the way the log is read stays with the
	// log, in the drawer over it.
	s.renderPage(w, r, status, pageHeader{Heading: "Log settings", Tone: "neutral"}, "narrow", s.sectionPages("System", r.URL.Path), template.HTML(content.String()))
}

func validateLogSettings(values map[string]any) error {
	port, err := strconv.Atoi(uciString(values["log_port"]))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("Enter a port from 1 to 65535.")
	}
	size, err := strconv.Atoi(uciString(values["log_buffer_size"]))
	if err != nil || size < 1 || size > 65536 {
		return fmt.Errorf("Enter a log buffer size from 1 to 65536 KiB.")
	}
	if proto := uciString(values["log_proto"]); proto != "tcp" && proto != "udp" {
		return fmt.Errorf("Choose TCP or UDP.")
	}
	host := uciString(values["log_ip"])
	if host != "" && net.ParseIP(host) == nil && !logHostname.MatchString(host) {
		return fmt.Errorf("Enter an IP address or hostname for the log server.")
	}
	file := uciString(values["log_file"])
	if file != "" && (!strings.HasPrefix(file, "/") || strings.ContainsAny(file, "\x00\r\n")) {
		return fmt.Errorf("Enter an absolute path for the log file.")
	}
	return nil
}

var logHostname = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*\.?$`)
