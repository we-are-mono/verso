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

// TestRadiosStandAControlTall: each choice is 32px, its 24px line and 4px
// over and under, so a run centres on the name's line as a control does, and
// a run that wraps grows by whole 32px lines.
func TestRadiosStandAControlTall(t *testing.T) {
	if RadioOptionLimit < 2 {
		t.Skip("radios disabled by the shared policy")
	}
	got := render(t, newRenderer(t), &Field{Name: "policy", Label: "Policy", Kind: "select", Value: "0", Options: choiceOptions(2)})
	if !strings.Contains(got, `class="verso-choice__group" data-verso-kind="radios">`) {
		t.Errorf("the run stands across the answer:\n%s", got)
	}
	declares(t, "choice", `.verso-choice__group[data-verso-kind="radios"]`, "display: flex", "flex-wrap: wrap", "align-items: flex-start")
	want := `<label class="verso-choice__option"`
	if strings.Count(got, want) != 2 {
		t.Errorf("each radio is the choice block's option, want %q:\n%s", want, got)
	}
	declares(t, "choice", ".verso-choice__option", "min-height: calc(var(--spacing) * 6)")
	declares(t, "choice", `.verso-choice__option:where(.verso-choice__group[data-verso-kind="radios"] *)`, "padding-block: var(--spacing)")
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
	if !strings.Contains(got, `class="verso-choice__group" data-verso-kind="checks"`) || strings.Count(got, `<label class="verso-choice__option">`) != 3 {
		t.Errorf("a set of boxes takes the radio set's rows:\n%s", got)
	}
	declares(t, "choice", `.verso-choice__group[data-verso-kind="checks"]`, "flex-direction: column", "align-items: flex-start")
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

// TestAFieldsHelpIsItsDescription: what a setting does is written under its
// name, in the question, not raised onto the name as a tooltip (form-fields):
// the help, then the tip where a plugin wrote both, so neither is dropped. The
// control is described by it, so a screen reader reads it on focus.
func TestAFieldsHelpIsItsDescription(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "duid", Label: "DUID", Value: "", Key: "duid", Source: "dhcp host",
		Tip:  "A DHCPv6 client identifies itself by a DUID, not by its MAC.",
		Help: "Match a DHCPv6 client by DUID instead of MAC.",
	})
	desc := `<p id="duid-desc" data-verso-field-desc class="verso-field__desc"><span>Match a DHCPv6 client by DUID instead of MAC. A DHCPv6 client identifies itself by a DUID, not by its MAC.</span></p>`
	at := strings.Index(got, desc)
	if at < 0 || at > strings.Index(got, "verso-field__answer") {
		t.Errorf("the help and the tip are not the description under the name:\n%s", got)
	}
	if !strings.Contains(got, `aria-describedby="duid-desc"`) {
		t.Errorf("the control is not described by its description:\n%s", got)
	}
	for _, gone := range []string{"data-verso-tip", "cursor-help", `tabindex="0"`, `role="tooltip"`, "duid-tip"} {
		if strings.Contains(got, gone) {
			t.Errorf("the name still raises a tooltip (%s):\n%s", gone, got)
		}
	}
}

// TestAnAnswerFillsItsColumnOrKeepsItsMeasure: in the answer column a field
// whose value has a measure of its own (a number, a port range, an address)
// keeps it, and one without fills the column; a secret is capped by the
// column rather than standing at its own 256px (form-fields). Its 384px
// measure belongs to a row that stacks, and the shell sets it there.
func TestAnAnswerFillsItsColumnOrKeepsItsMeasure(t *testing.T) {
	r := newRenderer(t)
	for name, c := range map[string]struct {
		f       *Field
		measure string
		width   string
	}{
		"number":  {&Field{Name: "mtu", Label: "MTU", Key: "mtu"}, "number", "24"},
		"range":   {&Field{Name: "dest_port", Label: "Port", Key: "dest_port"}, "short", "32"},
		"address": {&Field{Name: "ipaddr", Label: "Address", Key: "ipaddr"}, "address", "44"},
	} {
		want := `class="verso-input" data-verso-verbatim data-verso-measure="` + c.measure + `"`
		if got := render(t, r, c.f); !strings.Contains(got, want) {
			t.Errorf("%s keeps its measure (%s):\n%s", name, want, got)
		}
		// its own measure, a pixel more so its border lands on a line, but
		// never past the column
		declares(t, "input", `.verso-input[data-verso-measure="`+c.measure+`"]`,
			"width: calc(var(--spacing) * "+c.width+" + 1px);", "max-width: 100%;")
	}
	// one without a measure fills the column
	declares(t, "input", ".verso-input", "width: 100%;")
	got := render(t, r, &Field{Name: "key", Label: "Password", Kind: "password"})
	if strings.Contains(got, "w-measure-64") || !strings.Contains(got, `data-verso-control-measure="secret"`) {
		t.Errorf("a secret fills the answer column and keeps its measure for a stacked row:\n%s", got)
	}
}

// TestRadioChoicesRunAcrossTheirRow: a field's radio choices are its answer,
// read across one line in order, 16px apart, each a 32px row of its own that
// wraps onto the next line when the column runs out (form-fields); drawn down
// the column they looked like settings of their own.
func TestRadioChoicesRunAcrossTheirRow(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "input", Label: "Traffic to this router", Kind: "select", Value: "reject",
		Options: []Option{{Value: "accept", Label: "accept"}, {Value: "reject", Label: "reject"}, {Value: "drop", Label: "drop"}},
	})
	if !strings.Contains(got, `aria-labelledby="input-label" class="verso-choice__group" data-verso-kind="radios"`) {
		t.Errorf("the choices do not run across their row:\n%s", got)
	}
	radios := cssRule(blockCSS(t, "choice"), `.verso-choice__group[data-verso-kind="radios"]`)
	for _, want := range []string{"flex-wrap: wrap", "column-gap: calc(var(--spacing) * 4)"} {
		if !strings.Contains(radios, want) {
			t.Errorf("the choices do not run across their row, 16px apart (%s):\n%s", want, radios)
		}
	}
	if strings.Contains(radios, "flex-direction: column") {
		t.Errorf("the choices still run down the column:\n%s", radios)
	}
	accept, reject, drop := strings.Index(got, ">accept<"), strings.Index(got, ">reject<"), strings.Index(got, ">drop<")
	if accept < 0 || accept > reject || reject > drop {
		t.Errorf("the choices are not in their order:\n%s", got)
	}
}

// TestAHelpOnlyFieldIsDescribed: a plugin that only wrote Help gets the same
// description, without having to rename the field it already fills in.
func TestAHelpOnlyFieldIsDescribed(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "leasetime", Label: "Lease time", Key: "leasetime",
		Help: "How long a client may keep an address before it asks again.",
	})
	if !strings.Contains(got, `<p id="leasetime-desc" data-verso-field-desc class="verso-field__desc"><span>How long a client may keep an address before it asks again.</span></p>`) {
		t.Errorf("a help-only field is not described under its name:\n%s", got)
	}
}

// TestRenderFieldWithoutTip: a label with nothing to explain stays inert — no
// hover affordance, no tab stop of its own, no tooltip.
func TestRenderFieldWithoutTip(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "name", Label: "Name", Key: "name"})
	for _, unwanted := range []string{"data-verso-tip", "cursor-help", `tabindex="0"`, "role=\"tooltip\"", "data-verso-field-desc"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("plain field should not carry %q:\n%s", unwanted, got)
		}
	}
}

// TestAFieldsChipNamesWhatReadsIt: what reads an option joins the key chip
// after a middot (form-fields), and the chip is whichever halves it has, so an
// option named without a reader still places itself and neither shows a bare
// dot. A switch names its reader the same way.
func TestAFieldsChipNamesWhatReadsIt(t *testing.T) {
	for _, tc := range []struct{ key, source, want string }{
		{"mac", "dhcp host", ">mac · dhcp host</span>"},
		{"mac", "", ">mac</span>"},
		{"", "dhcp host", ">dhcp host</span>"},
	} {
		got := render(t, newRenderer(t), &Field{Name: "mac", Label: "MAC", Key: tc.key, Source: tc.source})
		if !strings.Contains(got, "verso-chip") || !strings.Contains(got, tc.want) {
			t.Errorf("chip for key %q source %q is not %s:\n%s", tc.key, tc.source, tc.want, got)
		}
	}
	if got := render(t, newRenderer(t), &Field{Name: "mac", Label: "MAC"}); strings.Contains(got, "verso-chip") {
		t.Errorf("a field naming neither shows no chip:\n%s", got)
	}
	if got := render(t, newRenderer(t), &Switch{Name: "wmm", Label: "WMM", Key: "wmm", Source: "hostapd"}); !strings.Contains(got, ">wmm · hostapd</span>") {
		t.Errorf("a switch's chip names what reads it:\n%s", got)
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
		`<span class="verso-menu__value" data-verso-verbatim>10.0.0.30</span><span class="verso-menu__note">nas</span>`,
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

// An empty box's hint is read in place of a value, so a box too narrow for it
// misleads ("default 1" for "default 150"): a number's box widens to a short
// one to hold its hint, and a short one to an address's.
func TestAShortBoxWidensToHoldItsHint(t *testing.T) {
	for _, tc := range []struct {
		field Field
		want  string
	}{
		{Field{Key: "cachesize"}, "number"},
		{Field{Key: "cachesize", Placeholder: "150"}, "number"},
		{Field{Key: "cachesize", Placeholder: "default 150"}, "short"},
		{Field{Key: "src_dport", Placeholder: "e.g. 8000-8010"}, "address"},
	} {
		if got := tc.field.Measure(); got != tc.want {
			t.Errorf("%s with hint %q measures %q, want %q", tc.field.Key, tc.field.Placeholder, got, tc.want)
		}
	}
}
