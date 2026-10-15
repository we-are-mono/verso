// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

// barColour is the palette's value for the token a page stands on, which the
// page declares as the browser's bar colour (theme-color) so the bar runs on
// into the page. Empty when the palette has no such token, and the page then
// declares none.
func (s *Server) barColour(token string) string {
	v, _ := s.palette.Colour(token)
	return v
}
