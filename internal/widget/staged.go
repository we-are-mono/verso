// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

// optionWriter is a control that writes one uci option: where the option
// lives (its own target, "config.section", which may be empty) and its key.
// It takes the stage's word on whether that option waits to be applied.
type optionWriter interface {
	writes() (target, key string)
	markStaged()
}

// optionRows is a widget whose rows write options without being widgets of
// their own — a settings block's editable rows.
type optionRows interface {
	optionWriters() []optionWriter
}

// targetHolder is a container that says where the options inside it live, so
// a form or section that writes one uci section says so once and its controls
// only name their keys.
type targetHolder interface {
	defaultTarget() string
}

// MarkStaged marks every control whose option waits on the stage. waits is
// asked of the option's full address, "config.section.option"; a control's
// own target wins over the one its form or section holds, and a control with
// no key, or nowhere to live, is never marked. The shell calls it before a
// page is drawn, so a change stays marked on every visit until it is applied
// or discarded.
func MarkStaged(w Widget, waits func(address string) bool) {
	markStaged(w, "", waits)
}

func markStaged(w Widget, at string, waits func(string) bool) {
	if w == nil {
		return
	}
	if holder, ok := w.(targetHolder); ok && holder.defaultTarget() != "" {
		at = holder.defaultTarget()
	}
	if writer, ok := w.(optionWriter); ok {
		markWriter(writer, at, waits)
	}
	if rows, ok := w.(optionRows); ok {
		for _, writer := range rows.optionWriters() {
			markWriter(writer, at, waits)
		}
	}
	for _, child := range w.children() {
		markStaged(child, at, waits)
	}
}

func markWriter(writer optionWriter, at string, waits func(string) bool) {
	target, key := writer.writes()
	if target == "" {
		target = at
	}
	if target != "" && key != "" && waits(target+"."+key) {
		writer.markStaged()
	}
}

func (f *Field) writes() (string, string)       { return f.Target, f.Key }
func (f *Field) markStaged()                    { f.Staged = true }
func (l *List) writes() (string, string)        { return l.Target, l.Key }
func (l *List) markStaged()                     { l.Staged = true }
func (s *Switch) writes() (string, string)      { return s.Target, s.Key }
func (s *Switch) markStaged()                   { s.Staged = true }
func (c *Conditional) writes() (string, string) { return "", c.Key }
func (c *Conditional) markStaged()              { c.Staged = true }

// writes is the option an editing row writes; a row that only reads writes
// none, whatever its chip says.
func (it *SettingsItem) writes() (string, string) {
	if it.edits() {
		return "", it.Code
	}
	return "", ""
}
func (it *SettingsItem) markStaged() { it.Staged = true }

func (s *Settings) optionWriters() []optionWriter {
	var out []optionWriter
	collect := func(items []SettingsItem) {
		for i := range items {
			out = append(out, &items[i])
		}
	}
	collect(s.Items)
	if s.Seam != nil {
		collect(s.Seam.Items)
	}
	return out
}

func (f *Form) defaultTarget() string    { return f.Target }
func (s *Section) defaultTarget() string { return s.Target }
