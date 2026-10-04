// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"strconv"
	"strings"
)

// looksLikeCPU reports whether a zone type or hwmon name reads as the processor.
func looksLikeCPU(t string) bool {
	t = strings.ToLower(t)
	for _, k := range []string{"x86_pkg", "cpu", "cluster", "coretemp", "package", "tctl"} {
		if strings.Contains(t, k) {
			return true
		}
	}
	return false
}

// suffixMatch reports whether an of_node path ends at the profile key — the whole
// path, or a trailing run of its segments.
func suffixMatch(ofNode, key string) bool {
	return ofNode != "" && (ofNode == key || strings.HasSuffix(ofNode, "/"+key))
}

// splitLastSeg splits a "/"-path into its parent and final segment.
func splitLastSeg(path string) (parent, last string) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

// indexAfterAt parses the unit-address of a node segment ("fan@0" → 0), which for
// a controller's channel child is its channel number.
func indexAfterAt(seg string) (int, bool) {
	i := strings.LastIndex(seg, "@")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(seg[i+1:])
	if err != nil {
		return 0, false
	}
	return n, true
}
