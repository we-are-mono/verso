// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
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
//	"mono"     — verbatim machine strings: addresses, ports, device names
//	"keyword"  — closed-vocabulary words (tcp, udp, icmpv6): sans, muted
//	"comment"  — optional free text (e.g. a UCI name), muted, blank when absent
//	"num"      — right-aligned tabular figures (counters); muted
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
	Style   string        `json:"style,omitempty"` // "" / "flat" (default) — bare hairline rows, flush edges, non-clickable (values stay selectable); "more" opens from a trailing link, never the whole row | "lined" — inset hairline rows, for a live listing like a process table | "card" — the framed box
	Columns []TableColumn `json:"columns"`
	Rows    []TableRow    `json:"rows"`
	Seam    *TableSeam    `json:"seam,omitempty"`

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
}

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
	Cells  []TableCell `json:"cells"`
	Drawer *RowDrawer  `json:"drawer,omitempty"`
}

// RowDrawer is a row's edit surface: a right slide-in panel — typically a form
// prefilled with the section's values, a blast-radius callout, and a confirm
// for deletion. Same shell behaviour as the drawer widget (ADR-005 §7). Size
// widens the panel ("" reading width | "wide") for detail views that carry
// tables beside prose.
type RowDrawer struct {
	Title    string   `json:"title"`
	Size     string   `json:"size,omitempty"`
	Children []Widget `json:"children"`
}

// UnmarshalJSON decodes the drawer's children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing.
func (tr *TableRow) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID     string      `json:"id"`
		Cells  []TableCell `json:"cells"`
		Drawer *struct {
			Title    string            `json:"title"`
			Size     string            `json:"size"`
			Children []json.RawMessage `json:"children"`
		} `json:"drawer"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	tr.ID = raw.ID
	tr.Cells = raw.Cells
	tr.Drawer = nil
	if raw.Drawer == nil {
		return nil
	}
	d := &RowDrawer{Title: raw.Drawer.Title, Size: raw.Drawer.Size, Children: make([]Widget, 0, len(raw.Drawer.Children))}
	for i, rc := range raw.Drawer.Children {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("row drawer child %d: %w", i, err)
		}
		d.Children = append(d.Children, w)
	}
	tr.Drawer = d
	return nil
}

// TableCell carries the value for one cell; which field applies is decided by
// the column's kind (Text for text/name/mono/keyword/comment/num, Text+Variant
// for pill, On/Name for toggle, Endpoints for endpoint).
type TableCell struct {
	Text       string          `json:"text,omitempty"`
	Variant    string          `json:"variant,omitempty"`     // pill cells: the badge vocabulary ("success" | "warning" | "danger" | "info" | "neutral")
	Dot        bool            `json:"dot,omitempty"`         // pill cells: leading status dot — the same cue the badge carries elsewhere
	Copy       bool            `json:"copy,omitempty"`        // mono cells: offer the inline copy button beside the value
	Chip       string          `json:"chip,omitempty"`        // name cells: a small category chip inline after the name (e.g. its zone)
	Muted      bool            `json:"muted,omitempty"`       // text/mono cells: render the value as secondary ink (a quiet or absent value)
	Sub        string          `json:"sub,omitempty"`         // addr cells: a second line under the primary (e.g. the IPv6 under the IPv4), muted and copyable
	Tag        string          `json:"tag,omitempty"`         // name/status cells: a small coloured label after the value (e.g. "new", "WAN")
	TagVariant string          `json:"tag_variant,omitempty"` // the tag's palette (badge vocabulary): "" neutral | "info" | "warning" | "success" | "danger"
	Href       string          `json:"href,omitempty"`        // link cells: the destination of the row's action link
	Button     string          `json:"button,omitempty"`      // an in-cell button that opens the row's drawer (label is the text); replaces the auto trailing "Details" link for that row
	On         bool            `json:"on,omitempty"`
	Name       string          `json:"name,omitempty"` // form name the toggle posts under
	Endpoints  []TableEndpoint `json:"endpoints,omitempty"`
	Chips      []TableChip     `json:"chips,omitempty"` // entity cells: one or more icon+label reference chips
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
	Card        bool
	Lined       bool
	Condensed   bool
	Title       string
	Detail      string
	Action      *TableAction
	HasLabels   bool // any column carries a header label; a labelless table draws no <thead>
	Columns     []TableColumn
	HasDetail   bool
	Rows        []tableRowView
	SeamSummary string
	SeamRows    []tableRowView
}

type tableRowView struct {
	ID          string
	Cells       []tableCellView
	HasDetail   bool // table-wide flag, copied so the rows sub-template needs no second argument
	Drawer      bool // this row has a drawer (hosts the modal scope)
	Inline      bool // the drawer opens from an in-cell button, so this row shows no trailing "Details"
	DrawerWide  bool // the drawer opens at the wide detail width
	DrawerTitle string
	DrawerBody  []template.HTML
}

type tableCellView struct {
	Kind    string
	Primary bool // the first column — the row's identity, set one step larger
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

func (t *Table) view(r *Renderer, csrf string) (tableView, error) {
	v := tableView{
		Card: t.Style == "card", Lined: t.Style == "lined",
		Condensed: t.Condensed, Title: t.Title, Detail: t.Detail, Action: t.Action,
		HasLabels: hasColumnLabels(t.Columns),
		Columns:   t.Columns, HasDetail: t.hasDetail(),
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
	}
	return v, nil
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
	for _, row := range rows {
		rv := tableRowView{ID: row.ID, HasDetail: hasDetail, Inline: rowHasButton(row), Cells: make([]tableCellView, 0, len(t.Columns))}
		if row.Drawer != nil {
			body, err := r.renderChildren(row.Drawer.Children, csrf)
			if err != nil {
				return nil, err
			}
			rv.Drawer = true
			rv.DrawerWide = row.Drawer.Size == "wide"
			rv.DrawerTitle = row.Drawer.Title
			rv.DrawerBody = body
		}
		for i := range t.Columns {
			kind := t.Columns[i].Kind
			if kind == "" {
				kind = "text"
			}
			cv := tableCellView{Kind: kind, Primary: i == 0}
			if i < len(row.Cells) {
				cv.TableCell = row.Cells[i]
				for _, ep := range row.Cells[i].Endpoints {
					icon, ok := endpointIcons[ep.Kind]
					if !ok {
						icon = "zone"
					}
					cv.Endpoints = append(cv.Endpoints, tableEndpointView{TableEndpoint: ep, Icon: icon})
				}
				if kind == "pill" && cv.Text != "" {
					cv.Pill = &Badge{Variant: cv.Variant, Text: cv.Text, Dot: cv.Dot}
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
