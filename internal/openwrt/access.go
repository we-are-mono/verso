// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"context"
	"strings"
)

type AccessCredentials struct {
	CertificateBytes []byte `json:"certificate_bytes"`
	AuthorizedKeys   string `json:"authorized_keys"`
	Certificate      string `json:"certificate"`
	CertificateFile  string `json:"certificate_file"`
}

func (b *NativeBackend) AccessCredentials(ctx context.Context, sid string) (AccessCredentials, error) {
	var result AccessCredentials
	err := callHelper(ctx, b.helperSocket, "accessCredentials", sid, nil, &result)
	return result, err
}
func (b *NativeBackend) SetAuthorizedKeys(ctx context.Context, sid, expected, keys string) error {
	return callHelper(ctx, b.helperSocket, "setAuthorizedKeys", sid, map[string]string{"expected": expected, "keys": keys}, nil)
}
func (b *NativeBackend) SetWebCertificate(ctx context.Context, sid, cert, key string) error {
	return callHelper(ctx, b.helperSocket, "setWebCertificate", sid, map[string]string{"certificate": cert, "key": key}, nil)
}
func (b *NativeBackend) MakeWebCertificate(ctx context.Context, sid string) error {
	return callHelper(ctx, b.helperSocket, "makeWebCertificate", sid, nil, nil)
}
func (b *NativeBackend) SetWebListeners(ctx context.Context, sid string, https, http []string, redirect string) error {
	return callHelper(ctx, b.helperSocket, "setWebListeners", sid, map[string]string{
		"https": strings.Join(https, " "), "http": strings.Join(http, " "), "redirect": redirect,
	}, nil)
}

// SetTheme asks the helper to put theme id in force for the session sid
// (ADR-024).
func (b *NativeBackend) SetTheme(ctx context.Context, sid, id string) error {
	return callHelper(ctx, b.helperSocket, "setTheme", sid, map[string]string{"theme": id}, nil)
}
func (b *NativeBackend) SetWebOwner(ctx context.Context, sid, owner string) error {
	return callHelper(ctx, b.helperSocket, "setWebOwner", sid, map[string]string{"owner": owner}, nil)
}
