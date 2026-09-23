// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"golang.org/x/crypto/ssh"
)

type credentialsFake struct {
	fakeBackend
	data                         openwrt.AccessCredentials
	expected, written, cert, key string
	conflict                     bool
}

func (b *credentialsFake) AccessCredentials(context.Context, string) (openwrt.AccessCredentials, error) {
	return b.data, nil
}
func (b *credentialsFake) SetAuthorizedKeys(_ context.Context, _ string, expected, keys string) error {
	b.expected = expected
	if b.conflict {
		return errors.New("keys changed")
	}
	b.written = keys
	b.data.AuthorizedKeys = keys
	return nil
}
func (b *credentialsFake) SetWebCertificate(_ context.Context, _, cert, key string) error {
	b.cert, b.key = cert, key
	return nil
}
func credentialManifest() plugin.Manifest {
	return plugin.Manifest{ID: "system", ACL: plugin.ACL{Write: []plugin.ACLScope{{Scope: "ubus", Object: "verso", Function: "setAuthorizedKeys"}, {Scope: "ubus", Object: "verso", Function: "setWebCertificate"}}}}
}
func testPublicKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + " test-key"
}
func TestAuthorizedKeyCommandsValidateAndCompareCurrentContents(t *testing.T) {
	b := &credentialsFake{data: openwrt.AccessCredentials{AuthorizedKeys: "# managed keys\n"}}
	s := &Server{backend: b}
	m := credentialManifest()
	ctx := context.Background()
	key := testPublicKey(t)
	command := plugin.ApplyAction{Name: "ssh-key-add", Args: map[string]string{"key": key}}
	if err := s.credentialCommand(ctx, plugin.Manifest{}, "sid", command); err == nil {
		t.Fatal("accepted undeclared write")
	}
	if err := s.credentialCommand(ctx, m, "sid", command); err != nil {
		t.Fatal(err)
	}
	if b.expected != "# managed keys\n" || !strings.Contains(b.written, "# managed keys\n"+key) {
		t.Fatalf("lost existing content: %q", b.written)
	}
	saved := b.written
	b.written = ""
	if err := s.credentialCommand(ctx, m, "sid", command); err != nil || b.written != "" {
		t.Fatalf("duplicate key wrote again: %v", err)
	}
	command.Args["key"] = key + "\n" + testPublicKey(t)
	if err := s.credentialCommand(ctx, m, "sid", command); err == nil {
		t.Fatal("accepted two keys")
	}
	b.conflict = true
	command.Args["key"] = testPublicKey(t)
	if err := s.credentialCommand(ctx, m, "sid", command); err == nil || b.data.AuthorizedKeys != saved {
		t.Fatal("concurrent edit overwritten")
	}
	b.conflict = false
	public, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(key))
	if err := s.credentialCommand(ctx, m, "sid", plugin.ApplyAction{Name: "ssh-key-remove", Args: map[string]string{"fingerprint": ssh.FingerprintSHA256(public)}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.written, key) || !strings.Contains(b.written, "# managed keys") {
		t.Fatalf("wrong removal: %q", b.written)
	}
}

// TestCertificateFactsReadAsTheBrowserDoes: the fingerprint is written the
// way a browser's certificate viewer writes it — SHA-256 of the DER, upper-case
// hex in space-separated pairs — so it can be matched against the browser's
// "not secure" page by eye; and the certificate's life is counted in whole days
// against the clock it is read by.
func TestCertificateFactsReadAsTheBrowserDoes(t *testing.T) {
	pemCert, _, err := generateWebCertificate("router.lan")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(pemCert))
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(c.Raw)
	pairs := make([]string, len(sum))
	for i, b := range sum {
		pairs[i] = fmt.Sprintf("%02X", b)
	}
	total := int(c.NotAfter.Sub(c.NotBefore) / (24 * time.Hour))

	facts := certificateFacts(c, c.NotAfter.Add(-(10*24*time.Hour + time.Hour)))
	if facts["fingerprint"] != strings.Join(pairs, " ") {
		t.Errorf("fingerprint = %q, want the browser's notation", facts["fingerprint"])
	}
	if facts["days_left"] != "10" || facts["days_total"] != strconv.Itoa(total) || facts["expired"] != "" {
		t.Errorf("life = %s of %s (expired %q), want 10 of %d", facts["days_left"], facts["days_total"], facts["expired"], total)
	}
	if facts["self_signed"] != "1" || facts["subject"] != "router.lan" {
		t.Errorf("identity lost: %v", facts)
	}
	gone := certificateFacts(c, c.NotAfter.Add(time.Hour))
	if gone["days_left"] != "0" || gone["expired"] != "1" {
		t.Errorf("an expired certificate has no days left and says so: %v", gone)
	}
}

func TestCertificateCommandsValidatePairAndExposeOnlyPublicMaterial(t *testing.T) {
	cert, key, err := generateWebCertificate("router.lan")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := validateWebCertificate(cert, key); err != nil {
		t.Fatal(err)
	}
	_, other, _ := generateWebCertificate("other.lan")
	if _, _, err := validateWebCertificate(cert, other); err == nil {
		t.Fatal("accepted mismatched key")
	}
	for _, host := range []string{"-bad", "has space", "router;reboot", strings.Repeat("x", 64) + ".lan"} {
		if _, _, err := generateWebCertificate(host); err == nil {
			t.Fatalf("accepted %q", host)
		}
	}
	block, _ := pem.Decode([]byte(cert))
	b := &credentialsFake{data: openwrt.AccessCredentials{CertificateBytes: block.Bytes, CertificateFile: "/etc/uhttpd.crt"}}
	s := &Server{backend: b}
	raw, err := s.readAccessCredentials(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Certificate map[string]string }
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data.Certificate["subject"] != "router.lan" || data.Certificate["self_signed"] != "1" || data.Certificate["fingerprint"] == "" {
		t.Fatalf("missing DER certificate metadata: %s", raw)
	}
	if strings.Contains(string(raw), "PRIVATE KEY") || strings.Contains(string(raw), key) {
		t.Fatal("exposed private key")
	}
	if certificatePEM(b.data) != cert {
		t.Fatal("DER download did not become PEM")
	}
	err = s.credentialCommand(context.Background(), credentialManifest(), "sid", plugin.ApplyAction{Name: "certificate-install", Args: map[string]string{"certificate": cert, "key": other}})
	if err == nil || b.cert != "" || b.key != "" {
		t.Fatal("invalid pair reached helper")
	}
}
