package index

import (
	"fmt"
	"strconv"
	"strings"
)

// toml_parser.go — hand-written TOML subset parser covering the
// shape of cjpm.toml (Cangjie package manager manifest).
//
// Why a bespoke subset rather than pelletier/go-toml/v2: we only
// need the top-level scalar keys of `[package]` and the flat
// key-value form of `[dependencies]`. The full TOML spec (datetime,
// array-of-tables, inline tables, dotted keys, multi-line strings)
// is unused by cjpm.toml and brings ~4000 LOC of parse surface we
// don't maintain.
//
// Supported:
//
//   - Whole-line `# comment` (end-of-line comments too)
//   - `[section]` headers (flat; no dotted tables today)
//   - `key = "string"` / `key = 'raw string'`
//   - `key = 123` / `key = -1.5` / `key = true` / `key = false`
//   - `key = ["a", "b"]` flat array of strings
//   - Unknown syntactic forms (multi-line strings, dotted tables,
//     array-of-tables) are silently skipped so a cjpm.toml that
//     uses advanced features still yields the subset we care about.
//
// Public API:
//
//	ParseTOMLSubset(src []byte) (map[string]map[string]any, error)
//
// Returns a `section → key → value` map. Keys before any `[section]`
// header live under the empty-string section.

// ParseTOMLSubset parses src as a subset of TOML and returns a map
// of section → key → value. See package comment for the supported
// surface.
func ParseTOMLSubset(src []byte) (map[string]map[string]any, error) {
	out := map[string]map[string]any{
		"": {},
	}
	section := ""
	lines := strings.Split(string(src), "\n")

	for lineNo, raw := range lines {
		line := stripTOMLComment(raw)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Section header. We accept `[dependencies]` and
			// `[dependencies.foo]` alike; the latter opens a
			// namespaced subsection which we flatten by using the
			// full bracketed name as the section key.
			section = strings.TrimSpace(line[1 : len(line)-1])
			if _, ok := out[section]; !ok {
				out[section] = map[string]any{}
			}
			continue
		}
		// key = value (the `=` is the first one; values may contain
		// `=` inside strings).
		eq := strings.Index(line, "=")
		if eq < 0 {
			// Not a key/value — likely the start of a multi-line
			// string or a malformed line. Skip silently.
			continue
		}
		key := strings.TrimSpace(line[:eq])
		valStr := strings.TrimSpace(line[eq+1:])
		val, err := parseTOMLValue(valStr)
		if err != nil {
			// Non-fatal — silently skip unknown value shapes. A
			// real parse failure on a critical key (e.g. `name`)
			// surfaces as a missing lookup result downstream.
			_ = err
			_ = lineNo
			continue
		}
		if _, ok := out[section]; !ok {
			out[section] = map[string]any{}
		}
		out[section][key] = val
	}
	return out, nil
}

// stripTOMLComment strips a trailing `# comment` from the line,
// taking care not to strip `#` inside a quoted string. We approximate
// by scanning left-to-right, flipping an `inString` flag on unescaped
// quote characters.
func stripTOMLComment(s string) string {
	inDQ := false
	inSQ := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			// Skip next char (escape) inside a string.
			if inDQ || inSQ {
				i++
			}
		case '"':
			if !inSQ {
				inDQ = !inDQ
			}
		case '\'':
			if !inDQ {
				inSQ = !inSQ
			}
		case '#':
			if !inDQ && !inSQ {
				return s[:i]
			}
		}
	}
	return s
}

// parseTOMLValue recognises the subset of TOML value shapes listed
// in the package doc. Returns string, float64, int64, bool, or
// []any. Multi-line strings and inline tables yield an error the
// caller quietly discards.
func parseTOMLValue(s string) (any, error) {
	if s == "" {
		return "", fmt.Errorf("toml: empty value")
	}
	// Boolean
	if s == "true" {
		return true, nil
	}
	if s == "false" {
		return false, nil
	}
	// String (double or single quoted)
	if strings.HasPrefix(s, `"""`) || strings.HasPrefix(s, `'''`) {
		return nil, fmt.Errorf("toml: multi-line string unsupported")
	}
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		return decodeTOMLBasicString(s[1 : len(s)-1])
	}
	if strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`) && len(s) >= 2 {
		// Literal string — no escape processing.
		return s[1 : len(s)-1], nil
	}
	// Array
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && len(s) >= 2 {
		inner := s[1 : len(s)-1]
		// Flat string array only: split on commas outside quotes.
		parts := splitTOMLArray(inner)
		out := make([]any, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			v, err := parseTOMLValue(p)
			if err != nil {
				continue
			}
			out = append(out, v)
		}
		return out, nil
	}
	// Inline table — unsupported, skip.
	if strings.HasPrefix(s, "{") {
		return nil, fmt.Errorf("toml: inline table unsupported")
	}
	// Number.
	if i, err := strconv.ParseInt(s, 0, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	return nil, fmt.Errorf("toml: unrecognised value %q", s)
}

// decodeTOMLBasicString decodes the minimum TOML escape set (\n,
// \t, \r, \\, \", \uXXXX).
func decodeTOMLBasicString(s string) (string, error) {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			return "", fmt.Errorf("toml: trailing backslash")
		}
		switch s[i+1] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '"', '\\':
			b.WriteByte(s[i+1])
		case 'u':
			if i+6 > len(s) {
				return "", fmt.Errorf("toml: truncated \\u")
			}
			hex := s[i+2 : i+6]
			v, err := strconv.ParseUint(hex, 16, 32)
			if err != nil {
				return "", err
			}
			b.WriteRune(rune(v))
			i += 4
		default:
			b.WriteByte(s[i+1])
		}
		i += 2
	}
	return b.String(), nil
}

// splitTOMLArray splits an array's inner text by top-level commas,
// respecting quoted strings and nested brackets.
func splitTOMLArray(s string) []string {
	var out []string
	var buf strings.Builder
	depth := 0
	inDQ := false
	inSQ := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !inDQ && !inSQ {
			switch c {
			case '[':
				depth++
			case ']':
				depth--
			case '"':
				inDQ = true
			case '\'':
				inSQ = true
			case ',':
				if depth == 0 {
					out = append(out, buf.String())
					buf.Reset()
					continue
				}
			}
		} else {
			if c == '\\' && i+1 < len(s) {
				buf.WriteByte(c)
				buf.WriteByte(s[i+1])
				i++
				continue
			}
			if inDQ && c == '"' {
				inDQ = false
			} else if inSQ && c == '\'' {
				inSQ = false
			}
		}
		buf.WriteByte(c)
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}
