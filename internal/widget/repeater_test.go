// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestRepeaterDecodeAndRender(t *testing.T) {
	js := `{
		"type":"repeater","config":"network","section_type":"wireguard_wg0","add_label":"Add peer",
		"items":[
			{"section":"cfg01","widget":{"type":"card","title":"Peer: phone","children":[
				{"type":"field","name":"pk","label":"Public key","value":"P1"}]}},
			{"section":"cfg02","widget":{"type":"card","title":"Peer: laptop","children":[]}}
		]
	}`
	w, err := Decode([]byte(js))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rp, ok := w.(*Repeater)
	if !ok {
		t.Fatalf("Decode returned %T, want *Repeater", w)
	}
	if rp.Config != "network" || rp.SectionType != "wireguard_wg0" || len(rp.Items) != 2 {
		t.Fatalf("repeater = %+v", rp)
	}
	if rp.Items[0].Section != "cfg01" || rp.Items[1].Section != "cfg02" {
		t.Errorf("item sections = %q, %q", rp.Items[0].Section, rp.Items[1].Section)
	}

	var b strings.Builder
	if err := newRenderer(t).RenderWithToken(&b, rp, "tok", "", nil); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := normalizeHTML(b.String())
	for _, want := range []string{
		"Peer: phone", "Peer: laptop", // the plugin's item subtrees rendered
		`name="_csrf" value="tok"`, // CSRF threaded to the shell-owned affordances
		`name="_repeater_op" value="remove"`, `name="_repeater_section" value="cfg01"`,
		`name="_repeater_config" value="network"`,
		`name="_repeater_op" value="add"`, `name="_repeater_type" value="wireguard_wg0"`,
		">Add peer</button>", ">Remove</button>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("repeater render missing %q", want)
		}
	}
}

// TestRepeaterUnknownItemWidgetFails: an item widget of an unknown type fails the
// decode loudly rather than vanishing, like any other nested widget.
func TestRepeaterUnknownItemWidgetFails(t *testing.T) {
	js := `{"type":"repeater","config":"network","section_type":"t",` +
		`"items":[{"section":"c1","widget":{"type":"bogus"}}]}`
	if _, err := Decode([]byte(js)); err == nil {
		t.Fatal("Decode: want error for an unknown item widget")
	}
}
