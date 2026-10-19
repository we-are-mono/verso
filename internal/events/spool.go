// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/sys/unix"

	"github.com/we-are-mono/verso/internal/plugin"
)

// What root announces reaches the shell through the events spool (ADR-028):
// one file an announcement, which the shell takes oldest first and records.
// The helper's init makes the spool at boot, the shell's account its owner,
// mode 0700, so no plugin's account may enter it; root writes past the mode.

// SpoolDir is the events spool.
const SpoolDir = "/var/run/verso/events"

// SpoolLimit is the most announcements that wait for the shell.
const SpoolLimit = 256

// FormatVersion is the announcement format this shell writes and reads.
const FormatVersion = 1

// The bounds of what an announcement carries: its arguments reach an alert's
// Title header, so none holds a control character.
const (
	maxArgs   = 8
	maxArgLen = 256
	// maxAnnouncement bounds an announcement file the shell reads.
	maxAnnouncement = 8 << 10
)

// spoolName is the name an announcement takes in the spool: when it was
// announced, then who announced it and a random tail, so two in one
// nanosecond never meet.
var spoolName = regexp.MustCompile(`^\d{20}-\d+-[0-9a-f]{8}\.json$`)

// IsSpoolName reports whether name is an announcement's in the spool, rather
// than a temporary being written or anything else.
func IsSpoolName(name string) bool { return spoolName.MatchString(name) }

// Announcement is one event as root announced it.
type Announcement struct {
	V       int      `json:"v"`
	Time    int64    `json:"time"`
	Kind    string   `json:"kind"`
	Subject string   `json:"subject,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// ownerName is what may own an event: a plugin's id, or one of core's.
var ownerName = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// eventName is the part of a kind after its owner.
var eventName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// Writer writes announcements into a spool, for root alone.
type Writer struct {
	Dir string
	// PluginsDir holds each plugin's commands.json, its declarations.
	PluginsDir string
	// ShellUID is the account the spool must belong to.
	ShellUID int
	EUID     func() int
	Now      func() time.Time
}

// NewWriter writes into the router's spool, as the process it runs in.
func NewWriter(pluginsDir string, shellUID int) Writer {
	return Writer{Dir: SpoolDir, PluginsDir: pluginsDir, ShellUID: shellUID, EUID: os.Geteuid, Now: time.Now}
}

// Announce leaves kind, about subject, with args filling its sentence, for the
// shell, or says why it will not. Nothing is written when it refuses.
func (w Writer) Announce(kind, subject string, args []string) (Announcement, error) {
	if w.EUID() != 0 {
		return Announcement{}, errors.New("the router's events are announced by root alone")
	}
	sentence, err := w.sentence(kind)
	if err != nil {
		return Announcement{}, err
	}
	if err := checkArgs(sentence, args); err != nil {
		return Announcement{}, err
	}
	if subject != "" && !DeviceName(subject) {
		return Announcement{}, fmt.Errorf("the subject %q is not a device name", subject)
	}
	a := Announcement{V: FormatVersion, Time: w.Now().UnixNano(), Kind: kind, Subject: subject, Args: args}
	body, err := json.Marshal(a)
	if err != nil {
		return Announcement{}, err
	}
	return a, w.write(a.Time, body)
}

// sentence is the sentence kind is declared with: one of core's root may
// announce, or its plugin's declaration.
func (w Writer) sentence(kind string) (string, error) {
	owner, name, ok := strings.Cut(kind, ".")
	if !ok || !ownerName.MatchString(owner) || !eventName.MatchString(name) {
		return "", fmt.Errorf("%q is no kind of event: <owner>.<event>", kind)
	}
	if slices.Contains(plugin.CoreNames, owner) {
		if kind == CertificateReplaced {
			return certificateSentence, nil
		}
		return "", fmt.Errorf("%s is the router's own to record; root does not announce it", kind)
	}
	k, ok := declaredOnDisk(w.PluginsDir, kind)
	if !ok {
		return "", fmt.Errorf("%s: no installed plugin declares it in its commands.json", kind)
	}
	return k.declared.Sentence, nil
}

// checkArgs holds args to the sentence they fill: one for each %s, each one
// line of text, short, with no control character.
func checkArgs(sentence string, args []string) error {
	if len(args) > maxArgs {
		return fmt.Errorf("an event carries at most %d arguments", maxArgs)
	}
	if want := strings.Count(sentence, "%s"); len(args) != want {
		return fmt.Errorf("%q takes %d arguments, not %d", sentence, want, len(args))
	}
	for i, a := range args {
		if len(a) > maxArgLen {
			return fmt.Errorf("argument %d is longer than %d bytes", i+1, maxArgLen)
		}
		if strings.ContainsFunc(a, unicode.IsControl) {
			return fmt.Errorf("argument %d holds a control character", i+1)
		}
	}
	return nil
}

// DeviceName reports whether s is a network device's name as the kernel takes
// one (dev_valid_name): at most 15 bytes, not "." or "..", with no slash,
// colon or space.
func DeviceName(s string) bool {
	return s != "" && len(s) <= 15 && s != "." && s != ".." && !strings.ContainsFunc(s, func(r rune) bool {
		return r == '/' || r == ':' || unicode.IsSpace(r)
	})
}

// write puts body in the spool under the name for at. Every name is reached
// through the spool's descriptor, checked once opened, never by its path
// again: the shell's account owns the spool, and a link swapped in at that
// name must not take root's write anywhere else.
func (w Writer) write(at int64, body []byte) error {
	dir, err := unix.Open(w.Dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("the events spool %s: %w", w.Dir, err)
	}
	defer func() { _ = unix.Close(dir) }()
	var st unix.Stat_t
	if err := unix.Fstat(dir, &st); err != nil {
		return err
	}
	if int(st.Uid) != w.ShellUID || st.Mode&0o777 != 0o700 {
		return fmt.Errorf("the events spool %s is not the shell's own directory", w.Dir)
	}
	waiting, err := countWaiting(dir)
	if err != nil {
		return err
	}
	if waiting >= SpoolLimit {
		return fmt.Errorf("%d announcements already wait for the shell", SpoolLimit)
	}
	tail := make([]byte, 4)
	if _, err := rand.Read(tail); err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%d-%s.json", at, os.Getpid(), hex.EncodeToString(tail))
	temporary := "." + name
	f, err := unix.Openat(dir, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o644)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(f), temporary)
	written := file.Chmod(0o644)
	if written == nil {
		_, written = file.Write(body)
	}
	if written == nil {
		written = file.Sync()
	}
	if err := file.Close(); written == nil {
		written = err
	}
	if written == nil {
		written = unix.Renameat(dir, temporary, dir, name)
	}
	if written != nil {
		_ = unix.Unlinkat(dir, temporary, 0)
	}
	return written
}

// countWaiting is how many announcements the spool open as dir holds.
func countWaiting(dir int) (int, error) {
	dup, err := unix.Dup(dir)
	if err != nil {
		return 0, err
	}
	d := os.NewFile(uintptr(dup), "events")
	defer func() { _ = d.Close() }()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, name := range names {
		if IsSpoolName(name) {
			n++
		}
	}
	return n, nil
}

// ReadAnnouncement reads the announcement name in the spool dir, which only
// owner may have written: a regular file, not a link, of the format this
// shell reads and within its bound.
func ReadAnnouncement(dir, name string, owner int) (Announcement, error) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return Announcement{}, err
	}
	defer func() { _ = f.Close() }()
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return Announcement{}, err
	}
	switch {
	case st.Mode&unix.S_IFMT != unix.S_IFREG:
		return Announcement{}, errors.New("not a regular file")
	case int(st.Uid) != owner:
		return Announcement{}, fmt.Errorf("written by uid %d, not root", st.Uid)
	case st.Size > maxAnnouncement:
		return Announcement{}, fmt.Errorf("%d bytes, over the %d an announcement may hold", st.Size, maxAnnouncement)
	}
	data := make([]byte, st.Size)
	if _, err := io.ReadFull(f, data); err != nil {
		return Announcement{}, err
	}
	return ParseAnnouncement(data)
}

// ServiceEvent is the command that sends a to procd as the service event
// verso.<owner>.<event>, which a plugin's init script answers with
// procd_add_raw_trigger. Its data, the subject and the arguments, can only
// choose which trigger fires: procd hands the command it runs none of it.
func ServiceEvent(a Announcement) []string {
	args := a.Args
	if args == nil {
		args = []string{}
	}
	msg, _ := json.Marshal(struct {
		Type string `json:"type"`
		Data struct {
			Subject string   `json:"subject"`
			Args    []string `json:"args"`
		} `json:"data"`
	}{Type: "verso." + a.Kind, Data: struct {
		Subject string   `json:"subject"`
		Args    []string `json:"args"`
	}{a.Subject, args}})
	return []string{"/bin/ubus", "call", "service", "event", string(msg)}
}

// ParseAnnouncement reads one announcement, refusing a format this shell
// does not write.
func ParseAnnouncement(data []byte) (Announcement, error) {
	var a Announcement
	if err := json.Unmarshal(data, &a); err != nil {
		return Announcement{}, err
	}
	if a.V != FormatVersion {
		return Announcement{}, fmt.Errorf("format %d is not %d", a.V, FormatVersion)
	}
	if a.Kind == "" || a.Time == 0 {
		return Announcement{}, errors.New("an announcement names its kind and its time")
	}
	return a, nil
}
