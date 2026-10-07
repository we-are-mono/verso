// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/datatype"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/tlscert"
	"golang.org/x/crypto/ssh"
)

// commandValidationError is safe, authored feedback for the form. Backend
// failures remain internal; they are never reflected as raw router output.
type commandValidationError string

func (e commandValidationError) Error() string { return string(e) }

func (s *Server) readAccessCredentials(ctx context.Context, sid string) (json.RawMessage, error) {
	data, err := s.backend.AccessCredentials(ctx, sid)
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
	cert := map[string]string{}
	if block, _ := pem.Decode([]byte(data.Certificate)); block != nil {
		if c, err := x509.ParseCertificate(block.Bytes); err == nil {
			cert = certificateFacts(c, time.Now())
		}
	}
	cert["file"] = data.CertificateFile
	return json.Marshal(map[string]any{"keys": keys, "certificate": cert})
}

// certificateFacts is what the Access page says about the web certificate. The
// fingerprint is written as a browser's certificate viewer writes it (SHA-256
// of the DER, upper-case hex pairs), so the one a "not secure" page shows can
// be matched against the router's by eye. Its life is counted in whole days
// against now: days_left rounds down, so the page never promises a day the
// certificate does not have.
func certificateFacts(c *x509.Certificate, now time.Time) map[string]string {
	sum := sha256.Sum256(c.Raw)
	pairs := make([]string, len(sum))
	for i, b := range sum {
		pairs[i] = strings.ToUpper(hex.EncodeToString([]byte{b}))
	}
	day := 24 * time.Hour
	facts := map[string]string{
		"subject":     c.Subject.CommonName,
		"issuer":      c.Issuer.CommonName,
		"from":        c.NotBefore.Format("2006-01-02"),
		"until":       c.NotAfter.Format("2006-01-02"),
		"fingerprint": strings.Join(pairs, " "),
		"serial":      hex.EncodeToString(c.SerialNumber.Bytes()),
		"days_total":  strconv.Itoa(int(c.NotAfter.Sub(c.NotBefore) / day)),
		"days_left":   "0",
	}
	if left := c.NotAfter.Sub(now); left > 0 {
		facts["days_left"] = strconv.Itoa(int(left / day))
	} else {
		facts["expired"] = "1"
	}
	if tlscert.SelfSigned(c) {
		facts["self_signed"] = "1"
	}
	return facts
}
func (s *Server) credentialCommand(ctx context.Context, m plugin.Manifest, sid string, cmd plugin.ApplyAction) error {
	function := "setAuthorizedKeys"
	if strings.HasPrefix(cmd.Name, "certificate-") {
		function = "setWebCertificate"
	}
	if !slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "verso", Function: function}) {
		return fmt.Errorf("undeclared credential write")
	}
	switch cmd.Name {
	case "ssh-key-add", "ssh-key-remove":
		current, err := s.backend.AccessCredentials(ctx, sid)
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
		return s.backend.SetAuthorizedKeys(ctx, sid, current.AuthorizedKeys, next)
	case "certificate-generate":
		cert, key, err := generateWebCertificate(cmd.Args["hostname"])
		if err != nil {
			return err
		}
		return s.backend.SetWebCertificate(ctx, sid, cert, key)
	case "certificate-install":
		cert, key, err := validateWebCertificate(cmd.Args["certificate"], cmd.Args["key"])
		if err != nil {
			return err
		}
		return s.backend.SetWebCertificate(ctx, sid, cert, key)
	}
	return fmt.Errorf("unknown credential command")
}
func generateWebCertificate(host string) (string, string, error) {
	if host == "" || (net.ParseIP(host) == nil && datatype.Validate("hostname", host) != nil) {
		return "", "", commandValidationError("Enter a valid hostname or IP address.")
	}
	cert, key, err := tlscert.Generate([]string{host}, time.Now())
	return string(cert), string(key), err
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
	data, err := s.backend.AccessCredentials(r.Context(), s.sessionSID(r))
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
