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
		target, key := writer.writes()
		if target == "" {
			target = at
		}
		if target != "" && key != "" && waits(target+"."+key) {
			writer.markStaged()
		}
	}
	for _, child := range w.children() {
		markStaged(child, at, waits)
	}
}

func (f *Field) writes() (string, string)  { return f.Target, f.Key }
func (f *Field) markStaged()               { f.Staged = true }
func (l *List) writes() (string, string)   { return l.Target, l.Key }
func (l *List) markStaged()                { l.Staged = true }
func (s *Switch) writes() (string, string) { return s.Target, s.Key }
func (s *Switch) markStaged()              { s.Staged = true }

func (f *Form) defaultTarget() string    { return f.Target }
func (s *Section) defaultTarget() string { return s.Target }
