// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"reflect"
	"strings"
	"testing"
)

// card is the untitled card the shell stands a loose run in.
func card(children ...Widget) *Section { return &Section{Hairline: true, Children: children} }

// TestDrawerLooseFieldsStandInACard: a drawer's form that opens on a run of
// fields stands that run in an untitled card, as a page form's untitled run
// does, so no field stands loose on the sand. Its titled sections are cards
// already and its configuration card is a card of its own; both stay as the
// plugin sent them.
func TestDrawerLooseFieldsStandInACard(t *testing.T) {
	name := &Field{Name: "name", Label: "Name"}
	mtu := &Field{Name: "mtu", Label: "MTU"}
	advanced := &Section{Title: "Advanced", Hairline: true, Children: []Widget{&Field{Name: "metric"}}}
	file := &Code{Label: "/etc/config/network", Live: true, Grammar: "uci", Value: "config interface 'lan'"}
	form := &Form{Submit: "Save interface", Fields: []Widget{name, mtu, advanced, file}}

	got := onCards([]Widget{form})

	want := []Widget{&Form{Submit: "Save interface", Fields: []Widget{card(name, mtu), advanced, file}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loose fields did not stand in one card:\n got %#v\nwant %#v", got[0].(*Form).Fields, want[0].(*Form).Fields)
	}
	if len(form.Fields) != 4 {
		t.Error("standing the form on cards rewrote the plugin's own tree")
	}
}

// TestDrawerOpeningBlockIsACard: the untitled block a drawer opens on (a
// forward's "Forward is active" and Name, sent flush because the chrome's
// hairline was its rule) is the drawer's first card, with no header.
func TestDrawerOpeningBlockIsACard(t *testing.T) {
	opening := &Section{Flush: true, Children: []Widget{&Field{Name: "enabled"}, &Field{Name: "name"}}}

	got := onCards([]Widget{opening})

	want := []Widget{&Section{Hairline: true, Children: opening.Children}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("opening block is not a card:\n got %#v\nwant %#v", got, want)
	}
	if !opening.Flush || opening.Hairline {
		t.Error("the plugin's own section was rewritten")
	}
}

// TestDrawerSandKeepsItsNoticesAndActs: a notice about the whole drawer and an
// act on the object (a removal's confirmation) stand on the sand between the
// cards, as a page's own notice and Save do; only what lies between them goes
// into cards. A run of hidden carriers alone draws nothing, so it gets no card.
func TestDrawerSandKeepsItsNoticesAndActs(t *testing.T) {
	notice := &Callout{Body: "This interface carries your connection."}
	gateway := &Field{Name: "gateway"}
	hidden := &Field{Kind: "hidden", Name: "id", Value: "cfg03"}
	remove := &Form{Fields: []Widget{hidden, &Confirm{Trigger: "Delete route"}}}

	got := onCards([]Widget{notice, gateway, remove})

	want := []Widget{notice, card(gateway), &Form{Fields: []Widget{hidden, &Confirm{Trigger: "Delete route"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sand lost its notice or act:\n got %#v\nwant %#v", got, want)
	}
}

// TestDrawerStackStandsOnCards: a tab's body that is a stack (a device's
// facts over its Usage section) stands each loose block of it in a card, so
// the facts are a card as Usage is.
func TestDrawerStackStandsOnCards(t *testing.T) {
	facts := &Properties{Items: []Property{{Label: "MAC address", Value: "52:75:3b:72:32:b7"}}}
	usage := &Section{Title: "Usage", Hairline: true}

	got := onCards([]Widget{&Stack{Children: []Widget{facts, usage}}})

	want := []Widget{&Stack{Children: []Widget{card(facts), usage}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stack's facts are not a card:\n got %#v\nwant %#v", got, want)
	}
}

// TestDrawerConfigurationCardLeavesTheCardHoldingIt: a reading whose section
// ends on the file it writes (a rule's Match, its preview and the carriers
// after its fields) stands the file after that card as a card of its own,
// since no card holds a card. The carriers stay where the plugin put them.
func TestDrawerConfigurationCardLeavesTheCardHoldingIt(t *testing.T) {
	name := &Field{Name: "name"}
	file := &Code{Label: "/etc/config/firewall", Live: true, Grammar: "uci", Value: "config rule"}
	carried := &Field{Kind: "hidden", Name: "tab", Value: "match"}
	match := &Section{Title: "What this rule matches", Flush: true, Children: []Widget{name, file, carried}}

	got := onCards([]Widget{match})

	want := []Widget{&Section{Title: "What this rule matches", Flush: true, Children: []Widget{name, carried}}, file}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the file stayed in the card that holds it:\n got %#v\nwant %#v", got, want)
	}
	if len(match.Children) != 3 {
		t.Error("the plugin's own section was rewritten")
	}
}

// TestDrawerCardsAreStoodOnce: standing a body on cards twice changes nothing
// the second time, so a tree rendered as a page and then as its open panel
// is not boxed twice.
func TestDrawerCardsAreStoodOnce(t *testing.T) {
	body := []Widget{
		&Section{Flush: true, Children: []Widget{&Field{Name: "enabled"}}},
		&Form{Fields: []Widget{&Field{Name: "name"}, &Code{Label: "/etc/config/firewall"}}},
	}
	once := onCards(body)
	if twice := onCards(once); !reflect.DeepEqual(once, twice) {
		t.Fatalf("second pass changed the cards:\n once %#v\ntwice %#v", once, twice)
	}
}

// TestOpenDrawerRendersItsFieldsInACard: the panel a row opens draws its loose
// fields inside an untitled card's section, and a chooser, a list of choices
// rather than a form, is left as it is.
func TestOpenDrawerRendersItsFieldsInACard(t *testing.T) {
	r := newRenderer(t)
	for _, size := range []string{"", "choices"} {
		d := &RowDrawer{Title: "New route", Open: true, Size: size, Children: []Widget{
			&Form{Fields: []Widget{&Field{Name: "target", Label: "Target"}}},
		}}
		var out strings.Builder
		if _, err := r.RenderOpenPanelWithToken(&out, &ActionBar{Drawer: d}, "", "", nil, Flash{}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		open := strings.Index(html, `<section data-verso-section="ruled" data-verso-headless data-verso-ruled`)
		field := strings.Index(html, `name="target"`)
		if size == "" && (open < 0 || field < open) {
			t.Errorf("the drawer's field does not stand in a card:\n%s", html)
		}
		if size == "choices" && open >= 0 {
			t.Errorf("a chooser was stood on cards:\n%s", html)
		}
	}
}
