// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"sort"
	"strings"
)

// Table is the Advanced-view listing: config sections as identical rows under
// fixed columns. Every column declares a kind, and that kind renders every cell
// in it the same way — rows cannot vary in shape, which is the point (the tuple
// is the row; names and comments are trailing metadata).
//
// Column kinds:
//
//	"text"     — plain ink text (the default)
//	"name"     — the row's identity (a zone, an interface): bold ink
//	"reference"— name-cell anatomy with regular-weight text (an interface cited elsewhere)
//	"mono"     — verbatim machine strings: addresses, ports, device names
//	"keyword"  — a closed vocabulary the device itself speaks (tcp, udp,
//	             icmpv6, A, CNAME): mono, because they are strings someone
//	             could type back into the config, at the secondary step
//	"comment"  — optional free text (e.g. a UCI name), muted, blank when absent
//	"num"      — right-aligned tabular figures (counters); muted
//	"rate"     — fixed-width, left-aligned live rate; tabular and non-wrapping
//	"runtime"  — compact process facts; sans, muted, tabular and non-wrapping
//	"action"   — a compact icon-only POST action
//	"reorder"  — a compact drag handle; interaction is shell-owned, and it draws
//	             only on a table that declares ReorderConfig
//	"toggle"   — an on/off switch (a section's enabled state)
//	"check"    — a yes/no fact: a checkmark for yes, nothing for no (cell On)
//	"endpoint" — one or more traffic endpoints, each a type icon + label
//	"status"   — a state as a bare dot + word (a link's up/down): the dot takes
//	             the cell's variant colour, the word reads calm; no pill frame
//	"link"     — a right-aligned action link to the cell's Href (e.g. "Details")
//	"pill"     — an enum value as a status pill (accept/reject/drop, NAT); the
//	             cell's variant uses the badge vocabulary, and an empty cell
//	             renders a faint dash — pills stay meaningful because most
//	             cells in such a column are empty or quiet
type Table struct {
	Style       string        `json:"style,omitempty"` // "" / "flat" (default) — bare hairline rows, non-clickable (values stay selectable); "more" opens from a trailing link, never the whole row | "card" — the framed box
	Columns     []TableColumn `json:"columns"`
	Rows        []TableRow    `json:"rows"`
	Seam        *TableSeam    `json:"seam,omitempty"`
	DrawerLabel string        `json:"drawer_label,omitempty"` // trailing drawer action; defaults to "Details"
	DrawerIcon  string        `json:"drawer_icon,omitempty"`

	// The flat style's optional header band — a top row aligned to the table's
	// own edges: the listing's name, a quiet detail beside it (a count/summary),
	// and an optional link hard-right. All three are optional; an absent Title
	// draws no band. Ignored by the other styles.
	Title  string       `json:"title,omitempty"`
	Detail string       `json:"detail,omitempty"`
	Action *TableAction `json:"action,omitempty"`

	// Dense pulls the row's edge inset in for a listing carrying many columns:
	// the same rows at the same height, with less of the measure spent holding
	// values off the table's edge. It is a judgement about horizontal room, so
	// the listing that knows how many columns it has is the one that says it.
	Dense bool `json:"dense,omitempty"`

	// Align "top" pins every cell to the row's top instead of centring it — so a
	// cell that stacks two values (an IPv4 over an IPv6) keeps its first line on
	// the shared top line with the single-value cells, the second dangling below.
	Align string `json:"align,omitempty"`

	// Legend explains the marks the rows carry, under the last row: a listing
	// that states something with a dot owes the reader the dot's meaning once,
	// where the dots are, rather than in a tooltip they have to go looking for.
	// Note rides hard right of the legend — the one caveat about what the
	// listing includes ("offline devices stay listed until their lease
	// expires"). Either may stand without the other.
	Legend []TableLegend `json:"legend,omitempty"`
	Note   string        `json:"note,omitempty"`

	// EmptyText is what a listing with nothing in it says. Column headings
	// describe data; over no data they are chrome, so an empty table drops them
	// and renders one quiet full-width row carrying this sentence — the treatment
	// for a listing that is one section among several. A listing that IS the page
	// deserves the full empty widget instead. Left blank it reads "Nothing here
	// yet" in the operator's language.
	EmptyText string `json:"empty_text,omitempty"`

	// Stream turns the listing live: its rows arrive after the render, over a
	// shell-owned event stream, newest on top. The plugin declares only which
	// stream it wants; transport, the ring, pausing, and the row collapse are
	// the shell's (ADR-005 §7 — declare the intent, the shell owns the
	// behaviour). A table that streams renders its columns even with nothing in
	// it yet: the heads describe what is about to arrive, not chrome over
	// nothing.
	Stream *TableStream `json:"stream,omitempty"`

	// ReorderConfig names the uci config whose sections these rows are, and it is
	// what makes a leading "reorder" column real: the shell posts the dragged id
	// sequence back and stages a `uci order` on that config. A table without it
	// draws no handle — a grip the device would not remember is a lie.
	ReorderConfig string `json:"reorder_config,omitempty"`

	// ReorderLabel names the order in the operator's words in the stage's
	// review ("Rule order"); an empty label falls back to the shell's generic
	// "Order". The listing knows what its rows are; the shell does not.
	ReorderLabel string `json:"reorder_label,omitempty"`

	// AddLabel and AddHref render a quiet add affordance after the last row —
	// the listing's next entry, exactly where it will land. A grouped table
	// prefers the per-lane adds on TableGroup. The href passes the shell's
	// URL policy like every plugin link.
	AddLabel string `json:"add_label,omitempty"`
	AddHref  string `json:"add_href,omitempty"`
}

// The form fields the shell's reorder interaction posts, and that the gateway
// reads to stage the new section order. They live here because this package
// renders the affordance; the gateway imports these same constants, so the two
// never drift. verso.js builds the body from the table's own data attributes.
const (
	ReorderConfigField = "_uci_order_config"
	ReorderIDField     = "_uci_order"
)

// TableStream names the live source a table's rows arrive from and how many of
// them the browser keeps. Source is a name from the shell's closed set, never a
// URL: a plugin cannot point the shell at an arbitrary endpoint, exactly as a
// manifest cannot name its way to an arbitrary brokered read. Ring is how many
// rows the client keeps before the oldest fall off the bottom.
type TableStream struct {
	Source string `json:"source"`
	Ring   int    `json:"ring,omitempty"`
}

// The ring a live listing keeps: the default when a table names none, and the
// bounds a declared one is held to. Too few is a listing that forgets what just
// happened; too many is a page the browser has to keep re-laying out.
const (
	StreamRingDefault = 200
	StreamRingMin     = 20
	StreamRingMax     = 500
)

// StreamSourceFirewallLog is the firewall's verdict stream: the fw4 log lines
// the kernel writes, parsed and resolved by the shell.
const StreamSourceFirewallLog = "firewall-log"

// streamSources is the closed set of live sources the shell knows how to serve.
// Declaring a stream is not enough — the shell must also know the source, so a
// plugin cannot name its way to a feed nobody wrote (the same bound the
// brokered ubus reads keep). An unknown source renders a static listing.
var streamSources = map[string]bool{
	StreamSourceFirewallLog: true,
}

// StreamSourceKnown reports whether source can supply widget table rows. Core
// pages may have additional streams with their own payload and renderer.
func StreamSourceKnown(source string) bool { return streamSources[source] }

// ring is the effective row budget: the default when unset, clamped to the
// bounds otherwise.
func (s *TableStream) ring() int {
	switch {
	case s.Ring <= 0:
		return StreamRingDefault
	case s.Ring < StreamRingMin:
		return StreamRingMin
	case s.Ring > StreamRingMax:
		return StreamRingMax
	}
	return s.Ring
}

// streaming reports whether this listing is live — it declared a stream and the
// shell knows that source.
func (t *Table) streaming() bool {
	return t.Stream != nil && StreamSourceKnown(t.Stream.Source)
}

// streamRing is the live listing's row budget; zero when it does not stream.
func (t *Table) streamRing() int {
	if !t.streaming() {
		return 0
	}
	return t.Stream.ring()
}

// TableAction is the flat header band's trailing link (e.g. "View all").
type TableAction struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	// Icon is the glyph the act wears, by Lucide name. An act that makes a new
	// object leaves it unset and takes the plus every "add" wears, so the one
	// affordance a listing is most likely to carry stays identical everywhere;
	// an act that does something else names its own.
	Icon string `json:"icon,omitempty"`
	// Style is the act's weight: "" is the bar's one forward act, filled with
	// the action colour because it is the thing the listing is for; "quiet"
	// draws it as the secondary button beside the bar's other controls — for an
	// act that takes something away from the page (a download) rather than
	// leading somewhere.
	Style string `json:"style,omitempty"`
}

// Quiet reports whether this act draws as the secondary button rather than as
// the bar's filled forward act.
func (a *TableAction) Quiet() bool { return a.Style == "quiet" }

// TableSeam is a collapsed block of extra rows inside the same card — the
// "stock rules that ship with the install" pattern: present and honest, but
// folded so the user's own sections carry the page. It expands in place under
// the same columns.
type TableSeam struct {
	Summary string     `json:"summary"`
	Rows    []TableRow `json:"rows"`
}

// TableColumn is one column: its header label and the kind every cell in it
// renders as.
type TableColumn struct {
	Label string `json:"label,omitempty"`
	Kind  string `json:"kind,omitempty"` // see Table; "" means "text"
	// Fit squeezes the column to its content's width instead of sharing the
	// table's slack, which collects in the growing columns — how a group of
	// related fact columns (a version pair and the arrow between them) huddles
	// at one edge instead of drifting apart. Pair it with kinds that do not
	// wrap; the table realizes it as a colgroup width, not text alignment.
	Fit bool `json:"fit,omitempty"`
	// Width fixes the column at a measure the listing decides rather than at
	// whatever this page's data happens to need — so a column of addresses keeps
	// its place when one device has a shorter one, and two boards' rosters line
	// up. A CSS length; the growing columns share what is left.
	Width string `json:"width,omitempty"`
}

// TableRow is one config section. ID is its stable handle (e.g. the UCI section
// name); it does not render. A row with a Drawer is an object you can open:
// clicking it slides in the drawer (the row gets a trailing chevron and the
// pointer; controls inside the row keep their own meaning).
type TableRow struct {
	Expanded []Widget `json:"expanded,omitempty"`
	Depth    int      `json:"depth,omitempty"`

	ID    string      `json:"id,omitempty"`
	Key   string      `json:"key,omitempty"`   // optional stable live-update hook; not displayed
	Group *TableGroup `json:"group,omitempty"` // optional evaluation-lane header before this row
	// Muted reads the whole row at the secondary step — the treatment for a row
	// whose subject is not here right now (a device known but absent). It is a
	// statement about the subject, not about the row's importance: the values
	// are still exact, and still copyable.
	Muted bool `json:"muted,omitempty"`
	// Tags and Facet are what an ActionBar narrows this row by, and they exist
	// only because narrowing a listing you are already looking at should not be
	// a round trip: Tags are the flags a tab matches ("online", "reserved"), and
	// Facet the key/value a select matches ("network" → "guest"). A listing with
	// no bar above it sets neither.
	Tags  []string          `json:"tags,omitempty"`
	Facet map[string]string `json:"facet,omitempty"`
	Cells []TableCell       `json:"cells"`
	// Entity names the subject this row is about, for a row whose panel is the
	// shell's rather than this listing's. The row then carries no drawer contents
	// at all: the panel is fetched when it opens, once, instead of every row of
	// a listing shipping a panel nobody asked for. Drawer stays the way a
	// single-owner row carries its own detail; a row sets one or the other.
	Entity *EntityRef `json:"entity,omitempty"`
	Drawer *RowDrawer `json:"drawer,omitempty"`
	// Panel is where this row's panel is fetched from — the row ships an empty
	// frame and asks for its contents the first time someone opens it. A listing
	// of forty rows then carries one frame apiece instead of forty rendered
	// panels for the at-most-one anybody will look at, and the frame, being
	// mounted once, slides in once: switching what is inside it afterwards
	// leaves the panel where it is.
	//
	// A cell whose link points at this same address opens the panel in place
	// rather than following the link. With no script the link is simply
	// followed, and the address answers with the page carrying the panel open —
	// the same screen, reached the slow way.
	Panel string `json:"panel,omitempty"`
}

// EntityRef names one subject: what kind of thing it is, and its own identity in
// that kind — a MAC for a device, a section name for an interface. The shell
// turns the pair into the panel's address; nothing here knows what the panel
// will contain, which is the point.
type EntityRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// TableGroup introduces a run of rows that belong together — one firewall
// evaluation lane, one network's devices. Label is human-facing; Chain is the
// machine-verbatim secondary beside it (firewall4's chain name, a network's
// CIDR), set in mono so an expert reads the exact string.
type TableGroup struct {
	// Key is the group's own handle — the config's name for it ("lan",
	// "input_wan") — which a dragged listing posts its order by. It is not
	// drawn: the band says what the group is called, not what the device calls
	// it.
	Key string `json:"key,omitempty"`
	// Label is what the group is called; for a traffic lane, where the traffic
	// comes from. To is where it goes, drawn after an arrow ("WAN → Router");
	// a group that is not a path sets none.
	Label string `json:"label"`
	To    string `json:"to,omitempty"`
	// Chain is the machine-verbatim string the group resolves to — a network's
	// CIDR — set in mono beside the label so an expert reads the exact string.
	Chain string `json:"chain,omitempty"`
	// Tally is what this group amounts to, already worded ("3 online · 1
	// offline", "6 rules"), after the label and a faint dot.
	Tally string `json:"tally,omitempty"`
	// AddLabel and AddHref put the lane's own add control in its band, hard
	// right — the head of the run it adds to — as a glyph that says these
	// words on hover; the href may pre-seed the editor with the lane's own
	// path. A group without them renders a band with nothing on its right.
	AddLabel string `json:"add_label,omitempty"`
	AddHref  string `json:"add_href,omitempty"`
	// AddPanel declares that address a panel rather than a page: the lane's add
	// opens it where the lane is, seeded with the lane's own path, and the
	// listing stays put behind it. Without it the add is an ordinary link and
	// leads away, which is right for a lane whose new object needs a page.
	AddPanel bool `json:"add_panel,omitempty"`
}

// RowDrawer is the row's object, opened beside the listing: a right slide-in
// panel holding the object's facts, a callout naming what depends on it, the
// form that edits it, and action buttons guarded by confirm where an act
// deserves a pause. An object lives in its drawer (ADR-005 §8): making one and
// editing one are the same panel, and its Save stages. Same shell behaviour as
// the drawer widget (ADR-005 §7). Size widens the panel ("" reading width | "wide") for
// detail views that carry tables beside prose. Open renders the panel already
// open, which is how a link from elsewhere arrives with the row's panel
// already in front of the operator.
type RowDrawer struct {
	Title string `json:"title"` // short action label, without the row's name
	// Verbatim declares the title an identity (the interface, the rule, the
	// device the drawer opens on) — data the localization walk leaves exactly
	// as authored. A prose title stays undeclared and translates.
	Verbatim bool `json:"verbatim,omitempty"`
	// Deprecated: these former header decorations are accepted for wire
	// compatibility but are not rendered. Details belong in Children.
	Sub     string   `json:"sub,omitempty"`
	Chain   string   `json:"chain,omitempty"`
	Tag     string   `json:"tag,omitempty"`
	Verdict *Badge   `json:"verdict,omitempty"`
	Lede    []string `json:"lede,omitempty"`
	// Closed is the address this page has when no panel is open. A panel the
	// address opened is a place, so closing it has to leave that place — the
	// shell rewrites the address to this one, and a reload then shows the
	// listing rather than reopening what was just dismissed. Empty leaves the
	// address alone, which is right for a panel no address opened.
	Closed string `json:"closed,omitempty"`
	// Tabs cut the panel's body into the questions its object answers — what a
	// rule matches, what it then does, when it applies. Each carries the state
	// the object is in under that heading, so the strip answers before a tab is
	// chosen. A panel with one thing to say sends none and shows its body whole.
	Tabs []DrawerTab `json:"tabs,omitempty"`
	Size string      `json:"size,omitempty"`
	// Deprecated: drawer headings are always visible.
	HideTitle bool     `json:"hide_title,omitempty"`
	Open      bool     `json:"open,omitempty"`
	Children  []Widget `json:"children"`
}

// DrawerTab is one heading in a panel's strip: what it is called, where the
// object stands under it, and the address that opens it. State is the chip
// beside the label — "5 conditions", "accept" — which takes the tab's own
// weight, denim on the one in force and quiet on the rest.
type DrawerTab struct {
	Label  string `json:"label"`
	State  string `json:"state,omitempty"`
	Href   string `json:"href,omitempty"`
	Active bool   `json:"active,omitempty"`
}

// UnmarshalJSON decodes the panel's children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing. It lives on RowDrawer
// rather than inside the row's decoder because a panel is not only a row's: the
// bar's own act opens one too, and a second copy of this list would be a second
// place for a field to go missing from.
func (d *RowDrawer) UnmarshalJSON(data []byte) error {
	// Every field a plugin may send is named here. A field left out of this
	// shape is silently dropped on the way in — the panel renders without it and
	// nothing fails — so the list has to stay complete as RowDrawer grows.
	var raw struct {
		Title     string            `json:"title"`
		Verbatim  bool              `json:"verbatim"`
		Sub       string            `json:"sub"`
		Chain     string            `json:"chain"`
		Tag       string            `json:"tag"`
		Verdict   *Badge            `json:"verdict"`
		Lede      []string          `json:"lede"`
		Closed    string            `json:"closed"`
		Tabs      []DrawerTab       `json:"tabs"`
		Size      string            `json:"size"`
		HideTitle bool              `json:"hide_title"`
		Open      bool              `json:"open"`
		Children  []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	children, err := decodeChildren(raw.Children, "row drawer child")
	if err != nil {
		return err
	}
	*d = RowDrawer{
		Title: raw.Title, Verbatim: raw.Verbatim,
		Sub: raw.Sub, Chain: raw.Chain, Tag: raw.Tag,
		Verdict: raw.Verdict, Lede: raw.Lede, Closed: raw.Closed, Tabs: raw.Tabs,
		Size: raw.Size, HideTitle: raw.HideTitle, Open: raw.Open,
		Children: children,
	}
	return nil
}

// UnmarshalJSON decodes the row, handing its panel to the panel's own decoder.
func (tr *TableRow) UnmarshalJSON(data []byte) error {
	// Every field a plugin may send is named here. A field left out of this
	// shape is silently dropped on the way in — the row renders without it and
	// nothing fails — so the list has to stay complete as TableRow grows.
	var raw struct {
		Expanded []json.RawMessage `json:"expanded"`
		Depth    int               `json:"depth"`
		ID       string            `json:"id"`
		Key      string            `json:"key"`
		Group    *TableGroup       `json:"group"`
		Muted    bool              `json:"muted"`
		Tags     []string          `json:"tags"`
		Facet    map[string]string `json:"facet"`
		Entity   *EntityRef        `json:"entity"`
		Panel    string            `json:"panel"`
		Cells    []TableCell       `json:"cells"`
		Drawer   *RowDrawer        `json:"drawer"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var err error
	tr.Expanded, err = decodeChildren(raw.Expanded, "table expanded")
	if err != nil {
		return err
	}
	tr.Depth = raw.Depth
	tr.ID = raw.ID
	tr.Key = raw.Key
	tr.Group = raw.Group
	tr.Muted = raw.Muted
	tr.Tags = raw.Tags
	tr.Facet = raw.Facet
	tr.Entity = raw.Entity
	tr.Panel = raw.Panel
	tr.Cells = raw.Cells
	tr.Drawer = raw.Drawer
	return nil
}

// TableCell carries the value for one cell; which field applies is decided by
// the column's kind (Text for text/name/mono/keyword/comment/num, Text+Variant
// for pill, On/Name for toggle, Endpoints for endpoint).
type TableCell struct {
	Text         string          `json:"text,omitempty"`
	Variant      string          `json:"variant,omitempty"`       // pill cells: the badge vocabulary ("success" | "warning" | "danger" | "info" | "neutral")
	Dot          bool            `json:"dot,omitempty"`           // pill cells: leading status dot — the same cue the badge carries elsewhere
	Icon         string          `json:"icon,omitempty"`          // pill cells: a leading Lucide icon on the badge (e.g. a firewall verdict's check/ban)
	Copy         bool            `json:"copy,omitempty"`          // mono cells: offer the inline copy button beside the value
	Emphasis     bool            `json:"emphasis,omitempty"`      // mono cells: promote an important value one size and weight step
	Chip         string          `json:"chip,omitempty"`          // name cells: a small category chip inline after the name (e.g. its zone)
	ChipIcon     string          `json:"chip_icon,omitempty"`     // name cells: optional Lucide icon inside the category chip
	LeadIcon     string          `json:"lead_icon,omitempty"`     // name cells: a device-type Lucide glyph before the name, plain slate ink (not a badge)
	Key          string          `json:"key,omitempty"`           // optional stable live-update hook; not displayed
	Muted        bool            `json:"muted,omitempty"`         // text/mono and empty pill cells: render the value as secondary ink
	Sub          string          `json:"sub,omitempty"`           // addr cells: a second line under the primary (e.g. the IPv6 under the IPv4), muted and copyable
	Tag          string          `json:"tag,omitempty"`           // name/status cells: a small coloured label after the value (e.g. "new", "WAN")
	TagVariant   string          `json:"tag_variant,omitempty"`   // the tag's palette (badge vocabulary): "" neutral | "info" | "warning" | "success" | "danger"
	TagIcon      string          `json:"tag_icon,omitempty"`      // name/status cells: a Lucide icon on the tag — promotes it to a ring-chip (e.g. WAN's globe), kept its colour to stand out
	Href         string          `json:"href,omitempty"`          // link cells: the destination of the row's action link
	Button       string          `json:"button,omitempty"`        // an in-cell row action or drawer trigger; replaces the auto trailing "Details" link for that row
	Disabled     bool            `json:"disabled,omitempty"`      // the cell's button is unavailable: rendered natively disabled and muted
	Action       string          `json:"action,omitempty"`        // _action value posted by a direct row action (defaults to the row id)
	ConfirmTitle string          `json:"confirm_title,omitempty"` // direct-action confirmation heading (default "Are you sure?")
	Confirm      string          `json:"confirm,omitempty"`       // direct-action consequence copy; enables the confirmation dialog
	On           bool            `json:"on,omitempty"`
	Name         string          `json:"name,omitempty"` // form name the toggle posts under
	Endpoints    []TableEndpoint `json:"endpoints,omitempty"`
	Chips        []TableChip     `json:"chips,omitempty"`   // entity/name/reference cells: one or more icon+label reference chips
	Actions      []TableRowAct   `json:"actions,omitempty"` // actions cells: the row's own acts, as quiet icon buttons
	// Opens makes this cell's own value the door to the row's drawer, instead of
	// a trailing "Details" link in a column of its own. The subject of the row is
	// what a person reaches for, so the subject is what opens it — and the row
	// keeps every other value selectable, which a whole-row click would not.
	Opens bool `json:"opens,omitempty"`
}

// TableRowAct is one act on a row, drawn as a bare icon button at the row's
// trailing edge: a glyph, and the sentence a pointer rests on. The acts sit in
// one fixed-width column so every row's controls line up, and they stay quiet —
// the row's subject is what the eye should find, not its controls.
//
// Href is where the act leads. An act with no Href is drawn and does nothing:
// the shape is decided but the behaviour behind it is not built, and a button
// that is present and inert is honest where one that is absent would hide that
// the act exists at all.
type TableRowAct struct {
	Icon  string `json:"icon"`
	Title string `json:"title"`
	Href  string `json:"href,omitempty"`
	// Name and Value make the act a submission rather than a destination: it
	// posts that one pair to the page it sits on, which is how a row's own state
	// is flipped from the row. An act sets a destination or a submission, never
	// both — one glyph cannot mean two things.
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
	// Opens makes the act open the row's own drawer instead of leading anywhere.
	// An act on a row's subject usually belongs in the panel that already holds
	// that subject, not on a page of its own — the icon is the shortcut, the
	// panel is the place.
	Opens bool `json:"opens,omitempty"`
	// Tab is the panel tab this act opens on, where the panel has several and
	// this icon is about one of them. Without it the panel opens on the tab its
	// design orders first.
	Tab string `json:"tab,omitempty"`
}

// TableChip is one entity-reference chip: a Lucide type icon (naming the kind —
// an interface, a zone, a physical port) plus the entity's label (naming the
// one). It is the shared treatment for referencing a network entity inside
// another entity's row, so the three never blur together (a shield is always a
// zone, a network glyph always an interface, a port glyph always a port).
type TableChip struct {
	Title string `json:"title,omitempty"` // localized explanation, including service status
	Icon  string `json:"icon"`
	Label string `json:"label"`
	// Tone is "" for the neutral chip every cited entity wears, or "accent"
	// where what the chip states is a decision someone made rather than one more
	// fact about the row.
	Tone string `json:"tone,omitempty"`
}

// TableLegend is one entry in a listing's legend: the same mark a row wears,
// and the plain words for what wearing it means. Variant is the status cell's
// own vocabulary, so the mark here and the mark in the row are one treatment —
// an empty Variant draws the empty ring, exactly as a row does.
type TableLegend struct {
	Variant string `json:"variant,omitempty"`
	Label   string `json:"label"`
}

// TableEndpoint is one traffic endpoint in an endpoint cell. The kind picks the
// type icon and treatment: a zone name reads sans, a device address reads mono,
// "router" (this device) carries the accent, "any" is muted.
type TableEndpoint struct {
	Kind  string `json:"kind"` // "zone" | "device" | "router" | "any"
	Label string `json:"label"`
}

func (*Table) isWidget() {}

func (t *Table) children() []Widget {
	var out []Widget
	collect := func(rows []TableRow) {
		for i := range rows {
			out = append(out, rows[i].Expanded...)
			if rows[i].Drawer != nil {
				out = append(out, rows[i].Drawer.Children...)
			}
		}
	}
	collect(t.Rows)
	if t.Seam != nil {
		collect(t.Seam.Rows)
	}
	return out
}

func (t *Table) prune(keep func(Widget) bool) {
	rows := func(rows []TableRow) {
		for i := range rows {
			rows[i].Expanded = pruneList(rows[i].Expanded, keep)
			if rows[i].Drawer != nil {
				rows[i].Drawer.Children = pruneList(rows[i].Drawer.Children, keep)
			}
		}
	}
	rows(t.Rows)
	if t.Seam != nil {
		rows(t.Seam.Rows)
	}
}

// endpointIcons maps an endpoint kind to its registered icon. Unknown kinds fall
// back to the zone glyph — a wrong icon beats a missing one in a listing.
var endpointIcons = map[string]string{
	"zone":   "zone",
	"device": "device",
	"router": "router",
	"any":    "globe",
}

// tableView is the render model: cells zipped with their column's kind, so the
// template stays a flat range with no positional arithmetic. HasDetail is
// table-wide (main rows and seam alike): it is true when some drawer row has no
// in-cell button of its own, so the table carries a trailing "Details" column
// and every row pads it to keep the grid aligned.
type tableView struct {
	Live bool
	Card bool
	// Style is the listing's shape where it is not a grid. "console" is the one
	// such shape: a live log drawn as a terminal writes one — no grid, no
	// hairlines, filling the frame and running to its edges.
	Style     string
	Dense     bool
	AlignTop  bool
	Title     string
	Detail    string
	Action    *TableAction
	HasLabels bool // any column carries a header label; a labelless table draws no <thead>
	HasFit    bool // any column squeezes to content; the table draws a colgroup
	Columns   []TableColumn
	HasDetail bool
	Empty     bool // nothing to list: no head, no rows, one quiet sentence
	EmptyText string
	Legend    []TableLegend
	Note      string
	// Stream marks the live listing: the table carries the source and ring the
	// client reads, and an empty one keeps its column heads and marks its quiet
	// sentence row so the first arriving event can replace it.
	Stream        bool
	StreamSource  string
	StreamRing    int
	Reorderable   bool
	ReorderConfig string
	ReorderLabel  string
	// ReorderIDs is every draggable row's id in the sequence the table renders
	// them: the order form's baseline, and what a drag rewrites.
	ReorderIDs  []string
	CSRFToken   string
	ColumnSpan  int
	Rows        []tableRowView
	SeamSummary string
	SeamRows    []tableRowView
}

// tableFacetView is one key/value the client narrows by, flattened for the
// template.
type tableFacetView struct{ Key, Value string }

// sortedKeys is a map's keys in a fixed order, so the same row renders the same
// attributes every time — a rendered page that differs run to run is a page no
// test can pin.
func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type tableRowView struct {
	Expanded         []template.HTML
	TreeNodeX        int
	TreeNodeCenter   int
	TreePath         string
	TreeContinuation string
	TreeRoot         bool

	ID    string
	Key   string
	Group *TableGroup
	// AddLabel/AddHref mark the synthetic lane-tail add row (see TableGroup);
	// a view row carrying them renders the affordance and nothing else.
	AddLabel     string
	AddHref      string
	ColumnSpan   int
	Reorder      bool
	ReorderGroup string
	// Entity is the panel's address for a row whose detail the shell owns; the
	// drawer fetches it on open instead of carrying a body. PanelURL is the same
	// idea for a panel the plugin renders: the row ships an empty frame and
	// fetches its contents from this address when it opens.
	Entity   string
	PanelURL string
	// Tags is the row's flags as one space-separated attribute, and Facets its
	// key/value pairs — both flattened here so the template writes plain data
	// attributes and the client reads them without parsing anything.
	Tags        string
	Facets      []tableFacetView
	Cells       []tableCellView
	HasDetail   bool // table-wide flag, copied so the rows sub-template needs no second argument
	Drawer      bool // this row has a drawer (hosts the modal scope)
	Inline      bool // the drawer opens from an in-cell button, so this row shows no trailing "Details"
	Dense       bool // the listing's geometry: cells carry no inset of their own, only the edges do
	Seam        bool // this row belongs to the collapsible continuation block
	Open        bool // the row's drawer renders already open
	DrawerLabel string
	DrawerIcon  string
	Panel       drawerPanelView // the row's slide-in detail panel (shared with the drawer widget)
}

type tableCellView struct {
	Expandable bool
	Indent     string

	Kind      string
	Primary   bool // the first column — the row's identity, set one step larger
	Draggable bool // reorder cells: the table persists an order, so draw the handle
	RowID     string
	CSRFToken string
	Drawer    bool
	// Panel is the row's panel address, so a cell can tell whether its own link
	// points there — a link to the panel opens it in place; any other link is an
	// ordinary link and still leaves the page.
	Panel     string
	ConfirmID string
	Dense     bool // the listing's geometry: no inset of the cell's own, the columns' widths space them
	TableCell
	Endpoints []tableEndpointView
	Pill      *Badge // pill cells render through the badge component
}

// OpensPanel reports whether this cell's link is the row's panel — the case
// where following it would fetch the very page we are already on, only to show
// what a swap can show without leaving.
func (c tableCellView) OpensPanel() bool { return c.Panel != "" && c.Href == c.Panel }

type tableEndpointView struct {
	TableEndpoint
	Icon string
}

// EntityPath is where one subject's panel is fetched from. The shell owns the
// route; the widget package only needs to be able to name it, because a row that
// points at a panel is still just a row.
func EntityPath(kind, id string) string {
	return "/entity/" + kind + "/" + url.PathEscape(id)
}

// rowOpensItself reports whether the row already carries a way in of its own: a
// cell that opens the panel, a button, a link on its subject, or the acts at its
// trailing edge. Any of them means the listing has said how this row is entered,
// and the shell's own "Details" affordance would be a second door beside a door
// — with the added cost of a whole extra column on a listing that had none.
func rowOpensItself(row TableRow) bool {
	for _, c := range row.Cells {
		if c.Button != "" || c.Opens || c.Href != "" || len(c.Actions) > 0 {
			return true
		}
	}
	return false
}

// hasDetail reports whether the table needs a trailing "Details" column: true
// when some drawer row carries no way of its own to be opened.
func (t *Table) hasDetail() bool {
	need := func(rows []TableRow) bool {
		for _, row := range rows {
			if row.Drawer != nil && !rowOpensItself(row) {
				return true
			}
		}
		return false
	}
	return need(t.Rows) || (t.Seam != nil && need(t.Seam.Rows))
}

// empty reports whether the listing has anything to show — its own rows or
// folded ones. Both absent is the state the quiet empty row serves.
func (t *Table) empty() bool {
	return len(t.Rows) == 0 && (t.Seam == nil || len(t.Seam.Rows) == 0)
}

// reorderable reports whether these rows really drag. The leading "reorder"
// column asks for the handle and ReorderConfig names the config the new order is
// staged against; only both together make the interaction persist, so only both
// together draw it. A listing with no rows has no order to state, so it carries
// neither handles nor the order form.
func (t *Table) reorderable() bool {
	return !t.empty() && t.ReorderConfig != "" && len(t.Columns) > 0 && t.Columns[0].Kind == "reorder"
}

// consoleStyle reports the console shape, and only for a listing that is really
// live. The shape is a log's: a table asking for it without a stream behind it
// would render an empty terminal that never fills, so it stays a grid.
func (t *Table) consoleStyle() string {
	if t.Style == "interfaces" {
		return "interfaces"
	}
	if t.Style == "console" && t.streaming() {
		return "console"
	}
	return ""
}

func (t *Table) view(r *Renderer, csrf string) (tableView, error) {
	empty := t.empty()
	stream := t.streaming()
	v := tableView{
		Live: t.Style == "live", Card: t.Style == "card", Style: t.consoleStyle(), Dense: t.Dense, AlignTop: t.Align == "top",
		Title: t.Title, Detail: t.Detail, Action: t.Action,
		// A live listing keeps its heads while it waits: they name what is
		// about to arrive, and the first event must not shift the layout.
		HasLabels: hasColumnLabels(t.Columns) && (!empty || stream),
		HasFit:    hasFitColumns(t.Columns),
		Columns:   t.Columns, HasDetail: t.hasDetail(),
		Empty: empty, EmptyText: t.EmptyText,
	}
	// A legend explains the marks the rows carry, so an empty listing carrying
	// no rows carries no legend either.
	if !empty {
		v.Legend, v.Note = t.Legend, t.Note
	}
	if stream {
		v.Stream, v.StreamSource, v.StreamRing = true, t.Stream.Source, t.Stream.ring()
	}
	if empty && v.EmptyText == "" {
		if stream {
			v.EmptyText = r.tr("Waiting for the first event…")
		} else {
			v.EmptyText = r.tr("Nothing here yet")
		}
	}
	// A plugin's hrefs land in shell chrome; the link widget's URL policy
	// applies here the same as there, and a reject reads "#", never ZgotmplZ.
	if v.Action != nil {
		action := *v.Action
		action.Href = SafeHref(action.Href)
		v.Action = &action
	}
	v.Reorderable = t.reorderable()
	v.ReorderConfig = t.ReorderConfig
	v.ReorderLabel = t.ReorderLabel
	v.CSRFToken = csrf
	v.ColumnSpan = len(v.Columns)
	if v.HasDetail {
		v.ColumnSpan++
	}
	// A spanning row (a group head, the empty sentence) needs a column to span;
	// a listing that declared none still has one cell's worth of width.
	if v.ColumnSpan < 1 {
		v.ColumnSpan = 1
	}
	var err error
	if v.Rows, err = t.rowViews(r, csrf, t.Rows, v.HasDetail); err != nil {
		return v, err
	}
	// The table-level add sits after the last row — before the seam, since a
	// fold of stock rows is history and the next entry belongs to the person.
	if !empty && t.AddLabel != "" && t.AddHref != "" {
		v.Rows = append(v.Rows, tableRowView{AddLabel: t.AddLabel, AddHref: SafeHref(t.AddHref), ColumnSpan: v.ColumnSpan})
	}
	if t.Seam != nil {
		v.SeamSummary = t.Seam.Summary
		if v.SeamRows, err = t.rowViews(r, csrf, t.Seam.Rows, v.HasDetail); err != nil {
			return v, err
		}
		for i := range v.SeamRows {
			v.SeamRows[i].Seam = true
		}
	}
	if v.Reorderable {
		v.ReorderIDs = reorderIDs(v.Rows, v.SeamRows)
	}
	return v, nil
}

// reorderIDs collects the draggable rows' ids in render order — the sequence the
// order form carries, and the one the browser rewrites on a drop. A seam's rows
// drag on the same terms as the rest, so they belong in the same sequence.
func reorderIDs(runs ...[]tableRowView) []string {
	var ids []string
	for _, rows := range runs {
		for _, row := range rows {
			if row.ID != "" {
				ids = append(ids, row.ID)
			}
		}
	}
	return ids
}

// hasColumnLabels reports whether any column carries a header label. A table
// with none (the connected-devices reference) draws no <thead>.
func hasColumnLabels(cols []TableColumn) bool {
	for _, c := range cols {
		if c.Label != "" {
			return true
		}
	}
	return false
}

// hasFitColumns reports whether any column squeezes to its content, which is
// what makes the table draw a colgroup to carry the widths.
func hasFitColumns(cols []TableColumn) bool {
	for _, c := range cols {
		if c.Fit || c.Width != "" {
			return true
		}
	}
	return false
}

func (t *Table) rowViews(r *Renderer, csrf string, rows []TableRow, hasDetail bool) ([]tableRowView, error) {
	out := make([]tableRowView, 0, len(rows))
	drawerLabel := t.DrawerLabel
	if drawerLabel == "" {
		drawerLabel = r.tr("Details")
	}
	primary := 0
	if len(t.Columns) > 0 && t.Columns[0].Kind == "reorder" {
		primary = 1
	}
	reorderable := t.reorderable()
	reorderGroup := ""
	columnSpan := len(t.Columns)
	if hasDetail {
		columnSpan++
	}
	// band is the group header of the run being rendered. A lane that declares
	// an add affordance carries it in that header, so the control sits at the
	// head of the run it adds to; the href passes the same URL policy every
	// plugin link does.
	var band *TableGroup
	for rowIndex, row := range rows {
		if row.Group != nil {
			lane := *row.Group
			lane.AddHref = SafeHref(lane.AddHref)
			band = &lane
			// A drag stays inside its group, so each needs a name of its own:
			// the handle the config uses, else the verbatim string, else the
			// words — a lane's two ends together, since two lanes can start from
			// the same zone.
			reorderGroup = row.Group.Key
			if reorderGroup == "" {
				reorderGroup = row.Group.Chain
			}
			if reorderGroup == "" {
				reorderGroup = row.Group.Label + " → " + row.Group.To
			}
		}
		rv := tableRowView{ID: row.ID, Key: row.Key, ColumnSpan: columnSpan, Reorder: reorderable, ReorderGroup: reorderGroup, HasDetail: hasDetail, Inline: rowOpensItself(row), Dense: t.Dense, DrawerLabel: drawerLabel, DrawerIcon: t.DrawerIcon, Cells: make([]tableCellView, 0, len(t.Columns))}
		if t.Style == "interfaces" {
			depth := max(0, min(row.Depth, 2))
			rv.TreeRoot = depth == 0
			rv.TreeNodeCenter = depth*24 + 8
			rv.TreeNodeX = rv.TreeNodeCenter - 4
			rv.TreePath = fmt.Sprintf("M%d 22H66", rv.TreeNodeCenter)
			// Each level continues until its last descendant, including through
			// an expanded row. The plugin supplies the topology as row depths.
			for level := 0; level < depth; level++ {
				x := level*24 + 8
				continues := false
				for _, next := range rows[rowIndex+1:] {
					if next.Depth <= level {
						break
					}
					if next.Depth == level+1 {
						continues = true
						break
					}
				}
				if continues {
					rv.TreePath += fmt.Sprintf("M%d 0V44", x)
					rv.TreeContinuation += fmt.Sprintf("M%d 0V1", x)
				} else if level == depth-1 {
					rv.TreePath += fmt.Sprintf("M%d 0V22", x)
				}
				if level == depth-1 {
					rv.TreePath += fmt.Sprintf("M%d 22h24", x)
				}
			}
			if rowIndex+1 < len(rows) && rows[rowIndex+1].Depth > row.Depth {
				rv.TreePath += fmt.Sprintf("M%d 22V44", rv.TreeNodeCenter)
				rv.TreeContinuation += fmt.Sprintf("M%d 0V1", rv.TreeNodeCenter)
			}
		}
		if row.Group != nil {
			rv.Group = band
		}
		var expandErr error
		rv.Expanded, expandErr = r.renderChildren(row.Expanded, csrf)
		if expandErr != nil {
			return nil, expandErr
		}
		rv.Tags = strings.Join(row.Tags, " ")
		for _, key := range sortedKeys(row.Facet) {
			rv.Facets = append(rv.Facets, tableFacetView{Key: key, Value: row.Facet[key]})
		}
		switch {
		case row.Entity != nil:
			// The panel is the shell's and arrives when it opens, so the row
			// ships its address and nothing else.
			rv.Drawer = true
			rv.Entity = EntityPath(row.Entity.Kind, row.Entity.ID)
		case row.Drawer != nil:
			panel, err := r.renderPanel(row.Drawer, csrf, Flash{})
			if err != nil {
				return nil, err
			}
			rv.Drawer = true
			rv.Open = row.Drawer.Open
			rv.Panel = panel
		case row.Panel != "":
			// Nothing to render yet: the panel is one request away, and the row
			// ships the frame that will hold it. A row carrying both — the one
			// the address asked to be opened — renders its panel above and keeps
			// this address for the readings swapped in afterwards.
			rv.Drawer = true
			rv.PanelURL = SafeHref(row.Panel)
		}
		for i := range t.Columns {
			kind := t.Columns[i].Kind
			if kind == "" {
				kind = "text"
			}
			cv := tableCellView{Kind: kind, Primary: i == primary, Draggable: reorderable, RowID: row.ID, CSRFToken: csrf, Drawer: row.Drawer != nil || row.Entity != nil, Panel: rv.PanelURL, Dense: t.Dense}
			cv.Expandable = i == primary && len(row.Expanded) > 0
			if i == primary {
				switch {
				case row.Depth >= 3:
					cv.Indent = "pl-18"
				case row.Depth == 2:
					cv.Indent = "pl-12"
				case row.Depth == 1:
					cv.Indent = "pl-6"
				}
			}
			if i < len(row.Cells) {
				cv.TableCell = row.Cells[i]
				// A muted row mutes every cell in it: the secondary step is a
				// statement about the row's subject, so no cell in that row can
				// disagree with the rest.
				cv.Muted = cv.Muted || row.Muted
				if len(cv.Actions) > 0 {
					acts := make([]TableRowAct, len(cv.Actions))
					for j, a := range cv.Actions {
						if a.Href != "" {
							a.Href = SafeHref(a.Href)
						}
						acts[j] = a
					}
					cv.Actions = acts
				}
				if cv.Href != "" {
					cv.Href = SafeHref(cv.Href)
				}
				if cv.Confirm != "" {
					cv.ConfirmID = fmt.Sprintf("verso-action-confirm-%d", r.seq.cfm.Add(1))
					if cv.ConfirmTitle == "" {
						cv.ConfirmTitle = r.tr("Are you sure?")
					}
				}
				for _, ep := range row.Cells[i].Endpoints {
					icon, ok := endpointIcons[ep.Kind]
					if !ok {
						icon = "zone"
					}
					cv.Endpoints = append(cv.Endpoints, tableEndpointView{TableEndpoint: ep, Icon: icon})
				}
				if kind == "pill" && cv.Text != "" {
					cv.Pill = &Badge{Variant: cv.Variant, Text: cv.Text, Dot: cv.Dot, Icon: cv.Icon}
				}
			}
			rv.Cells = append(rv.Cells, cv)
		}
		out = append(out, rv)
	}
	return out, nil
}

func (t *Table) renderInto(r *Renderer, out io.Writer, csrf string) error {
	v, err := t.view(r, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "table.html.tmpl", v)
}
