// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"html/template"
	"io/fs"
	"log"

	"github.com/we-are-mono/verso/internal/theme"
)

// themeState is the router's themes and the one in force, its style rendered
// once, when either changes, so a page only copies it (ADR-024).
type themeState struct {
	set    theme.Set
	choice string
	css    template.CSS
}

// SetThemes reads the themes in fsys and puts choice in force: the shell's
// start, given the themes directory and verso.ui.theme. Rescan reads fsys
// again and keeps the choice.
func (s *Server) SetThemes(fsys fs.FS, choice string) {
	s.themesFS = fsys
	s.useThemes(s.loadThemes(), choice)
}

// reloadThemes reads the themes directory again, keeping the choice in force:
// a theme package installed or removed since.
func (s *Server) reloadThemes() {
	if s.themesFS == nil {
		return
	}
	s.useThemes(s.loadThemes(), s.themeNow().choice)
}

// loadThemes reads the themes directory against the palette's tokens, logging
// each file it leaves out and each place a theme's words fall short.
func (s *Server) loadThemes() theme.Set {
	set, problems := theme.Load(s.themesFS, s.tokens)
	for _, p := range problems {
		log.Printf("verso: %v", p)
	}
	for _, t := range set {
		for _, short := range t.Contrast(s.palette) {
			log.Printf("verso: theme %s", short)
		}
	}
	if _, ok := set[theme.SandDark]; !ok {
		log.Printf("verso: theme %s is not installed, so Sand stays light under a dark preference", theme.SandDark)
	}
	return set
}

// useThemes puts set and choice in force. A choice no theme answers draws as
// Sand following the browser, and the log says which.
func (s *Server) useThemes(set theme.Set, choice string) {
	css, known := set.CSS(choice)
	if !known {
		log.Printf("verso: theme %q is not installed, so the pages follow the browser in Sand", choice)
	}
	s.theme.Store(&themeState{set: set, choice: choice, css: template.CSS(css)})
}

// themeNow is the themes and the choice in force; Sand light before any are
// set.
func (s *Server) themeNow() *themeState {
	if t := s.theme.Load(); t != nil {
		return t
	}
	return &themeState{set: theme.Set{}, choice: theme.SandLight}
}

// themeCSS is the style every page carries after the stylesheet.
func (s *Server) themeCSS() template.CSS { return s.themeNow().css }

// themeBars are the browser bar colours of a page standing on token (canvas
// or ground) under the theme in force.
func (s *Server) themeBars(token string) []theme.Bar {
	t := s.themeNow()
	return t.set.BarColours(t.choice, token, s.palette)
}
