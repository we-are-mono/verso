// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

// Wizard walks a person through a short sequence of steps one at a time, with a
// progress bar and Back/Continue navigation — the humane way to ask for several
// things without one intimidating wall of fields (e.g. name a device, choose what
// it can reach, then scan a QR). It is pure CSS (ADR-005 §7): a radio group holds
// the current step and the Back/Continue buttons are labels that select the
// adjacent step, so it needs no JavaScript. Composes naturally inside a modal.
type Wizard struct {
	Steps []WizardStep
}

// WizardStep is one screen of the sequence: the widget subtree shown while that
// step is active.
type WizardStep struct {
	Children []Widget
}

func (*Wizard) isWidget() {}

// UnmarshalJSON decodes each step's children recursively through Decode, so an
// unknown child type fails loudly rather than vanishing (as modal/tabs do).
func (w *Wizard) UnmarshalJSON(data []byte) error {
	var raw struct {
		Steps []struct {
			Children []json.RawMessage `json:"children"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	w.Steps = make([]WizardStep, 0, len(raw.Steps))
	for i, rs := range raw.Steps {
		step := WizardStep{Children: make([]Widget, 0, len(rs.Children))}
		for j, rc := range rs.Children {
			child, err := Decode(rc)
			if err != nil {
				return fmt.Errorf("wizard step %d child %d: %w", i, j, err)
			}
			step.Children = append(step.Children, child)
		}
		w.Steps = append(w.Steps, step)
	}
	return nil
}

// wizStepView is one rendered wizard step: its body (trusted HTML), the ids of the
// adjacent steps' radios (which the Back/Continue labels select), and whether each
// exists (first step has no Back, last has no Continue — it finishes instead).
type wizStepView struct {
	Body             template.HTML
	PrevID, NextID   string
	HasBack, HasNext bool
}

// wizardView is the wizard template's model: a group name unique to this render, the
// step count (for the progress bar), and the rendered steps.
type wizardView struct {
	Group string
	N     int
	Steps []wizStepView
}

// renderInto renders each step's subtree through the renderer, then hands the
// template a radio group whose :checked state (pure CSS) shows one step at a time.
// The plugin declared the steps; the progress bar and navigation are the shell's
// (ADR-005 §7).
func (w *Wizard) renderInto(r *Renderer, out io.Writer, csrf string) error {
	group := fmt.Sprintf("verso-wizard-%d", r.seq.wiz.Add(1))
	n := len(w.Steps)
	steps := make([]wizStepView, 0, n)
	for i, s := range w.Steps {
		body, err := r.renderChildren(s.Children, csrf)
		if err != nil {
			return err
		}
		steps = append(steps, wizStepView{
			Body:    joinHTML(body),
			PrevID:  fmt.Sprintf("%s-%d", group, i-1),
			NextID:  fmt.Sprintf("%s-%d", group, i+1),
			HasBack: i > 0,
			HasNext: i < n-1,
		})
	}
	return r.execute(out, "wizard.html.tmpl", wizardView{Group: group, N: n, Steps: steps})
}
