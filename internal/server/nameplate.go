// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"unicode"
)

// boardFacts is what this board says about itself without being asked: who built
// it, what they call it, and the id that selects its sensor profile. Maker and
// Model are empty on a board that names neither, and every caller drops those
// lines rather than inventing one.
type boardFacts struct {
	Maker string
	Model string
	Name  string // model.id, the same string `ubus call system board` reports as board_name
}

// board reads /etc/board.json once per process — OpenWrt's board detection
// writes it at first boot and it does not change while the system runs. Reading
// it locally is also what keeps the sensor profile out of the render path: the
// board name is otherwise a ubus round-trip, and the chrome needs it on every
// page.
var board = sync.OnceValue(func() boardFacts {
	data, err := os.ReadFile("/etc/board.json")
	if err != nil {
		return boardFacts{}
	}
	return boardIdentity(data)
})

// boardIdentity splits the board's name into the two lines a nameplate stacks:
// who made it, and what they call it. OpenWrt records both facts but only
// together — model.name is "Mono Gateway Development Kit" while model.id is
// "mono,gdk", a vendor token and a board token. Taking leading words off the
// name until they spell the vendor token is what tells "Raspberry Pi 4 Model B"
// apart from a split at the first space, which would leave "Raspberry" as a
// manufacturer. Boards whose id carries no vendor token (x86, mostly) fall back
// to that first space, and a single-word name is all maker and no model.
func boardIdentity(data []byte) boardFacts {
	var b struct {
		Model struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"model"`
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return boardFacts{}
	}
	facts := boardFacts{Name: strings.TrimSpace(b.Model.ID)}
	words := strings.Fields(b.Model.Name)
	switch len(words) {
	case 0:
		return facts
	case 1:
		facts.Maker = words[0]
		return facts
	}
	if vendor, _, ok := strings.Cut(b.Model.ID, ","); ok {
		if n := vendorWords(words, vendor); n > 0 && n < len(words) {
			facts.Maker, facts.Model = strings.Join(words[:n], " "), strings.Join(words[n:], " ")
			return facts
		}
	}
	facts.Maker, facts.Model = words[0], strings.Join(words[1:], " ")
	return facts
}

// vendorWords counts how many leading words of a board name spell out the
// vendor token from its id, comparing on letters and digits alone so that
// "GL.iNet" answers to "glinet" and "Raspberry Pi" to "raspberrypi". Zero when
// the name does not start with the vendor at all, which happens on boards whose
// id and name disagree; the caller then falls back to the first space.
func vendorWords(words []string, vendor string) int {
	want := foldAlphanumeric(vendor)
	if want == "" {
		return 0
	}
	var got string
	for i, word := range words {
		got += foldAlphanumeric(word)
		switch {
		case got == want:
			return i + 1
		case !strings.HasPrefix(want, got):
			return 0
		}
	}
	return 0
}

// foldAlphanumeric reduces a word to its letters and digits, lowercased — the
// form in which a vendor token and the name it was derived from can be compared.
func foldAlphanumeric(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// nameplate reads the running hostname on every render. The native read is a
// local syscall, so applied renames and rollbacks need no cache invalidation.
// A read failure leaves the template's localized "This device" fallback.
func (s *Server) nameplate(r *http.Request) string {
	name, err := s.backend.Hostname(r.Context(), s.sessionSID(r))
	if err != nil {
		return ""
	}
	return name
}
