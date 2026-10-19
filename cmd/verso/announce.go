// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/we-are-mono/verso/internal/events"
)

const announceUsage = "usage: verso announce <owner>.<event> [--subject <device>] [<arg>...]"

// raiseTimeout bounds telling procd of an event.
const raiseTimeout = 5 * time.Second

// announce is `verso announce`, root's one way to say an event happened
// (ADR-028): the event is left for the shell to record, then sent to procd as
// the service event verso.<owner>.<event>, which other plugins' triggers
// answer. The event stands whether or not procd is told.
func announce(args []string, w events.Writer, run func(context.Context, []string) error, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(announceUsage)
	}
	kind := args[0]
	flags := flag.NewFlagSet("announce", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	subject := flags.String("subject", "", "the device the event is on")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("%w\n%s", err, announceUsage)
	}
	a, err := w.Announce(kind, *subject, flags.Args())
	if err != nil {
		return err
	}
	raise(a, run, stderr)
	return nil
}

// raise sends a to procd, saying on stderr if procd was not told.
func raise(a events.Announcement, run func(context.Context, []string) error, stderr io.Writer) {
	ctx, cancel := context.WithTimeout(context.Background(), raiseTimeout)
	defer cancel()
	if err := run(ctx, events.ServiceEvent(a)); err != nil {
		fmt.Fprintf(stderr, "verso: announce: %s is recorded, but procd was not told: %v\n", a.Kind, err)
	}
}

// runQuietly runs argv, its output discarded, its failure an error.
func runQuietly(ctx context.Context, argv []string) error {
	return exec.CommandContext(ctx, argv[0], argv[1:]...).Run() //nolint:gosec // argv is built by events.ServiceEvent from a checked announcement
}
