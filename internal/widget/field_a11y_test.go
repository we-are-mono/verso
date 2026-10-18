// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
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

// TestFieldHelpDescribesTheControl: the description under the name is what
// the control is described by, so it is read when the field takes focus,
// whether or not it is folded. With an error, both are read.
func TestFieldHelpDescribesTheControl(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "mtu", Label: "MTU", Help: "Largest packet size.", Error: "Too small."})
	if !strings.Contains(got, `id="mtu" name="mtu"`) || !strings.Contains(got, `aria-describedby="mtu-desc mtu-error"`) {
		t.Errorf("the control must be described by its help and its error:\n%s", got)
	}
}

// TestFieldNoteStandsUnderTheControl: a note is what changing the field does,
// kept in view under its control in the answer, after the refusal, and the
// control is described by it — for every kind a row frames.
func TestFieldNoteStandsUnderTheControl(t *testing.T) {
	for _, kind := range []string{"", "select", "password", "textarea", "time", "datetime-local"} {
		f := &Field{Name: "theme", Label: "Theme", Kind: kind, Note: "Takes effect immediately.", Error: "Pick one."}
		if kind == "select" {
			f.Options = choiceOptions(RadioOptionLimit + 1)
		}
		got := render(t, newRenderer(t), f)
		answer := strings.Index(got, "verso-field__answer")
		note := strings.Index(got, `id="theme-note" data-verso-field-note`)
		refusal := strings.Index(got, `id="theme-error"`)
		if answer < 0 || note < answer || !strings.Contains(got, "Takes effect immediately.") {
			t.Fatalf("kind %q: the note is not under the control in the answer:\n%s", kind, got)
		}
		if refusal < 0 || refusal > note {
			t.Errorf("kind %q: the refusal must stand before the note:\n%s", kind, got)
		}
		if !strings.Contains(got, `aria-describedby="theme-error theme-note"`) {
			t.Errorf("kind %q: the control must be described by its refusal and its note:\n%s", kind, got)
		}
	}
	clean := render(t, newRenderer(t), &Field{Name: "theme", Label: "Theme"})
	if strings.Contains(clean, "theme-note") {
		t.Errorf("a field with no note points at none:\n%s", clean)
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

// Radio and checkbox sets have named groups; larger single-choice sets have a
// directly associated label. Both single-choice presentations keep help/errors.
func TestOptionSetsAreNamedGroups(t *testing.T) {
	opts := []Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}
	choice := render(t, newRenderer(t), &Field{Name: "family", Label: "Family", Kind: "select", Style: "segmented", Options: choiceOptions(RadioOptionLimit),
		Help: "Choose a family.", Error: "Choose an available family.", Required: true})
	dropdown := render(t, newRenderer(t), &Field{Name: "family", Label: "Family", Kind: "select", Options: choiceOptions(RadioOptionLimit + 1),
		Help: "Choose a family.", Error: "Choose an available family."})
	checks := render(t, newRenderer(t), &Field{Name: "days", Label: "Days", Kind: "checks", Options: opts})
	strip := render(t, newRenderer(t), &Field{Name: "days", Label: "Days", Kind: "checks", Style: "segmented", Options: opts})
	for name, c := range map[string]struct{ got, role, label string }{
		"checks": {checks, `role="group"`, `aria-labelledby="days-label"`},
		"strip":  {strip, `role="group"`, `aria-labelledby="days-label"`},
	} {
		if !strings.Contains(c.got, c.role) || !strings.Contains(c.got, c.label) {
			t.Errorf("%s: option set must be a group named by its label (%s %s):\n%s", name, c.role, c.label, c.got)
		}
	}
	for _, want := range []string{`for="family"`, `<select id="family" name="family"`, `aria-invalid="true"`, `aria-describedby="family-desc family-error"`} {
		if !strings.Contains(dropdown, want) {
			t.Errorf("dropdown must keep its label, help and error association (%s):\n%s", want, dropdown)
		}
	}
	if RadioOptionLimit > 0 {
		for _, want := range []string{`<span id="family-label"`, `role="radiogroup" aria-labelledby="family-label"`,
			`aria-required="true"`, `aria-describedby="family-desc family-error"`, `aria-invalid="true"`, `class="verso-choice__native"`} {
			if !strings.Contains(choice, want) {
				t.Errorf("radios must keep group naming, validation and visible focus (%s):\n%s", want, choice)
			}
		}
		if strings.Count(choice, `aria-invalid="true"`) != RadioOptionLimit+1 || strings.Count(choice, "<label ") != RadioOptionLimit {
			t.Errorf("each radio needs its own option label and validation state:\n%s", choice)
		}
	}
	declares(t, "choice", ".verso-choice__native:focus-visible",
		"outline: 2px solid var(--color-denim-deep)", "outline-offset: 2px")
	// A segment's input is screen-reader-only, so the segment shows focus.
	if !strings.Contains(strip, `<label class="verso-choice__segment">`) {
		t.Errorf("a segment must be the choice block's segment:\n%s", strip)
	}
	declares(t, "choice", ".verso-choice__segment:has(:focus-visible)",
		"outline: 2px solid var(--color-denim)", "outline-offset: 2px")
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

// TestDescribedControlsAreDescribed: whatever row says what it is for, under
// its name, the control the name labels is described by it, so the sentence
// is read when the control takes focus, and no name raises a tooltip.
func TestDescribedControlsAreDescribed(t *testing.T) {
	for name, c := range map[string]struct {
		w          Widget
		desc, read string
	}{
		"switch": {&Switch{Name: "wmm", Label: "WMM", Help: "Prioritise voice.", Tip: "Voice goes first."},
			`<p id="wmm-desc" data-verso-field-desc class="verso-field__desc"><span>Prioritise voice. Voice goes first.</span></p>`,
			`id="wmm" name="wmm" aria-describedby="wmm-desc"`},
		"gate": {&Conditional{Name: "psk", Label: "Use a key", Help: "A shared secret."},
			`<p id="psk-desc" data-verso-field-desc class="verso-field__desc"><span>A shared secret.</span></p>`,
			`id="psk" name="psk" aria-describedby="psk-desc"`},
		"list": {&List{Name: "ntp", Label: "Servers", Help: "Tried in order."},
			`<p id="ntp-desc" data-verso-field-desc class="verso-field__desc"><span>Tried in order.</span></p>`,
			`aria-labelledby="ntp-label" aria-describedby="ntp-desc"`},
	} {
		got := render(t, newRenderer(t), c.w)
		if !strings.Contains(got, c.desc) {
			t.Errorf("%s: its help is not its description:\n%s", name, got)
		}
		if !strings.Contains(got, c.read) {
			t.Errorf("%s: its control is not described by its description:\n%s", name, got)
		}
		if strings.Contains(got, "data-verso-tip") || strings.Contains(got, "-tip") {
			t.Errorf("%s: its name still raises a tooltip:\n%s", name, got)
		}
	}
}

// TestTipCanBeReached: the tip is not pointer-events-none, so a pointer can
// travel from the label onto the sentence without it vanishing (WCAG 1.4.13);
// the stylesheet decides when it takes the pointer.
func TestTipCanBeReached(t *testing.T) {
	got := render(t, newRenderer(t), &Conditions{Label: "Conditions", Items: []ConditionItem{
		{Key: "mtu", Label: "MTU", Help: "Largest packet size.", Active: true},
	}})
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
// The box takes the input role (type.css): its words the sans at 400, its
// verbatim form (data-verso-verbatim) mono at the reading size.
func TestInputsFollowTheTypographyContract(t *testing.T) {
	for name, c := range map[string]struct {
		f        *Field
		verbatim bool
	}{
		"words":    {&Field{Name: "name", Label: "Name"}, false},
		"password": {&Field{Name: "pw", Label: "Password", Kind: "password"}, false},
		"prose":    {&Field{Name: "desc", Label: "Description", Kind: "textarea"}, false},
		"address":  {&Field{Name: "ip", Label: "Address", Datatype: "ipaddr"}, true},
		"number":   {&Field{Name: "mtu", Label: "MTU", Key: "mtu"}, true},
		"unit":     {&Field{Name: "rate", Label: "Rate", Unit: "Mbit/s"}, true},
	} {
		got := render(t, newRenderer(t), c.f)
		at := strings.Index(got, `id="`+c.f.Name+`"`)
		if at < 0 {
			t.Fatalf("%s: no box:\n%s", name, got)
		}
		box := got[at : at+strings.Index(got[at:], ">")]
		if !strings.Contains(box, `class="verso-input"`) && !strings.Contains(box, `class="verso-box__input"`) {
			t.Errorf("%s: the box is not the input block's: %s", name, box)
		}
		if strings.Contains(box, "data-verso-verbatim") != c.verbatim {
			t.Errorf("%s: verbatim should be %v: %s", name, c.verbatim, box)
		}
		if strings.Contains(box, "font-") {
			t.Errorf("%s: a box's type is its role's, never a utility: %s", name, box)
		}
	}
	for block, sel := range map[string]string{"input": ".verso-input", "box": ".verso-box__input"} {
		if rule := cssRule(blockCSS(t, block), sel); !strings.Contains(rule, "@apply type-input;") || strings.Contains(rule, "type-value") {
			t.Errorf("blocks/%s.css %s should set its words in the input role, never the displayed value's:\n%s", block, sel, rule)
		}
	}
	typeCSS, err := os.ReadFile(filepath.Join("..", "server", "assets", "type.css"))
	if err != nil {
		t.Fatal(err)
	}
	role := cssRule(string(typeCSS), "@utility type-input")
	for _, want := range []string{"font-family: var(--font-sans);", "font-size: 0.875rem;", "font-weight: 400;", "&[data-verso-verbatim] { font-family: var(--font-mono); font-size: 1rem;"} {
		if !strings.Contains(role, want) {
			t.Errorf("the input role should declare %q:\n%s", want, role)
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
		if !strings.Contains(got, `<label class="verso-choice__option">`) || strings.Contains(got, "data-verso-verbatim") {
			t.Errorf("%s: an enumerated choice is words:\n%s", name, got)
		}
	}
	if !strings.Contains(host, `<label class="verso-choice__option" data-verso-verbatim>`) {
		t.Errorf("a select of machine strings is mono:\n%s", host)
	}
	// The option sets its words in the input role, whose verbatim form is mono.
	declares(t, "choice", ".verso-choice__option", "@apply type-input")
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
}

// refusedBorder is a refused box's border: crimson at rest, deep crimson under
// the pointer and while focused — the box stays the one that is wrong while
// it is being corrected. It hangs on aria-invalid, so once the value changes
// (the refusal was about the old one) dropping the attribute returns the box
// to the ordinary hairline and the action colour.
// Focused, the edge doubles to 2px — an inset line inside the border, beside
// the field's own inset shadow — so focus reads without the box growing.
// The input block (blocks/input.css) draws it on any box that carries the
// attribute, however it came to carry it.
func refusedBorder(t *testing.T) {
	t.Helper()
	declares(t, "input", ".verso-input", "border: 1px solid var(--color-rule-strong);")
	declares(t, "input", ".verso-input:hover", "border-color: var(--color-faint);")
	declares(t, "input", ".verso-input:focus", "border-color: var(--color-denim);")
	declares(t, "input", `.verso-input[aria-invalid="true"]`, "border-color: var(--color-crimson);")
	declares(t, "input", `.verso-input[aria-invalid="true"]:hover`, "border-color: var(--color-crimson-deep);")
	declares(t, "input", `.verso-input[aria-invalid="true"]:focus`, "border-color: var(--color-crimson-deep);",
		"box-shadow: inset 0 0 0 1px var(--color-crimson-deep), var(--shadow-field);")
}

// TestRefusedFieldStaysCrimsonWhileFocused: a refused field is marked as the
// one that is wrong at rest, under the pointer and while it has focus, and
// its refusal is marked as one the page can take away once it is answered.
func TestRefusedFieldStaysCrimsonWhileFocused(t *testing.T) {
	refusedBorder(t)
	got := render(t, newRenderer(t), &Field{Name: "ip", Label: "Address", Error: "Not an address."})
	if box := refusedBox(got); !strings.Contains(box, `class="verso-input"`) {
		t.Errorf("refused field's box is not the input block: %q\n%s", box, got)
	}
	if !strings.Contains(got, `id="ip-error" data-verso-error`) {
		t.Errorf("refused field missing its refusal:\n%s", got)
	}
	for name, w := range map[string]Widget{
		"collection add": &Collection{Add: &CollectionAdd{Label: "Add a key", Name: "authorized_key", Submit: "Add key", Error: "Paste one complete SSH public key."}},
		"list box":       &List{Name: "server", Label: "Servers", Items: []string{"x"}, Errors: map[string]string{"0": "Enter a hostname."}},
	} {
		got := render(t, newRenderer(t), w)
		if box := refusedBox(got); !strings.Contains(box, `class="verso-input"`) {
			t.Errorf("%s: the refused box is not the input block: %q\n%s", name, box, got)
		}
		if !strings.Contains(got, "data-verso-error") {
			t.Errorf("%s missing its refusal:\n%s", name, got)
		}
	}
}

// refusedBox is the open tag of the first element marked aria-invalid, or ""
// when none is.
func refusedBox(html string) string {
	at := strings.Index(html, `aria-invalid="true"`)
	if at < 0 {
		return ""
	}
	start := strings.LastIndex(html[:at], "<")
	return html[start : at+strings.Index(html[at:], ">")]
}

// A copy control is named with what it copies, as a row's act is named with
// its row ("Edit lan"): a screen reader hearing "Copy" four times in a list
// of addresses cannot tell which one each takes. A code block's says the file.
func TestACopyControlNamesWhatItCopies(t *testing.T) {
	r := newRenderer(t)
	props := render(t, r, &Properties{Items: []Property{{Label: "MAC address", Value: "aa:bb:cc:dd:ee:ff", Mono: true, Copy: true}}})
	if !strings.Contains(props, `aria-label="Copy aa:bb:cc:dd:ee:ff"`) {
		t.Errorf("a copied value should name itself:\n%s", props)
	}
	code := render(t, r, &Code{Label: "/etc/config/firewall", Value: "config zone", Copy: true})
	if !strings.Contains(code, `aria-label="Copy /etc/config/firewall"`) {
		t.Errorf("a code block's copy should name its file:\n%s", code)
	}
}
