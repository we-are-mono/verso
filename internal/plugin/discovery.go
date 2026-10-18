// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
)

// Discover parses every manifest matched by glob within fsys. It is resilient by
// design (ADR-006's isolation ethos, applied at load time): a malformed,
// invalid, or duplicate manifest is skipped and reported in problems, never
// fatal — one broken plugin cannot break the shell's navigation. Manifests are
// returned sorted by discovery path so the nav is stable across boots. No
// matches yields empty results and no problems.
//
// fsys is injected (ADR-003), so discovery is unit-tested against an in-memory
// filesystem with no real plugin directory.
func Discover(fsys fs.FS, glob string) (manifests []Manifest, problems []error) {
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return nil, []error{fmt.Errorf("plugin: bad glob %q: %w", glob, err)}
	}
	sort.Strings(matches)

	seen := make(map[string]bool)
	for _, path := range matches {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			problems = append(problems, fmt.Errorf("plugin: read %s: %w", path, err))
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			problems = append(problems, fmt.Errorf("plugin: parse %s: %w", path, err))
			continue
		}
		if err := readAlike(data); err != nil {
			problems = append(problems, fmt.Errorf("plugin: %s: %w", path, err))
			continue
		}
		problems = append(problems, wrapAll(path, m.readContributes(data))...)
		if err := m.validate(); err != nil {
			problems = append(problems, fmt.Errorf("plugin: %s: %w", path, err))
			continue
		}
		if seen[m.ID] {
			problems = append(problems, fmt.Errorf("plugin: %s: duplicate id %q, skipped", path, m.ID))
			continue
		}
		seen[m.ID] = true
		// The registry rides beside the manifest. A broken one costs the plugin
		// its root commands, never its pages.
		registryPath := pathpkg.Join(pathpkg.Dir(path), "commands.json")
		if data, err := fs.ReadFile(fsys, registryPath); err == nil {
			if r, err := parseRegistry(data); err != nil {
				problems = append(problems, fmt.Errorf("plugin: %s: %w", registryPath, err))
			} else {
				m.Commands = r
			}
		}
		manifests = append(manifests, m)
	}
	return manifests, problems
}

// readAlike is an error when a manifest, valid JSON, is one verso-rpcd, which
// reads the manifest for its secrets (ADR-021), would read otherwise than
// encoding/json does, or not at all: one spelling a key twice (keysOnce),
// nesting a value deeper than its reader allows (maxDepth), or escaping half
// a surrogate pair (surrogatesPaired). Neither reads such a manifest, so both
// take the same secrets from every manifest they read.
func readAlike(data []byte) error {
	if err := surrogatesPaired(data); err != nil {
		return err
	}
	return keysOnce(data)
}

// maxDepth is the deepest a manifest's objects and arrays nest, the manifest
// itself one level: serde_json, verso-rpcd's reader, refuses one deeper,
// where encoding/json reads it.
const maxDepth = 127

// keysOnce is an error when a manifest, valid JSON, spells a key twice in one
// object, in the same case or another (fieldName), or nests deeper than
// maxDepth. encoding/json takes the last of two spellings of one field, and
// decodes a repeated object into the first; verso-rpcd would keep both
// spellings, or the last object whole.
func keysOnce(data []byte) error {
	return valueKeysOnce(json.NewDecoder(bytes.NewReader(data)), 1)
}

// valueKeysOnce reads the next value from dec, at depth, as keysOnce. Token
// reads a number as a float64, so one past its range is an error here, as
// serde_json refuses it.
func valueKeysOnce(dec *json.Decoder, depth int) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); ok && depth > maxDepth {
		return fmt.Errorf("%c nested deeper than %d", delim, maxDepth)
	}
	switch token {
	case json.Delim('{'):
		spelled := map[string]string{}
		for dec.More() {
			token, err := dec.Token()
			if err != nil {
				return err
			}
			key, _ := token.(string)
			if before, twice := spelled[fieldName(key)]; twice {
				return fmt.Errorf("key %q spelled twice, also as %q", key, before)
			}
			spelled[fieldName(key)] = key
			if err := valueKeysOnce(dec, depth+1); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for dec.More() {
			if err := valueKeysOnce(dec, depth+1); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	_, err = dec.Token()
	return err
}

// surrogatesPaired is an error when a string in a manifest, valid JSON,
// escapes half a surrogate pair: a \u escape of a leading surrogate not
// followed at once by one of a trailing surrogate, or of a trailing one
// alone. encoding/json reads it as U+FFFD; serde_json refuses it.
func surrogatesPaired(data []byte) error {
	inString := false
	for i := 0; i < len(data); i++ {
		switch c := data[i]; {
		case !inString:
			inString = c == '"'
		case c == '"':
			inString = false
		case c == '\\':
			i++
			if data[i] != 'u' {
				continue
			}
			r := escaped(data[i+1 : i+5])
			i += 4
			switch {
			case isTrailing(r):
				return fmt.Errorf(`\u%04x escapes a trailing surrogate alone`, r)
			case isLeading(r):
				if len(data) < i+7 || !bytes.HasPrefix(data[i+1:], []byte(`\u`)) || !isTrailing(escaped(data[i+3:i+7])) {
					return fmt.Errorf(`\u%04x escapes a leading surrogate alone`, r)
				}
				i += 6
			}
		}
	}
	return nil
}

// escaped is the code unit four hex digits of a \u escape spell, or -1.
func escaped(hex []byte) rune {
	n, err := strconv.ParseUint(string(hex), 16, 16)
	if err != nil {
		return -1
	}
	return rune(n)
}

func isLeading(r rune) bool  { return r >= 0xd800 && r < 0xdc00 }
func isTrailing(r rune) bool { return r >= 0xdc00 && r < 0xe000 }

// Runes encoding/json folds into an ASCII letter of a field's name.
const (
	longS      = 0x17f  // ſ, an s
	kelvinSign = 0x212a // K, a k
)

// fieldName is the field of a manifest a key names, as encoding/json matches
// a key to a field. Every field's name is ASCII, so a key names one in any
// case, the long s for s and the Kelvin sign for k; any other rune names
// itself. verso-rpcd folds a key alike.
func fieldName(key string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		case r == longS:
			return 's'
		case r == kelvinSign:
			return 'k'
		}
		return r
	}, key)
}

// wrapAll prefixes each of a manifest's problems with the file it is in.
func wrapAll(path string, errs []error) []error {
	out := make([]error, 0, len(errs))
	for _, err := range errs {
		out = append(out, fmt.Errorf("plugin: %s: %w", path, err))
	}
	return out
}
