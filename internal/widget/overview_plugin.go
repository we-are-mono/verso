// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "fmt"

// OverviewPluginCard is a shell-composed, read-only plugin card.
type OverviewPluginCard struct {
	ID, PluginID, Title, Icon, Href string
	Body                            Widget
	Translate                       func(string) string
}

// DecodeOverviewBody accepts only the Overview's read-only card vocabulary.
func DecodeOverviewBody(data []byte) (Widget, error) {
	w, err := Decode(data)
	if err != nil {
		return nil, err
	}
	var invalid error
	Walk(w, func(n Widget) {
		if invalid != nil {
			return
		}
		switch x := n.(type) {
		case *Stack, *Properties:
		case *Callout:
			if x.Link != nil || len(x.Acts) != 0 {
				invalid = fmt.Errorf("overview callouts cannot contain links or actions")
			}
		default:
			invalid = fmt.Errorf("widget %T is not allowed in an overview card", n)
		}
	})
	if invalid != nil {
		return nil, invalid
	}
	return w, nil
}
