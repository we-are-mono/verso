// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Command vulngate is `make vuln`'s verdict: it reads what govulncheck and
// osv-scanner found, sets it against the ignore list, prints the findings, and
// exits non-zero when an untriaged advisory stands. It runs on the build host,
// never on a router.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/vulngate"
)

// files is a flag given once per file.
type files []string

func (f *files) String() string { return strings.Join(*f, " ") }

func (f *files) Set(path string) error {
	*f = append(*f, path)
	return nil
}

func main() {
	var goScans, lockfiles files
	flag.Var(&goScans, "govulncheck", "govulncheck -format json output, once per release target")
	osvScan := flag.String("osv", "", "osv-scanner --format json output")
	flag.Var(&lockfiles, "lockfile", "a file osv-scanner was asked to read, relative to the repository; once per file")
	ignoreList := flag.String("ignore", "", "the list of triaged advisories")
	flag.Parse()
	if len(goScans) == 0 || *osvScan == "" || len(lockfiles) == 0 || *ignoreList == "" {
		flag.Usage()
		os.Exit(2)
	}
	failed, err := run(goScans, *osvScan, lockfiles, *ignoreList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vulngate: %v\n", err)
		os.Exit(2)
	}
	if failed {
		os.Exit(1)
	}
}

// run reads the inputs and writes the verdict, reporting whether a release
// must stop.
func run(goScans []string, osvScan string, lockfiles []string, ignoreList string) (bool, error) {
	root, err := os.Getwd()
	if err != nil {
		return false, err
	}
	scans := make([][]vulngate.Finding, 0, len(goScans)+1)
	for _, path := range goScans {
		goFindings, err := readWith(path, vulngate.ReadGovulncheck)
		if err != nil {
			return false, err
		}
		scans = append(scans, goFindings)
	}
	osvFindings, err := readWith(osvScan, func(r io.Reader) ([]vulngate.Finding, error) {
		return vulngate.ReadOSVScanner(r, root, lockfiles)
	})
	if err != nil {
		return false, err
	}
	scans = append(scans, osvFindings)
	now := time.Now()
	ignores, err := readWith(ignoreList, func(r io.Reader) ([]vulngate.Ignore, error) {
		return vulngate.ParseIgnores(r, now)
	})
	if err != nil {
		return false, err
	}
	verdict := vulngate.Triage(vulngate.Merge(scans...), ignores, now)
	if err := verdict.Write(os.Stdout); err != nil {
		return false, err
	}
	return verdict.Failed(), nil
}

// readWith opens path and hands it to read, naming the file in any error.
func readWith[T any](path string, read func(io.Reader) ([]T, error)) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out, err := read(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}
