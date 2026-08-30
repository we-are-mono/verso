// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestDecodeAndRenderConditions(t *testing.T) {
	in := []byte(`{"type":"conditions","label":"Conditions","help":"Combined with and.","items":[` +
		`{"key":"dest_port","label":"Destination ports","active":true,"children":[` +
		`{"type":"list","name":"dest_port","label":"Included ports","items":["53","67"]}]},` +
		`{"key":"rate","label":"Rate limit","children":[` +
		`{"type":"field","name":"limit","label":"Rate","value":"1000"}]}` +
		`]}`)
	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	c, ok := w.(*Conditions)
	if !ok || len(c.Items) != 2 || !c.Items[0].Active {
		t.Fatalf("decoded conditions wrong: %#v", w)
	}
	got := render(t, newRenderer(t), c)
	for _, want := range []string{
		`data-verso-conditions`, `data-verso-condition="dest_port"`,
		`data-verso-condition-template="rate"`, `name="dest_port"`,
		`value="rate"`, `disabled hidden`, `>Remove</button>`, `Combined with and.`,
		"hover:border-sky-600 hover:text-sky-600", "dark:bg-transparent dark:text-gray-300",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conditions missing %q:\n%s", want, got)
		}
	}
}

func TestDecodeConditionsRejectsDuplicateKeys(t *testing.T) {
	_, err := Decode([]byte(`{"type":"conditions","items":[` +
		`{"key":"rate","label":"Rate"},{"key":"rate","label":"Again"}]}`))
	if err == nil {
		t.Fatal("Decode: want duplicate condition key error")
	}
}
