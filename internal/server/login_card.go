// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "github.com/we-are-mono/verso/internal/widget"

// loginRefusal is why a sign-in did not go through, said where its cause is.
// Wrong credentials are the password's answer (Field), under it. A refused
// request — another site's form, the attempt limit, an unreadable form — is
// no fault of what was typed, so it opens the form (Form) and marks no field.
type loginRefusal struct {
	Field string
	Form  string
}

// signIn is what the sign-in card says, in English source strings the
// renderer translates: why the visitor is here, what was refused, and the
// router's public facts. Attempts is already in the reader's words, its count
// composed into the sentence.
type signIn struct {
	Notice         string // how the visitor got here: a session that ended (expiryNotice)
	Refusal        loginRefusal
	Username       string
	Attempts       string
	Status         loginStatus
	Firmware       string
	VisitorIP      string
	VisitorNetwork string
}

// loginCard is the sign-in page's one card, built from the shell's own
// widgets so it reads as every other card does: the form's rows, its
// refusals and its act as the ledger's last row, then the router's public
// facts as a part of the card.
func loginCard(in signIn) widget.Widget {
	var children []widget.Widget
	if in.Notice != "" {
		children = append(children, &widget.Callout{Variant: "warning", Body: in.Notice})
	}
	children = append(children,
		&widget.Form{
			Action: "/login", Submit: "Sign in", Error: in.Refusal.Form,
			Note: in.Attempts, NoteVerbatim: true,
			Fields: []widget.Widget{
				&widget.Field{Name: "username", Label: "Username", Value: in.Username, Autocomplete: "username"},
				&widget.Field{Name: "password", Label: "Password", Kind: "password", Autocomplete: "current-password", Autofocus: true, Error: in.Refusal.Field},
			},
		},
		&widget.Section{Title: "This router", Icon: "router", Children: []widget.Widget{
			&widget.Properties{Align: "left", Items: loginFacts(in)},
		}},
	)
	return &widget.Section{Title: "Sign in", Icon: "lock", Children: children}
}

// loginFacts are the router's public facts. The live ones carry their key, so
// the page's refresh rewrites them in place (verso-login.js).
func loginFacts(in signIn) []widget.Property {
	words, tone := internetState(in.Status.Internet)
	up := in.Status.secondsUp
	firmware := in.Firmware
	if firmware == "" {
		firmware = "OpenWrt"
	}
	facts := []widget.Property{
		{Label: "Internet", Value: words, Dot: tone, Key: "internet"},
		{Label: "Running for", Duration: &up, Key: "uptime"},
		{Label: "Local time", Value: in.Status.Clock + " " + in.Status.Zone, Mono: true, Key: "clock"},
		{Label: "Firmware", Value: firmware, Mono: true},
		{Label: "You are", Value: in.VisitorIP, Mono: true},
	}
	if in.VisitorNetwork != "" {
		facts = append(facts, widget.Property{Label: "Your network", Value: in.VisitorNetwork, Mono: true})
	}
	return facts
}

// internetState is the uplink in words and in its packet's tone: online,
// offline, or unknown — the hollow packet — when the router cannot say.
func internetState(up *bool) (words, tone string) {
	switch {
	case up == nil:
		return "Unknown", "unknown"
	case *up:
		return "Online", "success"
	}
	return "Offline", "danger"
}
