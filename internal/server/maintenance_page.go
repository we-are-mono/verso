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
	Unchecked bool          // no check on record since the router started: the act reads Check now
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
				// The card's two acts are equals in one dress; the page's one
				// denim act, where it has one, stands on the heading line.
				&widget.Link{Style: "secondary", Label: "Download backup", Icon: "download", Href: backupPath},
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
// marigold band above the ledger. The automatic check stands under the acts
// as a switch; when it cannot be read, its notice is a warning too and leads
// with the ledger's own.
func firmwareSection(v maintenanceView) *widget.Section {
	label := "Check again"
	switch {
	case v.Checking:
		label = "Checking…"
	case v.Unchecked:
		label = "Check now"
	}
	check := &widget.Form{Action: updatesCheckPath, NoSubmit: true, Fields: []widget.Widget{
		&widget.Button{Label: label, Busy: "Checking…", Style: "act", Icon: "refresh-cw", Name: "action", Value: "check", Loading: v.Checking},
	}}
	s := &widget.Section{Title: "Firmware", Anchor: "firmware", Meta: v.Checked, MetaVerbatim: true, MetaPosition: "inline", Control: check}
	if l := v.Ledger; l.Title != "" {
		s.Children = append(s.Children, &widget.Callout{Variant: "warning", Compact: true, Title: l.Title, Body: l.Lede, Verbatim: l.Complaint})
	}
	autocheck := v.Autocheck
	if notice, unread := autocheck.(*widget.Callout); unread {
		s.Children = append(s.Children, notice)
		autocheck = nil
	}
	s.Children = append(s.Children, ledgerFacts(v.Ledger))
	if note := ledgerNote(v.Ledger); note != "" {
		s.Children = append(s.Children, &widget.Text{Markdown: note, Translated: true})
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
	if autocheck != nil {
		s.Children = append(s.Children, autocheck)
	}
	return s
}

// ledgerFacts is the firmware's diff, the fact list every card's facts are:
// each part beside what runs now, under the Part and Installed heads ("Current" would be read as a
// rail's current in a language that tells the two apart), and —
// while a build is offered — what it becomes, under the value with the arrow.
// Versions are machine strings, set in mono.
func ledgerFacts(l firmwareLedger) *widget.Properties {
	p := &widget.Properties{Align: "left", Head: &widget.PropertiesHead{Label: "Part", Value: "Installed"}}
	for _, row := range l.Rows {
		fact := widget.Property{Label: row.Label, Value: build(row.Current, row.CurrentRev), Mono: true}
		if row.Current == "" {
			fact = widget.Property{Label: row.Label, Value: "Unavailable"}
		}
		if l.Offer {
			ledgerAvailable(&fact, row)
		}
		p.Items = append(p.Items, fact)
	}
	return p
}

// ledgerAvailable says what the offered build can honestly say about a part:
// the build it becomes, under what runs now; that the update leaves it as it
// is; or nothing, because the server did not say.
func ledgerAvailable(fact *widget.Property, row ledgerRow) {
	switch row.Change {
	case ledgerChanged:
		fact.Next = build(row.Next, row.NextRev)
	case ledgerSame:
		fact.Help = "Stays as it is"
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
		// The box shows the hostname to type (the page's script gates the act
		// on it), and its note says so, so it never reads as a value in it.
		form.Fields = []widget.Widget{&widget.Field{Name: "hostname", Datatype: "hostname", Label: "Type the hostname to confirm", Placeholder: v.Hostname,
			Note: "The box shows this router’s hostname.", Autocomplete: "off", Required: true}}
	}
	// A reset leaves /data, and the router's history with it: it goes too
	// unless the operator keeps it (ADR-021).
	form.Fields = append(form.Fields, &widget.Field{Name: "history", Kind: "checks", Values: []string{"erase"},
		Options: []widget.Option{{Value: "erase", Label: "Also erase the router’s history"}}})
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
