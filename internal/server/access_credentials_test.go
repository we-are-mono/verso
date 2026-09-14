// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

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
