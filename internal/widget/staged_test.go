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

// TestFormsThatStageSaySo: on a page whose saves stage, every form says so —
// one in a row's drawer included — so the page's script can show an unsaved
// edit where the save will go; a row says its name for the same line.
func TestFormsThatStageSaySo(t *testing.T) {
	r := newRenderer(t)
	page := &Form{Fields: []Widget{&Field{Name: "port", Label: "Port"}}}
	inDrawer := &Form{Fields: []Widget{&Field{Name: "target", Label: "Action"}}}
	table := &Table{Columns: []TableColumn{{Label: "Name"}}, Rows: []TableRow{{
		ID: "cfg01", Cells: []TableCell{{Text: "Allow-Ping"}},
		Drawer: &RowDrawer{Title: "Edit rule", Children: Widgets{inDrawer}},
	}}}
	MarkStaging(&Stack{Children: []Widget{page, table}})
	if !page.Stages || !inDrawer.Stages {
		t.Fatalf("forms that stage: page %v, drawer %v", page.Stages, inDrawer.Stages)
	}
	if got := render(t, r, page); !strings.Contains(got, "data-verso-stages") {
		t.Errorf("a form that stages does not say so:\n%s", got)
	}
	if got := render(t, r, table); !strings.Contains(got, `data-verso-row-name="Allow-Ping"`) {
		t.Errorf("a row does not say its name:\n%s", got)
	}
	if got := render(t, r, &Form{Fields: []Widget{&Field{Name: "x", Label: "X"}}}); strings.Contains(got, "data-verso-stages") {
		t.Error("a form nobody marked says it stages")
	}
}

// TestAMarkedRowCarriesItsAddress: a marked row's mark is named by the
// option's address, so the Pending changes page can lead to it on its page;
// a row marked by its own target and one by its form's are named alike.
func TestAMarkedRowCarriesItsAddress(t *testing.T) {
	r := newRenderer(t)
	port := &Field{Name: "Port", Label: "Port", Key: "Port"}
	redirect := &Switch{Name: "redirect_https", Label: "Redirect", Key: "redirect_https", Target: "uhttpd.main"}
	servers := &List{Name: "server", Label: "Time servers", Key: "server"}
	tree := &Stack{Children: []Widget{
		&Form{Target: "dropbear.cfg01", Fields: []Widget{port, redirect}},
		&Section{Target: "system.ntp", Children: []Widget{servers}},
	}}
	MarkStaged(tree, func(string) bool { return true })
	for name, c := range map[string]struct {
		w    Widget
		want string
	}{
		"field":  {port, `id="staged-dropbear.cfg01.Port"`},
		"switch": {redirect, `id="staged-uhttpd.main.redirect_https"`},
		"list":   {servers, `id="staged-system.ntp.server"`},
	} {
		if got := render(t, r, c.w); !strings.Contains(got, c.want+` data-verso-staged-row`) {
			t.Errorf("%s: the mark is named %s:\n%s", name, c.want, got)
		}
	}
}

// TestAStagedRowWearsTheStagesMark: the mark is a bar in the stage's
// marigold down the row's left edge, its word said to a screen reader only,
// drawn by the server so it stands on every visit until the change is
// applied or discarded.
func TestAStagedRowWearsTheStagesMark(t *testing.T) {
	r := newRenderer(t)
	mark := `<span data-verso-staged-row class="verso-staged-bar"><span aria-hidden="true"></span><span class="sr-only">staged</span></span>`
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
