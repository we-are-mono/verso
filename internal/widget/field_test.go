// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strconv"
	"strings"
	"testing"
)

func choiceOptions(count int) []Option {
	options := make([]Option, count)
	for i := range options {
		options[i] = Option{Value: strconv.Itoa(i), Label: "Choice " + strconv.Itoa(i)}
	}
	return options
}

func TestSingleChoiceCutoff(t *testing.T) {
	r := newRenderer(t)
	for _, count := range []int{0, 1, RadioOptionLimit, RadioOptionLimit + 1} {
		for _, style := range []string{"", "segmented"} {
			t.Run(strconv.Itoa(count)+"/"+style, func(t *testing.T) {
				got := render(t, r, &Field{Name: "policy", Label: "Policy", Kind: "select", Style: style,
					Value: "0", Options: choiceOptions(count)})
				radios := count > 0 && count <= RadioOptionLimit
				if strings.Contains(got, `role="radiogroup"`) != radios || strings.Contains(got, "<select") == radios {
					t.Fatalf("wrong control for %d choices (limit %d):\n%s", count, RadioOptionLimit, got)
				}
				if radios {
					if strings.Count(got, `type="radio" name="policy"`) != count || strings.Count(got, " checked") != 1 {
						t.Errorf("each radio must share the field name, with exactly one selected:\n%s", got)
					}
					if strings.Contains(got, `class="sr-only"`) || strings.Contains(got, `data-verso-control-measure="choice"`) {
						t.Errorf("radios must remain visible and free of dropdown sizing:\n%s", got)
					}
				} else if strings.Count(got, "<option ") != count || !strings.Contains(got, `data-verso-control-measure="choice"`) {
					t.Errorf("dropdown must retain its full option set and intrinsic width:\n%s", got)
				}
			})
		}
	}
}

// TestRadiosStandACellAndAHalfApart: a line is too tight for a run of
// choices and a field's height too loose, so each is 28px: its 20px line and
// 4px over and under. The run starts straight under the field's label, so a
// run of any count is a multiple of 4px.
func TestRadiosStandACellAndAHalfApart(t *testing.T) {
	if RadioOptionLimit < 2 {
		t.Skip("radios disabled by the shared policy")
	}
	got := render(t, newRenderer(t), &Field{Name: "policy", Label: "Policy", Kind: "select", Value: "0", Options: choiceOptions(2)})
	if !strings.Contains(got, `class="flex min-w-0 flex-col items-start">`) {
		t.Errorf("the run stands straight under the label:\n%s", got)
	}
	want := `<label class="flex max-w-full cursor-pointer items-center gap-2 py-1 `
	if strings.Count(got, want) != 2 {
		t.Errorf("each radio is 28px, its 20px line and 4px over and under, want %q:\n%s", want, got)
	}
}

func TestRadiosPreserveSingleSelectDefaults(t *testing.T) {
	if RadioOptionLimit == 0 {
		t.Skip("radios disabled by the shared policy")
	}
	r := newRenderer(t)
	options := choiceOptions(RadioOptionLimit)
	// Include an explicit empty choice; it must win over the first option when
	// Value is empty. Otherwise native selects fall back to their first option.
	options[len(options)-1].Value = ""
	for _, tc := range []struct{ name, value, want string }{
		{"current value", "0", options[0].Value},
		{"empty choice", "", ""},
		{"unavailable value", "removed", options[0].Value},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, r, &Field{Name: "policy", Label: "Policy", Kind: "select", Value: tc.value, Options: options})
			if strings.Count(got, " checked") != 1 || !strings.Contains(got, `value="`+tc.want+`" checked`) {
				t.Errorf("radio selection must match a native single select:\n%s", got)
			}
		})
	}
	got := render(t, r, &Field{Name: "policy", Kind: "select", Options: choiceOptions(RadioOptionLimit)})
	if !strings.Contains(got, `value="0" checked`) {
		t.Errorf("an unset value must keep the first-option default:\n%s", got)
	}
}

// A choice that reshapes its form says so on the control that changes, whether
// it is drawn as a dropdown or as radios, so the shell can ask for the form
// again when it does. A choice that does not reshape says nothing.
func TestReshapingChoiceMarksItsControl(t *testing.T) {
	r := newRenderer(t)
	for _, count := range []int{RadioOptionLimit, RadioOptionLimit + 1} {
		got := render(t, r, &Field{Name: "kind", Label: "Type", Kind: "select", Reshapes: true, Options: choiceOptions(count)})
		radios := count <= RadioOptionLimit
		marked := `<select id="kind" name="kind" data-verso-reshape`
		if radios {
			marked = `<div role="radiogroup" data-verso-reshape`
		}
		if strings.Count(got, "data-verso-reshape ") != 1 || !strings.Contains(got, marked) {
			t.Errorf("%d choices: the reshaping control must carry the mark once, on %q:\n%s", count, marked, got)
		}
		// The form it asks for takes a moment: beside the choice stands the
		// waiting mark, hidden until the choice is made.
		if !strings.Contains(got, `<span data-verso-reshape-wait hidden`) || strings.Count(got, "data-verso-wait ") != 1 {
			t.Errorf("%d choices: the reshaping control has no waiting mark beside it:\n%s", count, got)
		}
		plain := render(t, r, &Field{Name: "kind", Label: "Type", Kind: "select", Options: choiceOptions(count)})
		if strings.Contains(plain, "data-verso-reshape") || strings.Contains(plain, "data-verso-wait") {
			t.Errorf("%d choices: a choice that does not reshape is unmarked:\n%s", count, plain)
		}
	}
}

// A hidden field that names an autocomplete purpose is an account a password
// form belongs to: present for the password manager, never drawn. A plain
// hidden carrier stays a hidden input.
func TestHiddenFieldWithAutocompleteNamesTheAccount(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{Name: "username", Kind: "hidden", Value: "root", Autocomplete: "username"})
	want := `<input type="text" name="username" value="root" autocomplete="username" readonly hidden>`
	if got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	plain := render(t, r, &Field{Name: "section", Kind: "hidden", Value: "ssh"})
	if plain != `<input type="hidden" name="section" value="ssh">` {
		t.Errorf("a plain carrier changed: %s", plain)
	}
}

// A number-measured field (a port, an MTU, a VLAN id) asks a phone for the
// digit pad; any other field keeps the full keyboard.
func TestNumberFieldsAskForTheDigitPad(t *testing.T) {
	r := newRenderer(t)
	for _, f := range []*Field{{Name: "p", Key: "Port"}, {Name: "q", Datatype: "port"}, {Name: "m", Key: "mtu"}, {Name: "cache", Key: "cachesize"}, {Name: "leases", Key: "dhcpleasemax"}} {
		if got := render(t, r, f); !strings.Contains(got, `inputmode="numeric"`) {
			t.Errorf("%s: want the digit pad:\n%s", f.Name, got)
		}
	}
	if got := render(t, r, &Field{Name: "hostname", Key: "hostname"}); strings.Contains(got, "inputmode") {
		t.Errorf("a words field keeps the full keyboard:\n%s", got)
	}
}

// TestRenderFieldChecks: the checks kind is membership in a known set — every
// box shares the field's name (posting multi-value, the list contract), the
// current values come back checked, and the rest don't.
func TestRenderFieldChecks(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{
		Name: "network", Label: "Networks", Kind: "checks",
		Values: []string{"lan", "lan2"},
		Options: []Option{
			{Value: "lan", Label: "lan"}, {Value: "lan2", Label: "lan2"}, {Value: "guest", Label: "guest"},
		},
	})
	if strings.Count(got, `<input type="checkbox" name="network"`) != 3 {
		t.Errorf("all boxes must share the field name:\n%s", got)
	}
	if strings.Count(got, " checked") != 2 {
		t.Errorf("exactly the current values should be checked:\n%s", got)
	}
	for _, want := range []string{`data-verso-change-name="network"`, `data-verso-change-label="Networks"`, `type="checkbox"`, ">Networks<", "guest"} {
		if !strings.Contains(got, want) {
			t.Errorf("checks field missing %q:\n%s", want, got)
		}
	}
	// A set of boxes is read down, one option a row, as a set of radios is:
	// options of any length start at the same edge rather than scattering
	// across a grid's columns, each on a 20px line of the notebook's grid.
	if strings.Contains(got, "grid-cols") {
		t.Errorf("a set of boxes is one column, not a grid:\n%s", got)
	}
	if !strings.Contains(got, `class="flex min-w-0 flex-col items-start"`) || !strings.Contains(got, `<label class="flex min-h-6 max-w-full cursor-pointer items-center gap-2 text-sm leading-6 text-body">`) {
		t.Errorf("a set of boxes takes the radio set's rows:\n%s", got)
	}
}

func TestRenderFieldDateTimeLocal(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "datetime", Label: "Date and time", Kind: "datetime-local",
		Value: "2026-08-29T22:14:08",
	})
	for _, want := range []string{
		`type="datetime-local"`, `step="1"`, `value="2026-08-29T22:14:08"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("datetime-local field missing %q:\n%s", want, got)
		}
	}
}

// TestRenderFieldTip: a label carrying an explanation becomes the thing you
// hover — help cursor, its own tab stop, and the control pointed at it — and the
// tooltip ends on the option and what reads it.
func TestRenderFieldTip(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "duid", Label: "DUID", Value: "", Key: "duid", Source: "dhcp host",
		Tip:  "A DHCPv6 client identifies itself by a DUID, not by its MAC.",
		Help: "Match a DHCPv6 client by DUID instead of MAC.",
	})
	for _, want := range []string{
		"cursor-help", `tabindex="0"`, `aria-describedby="duid-tip"`,
		`id="duid-tip"`, `role="tooltip"`,
		// The hook the stylesheet reveals the tip from and verso-page.js
		// places it by.
		"data-verso-tip",
		"A DHCPv6 client identifies itself by a DUID, not by its MAC.",
		"duid · dhcp host",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("explained field missing %q:\n%s", want, got)
		}
	}
	// The row is one line and its control. An explanation lives on the label
	// that raises it, never as a second line under the control: a sentence
	// per field pushes the rows apart until a form of eight settings reads as
	// a page of prose. Tip is the deliberate sentence, so it wins over Help.
	if strings.Contains(got, "Match a DHCPv6 client by DUID instead of MAC.") {
		t.Errorf("help must not take a line of its own beside a tip:\n%s", got)
	}
}

// TestRenderFieldHelpRaised: a plugin that only wrote Help still gets the
// explanation raised onto the label — the same sentence, said where it belongs,
// without every plugin having to rename the field it already fills in.
func TestRenderFieldHelpRaised(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "leasetime", Label: "Lease time", Key: "leasetime",
		Help: "How long a client may keep an address before it asks again.",
	})
	for _, want := range []string{
		"data-verso-tip", `id="leasetime-tip"`, `role="tooltip"`,
		"How long a client may keep an address before it asks again.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("help-only field missing %q:\n%s", want, got)
		}
	}
}

// TestRenderFieldWithoutTip: a label with nothing to explain stays inert — no
// hover affordance, no tab stop of its own, no tooltip.
func TestRenderFieldWithoutTip(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "name", Label: "Name", Key: "name"})
	for _, unwanted := range []string{"data-verso-tip", "cursor-help", `tabindex="0"`, "role=\"tooltip\""} {
		if strings.Contains(got, unwanted) {
			t.Errorf("plain field should not carry %q:\n%s", unwanted, got)
		}
	}
}

// TestFieldTipFooterHalves: the footer is whichever halves it has, so an option
// named without a reader still places itself and neither renders a bare dot.
func TestFieldTipFooterHalves(t *testing.T) {
	for _, tc := range []struct{ key, source, want string }{
		{"duid", "dhcp host", "duid · dhcp host"},
		{"duid", "", "duid"},
		{"", "dhcp host", "dhcp host"},
		{"", "", ""},
	} {
		f := &Field{Name: "duid", Tip: "…", Key: tc.key, Source: tc.source}
		if got := f.TipView().Footer; got != tc.want {
			t.Errorf("footer for key %q source %q = %q, want %q", tc.key, tc.source, got, tc.want)
		}
	}
}

func TestRenderFieldCurrentPasswordAutocomplete(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "current_password", Label: "Current password", Kind: "password", Autocomplete: "current-password",
	})
	if !strings.Contains(got, `autocomplete="current-password"`) {
		t.Errorf("current password autocomplete missing:\n%s", got)
	}
}

// An empty box never shows what could pass for a value it holds: the value
// the daemon falls back to reads "default 150", a value of the field's kind
// reads "e.g. 1.1.1.1", worded by the shell in the reader's language. A
// plugin's own placeholder still stands as given.
func TestAnEmptyBoxSaysWhatItsHintIs(t *testing.T) {
	r := newRenderer(t)
	for _, tc := range []struct {
		w    Widget
		want string
	}{
		{&Field{Name: "cachesize", Label: "Cache size", Kind: "text", Default: "150"}, `placeholder="default 150"`},
		{&Field{Name: "domain", Label: "Local domain", Kind: "text", Example: "lan"}, `placeholder="e.g. lan"`},
		{&Field{Name: "q", Kind: "text", Placeholder: "Package name", Example: "htop"}, `placeholder="Package name"`},
		{&List{Name: "upstream", Label: "Upstream servers", Kind: "text", Style: "rows", Example: "1.1.1.1"}, `placeholder="e.g. 1.1.1.1"`},
	} {
		if got := render(t, r, tc.w); !strings.Contains(got, tc.want) {
			t.Errorf("missing %s in: %s", tc.want, got)
		}
	}
}

// A forward's address and ports are typed back verbatim, so their boxes are
// mono at a machine measure: the address's, and a short one for a port that
// may be a range (8000-8010), which a numeric keypad could not type.
func TestAForwardsAddressAndPortsAreMachineStrings(t *testing.T) {
	for key, want := range map[string]string{"dest_ip": "address", "dest_port": "short", "src_dport": "short"} {
		f := &Field{Name: key, Kind: "text", Key: key}
		if got := f.Measure(); got != want || f.Words() {
			t.Errorf("%s measure = %q words = %v, want %q in mono", key, got, f.Words(), want)
		}
	}
}

// A text field that carries options suggests them as it is typed into: the box
// is a combobox driving a listbox of the values, each with what it is, and it
// still takes whatever is typed — a suggestion, never a restriction. An option
// whose label is its value says it once.
func TestATextFieldWithOptionsSuggestsThem(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "dest_ip", Label: "Destination address", Kind: "text",
		Options: []Option{{Value: "10.0.0.30", Label: "nas"}, {Value: "10.0.0.31", Label: "10.0.0.31"}}})
	for _, want := range []string{
		`name="dest_ip"`,
		`data-verso-suggest-input role="combobox" aria-expanded="false" aria-autocomplete="list" aria-controls="dest_ip-options" autocomplete="off"`,
		`<div id="dest_ip-options" data-verso-suggest-menu role="listbox" aria-labelledby="dest_ip-label" hidden`,
		`id="dest_ip-option-0" role="option" aria-selected="false" tabindex="-1" data-verso-suggest-option="10.0.0.30" data-verso-suggest-label="nas"`,
		`<span class="font-mono text-base font-medium">10.0.0.30</span><span class="text-sm text-meta">nas</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `<span class="text-sm text-meta">10.0.0.31</span>`) {
		t.Errorf("an option whose label is its value should say it once:\n%s", got)
	}
	if plain := render(t, newRenderer(t), &Field{Name: "name", Label: "Name", Kind: "text"}); strings.Contains(plain, "combobox") {
		t.Errorf("a text field with nothing to suggest is a plain box:\n%s", plain)
	}
}
