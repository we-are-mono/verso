// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/plugin"
)

// answer is what one plugin endpoint does when the shell asks it.
type answer struct {
	widget string // the envelope's widget; empty with empty set answers 204
	empty  bool
	err    error
	delay  time.Duration
	notice *plugin.Notice
	commit []plugin.CommitOp
	act    string // the page's heading act, its drawer and all
	back   string // the page's return address
}

// slotTransport answers by socket and path, as several plugins would, and
// records each request it is sent.
type slotTransport struct {
	mu       sync.Mutex
	answers  map[string]answer // "<socket> <path>"
	requests []plugin.Request
	sockets  []string
}

func (f *slotTransport) Fetch(ctx context.Context, target plugin.Endpoint, req plugin.Request) (*plugin.Envelope, error) {
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.sockets = append(f.sockets, target.Socket)
	a, ok := f.answers[target.Socket+" "+req.Path]
	f.mu.Unlock()
	if !ok {
		return nil, errors.New("no such endpoint")
	}
	select {
	case <-time.After(a.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	switch {
	case a.err != nil:
		return nil, a.err
	case a.empty:
		return &plugin.Envelope{Empty: true, Status: http.StatusNoContent}, nil
	}
	env := &plugin.Envelope{SchemaVersion: 1, Title: "T", Widget: json.RawMessage(a.widget), Notice: a.notice, Commit: a.commit, Status: http.StatusOK}
	if a.act != "" {
		env.Act = json.RawMessage(a.act)
	}
	if a.back != "" {
		env.Back = &plugin.PageAction{Label: "Back", Href: a.back}
	}
	return env, nil
}

func (f *slotTransport) Describe(context.Context, plugin.Endpoint, []plugin.DescribeChange, plugin.UCI) ([]plugin.Description, error) {
	return nil, nil
}

func (f *slotTransport) Alive(plugin.Endpoint) bool { return true }

// asked is the requests sent to one plugin's socket, by path.
func (f *slotTransport) asked(socket string) map[string]plugin.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]plugin.Request{}
	for i, req := range f.requests {
		if f.sockets[i] == socket {
			out[req.Path] = req
		}
	}
	return out
}

func slotPlugin(id string, nav bool, contributes ...plugin.Contribution) plugin.Manifest {
	m := plugin.Manifest{ID: id, Name: id, Socket: "/run/" + id + ".sock", SchemaVersion: supportedSchemaVersion, Contributes: contributes}
	if nav {
		m.Nav = []plugin.NavEntry{{Section: "System", Label: id, Path: "/"}}
	}
	return m
}

// certificatePage is the host's page: a facts list and an act row that each
// open a slot, and a notice slot.
const certificatePage = `{"type":"section","title":"HTTPS certificate","children":[
	{"type":"properties","items":[{"label":"Issued to","value":"router.example.com"}],"slot":"certificate.source"},
	{"type":"stack","inline":true,"children":[
		{"type":"link","label":"Install a certificate","href":"/plugins/system/install","style":"act","icon":"upload"},
		{"type":"slot","id":"certificate.acts","kind":"act"}]},
	{"type":"slot","id":"certificate.notices","kind":"notice"}]}`

func slotServer(t *testing.T, tr *slotTransport, manifests ...plugin.Manifest) *Server {
	t.Helper()
	// The operator may write what each plugin declares.
	return newServerWith(t, fakeBackend{access: true}, tr, manifests)
}

func TestContributionsFillTheSlotsAPageOpens(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /":     {widget: certificatePage},
		"/run/acme.sock /source": {widget: `{"type":"properties","items":[{"label":"Source","value":"Your domain"}]}`},
		"/run/acme.sock /use":    {widget: `{"type":"link","label":"Use your domain","href":"/plugins/acme/setup","style":"act","icon":"globe","panel":true}`},
		"/run/acme.sock /warn":   {widget: `{"type":"callout","variant":"warning","body":"Replacing it turns off the trusted certificate."}`},
	}}
	s := slotServer(t, tr,
		slotPlugin("system", true),
		slotPlugin("acme", false,
			contribution("source", "system.certificate.source", ""),
			contribution("use", "system.certificate.acts", ""),
			contribution("warn", "system.certificate.notices", ""),
		),
	)
	rec := get(t, s, "/plugins/system/")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	issued, source := strings.Index(body, "Issued to"), strings.Index(body, "Your domain")
	install, use := strings.Index(body, "Install a certificate"), strings.Index(body, "Use your domain")
	if issued < 0 || source < issued || install < 0 || use < install {
		t.Errorf("contributed fact and act do not follow the host's own:\n%s", body)
	}
	if !strings.Contains(body, "turns off the trusted certificate") {
		t.Error("the notice slot is not filled")
	}
	asked := tr.asked("/run/acme.sock")
	for path, slot := range map[string]string{"/source": "system.certificate.source", "/use": "system.certificate.acts", "/warn": "system.certificate.notices"} {
		if asked[path].Slot != slot {
			t.Errorf("%s asked with slot %q, want %q", path, asked[path].Slot, slot)
		}
		if asked[path].Method != http.MethodGet {
			t.Errorf("%s asked with %s", path, asked[path].Method)
		}
	}
}

func TestOnlyTheSlotsBeingDrawnAreAsked(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /": {widget: certificatePage},
	}}
	s := slotServer(t, tr, slotPlugin("system", true), slotPlugin("acme", false, contribution("later", "system.install.notices", "")))
	get(t, s, "/plugins/system/")
	if asked := tr.asked("/run/acme.sock"); len(asked) != 0 {
		t.Errorf("asked %v for a slot that is not on the page", asked)
	}
}

func TestContributionsAreAskedTogether(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{"/run/system.sock /": {widget: certificatePage}}}
	var manifests []plugin.Manifest
	manifests = append(manifests, slotPlugin("system", true))
	for _, id := range []string{"a", "b", "c"} {
		tr.answers["/run/"+id+".sock /act"] = answer{delay: 200 * time.Millisecond, widget: `{"type":"link","label":"Act ` + id + `","href":"/x","style":"act","icon":"globe"}`}
		manifests = append(manifests, slotPlugin(id, false, contribution("act", "system.certificate.acts", "")))
	}
	s := slotServer(t, tr, manifests...)
	start := time.Now()
	body := get(t, s, "/plugins/system/").Body.String()
	if took := time.Since(start); took > 450*time.Millisecond {
		t.Errorf("three 200 ms contributions took %v; they are not asked together", took)
	}
	a, b, c := strings.Index(body, "Act a"), strings.Index(body, "Act b"), strings.Index(body, "Act c")
	if a < 0 || b < a || c < b {
		t.Error("contributed acts are not in plugin id order")
	}
}

func TestAContributionWithNothingToShowDrawsNothing(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /":     {widget: certificatePage},
		"/run/acme.sock /source": {empty: true},
	}}
	s := slotServer(t, tr, slotPlugin("system", true), slotPlugin("acme", false, contribution("source", "system.certificate.source", "")))
	body := get(t, s, "/plugins/system/").Body.String()
	if strings.Contains(body, "Plugin unavailable") || strings.Contains(body, "isn’t responding") {
		t.Error("a contribution with nothing to show drew the unavailable state")
	}
	if !strings.Contains(body, "Issued to") {
		t.Error("the host's own facts are gone")
	}
}

func TestAStoppedContributorLeavesNoPlaceholder(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /":  {widget: certificatePage},
		"/run/acme.sock /use": {err: errors.New("connection refused")},
	}}
	s := slotServer(t, tr, slotPlugin("system", true), slotPlugin("acme", false, contribution("use", "system.certificate.acts", "")))
	s.probe = func(e plugin.Endpoint) bool { return e.Socket != "/run/acme.sock" }
	rec := get(t, s, "/plugins/system/")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "isn’t responding") {
		t.Errorf("a stopped contributor left a placeholder (%d)", rec.Code)
	}
}

func TestAFailingBlockIsContainedInItsPlace(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{
		"/run/host.sock /":     {widget: `{"type":"stack","children":[{"type":"section","title":"Own block","children":[]},{"type":"slot","id":"blocks","kind":"block"}]}`},
		"/run/acme.sock /part": {err: errors.New("timeout")},
	}}
	s := slotServer(t, tr, slotPlugin("host", true), slotPlugin("acme", false, contribution("part", "host.blocks", "")))
	rec := get(t, s, "/plugins/host/")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "isn’t responding") || !strings.Contains(body, "Own block") {
		t.Errorf("a failing block is not contained in its place (%d):\n%s", rec.Code, body)
	}
}

func TestAFailingActIsLeftOut(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /":  {widget: certificatePage},
		"/run/acme.sock /use": {err: errors.New("timeout")},
	}}
	s := slotServer(t, tr, slotPlugin("system", true), slotPlugin("acme", false, contribution("use", "system.certificate.acts", "")))
	rec := get(t, s, "/plugins/system/")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, "isn’t responding") || !strings.Contains(body, "Install a certificate") {
		t.Errorf("a failing act is not left out of its row (%d):\n%s", rec.Code, body)
	}
}

func TestAContributionOfAnotherKindIsNotDrawn(t *testing.T) {
	for name, tc := range map[string]struct {
		slot, widget string
		drawn        bool
	}{
		"a section where an act belongs": {"system.certificate.acts", `{"type":"section","title":"Odd","children":[]}`, false},
		"a link with no icon":            {"system.certificate.acts", `{"type":"link","label":"Odd","href":"/x","style":"act"}`, false},
		"an act":                         {"system.certificate.acts", `{"type":"link","label":"Odd","href":"/x","style":"act","icon":"globe"}`, true},
		"a table where facts belong":     {"system.certificate.source", `{"type":"table","columns":[],"rows":[],"title":"Odd"}`, false},
		"facts":                          {"system.certificate.source", `{"type":"properties","items":[{"label":"Odd","value":"x"}]}`, true},
		"a callout where a notice goes":  {"system.certificate.notices", `{"type":"callout","variant":"info","body":"Odd"}`, true},
		"a section where a notice goes":  {"system.certificate.notices", `{"type":"section","title":"Odd","children":[]}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			tr := &slotTransport{answers: map[string]answer{
				"/run/system.sock /":   {widget: certificatePage},
				"/run/odd.sock /piece": {widget: tc.widget},
			}}
			s := slotServer(t, tr, slotPlugin("system", true), slotPlugin("odd", false, contribution("piece", tc.slot, "")))
			body := get(t, s, "/plugins/system/").Body.String()
			if strings.Contains(body, "Odd") != tc.drawn {
				t.Errorf("drawn = %v, want %v:\n%s", !tc.drawn, tc.drawn, body)
			}
			if !strings.Contains(body, "Install a certificate") {
				t.Error("the host page did not draw")
			}
		})
	}
}

func TestSlotsAreFilledTwoLevelsDeep(t *testing.T) {
	tr := &slotTransport{answers: map[string]answer{
		"/run/host.sock /": {widget: `{"type":"stack","children":[{"type":"slot","id":"blocks","kind":"block"}]}`},
		// system's block, standing in host's slot, opens one of its own...
		"/run/system.sock /part": {widget: `{"type":"section","title":"Part","children":[{"type":"stack","inline":true,"children":[{"type":"slot","id":"acts","kind":"act"}]}]}`},
		// ...which acme fills; acme's act holds a slot nobody's answer fills.
		"/run/acme.sock /use":  {widget: `{"type":"link","label":"Use your domain","href":"/x","style":"act","icon":"globe"}`},
		"/run/deep.sock /deep": {widget: `{"type":"link","label":"Too deep","href":"/x","style":"act","icon":"globe"}`},
	}}
	tr.answers["/run/acme.sock /blocks"] = answer{widget: `{"type":"section","title":"Acme block","children":[{"type":"slot","id":"inner","kind":"act"}]}`}
	s := slotServer(t, tr,
		slotPlugin("host", true),
		slotPlugin("system", false, contribution("part", "host.blocks", "")),
		slotPlugin("acme", false, contribution("use", "system.acts", ""), contribution("blocks", "system.acts", "")),
		slotPlugin("deep", false, contribution("deep", "acme.inner", "")),
	)
	body := get(t, s, "/plugins/host/").Body.String()
	if !strings.Contains(body, "Use your domain") {
		t.Error("a slot a contribution opens is not filled")
	}
	if strings.Contains(body, "Too deep") || len(tr.asked("/run/deep.sock")) != 0 {
		t.Error("a slot two contributions deep was filled")
	}
}

// Each contribution reads in its own plugin's words, the host's catalog
// never standing in for them: the host knows nothing of what acme says.
func TestContributionsSpeakTheReadersLanguage(t *testing.T) {
	bundle, problems := i18n.Load(fstest.MapFS{
		"sl/base.json":   {Data: []byte(`{"Discard":"Zavrzi"}`)},
		"sl/system.json": {Data: []byte(`{"Issued to":"Izdano za","Source":"Gostitelj"}`)},
		"sl/acme.json":   {Data: []byte(`{"Source":"Vir","Your domain":"Vaša domena","Use your domain":"Uporabi svojo domeno"}`)},
	}, "*/*.json")
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /":     {widget: certificatePage},
		"/run/acme.sock /source": {widget: `{"type":"properties","items":[{"label":"Source","value":"Your domain"}]}`},
		"/run/acme.sock /use":    {widget: `{"type":"link","label":"Use your domain","href":"/plugins/acme/setup","style":"act","icon":"globe","panel":true}`},
	}}
	s := slotServer(t, tr, slotPlugin("system", true), slotPlugin("acme", false,
		contribution("source", "system.certificate.source", ""),
		contribution("use", "system.certificate.acts", ""),
	))
	s.SetBundle(bundle)
	body := getLang(t, s, "/plugins/system/", "sl-SI")
	for _, want := range []string{"Izdano za", ">Vir<", "Vaša domena", "Uporabi svojo domeno"} {
		if !strings.Contains(body, want) {
			t.Errorf("Slovenian page missing %q", want)
		}
	}
	if strings.Contains(body, "Gostitelj") {
		t.Error("acme's row was read in the host's words")
	}
}

// A drawer asked for on its own, over the page that opened it, has its slots
// filled as a page does: the Install drawer's notice slot takes a source's
// word on what replacing the certificate costs.
func TestADrawersSlotsAreFilledWhenItOpens(t *testing.T) {
	drawer := `{"label":"Install certificate","href":"/plugins/system/install","drawer":{"title":"Install a certificate","open":true,"closed":"/system/access","children":[
		{"type":"form","submit":"Install certificate","fields":[{"type":"slot","id":"certificate.notices","kind":"notice"},{"type":"field","name":"certificate","label":"Certificate"}]}]}}`
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /install": {widget: `{"type":"stack","children":[]}`, act: drawer},
		"/run/acme.sock /warn":      {widget: `{"type":"callout","variant":"warning","body":"Replacing it turns off the trusted certificate."}`},
	}}
	s := slotServer(t, tr, slotPlugin("system", true), slotPlugin("acme", false, contribution("warn", "system.certificate.notices", "")))
	req := httptest.NewRequest(http.MethodGet, "/plugins/system/install", nil)
	req.Header.Set("HX-Request", "true")
	body := serve(t, s, req).Body.String()
	if !strings.Contains(body, "Install a certificate") {
		t.Fatalf("no drawer:\n%s", body)
	}
	if !strings.Contains(body, "turns off the trusted certificate") {
		t.Errorf("the drawer's notice slot is not filled:\n%s", body)
	}
}

// A drawer's own address, visited as a page (a card's door, a bookmark),
// lands on the page it opens over, which opens it: the drawer alone has no
// page under it to stand on.
func TestADrawersAddressLandsOnItsPageWithItOpen(t *testing.T) {
	drawer := `{"label":"Use your domain","href":"/plugins/acme/setup","drawer":{"title":"Trusted certificate","open":true,"closed":"/system/access","children":[{"type":"text","markdown":"Set up."}]}}`
	tr := &cardTransport{slotTransport: slotTransport{answers: map[string]answer{
		"/run/acme.sock /setup": {widget: `{"type":"stack","children":[]}`, act: drawer, back: "/system/access"},
	}}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{slotPlugin("acme", false, contribution("use", "system.certificate.acts", ""))})
	rec := get(t, s, "/plugins/acme/setup")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/access?drawer=%2Fplugins%2Facme%2Fsetup" {
		t.Errorf("a visit answers %d → %q, want Access with the drawer named", rec.Code, rec.Header().Get("Location"))
	}
	// Asked for as the drawer it is, it is answered as one.
	req := httptest.NewRequest(http.MethodGet, "/plugins/acme/setup", nil)
	req.Header.Set("HX-Request", "true")
	if panel := serve(t, s, req); panel.Code != http.StatusOK || !strings.Contains(panel.Body.String(), "Set up.") {
		t.Errorf("the drawer asked for as one answers %d", panel.Code)
	}
}

// The system plugin's Install drawer, kept under its part of Access, lands on
// Access with the drawer open, as a drawer of any plugin does.
func TestADrawerUnderAnAccessPartLandsOnAccessWithItOpen(t *testing.T) {
	drawer := `{"label":"Install certificate","href":"/plugins/system/access/certificate/install","drawer":{"title":"Install a certificate","open":true,"closed":"/system/access","children":[]}}`
	tr := &slotTransport{answers: map[string]answer{
		"/run/system.sock /access/certificate/install": {widget: `{"type":"stack","children":[]}`, act: drawer, back: "/system/access"},
	}}
	system := slotPlugin("system", false)
	system.SystemAccess = "/access"
	s := slotServer(t, tr, system)
	rec := get(t, s, "/plugins/system/access/certificate/install")
	if want := "/system/access?drawer=%2Fplugins%2Fsystem%2Faccess%2Fcertificate%2Finstall"; rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
		t.Errorf("want Access opening the drawer, got %d → %q", rec.Code, rec.Header().Get("Location"))
	}
}

// A page with a body of its own that also has a drawer open is a page: it is
// drawn where it is.
func TestAPageWithADrawerOpenIsNotSentAway(t *testing.T) {
	drawer := `{"label":"Edit","href":"/plugins/host/x","drawer":{"title":"Edit","open":true,"closed":"/plugins/host/","children":[]}}`
	tr := &slotTransport{answers: map[string]answer{
		"/run/host.sock /x": {widget: `{"type":"section","title":"Listing","children":[]}`, act: drawer, back: "/plugins/host/"},
	}}
	s := slotServer(t, tr, slotPlugin("host", true))
	if rec := get(t, s, "/plugins/host/x"); rec.Code != http.StatusOK {
		t.Errorf("a page with its own body answers %d, want it drawn", rec.Code)
	}
}

func TestAContributionsFormPostsToItsPluginAlone(t *testing.T) {
	block := `{"type":"form","style":"page","fields":[{"type":"field","name":"domain","label":"Domain"}]}`
	tr := &slotTransport{answers: map[string]answer{
		"/run/host.sock /":  {widget: `{"type":"stack","children":[{"type":"slot","id":"blocks","kind":"block"}]}`},
		"/run/a.sock /part": {widget: `{"type":"section","title":"A part","children":[` + block + `]}`, notice: &plugin.Notice{Level: "success", Text: "Saved A."}, commit: []plugin.CommitOp{{Config: "a", Section: "main", Values: map[string]any{"domain": "x"}}}},
		"/run/b.sock /part": {widget: `{"type":"section","title":"B part","children":[]}`},
	}}
	a := slotPlugin("a", false, contribution("part", "host.blocks", ""))
	a.ACL.Write = []plugin.ACLScope{{Scope: "uci", Object: "a", Function: "write"}}
	s := slotServer(t, tr, slotPlugin("host", true), a, slotPlugin("b", false, contribution("part", "host.blocks", "")))
	page := get(t, s, "/plugins/host/").Body.String()
	if !strings.Contains(page, `action="/plugins/host/?contribution=a.part"`) {
		t.Fatalf("the contributed form does not post to its host page:\n%s", page)
	}
	rec := postPlugin(t, s, "/plugins/host/?contribution=a.part", url.Values{"domain": {"x"}})
	if got := tr.asked("/run/a.sock")["/part"].Method; got != http.MethodPost {
		t.Errorf("a was asked with %s, want POST", got)
	}
	if got := tr.asked("/run/b.sock")["/part"].Method; got == http.MethodPost {
		t.Error("b received a's post")
	}
	if got := tr.asked("/run/host.sock")["/"].Method; got == http.MethodPost {
		t.Error("the host page received a's post")
	}
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/plugins/host/" {
		t.Errorf("a staged save answers %d → %q, want a read of the page again", rec.Code, rec.Header().Get("Location"))
	}
}
