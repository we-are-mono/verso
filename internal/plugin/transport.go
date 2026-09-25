// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HeaderUCI carries the uci read snapshot the shell brokers to a plugin (ADR-007),
// as base64-encoded JSON. base64 keeps arbitrary config values — non-ASCII, or
// punctuation a raw header would mangle — intact across the header.
const HeaderUCI = "X-Verso-UCI"

// HeaderUbus carries the brokered helper reads that ride beside the uci snapshot
// (ADR-007), base64-encoded JSON keyed by the function that produced each result.
const HeaderUbus = "X-Verso-Ubus"

// HeaderDescribe marks a request as a describe call rather than a page render: the
// plugin answers the shell's pending-change list with plain-language descriptions
// of its own changes for the review drawer, not a widget envelope. A plugin
// that implements no describe hook returns nothing, and the shell falls back to the
// raw uci line — describe is always optional, the raw line always present.
const HeaderDescribe = "X-Verso-Describe"

// maxEnvelopeBytes bounds how much a plugin can return, so a misbehaving plugin
// cannot exhaust shell memory. Widget schema for an admin page is tiny; a
// megabyte is generous.
const maxEnvelopeBytes = 1 << 20

// Request is what the shell forwards to a plugin over its socket: the browser's
// method, sub-path (below the /plugins/<id>/ mount), query, any form body, and the
// reads the shell brokered on the plugin's behalf (ADR-007) — the uci snapshot and
// the results of the helper functions the plugin declared.
type Request struct {
	Method string
	Path   string
	Query  map[string][]string
	Form   map[string][]string
	UCI    UCI
	Ubus   Ubus
}

// UCI is the read snapshot the shell injects into a plugin request (ADR-007):
// config name → the rpcd `uci get` values for that config (section name → section
// table, carrying its `.type`/`.name` meta and its options). The shell reads each
// config the plugin declared in acl.read with the operator's sid and hands the
// result down, so a session-less plugin renders from config without linking a uci
// library or reading /etc/config. A list option is a JSON array; a scalar is a
// string.
type UCI map[string]map[string]any

// Ubus is the second read the shell injects (ADR-007): helper function name → the
// result that function returned, verbatim. It carries live system state a plugin
// cannot reach itself — nftables counters, for one — read with the operator's sid
// through the privileged helper. The shell brokers only functions it knows and the
// plugin declared; a read that fails contributes nothing, so a plugin renders
// without an entry rather than seeing an error. The shell does not interpret the
// results: they cross as raw JSON and the plugin owns their meaning.
type Ubus map[string]json.RawMessage

// Envelope is a plugin's schema response (ADR-006 §4). Widget is the raw schema
// subtree, decoded by the widget package — the transport stays ignorant of the
// widget vocabulary. Status is the HTTP status the plugin returned (200 or 422),
// set by the transport so the gateway can propagate a validation failure to the
// browser; it is not part of the plugin's JSON.
type Envelope struct {
	Commands []ApplyAction `json:"commands,omitempty"`
	// Entities is a bulk roster contribution. nil means unavailable; an empty
	// list means the read succeeded and no subjects have configuration.
	Entities *[]EntitySummary `json:"entities,omitempty"`

	SchemaVersion int         `json:"schema_version"`
	Title         string      `json:"title"`
	Kicker        string      `json:"kicker"`        // optional eyebrow above the heading, e.g. "System"
	KickerStatus  string      `json:"kicker_status"` // optional emerald completion/state label beside the kicker
	Immediate     bool        `json:"immediate"`     // page actions are immediate: nothing on the page stages
	Live          bool        `json:"live"`          // optional pulsing dot on the kicker
	Tone          string      `json:"tone"`          // the title is a message about now: tint by the tone vocabulary, drop the nav suffix
	Subheading    string      `json:"subheading"`    // optional lede under the heading
	Width         string      `json:"width"`         // page width preset: "form" (640px) | "narrow" | "normal" (default) | "wide"
	Pages         []PageTab   `json:"pages"`         // optional third navigation tier: this domain's subpages, rendered as the shell's top bar
	Action        *PageAction `json:"action"`        // optional primary doorway for the whole page, rendered beside the heading
	// Back is an edit page's quiet way home: the shell renders it as a "← Cancel"
	// back-link in the masthead, above the heading, so a page reached to edit one
	// record can return to the listing it came from. It reuses the PageAction shape,
	// but the plugin sets only Href and, if it wants other words, Label — the shell
	// fixes the glyph to arrow-left and defaults the label to "Cancel", so any
	// Back.Icon a plugin sends is ignored. Href is a route through the shell, like
	// the action's.
	Back   *PageAction     `json:"back"`
	Banner *Banner         `json:"banner"` // optional full-width semantic notice beneath the subpage bar
	Notice *Notice         `json:"notice"` // optional outcome flash for this render, shown in the shell's flash slot
	Widget json.RawMessage `json:"widget"`
	// CTA and Consequence are the commit row's words when this envelope is one
	// tab of a shell-owned entity panel: the verb for applying it ("Reserve
	// address"), and what applying it costs ("Applies immediately — dnsmasq
	// reloads, no rollback needed"). They belong to the plugin because only the
	// plugin knows; a tab that stages nothing sets neither and no commit row is
	// drawn. Ignored on an ordinary page render, where the form carries its own.
	CTA         string `json:"cta,omitempty"`
	Consequence string `json:"consequence,omitempty"`
	// State is where this tab's subject stands in one or two words — "blocked",
	// "no limit", an address. It rides as a small chip beside the tab's label so
	// the strip answers the question the panel was opened to ask before anything
	// is clicked. Only the plugin knows it; a tab that has no state to state
	// sets none and wears no chip.
	State  string        `json:"state,omitempty"`
	Commit []CommitOp    `json:"commit"`
	Apply  []ApplyAction `json:"apply"`
	Status int           `json:"-"`
}

// EntitySummary keeps configured subjects findable in the shell's roster.
// State names configuration, not live enforcement; the tab owns its details.
type EntitySummary struct {
	ID      string                `json:"id"`
	Name    string                `json:"name,omitempty"`
	State   string                `json:"state"`
	Details []EntitySummaryDetail `json:"details,omitempty"`
}

// EntitySummaryDetail is a configured fact shown in the subject's tooltip.
// Labels and individual values translate separately; names and units stay exact.
type EntitySummaryDetail struct {
	Kind   string   `json:"kind,omitempty"` // semantic fact kind; labels remain presentation text
	Label  string   `json:"label"`
	Values []string `json:"values"`
}

// Banner is a page-level notice rendered by the shell at the navigation seam.
// It is reserved for state important enough to remain visible above the page
// heading; ordinary contextual notes belong in a callout beside their content.
type Banner struct {
	Variant string `json:"variant"` // "info" | "warning" | "danger"
	Title   string `json:"title"`
	Body    string `json:"body"`
}

// Notice is the outcome of the action this render answers — "saved", "could not
// read the form", "takes effect on reboot" — stated as intent, never composed as
// widgets. The shell renders it in its own flash slot, so a plugin's outcome and
// the shell's are indistinguishable. Standing page state belongs in Banner; a
// contextual note beside content belongs in a callout; prose belongs in raw.
type Notice struct {
	Level string `json:"level"` // the tone vocabulary: "success" | "warning" | "danger" | "info"
	Text  string `json:"text"`
}

// PageAction is a page's one primary doorway — "New rule", "Add forward" —
// rendered as a button hard right on the heading row. A page has at most one:
// it is the thing to do here, not a menu, so a second affordance belongs beside
// the content it acts on. Href is a route through the shell, like a link
// widget's, so a plugin points it at its own mount.
type PageAction struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Icon  string `json:"icon,omitempty"`
}

// PageTab is one subpage in a domain's top bar (the third navigation tier:
// sidebar → domain, top bar → kind of visit, in-page → position). Path is
// relative to the plugin's mount; the shell builds the href and marks the
// active tab from the request path, so a plugin cannot point the bar outside
// itself. Mode is the reading the tab belongs to — "basic", "advanced", or empty
// for both — filtered exactly as a manifest nav entry is (ADR-015).
type PageTab struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Mode  string `json:"mode,omitempty"`
}

// CommitOp is one declarative uci write a plugin asks the shell to perform on its
// behalf (ADR-007). A de-privileged plugin holds no write access and no
// session; it returns intents, and the shell executes them through rpcd with the
// operator's sid — but only for a config the plugin declared in its manifest acl,
// so a plugin cannot broker a write outside its declared surface.
type CommitOp struct {
	Config  string         `json:"config"`
	Section string         `json:"section"`
	Type    string         `json:"type,omitempty"`   // create this type; Section optionally supplies a unique name
	Delete  bool           `json:"delete,omitempty"` // remove Section outright; carries no Type and no Values
	Values  map[string]any `json:"values"`           // option → value: a string, a list of strings (uci list option), or null to clear the option
}

// ApplyAction is one tightly typed non-UCI operation a plugin asks the shell to
// perform after its staged UCI writes have been applied. The shell accepts only
// known names, verifies the plugin declared the matching rpcd scope, and sends
// structured arguments to the persistent privileged helper; it is never a
// command-execution escape hatch.
type ApplyAction struct {
	Name string            `json:"name"`
	Args map[string]string `json:"args"`
}

// DescribeChange is one coalesced uci change the shell asks a plugin to describe
// in plain words for the review drawer. It is the net effect of a target,
// flattened to named fields with a normalized Op so a plugin never reasons about a
// raw tuple's length. Op is a small closed vocabulary:
//
//	set            Option set to Value on Section
//	add-section    new Section, Option is its uci type
//	remove-option  Option cleared from Section
//	remove-section Section removed whole
//	list-add       Value added to the list Option on Section
//	list-del       Value removed from the list Option on Section
type DescribeChange struct {
	Config  string `json:"config"`
	Op      string `json:"op"`
	Section string `json:"section"`
	Option  string `json:"option,omitempty"`
	Value   string `json:"value,omitempty"`
}

// Description is one plain-language sentence a plugin returns for a run of its
// pending changes ("Turned off the rule “Block Telnet”"). Covers names the
// indices, into the change slice the shell sent, that this one sentence accounts
// for — one sentence may fold several raw changes (a renamed object writes two
// options). A change no description covers keeps its raw uci line; a plugin that
// describes nothing leaves every change raw.
type Description struct {
	Plain  string `json:"plain"`
	Covers []int  `json:"covers"`
}

// describeRequest is the JSON body the shell POSTs to a plugin's describe hook.
type describeRequest struct {
	Changes []DescribeChange `json:"changes"`
}

// describeResponse is the JSON a plugin's describe hook returns.
type describeResponse struct {
	Descriptions []Description `json:"descriptions"`
}

// Transport exchanges a request with a plugin and returns its schema envelope.
// It is the injected seam (ADR-003): the shell depends on this interface, tests
// supply a fake, production dials the unix socket. Implementations MUST bound
// the call in time so a hung plugin cannot wedge the shell.
type Transport interface {
	Fetch(ctx context.Context, socket string, req Request) (*Envelope, error)
	// Describe asks the plugin to render its pending changes in plain words. The
	// snapshot is the plugin's own configs (as Fetch injects them), so it can
	// resolve a section handle to the name a person would recognize. A plugin
	// with no describe hook, or one that describes only some changes, is not an
	// error: the shell falls back to the raw uci line for whatever comes back
	// undescribed. Only a transport failure is an error.
	Describe(ctx context.Context, socket string, changes []DescribeChange, snapshot UCI) ([]Description, error)
}

// SocketTransport speaks HTTP/1.1 to a plugin over its unix domain socket. It
// dials per call (no pooling) — simple and correct for admin-UI request rates;
// a pool is a later optimization behind this same interface.
type SocketTransport struct {
	timeout time.Duration
}

// NewSocketTransport returns a transport with the default per-call timeout.
func NewSocketTransport() *SocketTransport { return newSocketTransport(5 * time.Second) }

func newSocketTransport(timeout time.Duration) *SocketTransport {
	return &SocketTransport{timeout: timeout}
}

// Fetch dials socket, issues req as an HTTP request, and decodes the JSON schema
// envelope. A 200 or a 422 both carry a renderable envelope (ADR-006 §5); any
// other status, a dial failure, a timeout, or a non-JSON body is an error — the
// gateway turns that into a contained "plugin unavailable" state, never a 500.
func (t *SocketTransport) Fetch(ctx context.Context, socket string, req Request) (*Envelope, error) {
	client := t.dial(socket)

	httpReq, err := t.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("plugin: fetch %s: %w", socket, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnprocessableEntity:
		// Both carry a renderable schema envelope.
	default:
		return nil, fmt.Errorf("plugin: %s returned status %d", socket, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvelopeBytes))
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", socket, err)
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("plugin: decode envelope from %s: %w", socket, err)
	}
	env.Status = resp.StatusCode
	return &env, nil
}

// Describe POSTs the pending-change list to the plugin's describe hook and decodes
// its plain-language answer. It carries the same uci snapshot header Fetch sends,
// so the plugin can resolve a section handle to a recognizable name. A non-200, a
// dial failure, a timeout, or a non-JSON body is a transport error; the caller then
// falls back to the raw uci line for every change.
func (t *SocketTransport) Describe(ctx context.Context, socket string, changes []DescribeChange, snapshot UCI) ([]Description, error) {
	client := t.dial(socket)

	body, err := json.Marshal(describeRequest{Changes: changes})
	if err != nil {
		return nil, fmt.Errorf("plugin: encode describe request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://plugin/", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("plugin: build describe request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set(HeaderDescribe, "1")
	if len(snapshot) > 0 {
		snap, err := json.Marshal(snapshot)
		if err != nil {
			return nil, fmt.Errorf("plugin: encode describe snapshot: %w", err)
		}
		httpReq.Header.Set(HeaderUCI, base64.StdEncoding.EncodeToString(snap))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("plugin: describe %s: %w", socket, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin: describe %s returned status %d", socket, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvelopeBytes))
	if err != nil {
		return nil, fmt.Errorf("plugin: read describe %s: %w", socket, err)
	}
	var out describeResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("plugin: decode describe from %s: %w", socket, err)
	}
	return out.Descriptions, nil
}

// dial builds the per-call HTTP client that speaks to a plugin over its unix
// socket — one dial per call, no pooling (see SocketTransport).
func (t *SocketTransport) dial(socket string) *http.Client {
	return &http.Client{
		Timeout: t.timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
			DisableKeepAlives: true,
		},
	}
}

// buildRequest turns a plugin Request into an HTTP request. The host is a fixed
// placeholder — the unix dialer ignores it — but must be a valid URL.
func (t *SocketTransport) buildRequest(ctx context.Context, req Request) (*http.Request, error) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	u := "http://plugin" + normalizePath(req.Path)
	if len(req.Query) > 0 {
		u += "?" + encodeValues(req.Query)
	}

	var body io.Reader
	var contentType string
	if method != http.MethodGet && method != http.MethodHead && len(req.Form) > 0 {
		body = strings.NewReader(encodeValues(req.Form))
		contentType = "application/x-www-form-urlencoded"
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, fmt.Errorf("plugin: build request: %w", err)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Accept", "application/json")

	// The read snapshot rides a header, orthogonal to method/query/form: it reaches
	// a GET as readily as a POST, and never disturbs the plugin's form parsing.
	if len(req.UCI) > 0 {
		snapshot, err := json.Marshal(req.UCI)
		if err != nil {
			return nil, fmt.Errorf("plugin: encode uci snapshot: %w", err)
		}
		httpReq.Header.Set(HeaderUCI, base64.StdEncoding.EncodeToString(snapshot))
	}
	// The brokered helper reads ride their own header, so a plugin that wants only
	// config never has to look past the snapshot to find it.
	if len(req.Ubus) > 0 {
		reads, err := json.Marshal(req.Ubus)
		if err != nil {
			return nil, fmt.Errorf("plugin: encode brokered reads: %w", err)
		}
		httpReq.Header.Set(HeaderUbus, base64.StdEncoding.EncodeToString(reads))
	}
	return httpReq, nil
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

func encodeValues(v map[string][]string) string {
	return url.Values(v).Encode()
}
