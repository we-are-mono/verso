// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"testing"
)

// TestStylesheetGuardsCatchWhatTheyName runs each guard over sources written
// for it: once breaking the contract the way the guard names, once keeping it,
// so a guard that stopped looking would fail here before it passed the tree.
func TestStylesheetGuardsCatchWhatTheyName(t *testing.T) {
	cases := []struct {
		name  string
		guard func(styleSources) []violation
		bad   styleSources
		good  styleSources
		want  string // a fragment of the bad finding's key
	}{
		{
			name:  "a block's rule outside the block files",
			guard: rulesLiveInBlocks,
			bad:   styleSources{css: map[string]string{"input.css": ".verso-field-row { padding: 0; }"}},
			good:  styleSources{css: map[string]string{"input.css": "summary::-webkit-details-marker { display: none; }", "type.css": "@utility type-value { &[data-verso-verbatim] { font-size: 1rem; } }"}},
			want:  ".verso-field-row",
		},
		{
			name:  "a block rule starting from outside the block",
			guard: aBlockStartsFromItself,
			bad:   styleSources{css: map[string]string{"blocks/card.css": ".verso-page-body .verso-card { margin: 0; }"}},
			good:  styleSources{css: map[string]string{"blocks/card.css": ".verso-card > .x {} .verso-card[data-verso-headless] {} .verso-card__head:hover {} .verso-card { &:hover {} }"}},
			want:  ".verso-page-body .verso-card",
		},
		{
			name:  "a block setting another block's declaration",
			guard: crossBlockRulesSetOnlyProperties,
			bad:   styleSources{css: map[string]string{"blocks/card.css": ".verso-card__body > .verso-field { margin-inline: -23px; }"}},
			good:  styleSources{css: map[string]string{"blocks/card.css": ".verso-card__body > .verso-field { --verso-field-bleed: 23px; } .verso-card .verso-card__title { color: red; }"}},
			want:  "margin-inline",
		},
		{
			name:  "a class that is no block's",
			guard: classesAreBlocks,
			bad: styleSources{
				css:    map[string]string{"blocks/chart.css": ".verso-chart {} .verso-chart__line {}"},
				markup: map[string]string{"widget/templates/chart.html.tmpl": `<svg class="verso-chart"><path class="verso-chart-line verso-chart__line"/></svg>`},
			},
			good: styleSources{
				css:    map[string]string{"blocks/chart.css": ".verso-chart {} .verso-chart__line {}"},
				markup: map[string]string{"widget/templates/chart.html.tmpl": `<svg class="verso-chart" data-verso-tone="denim"><path class="verso-chart__line"/></svg><text class="verso-port-role"></text><a class="verso-nav-sub-3"></a>`},
			},
			want: "verso-chart-line",
		},
		{
			name:  "a block part nothing draws",
			guard: classesAreBlocks,
			bad: styleSources{
				css:    map[string]string{"blocks/card.css": ".verso-card {} .verso-card__foot {}"},
				markup: map[string]string{"widget/templates/section.html.tmpl": `<section class="verso-card"></section>`},
			},
			good: styleSources{
				css:    map[string]string{"blocks/card.css": ".verso-card {}"},
				markup: map[string]string{"widget/templates/section.html.tmpl": `<section class="verso-card"></section>`},
			},
			want: "verso-card__foot",
		},
		{
			name:  "a modifier class",
			guard: classesAreBlocks,
			bad: styleSources{
				css:    map[string]string{"blocks/chart.css": ".verso-chart {}"},
				markup: map[string]string{"widget/templates/chart.html.tmpl": `<svg class="verso-chart verso-chart--denim"></svg>`},
			},
			good: styleSources{
				css:    map[string]string{"blocks/chart.css": ".verso-chart {}"},
				markup: map[string]string{"widget/templates/chart.html.tmpl": `<svg class="verso-chart"></svg>`},
			},
			want: "verso-chart--denim",
		},
		{
			name:  "one look drawn twice from utilities",
			guard: noRepeatedBundles,
			bad: styleSources{markup: map[string]string{
				"widget/templates/a.html.tmpl": `<span class="size-1.5 shrink-0 rounded-[1px] border border-faint"></span>`,
				"server/assets/verso-x.js":     `el.className = "border border-faint shrink-0 size-1.5 rounded-[1px]";`,
			}},
			good: styleSources{markup: map[string]string{
				"widget/templates/a.html.tmpl": `<span class="verso-mark"></span><span class="flex items-center gap-2"></span>`,
				"server/assets/verso-x.js":     `el.className = "verso-mark"; b.className = "flex items-center gap-2";`,
			}},
			want: "border border-faint rounded-[1px] shrink-0 size-1.5",
		},
		{
			name:  "a long class list",
			guard: shortClassLists,
			bad:   styleSources{markup: map[string]string{"widget/templates/button.html.tmpl": `<button class="flex h-control items-center gap-2 rounded-xs border px-4 text-sm font-semibold {{template "tone" .}}"></button>`}},
			good:  styleSources{markup: map[string]string{"widget/templates/button.html.tmpl": `<button class="verso-button flex items-center gap-2" data-verso-tone="{{.Tone}}"></button>`}},
			want:  "flex h-control",
		},
		{
			name:  "a selector written into a class attribute",
			guard: noRulesInMarkup,
			bad:   styleSources{markup: map[string]string{"widget/templates/table.html.tmpl": `<table class="w-full [&_td:last-child]:!text-right pl-1.75!"></table>`}},
			good:  styleSources{markup: map[string]string{"widget/templates/table.html.tmpl": `<table class="verso-table w-full" data-verso-narrow="2 5"></table>`}},
			want:  "[&_td:last-child]:!text-right",
		},
		{
			name:  "a template that only prints classes",
			guard: noClassStringTemplates,
			bad:   styleSources{markup: map[string]string{"widget/templates/shared.html.tmpl": `{{define "verso-chip-class"}}inline-flex rounded-xs border px-1.5{{end}}`}},
			good:  styleSources{markup: map[string]string{"widget/templates/shared.html.tmpl": `{{define "verso-chip"}}<span class="verso-chip">{{.}}</span>{{end}}{{define "label"}}{{t "Copy"}}{{end}}`}},
			want:  "verso-chip-class",
		},
		{
			name:  "type written outside the roles",
			guard: typeComesFromRoles,
			bad: styleSources{
				markup: map[string]string{"widget/templates/field.html.tmpl": `<label class="text-sm font-medium text-ink">x</label>`},
				css:    map[string]string{"blocks/card.css": ".verso-card__title { font-weight: 600; }"},
			},
			good: styleSources{
				markup: map[string]string{"widget/templates/field.html.tmpl": `<label class="verso-field__label text-ink">x</label><td class="text-right">y</td>`},
				css:    map[string]string{"blocks/card.css": ".verso-card__title { @apply type-card-title; color: var(--color-ink); }", "type.css": "@utility type-card-title { font-size: 1rem; font-weight: 600; }"},
			},
			want: "text-sm",
		},
		{
			name:  "a block file the entry does not import",
			guard: everyBlockIsImported,
			bad:   styleSources{css: map[string]string{"input.css": `@import "./palette.css";`, "blocks/card.css": ".verso-card {}"}},
			good:  styleSources{css: map[string]string{"input.css": `@import "./blocks/card.css" layer(components);`, "blocks/card.css": ".verso-card {}"}},
			want:  "card",
		},
		{
			name:  "a role DESIGN.md does not list",
			guard: typeRolesMatchDesign,
			bad: styleSources{
				css:    map[string]string{"type.css": "@utility type-caption { font-size: 12px; }"},
				design: "## Typography\n- **Card title** (`type-card-title`) …\n## Layout\n",
			},
			good: styleSources{
				css:    map[string]string{"type.css": "@utility type-card-title { font-size: 1rem; }"},
				design: "## Typography\n- **Card title** (`type-card-title`) …\n## Layout\n",
			},
			want: "type-caption",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bad := c.guard(c.bad)
			if !strings.Contains(fmtViolations(bad), c.want) {
				t.Errorf("the guard does not catch %q:\n%s", c.want, fmtViolations(bad))
			}
			if good := c.guard(c.good); len(good) > 0 {
				t.Errorf("the guard faults sources that keep the contract:\n%s", fmtViolations(good))
			}
		})
	}
}

func fmtViolations(vs []violation) string {
	var b strings.Builder
	for _, v := range vs {
		b.WriteString(v.guard + ": " + v.key + "\n")
	}
	return b.String()
}
