// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"github.com/we-are-mono/verso/internal/widget"
)

// maintenanceView is what Maintenance shows, gathered by the handler: every
// string already localized, every act already a widget.
type maintenanceView struct {
	Ledger    firmwareLedger
	Checking  bool
	Checked   string        // when the firmware was last checked, beside Check again
	Notice    widget.Widget // a failed check, said above Firmware; nil when nothing failed
	Install   widget.Widget // the offered build's act; nil when nothing is offered
	Owut      widget.Widget // the way to the update tool; nil when it is installed
	Manual    widget.Widget // the custom image's dialog
	Packages  widget.Widget // newer packages, while there are any; nil otherwise
	Autocheck widget.Widget // whether the router checks on its own
	Restore   widget.Widget // the restore dialog
	Uptime    string
	Hostname  string // the key a factory reset asks for; empty when unreadable
	Staged    string // the stage's label while changes wait on it; empty otherwise
}

// Maintenance's routes, for the forms the page composes.
const (
	updatesCheckPath = maintenancePath + "/updates/check"
	restartPath      = maintenancePath + "/restart"
	factoryResetPath = maintenancePath + "/factory-reset"
	backupPath       = maintenancePath + "/backup"
)

// maintenanceBody is the page: the work in the form's measure, and the rail
// beside it that names its sections and what to do before flashing. It is
// composed from the shell's widgets alone, so it stands on the notebook's grid
// as every widget-drawn page does.
func maintenanceBody(v maintenanceView) widget.Widget {
	work := &widget.Stack{}
	if v.Notice != nil {
		work.Children = append(work.Children, v.Notice)
	}
	work.Children = append(work.Children,
		firmwareSection(v),
		&widget.Section{Title: "Back up and restore", Anchor: "back-up-and-restore", Hairline: true,
			Sub: "Restoring replaces the saved files and reboots the router.",
			Children: []widget.Widget{&widget.Stack{Inline: true, Children: []widget.Widget{
				&widget.Link{Style: "button", Label: "Download backup", Icon: "download", Href: backupPath},
				&widget.Text{Markdown: "or"},
				v.Restore,
			}}}},
		rebootSection(v),
		factoryResetSection(v),
	)
	return &widget.Grid{Style: "rail", Children: []widget.Widget{work, maintenanceRail()}}
}

// firmwareSection is the ledger: what this router runs part by part, with an
// Available column only while a build is offered. The check stands on the
// heading line with when it last ran; only a warning is explained, in the
// marigold band above the ledger.
func firmwareSection(v maintenanceView) *widget.Section {
	label := "Check again"
	if v.Checking {
		label = "Checking…"
	}
	check := &widget.Form{Action: updatesCheckPath, NoSubmit: true, Fields: []widget.Widget{
		&widget.Button{Label: label, Busy: "Checking…", Style: "act", Icon: "refresh-cw", Name: "action", Value: "check", Loading: v.Checking},
	}}
	s := &widget.Section{Title: "Firmware", Anchor: "firmware", Meta: v.Checked, MetaVerbatim: true, MetaPosition: "inline", Control: check}
	if l := v.Ledger; l.Title != "" {
		s.Children = append(s.Children, &widget.Callout{Variant: "warning", Compact: true, Title: l.Title, Body: l.Lede, Verbatim: l.Complaint})
	}
	s.Children = append(s.Children, ledgerTable(v.Ledger))
	if note := ledgerNote(v.Ledger); note != "" {
		s.Children = append(s.Children, &widget.Text{Markdown: note})
	}
	acts := &widget.Stack{Inline: true}
	for _, act := range []widget.Widget{v.Install, v.Owut, v.Manual} {
		if act != nil {
			acts.Children = append(acts.Children, act)
		}
	}
	s.Children = append(s.Children, acts)
	if v.Packages != nil {
		s.Children = append(s.Children, v.Packages)
	}
	s.Children = append(s.Children, v.Autocheck)
	return s
}

// ledgerTable is the firmware's diff: each part, what runs now, and — while a
// build is offered — what it becomes. Versions stay whole on one line: a
// version broken mid-token is a misread one.
func ledgerTable(l firmwareLedger) *widget.Table {
	t := &widget.Table{Columns: []widget.TableColumn{{Label: "Part", Kind: "name"}, {Label: "Current", Kind: "mono"}}}
	if l.Offer {
		t.Columns = append(t.Columns, widget.TableColumn{Label: "Available", Kind: "status"})
	}
	for _, row := range l.Rows {
		current := widget.TableCell{Text: build(row.Current, row.CurrentRev), Verbatim: true}
		if row.Current == "" {
			current = widget.TableCell{Text: "Unavailable", Muted: true}
		}
		cells := []widget.TableCell{{Text: row.Label, Verbatim: true}, current}
		if l.Offer {
			cells = append(cells, ledgerAvailable(row))
		}
		t.Rows = append(t.Rows, widget.TableRow{Cells: cells})
	}
	return t
}

// ledgerAvailable is what the Available column can honestly say about a part:
// the build it becomes, marked as the change; that a sysupgrade cannot change
// it; or nothing, because the server did not say.
func ledgerAvailable(row ledgerRow) widget.TableCell {
	switch row.Change {
	case ledgerChanged:
		return widget.TableCell{Text: build(row.Next, row.NextRev), Variant: "info", Verbatim: true}
	case ledgerSame:
		return widget.TableCell{Text: "same"}
	default:
		return widget.TableCell{Text: "—"}
	}
}

// build joins a version and its revision as the update tool writes them.
func build(version, revision string) string {
	if revision == "" {
		return version
	}
	return version + " " + revision
}

// ledgerNote is the line under the ledger: which server answered, and how many
// packages the offered build changes.
func ledgerNote(l firmwareLedger) string {
	switch {
	case l.Server != "" && l.Changes != "":
		return l.Server + " · " + l.Changes
	case l.Server != "":
		return l.Server
	}
	return l.Changes
}

// rebootSection: a reboot drops every device on the network, so the plain one
// asks first, in place. With changes staged the two named choices are the
// question already: apply them, or let them go, and then reboot.
func rebootSection(v maintenanceView) *widget.Section {
	s := &widget.Section{Title: "Reboot", Anchor: "reboot", Hairline: true, Meta: v.Uptime, MetaVerbatim: true, MetaLabel: "Running for"}
	if v.Staged == "" {
		s.Children = []widget.Widget{&widget.Form{Action: restartPath, NoSubmit: true, Fields: []widget.Widget{&widget.Confirm{
			Trigger: "Reboot now", Title: "Reboot the router now?",
			Message: "Every device on the network loses its connection for about a minute, then reconnects on its own.",
			Confirm: "Reboot", Cancel: "Not now",
		}}}}
		return s
	}
	s.Children = []widget.Widget{&widget.Form{Action: restartPath, NoSubmit: true, Fields: []widget.Widget{
		&widget.Callout{Variant: "warning", Compact: true, Body: v.Staged + " · Apply or discard these before rebooting."},
		&widget.Stack{Inline: true, Children: []widget.Widget{
			&widget.Button{Label: "Apply changes, then reboot", Style: "secondary", Name: "stage", Value: "apply"},
			&widget.Button{Label: "Discard and reboot", Style: "danger", Name: "stage", Value: "discard"},
		}},
	}}}
	return s
}

// factoryResetSection takes one deliberate key: the hostname, typed from what
// the box shows. The page's script only gates the act; the server holds the
// key too. A router whose hostname cannot be read shows no field, and the act
// is the whole form.
func factoryResetSection(v maintenanceView) *widget.Section {
	form := &widget.Form{Action: factoryResetPath, Submit: "Erase and start over", Tone: widget.ToneDanger}
	if v.Hostname != "" {
		// One field and its act on one row, the act beside the field it gates.
		form.Style = "search"
		form.Fields = []widget.Widget{&widget.Field{Name: "hostname", Datatype: "hostname", Label: "Type the hostname to confirm", Placeholder: v.Hostname, Autocomplete: "off", Required: true}}
	}
	return &widget.Section{Title: "Factory reset", Anchor: "factory-reset", Hairline: true,
		Sub:      "Erase settings, installed plugins, and local data. This cannot be undone and no backup is taken for you.",
		Children: []widget.Widget{form}}
}

// maintenanceRail names the page's sections, and what to do before flashing.
func maintenanceRail() widget.Widget {
	link := func(label, anchor string) widget.Widget {
		return &widget.Link{Style: "rail", Label: label, Href: "#" + anchor}
	}
	return &widget.Stack{Children: []widget.Widget{
		&widget.Section{Title: "On this page", Kicker: true, Children: []widget.Widget{&widget.Stack{Flush: true, Children: []widget.Widget{
			link("Firmware", "firmware"), link("Back up and restore", "back-up-and-restore"),
			link("Reboot", "reboot"), link("Factory reset", "factory-reset"),
		}}}},
		&widget.Section{Title: "Before you flash", Kicker: true, Children: []widget.Widget{&widget.Text{Markdown: "- Download a backup before replacing the firmware.\n- Use an image built for this board.\n- Keep the router connected to power during the upgrade."}}},
	}}
}
