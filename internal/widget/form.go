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
	Submit  string       // submit button label (default "Save")
	Success string       // optional message shown after a successful save
	Error   string       // optional error not tied to a single field, shown above the fields
	Actions []FormAction // secondary submit buttons besides Save (below)
	Fields  []Widget     // form contents
}

// FormAction is a secondary submit button: it submits the form — all its fields —
// with an `_action` marker the plugin reads, so the plugin can compute on the
// submitted values and re-render. It is the plugin-computed round-trip (ADR-005 §7):
// the shell owns the button and forwards the submission; the plugin owns the
// computation (e.g. generating a keypair) and returns fresh schema, not a save.
type FormAction struct {
	Label  string `json:"label"`
	Action string `json:"action"` // posted as _action=<Action>
}

func (*Form) isWidget() {}

// UnmarshalJSON decodes a form's fields recursively through Decode, so an unknown
// field type fails loudly rather than vanishing.
func (f *Form) UnmarshalJSON(data []byte) error {
	var raw struct {
		Submit  string            `json:"submit"`
		Success string            `json:"success"`
		Error   string            `json:"error"`
		Actions []FormAction      `json:"actions"`
		Fields  []json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Submit = raw.Submit
	f.Success = raw.Success
	f.Error = raw.Error
	f.Actions = raw.Actions
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
