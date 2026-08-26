// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "testing"

func TestDecodeUnknownType(t *testing.T) {
	if _, err := Decode([]byte(`{"type":"nope"}`)); err == nil {
		t.Fatal("Decode: want error for unknown type, got nil")
	}
}

func TestDecodeMissingType(t *testing.T) {
	if _, err := Decode([]byte(`{"columns":[]}`)); err == nil {
		t.Fatal("Decode: want error for missing type, got nil")
	}
}

func TestDecodeCard(t *testing.T) {
	in := []byte(`{"type":"card","title":"System","children":[{"type":"table","columns":[{"label":"A"}],"rows":[{"cells":[{"text":"1"}]}]}]}`)

	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}

	card, ok := w.(*Card)
	if !ok {
		t.Fatalf("Decode: got %T, want *Card", w)
	}
	if card.Title != "System" {
		t.Errorf("Title = %q, want %q", card.Title, "System")
	}
	if len(card.Children) != 1 {
		t.Fatalf("Children = %d, want 1", len(card.Children))
	}
	if _, ok := card.Children[0].(*Table); !ok {
		t.Errorf("child 0 = %T, want *Table", card.Children[0])
	}
}

// TestDecodeCardNested exercises the recursion the whole schema bet rests on: a
// container inside a container. If this decodes, nesting is real, not a leaf trick.
func TestDecodeCardNested(t *testing.T) {
	in := []byte(`{"type":"card","title":"outer","children":[{"type":"card","title":"inner","children":[{"type":"table","columns":[{"label":"A"}],"rows":[]}]}]}`)

	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	outer, ok := w.(*Card)
	if !ok {
		t.Fatalf("Decode: got %T, want *Card", w)
	}
	inner, ok := outer.Children[0].(*Card)
	if !ok {
		t.Fatalf("outer child 0 = %T, want *Card", outer.Children[0])
	}
	if _, ok := inner.Children[0].(*Table); !ok {
		t.Errorf("inner child 0 = %T, want *Table", inner.Children[0])
	}
}

func TestDecodeCardUnknownChildErrors(t *testing.T) {
	in := []byte(`{"type":"card","children":[{"type":"nope"}]}`)
	if _, err := Decode(in); err == nil {
		t.Fatal("Decode: want error for unknown child type, got nil")
	}
}

func TestDecodeCardEmptyChildren(t *testing.T) {
	w, err := Decode([]byte(`{"type":"card","title":"empty"}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c := w.(*Card); len(c.Children) != 0 {
		t.Errorf("Children = %d, want 0", len(c.Children))
	}
}

func TestDecodeForm(t *testing.T) {
	in := []byte(`{"type":"form","submit":"Apply","fields":[
		{"type":"field","name":"hostname","label":"Hostname","kind":"text","value":"OpenWrt","datatype":"hostname"},
		{"type":"list","name":"server","label":"NTP","kind":"text","datatype":"host","items":["a","b"]}
	]}`)

	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	f, ok := w.(*Form)
	if !ok {
		t.Fatalf("got %T, want *Form", w)
	}
	if f.Submit != "Apply" {
		t.Errorf("Submit = %q, want Apply", f.Submit)
	}
	if len(f.Fields) != 2 {
		t.Fatalf("Fields = %d, want 2", len(f.Fields))
	}
	if fld, ok := f.Fields[0].(*Field); !ok || fld.Name != "hostname" || fld.Datatype != "hostname" {
		t.Errorf("field 0 wrong: %+v", f.Fields[0])
	}
	if lst, ok := f.Fields[1].(*List); !ok || lst.Name != "server" || len(lst.Items) != 2 {
		t.Errorf("field 1 wrong: %+v", f.Fields[1])
	}
}

func TestDecodeFieldSelect(t *testing.T) {
	in := []byte(`{"type":"field","name":"tz","label":"Timezone","kind":"select","value":"CET",` +
		`"options":[{"value":"UTC","label":"UTC"},{"value":"CET","label":"Central European"}]}`)

	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	f := w.(*Field)
	if f.Kind != "select" || len(f.Options) != 2 || f.Options[1].Value != "CET" {
		t.Errorf("select decode wrong: %+v", f)
	}
}

// TestDecodeFormNestedInCard is the full shape the hostname plugin emits:
// card > form > field/list. If this decodes, the interactive schema nests.
func TestDecodeFormNestedInCard(t *testing.T) {
	in := []byte(`{"type":"card","title":"General","children":[` +
		`{"type":"form","fields":[{"type":"field","name":"h","label":"H"},` +
		`{"type":"list","name":"s","label":"S","items":["x"]}]}]}`)

	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	card := w.(*Card)
	form, ok := card.Children[0].(*Form)
	if !ok {
		t.Fatalf("card child = %T, want *Form", card.Children[0])
	}
	if _, ok := form.Fields[0].(*Field); !ok {
		t.Errorf("form field 0 = %T, want *Field", form.Fields[0])
	}
	if _, ok := form.Fields[1].(*List); !ok {
		t.Errorf("form field 1 = %T, want *List", form.Fields[1])
	}
}

func TestDecodeFormUnknownFieldErrors(t *testing.T) {
	in := []byte(`{"type":"form","fields":[{"type":"nope"}]}`)
	if _, err := Decode(in); err == nil {
		t.Fatal("Decode: want error for unknown form field type")
	}
}

func TestDecodeRaw(t *testing.T) {
	w, err := Decode([]byte(`{"type":"raw","markdown":"# Hi"}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if rw, ok := w.(*Raw); !ok || rw.Markdown != "# Hi" {
		t.Errorf("raw decode wrong: %+v", w)
	}
}
