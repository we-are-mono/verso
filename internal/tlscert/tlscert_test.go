// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func parse(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("no PEM block")
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAGeneratedCertificateNamesEveryWayTheRouterIsReached(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	certPEM, keyPEM, err := Generate([]string{"verso-lab", "verso-lab.lan", "192.168.1.1", "fd42:7ea:aa00::1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("the key does not match the certificate: %v", err)
	}
	c := parse(t, certPEM)
	if c.Subject.CommonName != "verso-lab" {
		t.Errorf("common name = %q, want the first name", c.Subject.CommonName)
	}
	if !slices.Equal(c.DNSNames, []string{"verso-lab", "verso-lab.lan"}) {
		t.Errorf("DNS names = %v", c.DNSNames)
	}
	if len(c.IPAddresses) != 2 || !c.IPAddresses[0].Equal(net.ParseIP("192.168.1.1")) || !c.IPAddresses[1].Equal(net.ParseIP("fd42:7ea:aa00::1")) {
		t.Errorf("IP addresses = %v", c.IPAddresses)
	}
	if _, ok := c.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Errorf("key is %T, want ECDSA", c.PublicKey)
	}
	if !c.NotAfter.Equal(now.AddDate(2, 0, 0).Truncate(time.Second)) || !c.NotBefore.Before(now) {
		t.Errorf("valid %v – %v, want two years from %v", c.NotBefore, c.NotAfter, now)
	}
	if !slices.Equal(c.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}) {
		t.Errorf("extended key usage = %v", c.ExtKeyUsage)
	}
	if !SelfSigned(c) {
		t.Error("a generated certificate is self-signed")
	}
}

func TestGenerateRefusesANameThatIsNeitherHostnameNorAddress(t *testing.T) {
	for _, names := range [][]string{nil, {"bad name"}, {"router", "-x"}} {
		if _, _, err := Generate(names, time.Now()); err == nil {
			t.Errorf("Generate(%q) succeeded", names)
		}
	}
}

func TestLANNamesKeepWhatALANVisitUsesAndDropTheRest(t *testing.T) {
	got := LANNames([]string{"verso-lab", "", "verso-lab", "verso-lab.lan", "192.168.1.1", "2a02:1:2::1", "fd42:7ea:aa00::1", "fe80::1", "not a name"})
	want := []string{"verso-lab", "verso-lab.lan", "192.168.1.1", "fd42:7ea:aa00::1"}
	if !slices.Equal(got, want) {
		t.Errorf("LANNames = %v, want %v", got, want)
	}
}

func TestWrittenFilesAreThePairWithAPrivateKeyAndNoLeftovers(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM, err := Generate([]string{"router"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	for file, mode := range map[string]os.FileMode{CertFile: 0o644, KeyFile: 0o640} {
		info, err := os.Stat(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s mode = %v, want %v", file, info.Mode().Perm(), mode)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("directory holds %d entries, want the pair alone", len(entries))
	}
	if _, err := tls.LoadX509KeyPair(filepath.Join(dir, CertFile), filepath.Join(dir, KeyFile)); err != nil {
		t.Fatal(err)
	}
}

func TestTheStoreServesThePairOnDiskAndKeepsItWhenAReloadFails(t *testing.T) {
	dir := t.TempDir()
	first, firstKey, _ := Generate([]string{"first"}, time.Now())
	if err := Write(dir, first, firstKey); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	served := func() string {
		c, err := store.GetCertificate(&tls.ClientHelloInfo{})
		if err != nil {
			t.Fatal(err)
		}
		return c.Leaf.Subject.CommonName
	}
	if served() != "first" {
		t.Fatalf("serves %q", served())
	}
	second, secondKey, _ := Generate([]string{"second"}, time.Now())
	if err := Write(dir, second, secondKey); err != nil {
		t.Fatal(err)
	}
	if err := store.Load(); err != nil || served() != "second" {
		t.Fatalf("reload: %v, serves %q", err, served())
	}
	if err := os.WriteFile(filepath.Join(dir, KeyFile), firstKey, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := store.Load(); err == nil {
		t.Fatal("a mismatched pair loaded")
	}
	if served() != "second" {
		t.Errorf("a failed reload replaced the served certificate with %q", served())
	}
}

func TestAStoreWithNothingOnDiskFailsToLoad(t *testing.T) {
	if err := NewStore(t.TempDir()).Load(); err == nil {
		t.Fatal("an absent pair loaded")
	}
}

// signedBy makes a leaf for name issued by a throwaway authority, as a
// certificate from Let's Encrypt is: its issuer is not its subject.
func signedBy(t *testing.T, names ...string) ([]byte, []byte) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	pk, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk})
}

func TestTheStoreNamesTheDomainATrustedCertificateIsIssuedFor(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if store.Name() != "" {
		t.Errorf("an empty store names %q", store.Name())
	}
	selfCert, selfKey, _ := Generate([]string{"router.example.com"}, time.Now())
	_ = Write(dir, selfCert, selfKey)
	_ = store.Load()
	if store.Name() != "" {
		t.Errorf("a self-signed certificate names %q", store.Name())
	}
	cert, key := signedBy(t, "*.example.com", "router.example.com")
	_ = Write(dir, cert, key)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if store.Name() != "router.example.com" {
		t.Errorf("Name() = %q, want the first name that is not a wildcard", store.Name())
	}
	if SelfSigned(parse(t, cert)) {
		t.Error("a certificate an authority issued reads as self-signed")
	}
}
