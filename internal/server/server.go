// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package server wires the Verso shell's HTTP routes and lifecycle.
//
// The Server owns routing only. Its side-effecting dependencies — the widget
// renderer, the OpenWrt backend, and the plugin transport — are injected as
// interfaces, so the shell stays unit-testable without a real device or a live
// plugin (see ADR-003, ADR-006).
package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/updatecheck"
	"github.com/we-are-mono/verso/internal/widget"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

//go:embed assets/verso.css
var cssText string

// scriptFS holds the shell's client-side JS and self-hosted fonts, served under
// /assets. The page chrome loads the scripts (ADR-004); CSS loads the embedded
// font subsets without a third-party request.
//
//go:embed assets/htmx.min.js assets/alpine.csp.min.js assets/verso-dev.js assets/verso-boot.js
//go:embed assets/verso.js assets/verso-forms.js assets/verso-tables.js assets/verso-commit.js
//go:embed assets/verso-packages.js
//go:embed assets/verso-system.js
//go:embed assets/verso-stream.js assets/verso-listing.js assets/verso-takeover.js assets/verso-page.js
//go:embed assets/verso-login.js
//go:embed assets/fonts
var scriptFS embed.FS

// devCSSPath is the hot-reload stylesheet drop point. When scripts/dev.sh is driving a
// session it compiles the CSS here, and the shell reads it fresh on every render (and
// serves it at /assets/verso.css for the in-place swap) — so a CSS edit lands with no
// rebuild. The file exists only under dev.sh; every real deployment has no such file and
// uses the embedded stylesheet. (Not under /tmp: the dev container mounts /tmp as tmpfs,
// which `docker cp` cannot write into.)
const devCSSPath = "/usr/share/verso/verso-dev.css"

// The production authenticator must satisfy sidKeeper, or the runtime assertion
// in New silently skips sid renewal and the 300 s-vs-session mismatch returns
// (ADR-007 §7). This guards that wiring at compile time.
var _ sidKeeper = (*openwrt.RPCDAuthenticator)(nil)

// Server is the Verso HTTP shell.
type Server struct {
	mux       *http.ServeMux
	widgets   *widget.Renderer
	backend   openwrt.Backend
	transport plugin.Transport
	// telemetry is the shell's process-wide interface sampler. It owns one
	// sampling clock and one bounded history regardless of viewer count.
	telemetry     telemetrySource
	telemetryStop func()
	telemetryMu   sync.Mutex
	telemetryAt   time.Time
	telemetrySnap telemetry.Snapshot
	telemetryErr  error
	// pendingApply is the non-UCI tail a plugin POST prepares, keyed by the
	// operator's session. The apply drains and clears the session's entry only
	// after rpcd applies the UCI stage, so one operator's tail can never fire under
	// another operator's apply, and a drained or failed action never lingers.
	pendingApplyMu sync.Mutex
	pendingApply   map[string][]plugin.ApplyAction
	// The discovered manifests and their id index, guarded by manifestsMu:
	// the management surface rescans them at runtime after an install or
	// remove (ADR-011 §7), so every read goes through the accessors below.
	manifestsMu sync.RWMutex
	manifests   []plugin.Manifest
	pluginByID  map[string]plugin.Manifest
	// rescan re-reads the manifest directory (wired by cmd/verso; nil in
	// tests that never install). Its result replaces the served set.
	rescan       func() []plugin.Manifest
	auth         Authenticator
	sessions     *Sessions
	loginLimiter *loginLimiter
	// loginInternet reads only the public uplink boolean, without an rpcd session.
	loginInternet func() (bool, error)
	allowedHosts  map[string]bool
	// pages is the page-template cache, one parsed set per installed language
	// with "" the English (identity) set, its {{ t }} bound at parse time
	// (ADR-012). Swapped atomically on a catalog rescan; the request selects
	// its set by the negotiated language.
	pages atomic.Pointer[map[string]*template.Template]
	// bundle is the loaded localization catalogs, read per request to negotiate
	// the language and translate. Immutable after load; a rescan swaps the
	// pointer atomically (ADR-012).
	bundle atomic.Pointer[i18n.Bundle]
	css    template.CSS
	devCSS string // dev hot-reload stylesheet path, "" in a normal build
	bootID string // per-process id handleCSS exposes in dev, so the hot-reload script detects a redeploy
	// probe reports whether a plugin's unix socket accepts a connection — the
	// liveness half of the management surface (ADR-011); a seam so tests need
	// no real sockets.
	probe func(path string) bool
	// stats reads local machine health (CPU busy share, root fullness) for the
	// overview meters; a seam so tests need no kernel.
	stats statSource
	// eventInterval paces the overview stream's readings clock; tests shrink it.
	eventInterval time.Duration
	// readLeases and neighbors feed the device roster and its detail drawer
	// (devices.go), behind seams so tests need none of them.
	readLeases  func() ([]byte, error)
	neighbors   func() ([]sysstat.Neighbor, error)
	bridgePorts func() (map[string]string, error)
	// wanHist accumulates the WAN device's throughput history from the same
	// stream's once-a-second counter observations (wan_history.go).
	wanHist *wanHistory
	// pendingRestores bind a verified, private upload to the session that chose
	// it. Browser forms carry only the opaque token, never a server path.
	pendingRestoreMu  sync.Mutex
	pendingRestores   map[string]pendingRestore
	pendingFirmwareMu sync.Mutex
	pendingFirmwares  map[string]pendingFirmware
	maintenanceDir    string
	// stateDir is where the update check's answer is recorded and read back
	// (ADR-014 §5). The daily cron run writes the same file, so this is a shared
	// location on the device rather than anything this process owns; tests point
	// it at a directory of their own.
	stateDir string
}

// SetAllowedHosts configures the Host allowlist for the DNS-rebinding guard
// (VS-01). An empty list leaves the check disabled; production supplies the
// device's hostnames and LAN addresses.
func (s *Server) SetAllowedHosts(hosts []string) { s.allowedHosts = hostSet(hosts) }

// New constructs a Server. It renders widgets through the injected renderer,
// reads live state through the injected backend, and reaches plugins through the
// injected transport; manifests are the plugins discovered at startup.
func New(
	widgets *widget.Renderer,
	backend openwrt.Backend,
	transport plugin.Transport,
	manifests []plugin.Manifest,
	auth Authenticator,
) (*Server, error) {
	page, err := parsePageTemplates(identityTranslator)
	if err != nil {
		return nil, fmt.Errorf("server: parse templates: %w", err)
	}
	interfaceSampler := telemetry.NewSampler()
	interfaceSampler.Start()
	s := &Server{
		mux:              http.NewServeMux(),
		widgets:          widgets,
		backend:          backend,
		transport:        transport,
		manifests:        manifests,
		pluginByID:       indexByID(manifests),
		auth:             auth,
		sessions:         newSessions(),
		loginLimiter:     newLoginLimiter(time.Now),
		loginInternet:    openwrt.InternetAvailable,
		css:              template.CSS(cssText),
		probe:            probeSocket,
		stats:            sysstat.New(),
		telemetry:        interfaceSampler,
		telemetryStop:    interfaceSampler.Stop,
		eventInterval:    time.Second,
		readLeases:       func() ([]byte, error) { return os.ReadFile(leasesPath) },
		neighbors:        sysstat.Neighbors,
		bridgePorts:      sysstat.BridgePorts,
		wanHist:          newWanHistory(time.Now),
		pendingRestores:  make(map[string]pendingRestore),
		pendingFirmwares: make(map[string]pendingFirmware),
		maintenanceDir:   "/var/run/verso",
		stateDir:         updatecheck.DefaultDir,
	}
	// Start English-only: the page cache holds just the identity set and the
	// bundle stays nil (English) until SetBundle — wired by cmd/verso once the
	// catalogs are loaded, and again on rescan — installs the localized sets.
	s.pages.Store(&map[string]*template.Template{"": page})
	// Enter CSS hot-reload only when the dev drop file is present (scripts/dev.sh);
	// checked once, so a normal deployment pays nothing per render.
	if _, err := os.Stat(devCSSPath); err == nil {
		s.devCSS = devCSSPath
		s.bootID = fmt.Sprintf("%x", time.Now().UnixNano())
	}
	// Keep each session's rpcd sid alive for the session's lifetime and tear it
	// down when the session ends (ADR-007 §7). Only the native authenticator
	// reaches rpcd; a test auth that cannot is left renewal-free.
	if keeper, ok := auth.(sidKeeper); ok {
		s.sessions.startRenewer(keeper, sessionRenewInterval)
	}
	s.routes()
	return s, nil
}

// identityTranslator is the English translator: each source string is its own
// translation. It backs the "" page-template set and any nil per-request lookup.
func identityTranslator(s string) string { return s }

// parsePageTemplates parses the shell's page templates with t bound as the {{ t }}
// function, so each installed language gets its own set (html/template binds funcs
// at parse time). icon is bound as it is for the widget renderer.
func parsePageTemplates(t func(string) string) (*template.Template, error) {
	set, err := template.New("page").
		Funcs(template.FuncMap{"icon": widget.Icon, "t": t}).
		ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, err
	}
	// The shared partials, parsed into the same set: a page that renders a fact
	// the way a table cell does renders it with the same markup (widget.Partials).
	return set.ParseFS(widget.Partials(), "templates/properties.html.tmpl", "templates/shared.html.tmpl")
}

// SetBundle installs a freshly loaded catalog set (ADR-012): it rebuilds the
// page-template cache (English plus one set per installed language) and the widget
// renderer's matching cache, then swaps the bundle pointer — so the request path
// reads immutable snapshots with no lock. Called at startup and on every catalog
// rescan; safe to call while requests render. A nil bundle is ignored, and a parse
// failure keeps the previous caches (it cannot happen for the embedded templates).
func (s *Server) SetBundle(b *i18n.Bundle) {
	if b == nil {
		return
	}
	base, err := parsePageTemplates(identityTranslator)
	if err != nil {
		log.Printf("verso: i18n: parse page templates: %v", err)
		return
	}
	pages := map[string]*template.Template{"": base}
	for _, code := range b.Codes() {
		set, err := parsePageTemplates(b.Translator(code))
		if err != nil {
			log.Printf("verso: i18n: parse page templates for %q: %v", code, err)
			continue
		}
		pages[code] = set
	}
	if err := s.widgets.SetLanguages(b.Codes(), b.Translator); err != nil {
		log.Printf("verso: i18n: rebuild widget templates: %v", err)
		return
	}
	s.pages.Store(&pages)
	s.bundle.Store(b)
}

// pageSet returns the page-template set for a language code, falling back to the
// English set for an empty or uninstalled code.
func (s *Server) pageSet(lang string) *template.Template {
	sets := *s.pages.Load()
	if set, ok := sets[lang]; ok {
		return set
	}
	return sets[""]
}

// localize negotiates the request's language from Accept-Language against the
// installed catalogs, returning the language code and its translator. English —
// no installed match, or no catalogs — is ("", nil): the caller then renders the
// English template set and the render skips the translation walk.
func (s *Server) localize(r *http.Request) (lang string, t func(string) string) {
	b := s.bundle.Load()
	if b == nil {
		return "", nil
	}
	lang = i18n.Negotiate(r.Header.Get("Accept-Language"), b.Codes())
	if lang == "" {
		return "", nil
	}
	return lang, b.Translator(lang)
}

// headingAct renders a listing's forward act for the heading line, where every
// listing keeps its primary. A nil act, or one that fails to render, leaves the
// heading line bare rather than failing the page.
func (s *Server) headingAct(r *http.Request, act *widget.ActionBar, lang string, t func(string) string) template.HTML {
	if act == nil {
		return ""
	}
	var b strings.Builder
	if err := s.widgets.RenderWithToken(&b, act, s.sessionCSRF(r), lang, t); err != nil {
		log.Printf("verso: heading act render failed: %v", err)
		return ""
	}
	return template.HTML(b.String()) //nolint:gosec // rendered by the shell's own templates
}

// translatorOrIdentity returns t, or the identity function for a nil t (English),
// so shell code can localize a string unconditionally.
func translatorOrIdentity(t func(string) string) func(string) string {
	if t == nil {
		return identityTranslator
	}
	return t
}

// localizeLabel localizes a label from its owner's catalog: base (tr) for a
// shell-owned label, the plugin's catalog for a plugin's (ADR-012 §5).
func localizeLabel(pluginID, label string, tr func(string) string, pluginTr func(string) func(string) string) string {
	if pluginID == "" {
		return tr(label)
	}
	return pluginTr(pluginID)(label)
}

// langAttr is the value for <html lang>: the negotiated code, or "en" for English.
func langAttr(lang string) string {
	if lang == "" {
		return "en"
	}
	return lang
}

// localizeBanner returns a copy of the banner with its shell-facing text localized,
// or nil for no banner. The banner is chrome the page template renders, outside the
// widget walk.
func localizeBanner(b *plugin.Banner, tr func(string) string) *plugin.Banner {
	if b == nil {
		return nil
	}
	c := *b
	c.Title = tr(c.Title)
	c.Body = tr(c.Body)
	return &c
}

// localizeNotice returns a copy of the notice with its text localized, or nil
// for no notice — the flash-slot counterpart to localizeBanner.
func localizeNotice(n *plugin.Notice, tr func(string) string) *plugin.Notice {
	if n == nil {
		return nil
	}
	c := *n
	c.Text = tr(c.Text)
	return &c
}

// localizeAction returns a copy of the page action with its label localized, or
// nil for no action. The href and the icon name are addresses, not words.
func localizeAction(a *plugin.PageAction, tr func(string) string) *plugin.PageAction {
	if a == nil {
		return nil
	}
	c := *a
	c.Label = tr(c.Label)
	c.Href = widget.SafeHref(c.Href)
	return &c
}

// localizeBack returns a copy of the masthead back-link with its label localized
// and its glyph fixed, or nil for none. An empty label becomes "Cancel" — the
// quiet default an edit page leans on — and the icon is always the left arrow,
// since the back-link's glyph is the shell's to choose, never the plugin's. The
// href is a shell route, sanitized like any plugin-supplied link.
func localizeBack(a *plugin.PageAction, tr func(string) string) *plugin.PageAction {
	if a == nil {
		return nil
	}
	c := *a
	c.Label = tr(c.Label)
	if c.Label == "" {
		c.Label = tr("Cancel")
	}
	c.Href = widget.SafeHref(c.Href)
	c.Icon = "arrow-left"
	return &c
}

// pluginLocalize is localize for a plugin render: the request's language and the
// plugin's base⊕plugin translator, so the plugin's page and manifest labels resolve
// from its own catalog while shell-owned widget defaults fall through to base
// (ADR-012 §5). English (nil t) when no language is negotiated.
func (s *Server) pluginLocalize(r *http.Request, pluginID string) (lang string, t func(string) string) {
	b := s.bundle.Load()
	if b == nil {
		return "", nil
	}
	lang = i18n.Negotiate(r.Header.Get("Accept-Language"), b.Codes())
	if lang == "" {
		return "", nil
	}
	return lang, b.PluginTranslator(lang, pluginID)
}

// pluginTranslators returns a function mapping a plugin id to its base⊕plugin
// translator for the request's negotiated language, so the nav can localize each
// plugin's label from that plugin's catalog (ADR-012 §5). Identity for English.
func (s *Server) pluginTranslators(r *http.Request) func(pluginID string) func(string) string {
	b := s.bundle.Load()
	lang := ""
	if b != nil {
		lang = i18n.Negotiate(r.Header.Get("Accept-Language"), b.Codes())
	}
	if b == nil || lang == "" {
		return func(string) func(string) string { return identityTranslator }
	}
	return func(pluginID string) func(string) string {
		return b.PluginTranslator(lang, pluginID)
	}
}

// Close stops background services owned by the shell.
func (s *Server) Close() {
	s.sessions.stopRenewer()
	if s.telemetryStop != nil {
		s.telemetryStop()
	}
	s.pendingRestoreMu.Lock()
	for token, pending := range s.pendingRestores {
		_ = os.Remove(pending.path)
		delete(s.pendingRestores, token)
	}
	s.pendingRestoreMu.Unlock()
	s.pendingFirmwareMu.Lock()
	for token, pending := range s.pendingFirmwares {
		_ = os.Remove(pending.path)
		delete(s.pendingFirmwares, token)
	}
	s.pendingFirmwareMu.Unlock()
}

// currentCSS is the stylesheet to inline: the live dev file (read fresh each render) in
// a hot-reload session, otherwise the embedded copy. A missing or empty dev file falls
// back to embedded, so a mid-write read never blanks the page.
func (s *Server) currentCSS() template.CSS {
	if s.devCSS != "" {
		if b, err := os.ReadFile(s.devCSS); err == nil && len(b) > 0 {
			return template.CSS(b)
		}
	}
	return s.css
}

func indexByID(manifests []plugin.Manifest) map[string]plugin.Manifest {
	byID := make(map[string]plugin.Manifest, len(manifests))
	for _, m := range manifests {
		byID[m.ID] = m
	}
	return byID
}

// SetRescan wires the manifest re-reader the management surface runs after an
// install or remove completes (ADR-011 §7).
func (s *Server) SetRescan(rescan func() []plugin.Manifest) { s.rescan = rescan }

// rescanManifests replaces the served manifest set from disk. With no
// re-reader wired it is a no-op — the startup set stands.
func (s *Server) rescanManifests() {
	if s.rescan == nil {
		return
	}
	manifests := s.rescan()
	s.manifestsMu.Lock()
	s.manifests = manifests
	s.pluginByID = indexByID(manifests)
	s.manifestsMu.Unlock()
}

// manifestList is the served manifest set; manifestByID resolves one plugin.
// Both take the read lock so a concurrent rescan swaps atomically underneath.
func (s *Server) manifestList() []plugin.Manifest {
	s.manifestsMu.RLock()
	defer s.manifestsMu.RUnlock()
	return s.manifests
}

func (s *Server) manifestByID(id string) (plugin.Manifest, bool) {
	s.manifestsMu.RLock()
	defer s.manifestsMu.RUnlock()
	m, ok := s.pluginByID[id]
	return m, ok
}

// Handler returns the root HTTP handler for the shell: security headers, then
// the session/CSRF gate, wrapping the routing mux.
func (s *Server) Handler() http.Handler {
	// Bound every request body before requireAuth's CSRF check parses it:
	// validCSRF reads multipart fields for any multipart POST, and an unbounded
	// multipart part spills to tmpfs (TMPDIR). Forms are tiny, so a small default
	// caps them all; the two upload routes lift the cap to their archive/image
	// size, where the handler then applies the tighter check.
	next := s.hostGuard(s.requireAuth(s.mux))
	bounded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !safeMethod(r.Method) {
			limit := int64(2 << 20)
			switch r.URL.Path {
			case "/system/maintenance/restore":
				limit = maxRestoreSize + (1 << 20)
			case "/system/maintenance/firmware":
				limit = maxFirmwareSize + (1 << 20)
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
	return securityHeaders(bounded)
}

// assets serves the shell's embedded client-side JS (ADR-004) under /assets. The
// files are first-party and static; they carry no session data, so the path is
// public (see isPublicPath) and cacheable.
func (s *Server) assets() http.Handler {
	sub, err := fs.Sub(scriptFS, "assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	// No per-asset cache headers: securityHeaders sets Cache-Control: no-store on
	// every response, so assets (JS, CSS, fonts) are refetched each load. Verso is
	// a LAN-local app where a fetch is effectively free, and never caching means a
	// redeployed binary is always what the browser shows (no stale asset, no hash
	// in the URL to bust). The one cost is a possible font re-fetch flash on
	// navigation; on a local network it is negligible.
	return http.StripPrefix("/assets/", http.FileServerFS(sub))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// handleCSS serves the current stylesheet at a stable URL. The page still inlines the
// CSS for first paint; this is what the dev hot-reload script re-fetches to swap the
// <style> in place. Fresh from disk in a dev session, embedded otherwise.
func (s *Server) handleCSS(w http.ResponseWriter, _ *http.Request) {
	if s.devCSS != "" {
		// A CSS swap cannot show a template edit (templates are embedded), so the
		// dev script compares this id and reloads the page when the shell restarts.
		w.Header().Set("X-Verso-Boot", s.bootID)
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(s.currentCSS()))
}

type pageData struct {
	Overview      bool
	UpdateNotice  string
	Lang          string // negotiated language for <html lang>, "en" when English
	Title         string
	Heading       string
	HeadingDetail string // the active subpage's name, muted beside the heading
	Kicker        string // optional eyebrow above the heading (with a live dot when Live)
	KickerStatus  string // optional emerald status beside the kicker
	Live          bool
	Display       bool               // opt into the display masthead without a kicker or lede
	Tone          string             // the heading is a message about now: tint by the tone vocabulary, drop the nav suffix
	Ruled         bool               // the masthead ends in a hairline, ruled off from the page's first section
	Subheading    string             // optional lede under the heading
	Action        *plugin.PageAction // the page's one primary doorway, rendered as a button beside the heading
	HeadingAct    template.HTML      // a listing's lone act, rendered in the Action's place (pageHeader.HeadingAct)
	Back          *plugin.PageAction // an edit page's quiet "← Cancel" back-link, rendered in the masthead above the heading
	Width         string             // content-column width preset: "form" (640px) | "narrow" | "normal" (default) | "wide"
	CSS           template.CSS
	Nav           navModel
	Body          template.HTML
	// The reader mode (ADR-015): Advanced is what the sidebar switch shows, and
	// ModeReturn is the page the flip comes back to — this one.
	Advanced   bool
	ModeReturn string
	NoPassword bool
	Banner     *plugin.Banner
	CSRFToken  string
	// SessionExpiry is the moment this session ends (RFC3339, UTC), restated by
	// every render: the browser follows it out rather than waiting for a click.
	SessionExpiry string
	// Nameplate is the identity the top bar wears: the running router hostname,
	// falling back to prose when a fresh box has no
	// readable name yet. The bar is the device's, not the software's; Verso
	// the name lives on the login page and in Maintenance. Maker and Model sit
	// beside it — who built this board and what they call it, read from
	// /etc/board.json; both empty on a board that says neither.
	Nameplate string
	Maker     string
	Model     string
	Dev       bool       // dev session: inject the CSS hot-reload script
	Staged    stagedView // pending uci changes the staged-changes chip shows (ADR-010)
	// JSStrings is the localized catalog for the strings the shell's client
	// script writes into the page after load (verso.js T) — client JS has no
	// translator, so the render hands it these as a JSON blob (ADR-012).
	JSStrings template.JS
	Pages     []pageTab // the domain's subpages, rendered as the top bar (third navigation tier)
	Modes     []pageTab // optional local views, rendered as a compact switch beside the heading
	// Flash is the one-shot outcome at the top of the content: the PRG
	// confirmation from a redirect, or a plugin envelope's notice.
	FlashVariant string // the tone vocabulary: "success" | "warning" | "danger" | "info" | "" (no flash)
	FlashMessage string
}

// Flash is the page's one-shot outcome as the shared flash partial draws it —
// the same treatment a panel gives its own outcome, so a save reads the same
// wherever it lands.
func (d pageData) Flash() widget.Flash {
	return widget.Flash{Variant: d.FlashVariant, Message: d.FlashMessage}
}

// pageTab is one entry in the top bar: the shell-built href and whether it is
// the page being viewed. PluginID names the plugin that authored the label ("" for
// a shell-owned tab), so it is localized from that plugin's catalog (ADR-012 §5).
type pageTab struct {
	Label    string
	Href     string
	Active   bool
	PluginID string
}

// pageHeader is the masthead the shell renders above a page body. Heading is always
// shown; a page may also declare a kicker (an eyebrow, optionally with a live dot) and a
// lede subheading to get the fuller "your connection, live" header, otherwise it stays a
// plain heading.
type pageHeader struct {
	Heading      string
	Kicker       string
	KickerStatus string
	Immediate    bool
	Live         bool
	// Display opts a page into the display masthead even without a kicker or
	// lede — for a page whose heading is its own subject (the device's name on the
	// Hardware page), not a section label.
	Display bool
	// Tone declares the heading a message about now rather than a place-label:
	// it tints in the closed tone vocabulary ("info" | "success" | "warning" |
	// "danger" | "neutral" — never a colour) and the " — <tab>" navigation
	// suffix drops, both from the one word. "neutral" drops the suffix without
	// a tint. Unknown values are ignored; absent means the plain masthead.
	Tone string
	// Ruled ends the masthead in a hairline: the title and lede are the first
	// of the page's subjects, set off from the next the way its ruled sections
	// are from each other. A listing, whose toolbar follows the heading, is not.
	Ruled bool
	// PanelOnly marks a render that answered with one panel's contents rather
	// than a page — the gateway then sends the body alone, with none of the
	// chrome the frame around it already has.
	PanelOnly bool
	// PreviewOnly marks a render that answered with one live preview block — what
	// the form on screen would write — and nothing else. The page around it is
	// not being replaced: the operator is mid-edit in it.
	PreviewOnly bool
	// PanelDone marks a render that answered a panel's own submission with the
	// outcome alone: the change is in the stage and the panel is closing on it,
	// so the frame swaps nothing and says the outcome where the panel was.
	PanelDone bool
	// StagedStructure marks a staged commit that changed which sections a config
	// holds — one made or removed — rather than the values of ones it already
	// had. The listing drawn from it gains or loses a row, which only a fresh
	// page can show.
	StagedStructure bool
	Subheading      string
	Action          *plugin.PageAction // the page's one primary doorway, hard right on the heading row
	// HeadingAct is a listing's forward act lifted off its control band
	// (widget.TakeHeadingAct), already rendered (Server.headingAct): it takes
	// the Action's place on the heading row and opens what the bar's act would
	// have opened.
	HeadingAct template.HTML
	// Back is an edit page's way home: the shell renders it as a quiet "← Cancel"
	// back-link in the masthead above the heading. The plugin supplies the Href and
	// optionally a Label; the shell defaults the label to "Cancel" and fixes the
	// arrow-left glyph (localizeBack).
	Back   *plugin.PageAction
	Banner *plugin.Banner
	Notice *plugin.Notice // a plugin's outcome for this render, shown in the flash slot
	Modes  []pageTab
	// StagedCommit records that this render staged a uci commit (a successful
	// editor submit). The gateway reads it to send an editor back to its listing
	// after a save, rather than re-rendering the form
	// in place; a listing's inline toggle stages too, but is not a page form, so
	// it is synced in place instead of redirected.
	CommandDone  bool
	StagedCommit bool
}

// renderPage wraps a rendered body in the shell chrome — the <title>, the
// manifest-driven nav with the active link marked, and the page heading — and
// sends it with the given status (200 normally; a plugin's 422 is propagated).
// stages declares whether the page's own edits go through the uci stage: such
// pages carry the staged-changes bar even when clean; immediate-action pages
// get it only when the shared stage is non-empty.
// jsCatalog localizes the fixed set of strings verso.js writes into the page
// after load — the staged-changes chip's words, an inline field's refusal.
// Client JS has no translator (ADR-012), so the render serializes these into
// the #verso-i18n blob; an unknown key falls back to its English source in the
// script exactly as it would in the translator.
func jsCatalog(tr func(string) string) template.JS {
	keys := []string{
		"Enter a valid hostname or IP address.", "Enter a valid IP address.",
		// What a keyboard reorder says once the row has moved, and what a copy
		// button says once the value is on the clipboard.
		"Moved to position %d of %d", "Copied",
		// What staying signed in says once the session has been extended.
		"You’re still signed in.",
		// The staged-changes chip, kept in step after an act on the page
		// stages something, and how an apply went.
		"1 staged change", "%d staged changes",
		"1 change applied", "%d changes applied", "Apply rolled back",
		"Couldn’t apply — check the settings and try again",
		"Couldn’t confirm — the router may have rolled back",
		"Couldn’t discard — try again",
		// Inline-field validation, shown beneath the field on blur.
		"Enter a value.",
		"Use letters, numbers and hyphens — no spaces.",
		"Couldn’t save that just now — try again.",
		// The live listing: its pause control, the shelf of plucked values,
		// what the section's meta says while events flow, and the words a
		// row's age is stated in.
		"Pause", "Resume", "Resume · %d new", "Clear", "Show only %s", "Remove filter %s",
		"%d of %d events shown", "~%d events/s",
		"now", "%d s", "%d min", "%d h",
		// Package and service actions.
		"Packages could not be loaded. Try again.",
		"Could not check refresh status. Try again.",
		"Installed files could not be loaded. Try again.",
		"Could not confirm the action. Check the service state before trying again.",
		"Searching…",
		"Installing…",
		"Removing…",
		"Upgrading…",
		"Refresh index", "Refreshing index…",
		// System logs and staged reboot controls.
		"Paused", "Every source", "Firewall logs unavailable", "Some firewall events were lost.", "Logs unavailable", "Live", "Connecting…", "Nothing matches.",
		"The operation could not be completed. Review staged changes before retrying.",
	}
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k] = tr(k)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return template.JS(b) //nolint:gosec // a marshalled map of catalog strings, not user input
}

// pageTone bounds a declared masthead tone to the closed vocabulary; anything
// else renders the plain masthead, mechanically, the way every unknown semantic
// value degrades (ADR-005).
func pageTone(tone string) string {
	switch tone {
	case "info", "success", "warning", "danger", "neutral":
		return tone
	}
	return ""
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, hdr pageHeader, width string, pages []pageTab, body template.HTML) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	pluginTr := s.pluginTranslators(r)
	mode := readerMode(r)
	staged := s.staged(r.Context(), s.sessionSID(r), tr, pluginTr)
	flashVariant, flashMessage := s.takeFlash(r)
	// A plugin's outcome notice rides the same flash slot as the shell's PRG
	// flash (one treatment for one meaning); the redirect flash, being the
	// operator's own just-completed action, wins if both are present.
	if flashMessage == "" && hdr.Notice != nil && hdr.Notice.Text != "" {
		flashVariant, flashMessage = hdr.Notice.Level, hdr.Notice.Text
	}
	// The top bar mixes shell tabs and plugin tabs; localize each label from its
	// owner's catalog (base for the shell's, the plugin's for a plugin's — ADR-012
	// §5) and name the active face in the headline in muted ink so the domain stays
	// the title.
	headingDetail := ""
	localizedPages := make([]pageTab, len(pages))
	for i, p := range pages {
		p.Label = localizeLabel(p.PluginID, p.Label, tr, pluginTr)
		localizedPages[i] = p
		if p.Active {
			headingDetail = p.Label
		}
	}
	localizedModes := make([]pageTab, len(hdr.Modes))
	for i, mode := range hdr.Modes {
		mode.Label = tr(mode.Label)
		localizedModes[i] = mode
	}
	// Whether root has a password is read from /etc/shadow by verso-rpcd (the
	// shell can't); a helper miss fails safe to "has one" so it never falsely warns.
	hasPassword, hpErr := s.backend.RootHasPassword(r.Context(), s.sessionSID(r))
	if hpErr != nil {
		hasPassword = true
	}
	nameplate := s.nameplate(r)
	hw := board()
	var buf bytes.Buffer
	if err := s.pageSet(lang).ExecuteTemplate(&buf, "page.html.tmpl", pageData{
		Overview:      r.URL.Path == "/",
		UpdateNotice:  s.homeUpdateNotice(r, tr),
		Lang:          langAttr(lang),
		Title:         "Verso",
		Heading:       tr(hdr.Heading),
		HeadingDetail: headingDetail,
		Kicker:        tr(hdr.Kicker),
		Display:       hdr.Display,
		Tone:          pageTone(hdr.Tone),
		Ruled:         hdr.Ruled,
		KickerStatus:  tr(hdr.KickerStatus),
		Live:          hdr.Live,
		Subheading:    tr(hdr.Subheading),
		Action:        localizeAction(hdr.Action, tr),
		HeadingAct:    hdr.HeadingAct,
		Back:          localizeBack(hdr.Back, tr),
		Width:         width,
		CSS:           s.currentCSS(),
		Nav:           s.buildSidebar(r.URL.Path, mode, tr, pluginTr, localizedPages),
		Body:          body,
		Advanced:      mode == widget.ModeAdvanced,
		ModeReturn:    r.URL.RequestURI(),
		NoPassword:    !hasPassword,
		Banner:        localizeBanner(hdr.Banner, tr),
		CSRFToken:     s.sessionCSRF(r),
		SessionExpiry: s.sessionExpiryStamp(r),
		Nameplate:     nameplate,
		Maker:         hw.Maker,
		Model:         hw.Model,
		Dev:           s.devCSS != "",
		Staged:        staged,
		JSStrings:     jsCatalog(tr),
		Pages:         localizedPages,
		Modes:         localizedModes,
		FlashVariant:  flashVariant,
		FlashMessage:  tr(flashMessage),
	}); err != nil {
		http.Error(w, "page error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// handleIndex renders the system-status page from live backend data. Unlike a
// plugin page, this is the shell's own content, so a render failure is a real
// 500, not a contained notice.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// The shell owns the landing page's composition and reads its live facts.
	// Plugin destinations come from their manifests; no plugin markup crosses
	// into this page. A failed source leaves an explicit unavailable state.
	sid := s.sessionSID(r)
	lang, catalog := s.localize(r)
	tr := translatorOrIdentity(catalog)
	now := time.Now()
	zone, offset := now.Zone()
	ov := &widget.Overview{Clock: now.Format("15:04:05"), Zone: zone, Unix: now.Unix(), Offset: offset}
	board, boardErr := s.backend.Board(r.Context(), sid)
	if boardErr == nil {
		ov.Firmware, ov.Kernel, ov.Model = board.Firmware, board.Kernel, board.Model
	} else {
		log.Printf("verso: overview: board unavailable: %v", boardErr)
	}
	if si, err := s.backend.SystemInfo(r.Context(), sid); err == nil {
		ov.Uptime = loginDuration(tr, si.Uptime)
	} else {
		log.Printf("verso: overview: system info unavailable: %v", err)
	}
	// The process-wide sampler normally supplies an already-warm minute. WAN data
	// can still use the low-cost netifd device counters if sampling fails.
	var d, u float64
	wan, wanErr := s.wanStatus(r)
	primaryWAN, primaryWANOK := wan.Primary()
	if wanErr == nil {
		ov.WANKnown, ov.WANUp = true, wan.Up()
		if primaryWANOK {
			ov.WANDevice = primaryWAN.Device
			ov.WANUptime = loginDuration(tr, primaryWAN.Uptime)
		}
	} else {
		log.Printf("verso: overview: wan status unavailable: %v", wanErr)
	}
	snapshot, telemetryOK := s.telemetrySnapshot(r.Context())
	ov.InterfacesKnown = telemetryOK
	if telemetryOK && wanErr == nil && primaryWANOK {
		if device, found := snapshot.Interface(primaryWAN.Device); found {
			ov.DownSeries, ov.UpSeries = telemetryInterfaceRates(device.History)
			if len(ov.DownSeries) != 0 {
				d = ov.DownSeries[len(ov.DownSeries)-1]
				u = ov.UpSeries[len(ov.UpSeries)-1]
			}
		}
	}
	if len(ov.DownSeries) == 0 {
		ov.DownSeries, ov.UpSeries = s.wanHist.Series()
		d, u = s.wanHist.Latest()
	}
	ov.DownVal, ov.UpVal = fmt.Sprintf("%.1f", d), fmt.Sprintf("%.1f", u)
	// The IPv4/IPv6 panel reads the uplink's live connection facts. It is the
	// advanced face of what the internet tile above it says in task language
	// (ADR-015 §2): addresses, gateways and lease times are the machinery behind
	// "you are online", so the basic reading states the verdict and stops there.
	if readerMode(r) == widget.ModeAdvanced {
		if conn, err := s.backend.WANConn(r.Context(), sid); err == nil {
			ov.V4Proto, ov.V4 = conn.V4Proto, wanFactsV4(conn)
			ov.V6Proto, ov.V6 = conn.V6Proto, wanFactsV6(conn)
			for i := range ov.V6 {
				if ov.V6[i].Label == "Expires" {
					ov.V6[i].Value = loginDuration(tr, conn.V6Valid)
				}
			}
		} else {
			log.Printf("verso: overview: wan connection unavailable: %v", err)
			unavailable := []widget.OverviewFact{{Label: "Status", Value: "unavailable"}}
			ov.V4, ov.V6 = unavailable, unavailable
		}
	}
	// The System gauges read live load/CPU/memory/storage; the stream keeps them current.
	ov.SysMetrics = sysMetricsToWidget(s.systemMeters(r.Context(), sid, tr))
	// Hardware sensors — CPU temp, fan, power — resolved through the board profile.
	s.applySensors(ov, board.BoardName, tr)
	// Kernel interfaces come from the same process-wide telemetry snapshot as
	// the WAN graph and are enriched with UCI topology.
	ov.Interfaces = s.interfaceList(r.Context(), sid, snapshot, wan)
	// Devices remain their own page; the Interfaces tile carries their count.
	ov.DevicesOnline, ov.DevicesKnown = s.onlineDevices()
	// Resolve each domain by its registered navigation entry, leaving a tile
	// unlinked while no running plugin serves its destination.
	ov.SecurityHref = s.navLabelHref("Firewall")
	ov.TunnelsHref = s.navLabelHref("Tunnels")
	ov.InterfacesHref = s.navLabelHref("Interfaces")

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, s.reading(r, ov), s.sessionCSRF(r), lang, catalog); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{}, "wide", nil, template.HTML(body.String()))
}

// sysMetricsToWidget maps the System readings onto the overview's gauge fields,
// resolving each metric's icon by name.
func sysMetricsToWidget(rs []meterReading) []widget.OverviewMeter {
	icons := map[string]string{
		"sys-load": "activity", "sys-cpu": "settings",
		"sys-memory": "memory-stick", "sys-storage": "hard-drive",
	}
	out := make([]widget.OverviewMeter, 0, len(rs))
	for _, r := range rs {
		out = append(out, widget.OverviewMeter{
			Name: r.Name, Label: r.Label, Icon: icons[r.Name], Band: r.Band,
			Value: r.Value, Unit: r.Unit, Fill: r.Fill, Detail: r.Detail,
		})
	}
	return out
}

// wanFactsV4 / wanFactsV6 turn the uplink's connection facts into the overview's
// IPv4/IPv6 rows — addresses copyable, the IPv6 lease shown as time remaining. An
// absent side reads "Not configured" rather than an empty column.
func wanFactsV4(c openwrt.WANConn) []widget.OverviewFact {
	var f []widget.OverviewFact
	if c.V4Addr != "" {
		f = append(f, widget.OverviewFact{Label: "Address", Value: c.V4Addr, Copy: true})
	}
	if c.V4Gateway != "" {
		f = append(f, widget.OverviewFact{Label: "Gateway", Value: c.V4Gateway, Copy: true})
	}
	for _, d := range c.V4DNS {
		f = append(f, widget.OverviewFact{Label: "DNS", Value: d, Copy: true})
	}
	if len(f) == 0 {
		return []widget.OverviewFact{{Label: "Status", Value: "Not configured"}}
	}
	return f
}

func wanFactsV6(c openwrt.WANConn) []widget.OverviewFact {
	var f []widget.OverviewFact
	if c.V6Prefix != "" {
		f = append(f, widget.OverviewFact{Label: "Prefix", Value: c.V6Prefix, Copy: true})
	}
	if c.V6Addr != "" {
		f = append(f, widget.OverviewFact{Label: "Address", Value: c.V6Addr, Copy: true})
	}
	if c.V6Gateway != "" {
		f = append(f, widget.OverviewFact{Label: "Gateway", Value: c.V6Gateway, Copy: true})
	}
	for _, d := range c.V6DNS {
		f = append(f, widget.OverviewFact{Label: "DNS", Value: d, Copy: true})
	}
	if c.V6Valid > 0 {
		f = append(f, widget.OverviewFact{Label: "Expires", Value: formatUptime(c.V6Valid)})
	}
	if len(f) == 0 {
		return []widget.OverviewFact{{Label: "Status", Value: "Not configured"}}
	}
	return f
}
