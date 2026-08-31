// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// CapsulePreview is the styleguide workbench for the shell-owned staged-change
// capsule and its upward-expanding review surface. It deliberately has no behaviour;
// production interaction is added only after the visual contract is settled.
type CapsulePreview struct {
	Count    int      `json:"count"`
	Children []Widget `json:"-"`
}

func (*CapsulePreview) isWidget() {}

func (c *CapsulePreview) children() []Widget { return c.Children }

func (c *CapsulePreview) UnmarshalJSON(data []byte) error {
	var raw struct {
		Count    int               `json:"count"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Count = raw.Count
	children, err := decodeChildren(raw.Children, "capsule preview child")
	if err != nil {
		return err
	}
	c.Children = children
	return nil
}

type capsulePreviewView struct {
	Count    int
	Label    string
	Children []template.HTML
}

func (c *CapsulePreview) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(c.Children, csrf)
	if err != nil {
		return err
	}
	// TODO(i18n plurals): a flat "%d pending changes" renders one plural form;
	// Slovenian has four. An additive tn(one, other, n) helper (ADR-012) is the
	// fix when count strings become load-bearing. This is a styleguide workbench,
	// so the general form is enough for now.
	label := fmt.Sprintf(r.tr("%d pending changes"), c.Count)
	if c.Count == 1 {
		label = r.tr("1 pending change")
	}
	return r.execute(out, "capsule_preview.html.tmpl", capsulePreviewView{
		Count: c.Count, Label: label, Children: children,
	})
}
