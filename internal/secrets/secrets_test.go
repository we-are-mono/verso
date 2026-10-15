// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package secrets

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestEveryNameIsAConfigOption: config.option, each once, as uci names them.
func TestEveryNameIsAConfigOption(t *testing.T) {
	names := Base()
	if len(names) == 0 {
		t.Fatal("the list is empty")
	}
	name := regexp.MustCompile(`^[a-z0-9_-]+\.[A-Za-z0-9_]+$`)
	for i, n := range names {
		if !name.MatchString(n) {
			t.Errorf("%q is not config.option", n)
		}
		if slices.Contains(names[i+1:], n) {
			t.Errorf("%q is listed twice", n)
		}
	}
	if !slices.Contains(names, "wireless.key") {
		t.Error("Wi-Fi's key is missing")
	}
}

// secretish is an option name that may hold a secret.
var secretish = regexp.MustCompile(`secret|key|pin|pwd|passwd|password|kh|psk`)

// notSecrets is every wifi-iface option secretish matches that holds no
// secret, and what it holds instead.
var notSecrets = map[string]string{
	"ft_psk_generate_local":         "a switch",
	"multi_ap_backhaul_ssid":        "the backhaul network's name",
	"ppsk":                          "a switch",
	"priv_key":                      "a key file's path",
	"priv_key2":                     "a key file's path",
	"private_key":                   "a key file's path",
	"private_key2":                  "a key file's path",
	"r0_key_lifetime":               "a lifetime",
	"r1_key_holder":                 "the key holder's identifier",
	"sae_ext_key":                   "a switch",
	"sae_password_file":             "a file's path",
	"sae_track_password":            "a count",
	"wnm_sleep_mode_no_keys":        "a switch",
	"wpa_disable_eapol_key_retries": "a switch",
	"wpa_strict_rekey":              "a switch",
	"wpa_gmk_rekey":                 "an interval",
	"wpa_group_rekey":               "an interval",
	"wpa_master_rekey":              "an interval",
	"wpa_pair_rekey":                "an interval",
	"wpa_ptk_rekey":                 "an interval",
	"wpa_psk_file":                  "a file's path",
}

// carriesSecrets is every wifi-iface option secretish does not match that
// may hold a secret, and how.
var carriesSecrets = map[string]string{
	"hostapd_bss_options": "raw hostapd lines, wpa_passphrase=, sae_password= and auth_server_shared_secret= among them",
}

// wifiIface is the options of OpenWrt's wifi-iface schema, kept in testdata
// as the schema at that path in the OpenWrt tree names them: each as uci
// spells it, the schema's "name:type" cut at the colon.
type wifiIface struct {
	Schema  string   `json:"schema"`
	Options []string `json:"options"`
}

func wifiIfaceOptions(t *testing.T) wifiIface {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "wireless.wifi-iface.json"))
	if err != nil {
		t.Fatal(err)
	}
	var options wifiIface
	if err := json.Unmarshal(data, &options); err != nil {
		t.Fatal(err)
	}
	return options
}

// TestEveryWifiIfaceSecretIsListed: each option of a Wi-Fi network whose
// name says it may hold a secret is listed, under every name uci knows it
// by, or named here as holding none; each named here as carrying one,
// though its name does not say so, is listed.
func TestEveryWifiIfaceSecretIsListed(t *testing.T) {
	listed := map[string]bool{}
	for _, name := range Base() {
		listed[name] = true
	}
	schema := wifiIfaceOptions(t)
	for _, option := range schema.Options {
		if !secretish.MatchString(option) {
			continue
		}
		holds, notSecret := notSecrets[option]
		switch {
		case notSecret && listed["wireless."+option]:
			t.Errorf("wireless.%s is listed, and holds %s", option, holds)
		case !notSecret && !listed["wireless."+option]:
			t.Errorf("wireless.%s may hold a secret, and is not listed", option)
		}
	}
	for option := range notSecrets {
		if !slices.Contains(schema.Options, option) || !secretish.MatchString(option) {
			t.Errorf("%s is no wifi-iface option a secret's name matches", option)
		}
	}
	for option, how := range carriesSecrets {
		switch {
		case !slices.Contains(schema.Options, option) || secretish.MatchString(option):
			t.Errorf("%s is no wifi-iface option a secret's name misses", option)
		case !listed["wireless."+option]:
			t.Errorf("wireless.%s carries %s, and is not listed", option, how)
		}
	}
}

// TestTheWifiIfaceOptionsAreTheSchemas: testdata holds the options of the
// schema in the OpenWrt tree under sources/, where one is checked out.
func TestTheWifiIfaceOptionsAreTheSchemas(t *testing.T) {
	kept := wifiIfaceOptions(t)
	path := filepath.Join("..", "..", "sources", "openwrt", filepath.FromSlash(kept.Schema))
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no OpenWrt tree: %s", path)
	}
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	var options []string
	for name := range schema.Properties {
		option, _, _ := strings.Cut(name, ":")
		options = append(options, option)
	}
	slices.Sort(options)
	options = slices.Compact(options)
	if !slices.Equal(options, kept.Options) {
		t.Errorf("testdata keeps %d options, the schema has %d: %q", len(kept.Options), len(options), options)
	}
}
