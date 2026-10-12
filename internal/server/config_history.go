// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"log"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
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
// the uci changes it stands for and what each replaced, and who applied it
// from where. It is read before the apply, while the configs still hold what
// the stage replaces.
func (s *Server) stagedChange(ctx context.Context, sid, user, address string) history.Change {
	identity := func(s string) string { return s }
	view := s.staged(ctx, sid, identity, func(string) func(string) string { return identity })
	secrets := s.secretOptions()
	change := history.Change{User: user, Address: address}
	var configs []string
	seen := map[string]bool{}
	for _, g := range view.Groups {
		for _, item := range g.Items {
			kept := history.ChangeItem{Page: g.Page, Plugin: item.plugin, Verb: item.Verb, Words: item.Label, Verbatim: item.Mono,
				SubjectKind: item.subject.Kind, SubjectName: item.subject.Name}
			for _, c := range item.changes {
				kept.Ops = append(kept.Ops, changeOp(c, secrets))
				if !seen[c.config] && describeChange(c).Op != "file" {
					seen[c.config] = true
					configs = append(configs, c.config)
				}
			}
			change.Items = append(change.Items, kept)
		}
	}
	if len(configs) == 0 {
		return change
	}
	// What the configs held is the history's to keep, not the apply's to
	// wait on: a read that fails leaves the change without old values.
	committed, err := s.backend.UCICommitted(ctx, sid, configs)
	if err != nil {
		log.Printf("verso: what the apply replaces could not be read: %v", err)
		return change
	}
	for i := range change.Items {
		for j := range change.Items[i].Ops {
			op := &change.Items[i].Ops[j]
			op.Old = replaced(committed, *op)
		}
	}
	return change
}

// subjectOf is what a described row is about, as config history keeps it: the
// subject its plugin named, called by the section of its first change where
// the plugin gave no name. A kind outside the vocabulary is no subject, and
// the row is named by its config and section, as an undescribed row is.
func subjectOf(named *plugin.Subject, first stagedChange) plugin.Subject {
	if named == nil || !plugin.SubjectKinds[named.Kind] {
		return plugin.Subject{}
	}
	subject := *named
	if subject.Name == "" {
		subject.Name = describeChange(first).Section
	}
	return subject
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

// replaced is what op's target held committed, as JSON: the option's value
// for a change to an option, the whole section for a section removed. A new
// section, a file, and anything that was not there replaced nothing.
func replaced(committed map[string]map[string]any, op history.ChangeOp) json.RawMessage {
	section, ok := committed[op.Config][op.Section].(map[string]any)
	if !ok {
		return nil
	}
	var old any
	switch op.Op {
	case "set", "list-add", "list-del", "remove-option":
		value, present := section[op.Option]
		if !present {
			return nil
		}
		old = value
	case "remove-section":
		old = section
	default:
		return nil
	}
	encoded, err := json.Marshal(old)
	if err != nil {
		return nil
	}
	return encoded
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
