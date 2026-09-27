// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
	"strings"
)

// fieldFrame is one setting's row as every form surface draws it (the
// verso-field partial): the label — the setting's name, the option it writes
// as a key chip, the stage's mark while its change waits, the explanation the
// label raises — then the control, a description kept in view, the refusal
// band and the remove lane. A widget that is one setting draws only its
// control and hands it over whole, so a value to type, a list to edit and a
// state to flip cannot come to disagree about the rest of the row.
type fieldFrame struct {
	Label  fieldLabel
	Change fieldChange
	// Toggle puts a checkbox before its label rather than under it.
	Toggle bool
	// Class adds a surface's own hooks to the row (a settings block's last row,
	// a conditional's gate); the row's geometry is the frame's.
	Class   string
	Measure string // the shell's content measure for the control (ControlMeasure)
	// Inline marks a value that stages its own change: the refusal band waits
	// under it, empty, for the value's own check to fill.
	Inline  bool
	Control template.HTML
	// Desc is a sentence kept in view under the row. A field raises its help
	// onto the label instead; a settings row's description is its point.
	Desc string
	// Errors are the refusals under the control, one band each, each named by
	// the id its own input points at.
	Errors []fieldError
	// Lane reserves the trailing remove glyph's width; Removable draws it.
	Lane, Removable bool
}

// fieldError is one refusal under a control and the id its input points at.
type fieldError struct {
	ID, Text string
}

// fieldChange is what the stage reads off a row: whether it tracks a change at
// all (a locked value posts nothing), the name it posts under, the words the
// review names it by, what kind of control it is, and the option a live
// preview patches as it changes. The remove glyph names its row by the same
// Name and Label.
type fieldChange struct {
	Track                     bool
	Name, Label, Kind, Writes string
}

// ErrorID is the id the control points at for the refusal under it.
func (f fieldFrame) ErrorID() string { return f.Label.For + "-error" }

// DescID is the id the control points at for the description under it. A
// control with no id of its own has nothing to point from, so the description
// takes none.
func (f fieldFrame) DescID() string {
	if f.Label.For == "" {
		return ""
	}
	return f.Label.For + "-desc"
}

// ShowsLabel reports whether the row has a label column at all. A bare
// control (a field with neither a name nor a key) is still one row.
func (f fieldFrame) ShowsLabel() bool { return f.Label.Label != "" || f.Label.Key != "" }

// boxView is one box holding one or more typed values (verso-box): a field
// counted in a unit, or a group of related values fused into one control.
// Group is the id of the fused row's label, which names the box, and
// Described the id of the explanation that label raises.
type boxView struct {
	Parts     []boxPart
	Group     string
	Described string
}

// boxPart is one value in a box. Track puts the change hooks on the part
// itself, because in a fused box the row holds several changes; Named gives
// the part its own name, because the row's label names the group.
type boxPart struct {
	*Field
	Track, Named bool
}

// Measure is the part's width: a secret's is its own, narrower than a lone
// secret field's so a new password and its repeat sit side by side; any
// other part takes its value's measure.
func (p boxPart) Measure() string {
	if p.Kind == "password" {
		return "secret"
	}
	return p.Field.Measure()
}

// ChangeKind is what the stage is told the part holds.
func (p boxPart) ChangeKind() string {
	if p.Kind == "" {
		return "text"
	}
	return p.Kind
}

// DescribedBy is what a part is read with: its refusal and its unit. A
// fused part's explanation is raised onto the group's label, which the box
// points at, so the part does not point at a tip of its own.
func (p boxPart) DescribedBy() string {
	if !p.Named {
		return p.Field.DescribedBy()
	}
	ids := make([]string, 0, 2)
	if p.Error != "" {
		ids = append(ids, p.Name+"-error")
	}
	if p.Unit != "" {
		ids = append(ids, p.Name+"-unit")
	}
	return strings.Join(ids, " ")
}

// renderFrame draws a control through its own template and the row around it
// through the frame.
func (r *Renderer) renderFrame(out io.Writer, control string, data any, frame fieldFrame) error {
	var b strings.Builder
	if err := r.execute(&b, control, data); err != nil {
		return err
	}
	frame.Control = template.HTML(b.String())
	return r.execute(out, "verso-field", frame)
}
