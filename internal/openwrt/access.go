// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import "context"

type AccessCredentials struct {
	CertificateBytes []byte `json:"certificate_bytes"`
	AuthorizedKeys   string `json:"authorized_keys"`
	Certificate      string `json:"certificate"`
	CertificateFile  string `json:"certificate_file"`
}

func (*NativeBackend) AccessCredentials(ctx context.Context, sid string) (AccessCredentials, error) {
	var result AccessCredentials
	err := callHelper(ctx, "", "accessCredentials", sid, nil, &result)
	return result, err
}
func (*NativeBackend) SetAuthorizedKeys(ctx context.Context, sid, expected, keys string) error {
	return callHelper(ctx, "", "setAuthorizedKeys", sid, map[string]string{"expected": expected, "keys": keys}, nil)
}
func (*NativeBackend) SetWebCertificate(ctx context.Context, sid, cert, key string) error {
	return callHelper(ctx, "", "setWebCertificate", sid, map[string]string{"certificate": cert, "key": key}, nil)
}
func (*NativeBackend) MakeWebCertificate(ctx context.Context, sid string) error {
	return callHelper(ctx, "", "makeWebCertificate", sid, nil, nil)
}
func (*NativeBackend) SetWebOwner(ctx context.Context, sid, owner string) error {
	return callHelper(ctx, "", "setWebOwner", sid, map[string]string{"owner": owner}, nil)
}
