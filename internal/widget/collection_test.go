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

// TestCollectionItemsStandOnSpaceAlone: a short set of kept things — keys,
// peers — is a list of identities, each its name over its detail in the
// machine's own type, separated by the page's space and not by rules.
func TestCollectionItemsStandOnSpaceAlone(t *testing.T) {
	got := renderCollection(t, keyCollection(), "/system/access?plugin=system")
	for _, want := range []string{
		`>demo@laptop</p>`, `>SHA256:UPedI7ax</p>`, "font-mono",
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
		`class="flex flex-wrap items-start justify-between gap-x-6 py-2.5">`,
		`<p class="font-mono text-base leading-5 font-medium wrap-anywhere text-ink">demo@laptop</p>`,
		`<div class="-my-1 shrink-0">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("an item is not whole cells of the grid, want %s in:\n%s", want, got)
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
		"border-crimson-line bg-crimson-soft",
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
	dress := "group/act relative inline-flex h-7 shrink-0 cursor-pointer items-center gap-1.5 rounded-xs border border-rule-strong bg-transparent text-sm font-medium whitespace-nowrap text-meta transition-colors hover:border-sand-5 hover:bg-rule hover:text-ink"
	for name, got := range map[string]string{"add": add, "icon link": withIcon, "plain link": plain} {
		if !strings.Contains(got, dress) {
			t.Errorf("%s does not wear the subsection act's dress:\n%s", name, got)
		}
	}
	if !strings.Contains(add, dress+" focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-denim verso-press pl-2 pr-2.5") ||
		!strings.Contains(withIcon, "pl-2 pr-2.5") || !strings.Contains(plain, " px-2.5") {
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
		`<div data-verso-slot class="-mt-px -ml-px flex w-[calc(100%_+_1px)] items-center gap-3 rounded-xs border border-dashed border-rule-strong bg-quiet px-4 pt-2.5 pb-2.25 leading-5">`,
		`<span aria-hidden="true" class="size-1.5 shrink-0 rounded-[1px] border border-faint"></span>`,
		`<p class="min-w-0 flex-1 text-sm leading-5 text-meta">No keys are authorized.</p>`,
		`x-data="confirm" @keydown.escape="escape" class="pt-6.5 pb-1.5"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
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
	if strings.Contains(got, "data-verso-slot") || !strings.Contains(got, `x-data="confirm" @keydown.escape="escape" class="pt-1.5 pb-1.5"`) {
		t.Errorf("a set with items adds from its foot:\n%s", got)
	}
}
