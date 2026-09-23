// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestMarkStagedFindsWhatWaits: a control that writes an option says where it
// lives — its own target, or the target of the form or section it sits in —
// and the shell marks every one whose option waits on the stage. An option of
// the same name in another section is another option.
func TestMarkStagedFindsWhatWaits(t *testing.T) {
	password := &Field{Name: "PasswordAuth", Label: "Allow password login", Key: "PasswordAuth"}
	port := &Field{Name: "Port", Label: "Port", Key: "Port"}
	root := &Switch{Name: "RootPasswordAuth", Label: "Allow root", Key: "RootPasswordAuth"}
	redirect := &Switch{Name: "redirect_https", Label: "Redirect", Key: "redirect_https", Target: "uhttpd.main"}
	servers := &List{Name: "server", Label: "Time servers", Key: "server"}
	unkeyed := &Field{Name: "note", Label: "Note"}
	tree := &Stack{Children: []Widget{
		&Form{Target: "dropbear.cfg01", Fields: []Widget{password, port, root, redirect, unkeyed}},
		&Section{Target: "system.ntp", Children: []Widget{servers}},
	}}
	staged := map[string]bool{
		"dropbear.cfg01.PasswordAuth":     true,
		"dropbear.cfg01.RootPasswordAuth": true,
		"uhttpd.main.redirect_https":      true,
		"system.ntp.server":               true,
		"dropbear.cfg02.Port":             true, // another section's Port
	}
	MarkStaged(tree, func(address string) bool { return staged[address] })
	for name, got := range map[string]bool{
		"password": password.Staged, "root": root.Staged, "redirect": redirect.Staged, "servers": servers.Staged,
	} {
		if !got {
			t.Errorf("%s waits on the stage and is not marked", name)
		}
	}
	if port.Staged || unkeyed.Staged {
		t.Error("a control whose own option does not wait is not marked")
	}
}

// TestAStagedRowWearsTheStagesMark: the mark is the chip's own square and
// word beside the row's name, drawn by the server so it stands on every visit
// until the change is applied or discarded.
func TestAStagedRowWearsTheStagesMark(t *testing.T) {
	r := newRenderer(t)
	mark := `<span data-verso-staged-row class="inline-flex items-center gap-1.5 text-sm font-medium text-marigold-deep"><span aria-hidden="true" class="size-1.5 shrink-0 rounded-[1px] bg-marigold"></span>staged</span>`
	for name, w := range map[string]Widget{
		"field":  &Field{Name: "Port", Label: "Port", Key: "Port", Staged: true},
		"switch": &Switch{Name: "RootPasswordAuth", Label: "Allow root", Key: "RootPasswordAuth", Staged: true},
		"list":   &List{Name: "server", Label: "Time servers", Key: "server", Staged: true},
	} {
		if got := render(t, r, w); !strings.Contains(got, mark) {
			t.Errorf("%s: a staged row wears the mark:\n%s", name, got)
		}
	}
	if got := render(t, r, &Field{Name: "Port", Label: "Port", Key: "Port"}); strings.Contains(got, "data-verso-staged-row") {
		t.Errorf("a row with nothing waiting wears no mark:\n%s", got)
	}
}
