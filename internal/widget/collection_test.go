// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func keyCollection() string {
	return `{"type":"collection","empty":"No keys are authorized.",
	  "items":[{"title":"demo@laptop","detail":"SHA256:UPedI7ax",
	    "remove":{"name":"_key_remove","value":"SHA256:UPedI7ax",
	      "confirm":{"trigger":"Remove","icon":"trash-2","title":"Remove this key?","message":"It can no longer sign in.","confirm":"Remove key","cancel":"Not now"}}}],
	  "add":{"label":"Add a key","name":"authorized_key","placeholder":"ssh-ed25519 AAAA… you@laptop","submit":"Add key",
	    "preview":{"type":"code","label":"Reads as","value":"","live":true}}}`
}

func renderCollection(t *testing.T, raw string, action string) string {
	t.Helper()
	w, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	c := w.(*Collection)
	c.Action = action
	return render(t, newRenderer(t), c)
}

// declaresTheSlot fails t unless the slot block draws the empty set's place
// as every set draws it: a dashed Strong Hairline on Quiet Sand, its words in
// Meta on one 24px line 7px after the top border and 8px over the bottom one,
// 12px after the hollow packet, the bottom border's pixel given back.
func declaresTheSlot(t *testing.T) {
	t.Helper()
	declares(t, "slot", ".verso-slot",
		"display: flex", "align-items: center", "gap: calc(var(--spacing) * 3)", "width: 100%",
		"margin-bottom: -1px",
		"padding: calc(var(--spacing) * 1.75) calc(var(--spacing) * 4) calc(var(--spacing) * 2)",
		"border: 1px dashed var(--color-rule-strong)", "border-radius: var(--radius-xs)",
		"background-color: var(--color-quiet)", "@apply type-body")
	declares(t, "slot", ".verso-slot__words",
		"flex: 1 1 0%", "min-width: 0", "color: var(--color-meta)", "@apply type-body")
}

// TestCollectionItemsStandOnSpaceAlone: a short set of kept things — keys,
// peers — is a list of identities, each its name over its detail in the
// machine's own type, separated by the page's space and not by rules.
func TestCollectionItemsStandOnSpaceAlone(t *testing.T) {
	got := renderCollection(t, keyCollection(), "/system/access?plugin=system")
	for _, want := range []string{
		`>demo@laptop</p>`, `>SHA256:UPedI7ax</p>`,
		`<p class="verso-collection__title" data-verso-verbatim>demo@laptop</p>`,
		`<p class="verso-collection__detail" data-verso-verbatim>SHA256:UPedI7ax</p>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	for _, absent := range []string{"border-b", "divide-y", "<table"} {
		if strings.Contains(got, absent) {
			t.Errorf("a collection draws no %s:\n%s", absent, got)
		}
	}
	// Every item, the first included, writes on the notebook's 20px lines
	// with half a cell over and under, so each is whole cells; its act rides
	// the first line, 4px over at either edge.
	for _, want := range []string{
		`<li>`,
		`class="verso-collection__item">`,
		`<div class="verso-collection__words">`,
		`<div class="verso-collection__act">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("an item is not whole cells of the grid, want %s in:\n%s", want, got)
		}
	}
	declares(t, "collection", ".verso-collection__item",
		"display: flex", "flex-wrap: wrap", "align-items: flex-start",
		"justify-content: space-between", "column-gap: calc(var(--spacing) * 6)",
		"padding-block: calc(var(--spacing) * 2)")
	declares(t, "collection", ".verso-collection__words", "flex: 1 1 0%", "min-width: 0")
	declares(t, "collection", ".verso-collection__title",
		"overflow-wrap: anywhere", "color: var(--color-ink)", "@apply type-value")
	declares(t, "collection", ".verso-collection__detail",
		"overflow-wrap: anywhere", "color: var(--color-meta)")
	declares(t, "collection", ".verso-collection__act",
		"flex-shrink: 0", "margin-block: calc(var(--spacing) * -1)")
	css := blockCSS(t, "collection")
	for _, absent := range []string{"border-bottom", "border-top"} {
		if strings.Contains(css, absent) {
			t.Errorf("a collection draws no rules (%s):\n%s", absent, css)
		}
	}
}

// TestCollectionRemovesInPlace: an item's act is the icon on its first line;
// pressed, it asks in place under the item — the one hue of the confirmation
// pattern — and the answer posts the item's own pair to the page's address.
func TestCollectionRemovesInPlace(t *testing.T) {
	got := renderCollection(t, keyCollection(), "/system/access?plugin=system")
	for _, want := range []string{
		`<form method="post" action="/system/access?plugin=system"`,
		`<input type="hidden" name="_key_remove" value="SHA256:UPedI7ax">`,
		`aria-label="Remove demo@laptop"`,
		`x-data="confirm"`,
		`<div data-verso-callout class="verso-notice" data-verso-tone="danger"`,
		">Remove this key?<", ">Remove key</button>", ">Not now</button>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}

// TestCollectionAddsInPlace: the next item is added from the foot of the set:
// a quiet act that unfolds, where it stands, into the box that takes one — set
// in the machine's type — its live reading, and the act that adds it.
func TestCollectionAddsInPlace(t *testing.T) {
	got := renderCollection(t, keyCollection(), "/system/access?plugin=system")
	for _, want := range []string{
		">Add a key</button>",
		`name="authorized_key"`, `placeholder="ssh-ed25519 AAAA… you@laptop"`,
		`autocomplete="off" spellcheck="false"`,
		"data-verso-preview",
		">Add key</button>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "data-verso-open") {
		t.Errorf("the add slot rests folded:\n%s", got)
	}
	refused := strings.Replace(keyCollection(), `"submit":"Add key",`, `"submit":"Add key","value":"ssh-rsa AAAA","error":"Paste one complete SSH public key.",`, 1)
	open := renderCollection(t, refused, "")
	for _, want := range []string{"data-verso-open", `value="ssh-rsa AAAA"`, "Paste one complete SSH public key.", `aria-invalid="true"`} {
		if !strings.Contains(open, want) {
			t.Errorf("a refused paste comes back open, kept, and told why — want %s in:\n%s", want, open)
		}
	}
}

// TestLivePreviewWithNothingToSayTakesNoRoom: a live reading of a form that
// has nothing to read yet is present for the form watcher to replace, but draws
// nothing — an empty box would read as a box to fill.
func TestLivePreviewWithNothingToSayTakesNoRoom(t *testing.T) {
	r := newRenderer(t)
	if got := render(t, r, &Code{Live: true}); !strings.Contains(got, "data-verso-preview hidden") {
		t.Errorf("an empty live reading is hidden:\n%s", got)
	}
	if got := render(t, r, &Code{Live: true, Value: "256 SHA256:x me (ED25519)"}); strings.Contains(got, " hidden") {
		t.Errorf("a reading with something to say shows:\n%s", got)
	}
}

// TestSubsectionActsWearOneDress: the acts on a part of a section — adding a
// key, installing or fetching a certificate — are one control: the 28px
// quiet size, a strong hairline at rest, and the same warming under the
// pointer, whether a set's add slot draws it or a plugin's link does.
func TestSubsectionActsWearOneDress(t *testing.T) {
	r := newRenderer(t)
	add := renderCollection(t, keyCollection(), "")
	withIcon := render(t, r, &Link{Label: "Download", Href: "/x", Style: "act", Icon: "download"})
	plain := render(t, r, &Link{Label: "Make a new one", Href: "/y", Style: "act"})
	dress := `class="verso-button group/act relative" data-verso-size="act"`
	for name, got := range map[string]string{"add": add, "icon link": withIcon, "plain link": plain} {
		if !strings.Contains(got, dress) {
			t.Errorf("%s does not wear the subsection act's dress:\n%s", name, got)
		}
	}
	// The button block sets the tighter inset on an act marked with a glyph.
	if !strings.Contains(add, dress+" data-verso-glyph>") ||
		!strings.Contains(withIcon, dress+" data-verso-glyph>") || !strings.Contains(plain, dress+">") {
		t.Errorf("a leading glyph takes the tighter inset, words alone an even one:\n%s\n%s\n%s", add, withIcon, plain)
	}
}

// TestCollectionSaysWhenEmpty: with nothing kept, the set draws the place its
// first item would take — a dashed Quiet Sand slot marked absent, saying so in
// the words of a state — and the add stands at the foot under it, where it
// stands under the items once there are some.
func TestCollectionSaysWhenEmpty(t *testing.T) {
	empty := `{"type":"collection","empty":"No keys are authorized.","items":[],
	  "add":{"label":"Add a key","name":"authorized_key","submit":"Add key"}}`
	got := renderCollection(t, empty, "")
	for _, want := range []string{
		`<div data-verso-slot class="verso-slot">`,
		`<span aria-hidden="true" class="verso-mark"></span>`,
		`<p class="verso-slot__words">No keys are authorized.</p>`,
		`x-data="confirm" @keydown.escape="escape" class="verso-collection__add" data-verso-first>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// A cell under the empty slot's edge, then the act centred in two.
	declares(t, "collection", ".verso-collection__add[data-verso-first]",
		"padding-block: calc(var(--spacing) * 6) calc(var(--spacing) * 1)")
	declaresTheSlot(t)
	// The slot is a place, not a control: nothing in it answers the pointer.
	slot := got[strings.Index(got, "data-verso-slot"):strings.Index(got, `x-data="confirm"`)]
	if strings.Contains(slot, "hover:") || strings.Contains(slot, "@click") || strings.Contains(slot, "<button") {
		t.Errorf("the empty slot is inert:\n%s", slot)
	}
}

// TestCollectionWithItemsAddsFromTheFoot: once the set holds something, no
// slot is drawn and the add stands under the last item.
func TestCollectionWithItemsAddsFromTheFoot(t *testing.T) {
	got := renderCollection(t, keyCollection(), "")
	if strings.Contains(got, "data-verso-slot") || !strings.Contains(got, `x-data="confirm" @keydown.escape="escape" class="verso-collection__add">`) ||
		strings.Index(got, "verso-collection__add") < strings.LastIndex(got, "verso-collection__item") {
		t.Errorf("a set with items adds from its foot:\n%s", got)
	}
	// The 28px act centred in the 40px under the last item.
	declares(t, "collection", ".verso-collection__add", "padding-block: calc(var(--spacing) * 1.5)")
}
