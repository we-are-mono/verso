// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/we-are-mono/verso/internal/datatype"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"golang.org/x/crypto/ssh"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"
)

// commandValidationError is safe, authored feedback for the form. Backend
// failures remain internal; they are never reflected as raw router output.
type commandValidationError string

func (e commandValidationError) Error() string { return string(e) }

type credentialBackend interface {
	AccessCredentials(context.Context, string) (openwrt.AccessCredentials, error)
	SetAuthorizedKeys(context.Context, string, string, string) error
	SetWebCertificate(context.Context, string, string, string) error
}

func (s *Server) readAccessCredentials(ctx context.Context, sid string) (json.RawMessage, error) {
	backend, ok := s.backend.(credentialBackend)
	if !ok {
		return nil, fmt.Errorf("credential service unavailable")
	}
	data, err := backend.AccessCredentials(ctx, sid)
	if err != nil {
		return nil, err
	}
	data.Certificate = certificatePEM(data)
	keys := []map[string]string{}
	for _, line := range strings.Split(data.AuthorizedKeys, "\n") {
		key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			continue
		}
		keys = append(keys, map[string]string{"comment": comment, "fingerprint": ssh.FingerprintSHA256(key)})
	}
	cert := map[string]string{"file": data.CertificateFile}
	if block, _ := pem.Decode([]byte(data.Certificate)); block != nil {
		if c, err := x509.ParseCertificate(block.Bytes); err == nil {
			sum := sha256.Sum256(c.Raw)
			cert["subject"] = c.Subject.CommonName
			cert["issuer"] = c.Issuer.CommonName
			cert["from"] = c.NotBefore.Format("2006-01-02")
			cert["until"] = c.NotAfter.Format("2006-01-02")
			cert["fingerprint"] = "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
			cert["serial"] = hex.EncodeToString(c.SerialNumber.Bytes())
			if c.Issuer.String() == c.Subject.String() && c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil {
				cert["self_signed"] = "1"
			}
		}
	}
	return json.Marshal(map[string]any{"keys": keys, "certificate": cert})
}
func (s *Server) credentialCommand(ctx context.Context, m plugin.Manifest, sid string, cmd plugin.ApplyAction) error {
	function := "setAuthorizedKeys"
	if strings.HasPrefix(cmd.Name, "certificate-") {
		function = "setWebCertificate"
	}
	allowed := false
	for _, a := range m.ACL.Write {
		if a.Scope == "ubus" && a.Object == "verso" && a.Function == function {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("undeclared credential write")
	}
	backend, ok := s.backend.(credentialBackend)
	if !ok {
		return fmt.Errorf("credential service unavailable")
	}
	switch cmd.Name {
	case "ssh-key-add", "ssh-key-remove":
		current, err := backend.AccessCredentials(ctx, sid)
		if err != nil {
			return err
		}
		next := current.AuthorizedKeys
		if cmd.Name == "ssh-key-add" {
			line := strings.TrimSpace(cmd.Args["key"])
			key, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(line))
			if err != nil || len(rest) > 0 || strings.ContainsAny(line, "\r\n") {
				return commandValidationError("Enter one valid SSH public key.")
			}
			for _, existing := range strings.Split(next, "\n") {
				old, _, _, _, e := ssh.ParseAuthorizedKey([]byte(existing))
				if e == nil && ssh.FingerprintSHA256(old) == ssh.FingerprintSHA256(key) {
					return nil
				}
			}
			next = strings.TrimRight(next, "\n") + "\n" + line + "\n"
			next = strings.TrimLeft(next, "\n")
		} else {
			fingerprint := cmd.Args["fingerprint"]
			found := false
			lines := []string{}
			for _, line := range strings.Split(next, "\n") {
				key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
				if err == nil && ssh.FingerprintSHA256(key) == fingerprint {
					found = true
					continue
				}
				lines = append(lines, line)
			}
			if !found {
				return commandValidationError("This key no longer exists. Reload the page.")
			}
			next = strings.Join(lines, "\n")
		}
		return backend.SetAuthorizedKeys(ctx, sid, current.AuthorizedKeys, next)
	case "certificate-generate":
		cert, key, err := generateWebCertificate(cmd.Args["hostname"])
		if err != nil {
			return err
		}
		return backend.SetWebCertificate(ctx, sid, cert, key)
	case "certificate-install":
		cert, key, err := validateWebCertificate(cmd.Args["certificate"], cmd.Args["key"])
		if err != nil {
			return err
		}
		return backend.SetWebCertificate(ctx, sid, cert, key)
	}
	return fmt.Errorf("unknown credential command")
}
func generateWebCertificate(host string) (string, string, error) {
	if host == "" || (net.ParseIP(host) == nil && datatype.Validate("hostname", host) != nil) {
		return "", "", commandValidationError("Enter a valid hostname or IP address.")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: host}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(2, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	if ip := net.ParseIP(host); ip != nil {
		cert.IPAddresses = []net.IP{ip}
	} else {
		cert.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	pk, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk})), nil
}
func validateWebCertificate(cert, key string) (string, string, error) {
	if len(cert) > 64*1024 || len(key) > 64*1024 {
		return "", "", commandValidationError("The certificate or key is too large.")
	}
	pair, err := tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil {
		return "", "", commandValidationError("The certificate and private key do not match.")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return "", "", commandValidationError("The certificate is not currently valid.")
	}
	pk, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(cert) + "\n", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk})), nil
}
func (s *Server) handleCertificateDownload(w http.ResponseWriter, r *http.Request) {
	backend, ok := s.backend.(credentialBackend)
	if !ok {
		http.Error(w, "Certificate unavailable", http.StatusServiceUnavailable)
		return
	}
	data, err := backend.AccessCredentials(r.Context(), s.sessionSID(r))
	data.Certificate = certificatePEM(data)
	if err != nil || data.Certificate == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="router-certificate.pem"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(data.Certificate))
}

func certificatePEM(data openwrt.AccessCredentials) string {
	if len(data.CertificateBytes) == 0 {
		return data.Certificate
	}
	if block, _ := pem.Decode(data.CertificateBytes); block != nil {
		return string(data.CertificateBytes)
	}
	if _, err := x509.ParseCertificate(data.CertificateBytes); err != nil {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: data.CertificateBytes}))
}
