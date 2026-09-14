// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"net/http"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// The landing page moves update notices into the header. It reads the same
// recorded check as maintenance and never starts a network check on page load.
func (s *Server) homeUpdateNotice(r *http.Request, tr func(string) string) string {
	if r.URL.Path != "/" {
		return ""
	}
	truth, known := s.updateTruth()
	if !known {
		return ""
	}
	if truth.Firmware.State == openwrt.FirmwareUpdateAvailable {
		if truth.Firmware.To != "" {
			return fmt.Sprintf(tr("%s available"), truth.Firmware.To)
		}
		return tr("Firmware update available")
	}
	count := len(truth.Packages)
	switch count {
	case 0:
		return ""
	case 1:
		return tr("1 package update available")
	case 2:
		return tr("2 package updates available")
	case 3:
		return tr("3 package updates available")
	case 4:
		return tr("4 package updates available")
	default:
		return fmt.Sprintf(tr("%d package updates available"), count)
	}
}
