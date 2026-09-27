// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

const maxFirmwareSize = 128 << 20

type pendingFirmware struct {
	sid     string
	path    string
	name    string
	info    openwrt.FirmwareInfo
	model   string
	created time.Time
}

type firmwareState struct {
	open          bool
	verified      bool
	token         string
	name          string
	info          openwrt.FirmwareInfo
	model         string
	errorMessage  string
	passwordError string
	installError  string
}

// boardSentence words a sentence about the board in the reader's language. With
// no model known it is a whole sentence of its own, because a language may
// inflect "this device" differently in each; with one, the model's name — which
// does not inflect — goes into the translated sentence. Either way it arrives
// localized, so the walk must leave it (…Verbatim).
func boardSentence(tr func(string) string, unknown, known, model string) string {
	if model == "" {
		return tr(unknown)
	}
	return fmt.Sprintf(tr(known), model)
}

func (s *Server) firmwareModal(tr func(string) string, state firmwareState, board openwrt.Board) *widget.Modal {
	// The dialog says the words its trigger says, and where it is: an image is
	// chosen, then verified by OpenWrt, then installed.
	m := &widget.Modal{
		Trigger: "Choose firmware…", TriggerIcon: "upload",
		Open: state.open, Title: "Upload a custom image", BusyTitle: "Verifying firmware",
		BusyBody: boardSentence(tr,
			"Checking the image and confirming that it matches this device.",
			"Checking the image and confirming that it matches %s.", board.Model),
		BusyBodyVerbatim: true,
		Steps:            []string{"Choose", "Verify", "Install"},
	}
	if state.verified {
		m.Step = 2
		m.BusyTitle = "Installing firmware"
		m.BusyBody = "Writing the verified image. Do not disconnect power while the router restarts."
		result := widget.Widget(&widget.Callout{
			Variant: "success", Title: "Firmware verified",
			Body: fmt.Sprintf("The image passed OpenWrt's compatibility check and matches %s.", state.model),
		})
		if state.installError != "" {
			result = &widget.Callout{Variant: "danger", Title: "Firmware was not installed", Body: state.installError}
		}
		m.Children = []widget.Widget{
			result,
			&widget.Properties{Items: []widget.Property{
				{Label: "File", Value: state.name, Mono: true, Emphasis: true},
				{Label: "Version", Value: firmwareVersion(state.info), Emphasis: true},
				{Label: "Settings", Value: "Keep current settings", Emphasis: true},
			}},
			&widget.Form{Action: "/system/maintenance/firmware/apply", NoSubmit: true, Fields: []widget.Widget{
				&widget.Field{Name: "firmware_token", Kind: "hidden", Value: state.token},
				&widget.Field{Name: "password", Label: "Password", Kind: "password", Autocomplete: "current-password", Error: state.passwordError, Help: "Re-enter your password to authorize the upgrade."},
				// Installing firmware is disruptive but wanted: it asks in caution's
				// marigold, as every firmware install does, not in crimson.
				&widget.Confirm{Trigger: "Install and restart", Message: "Install this verified firmware now? The router will be unavailable for several minutes. Do not disconnect its power.", Confirm: "Install firmware", Cancel: "Not yet", Tone: "caution"},
			}},
		}
		return m
	}
	children := []widget.Widget{}
	if state.errorMessage != "" {
		children = append(children, &widget.Callout{
			Variant: "danger", Title: "Firmware could not be verified",
			Body: state.errorMessage,
		})
	}
	children = append(children, &widget.Form{
		Action: "/system/maintenance/firmware", Multipart: true, AutoSubmit: true, NoSubmit: true,
		Fields: []widget.Widget{&widget.Field{
			Name: "firmware_image", Kind: "file", Accept: ".bin,application/octet-stream",
			Prompt: "Drop a sysupgrade image here", Required: true,
			Help: boardSentence(tr,
				"A sysupgrade image built for this device · .bin, up to 128 MiB",
				"A sysupgrade image built for %s · .bin, up to 128 MiB", board.Model),
			HelpVerbatim: true,
		}},
	})
	m.Children = children
	return m
}

func firmwareVersion(info openwrt.FirmwareInfo) string {
	parts := make([]string, 0, 2)
	if info.Version != "" {
		parts = append(parts, info.Version)
	}
	if info.Revision != "" {
		parts = append(parts, info.Revision)
	}
	if len(parts) == 0 {
		return "Unknown"
	}
	return strings.Join(parts, " · ")
}

func (s *Server) handleFirmwareInspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxFirmwareSize); err != nil {
		s.renderMaintenanceFirmware(w, r, http.StatusUnprocessableEntity, firmwareState{open: true, errorMessage: "The file is too large or could not be uploaded. Choose a sysupgrade image up to 128 MiB."})
		return
	}
	source, header, err := r.FormFile("firmware_image")
	if err != nil {
		s.renderMaintenanceFirmware(w, r, http.StatusUnprocessableEntity, firmwareState{open: true, errorMessage: "Choose an OpenWrt sysupgrade image first."})
		return
	}
	defer source.Close()
	temp, err := os.CreateTemp(s.maintenanceDir, "verso-firmware-*.bin")
	if err != nil {
		http.Error(w, "Could not store the firmware image.", http.StatusInternalServerError)
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
	written, err := io.Copy(temp, io.LimitReader(source, maxFirmwareSize+1))
	if err != nil || written == 0 || written > maxFirmwareSize {
		s.renderMaintenanceFirmware(w, r, http.StatusUnprocessableEntity, firmwareState{open: true, errorMessage: "The selected image is empty, too large, or could not be read."})
		return
	}
	if err := temp.Close(); err != nil {
		http.Error(w, "Could not store the firmware image.", http.StatusInternalServerError)
		return
	}
	info, err := s.backend.ValidateFirmware(r.Context(), s.sessionSID(r), tempPath)
	if err != nil {
		log.Printf("verso: validate firmware: %v", err)
		s.renderMaintenanceFirmware(w, r, http.StatusBadGateway, firmwareState{open: true, errorMessage: "OpenWrt could not inspect the image. Choose another sysupgrade image and try again."})
		return
	}
	if !info.Valid {
		if info.Error != "" {
			log.Printf("verso: firmware rejected: %s", info.Error)
		}
		s.renderMaintenanceFirmware(w, r, http.StatusUnprocessableEntity, firmwareState{open: true, errorMessage: "The image is damaged, unsigned where signatures are required, or intended for a different device. Choose another sysupgrade image and try again."})
		return
	}
	token, err := randomRestoreToken()
	if err != nil {
		http.Error(w, "Could not prepare the firmware upgrade.", http.StatusInternalServerError)
		return
	}
	board, _ := s.backend.Board(r.Context(), s.sessionSID(r))
	model := board.Model
	if model == "" {
		model = "this device"
	}
	keep = true
	pending := pendingFirmware{sid: s.sessionSID(r), path: tempPath, name: path.Base(header.Filename), info: info, model: model, created: time.Now()}
	s.putPendingFirmware(token, pending)
	s.renderMaintenanceFirmware(w, r, http.StatusOK, firmwareState{open: true, verified: true, token: token, name: pending.name, info: info, model: model})
}

func (s *Server) handleFirmwareApply(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	token := r.FormValue("firmware_token")
	pending, ok := s.pendingFirmware(token, s.sessionSID(r))
	if !ok {
		s.renderMaintenanceFirmware(w, r, http.StatusUnprocessableEntity, firmwareState{open: true, errorMessage: "That verified firmware image has expired. Choose it again."})
		return
	}
	verifier, ok := s.auth.(credentialVerifier)
	if !ok || verifier.Verify(r.Context(), s.sessionUser(r), r.FormValue("password")) != nil {
		s.renderMaintenanceFirmware(w, r, http.StatusUnprocessableEntity, firmwareState{open: true, verified: true, token: token, name: pending.name, info: pending.info, model: pending.model, passwordError: "The current password is incorrect."})
		return
	}
	if !s.consumePendingFirmware(token) {
		s.renderMaintenanceFirmware(w, r, http.StatusUnprocessableEntity, firmwareState{open: true, errorMessage: "That verified image was already used. Upload it again to install once more."})
		return
	}
	if err := s.backend.InstallFirmware(r.Context(), s.sessionSID(r), pending.path); err != nil {
		log.Printf("verso: install firmware: %v", err)
		s.putPendingFirmware(token, pending) // re-stage so the operator can retry the same verified image
		s.renderMaintenanceFirmware(w, r, http.StatusBadGateway, firmwareState{open: true, verified: true, token: token, name: pending.name, info: pending.info, model: pending.model, installError: "OpenWrt could not start the firmware upgrade. The verified image is still available to retry."})
		return
	}
	_ = os.Remove(pending.path)
	s.renderFirmwareStarted(w, r)
}

func (s *Server) putPendingFirmware(token string, pending pendingFirmware) {
	s.pendingFirmwareMu.Lock()
	defer s.pendingFirmwareMu.Unlock()
	now := time.Now()
	for key, old := range s.pendingFirmwares {
		if old.sid == pending.sid || now.Sub(old.created) > restoreLifetime {
			_ = os.Remove(old.path)
			delete(s.pendingFirmwares, key)
		}
	}
	s.pendingFirmwares[token] = pending
}

func (s *Server) pendingFirmware(token, sid string) (pendingFirmware, bool) {
	s.pendingFirmwareMu.Lock()
	defer s.pendingFirmwareMu.Unlock()
	pending, ok := s.pendingFirmwares[token]
	if !ok || pending.sid != sid || time.Since(pending.created) > restoreLifetime {
		if ok {
			_ = os.Remove(pending.path)
			delete(s.pendingFirmwares, token)
		}
		return pendingFirmware{}, false
	}
	return pending, true
}

// consumePendingFirmware claims the token under the lock and reports whether it
// was present, so two racing applies of one verified image cannot both flash.
func (s *Server) consumePendingFirmware(token string) bool {
	s.pendingFirmwareMu.Lock()
	defer s.pendingFirmwareMu.Unlock()
	_, ok := s.pendingFirmwares[token]
	delete(s.pendingFirmwares, token)
	return ok
}

func (s *Server) renderFirmwareStarted(w http.ResponseWriter, r *http.Request) {
	lang, _ := s.localize(r)
	var body strings.Builder
	if err := s.pageSet(lang).ExecuteTemplate(&body, "firmware-started.html.tmpl", struct {
		Lang string
		CSS  template.CSS
	}{Lang: langAttr(lang), CSS: s.currentCSS()}); err != nil {
		http.Error(w, "firmware started page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "300")
	_, _ = io.WriteString(w, body.String())
}
