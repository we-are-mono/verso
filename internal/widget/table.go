// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"io"
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
//	"keyword"  — closed-vocabulary words (tcp, udp, icmpv6): sans, muted
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
	Style       string        `json:"style,omitempty"` // "" / "flat" (default) — bare hairline rows, flush edges, non-clickable (values stay selectable); "more" opens from a trailing link, never the whole row | "lined" — inset hairline rows, for a live listing like a process table | "card" — the framed box
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

	// Condensed lowers the flat style's row padding only — same anatomy, tighter
	// vertical rhythm, for a dense listing. Ignored by the other styles.
	Condensed bool `json:"condensed,omitempty"`

	// Align "top" pins every cell to the row's top instead of centring it — so a
	// cell that stacks two values (an IPv4 over an IPv6) keeps its first line on
	// the shared top line with the single-value cells, the second dangling below.
	Align string `json:"align,omitempty"`

	// EmptyText is what a listing with nothing in it says. Column headings
	// describe data; over no data they are chrome, so an empty table drops them
	// and renders one quiet full-width row carrying this sentence — the treatment
	// for a listing that is one section among several. A listing that IS the page
	// deserves the full empty widget instead. Left blank it reads "Nothing here
	// yet" in the operator's language.
	EmptyText string `json:"empty_text,omitempty"`

	// ReorderConfig names the uci config whose sections these rows are, and it is
	// what makes a leading "reorder" column real: the shell posts the dragged id
	// sequence back and stages a `uci order` on that config. A table without it
	// draws no handle — a grip the device would not remember is a lie.
	ReorderConfig string `json:"reorder_config,omitempty"`

	// ReorderLabel names the order in the operator's words in the capsule's
	// review ("Rule order"); an empty label falls back to the shell's generic
	// "Order". The listing knows what its rows are; the shell does not.
	ReorderLabel string `json:"reorder_label,omitempty"`
}

// The form fields the shell's reorder interaction posts, and that the gateway
// reads to stage the new section order. They live here because this package
// renders the affordance; the gateway imports these same constants, so the two
// never drift. verso.js builds the body from the table's own data attributes.
const (
	ReorderConfigField = "_uci_order_config"
	ReorderIDField     = "_uci_order"
)

// TableAction is the flat header band's trailing link (e.g. "View all").
type TableAction struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

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
}

// TableRow is one config section. ID is its stable handle (e.g. the UCI section
// name); it does not render. A row with a Drawer is an object you can open:
// clicking it slides in the drawer (the row gets a trailing chevron and the
// pointer; controls inside the row keep their own meaning).
type TableRow struct {
	ID     string      `json:"id,omitempty"`
	Key    string      `json:"key,omitempty"`   // optional stable live-update hook; not displayed
	Group  *TableGroup `json:"group,omitempty"` // optional evaluation-lane header before this row
	Cells  []TableCell `json:"cells"`
	Drawer *RowDrawer  `json:"drawer,omitempty"`
}

// TableGroup introduces a run of rows that share one effective evaluation lane.
// Label is human-facing; Chain keeps firewall4's exact name visible to experts.
type TableGroup struct {
	Label string `json:"label"`
	Chain string `json:"chain"`
	Count int    `json:"count"`
}

// RowDrawer is a row's edit surface: a right slide-in panel — typically a form
// prefilled with the section's values, a blast-radius callout, and a confirm
// for deletion. Same shell behaviour as the drawer widget (ADR-005 §7). Size
// widens the panel ("" reading width | "wide") for detail views that carry
// tables beside prose. Open renders the panel already open, which is how a
// submission the plugin refused comes back with the failed drawer in front of
// the operator instead of silently closed.
type RowDrawer struct {
	Title     string   `json:"title"`
	Size      string   `json:"size,omitempty"`
	HideTitle bool     `json:"hide_title,omitempty"`
	Open      bool     `json:"open,omitempty"`
	Children  []Widget `json:"children"`
}

// UnmarshalJSON decodes the drawer's children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing.
func (tr *TableRow) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID     string      `json:"id"`
		Key    string      `json:"key"`
		Group  *TableGroup `json:"group"`
		Cells  []TableCell `json:"cells"`
		Drawer *struct {
			Title     string            `json:"title"`
			Size      string            `json:"size"`
			HideTitle bool              `json:"hide_title"`
			Open      bool              `json:"open"`
			Children  []json.RawMessage `json:"children"`
		} `json:"drawer"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	tr.ID = raw.ID
	tr.Key = raw.Key
	tr.Group = raw.Group
	tr.Cells = raw.Cells
	tr.Drawer = nil
	if raw.Drawer == nil {
		return nil
	}
	children, err := decodeChildren(raw.Drawer.Children, "row drawer child")
	if err != nil {
		return err
	}
	tr.Drawer = &RowDrawer{Title: raw.Drawer.Title, Size: raw.Drawer.Size, HideTitle: raw.Drawer.HideTitle, Open: raw.Drawer.Open, Children: children}
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
	Chips        []TableChip     `json:"chips,omitempty"` // entity/name/reference cells: one or more icon+label reference chips
}

// TableChip is one entity-reference chip: a Lucide type icon (naming the kind —
// an interface, a zone, a physical port) plus the entity's label (naming the
// one). It is the shared treatment for referencing a network entity inside
// another entity's row, so the three never blur together (a shield is always a
// zone, a network glyph always an interface, a port glyph always a port).
type TableChip struct {
	Icon  string `json:"icon"`
	Label string `json:"label"`
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
	Card          bool
	Lined         bool
	Condensed     bool
	AlignTop      bool
	Title         string
	Detail        string
	Action        *TableAction
	HasLabels     bool // any column carries a header label; a labelless table draws no <thead>
	Columns       []TableColumn
	HasDetail     bool
	Empty         bool // nothing to list: no head, no rows, one quiet sentence
	EmptyText     string
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

type tableRowView struct {
	ID           string
	Key          string
	Group        *TableGroup
	ColumnSpan   int
	Reorder      bool
	ReorderGroup string
	Cells        []tableCellView
	HasDetail    bool // table-wide flag, copied so the rows sub-template needs no second argument
	Drawer       bool // this row has a drawer (hosts the modal scope)
	Inline       bool // the drawer opens from an in-cell button, so this row shows no trailing "Details"
	Seam         bool // this row belongs to the collapsible continuation block
	Open         bool // the row's drawer renders already open
	DrawerLabel  string
	DrawerIcon   string
	Panel        drawerPanelView // the row's slide-in detail panel (shared with the drawer widget)
}

type tableCellView struct {
	Kind      string
	Primary   bool // the first column — the row's identity, set one step larger
	Draggable bool // reorder cells: the table persists an order, so draw the handle
	RowID     string
	CSRFToken string
	Drawer    bool
	ConfirmID string
	TableCell
	Endpoints []tableEndpointView
	Pill      *Badge // pill cells render through the badge component
}

type tableEndpointView struct {
	TableEndpoint
	Icon string
}

// rowHasButton reports whether any cell offers an in-cell button (which opens
// the row's drawer in place of a trailing "Details" link).
func rowHasButton(row TableRow) bool {
	for _, c := range row.Cells {
		if c.Button != "" {
			return true
		}
	}
	return false
}

// hasDetail reports whether the table needs a trailing "Details" column: true
// when some drawer row has no in-cell button to open itself.
func (t *Table) hasDetail() bool {
	need := func(rows []TableRow) bool {
		for _, row := range rows {
			if row.Drawer != nil && !rowHasButton(row) {
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

func (t *Table) view(r *Renderer, csrf string) (tableView, error) {
	empty := t.empty()
	v := tableView{
		Card: t.Style == "card", Lined: t.Style == "lined",
		Condensed: t.Condensed, AlignTop: t.Align == "top",
		Title: t.Title, Detail: t.Detail, Action: t.Action,
		HasLabels: hasColumnLabels(t.Columns) && !empty,
		Columns:   t.Columns, HasDetail: t.hasDetail(),
		Empty: empty, EmptyText: t.EmptyText,
	}
	if empty && v.EmptyText == "" {
		v.EmptyText = r.tr("Nothing here yet")
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
	for _, row := range rows {
		if row.Group != nil {
			reorderGroup = row.Group.Chain
			if reorderGroup == "" {
				reorderGroup = row.Group.Label
			}
		}
		columnSpan := len(t.Columns)
		if hasDetail {
			columnSpan++
		}
		rv := tableRowView{ID: row.ID, Key: row.Key, Group: row.Group, ColumnSpan: columnSpan, Reorder: reorderable, ReorderGroup: reorderGroup, HasDetail: hasDetail, Inline: rowHasButton(row), DrawerLabel: drawerLabel, DrawerIcon: t.DrawerIcon, Cells: make([]tableCellView, 0, len(t.Columns))}
		if row.Drawer != nil {
			body, err := r.renderChildren(row.Drawer.Children, csrf)
			if err != nil {
				return nil, err
			}
			rv.Drawer = true
			rv.Open = row.Drawer.Open
			rv.Panel = drawerPanelView{
				Title: row.Drawer.Title, Wide: row.Drawer.Size == "wide",
				HideTitle: row.Drawer.HideTitle, Body: body,
			}
		}
		for i := range t.Columns {
			kind := t.Columns[i].Kind
			if kind == "" {
				kind = "text"
			}
			cv := tableCellView{Kind: kind, Primary: i == primary, Draggable: reorderable, RowID: row.ID, CSRFToken: csrf, Drawer: row.Drawer != nil}
			if i < len(row.Cells) {
				cv.TableCell = row.Cells[i]
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
