// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"
)

// Collection is a short set of things a page keeps — authorized keys, a
// tunnel's peers — where each one is a machine string someone added by hand
// and may take away again. Every item is its identity over one line of detail;
// its one act (removal) asks in place under it; the next item is added from a
// quiet slot at the foot of the set that unfolds, where it stands, into the box
// that takes one. Nothing about keeping the set leaves the page, and the set
// draws no rules: a handful of identities is separated by space.
type Collection struct {
	Items []CollectionItem `json:"items"`
	// Empty is the one line that stands where the first item would.
	Empty string         `json:"empty,omitempty"`
	Add   *CollectionAdd `json:"add,omitempty"`
	// Action is where the set's forms post: the page's own address unless the
	// shell hosts the plugin on a page of its own (Access), which it names here.
	Action string `json:"-"`
}

// CollectionItem is one kept thing: its identity, a line of detail (a
// fingerprint, an address), and how it is removed.
type CollectionItem struct {
	Title  string            `json:"title"`
	Detail string            `json:"detail,omitempty"`
	Remove *CollectionRemove `json:"remove,omitempty"`
}

// CollectionRemove is an item's removal: the pair it posts, asked first by the
// confirmation it carries. The confirmation's trigger is the act's short word;
// the item's title completes it for anyone who hears it rather than sees it.
type CollectionRemove struct {
	Name    string   `json:"name"`
	Value   string   `json:"value"`
	Confirm *Confirm `json:"confirm"`
}

// CollectionAdd is the slot the next item is added from: the act's name at
// rest, and once open the box that takes one machine string, the reading the
// plugin gives of what was typed (a live preview), and the act that adds it.
// A refused paste comes back open, as typed, with the reason.
type CollectionAdd struct {
	Label       string `json:"label"`
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Submit      string `json:"submit"`
	Error       string `json:"error,omitempty"`
	Open        bool   `json:"open,omitempty"`
	Preview     Widget `json:"-"`
}

func (*Collection) isWidget() {}

// tally is how many things the set holds, which a subheading over it reads
// out, so the number can never disagree with what is listed.
func (c *Collection) tally() int { return len(c.Items) }

func (c *Collection) children() []Widget {
	var out []Widget
	for _, item := range c.Items {
		if item.Remove != nil && item.Remove.Confirm != nil {
			out = append(out, item.Remove.Confirm)
		}
	}
	if c.Add != nil && c.Add.Preview != nil {
		out = append(out, c.Add.Preview)
	}
	return out
}

// UnmarshalJSON decodes the add slot's preview through Decode, so an unknown
// widget there fails loudly rather than vanishing.
func (a *CollectionAdd) UnmarshalJSON(data []byte) error {
	type plain CollectionAdd
	var raw struct {
		plain
		Preview json.RawMessage `json:"preview"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*a = CollectionAdd(raw.plain)
	if len(raw.Preview) != 0 && string(raw.Preview) != "null" {
		preview, err := Decode(raw.Preview)
		if err != nil {
			return fmt.Errorf("collection add preview: %w", err)
		}
		a.Preview = preview
	}
	return nil
}

type collectionItemView struct {
	Title, Detail string
	Name, Value   string
	Remove        template.HTML
}

type collectionView struct {
	Action    string
	CSRFToken string
	Items     []collectionItemView
	Empty     string
	Add       *CollectionAdd
	AddID     string
	Open      bool
	Preview   template.HTML
}

func (c *Collection) renderInto(r *Renderer, out io.Writer, csrf string) error {
	v := collectionView{Action: c.Action, CSRFToken: csrf, Empty: c.Empty, Add: c.Add}
	for _, item := range c.Items {
		iv := collectionItemView{Title: item.Title, Detail: item.Detail}
		if item.Remove != nil && item.Remove.Confirm != nil {
			confirm := *item.Remove.Confirm
			confirm.subject = item.Title
			var b strings.Builder
			if err := r.render(&b, &confirm, csrf); err != nil {
				return err
			}
			iv.Name, iv.Value, iv.Remove = item.Remove.Name, item.Remove.Value, template.HTML(b.String()) //nolint:gosec // rendered by the shell's own templates
		}
		v.Items = append(v.Items, iv)
	}
	if c.Add != nil {
		v.AddID = fmt.Sprintf("verso-collection-add-%d", r.seq.cfm.Add(1))
		v.Open = c.Add.Open || c.Add.Error != ""
		if c.Add.Preview != nil {
			var b strings.Builder
			if err := r.render(&b, c.Add.Preview, csrf); err != nil {
				return err
			}
			v.Preview = template.HTML(b.String()) //nolint:gosec // rendered by the shell's own templates
		}
	}
	return r.execute(out, "collection.html.tmpl", v)
}
