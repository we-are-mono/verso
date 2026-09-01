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
	"github.com/we-are-mono/verso/internal/version"
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
//go:embed assets/htmx.min.js assets/alpine.csp.min.js assets/verso.js assets/verso-dev.js assets/verso-boot.js assets/login.js
//go:embed assets/fonts
var scriptFS embed.FS

// devCSSPath is the hot-reload stylesheet drop point. When scripts/dev.sh is driving a
// session it compiles the CSS here, and the shell reads it fresh on every render (and
// serves it at /assets/verso.css for the in-place swap) — so a CSS edit lands with no
// rebuild. The file exists only under dev.sh; every real deployment has no such file and
// uses the embedded stylesheet. (Not under /tmp: the dev container mounts /tmp as tmpfs,
// which `docker cp` cannot write into.)
const devCSSPath = "/usr/share/verso/verso-dev.css"

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
	// operator's session. Save & Apply drains and clears the session's entry only
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
	allowedHosts map[string]bool
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
	return template.New("page").
		Funcs(template.FuncMap{"icon": widget.Icon, "t": t}).
		ParseFS(templateFS, "templates/*.tmpl")
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
	Lang          string // negotiated language for <html lang>, "en" when English
	Title         string
	Heading       string
	HeadingDetail string // the active subpage's name, muted beside the heading
	Kicker        string // optional eyebrow above the heading (with a live dot when Live)
	KickerStatus  string // optional emerald status beside the kicker
	Live          bool
	Display       bool               // opt into the serif display masthead without a kicker or lede
	Subheading    string             // optional lede under the heading
	Action        *plugin.PageAction // the page's one primary doorway, rendered as a button beside the heading
	Width         string             // content-column width preset: "narrow" | "normal" (default) | "wide"
	CSS           template.CSS
	Nav           navModel
	Body          template.HTML
	NoPassword    bool
	Banner        *plugin.Banner
	CSRFToken     string
	// SessionExpiry is the moment this session ends (RFC3339, UTC), restated by
	// every render: the browser follows it out rather than waiting for a click.
	SessionExpiry string
	// The sidebar's device row: the router's own name, the Verso release it runs
	// as the quiet trailing detail, and UpdateReady when the cached update truth
	// names something to install — the row's amber mark is that truth's only
	// claim on a person's attention anywhere in the chrome.
	DeviceName    string
	DeviceVersion string
	UpdateReady   bool
	Dev           bool        // dev session: inject the CSS hot-reload script
	Capsule       capsuleView // pending uci changes the staged-changes capsule shows (ADR-010)
	// ShowCapsule: staging pages carry the bar always (inert when clean — a
	// real control at rest, ADR-010); pages whose actions are immediate
	// (Plugins, Password, Overview) show it only when the shared stage holds
	// changes from elsewhere — there it is a truth-carrier, not furniture.
	ShowCapsule bool
	// HasPageForm marks a page whose fields are submitted by the capsule's
	// single Save & Apply action rather than by an extra in-content button.
	HasPageForm bool
	Pages       []pageTab // the domain's subpages, rendered as the top bar (third navigation tier)
	Modes       []pageTab // optional local views, rendered as a compact switch beside the heading
	// Flash is the one-shot outcome at the top of the content: the PRG
	// confirmation from a redirect, or a plugin envelope's notice.
	FlashVariant string // the tone vocabulary: "success" | "warning" | "danger" | "info" | "" (no flash)
	FlashMessage string
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
	// Display opts a page into the serif display masthead even without a kicker or
	// lede — for a page whose heading is its own subject (the device's name on the
	// Hardware page), not a section label.
	Display    bool
	Subheading string
	Action     *plugin.PageAction // the page's one primary doorway, hard right on the heading row
	Banner     *plugin.Banner
	Notice     *plugin.Notice // a plugin's outcome for this render, shown in the flash slot
	Modes      []pageTab
}

// renderPage wraps a rendered body in the shell chrome — the <title>, the
// manifest-driven nav with the active link marked, and the page heading — and
// sends it with the given status (200 normally; a plugin's 422 is propagated).
// stages declares whether the page's own edits go through the uci stage: such
// pages carry the staged-changes bar even when clean; immediate-action pages
// get it only when the shared stage is non-empty.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, hdr pageHeader, width string, pages []pageTab, stages bool, body template.HTML) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	pluginTr := s.pluginTranslators(r)
	capsule := s.capsule(r.Context(), s.sessionSID(r))
	hasPageForm := strings.Contains(string(body), "data-verso-page-form")
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
	// The device row names the router itself. A box whose hostname cannot be read
	// still has a row — it just says what it is rather than who it is.
	deviceName, hostErr := s.backend.Hostname(r.Context(), s.sessionSID(r))
	if hostErr != nil || deviceName == "" {
		deviceName = tr("This device")
	}
	updates, updatesKnown := s.updateTruth()
	var buf bytes.Buffer
	if err := s.pageSet(lang).ExecuteTemplate(&buf, "page.html.tmpl", pageData{
		Lang:          langAttr(lang),
		Title:         "Verso",
		Heading:       tr(hdr.Heading),
		HeadingDetail: headingDetail,
		Kicker:        tr(hdr.Kicker),
		Display:       hdr.Display,
		KickerStatus:  tr(hdr.KickerStatus),
		Live:          hdr.Live,
		Subheading:    tr(hdr.Subheading),
		Action:        localizeAction(hdr.Action, tr),
		Width:         width,
		CSS:           s.currentCSS(),
		Nav:           s.buildSidebar(r.URL.Path, tr, pluginTr),
		Body:          body,
		NoPassword:    !hasPassword,
		Banner:        localizeBanner(hdr.Banner, tr),
		CSRFToken:     s.sessionCSRF(r),
		SessionExpiry: s.sessionExpiryStamp(r),
		DeviceName:    deviceName,
		DeviceVersion: version.Version,
		UpdateReady:   updatesKnown && updates.Pending(),
		Dev:           s.devCSS != "",
		Capsule:       capsule,
		ShowCapsule:   stages || capsule.Count > 0,
		HasPageForm:   hasPageForm,
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
	// The advanced overview, transferred hardcoded from the design: verdict,
	// status tiles, IPv4/IPv6 facts, the traffic graph, System, and the Interfaces
	// listing (a flat Table). It opens on the verdict, so the page carries no
	// masthead heading. The System panel's firmware/kernel/uptime are live; the
	// shell fills them, degrading to "unavailable" on a backend miss.
	sid := s.sessionSID(r)
	ov := &widget.Overview{}
	board, boardErr := s.backend.Board(r.Context(), sid)
	if boardErr == nil {
		ov.Firmware, ov.Kernel, ov.Model = board.Firmware, board.Kernel, board.Model
	} else {
		log.Printf("verso: overview: board unavailable: %v", boardErr)
	}
	if si, err := s.backend.SystemInfo(r.Context(), sid); err == nil {
		ov.Uptime = formatUptime(si.Uptime)
	} else {
		log.Printf("verso: overview: system info unavailable: %v", err)
	}
	// The process-wide sampler normally supplies an already-warm minute. WAN data
	// can still use the low-cost netifd device counters if sampling fails.
	var d, u float64
	wan, wanErr := s.backend.WANStatus(r.Context(), sid)
	primaryWAN, primaryWANOK := wan.Primary()
	if wanErr == nil {
		ov.WANKnown, ov.WANUp = true, wan.Up()
		if primaryWANOK {
			ov.WANDevice = primaryWAN.Device
			ov.WANUptime = formatUptime(primaryWAN.Uptime)
		}
	} else {
		log.Printf("verso: overview: wan status unavailable: %v", wanErr)
	}
	snapshot, telemetryOK := s.telemetrySnapshot(r.Context())
	ov.WiFiPresent = telemetryOK && len(snapshot.WirelessPHYs) != 0
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
	// The IPv4/IPv6 panel reads the uplink's live connection facts.
	if conn, err := s.backend.WANConn(r.Context(), sid); err == nil {
		ov.V4Proto, ov.V4 = conn.V4Proto, wanFactsV4(conn)
		ov.V6Proto, ov.V6 = conn.V6Proto, wanFactsV6(conn)
	} else {
		log.Printf("verso: overview: wan connection unavailable: %v", err)
		unavailable := []widget.OverviewFact{{Label: "Status", Value: "unavailable"}}
		ov.V4, ov.V6 = unavailable, unavailable
	}
	// The System gauges read live load/CPU/memory/storage; the stream keeps them current.
	ov.SysMetrics = sysMetricsToWidget(s.systemMeters(r.Context(), sid))
	// Hardware sensors — CPU temp, fan, power — resolved through the board profile.
	s.applySensors(ov, board.BoardName)
	// Kernel interfaces come from the same process-wide telemetry snapshot as
	// the WAN graph and are enriched with UCI topology.
	ov.Interfaces = s.interfaceList(r.Context(), sid, snapshot, wan)
	// The roster is its own page; the tile carries the one number and the way in.
	ov.DevicesOnline, ov.DevicesKnown = s.onlineDevices()
	ov.DevicesHref = devicesPath
	// Each remaining tile is the doorway to the page its fact belongs to. Security
	// resolves through the same section resolver the sidebar's row uses, so the two
	// surfaces can never disagree about where the domain lives — or that it is not
	// there. The software tile reads the cached update truth and nothing live: the
	// home page never waits on a package feed.
	ov.SecurityHref = liveSectionHref(s.sectionHref("Security"))
	ov.SoftwareHref, ov.UpdatesHref = packagesPath, maintenancePath
	if truth, known := s.updateTruth(); known {
		ov.UpdatesKnown = true
		ov.UpdatesPackages = len(truth.Packages)
		ov.UpdatesFirmware = truth.Firmware.State == openwrt.FirmwareUpdateAvailable
		ov.UpdatesCheckedAgo = humanAgo(time.Since(truth.CheckedAt))
	}

	var body strings.Builder
	lang, t := s.localize(r)
	if err := s.widgets.RenderWithToken(&body, ov, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{}, "", nil, false, template.HTML(body.String()))
}

// sysMetricsToWidget maps the System readings onto the overview's gauge fields,
// resolving each metric's icon by name.
func sysMetricsToWidget(rs []meterReading) []widget.OverviewMeter {
	icons := map[string]string{
		"sys-load": "activity", "sys-cpu": "cpu",
		"sys-memory": "memory-stick", "sys-storage": "hard-drive",
	}
	out := make([]widget.OverviewMeter, 0, len(rs))
	for _, r := range rs {
		out = append(out, widget.OverviewMeter{
			Name: r.Name, Label: r.Label, Icon: icons[r.Name], Role: r.Role,
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
