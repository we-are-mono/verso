// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package tlscert is the certificate Verso serves HTTPS with (ADR-017): made
// self-signed for the names a LAN reaches the router by, kept as a pair of
// files, and loaded into the server, again on every reload.
package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/we-are-mono/verso/internal/datatype"
)

// The pair's file names inside its directory, /etc/verso on a router.
const (
	CertFile = "tls.crt"
	KeyFile  = "tls.key"
)

// Generate makes a self-signed certificate for names, each a hostname or an
// address: ECDSA P-256, valid two years from now, for serving only. The first
// name is its subject.
func Generate(names []string, now time.Time) (certPEM, keyPEM []byte, err error) {
	if len(names) == 0 {
		return nil, nil, errors.New("a certificate needs at least one name")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	cert := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: names[0]},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, name := range names {
		if ip := net.ParseIP(name); ip != nil {
			cert.IPAddresses = append(cert.IPAddresses, ip)
		} else if datatype.Validate("hostname", name) == nil {
			cert.DNSNames = append(cert.DNSNames, name)
		} else {
			return nil, nil, fmt.Errorf("%q is neither a hostname nor an address", name)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	pk, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}), nil
}

// LANNames keeps, in order and once each, the candidates a visit from the LAN
// can use: hostnames, IPv4 addresses and IPv6 unique local addresses. A global
// IPv6 address comes from the upstream's delegated prefix and changes with it;
// a link-local one is never typed into a browser.
func LANNames(candidates []string) []string {
	var names []string
	for _, c := range candidates {
		if c == "" || slices.Contains(names, c) {
			continue
		}
		ip := net.ParseIP(c)
		switch {
		case ip != nil && ip.To4() != nil:
		case ip != nil && ip.IsPrivate():
		case ip == nil && datatype.Validate("hostname", c) == nil:
		default:
			continue
		}
		names = append(names, c)
	}
	return names
}

// Write replaces the pair in dir: the certificate readable by all, the key by
// its owner and group, which is the verso group the shell runs in. Each file
// is written aside and renamed into place, so a reader never meets half a file.
func Write(dir string, certPEM, keyPEM []byte) error {
	if err := replace(filepath.Join(dir, KeyFile), keyPEM, 0o640); err != nil {
		return err
	}
	return replace(filepath.Join(dir, CertFile), certPEM, 0o644)
}

func replace(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SelfSigned reports whether c vouches for itself: issued by its own subject,
// under its own signature.
func SelfSigned(c *x509.Certificate) bool {
	return c.Issuer.String() == c.Subject.String() &&
		c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil
}

// Store holds the pair the server is presenting. Load reads it from disk; the
// last pair that loaded is served until another one does.
type Store struct {
	dir     string
	current atomic.Pointer[tls.Certificate]
}

func NewStore(dir string) *Store { return &Store{dir: dir} }

// Load reads the pair in the store's directory and serves it from the next
// handshake. A pair that fails to load leaves the served one in place.
func (s *Store) Load() error {
	pair, err := tls.LoadX509KeyPair(filepath.Join(s.dir, CertFile), filepath.Join(s.dir, KeyFile))
	if err != nil {
		return err
	}
	s.current.Store(&pair)
	return nil
}

// GetCertificate is the server's tls.Config hook.
func (s *Store) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if pair := s.current.Load(); pair != nil {
		return pair, nil
	}
	return nil, errors.New("no certificate is loaded")
}

// Name is the domain a certificate someone else issued is for: its first name
// that is not a wildcard. A self-signed certificate names nothing a browser
// trusts, so it has no name.
func (s *Store) Name() string {
	pair := s.current.Load()
	if pair == nil || pair.Leaf == nil || SelfSigned(pair.Leaf) {
		return ""
	}
	for _, name := range pair.Leaf.DNSNames {
		if !strings.HasPrefix(name, "*.") {
			return name
		}
	}
	return ""
}
