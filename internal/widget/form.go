// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
)

// Form is an interactive container: it renders its fields inside a POST form that
// submits back to the same page (the shell's /plugins/<id>/ URL). Fields decode
// recursively through Decode, so a form composes the closed set — typically
// fields and lists, but any widget nests.
type Form struct {
	Submit  string   // submit button label (default "Save")
	Success string   // optional message shown after a successful save
	Fields  []Widget // form contents
}

func (*Form) isWidget() {}

// UnmarshalJSON decodes a form's fields recursively through Decode, so an unknown
// field type fails loudly rather than vanishing.
func (f *Form) UnmarshalJSON(data []byte) error {
	var raw struct {
		Submit  string            `json:"submit"`
		Success string            `json:"success"`
		Fields  []json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Submit = raw.Submit
	f.Success = raw.Success
	f.Fields = make([]Widget, 0, len(raw.Fields))
	for i, rf := range raw.Fields {
		field, err := Decode(rf)
		if err != nil {
			return fmt.Errorf("form field %d: %w", i, err)
		}
		f.Fields = append(f.Fields, field)
	}
	return nil
}
