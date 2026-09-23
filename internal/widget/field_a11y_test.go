// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestFieldErrorIsTiedToItsControl: a refusal is read with the field it
// refuses. The error band carries an id, the control points at it and says it
// is invalid, so a screen reader landing on the field hears why it came back.
func TestFieldErrorIsTiedToItsControl(t *testing.T) {
	for _, kind := range []string{"", "select", "password", "textarea", "time", "datetime-local"} {
		f := &Field{Name: "addr", Label: "Address", Kind: kind, Error: "Enter a valid IP address."}
		if kind == "select" {
			f.Options = []Option{{Value: "a", Label: "A"}}
		}
		got := render(t, newRenderer(t), f)
		for _, want := range []string{`aria-invalid="true"`, `aria-describedby="addr-error"`, `id="addr-error"`} {
			if !strings.Contains(got, want) {
				t.Errorf("kind %q: refused field missing %q:\n%s", kind, want, got)
			}
		}
	}
	clean := render(t, newRenderer(t), &Field{Name: "addr", Label: "Address"})
	if strings.Contains(clean, "aria-invalid") || strings.Contains(clean, "addr-error") {
		t.Errorf("a field with no error claims none:\n%s", clean)
	}
}

// TestFieldHelpDescribesTheControl: the explanation raised onto the label is
// also what the control is described by, so it is read when the field takes
// focus, not only when the label is hovered. With an error, both are read.
func TestFieldHelpDescribesTheControl(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "mtu", Label: "MTU", Help: "Largest packet size.", Error: "Too small."})
	if !strings.Contains(got, `id="mtu" name="mtu"`) || !strings.Contains(got, `aria-describedby="mtu-tip mtu-error"`) {
		t.Errorf("the control must be described by its help and its error:\n%s", got)
	}
}

// TestFieldRequiredIsStated: a field the plugin marks required says so to
// assistive technology. Native required would put the browser's own bubble in
// front of the shell's refusal, so it is stated, not enforced.
func TestFieldRequiredIsStated(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "name", Label: "Name", Required: true})
	if !strings.Contains(got, `aria-required="true"`) {
		t.Errorf("required field must carry aria-required:\n%s", got)
	}
	if strings.Contains(got, " required") {
		t.Errorf("required must not be enforced natively on a text field:\n%s", got)
	}
}

// TestOptionSetsAreNamedGroups: segmented radios and check sets have no single
// control a label can point at, so the set is a group named by the row's label.
func TestOptionSetsAreNamedGroups(t *testing.T) {
	opts := []Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}
	radios := render(t, newRenderer(t), &Field{Name: "family", Label: "Family", Kind: "select", Style: "segmented", Options: opts})
	checks := render(t, newRenderer(t), &Field{Name: "days", Label: "Days", Kind: "checks", Options: opts})
	strip := render(t, newRenderer(t), &Field{Name: "days", Label: "Days", Kind: "checks", Style: "segmented", Options: opts})
	for name, c := range map[string]struct{ got, role, label string }{
		"radios": {radios, `role="radiogroup"`, `aria-labelledby="family-label"`},
		"checks": {checks, `role="group"`, `aria-labelledby="days-label"`},
		"strip":  {strip, `role="group"`, `aria-labelledby="days-label"`},
	} {
		if !strings.Contains(c.got, c.role) || !strings.Contains(c.got, c.label) {
			t.Errorf("%s: option set must be a group named by its label (%s %s):\n%s", name, c.role, c.label, c.got)
		}
	}
	if !strings.Contains(radios, `id="family-label"`) {
		t.Errorf("the row label must carry the id the group points at:\n%s", radios)
	}
	// A segment's input is screen-reader-only, so the segment shows focus.
	if !strings.Contains(radios, "has-focus-visible:outline-2") {
		t.Errorf("a segment must show keyboard focus:\n%s", radios)
	}
}

// TestListValuesAreNamed: every box in a list is one value of the setting its
// row label names; a refused value points at its refusal; the token box is the
// input its label is for, and its suggestions are a listbox it controls.
func TestListValuesAreNamed(t *testing.T) {
	boxes := render(t, newRenderer(t), &List{Name: "server", Label: "NTP servers",
		Items: []string{"0.pool.ntp.org", "bad host"}, Errors: map[string]string{"1": "Not a host."}})
	for _, want := range []string{
		`value="0.pool.ntp.org" type="text" aria-labelledby="server-label"`,
		`aria-invalid="true" aria-describedby="server-error-1"`, `id="server-error-1"`,
		// The add box is named through the label's own id as well as by its
		// for: a page section may carry the same id as the field, and a for
		// that resolves to the section names nothing.
		`id="server" name="server" value="" type="text" aria-labelledby="server-label"`,
	} {
		if !strings.Contains(boxes, want) {
			t.Errorf("list missing %q:\n%s", want, boxes)
		}
	}
	tokens := render(t, newRenderer(t), &List{Name: "proto", Label: "Protocols", Style: "tokens",
		Options: []Option{{Value: "tcp", Label: "tcp"}, {Value: "udp", Label: "udp"}}})
	for _, want := range []string{
		`id="proto" aria-labelledby="proto-label" data-verso-token-input`, `role="combobox"`, `aria-controls="proto-options"`,
		`id="proto-options" data-verso-token-menu role="listbox" aria-labelledby="proto-label"`,
		`id="proto-option-0" role="option" aria-selected="false" tabindex="-1"`,
	} {
		if !strings.Contains(tokens, want) {
			t.Errorf("token list missing %q:\n%s", want, tokens)
		}
	}
}

// TestExplainedControlsAreDescribed: whatever row raises an explanation onto
// its label, the control the label names is described by it, so the sentence
// is read when the control takes focus, not only when the label is hovered.
func TestExplainedControlsAreDescribed(t *testing.T) {
	sw := render(t, newRenderer(t), &Switch{Name: "wmm", Label: "WMM", Help: "Prioritise voice."})
	if !strings.Contains(sw, `id="wmm" name="wmm" aria-describedby="wmm-tip"`) {
		t.Errorf("an explained switch must be described by its tip:\n%s", sw)
	}
	gate := render(t, newRenderer(t), &Conditional{Name: "psk", Label: "Use a key", Help: "A shared secret."})
	if !strings.Contains(gate, `id="psk" name="psk" aria-describedby="psk-tip"`) {
		t.Errorf("an explained gate must be described by its tip:\n%s", gate)
	}
	list := render(t, newRenderer(t), &List{Name: "ntp", Label: "Servers", Help: "Tried in order."})
	if !strings.Contains(list, `aria-labelledby="ntp-label" aria-describedby="ntp-tip"`) {
		t.Errorf("an explained list must be described by its tip:\n%s", list)
	}
}

// TestTipCanBeReached: the tip is not pointer-events-none, so a pointer can
// travel from the label onto the sentence without it vanishing (WCAG 1.4.13);
// the stylesheet decides when it takes the pointer.
func TestTipCanBeReached(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "mtu", Label: "MTU", Help: "Largest packet size."})
	start := strings.Index(got, `role="tooltip"`)
	if start < 0 {
		t.Fatalf("no tip:\n%s", got)
	}
	if tip := got[start : start+strings.Index(got[start:], ">")]; strings.Contains(tip, "pointer-events-none") {
		t.Errorf("the tip must be reachable by the pointer: %s", tip)
	}
}

// TestInputsFollowTheTypographyContract: what is typed into a box is set the
// way it is read elsewhere. A machine string (an address, a port, a hostname)
// is mono at the reading size and regular weight, a step under the 500 a
// displayed value takes; words (a rule's name, a description, a password) are
// sans at 14px, which is what Hanken's taller x-height reads as beside mono 16.
func TestInputsFollowTheTypographyContract(t *testing.T) {
	const mono, sans = "font-mono text-base font-normal", "font-sans text-sm font-normal"
	for name, c := range map[string]struct {
		f    *Field
		want string
	}{
		"words":    {&Field{Name: "name", Label: "Name"}, sans},
		"password": {&Field{Name: "pw", Label: "Password", Kind: "password"}, sans},
		"prose":    {&Field{Name: "desc", Label: "Description", Kind: "textarea"}, sans},
		"address":  {&Field{Name: "ip", Label: "Address", Datatype: "ipaddr"}, mono},
		"number":   {&Field{Name: "mtu", Label: "MTU", Key: "mtu"}, mono},
		"unit":     {&Field{Name: "rate", Label: "Rate", Unit: "Mbit/s"}, mono},
	} {
		got := render(t, newRenderer(t), c.f)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q:\n%s", name, c.want, got)
		}
		if strings.Contains(got, "font-mono text-base font-medium text-ink shadow") {
			t.Errorf("%s: an input is never set at the displayed value's 500:\n%s", name, got)
		}
	}
}

// TestSelectIsAChoiceUnlessItHoldsAMachineString: a select is an enumerated
// choice, and a choice is words: a timezone, a policy (accept, reject), a
// protocol all take the sans, whatever their labels look like. Only a select
// whose field declares a machine-string datatype is mono, because then what is
// picked is a value someone would retype verbatim.
func TestSelectIsAChoiceUnlessItHoldsAMachineString(t *testing.T) {
	policy := render(t, newRenderer(t), &Field{Name: "target", Label: "Action", Kind: "select",
		Options: []Option{{Value: "accept", Label: "accept"}, {Value: "reject", Label: "reject"}}})
	zone := render(t, newRenderer(t), &Field{Name: "zonename", Label: "Timezone", Kind: "select",
		Options: []Option{{Value: "UTC", Label: "UTC"}, {Value: "Europe/Ljubljana", Label: "Europe/Ljubljana"}}})
	host := render(t, newRenderer(t), &Field{Name: "server", Label: "Server", Kind: "select", Datatype: "hostname",
		Options: []Option{{Value: "0.pool.ntp.org", Label: "0.pool.ntp.org"}}})
	for name, got := range map[string]string{"policy": policy, "timezone": zone} {
		if !strings.Contains(got, "font-sans text-sm font-normal") || strings.Contains(got, "font-mono text-base") {
			t.Errorf("%s: an enumerated choice is words:\n%s", name, got)
		}
	}
	if !strings.Contains(host, "font-mono text-base font-normal") {
		t.Errorf("a select of machine strings is mono:\n%s", host)
	}
}

// TestCoarseCutsAreOneCountedDropdown: a control surface cuts by a dropdown,
// never a switch — each option priced with what taking it would leave, set in
// the sans like the label it prices, and carrying its bare label so a live
// listing can re-price it as lines arrive.
func TestCoarseCutsAreOneCountedDropdown(t *testing.T) {
	got := render(t, newRenderer(t), &ActionBar{Tabs: []ActionTab{
		{Label: "All families", Count: 7, Active: true}, {Label: "IPv4", Count: 4, Match: "ipv4"},
	}})
	if strings.Contains(got, "font-mono") || strings.Contains(got, "data-verso-tab") {
		t.Errorf("the cut is one sans dropdown, not a segmented switch:\n%s", got)
	}
	for _, want := range []string{
		`<select data-verso-listing-cut`,
		`<option value="" data-label="All families" selected>All families · 7</option>`,
		`<option value="ipv4" data-label="IPv4">IPv4 · 4</option>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("cut missing %q:\n%s", want, got)
		}
	}
	problems := render(t, newRenderer(t), &ActionBar{Style: "interfaces", Tabs: []ActionTab{{Label: "Needs a look", Count: 2, Match: "problem"}}})
	if !strings.Contains(problems, `data-verso-problem-count class="text-sm tabular-nums"`) {
		t.Errorf("the problem count must be a sans count:\n%s", problems)
	}
}

// refusedBorder is a refused box's border: crimson at rest, deep crimson under
// the pointer and while focused — the box stays the one that is wrong while
// it is being corrected. It hangs on aria-invalid, so once the value changes
// (the refusal was about the old one) dropping the attribute returns the box
// to the ordinary hairline and the action colour.
// Focused, the edge doubles to 2px — an inset line inside the border, beside
// the field's own inset shadow — so focus reads without the box growing.
const refusedBorder = "border-rule-strong hover:border-faint focus:border-denim aria-invalid:border-crimson aria-invalid:hover:border-crimson-deep aria-invalid:focus:border-crimson-deep aria-invalid:focus:shadow-[inset_0_0_0_1px_var(--color-crimson-deep),inset_0_1px_2px_rgba(27,25,23,.06)]"

// TestRefusedFieldStaysCrimsonWhileFocused: a refused field is marked as the
// one that is wrong at rest, under the pointer and while it has focus, and
// its refusal is marked as one the page can take away once it is answered.
func TestRefusedFieldStaysCrimsonWhileFocused(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "ip", Label: "Address", Error: "Not an address."})
	for _, want := range []string{refusedBorder, `aria-invalid="true"`, `id="ip-error" data-verso-error`} {
		if !strings.Contains(got, want) {
			t.Errorf("refused field missing %q:\n%s", want, got)
		}
	}
	for name, w := range map[string]Widget{
		"collection add": &Collection{Add: &CollectionAdd{Label: "Add a key", Name: "authorized_key", Submit: "Add key", Error: "Paste one complete SSH public key."}},
		"list box":       &List{Name: "server", Label: "Servers", Items: []string{"x"}, Errors: map[string]string{"0": "Enter a hostname."}},
	} {
		got := render(t, newRenderer(t), w)
		for _, want := range []string{refusedBorder, `aria-invalid="true"`, "data-verso-error"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s missing %q:\n%s", name, want, got)
			}
		}
	}
}

// TestPairSecondHalfIsNamed: the range's closing box has no label of its own;
// it is named by the row's label and the word that joins the two.
func TestPairSecondHalfIsNamed(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "from", Label: "Ports", Value: "1000", Pair: &FieldPair{Name: "to", Value: "2000", Join: "to"}})
	if !strings.Contains(got, `id="to" name="to"`) || !strings.Contains(got, `aria-labelledby="from-label from-join"`) || !strings.Contains(got, `id="from-join"`) {
		t.Errorf("the pair's second input must be named by the label and the joining word:\n%s", got)
	}
}
