// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// packet is the chip's packet square in a hue: the app's 6px state mark,
// still (a chip states a fact; it does not mark something live).
func packet(fill string) string {
	return `<span class="size-1.5 shrink-0 rounded-[1px] ` + fill + `" aria-hidden="true"></span>`
}

// TestChipMarkIsNoneAnIconOrThePacket: a chip leads with one of three
// things — nothing, a Lucide icon, or the packet square in the chip's hue
// (hollow when the chip has none) — and never two: an icon wins over the
// packet. The same holds for every chip: a pill, a cited entity, a row's tag.
func TestChipMarkIsNoneAnIconOrThePacket(t *testing.T) {
	r := newRenderer(t)
	for name, tc := range map[string]struct {
		got, want string
	}{
		"pill packet":   {render(t, r, &Badge{Variant: "success", Text: "accept", Dot: true}), packet("bg-green")},
		"pill hollow":   {render(t, r, &Badge{Variant: "neutral", Text: "drop", Dot: true}), packet("border border-faint")},
		"entity packet": {renderChipCell(t, r, TableChip{Label: "lan", Tone: "warning", Dot: true}), packet("bg-marigold")},
		"entity accent": {renderChipCell(t, r, TableChip{Label: "lan", Tone: "accent", Dot: true}), packet("bg-denim")},
		"tag packet":    {renderTagCell(t, r, TableCell{Text: "x", Tag: "new", TagVariant: "danger", TagDot: true}), packet("bg-crimson")},
		"tag neutral":   {renderTagCell(t, r, TableCell{Text: "x", Tag: "new", TagDot: true}), packet("border border-faint")},
	} {
		if !strings.Contains(tc.got, tc.want) {
			t.Errorf("%s: want %s in:\n%s", name, tc.want, tc.got)
		}
	}
	// One mark only: an icon wins over the packet, and nothing means nothing.
	both := render(t, r, &Badge{Variant: "success", Text: "accept", Dot: true, Icon: "check"})
	if strings.Contains(both, "size-1.5 shrink-0 rounded-[1px]") || !strings.Contains(both, lucideIcons["check"]) {
		t.Errorf("an icon and a packet on one chip: the icon wins:\n%s", both)
	}
	if bare := render(t, r, &Badge{Variant: "success", Text: "accept"}); strings.Contains(bare, "rounded-[1px]") || strings.Contains(bare, "<svg") {
		t.Errorf("a chip with no mark leads with nothing:\n%s", bare)
	}
	// A mono chip is one step heavier than a sans one, wherever it stands.
	for name, got := range map[string]string{
		"pill":          render(t, r, &Badge{Variant: "success", Text: "accept"}),
		"entity":        renderChipCell(t, r, TableChip{Label: "lan", Icon: "network"}),
		"entity toned":  renderChipCell(t, r, TableChip{Label: "lan", Tone: "success"}),
		"entity accent": renderChipCell(t, r, TableChip{Label: "lan", Tone: "accent"}),
	} {
		if !strings.Contains(got, chipMonoBox) || strings.Contains(got, "font-normal font-mono") {
			t.Errorf("%s: a mono chip is the 500 box:\n%s", name, got)
		}
	}
	// The packet in a chip is still: the pulse is for something live.
	if live := render(t, r, &Badge{Variant: "success", Text: "accept", Dot: true}); strings.Contains(live, "verso-live-dot") {
		t.Errorf("a chip's packet must not pulse:\n%s", live)
	}
}

func renderChipCell(t *testing.T, r *Renderer, chip TableChip) string {
	t.Helper()
	return render(t, r, &Table{
		Columns: []TableColumn{{Label: "Via", Kind: "entity"}},
		Rows:    []TableRow{{ID: "r", Cells: []TableCell{{Chips: []TableChip{chip}}}}},
	})
}

func renderTagCell(t *testing.T, r *Renderer, cell TableCell) string {
	t.Helper()
	return render(t, r, &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows:    []TableRow{{ID: "r", Cells: []TableCell{cell}}},
	})
}

// TestEveryCitationChipIsOneBox: a chip is one size wherever it stands — the
// masthead's protocol beside IPv4, a choice's config value, a panel tab's
// state — the same box as a table's tag and an interface cited in a row.
// Only the face follows what it says: mono for a string the machine wrote,
// sans for words.
func TestEveryCitationChipIsOneBox(t *testing.T) {
	r := newRenderer(t)
	act, err := DecodeHeadingAct([]byte(`{"label":"Add rule","href":"/x?open=new",
		"drawer":{"title":"New rule","open":true,"closed":"/x",
			"tabs":[{"label":"Match","state":"0 conditions","active":true},{"label":"Action","state":"accept"}],
			"children":[{"type":"text","text":"."}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	tabs := act.Bar()
	for name, tc := range map[string]struct {
		got  string
		want []string
	}{
		"masthead protocol": {
			render(t, r, &Overview{V4Proto: "DHCP", V4: []OverviewFact{{Label: "Address", Value: "172.30.1.171/24"}}}),
			[]string{chipMonoBox + " border-rule bg-quiet text-meta\">dhcp"},
		},
		"choice value": {
			render(t, r, &Link{Style: "choice", Label: "WPA3", Code: "sae", Href: "/x"}),
			[]string{chipMonoBox + " border-rule bg-quiet text-meta\">sae"},
		},
		"panel tab state": {
			render(t, r, tabs),
			[]string{
				chipBox + " border-denim-line bg-denim-soft text-denim-deep\">0 conditions",
				chipBox + " border-rule bg-quiet text-meta\">accept",
			},
		},
	} {
		for _, want := range tc.want {
			if !strings.Contains(tc.got, want) {
				t.Errorf("%s: want %s in:\n%s", name, want, tc.got)
			}
		}
	}
}
