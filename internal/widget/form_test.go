// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestFormActionsFinishTheirConfigurationCard(t *testing.T) {
	for name, tail := range map[string][]Widget{
		"direct":              {&Code{Label: "/etc/config/wireless", Live: true}},
		"nested with carrier": {&Section{Children: []Widget{&Code{Label: "/etc/config/firewall", Live: true}}}, &Field{Name: "_panel", Kind: "hidden", Value: "1"}},
		"static cards":        {&Code{Label: "/etc/config/firewall"}, &Code{Label: "/etc/config/qos"}},
	} {
		t.Run(name, func(t *testing.T) {
			for _, f := range []*Form{
				{Frame: FramePanel, Submit: "Save", Fields: tail},
				{Style: "page", Submit: "Save", Fields: tail},
				{Style: "settings", Submit: "Save", Fields: tail},
			} {
				got := render(t, newRenderer(t), f)
				if !strings.Contains(got, `<div data-verso-form-actions class="verso-form__acts" data-verso-close="flush">`) {
					t.Errorf("Save must join the card without another rule:\n%s", got)
				}
			}
			if EndsWithCode(&Stack{Children: append(append([]Widget{}, tail...), &Field{Name: "after", Kind: "text"})}) {
				t.Error("a card followed by another control must not absorb the actions")
			}
		})
	}
}

// A form whose questions are names alone (signing in) is a stack of fields at
// every width: each name over its box, the box the form's width, the rows
// 24px apart with no hairline between them or over the first. Its rows answer
// to a container of their own, so the ledger's measures never reach them, and
// the ledger's hairlines pass them by.
func TestShortQuestionsStandAsAStackOfFields(t *testing.T) {
	r := newRenderer(t)
	fields := []Widget{&Field{Name: "username", Label: "Username"}}
	if got := render(t, r, &Form{Action: "/login", Submit: "Sign in", ShortQuestions: true, Fields: fields}); !strings.Contains(got, `<form method="post" action="/login" data-verso-questions="short" class="verso-form verso-rhythm">`) {
		t.Errorf("a form of short questions does not say so:\n%s", got)
	}
	if got := render(t, r, &Form{Submit: "Save", Fields: fields}); strings.Contains(got, "data-verso-questions") {
		t.Errorf("a form of described questions claims short ones:\n%s", got)
	}
	const short = `.verso-form[data-verso-questions="short"] *`
	declares(t, "field", `.verso-field:where(`+short+`)`, "container-name: verso-field-stack")
	declares(t, "field", `.verso-field__ledger:is(`+short+`)`, "grid-template-columns: minmax(0, 1fr)")
	declares(t, "field", `.verso-field__answer:is(`+short+`)`, "grid-column: 1")
	css := blockCSS(t, "field")
	// Rows meeting on a hairline, the trail over the next row, a form opening
	// on a hairline under what stands over it in a card, the row's 16px over
	// the acts; and the acts' own hairline.
	if n := strings.Count(css, `:not(:where(`+short+`))`); n != 4 {
		t.Errorf("the ledger's hairlines pass a stack of fields by in %d rules, want 4", n)
	}
	if !strings.Contains(blockCSS(t, "form"), `[data-verso-rows]) + *):not(:where(`+short+`)) {`) {
		t.Error("the acts under a stack of fields draw the ledger's hairline")
	}
	if strings.Contains(css, "@container verso-field-stack") {
		t.Error("a stack of fields takes a measure of the ledger's")
	}
}

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

// TestFormPostsIntoItsFrame: a form inside a panel posts back into the frame
// that holds it rather than navigating — the shell's entity panel body at the
// tab's own route, or a plugin's open panel at the page's own address, which is
// the one the address bar already names. An ordinary form carries none of it,
// and posts its page as it always did.
func TestFormPostsIntoItsFrame(t *testing.T) {
	r := newRenderer(t)
	field := func() Widget { return &Field{Name: "k", Label: "Key"} }

	entity := render(t, r, &Form{Frame: FrameEntity, Panel: "/entity/device/aa?tab=shape", Fields: []Widget{field()}})
	if !strings.Contains(entity, `hx-post="/entity/device/aa?tab=shape" hx-target="closest [data-verso-entity-body]" hx-swap="innerHTML"`) {
		t.Errorf("an entity tab's form should post into the entity body at its route:\n%s", entity)
	}

	panel := render(t, r, &Form{Frame: FramePanel, Fields: []Widget{field()}})
	if !strings.Contains(panel, `hx-post="" hx-target="closest [data-verso-panel]" hx-swap="innerHTML"`) {
		t.Errorf("a panel's form should post into the panel at the page's own address:\n%s", panel)
	}

	plain := render(t, r, &Form{Fields: []Widget{field()}})
	if strings.Contains(plain, "hx-post") || strings.Contains(plain, "hx-target") {
		t.Errorf("an ordinary form posts its page and carries no frame:\n%s", plain)
	}
}

// TestPostIntoFrameReachesEveryUnframedForm: framing a tree frames every form in
// it, however deep, and leaves one that already knows its frame alone — the
// call closest to a form is the one that knows where it is.
func TestPostIntoFrameReachesEveryUnframedForm(t *testing.T) {
	inner := &Form{Fields: []Widget{&Field{Name: "a"}}}
	framed := &Form{Frame: FrameEntity, Panel: "/entity/x", Fields: []Widget{&Field{Name: "b"}}}
	tree := &Stack{Children: []Widget{&Section{Children: []Widget{inner}}, framed}}
	PostIntoFrame(tree, FramePanel, "")
	if inner.Frame != FramePanel || inner.Panel != "" {
		t.Errorf("the nested form should post into the panel, got frame %q at %q", inner.Frame, inner.Panel)
	}
	if framed.Frame != FrameEntity || framed.Panel != "/entity/x" {
		t.Errorf("a form already framed must keep its frame, got %q at %q", framed.Frame, framed.Panel)
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
		`value="generate-keypair" class="verso-button" data-verso-style="secondary">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form render missing %q", want)
		}
	}
}

// A page form with no submit label renders no button of its own: the label is
// the gateway's to supply, so the widget never invents one that would compete
// with it.
func TestPageFormWithoutLabelRendersNoButtonOfItsOwn(t *testing.T) {
	f := &Form{Style: "page", Fields: []Widget{&Field{Name: "hostname", Label: "Hostname"}}}
	got := render(t, newRenderer(t), f)
	if !strings.Contains(got, "data-verso-page-form") {
		t.Fatalf("page form missing hook: %s", got)
	}
	if strings.Contains(got, "<button") {
		t.Fatalf("a label-less page form must not render a competing submit button: %s", got)
	}
}

// A settings form's Save waits for a change, and while it waits it rests in
// sand, not in a faded denim: denim means there is something to act on, and a
// form with nothing changed has nothing.
func TestSettingsFormSaveRestsInSandUntilChanged(t *testing.T) {
	f := &Form{Style: "settings", Submit: "Save", Fields: []Widget{&Field{Name: "x"}}}
	got := render(t, newRenderer(t), f)
	for _, want := range []string{
		"data-verso-dirty-form",
		// The button block's standby rests the Save in sand.
		`class="verso-button" data-verso-standby>Save</button>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings form missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "disabled:opacity-40") {
		t.Errorf("a waiting Save must not be a faded denim:\n%s", got)
	}
	// It is drawn resting: a Save drawn denim and put to rest by the page's
	// script a moment later blinks denim on every visit.
	if !strings.Contains(got, `<button type="submit" disabled class=`) {
		t.Errorf("a settings form's Save is drawn resting, not put to rest after:\n%s", got)
	}
	if plain := render(t, newRenderer(t), &Form{Submit: "Save", Fields: []Widget{&Field{Name: "x"}}}); strings.Contains(plain, " disabled class=") {
		t.Errorf("an ordinary form's Save is not held:\n%s", plain)
	}
}

// A form that commits one section stands its Save 24px under its own fields,
// with no rule of its own: the next section's rule is the only line between
// them, and a ruled Save would read as a section of its own. A form that is
// the page's (its sections inside it) commits the whole page, so its Save
// stands on the sand under the page's cards, whose frames rule it off: 24px
// under the last card's line, a pixel short after its frame, drawing no rule
// of its own. A form in a panel keeps its own rule inside the panel. Every
// rule lands on a line of the grid (its margin its air less its own pixel).
func TestSectionFormCommitsWithoutARule(t *testing.T) {
	r := newRenderer(t)
	form := func() *Form {
		return &Form{Style: "settings", Submit: "Save", Fields: []Widget{&Field{Name: "x"}}}
	}
	sectioned := render(t, r, &Section{Title: "SSH", Hairline: true, Children: []Widget{form()}})
	if !strings.Contains(sectioned, `<div data-verso-form-actions class="verso-form__acts" data-verso-close="flush">`) {
		t.Errorf("a section's Save stands under its fields, unruled:\n%s", sectioned)
	}
	if got := render(t, r, form()); !strings.Contains(got, `<div data-verso-form-actions class="verso-form__acts" data-verso-close="page">`) {
		t.Errorf("the page's form does not close the page:\n%s", got)
	}
	framed := form()
	framed.Frame = "panel"
	if got := render(t, r, framed); !strings.Contains(got, `<div data-verso-form-actions class="verso-form__acts">`) {
		t.Errorf("a panel's form closes on its own rule, inside the panel:\n%s", got)
	}
	// The acts' rule and air: 24px with its own pixel by default; on the
	// page's close, or under a card, no rule and 24px under the card's line
	// less its frame's pixel; none and 24px flush.
	declares(t, "form", ".verso-form__acts",
		"--verso-acts-after: calc(var(--spacing) * 5.75);", "margin-top: var(--verso-acts-after);",
		"padding-top: calc(var(--spacing) * 5.75);", "border-top-width: 1px;")
	declares(t, "form", `.verso-form__acts:is([data-verso-close="page"], :is(.verso-card, .verso-code[data-verso-place="card"], .verso-section:has(> .verso-section__body > .verso-code[data-verso-place="card"]:last-child)) ~ *)`,
		"--verso-acts-after: calc(var(--spacing-inset) - 1px);", "padding-top: 0;", "border-top-width: 0;")
	declares(t, "form", `.verso-form__acts[data-verso-close="flush"]`,
		"--verso-acts-after: var(--spacing-inset);", "padding-top: 0;", "border-top-width: 0;")
	// A form that is only its act (Install htop, under the package's facts)
	// has no fields to close off: its button stands 24px under whatever is
	// above it, drawing no rule of its own.
	lone := &Form{Frame: "panel", Submit: "Install htop", Fields: []Widget{
		&Field{Kind: "hidden", Name: "package", Value: "htop"},
		&Field{Kind: "hidden", Name: "_primary", Value: "install"},
	}}
	if got := render(t, r, lone); !strings.Contains(got, `<div data-verso-form-actions class="verso-form__acts" data-verso-close="flush">`) {
		t.Errorf("a lone act draws a rule over itself:\n%s", got)
	}
}

// A record editor opts into its own submit by giving the page form a label; it
// then carries its own button (it stages and returns to the listing).
func TestPageFormWithLabelCarriesOwnSubmit(t *testing.T) {
	f := &Form{Style: "page", Submit: "Add rule", Fields: []Widget{&Field{Name: "x"}}}
	got := render(t, newRenderer(t), f)
	if !strings.Contains(got, `type="submit"`) || !strings.Contains(got, "Add rule") {
		t.Fatalf("a labelled page form must render its own submit button: %s", got)
	}
}

// TestSearchFormSetsSubmitBesideItsField: a search and its act stand side by
// side, 8px apart, each control whole with its own corners and border, as
// Factory reset's confirm row does; the button never sits inside the field,
// and stands on the field's own two cells, under a label when it has one.
func TestSearchFormSetsSubmitBesideItsField(t *testing.T) {
	f := &Form{
		Style: "search", Icon: "search", Submit: "Search",
		Fields: []Widget{&Field{Name: "q", Placeholder: "Package name"}},
	}
	got := render(t, newRenderer(t), f)
	for _, want := range []string{
		`data-verso-kind="search"`,
		`<div class="verso-form__search">`,
		`<button type="submit" class="verso-button shrink-0">`,
		`placeholder="Package name"`,
		">Search</button>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("search form render missing %q: %s", want, got)
		}
	}
	declares(t, "form", ".verso-form__search", "display: flex;", "align-items: flex-end;", "gap: calc(var(--spacing) * 2);", "max-width: 100%;")
	declares(t, "field", ".verso-field:is(.verso-form__search > *)", "width: calc(var(--spacing) * 80);", "min-width: 0;")
	for _, never := range []string{"absolute inset-y-1", "pr-28"} {
		if strings.Contains(got, never) {
			t.Errorf("the search button sits inside its field (%q): %s", never, got)
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

// TestShellFormDrivenByItsConfirm: a form whose contents carry a confirm already
// has its submit — the guarded one — so no Save is generated beside it. Naming a
// submit label explicitly still yields both.
func TestShellFormDrivenByItsConfirm(t *testing.T) {
	fields := []Widget{
		&Field{Name: "_delete", Kind: "hidden", Value: "1"},
		&Stack{Children: []Widget{&Confirm{Trigger: "Delete rule", Message: "Delete this rule?"}}},
	}
	got := render(t, newRenderer(t), &Form{Fields: fields})
	if strings.Contains(got, ">Save</button>") {
		t.Errorf("a confirm-driven form rendered a generated Save button: %s", got)
	}
	if !strings.Contains(got, ">Delete rule</button>") {
		t.Errorf("the confirm's own trigger is missing: %s", got)
	}

	both := render(t, newRenderer(t), &Form{Submit: "Save", Fields: fields})
	if !strings.Contains(both, ">Save</button>") {
		t.Errorf("an explicit submit label must still render its button: %s", both)
	}
}
