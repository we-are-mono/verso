// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

// translateSchema walks the decoded widget tree and replaces each user-facing
// text field with its translation in place — the localization counterpart to
// the server's datatype walk over these same structs (ADR-008, ADR-012). It
// runs once, at the top of a render, so every widget (plugin-supplied and the
// shell's own trees alike) is covered from one place. Recursion is Walk over
// the widgets' shared children seam; the switch below holds only each widget's
// own fields, so a container never needs a case just to keep the walk going.
//
// t is a source-string-as-key lookup with English fallback: a field whose source
// text has no catalog entry keeps its English, so a machine value that happens to
// sit in a text field (an address, an id) passes through unchanged. Only prose is
// touched here; identifiers, enums, icons, hrefs, datatypes, form-field names and
// verbatim machine strings (Copy.Text, Code.Value, chart data, entity chips/tags)
// are left exactly as authored.
//
// The walk runs before Markdown conversion, so Markdown/Note/Sub source is
// translated too. Render-time string DEFAULTS (Form "Save", Confirm "Confirm"/
// "Cancel", Table "Details", …) are NOT applied here — they have no struct field
// to carry, so the renderer injects and translates them at render (Renderer.tr).
func translateSchema(w Widget, t func(string) string) {
	Walk(w, func(n Widget) { translateFields(n, t) })
}

// translateFields localizes one widget's own prose fields; nested widgets are
// reached by translateSchema's walk, never from a case here.
func translateFields(w Widget, t func(string) string) {
	switch n := w.(type) {
	case *Badge:
		n.Text = t(n.Text)
	case *Button:
		n.Label = t(n.Label)
		n.Busy = t(n.Busy)
	case *Callout:
		n.Title = t(n.Title)
		n.Body = t(n.Body)
	case *Card:
		n.Title = t(n.Title)
		if !n.SubtitleMono {
			n.Subtitle = t(n.Subtitle)
		}
	case *Chart:
		n.Label = t(n.Label)
		n.AxisStart = t(n.AxisStart)
		n.AxisEnd = t(n.AxisEnd)
		for i := range n.Series {
			n.Series[i].Label = t(n.Series[i].Label)
		}
	case *Code:
		n.Label = t(n.Label)
	case *Conditional:
		n.Label = t(n.Label)
	case *Conditions:
		n.Label = t(n.Label)
		n.Help = t(n.Help)
		for i := range n.Items {
			n.Items[i].Label = t(n.Items[i].Label)
			n.Items[i].Help = t(n.Items[i].Help)
		}
	case *Collection:
		// An item is a machine string (a key's name, its fingerprint) and is
		// never ours to translate; the words around the set are. The removal's
		// confirmations are children, and translate as every confirm does.
		n.Empty = t(n.Empty)
		if n.Add != nil {
			n.Add.Label = t(n.Add.Label)
			n.Add.Submit = t(n.Add.Submit)
			n.Add.Error = t(n.Add.Error)
		}
	case *Confirm:
		n.Trigger = t(n.Trigger)
		n.Title = t(n.Title)
		n.Message = t(n.Message)
		n.Confirm = t(n.Confirm)
		n.Cancel = t(n.Cancel)
	case *Disclosure:
		n.Summary = t(n.Summary)
	case *Empty:
		n.Title = t(n.Title)
		n.Body = t(n.Body)
	case *Field:
		n.Label = t(n.Label)
		n.Prompt = t(n.Prompt)
		n.Placeholder = t(n.Placeholder)
		if !n.HelpVerbatim {
			n.Help = t(n.Help)
		}
		// Key and Source are the config as it is written — machine strings, not
		// words — so they travel verbatim and only the sentence is looked up.
		n.Tip = t(n.Tip)
		// A shell- or plugin-authored inline error is prose; a datatype-validation
		// message (set by validateSchema, from the datatype package) simply misses
		// the catalog and stays English.
		n.Error = t(n.Error)
		// A locked field's value is the fixed setting said plainly ("Access
		// point"), not data someone typed, so it reads in the operator's language.
		if n.Style == "locked" {
			n.Value = t(n.Value)
		}
		for i := range n.Options {
			n.Options[i].Label = t(n.Options[i].Label)
		}
	case *Filter:
		n.Placeholder = t(n.Placeholder)
	case *Form:
		n.Submit = t(n.Submit)
		n.Error = t(n.Error)
		if !n.NoteVerbatim {
			n.Note = t(n.Note)
		}
		for i := range n.Actions {
			n.Actions[i].Label = t(n.Actions[i].Label)
		}
	case *Link:
		n.Desc = t(n.Desc)
		if n.Code == "Tunnels page" {
			n.Code = t(n.Code)
		}
		n.Label = t(n.Label)
	case *List:
		n.Label = t(n.Label)
		n.Prompt = t(n.Prompt)
		n.Help = t(n.Help)
		n.Tip = t(n.Tip)
	case *Meter:
		if !n.Verbatim {
			n.Label = t(n.Label)
		}
	case *Modal:
		n.Trigger = t(n.Trigger)
		n.BusyTitle = t(n.BusyTitle)
		if !n.BusyBodyVerbatim {
			n.BusyBody = t(n.BusyBody)
		}
		n.Title = t(n.Title)
		for i := range n.Steps {
			n.Steps[i] = t(n.Steps[i])
		}
	case *Ports:
		// The port's role name ("Internet", "Network 1") is prose; its interface,
		// address, speed and hover Note are machine facts left verbatim, and a
		// label declared Verbatim (a kernel name on an unprofiled board) is too.
		for i := range n.Items {
			if !n.Items[i].Verbatim {
				n.Items[i].Label = t(n.Items[i].Label)
			}
		}
	case *Properties:
		// A row's value is prose unless the row declares otherwise: Mono marks
		// a monospaced machine string, Chip an entity identity, Verbatim data
		// in sans type — none is ours to translate, and a machine value that
		// collides with a catalog key must not come back as words.
		for i := range n.Items {
			n.Items[i].Label = t(n.Items[i].Label)
			if !n.Items[i].HelpVerbatim {
				n.Items[i].Help = t(n.Items[i].Help)
			}
			if !n.Items[i].Mono && !n.Items[i].Chip && !n.Items[i].Verbatim {
				n.Items[i].Value = t(n.Items[i].Value)
			}
		}
	case *Grid:
		n.Label = t(n.Label)
		n.Help = t(n.Help)
		n.Join = t(n.Join)
	case *Raw:
		n.Markdown = t(n.Markdown)
	case *Repeater:
		n.AddLabel = t(n.AddLabel)
	case *Section:
		n.Title = t(n.Title)
		n.Sub = t(n.Sub)
		n.MetaLabel = t(n.MetaLabel)
		// Meta is compact status text beside the title — prose ("Last synchronization
		// not reported") translates; a live value or pre-composed string declares
		// MetaVerbatim and stays exactly as authored.
		if !n.MetaVerbatim {
			n.Meta = t(n.Meta)
		}
	case *Settings:
		n.Title = t(n.Title)
		translateSettingsItems(n.Items, t)
		if n.Seam != nil {
			n.Seam.Summary = t(n.Seam.Summary)
			translateSettingsItems(n.Seam.Items, t)
		}
	case *Stat:
		n.Label = t(n.Label)
		// Value is prose in a status tile ("Online") and a declared measurement
		// in a metric tile ("300", Verbatim) that stays exactly as authored;
		// the sub line carries its own declaration the same way.
		if !n.Verbatim {
			n.Value = t(n.Value)
		}
		if !n.SubVerbatim {
			n.Sub = t(n.Sub)
		}
	case *Switch:
		if !n.Verbatim {
			n.Label = t(n.Label)
		}
		n.OffLabel = t(n.OffLabel)
		n.Help = t(n.Help)
		n.Tip = t(n.Tip)
		n.Error = t(n.Error)
	case *ActionBar:
		translateDrawer(n.Drawer, t)
		n.Filter = t(n.Filter)
		for i := range n.Tabs {
			n.Tabs[i].Label = t(n.Tabs[i].Label)
		}
		if n.Select != nil {
			for i := range n.Select.Options {
				n.Select.Options[i].Label = t(n.Select.Options[i].Label)
			}
		}
		if n.Action != nil {
			n.Action.Label = t(n.Action.Label)
		}
	case *Table:
		translateTable(n, t)
	case *Text:
		if !n.Translated {
			n.Markdown = t(n.Markdown)
		}
	}
	// Any other widget (pure containers like Stack, or Overview, which is
	// shell page content that never reaches Decode) carries no prose of its own.
}

// translateSettingsItems localizes the rows' prose; a row's pill badges are
// widgets the walk reaches through the settings' children seam.
func translateSettingsItems(items []SettingsItem, t func(string) string) {
	for i := range items {
		items[i].Title = t(items[i].Title)
		items[i].Desc = t(items[i].Desc)
	}
}

// machineCellKinds are the column kinds whose cell Text/Sub carry identities,
// machine values, or a person's own text (a device name, a UCI comment) — never
// shell prose. The walk leaves them verbatim: the typography contract sets them
// in mono or muted type precisely because they are not words, and a machine
// string that collides with a catalog key must not come back as prose.
var machineCellKinds = map[string]bool{
	"path": true,
	"name": true, "reference": true, "mono": true, "keyword": true,
	"comment": true, "num": true, "rate": true, "runtime": true,
}

// translateTable localizes a table's chrome and its cells' prose. Column labels,
// the header band, the drawer action and group lane names are always prose; a
// cell's Text/Sub is prose in a status/pill/text column and stays verbatim in a
// machine-kind column (machineCellKinds) — the column's kind is the declaration.
// The entity fields a cell can carry (Chip, Tag, endpoint/chip labels) are
// identities (a zone, an interface, an address) and are deliberately left
// untranslated in every column. Row drawers' contents are widgets the walk
// reaches through the table's children seam; only the drawer title is a
// table-owned field.
func translateTable(n *Table, t func(string) string) {
	n.Title = t(n.Title)
	n.DrawerLabel = t(n.DrawerLabel)
	n.EmptyText = t(n.EmptyText)
	n.AddLabel = t(n.AddLabel)
	n.Note = t(n.Note)
	for i := range n.Legend {
		n.Legend[i].Label = t(n.Legend[i].Label)
		n.Legend[i].Detail = t(n.Legend[i].Detail)
	}
	for i := range n.Columns {
		n.Columns[i].Label = t(n.Columns[i].Label)
	}
	if n.Action != nil {
		n.Action.Label = t(n.Action.Label)
	}
	translateRows(n.Rows, n.Columns, t)
	if n.Seam != nil {
		n.Seam.Summary = t(n.Seam.Summary)
		translateRows(n.Seam.Rows, n.Columns, t)
	}
}

func translateRows(rows []TableRow, columns []TableColumn, t func(string) string) {
	for i := range rows {
		if rows[i].Group != nil {
			rows[i].Group.Label = t(rows[i].Group.Label)
			rows[i].Group.AddLabel = t(rows[i].Group.AddLabel)
		}
		for j := range rows[i].Cells {
			c := &rows[i].Cells[j]
			// A cell's action prose (Button, Confirm) is always words; Text and
			// Sub follow the column's declared kind. A row wider than its
			// columns keeps its overflow verbatim — shape errors must not turn
			// data into prose.
			if j < len(columns) && !machineCellKinds[columns[j].Kind] && !c.Verbatim {
				c.Text = t(c.Text)
				c.Sub = t(c.Sub)
			}
			c.Button = t(c.Button)
			c.Confirm = t(c.Confirm)
			c.ConfirmTitle = t(c.ConfirmTitle)
			for k := range c.Chips {
				c.Chips[k].Title = t(c.Chips[k].Title)
				if c.Chips[k].LocalizeLabel {
					c.Chips[k].Label = t(c.Chips[k].Label)
				}
			}
			for k := range c.Actions {
				c.Actions[k].Title = t(c.Actions[k].Title)
				c.Actions[k].ConfirmTitle = t(c.Actions[k].ConfirmTitle)
				c.Actions[k].Confirm = t(c.Actions[k].Confirm)
			}
		}
		translateDrawer(rows[i].Drawer, t)
	}
}

// A row and an Add action carry the same drawer schema. Its heading and tabs
// translate together; declared identities and tab addresses remain verbatim.
func translateDrawer(d *RowDrawer, t func(string) string) {
	if d == nil {
		return
	}
	if !d.Verbatim {
		d.Title = t(d.Title)
	}
	for i := range d.Tabs {
		d.Tabs[i].Label = t(d.Tabs[i].Label)
	}
}
