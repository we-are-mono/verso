// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/version"
	"github.com/we-are-mono/verso/internal/widget"
)

const (
	maxRestoreSize  = 32 << 20
	restoreLifetime = 15 * time.Minute
)

type pendingRestore struct {
	sid     string
	path    string
	name    string
	entries int
	created time.Time
}

type restoreState struct {
	open          bool
	verified      bool
	token         string
	name          string
	entries       int
	errorMessage  string
	passwordError string
	restoreError  string
}

func (s *Server) handleSystemMaintenance(w http.ResponseWriter, r *http.Request) {
	s.renderMaintenance(w, r, http.StatusOK, restoreState{})
}

func (s *Server) renderMaintenance(w http.ResponseWriter, r *http.Request, status int, restore restoreState) {
	s.renderMaintenancePage(w, r, status, restore, firmwareState{})
}

func (s *Server) renderMaintenanceFirmware(w http.ResponseWriter, r *http.Request, status int, firmware firmwareState) {
	s.renderMaintenancePage(w, r, status, restoreState{}, firmware)
}

func (s *Server) renderMaintenancePage(w http.ResponseWriter, r *http.Request, status int, restore restoreState, firmware firmwareState) {
	sid := s.sessionSID(r)
	board, boardErr := s.backend.Board(r.Context(), sid)
	si, systemErr := s.backend.SystemInfo(r.Context(), sid)
	if boardErr != nil {
		log.Printf("verso: maintenance: board unavailable: %v", boardErr)
	}
	if systemErr != nil {
		log.Printf("verso: maintenance: system info unavailable: %v", systemErr)
	}

	value := func(v string) string {
		if v == "" {
			return "Unavailable"
		}
		return v
	}
	software := &widget.Section{
		Title: "Software",
		Sub:   "Firmware updates replace the operating system while keeping your settings.",
		Children: []widget.Widget{
			&widget.Properties{Items: []widget.Property{
				{Label: "OpenWrt version", Value: value(board.Firmware), Emphasis: true},
				{Label: "Verso version", Value: version.Version, Emphasis: true},
				{Label: "Kernel build", Value: value(board.KernelBuild), Mono: true, Emphasis: true},
				{Label: "Target", Value: value(board.Target), Mono: true, Emphasis: true},
			}},
			s.firmwareModal(firmware, board),
		},
	}

	restoreModal := s.restoreModal(restore)
	backup := &widget.Section{
		Title: "Backup and restore", Hairline: true,
		Sub: "Download a copy before a larger change, or restore a setup you saved earlier.",
		Children: []widget.Widget{
			&widget.Stack{Inline: true, Children: []widget.Widget{
				&widget.Link{Style: "button", Label: "Download backup", Icon: "download", Href: "/system/maintenance/backup", Download: "openwrt-backup.tar.gz"},
				&widget.Text{Markdown: "or"}, restoreModal,
			}},
			&widget.Callout{Variant: "neutral", Compact: true, Body: "The backup contains system and plugin settings registered with OpenWrt. It does not execute plugin code."},
		},
	}

	uptime := "Unavailable"
	if systemErr == nil {
		uptime = maintenanceUptime(si.Uptime)
	}
	restart := &widget.Section{
		Title: "Restart", Hairline: true,
		Sub:       "The connection will disappear briefly; settings and installed software stay unchanged.",
		MetaLabel: "Running for", Meta: uptime, MetaIcon: "clock",
		Children: []widget.Widget{&widget.Form{Action: "/system/maintenance/restart", NoSubmit: true, Fields: []widget.Widget{
			&widget.Button{Label: "Restart router", Style: "secondary", Name: "action", Value: "restart"},
		}}},
	}
	factory := &widget.Section{
		Title: "Factory reset", Hairline: true,
		Sub: "Erase settings, installed plugins, and local data, then return the router to its first-run state.",
		Children: []widget.Widget{&widget.Form{Action: "/system/maintenance/factory-reset", NoSubmit: true, Fields: []widget.Widget{
			&widget.Confirm{Trigger: "Erase everything and reset", Message: "Enter your administrator password to erase all settings, installed plugins, and local data. This cannot be undone.", Confirm: "Factory reset", Cancel: "No, I changed my mind", RequirePassword: true},
		}}},
	}

	var body strings.Builder
	// Updates lead: what the router could install is the question a person opens
	// this page with. The manual image upload below is the permanent floor under
	// them — the way in when no server can build for this device.
	root := &widget.Stack{Children: []widget.Widget{s.updatesSection(r.Context(), sid), software, backup, restart, factory}}
	lang, t := s.localize(r)
	if err := s.widgets.RenderWithToken(&body, s.reading(r, root), s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, status, pageHeader{
		Heading:    "System",
		Subheading: "Keep this router current, backed up, and recoverable.",
	}, "narrow", s.systemPages(r.URL.Path, readerMode(r)), false, template.HTML(body.String()))
}

func (s *Server) restoreModal(state restoreState) *widget.Modal {
	m := &widget.Modal{
		Trigger: "Restore a backup…", TriggerIcon: "upload", TriggerStyle: "secondary",
		Open: state.open, Title: "Restore a backup", BusyTitle: "Checking backup",
		BusyBody: "Reading the archive and confirming that OpenWrt can restore it.",
	}
	if state.verified {
		m.BusyTitle = "Restoring backup"
		m.BusyBody = "Applying the saved settings. The router will restart when the restore is complete."
		result := widget.Widget(&widget.Callout{Variant: "success", Title: "Backup is ready", Body: "The archive is readable and can be restored by OpenWrt."})
		if state.restoreError != "" {
			result = &widget.Callout{Variant: "danger", Title: "Restore did not complete", Body: state.restoreError}
		}
		m.Children = []widget.Widget{
			result,
			&widget.Properties{Items: []widget.Property{
				{Label: "File", Value: state.name, Mono: true, Emphasis: true},
				{Label: "Settings", Value: fmt.Sprintf("%d archived files", state.entries), Emphasis: true},
			}},
			&widget.Form{Action: "/system/maintenance/restore/apply", Submit: "Restore and restart", Fields: []widget.Widget{
				&widget.Field{Name: "restore_token", Kind: "hidden", Value: state.token},
				&widget.Field{Name: "password", Label: "Password", Kind: "password", Autocomplete: "current-password", Error: state.passwordError, Help: "Re-enter your password to authorize the restore."},
			}},
		}
		return m
	}
	children := []widget.Widget{}
	if state.errorMessage != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Backup could not be verified", Body: state.errorMessage})
	}
	children = append(children, &widget.Form{
		Action: "/system/maintenance/restore", Multipart: true, AutoSubmit: true, NoSubmit: true,
		Fields: []widget.Widget{&widget.Field{Name: "backup", Kind: "file", Accept: ".tar.gz,.tgz,.gz,application/gzip,application/x-gzip,application/x-compressed-tar", Prompt: "Drop an OpenWrt backup here", Required: true}},
	})
	m.Children = children
	return m
}

func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	file, err := os.CreateTemp(s.maintenanceDir, "verso-backup-*.tar.gz")
	if err != nil {
		http.Error(w, "Could not prepare the backup.", http.StatusInternalServerError)
		return
	}
	name := file.Name()
	_ = file.Close()
	defer os.Remove(name)
	if err := s.backend.CreateBackup(r.Context(), s.sessionSID(r), name); err != nil {
		log.Printf("verso: create backup: %v", err)
		http.Error(w, "Could not create the OpenWrt backup.", http.StatusBadGateway)
		return
	}
	archive, err := os.Open(name)
	if err != nil {
		http.Error(w, "Could not open the backup.", http.StatusInternalServerError)
		return
	}
	defer archive.Close()
	hostname, _ := s.backend.Hostname(r.Context(), s.sessionSID(r))
	filename := backupFilename(hostname, time.Now())
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, archive)
}

func backupFilename(hostname string, now time.Time) string {
	var clean strings.Builder
	separator := false
	for _, r := range strings.ToLower(hostname) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if separator && clean.Len() > 0 {
				clean.WriteByte('-')
			}
			clean.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	if clean.Len() == 0 {
		clean.WriteString("openwrt")
	}
	return fmt.Sprintf("%s-backup-%s.tar.gz", clean.String(), now.Format("20060102-150405"))
}

func (s *Server) handleRestoreInspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxRestoreSize); err != nil {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, restoreState{open: true, errorMessage: "The file is too large or could not be uploaded. Choose an OpenWrt backup up to 32 MiB."})
		return
	}
	source, header, err := r.FormFile("backup")
	if err != nil {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, restoreState{open: true, errorMessage: "Choose an OpenWrt backup archive first."})
		return
	}
	defer source.Close()
	temp, err := os.CreateTemp(s.maintenanceDir, "verso-restore-*.tar.gz")
	if err != nil {
		http.Error(w, "Could not store the upload.", http.StatusInternalServerError)
		return
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		_ = temp.Close()
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	written, err := io.Copy(temp, io.LimitReader(source, maxRestoreSize+1))
	if err != nil || written == 0 || written > maxRestoreSize {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, restoreState{open: true, errorMessage: "The selected file is empty, too large, or could not be read."})
		return
	}
	if err := temp.Close(); err != nil {
		http.Error(w, "Could not store the upload.", http.StatusInternalServerError)
		return
	}
	entries, err := inspectBackup(tempPath)
	if err != nil {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, restoreState{open: true, errorMessage: "The selected file is not a readable OpenWrt backup archive. Choose another file and try again."})
		return
	}
	token, err := randomRestoreToken()
	if err != nil {
		http.Error(w, "Could not prepare the restore.", http.StatusInternalServerError)
		return
	}
	keep = true
	s.putPendingRestore(token, pendingRestore{sid: s.sessionSID(r), path: tempPath, name: path.Base(header.Filename), entries: entries, created: time.Now()})
	s.renderMaintenance(w, r, http.StatusOK, restoreState{open: true, verified: true, token: token, name: path.Base(header.Filename), entries: entries})
}

func inspectBackup(filename string) (int, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		clean := path.Clean(h.Name)
		if strings.HasPrefix(h.Name, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
			return 0, fmt.Errorf("unsafe archive path")
		}
		if (h.Typeflag == tar.TypeSymlink || h.Typeflag == tar.TypeLink) &&
			(strings.HasPrefix(h.Linkname, "/") || path.Clean(h.Linkname) == ".." || strings.HasPrefix(path.Clean(h.Linkname), "../")) {
			return 0, fmt.Errorf("unsafe archive link")
		}
		count++
		if count > 100000 {
			return 0, fmt.Errorf("too many archive entries")
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("empty archive")
	}
	return count, nil
}

func randomRestoreToken() (string, error) {
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func (s *Server) putPendingRestore(token string, pending pendingRestore) {
	s.pendingRestoreMu.Lock()
	defer s.pendingRestoreMu.Unlock()
	now := time.Now()
	for key, old := range s.pendingRestores {
		if old.sid == pending.sid || now.Sub(old.created) > restoreLifetime {
			_ = os.Remove(old.path)
			delete(s.pendingRestores, key)
		}
	}
	s.pendingRestores[token] = pending
}

func (s *Server) pendingRestore(token, sid string) (pendingRestore, bool) {
	s.pendingRestoreMu.Lock()
	defer s.pendingRestoreMu.Unlock()
	pending, ok := s.pendingRestores[token]
	if !ok || pending.sid != sid || time.Since(pending.created) > restoreLifetime {
		if ok {
			_ = os.Remove(pending.path)
			delete(s.pendingRestores, token)
		}
		return pendingRestore{}, false
	}
	return pending, true
}

// consumePendingRestore claims the token — removing it under the lock and
// reporting whether it was present — so two racing applies of one verified
// upload cannot both run the privileged restore.
func (s *Server) consumePendingRestore(token string) bool {
	s.pendingRestoreMu.Lock()
	defer s.pendingRestoreMu.Unlock()
	_, ok := s.pendingRestores[token]
	delete(s.pendingRestores, token)
	return ok
}

func (s *Server) handleRestoreApply(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	token := r.FormValue("restore_token")
	pending, ok := s.pendingRestore(token, s.sessionSID(r))
	if !ok {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, restoreState{open: true, errorMessage: "That verified backup has expired. Choose it again."})
		return
	}
	verifier, ok := s.auth.(credentialVerifier)
	if !ok || verifier.Verify(r.Context(), s.sessionUser(r), r.FormValue("password")) != nil {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, restoreState{open: true, verified: true, token: token, name: pending.name, entries: pending.entries, passwordError: "The current password is incorrect."})
		return
	}
	if !s.consumePendingRestore(token) {
		s.renderMaintenance(w, r, http.StatusUnprocessableEntity, restoreState{open: true, errorMessage: "That verified backup was already used. Upload it again to restore once more."})
		return
	}
	if err := s.backend.RestoreBackup(r.Context(), s.sessionSID(r), pending.path); err != nil {
		log.Printf("verso: restore backup: %v", err)
		s.putPendingRestore(token, pending) // re-stage so the operator can retry the same verified upload
		s.renderMaintenance(w, r, http.StatusBadGateway, restoreState{open: true, verified: true, token: token, name: pending.name, entries: pending.entries, restoreError: "OpenWrt could not complete the restore. The saved archive is still available to retry."})
		return
	}
	_ = os.Remove(pending.path)
	s.renderRestoreComplete(w, r)
}

func (s *Server) renderRestoreComplete(w http.ResponseWriter, r *http.Request) {
	lang, _ := s.localize(r)
	var body strings.Builder
	if err := s.pageSet(lang).ExecuteTemplate(&body, "restore-complete.html.tmpl", struct {
		Lang string
		CSS  template.CSS
	}{Lang: langAttr(lang), CSS: s.currentCSS()}); err != nil {
		http.Error(w, "restore complete page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "120")
	_, _ = io.WriteString(w, body.String())
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.Restart(r.Context(), s.sessionSID(r)); err != nil {
		log.Printf("verso: restart: %v", err)
		http.Error(w, "Could not restart the router.", http.StatusBadGateway)
		return
	}
	_, _ = io.WriteString(w, "<!doctype html><title>Restarting</title><p>The router is restarting.</p>")
}

func (s *Server) handleFactoryReset(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	verifier, ok := s.auth.(credentialVerifier)
	if !ok || verifier.Verify(r.Context(), s.sessionUser(r), r.FormValue("password")) != nil {
		s.flash(r, "danger", "The current administrator password is incorrect.")
		http.Redirect(w, r, "/system/maintenance", http.StatusSeeOther)
		return
	}
	if err := s.backend.FactoryReset(r.Context(), s.sessionSID(r)); err != nil {
		log.Printf("verso: factory reset: %v", err)
		http.Error(w, "Could not reset the router.", http.StatusBadGateway)
		return
	}
	_, _ = io.WriteString(w, "<!doctype html><title>Factory reset</title><p>The router is erasing its settings and restarting.</p>")
}

func maintenanceUptime(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days, hours := seconds/86400, (seconds%86400)/3600
	minutes := (seconds % 3600) / 60
	parts := make([]string, 0, 2)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", days, plural(days, "day", "days")))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", hours, plural(hours, "hour", "hours")))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d %s", minutes, plural(minutes, "minute", "minutes")))
	}
	return strings.Join(parts, ", ")
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

var _ openwrt.Backend = (*openwrt.NativeBackend)(nil)
