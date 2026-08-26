// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package rpcdhelper implements the rpcd exec-plugin protocol for Verso's
// privileged actions — the small root-owned surface the unprivileged shell
// cannot perform itself (ADR-007). rpcd runs the plugin as root and names the
// ubus object after its install filename (/usr/libexec/rpcd/verso → object
// "verso"). rpcd does not itself enforce the session ACL on exec plugins, so the
// helper self-gates every call through session.access. Add a root action by
// registering another method — the gating and protocol are shared.
package rpcdhelper

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
)

// Exit codes are ubus status codes: rpcd maps the plugin's exit code onto the
// ubus reply status, so the caller sees a proper error rather than a bare result.
const (
	statusOK               = 0
	statusInvalidArgument  = 2
	statusMethodNotFound   = 3
	statusPermissionDenied = 6
	statusUnknownError     = 9
)

// Authorizer reports whether a session may invoke a helper method. The helper
// self-gates every call: it runs as root, and ubusd exempts root from its uid
// ACL, so authorization must come from the session's rpcd ACL — checked here.
type Authorizer interface {
	// Access reports whether sid may call verso.<function>.
	Access(sid, function string) (bool, error)
}

// PasswordSetter sets a system password. Split behind an interface so the
// protocol is testable without mutating the host (ADR-003).
type PasswordSetter interface {
	SetPassword(username, password string) error
}

// PackageManager wraps the system package manager — apk on the 25.12 target
// (ADR-011 §4). Split behind an interface so the protocol is testable without
// touching the host's package database.
type PackageManager interface {
	// Update refreshes the feed indexes from the network.
	Update() error
	// Search lists packages matching the query (substring on the name), at
	// most limit, with descriptions filled in.
	Search(query string, limit int) ([]Package, int, error)
	// Installed lists every installed package (descriptions skipped — the
	// full set in one exec).
	Installed() ([]Package, error)
	// Install and Remove act on one exact package name and return the
	// manager's output tail for the caller to surface.
	Install(name string) (string, error)
	Remove(name string) (string, error)
	// CheckedAt is the unix mtime of the freshest feed index — the "checked
	// N ago" honesty on the Discover face. Zero means never.
	CheckedAt() int64
}

// Package is one row of a package search or the installed listing. The
// detail fields (license, webpage, size) ride along where the source provides
// them in bulk — the drawers are server-rendered, so detail must be cheap.
type Package struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Feed        string `json:"feed"`
	Description string `json:"description"`
	License     string `json:"license,omitempty"`
	Webpage     string `json:"webpage,omitempty"`
	Size        int64  `json:"size,omitempty"` // package file size, bytes
	Installed   bool   `json:"installed"`
}

// pkgNameRe is the exact shape of an installable package name — nothing else
// reaches the package manager's argv. Globs, options, and path characters are
// untypeable by construction.
var pkgNameRe = regexp.MustCompile(`^[a-z0-9][a-zA-Z0-9._+-]{0,63}$`)

// pkgQueryRe is the searchable shape — the same alphabet, anywhere in the name.
var pkgQueryRe = regexp.MustCompile(`^[a-zA-Z0-9._+-]{1,64}$`)

// pkgKeep is the base set the remove verb refuses to touch: losing any of
// these severs the device or this very control surface.
var pkgKeep = map[string]bool{
	"busybox": true, "apk": true, "procd": true, "ubusd": true, "ubus": true,
	"rpcd": true, "uhttpd": true, "netifd": true, "libc": true, "musl": true,
}

// statusError carries a ubus status for an action failure so Run can map it to
// the right exit code.
type statusError struct {
	code int
	msg  string
}

func (e statusError) Error() string { return e.msg }

func argErr(format string, a ...any) error {
	return statusError{statusInvalidArgument, fmt.Sprintf(format, a...)}
}

// method is one privileged action: the arg schema rpcd advertises in `list`, and
// the action itself.
type method struct {
	signature map[string]string
	run       func(args map[string]any) (any, error)
}

// Helper dispatches the rpcd exec-plugin protocol across a registry of methods.
type Helper struct {
	auth    Authorizer
	methods map[string]method
}

// New builds the helper with the standard method set. Register further root
// actions here; each is self-gated identically.
func New(auth Authorizer, pw PasswordSetter, pkgs PackageManager) *Helper {
	h := &Helper{auth: auth, methods: map[string]method{}}

	h.methods["setPassword"] = method{
		signature: map[string]string{"username": "", "password": ""},
		run: func(args map[string]any) (any, error) {
			username, _ := args["username"].(string)
			password, _ := args["password"].(string)
			if username == "" || password == "" {
				return nil, argErr("username and password are required")
			}
			if err := pw.SetPassword(username, password); err != nil {
				return nil, err
			}
			return map[string]any{"result": true}, nil
		},
	}

	// The package verbs (ADR-011 §4): the one privileged path to apk. Names
	// and queries are validated to an exact alphabet before touching argv.
	h.methods["pkgStatus"] = method{
		signature: map[string]string{},
		run: func(map[string]any) (any, error) {
			return map[string]any{"checked_at": pkgs.CheckedAt()}, nil
		},
	}
	h.methods["pkgUpdate"] = method{
		signature: map[string]string{},
		run: func(map[string]any) (any, error) {
			if err := pkgs.Update(); err != nil {
				return nil, err
			}
			return map[string]any{"result": true, "checked_at": pkgs.CheckedAt()}, nil
		},
	}
	h.methods["pkgInstalled"] = method{
		signature: map[string]string{},
		run: func(map[string]any) (any, error) {
			pkgsFound, err := pkgs.Installed()
			if err != nil {
				return nil, err
			}
			return map[string]any{"packages": pkgsFound}, nil
		},
	}
	h.methods["pkgSearch"] = method{
		signature: map[string]string{"query": ""},
		run: func(args map[string]any) (any, error) {
			q, _ := args["query"].(string)
			if !pkgQueryRe.MatchString(q) {
				return nil, argErr("query must match %s", pkgQueryRe)
			}
			pkgsFound, total, err := pkgs.Search(q, searchLimit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"packages": pkgsFound, "total": total}, nil
		},
	}
	h.methods["pkgInstall"] = method{
		signature: map[string]string{"package": ""},
		run: func(args map[string]any) (any, error) {
			name, _ := args["package"].(string)
			if !pkgNameRe.MatchString(name) {
				return nil, argErr("package must match %s", pkgNameRe)
			}
			out, err := pkgs.Install(name)
			if err != nil {
				return nil, err
			}
			return map[string]any{"result": true, "output": out}, nil
		},
	}
	h.methods["pkgRemove"] = method{
		signature: map[string]string{"package": ""},
		run: func(args map[string]any) (any, error) {
			name, _ := args["package"].(string)
			if !pkgNameRe.MatchString(name) {
				return nil, argErr("package must match %s", pkgNameRe)
			}
			if pkgKeep[name] {
				return nil, argErr("%s is part of the device's base and stays", name)
			}
			out, err := pkgs.Remove(name)
			if err != nil {
				return nil, err
			}
			return map[string]any{"result": true, "output": out}, nil
		},
	}

	return h
}

// searchLimit caps one search's rows (and its per-package description reads).
const searchLimit = 30

// Run executes the rpcd protocol for one invocation and returns the process exit
// code. args is os.Args[1:]: "list" to advertise methods, or "call <method>" with
// the request JSON (including ubus_rpc_session) on stdin.
func (h *Helper) Run(args []string, stdin io.Reader, stdout io.Writer) int {
	if len(args) == 0 {
		return statusInvalidArgument
	}
	switch args[0] {
	case "list":
		h.list(stdout)
		return statusOK
	case "call":
		if len(args) < 2 {
			return statusInvalidArgument
		}
		return h.call(args[1], stdin, stdout)
	default:
		return statusInvalidArgument
	}
}

func (h *Helper) list(out io.Writer) {
	sig := make(map[string]map[string]string, len(h.methods))
	for name, m := range h.methods {
		sig[name] = m.signature
	}
	_ = json.NewEncoder(out).Encode(sig)
}

func (h *Helper) call(name string, stdin io.Reader, out io.Writer) int {
	m, ok := h.methods[name]
	if !ok {
		return statusMethodNotFound
	}

	// Missing/garbled args decode to an empty map: the self-gate below then fails
	// (no sid), so an argument-less call is denied rather than acted on.
	var args map[string]any
	if err := json.NewDecoder(stdin).Decode(&args); err != nil {
		args = map[string]any{}
	}

	sid, _ := args["ubus_rpc_session"].(string)
	if allowed, err := h.auth.Access(sid, name); err != nil || !allowed {
		return statusPermissionDenied
	}

	res, err := m.run(args)
	if err != nil {
		if se, ok := err.(statusError); ok {
			return se.code
		}
		return statusUnknownError
	}
	if res != nil {
		_ = json.NewEncoder(out).Encode(res)
	}
	return statusOK
}
