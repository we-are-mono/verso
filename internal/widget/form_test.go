// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestFormDecodesActions(t *testing.T) {
	w, err := Decode([]byte(`{"type":"form","submit":"Save",` +
		`"actions":[{"label":"Generate keypair","action":"generate-keypair"}],"fields":[]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	f, ok := w.(*Form)
	if !ok {
		t.Fatalf("Decode returned %T, want *Form", w)
	}
	if len(f.Actions) != 1 || f.Actions[0].Action != "generate-keypair" || f.Actions[0].Label != "Generate keypair" {
		t.Errorf("actions not decoded: %+v", f.Actions)
	}
}

// TestFormRendersSecondaryActions: a secondary action renders as a submit button
// carrying its _action marker, so clicking it submits the whole form for the plugin
// to compute on (ADR-005 §7).
func TestFormRendersSecondaryActions(t *testing.T) {
	f := &Form{
		Submit:  "Save",
		Actions: []FormAction{{Label: "Generate keypair", Action: "generate-keypair"}},
		Fields:  []Widget{&Field{Name: "k", Label: "Key"}},
	}
	got := render(t, newRenderer(t), f)
	for _, want := range []string{
		">Save</button>",
		`name="_action" value="generate-keypair"`, ">Generate keypair</button>",
		"active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0",
		"dark:hover:border-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form render missing %q", want)
		}
	}
}

func TestPageFormUsesCapsuleInsteadOfOwnSubmit(t *testing.T) {
	f := &Form{Style: "page", Fields: []Widget{&Field{Name: "hostname", Label: "Hostname"}}}
	got := render(t, newRenderer(t), f)
	if !strings.Contains(got, "data-verso-page-form") {
		t.Fatalf("page form missing capsule hook: %s", got)
	}
	if strings.Contains(got, "<button") {
		t.Fatalf("page form must not render a competing submit button: %s", got)
	}
}

func TestSearchFormRendersSubmitInsideInputOutline(t *testing.T) {
	f := &Form{
		Style: "search", Icon: "search", Submit: "Search",
		Fields: []Widget{&Field{Name: "q", Placeholder: "Package name"}},
	}
	got := render(t, newRenderer(t), f)
	for _, want := range []string{
		"relative w-full max-w-sm",
		"[&_input[type=text]]:pr-28",
		"absolute inset-y-1 right-1",
		"bg-sky-600",
		`placeholder="Package name"`,
		">Search</button>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("search form render missing %q: %s", want, got)
		}
	}
}

func TestShellFormCanOmitGeneratedSubmitAndAutoSubmitFile(t *testing.T) {
	f := &Form{
		Action: "/system/maintenance/restore", Multipart: true,
		NoSubmit: true, AutoSubmit: true,
		Fields: []Widget{&Field{Name: "backup", Kind: "file"}},
	}
	got := render(t, newRenderer(t), f)
	for _, want := range []string{`action="/system/maintenance/restore"`, `enctype="multipart/form-data"`, `data-verso-autosubmit`} {
		if !strings.Contains(got, want) {
			t.Errorf("form render missing %q", want)
		}
	}
	if strings.Contains(got, ">Save</button>") {
		t.Fatalf("NoSubmit form rendered a generated Save button: %s", got)
	}
}
