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
	// While a firmware upgrade is in flight — or has failed and no one has yet
	// been shown why — the maintenance area is the takeover, not the ordinary
	// page: a full-screen, chrome-less surface that holds the person on the one
	// outcome that matters. Because the branch is server-authoritative, a reload,
	// a second tab, and the back button all land back here for as long as the job
	// owns the router; nothing else renders until it lets go.
	if firmwareTakeoverActive() {
		s.renderUpgrading(w, r)
		return
	}
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

	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	var renderErr error
	render := func(w widget.Widget) template.HTML {
		var b strings.Builder
		if err := s.widgets.RenderWithToken(&b, s.reading(r, w), s.sessionCSRF(r), lang, t); err != nil {
			renderErr = err
		}
		return template.HTML(b.String())
	}
	verRow := func(label, value string) widget.Property {
		if value == "" {
			return widget.Property{Label: label, Value: "Unavailable"}
		}
		return widget.Property{Label: label, Value: value, Mono: true, Verbatim: true}
	}
	kernel := board.KernelBuild
	if kernel == "" {
		kernel = board.Kernel
	}
	identity := render(&widget.Properties{Style: "overview-system", Items: []widget.Property{
		verRow("OpenWrt", board.Firmware), verRow("Verso", version.Version),
		verRow("Kernel", kernel), verRow("Target", board.Target),
	}})
	truth, known := s.updateTruth()
	checking := updateChecks.running()
	statusWord := tr("Not checked yet")
	switch {
	case checking:
		statusWord = tr("Checking for updates…")
	case known && truth.Firmware.State == openwrt.FirmwareCurrent:
		statusWord = tr("Up to date")
	case known && truth.Firmware.State == openwrt.FirmwareUpdateAvailable:
		statusWord = fmt.Sprintf(tr("%s is available"), truth.Firmware.To)
	case known:
		statusWord = tr("Firmware check unavailable")
	}
	// Details keeps the existing firmware checks, automatic-check setting and
	// install confirmation together, while the resting page states one verdict.
	lane := firmwareLane(truth, known, checking, nil).(*widget.Section)
	detailChildren := []widget.Widget{}
	if lane.Sub != "" {
		detailChildren = append(detailChildren, &widget.Text{Markdown: lane.Sub})
	}
	detailChildren = append(detailChildren, lane.Children...)
	detailChildren = append(detailChildren, s.autocheckLane(r.Context(), sid))
	details := render(&widget.Modal{Title: "Firmware", Trigger: "Details", TriggerStyle: "secondary", Children: detailChildren})
	manual := s.firmwareModal(firmware, board)
	manual.Trigger, manual.TriggerStyle, manual.TriggerIcon = "Upload a custom image…", "link", "upload"
	var notices []widget.Widget
	if err := updateChecks.takeFailure(); err != nil {
		notices = append(notices, &widget.Callout{Variant: "danger", Compact: true, Body: fmt.Sprintf(tr("Update check failed: %v"), err)})
	}
	if err := packageUpgrade.takeFailure(); err != nil {
		notices = append(notices, &widget.Callout{Variant: "danger", Compact: true, Body: fmt.Sprintf(tr("Package update failed: %v"), err)})
	}
	packages := template.HTML("")
	if len(truth.Packages) > 0 || packageUpgrade.running() {
		packages = render(softwareLane(truth, known, checking))
	}
	uptime := tr("Unavailable")
	if systemErr == nil {
		uptime = maintenanceUptime(tr, si.Uptime)
	}
	checked := ""
	if known && !checking {
		checked = fmt.Sprintf(tr("Checked %s"), localizedAgo(time.Since(truth.CheckedAt), tr))
	}
	stage := s.staged(r.Context(), sid, tr, s.pluginTranslators(r))
	data := struct {
		Identity, Details, Manual, Restore, Packages, Notices    template.HTML
		CSRFToken, Status, Checked, Uptime, Hostname, StageLabel string
		Checking, Staged                                         bool
	}{identity, details, render(manual), render(s.restoreModal(restore)), packages,
		render(&widget.Stack{Children: notices}), s.sessionCSRF(r), statusWord,
		checked, uptime, s.nameplate(r), stage.Label, checking, stage.Count > 0}
	if renderErr != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	var body strings.Builder
	if err := s.pageSet(lang).ExecuteTemplate(&body, "maintenance.html.tmpl", data); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, status, pageHeader{Heading: "Maintenance", Tone: "neutral"}, "wide", s.systemPages(r.URL.Path, readerMode(r)), template.HTML(body.String()))
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
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	changes, err := s.backend.UCIChanges(r.Context(), s.sessionSID(r))
	if err != nil {
		http.Error(w, tr("Could not check staged changes."), http.StatusBadGateway)
		return
	}
	for _, changes := range changes {
		if len(changes) > 0 {
			http.Error(w, tr("Review and apply or discard staged changes before rebooting."), http.StatusConflict)
			return
		}
	}

	if err := s.backend.Restart(r.Context(), s.sessionSID(r)); err != nil {
		log.Printf("verso: restart: %v", err)
		http.Error(w, tr("Could not restart the router."), http.StatusBadGateway)
		return
	}
	_, _ = fmt.Fprintf(w, "<!doctype html><title>%s</title><p>%s</p>", template.HTMLEscapeString(tr("Restarting")), template.HTMLEscapeString(tr("The router is restarting.")))
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

// maintenanceUptime words the router's uptime. Two flat forms per unit (one,
// and many), translated at composition — the same plural shape the staged-changes chip
// label carries, with the same TODO(i18n plurals) caveat.
func maintenanceUptime(tr func(string) string, seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days, hours := seconds/86400, (seconds%86400)/3600
	minutes := (seconds % 3600) / 60
	parts := make([]string, 0, 2)
	if days > 0 {
		parts = append(parts, fmt.Sprintf(tr(plural(days, "1 day", "%d days")), days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf(tr(plural(hours, "1 hour", "%d hours")), hours))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf(tr(plural(minutes, "1 minute", "%d minutes")), minutes))
	}
	return strings.Join(parts, ", ")
}

// plural picks the flat one/many form; a "1 …" form carries no verb, so the
// Sprintf over it is a no-op.
func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

var _ openwrt.Backend = (*openwrt.NativeBackend)(nil)
