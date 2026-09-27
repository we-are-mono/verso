// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// fakeCatalog is a test translator: a source string with an entry is translated,
// anything else falls back to the source — exactly the i18n.Translator contract.
func fakeCatalog(m map[string]string) func(string) string {
	return func(s string) string {
		if v, ok := m[s]; ok {
			return v
		}
		return s
	}
}

// slLoud is the fixed test catalog used across these tests: a handful of prose
// keys plus the renderer's injected defaults, so both walk-translation and
// default-translation can be asserted.
var slLoud = map[string]string{
	"Devices":      "Naprave",
	"Hostname":     "Ime gostitelja",
	"Save":         "Shrani",
	"Save changes": "Shrani spremembe",
	"Delete rule":  "Izbriši pravilo",
	"Confirm":      "Potrdi",
	"Cancel":       "Prekliči",
	"Details":      "Podrobnosti",
	"Online":       "Povezano",
	"Copy":         "Kopiraj", // a {{ t }} template literal (properties.html.tmpl)
}

// TestTranslateSchemaWalksEveryTextField mirrors the validateSchema walk tests: a
// nested tree is translated in place, prose fields become their catalog values and
// machine fields (names, ids) are left verbatim.
func TestTranslateSchemaWalksEveryTextField(t *testing.T) {
	tr := fakeCatalog(slLoud)
	tree := &Card{
		Title: "Devices",
		Children: []Widget{
			&Form{Submit: "Save", Fields: []Widget{
				&Field{Name: "host", Label: "Hostname", Datatype: "hostname"},
			}},
			&Table{
				Columns: []TableColumn{{Label: "Devices", Kind: "name"}, {Kind: "status"}},
				Rows: []TableRow{{Cells: []TableCell{
					{Text: "Online"}, // a name cell: an identity, even one spelling a word
					{Text: "Online", Variant: "success"},
				}}},
			},
		},
	}
	translateSchema(tree, tr)

	if tree.Title != "Naprave" {
		t.Errorf("card title = %q, want translated", tree.Title)
	}
	form := tree.Children[0].(*Form)
	if form.Submit != "Shrani" {
		t.Errorf("form submit = %q, want translated", form.Submit)
	}
	field := form.Fields[0].(*Field)
	if field.Label != "Ime gostitelja" {
		t.Errorf("field label = %q, want translated", field.Label)
	}
	if field.Name != "host" || field.Datatype != "hostname" {
		t.Errorf("machine fields must not be translated: name=%q datatype=%q", field.Name, field.Datatype)
	}
	table := tree.Children[1].(*Table)
	if table.Columns[0].Label != "Naprave" {
		t.Errorf("column label = %q, want translated", table.Columns[0].Label)
	}
	if got := table.Rows[0].Cells[0].Text; got != "Online" {
		t.Errorf("name cell = %q, must stay verbatim (identity column)", got)
	}
	if got := table.Rows[0].Cells[1].Text; got != "Povezano" {
		t.Errorf("status cell = %q, want translated", got)
	}
}

// TestTranslateSkipsMachineContent pins the typography contract at the walk: a
// machine-kind column's Text/Sub, an overflow cell beyond the declared columns,
// and a Mono or Chip property value all stay verbatim, while action prose in
// the same cells and plain property values still translate. A machine string
// that happens to collide with a catalog key ("Online" as a device's name) must
// not come back as words.
func TestTranslateSkipsMachineContent(t *testing.T) {
	tr := fakeCatalog(map[string]string{
		"Online": "Povezano", "Remove": "Odstrani", "Address": "Naslov",
		"br0": "MISTRANSLATED", "Receiving light": "Sprejema svetlobo",
	})

	table := &Table{
		Columns: []TableColumn{{Kind: "mono"}, {Kind: "num"}, {Kind: "text"}},
		Rows: []TableRow{{Cells: []TableCell{
			{Text: "br0", Sub: "Online", Button: "Remove", Actions: []TableRowAct{{Title: "Remove", Name: "Online", Value: "br0"}}},
			{Text: "Online"},
			{Text: "Online"},
			{Text: "br0"}, // overflow: no declared column, stays verbatim
		}}},
	}
	translateSchema(table, tr)
	cells := table.Rows[0].Cells
	if cells[0].Text != "br0" || cells[0].Sub != "Online" {
		t.Errorf("mono cell = %q/%q, must stay verbatim", cells[0].Text, cells[0].Sub)
	}
	if cells[0].Button != "Odstrani" {
		t.Errorf("button in a mono column = %q, want translated", cells[0].Button)
	}
	if action := cells[0].Actions[0]; action.Title != "Odstrani" || action.Name != "Online" || action.Value != "br0" {
		t.Errorf("action titles translate; submission targets stay verbatim: %+v", action)
	}
	if cells[1].Text != "Online" {
		t.Errorf("num cell = %q, must stay verbatim", cells[1].Text)
	}
	if cells[2].Text != "Povezano" {
		t.Errorf("text cell = %q, want translated", cells[2].Text)
	}
	if cells[3].Text != "br0" {
		t.Errorf("overflow cell = %q, must stay verbatim", cells[3].Text)
	}

	props := &Properties{Items: []Property{
		{Label: "Address", Value: "br0", Mono: true},
		{Label: "Address", Value: "Online", Chip: true},
		{Label: "Address", Value: "Receiving light"},
		{Label: "Address", Value: "Online", Verbatim: true},
	}}
	translateSchema(props, tr)
	if props.Items[0].Value != "br0" || props.Items[1].Value != "Online" {
		t.Errorf("mono/chip values = %q/%q, must stay verbatim", props.Items[0].Value, props.Items[1].Value)
	}
	if props.Items[0].Label != "Naslov" {
		t.Errorf("mono row label = %q, want translated", props.Items[0].Label)
	}
	if props.Items[2].Value != "Sprejema svetlobo" {
		t.Errorf("prose value = %q, want translated", props.Items[2].Value)
	}
	if props.Items[3].Value != "Online" {
		t.Errorf("verbatim value = %q, must stay verbatim", props.Items[3].Value)
	}
}

// TestTranslateHonorsVerbatimDeclarations covers the remaining dual-use fields:
// a Stat's value, a Section's meta, and a row drawer's title each hold words in
// one page and data in another, so each carries its own verbatim declaration.
func TestTranslateHonorsVerbatimDeclarations(t *testing.T) {
	tr := fakeCatalog(map[string]string{"Online": "Povezano", "up to date": "posodobljeno"})

	measured := &Stat{Label: "Devices", Value: "Online", Verbatim: true}
	prose := &Stat{Label: "Devices", Value: "Online"}
	translateSchema(measured, tr)
	translateSchema(prose, tr)
	if measured.Value != "Online" {
		t.Errorf("verbatim stat value = %q, must stay verbatim", measured.Value)
	}
	if prose.Value != "Povezano" {
		t.Errorf("prose stat value = %q, want translated", prose.Value)
	}

	composed := &Section{Title: "Devices", Meta: "up to date", MetaVerbatim: true}
	worded := &Section{Title: "Devices", Meta: "up to date"}
	translateSchema(composed, tr)
	translateSchema(worded, tr)
	if composed.Meta != "up to date" {
		t.Errorf("verbatim meta = %q, must stay verbatim", composed.Meta)
	}
	if worded.Meta != "posodobljeno" {
		t.Errorf("prose meta = %q, want translated", worded.Meta)
	}

	table := &Table{
		Columns: []TableColumn{{Kind: "text"}},
		Rows: []TableRow{
			{Cells: []TableCell{{}}, Drawer: &RowDrawer{Title: "Online", Verbatim: true}},
			{Cells: []TableCell{{}}, Drawer: &RowDrawer{Title: "Online"}},
		},
	}
	translateSchema(table, tr)
	if got := table.Rows[0].Drawer.Title; got != "Online" {
		t.Errorf("verbatim drawer title = %q, must stay verbatim", got)
	}
	if got := table.Rows[1].Drawer.Title; got != "Povezano" {
		t.Errorf("prose drawer title = %q, want translated", got)
	}

	// A cell composed and localized where it was made (a relative time) is
	// not looked up again: its words are already the reader's.
	cells := &Table{
		Columns: []TableColumn{{Kind: "text"}},
		Rows: []TableRow{
			{Cells: []TableCell{{Text: "Online", Sub: "Online", Verbatim: true}}},
			{Cells: []TableCell{{Text: "Online", Sub: "Online"}}},
		},
	}
	translateSchema(cells, tr)
	if got := cells.Rows[0].Cells[0]; got.Text != "Online" || got.Sub != "Online" {
		t.Errorf("verbatim cell = %q / %q, must stay verbatim", got.Text, got.Sub)
	}
	if got := cells.Rows[1].Cells[0].Text; got != "Povezano" {
		t.Errorf("prose cell = %q, want translated", got)
	}

	// Likewise a field's help the shell composed around data after
	// translating it (a board's model in a sentence).
	filled := &Field{Label: "Devices", Help: "Online", HelpVerbatim: true}
	plain := &Field{Label: "Devices", Help: "Online"}
	translateSchema(filled, tr)
	translateSchema(plain, tr)
	if filled.Help != "Online" {
		t.Errorf("verbatim field help = %q, must stay verbatim", filled.Help)
	}
	if plain.Help != "Povezano" {
		t.Errorf("prose field help = %q, want translated", plain.Help)
	}

	// And a dialog's busy line composed the same way.
	busy := &Modal{BusyBody: "Online", BusyBodyVerbatim: true}
	idle := &Modal{BusyBody: "Online"}
	translateSchema(busy, tr)
	translateSchema(idle, tr)
	if busy.BusyBody != "Online" {
		t.Errorf("verbatim busy body = %q, must stay verbatim", busy.BusyBody)
	}
	if idle.BusyBody != "Povezano" {
		t.Errorf("prose busy body = %q, want translated", idle.BusyBody)
	}
}

// TestTranslateSchemaFallsBackToSource pins the guarantee that an untranslated key
// keeps its English source, so a partial catalog never blanks a field.
func TestTranslateSchemaFallsBackToSource(t *testing.T) {
	tr := fakeCatalog(slLoud)
	card := &Card{Title: "Not in the catalog"}
	translateSchema(card, tr)
	if card.Title != "Not in the catalog" {
		t.Errorf("missing key must fall back to source, got %q", card.Title)
	}
}

// TestRenderTranslatesInjectedDefaults proves the render-time defaults (Form's
// "Save", Confirm's "Confirm"/"Cancel", the table drawer's "Details") are localized
// through the render handle, not printed raw — the seam that has no struct field
// for the walk to reach.
func TestRenderTranslatesInjectedDefaults(t *testing.T) {
	tr := fakeCatalog(slLoud)
	r := newRenderer(t)

	var form strings.Builder
	if err := r.RenderWithToken(&form, &Form{Fields: []Widget{&Field{Name: "h", Label: "Hostname"}}}, "", "", tr); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	if !strings.Contains(form.String(), ">Shrani spremembe<") {
		t.Errorf("form default submit not localized: %s", form.String())
	}

	// The confirm's default is its act's own name, so it reads in the
	// reader's language as the act does.
	var confirm strings.Builder
	if err := r.RenderWithToken(&confirm, &Confirm{Trigger: "Delete rule", Message: "y"}, "", "", tr); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	for _, want := range []string{">Izbriši pravilo<", ">Prekliči<"} {
		if !strings.Contains(confirm.String(), want) {
			t.Errorf("confirm default label not localized (%q): %s", want, confirm.String())
		}
	}
}

// TestRenderEnglishWhenNoTranslator confirms the English path: a nil translator
// skips the walk and prints the English defaults, unchanged.
func TestRenderEnglishWhenNoTranslator(t *testing.T) {
	r := newRenderer(t)
	var b strings.Builder
	if err := r.RenderWithToken(&b, &Form{Fields: []Widget{&Field{Name: "h", Label: "Hostname"}}}, "", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	got := b.String()
	if !strings.Contains(got, ">Save changes<") || strings.Contains(got, "Shrani") {
		t.Errorf("English render must keep English defaults: %s", got)
	}
}

// TestTranslatedValuesAreEscaped is the security invariant (ADR-012): a catalog is
// untrusted data, so a translated value carrying markup must reach the browser only
// through html/template autoescaping — never as raw HTML. A malicious translation
// of a walked field and of a template-baked {{ t }} literal must both come out
// escaped.
func TestTranslatedValuesAreEscaped(t *testing.T) {
	evil := fakeCatalog(map[string]string{
		"Devices": `<script>alert(1)</script>`,    // a walked struct field (Card.Title)
		"Copy":    `<img src=x onerror=alert(2)>`, // a {{ t }} template literal (properties.html.tmpl)
	})
	r := newRenderer(t)
	if err := r.SetLanguages([]string{"xx"}, func(string) func(string) string { return evil }); err != nil {
		t.Fatalf("SetLanguages: %v", err)
	}

	var b strings.Builder
	tree := &Card{Title: "Devices", Children: []Widget{&Properties{Items: []Property{{Label: "x", Value: "y", Copy: true}}}}}
	if err := r.RenderWithToken(&b, tree, "", "xx", evil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	got := b.String()
	// The dangerous forms are LIVE tags — a real <script> element or an <img> whose
	// onerror fires. Escaped text ("&lt;img … onerror=alert(2)&gt;") is inert, so we
	// assert on the raw opening tags, not on substrings that survive escaping.
	if strings.Contains(got, "<script>") || strings.Contains(got, "<img ") {
		t.Fatalf("translated value reached the browser as live markup — XSS: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("walked field's markup not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;img") {
		t.Errorf("template literal's markup not escaped: %s", got)
	}
}

// TestTranslatedMarkdownIsSanitized guards the one seam that skips plain {{.}}
// autoescaping: a translated Markdown field (Text/Raw/Form.Note/Section.Sub) is
// converted by goldmark and wrapped as template.HTML. A hostile catalog value must
// be neutralized by the sanitizing engine, not rendered as live markup.
func TestTranslatedMarkdownIsSanitized(t *testing.T) {
	evil := fakeCatalog(map[string]string{
		"body": `<script>alert(1)</script>[x](javascript:alert(2))`,
	})
	r := newRenderer(t)
	var b strings.Builder
	if err := r.RenderWithToken(&b, &Text{Markdown: "body"}, "", "xx", evil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	got := b.String()
	if strings.Contains(got, "<script>") {
		t.Fatalf("translated markdown emitted a live script tag — XSS: %s", got)
	}
	if strings.Contains(got, "javascript:") {
		t.Errorf("javascript: scheme not stripped from translated markdown: %s", got)
	}
}

// TestPerLanguageTemplateSelection proves the {{ t }} template seam: after
// SetLanguages installs a language's set, a request for that language renders the
// baked-in translation of a template literal, while an English request does not.
func TestPerLanguageTemplateSelection(t *testing.T) {
	r := newRenderer(t)
	tr := fakeCatalog(slLoud)
	if err := r.SetLanguages([]string{"sl"}, func(code string) func(string) string {
		if code == "sl" {
			return tr
		}
		return identityTranslator
	}); err != nil {
		t.Fatalf("SetLanguages: %v", err)
	}

	// properties.html.tmpl carries the {{ t "Copy" }} literal on a copyable row.
	widget := &Properties{Items: []Property{{Label: "x", Value: "y", Copy: true}}}

	var sl strings.Builder
	if err := r.RenderWithToken(&sl, widget, "", "sl", tr); err != nil {
		t.Fatalf("RenderWithToken(sl): %v", err)
	}
	if !strings.Contains(sl.String(), "Kopiraj") || strings.Contains(sl.String(), ">Copy<") {
		t.Errorf("sl render must use the sl template set: %s", sl.String())
	}

	var en strings.Builder
	if err := r.RenderWithToken(&en, widget, "", "", nil); err != nil {
		t.Fatalf("RenderWithToken(en): %v", err)
	}
	if !strings.Contains(en.String(), ">Copy<") || strings.Contains(en.String(), "Kopiraj") {
		t.Errorf("English render must use the English template set: %s", en.String())
	}
}
