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
	case *Callout:
		n.Title = t(n.Title)
		n.Body = t(n.Body)
	case *Card:
		n.Title = t(n.Title)
		n.Subtitle = t(n.Subtitle)
	case *Changes:
		for i := range n.Items {
			n.Items[i].Label = t(n.Items[i].Label)
		}
		for i := range n.Groups {
			n.Groups[i].Label = t(n.Groups[i].Label)
			for j := range n.Groups[i].Values {
				n.Groups[i].Values[j].Label = t(n.Groups[i].Values[j].Label)
			}
		}
	case *Chart:
		n.Label = t(n.Label)
		n.Title = t(n.Title)
		n.Note = t(n.Note)
		n.AxisStart = t(n.AxisStart)
		n.AxisEnd = t(n.AxisEnd)
		for i := range n.Series {
			n.Series[i].Label = t(n.Series[i].Label)
		}
	case *Choice:
		n.Label = t(n.Label)
		for i := range n.Options {
			n.Options[i].Label = t(n.Options[i].Label)
			n.Options[i].Desc = t(n.Options[i].Desc)
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
	case *Confirm:
		n.Trigger = t(n.Trigger)
		n.Message = t(n.Message)
		n.Confirm = t(n.Confirm)
		n.Cancel = t(n.Cancel)
	case *Copy:
		n.Label = t(n.Label)
	case *Disclosure:
		n.Summary = t(n.Summary)
	case *Divider:
		n.Label = t(n.Label)
	case *Drawer:
		n.Title = t(n.Title)
	case *Empty:
		n.Title = t(n.Title)
		n.Body = t(n.Body)
	case *Field:
		n.Label = t(n.Label)
		n.Prompt = t(n.Prompt)
		n.Placeholder = t(n.Placeholder)
		n.Help = t(n.Help)
		// A shell- or plugin-authored inline error is prose; a datatype-validation
		// message (set by validateSchema, from the datatype package) simply misses
		// the catalog and stays English.
		n.Error = t(n.Error)
		for i := range n.Options {
			n.Options[i].Label = t(n.Options[i].Label)
		}
	case *Filter:
		n.Placeholder = t(n.Placeholder)
	case *Form:
		n.Submit = t(n.Submit)
		n.Success = t(n.Success)
		n.Error = t(n.Error)
		n.Note = t(n.Note)
		for i := range n.Actions {
			n.Actions[i].Label = t(n.Actions[i].Label)
		}
	case *Hero:
		n.Title = t(n.Title)
		n.Body = t(n.Body)
	case *Link:
		n.Label = t(n.Label)
	case *List:
		n.Label = t(n.Label)
		n.Prompt = t(n.Prompt)
		n.Help = t(n.Help)
	case *Meter:
		n.Label = t(n.Label)
	case *Modal:
		n.Trigger = t(n.Trigger)
		n.BusyTitle = t(n.BusyTitle)
		n.BusyBody = t(n.BusyBody)
		n.Title = t(n.Title)
		n.Body = t(n.Body)
		n.Confirm = t(n.Confirm)
		n.Cancel = t(n.Cancel)
	case *NetMap:
		translateNetNode(&n.Source, t)
		translateNetNode(&n.Hub, t)
		for i := range n.Leaves {
			translateNetNode(&n.Leaves[i], t)
		}
	case *Ports:
		// The port's role name ("Internet", "Network 1") is prose; its interface,
		// address, speed and hover Note are machine facts left verbatim.
		for i := range n.Items {
			n.Items[i].Label = t(n.Items[i].Label)
		}
	case *Progress:
		n.Title = t(n.Title)
		n.Body = t(n.Body)
	case *Properties:
		for i := range n.Items {
			n.Items[i].Label = t(n.Items[i].Label)
			n.Items[i].Help = t(n.Items[i].Help)
			n.Items[i].Value = t(n.Items[i].Value)
		}
	case *Qr:
		n.Caption = t(n.Caption)
		n.DownloadLabel = t(n.DownloadLabel)
	case *Raw:
		n.Markdown = t(n.Markdown)
	case *Repeater:
		n.AddLabel = t(n.AddLabel)
	case *Row:
		n.Title = t(n.Title)
	case *Section:
		n.Title = t(n.Title)
		n.Sub = t(n.Sub)
		n.MetaLabel = t(n.MetaLabel)
		// Meta is compact status text beside the title — prose ("Last synchronization
		// not reported") in some sections, a live value (a timestamp, a subnet) in
		// others; the latter simply misses the catalog and stays verbatim.
		n.Meta = t(n.Meta)
	case *Settings:
		n.Title = t(n.Title)
		translateSettingsItems(n.Items, t)
		if n.Seam != nil {
			n.Seam.Summary = t(n.Seam.Summary)
			translateSettingsItems(n.Seam.Items, t)
		}
	case *Stat:
		n.Label = t(n.Label)
		n.Sub = t(n.Sub)
	case *Switch:
		n.Label = t(n.Label)
		n.OffLabel = t(n.OffLabel)
		n.Help = t(n.Help)
	case *Table:
		translateTable(n, t)
	case *Tabs:
		for i := range n.Tabs {
			n.Tabs[i].Label = t(n.Tabs[i].Label)
		}
	case *Text:
		n.Markdown = t(n.Markdown)
	case *Toggle:
		n.Label = t(n.Label)
		n.OffLabel = t(n.OffLabel)
		n.Meta = t(n.Meta)
	}
	// Any other widget (pure containers like Stack/Grid, or Overview, which is
	// shell page content that never reaches Decode) carries no prose of its own.
}

// translateNetNode localizes a map node's label and detail. A leaf's label is
// often a zone identity (lan, guest) rather than prose — those simply miss the
// catalog and stay verbatim, the same way a table leaves its entity chips alone.
func translateNetNode(n *NetNode, t func(string) string) {
	n.Label = t(n.Label)
	n.Detail = t(n.Detail)
}

// translateSettingsItems localizes the rows' prose; a row's pill badges are
// widgets the walk reaches through the settings' children seam.
func translateSettingsItems(items []SettingsItem, t func(string) string) {
	for i := range items {
		items[i].Title = t(items[i].Title)
		items[i].Desc = t(items[i].Desc)
	}
}

// translateTable localizes a table's chrome and its cells' prose. Column labels,
// the header band, the drawer action and group lane names are always prose; a
// cell's Text/Sub is prose in a status/pill/text column and a machine value in a
// mono/num column — the latter simply misses the catalog and stays verbatim. The
// entity fields a cell can carry (Chip, Tag, endpoint/chip labels) are identities
// (a zone, an interface, an address) and are deliberately left untranslated. Row
// drawers' contents are widgets the walk reaches through the table's children
// seam; only the drawer title is a table-owned field.
func translateTable(n *Table, t func(string) string) {
	n.Title = t(n.Title)
	n.DrawerLabel = t(n.DrawerLabel)
	for i := range n.Columns {
		n.Columns[i].Label = t(n.Columns[i].Label)
	}
	if n.Action != nil {
		n.Action.Label = t(n.Action.Label)
	}
	translateRows(n.Rows, t)
	if n.Seam != nil {
		n.Seam.Summary = t(n.Seam.Summary)
		translateRows(n.Seam.Rows, t)
	}
}

func translateRows(rows []TableRow, t func(string) string) {
	for i := range rows {
		if rows[i].Group != nil {
			rows[i].Group.Label = t(rows[i].Group.Label)
		}
		for j := range rows[i].Cells {
			c := &rows[i].Cells[j]
			c.Text = t(c.Text)
			c.Sub = t(c.Sub)
			c.Button = t(c.Button)
			c.Confirm = t(c.Confirm)
			c.ConfirmTitle = t(c.ConfirmTitle)
		}
		if rows[i].Drawer != nil {
			rows[i].Drawer.Title = t(rows[i].Drawer.Title)
		}
	}
}
