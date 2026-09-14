// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The sign-in page states facts before it knows who is asking, so each source
// has to fail into silence rather than into a wrong claim: a board with no
// board.json shows no model, an unreadable /proc/uptime reads as a fresh boot.

func TestBoardIdentity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		data      string
		wantMaker string
		wantModel string
	}{
		{
			"the vendor token names the maker",
			`{"model":{"id":"mono,gdk","name":"Mono Gateway Development Kit"}}`,
			"Mono", "Gateway Development Kit",
		},
		{
			"a two-word maker survives the split",
			`{"model":{"id":"raspberrypi,4-model-b","name":"Raspberry Pi 4 Model B"}}`,
			"Raspberry Pi", "4 Model B",
		},
		{
			"punctuation in the maker is folded away",
			`{"model":{"id":"glinet,gl-mt6000","name":"GL.iNet GL-MT6000"}}`,
			"GL.iNet", "GL-MT6000",
		},
		{
			"no vendor token falls back to the first space",
			`{"model":{"id":"supermicro-h13sae-mf","name":"Supermicro H13SAE-MF"}}`,
			"Supermicro", "H13SAE-MF",
		},
		{
			"a vendor the name does not start with falls back too",
			`{"model":{"id":"friendlyarm,nanopi-r4s","name":"NanoPi R4S"}}`,
			"NanoPi", "R4S",
		},
		{"one word is all maker", `{"model":{"id":"turris,omnia","name":"Turris"}}`, "Turris", ""},
		{"no model section", `{"network":{"lan":{"device":"eth0"}}}`, "", ""},
		{"not json at all", `this is not json`, "", ""},
		{"empty file", ``, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := boardIdentity([]byte(tc.data))
			if got.Maker != tc.wantMaker || got.Model != tc.wantModel {
				t.Errorf("boardIdentity = (%q, %q), want (%q, %q)", got.Maker, got.Model, tc.wantMaker, tc.wantModel)
			}
		})
	}
}

// The release is named and versioned, never revisioned: r32933-4ccb782af7 says
// nothing to someone signing in.
func TestReleaseName(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{
			"drops the revision",
			"DISTRIB_ID='OpenWrt'\nDISTRIB_RELEASE='25.12.4'\nDISTRIB_REVISION='r32933-4ccb782af7'\nDISTRIB_DESCRIPTION='OpenWrt 25.12.4 r32933-4ccb782af7'\n",
			"OpenWrt 25.12.4",
		},
		{
			"keeps a vendor's own name",
			"DISTRIB_ID='Mono'\nDISTRIB_RELEASE='25.12.4'\n",
			"Mono 25.12.4",
		},
		{
			"a snapshot is a version too",
			"DISTRIB_ID='OpenWrt'\nDISTRIB_RELEASE='SNAPSHOT'\n",
			"OpenWrt SNAPSHOT",
		},
		{
			"no id falls back to the distribution",
			"DISTRIB_RELEASE='25.12.4'\n",
			"OpenWrt 25.12.4",
		},
		{
			"no release keeps the description",
			"DISTRIB_DESCRIPTION='OpenWrt 25.12.4 r32933-4ccb782af7'\n",
			"OpenWrt 25.12.4 r32933-4ccb782af7",
		},
		{"nothing readable", "# a comment\n\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseName([]byte(tc.data)); got != tc.want {
				t.Errorf("releaseName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProcUptimeSeconds(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want int64
	}{
		{"truncates the fraction", "1686383.34 53101308.34\n", 1686383},
		{"a fresh boot", "0.42 0.31\n", 0},
		{"idle field missing", "900.00", 900},
		{"unparseable", "nonsense\n", 0},
		{"negative is not a duration", "-5.00 0.00\n", 0},
		{"empty", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := procUptimeSeconds([]byte(tc.data)); got != tc.want {
				t.Errorf("procUptimeSeconds(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// A clean address says nothing. Anything above zero is stated, with the limit
// that follows it, so the visitor learns both that guessing is happening and
// what the router will do about it.
func TestAttemptsNote(t *testing.T) {
	if got := attemptsNote(identity, 0); got != "" {
		t.Errorf("a clean address should say nothing, got %q", got)
	}
	if got := attemptsNote(identity, -1); got != "" {
		t.Errorf("a negative count should say nothing, got %q", got)
	}
	got := attemptsNote(identity, 3)
	if got != "3 failed sign-ins from your address." {
		t.Errorf("attemptsNote(3) = %q", got)
	}
}

// The page answers with the board's own facts and with what it can see of the
// connection — the value a visitor gets before signing in.
func TestLoginPageStatesPreAuthFacts(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.RemoteAddr = "10.0.10.42:51234"
	req.Host = "10.0.10.1"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname: %v", err)
	}
	for _, want := range []string{
		hostname, // the device name in the public header
		"Uptime", "Local time",
		"10.0.10.42", // only the visitor’s own address
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sign-in page does not state %q", want)
		}
	}

	// Nothing about the network it runs, and no session behind any of it.
	if strings.Contains(body, "br-lan") {
		t.Error("the sign-in page must not describe the router's networks")
	}
}

// identity stands in for a translator in the helper tests: the English source
// string, unchanged, so an assertion reads the copy the handler composed.
func identity(s string) string { return s }
