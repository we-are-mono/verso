// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"

	"github.com/we-are-mono/verso/internal/history"
)

// changeRecorder keeps config history (ADR-021): history.db's, or a test's.
type changeRecorder interface {
	RecordChange(ctx context.Context, c history.Change) (int64, error)
}

// baseSecrets are the options OpenWrt's own packages keep a secret in, as
// config.option. Config history records that they changed and never their
// values, whether or not a plugin's manifest names them too.
var baseSecrets = []string{
	"network.password", "network.private_key", "network.preshared_key", "network.pincode",
	"wireless.key", "wireless.key1", "wireless.key2", "wireless.key3", "wireless.key4",
	"wireless.sae_password", "wireless.password", "wireless.auth_secret", "wireless.acct_secret",
	"wireless.priv_key_pwd", "wireless.private_key_passwd",
	"ddns.password",
	"rpcd.password",
	"acme.credentials",
}

// secretOptions is every option config history keeps no value of: OpenWrt's
// own, and each installed plugin's.
func (s *Server) secretOptions() map[string]bool {
	secrets := make(map[string]bool, len(baseSecrets))
	for _, name := range baseSecrets {
		secrets[name] = true
	}
	for _, m := range s.manifestList() {
		for _, name := range m.Secrets {
			secrets[name] = true
		}
	}
	return secrets
}

// stagedChange is sid's stage as config history keeps it once applied: the
// review drawer's rows, under the page each was made on in English, each with
// the uci changes it stands for, and who applied it from where.
func (s *Server) stagedChange(ctx context.Context, sid, user, address string) history.Change {
	identity := func(s string) string { return s }
	view := s.staged(ctx, sid, identity, func(string) func(string) string { return identity })
	secrets := s.secretOptions()
	change := history.Change{User: user, Address: address}
	for _, g := range view.Groups {
		for _, item := range g.Items {
			kept := history.ChangeItem{Page: g.Page, Plugin: item.plugin, Verb: item.Verb, Words: item.Label, Verbatim: item.Mono}
			for _, c := range item.changes {
				kept.Ops = append(kept.Ops, changeOp(c, secrets))
			}
			change.Items = append(change.Items, kept)
		}
	}
	return change
}

// changeOp is one staged uci change as config history keeps it. A value is
// kept for the ops that carry one; a section's type rides as its option, as
// the describe hook has it.
func changeOp(c stagedChange, secrets map[string]bool) history.ChangeOp {
	d := describeChange(c)
	op := history.ChangeOp{Config: d.Config, Section: d.Section, Op: d.Op, Option: d.Option}
	switch d.Op {
	case "set", "list-add", "list-del", "file":
		value := d.Value
		op.Value = &value
	}
	switch d.Op {
	case "set", "list-add", "list-del", "remove-option":
		op.Secret = secrets[d.Config+"."+d.Option]
	}
	return op
}

// changePages is the pages a change was made on, in the order its rows have
// them.
func changePages(c history.Change) []string {
	var pages []string
	seen := map[string]bool{}
	for _, item := range c.Items {
		if !seen[item.Page] {
			seen[item.Page] = true
			pages = append(pages, item.Page)
		}
	}
	return pages
}
