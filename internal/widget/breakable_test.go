// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"strings"
	"testing"
)

// TestBreakableBreaksAfterTheValuesOwnSeparator: a machine string that has to
// wrap wraps where it divides itself — after its slashes if it has any, else
// after its dots, else after its colons — never inside one of the parts. So a
// forwarding rule keeps its address whole, a hostname its labels, an IPv6 its
// groups; and everything else is escaped as text.
func TestBreakableBreaksAfterTheValuesOwnSeparator(t *testing.T) {
	for in, want := range map[string]template.HTML{
		"/corp.example.com/10.66.0.53": "/<wbr>corp.example.com/<wbr>10.66.0.53",
		"nas.lan/10.0.0.12":            "nas.lan/<wbr>10.0.0.12", // the slash outranks an earlier dot
		"0.openwrt.pool.ntp.org":       "0.<wbr>openwrt.<wbr>pool.<wbr>ntp.<wbr>org",
		"fd42:7ea:aa00::1":             "fd42:<wbr>7ea:<wbr>aa00:<wbr>:<wbr>1",
		"1.1.1.1":                      "1.<wbr>1.<wbr>1.<wbr>1",
		"lan":                          "lan",
		"<b>&x/":                       "&lt;b&gt;&amp;x/",
	} {
		if got := Breakable(in); got != want {
			t.Errorf("Breakable(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestListValueWrapsAtItsSeparators: a list's value is set breakable and wraps
// at those points, falling back to anywhere only where one part alone is wider
// than the column — never break-all, which cut a number in two.
func TestListValueWrapsAtItsSeparators(t *testing.T) {
	got := render(t, newRenderer(t), &List{Name: "forwarding", Label: "Send these domains to a specific server", Style: "rows",
		Items: []string{"/corp.example.com/10.66.0.53"}})
	// The remove hangs from the value's first line: the row aligns to the top,
	// and the value's 2px above centres its first 24px line on the 28px remove.
	if !strings.Contains(got, `class="min-w-0 pt-0.5 wrap-anywhere font-mono text-base font-medium text-ink">/<wbr>corp.example.com/<wbr>10.66.0.53</span>`) {
		t.Errorf("the value wraps at its own separators, hung from its first line:\n%s", got)
	}
	if strings.Contains(got, "break-all") {
		t.Errorf("no break-all on a list value:\n%s", got)
	}
}
