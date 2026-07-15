package tracebundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxJSONDepth                  = 64
	maxJSONTokens                 = 262144
	maxJSONObjectMembers          = 4096
	maxUnknownArrayElements       = 65536
	maxArtifactElements           = 256
	maxAlignmentOrGateElements    = 256
	maxDecisionOrCoverageElements = 4096
	maxTopLevelCaveatElements     = 1024
	maxCaveatEvidenceElements     = 4096
	maxJSONStringBytes            = 64 << 10
)

type envelopeValidator struct {
	ctx                    context.Context
	decoder                *json.Decoder
	tokens                 int
	caveatEvidenceElements int
}

func validateJSONEnvelope(ctx context.Context, data []byte) error {
	if !utf8.Valid(data) {
		return invalidManifestf("UTF-8 is required")
	}
	if err := validateJSONSurrogateEscapes(data); err != nil {
		return err
	}
	v := envelopeValidator{ctx: ctx, decoder: json.NewDecoder(bytes.NewReader(data))}
	v.decoder.UseNumber()

	root, err := v.nextToken()
	if err != nil {
		if err == io.EOF {
			return invalidManifestf("root object is missing")
		}
		return err
	}
	delim, ok := root.(json.Delim)
	if !ok || delim != '{' {
		return invalidManifestf("root must be an object")
	}
	if err := v.parseObject(1); err != nil {
		return err
	}
	trailing, err := v.nextToken()
	if err == nil {
		return invalidManifestf("trailing JSON value starts with %s", compactToken(trailing))
	}
	if err != io.EOF {
		return err
	}
	return nil
}

// validateJSONSurrogateEscapes rejects the lossy corner of encoding/json's
// string decoder: an unpaired UTF-16 surrogate escape is silently converted
// to U+FFFD. Manifest paths and provenance keys must never be rewritten while
// crossing the hard intake boundary.
func validateJSONSurrogateEscapes(data []byte) error {
	inString := false
	for i := 0; i < len(data); {
		switch data[i] {
		case '"':
			inString = !inString
			i++
		case '\\':
			if !inString {
				i++
				continue
			}
			if i+1 >= len(data) {
				return invalidManifestf("unterminated JSON string escape")
			}
			if data[i+1] != 'u' {
				i += 2
				continue
			}
			unit, ok := parseJSONHexUnit(data, i+2)
			if !ok {
				return invalidManifestf("invalid Unicode escape in JSON string")
			}
			switch {
			case unit >= 0xD800 && unit <= 0xDBFF:
				if i+12 > len(data) || data[i+6] != '\\' || data[i+7] != 'u' {
					return invalidManifestf("unpaired high surrogate escape in JSON string")
				}
				low, lowOK := parseJSONHexUnit(data, i+8)
				if !lowOK || low < 0xDC00 || low > 0xDFFF {
					return invalidManifestf("high surrogate escape is not followed by a low surrogate")
				}
				i += 12
			case unit >= 0xDC00 && unit <= 0xDFFF:
				return invalidManifestf("unpaired low surrogate escape in JSON string")
			default:
				i += 6
			}
		default:
			i++
		}
	}
	return nil
}

func parseJSONHexUnit(data []byte, start int) (uint16, bool) {
	if start < 0 || start+4 > len(data) {
		return 0, false
	}
	var value uint16
	for _, b := range data[start : start+4] {
		value <<= 4
		switch {
		case b >= '0' && b <= '9':
			value |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			value |= uint16(b-'a') + 10
		case b >= 'A' && b <= 'F':
			value |= uint16(b-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func (v *envelopeValidator) parseValue(token json.Token, depth int, field string, parentObjectDepth int) error {
	if delim, ok := token.(json.Delim); ok {
		if depth > maxJSONDepth {
			return invalidManifestf("maximum nesting depth exceeded: limit=%d", maxJSONDepth)
		}
		switch delim {
		case '{':
			return v.parseObject(depth)
		case '[':
			return v.parseArray(depth, field, parentObjectDepth)
		default:
			return invalidManifestf("unexpected delimiter %q", delim)
		}
	}
	if value, ok := token.(string); ok {
		return validateString(value)
	}
	return nil
}

// parseObject consumes members after the opening '{' and the closing '}'.
func (v *envelopeValidator) parseObject(depth int) error {
	seen := make(map[string]string)
	members := 0
	for v.decoder.More() {
		keyToken, err := v.nextToken()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return invalidManifestf("object key is not a string")
		}
		if err := validateString(key); err != nil {
			return err
		}
		members++
		if members > maxJSONObjectMembers {
			return invalidManifestf("object member limit exceeded: limit=%d", maxJSONObjectMembers)
		}
		canonical := canonicalJSONKey(key)
		if previous, exists := seen[canonical]; exists {
			return invalidManifestf("duplicate object key under Unicode case folding: %s conflicts with %s", compactString(previous), compactString(key))
		}
		seen[canonical] = key

		value, err := v.nextToken()
		if err != nil {
			return err
		}
		if err := v.parseValue(value, depth+1, canonical, depth); err != nil {
			return err
		}
	}
	end, err := v.nextToken()
	if err != nil {
		return err
	}
	if delim, ok := end.(json.Delim); !ok || delim != '}' {
		return invalidManifestf("object is not closed")
	}
	return nil
}

// parseArray consumes elements after the opening '[' and the closing ']'.
func (v *envelopeValidator) parseArray(depth int, field string, parentObjectDepth int) error {
	limit := arrayLimit(field, parentObjectDepth)
	countCaveatEvidence := field == canonicalJSONKey("caveats") || field == canonicalJSONKey("evidence")
	elements := 0
	for v.decoder.More() {
		elements++
		if elements > limit {
			return invalidManifestf("array element limit exceeded: field=%s limit=%d", compactString(field), limit)
		}
		if countCaveatEvidence {
			v.caveatEvidenceElements++
			if v.caveatEvidenceElements > maxCaveatEvidenceElements {
				return invalidManifestf("manifest caveat/evidence element limit exceeded: limit=%d", maxCaveatEvidenceElements)
			}
		}
		value, err := v.nextToken()
		if err != nil {
			return err
		}
		if err := v.parseValue(value, depth+1, "", 0); err != nil {
			return err
		}
	}
	end, err := v.nextToken()
	if err != nil {
		return err
	}
	if delim, ok := end.(json.Delim); !ok || delim != ']' {
		return invalidManifestf("array is not closed")
	}
	return nil
}

func (v *envelopeValidator) nextToken() (json.Token, error) {
	if err := contextError(v.ctx); err != nil {
		return nil, err
	}
	token, err := v.decoder.Token()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, invalidManifestf("syntax error: %v", err)
	}
	v.tokens++
	if v.tokens > maxJSONTokens {
		return nil, invalidManifestf("JSON token limit exceeded: limit=%d", maxJSONTokens)
	}
	if value, ok := token.(string); ok {
		if err := validateString(value); err != nil {
			return nil, err
		}
	}
	return token, nil
}

func arrayLimit(field string, parentObjectDepth int) int {
	// Schema-specific lanes are precise only at the manifest root. A future
	// extension may legitimately use the same leaf name inside its own object;
	// that unknown subtree receives the generic budget instead of inheriting a
	// noisy name-only hard gate.
	if parentObjectDepth != 1 {
		return maxUnknownArrayElements
	}
	switch field {
	case "ARTIFACTS":
		return maxArtifactElements
	case "PERF_CLOCK_ALIGNMENTS", "TRACE_TOOL_GATES":
		return maxAlignmentOrGateElements
	case "PROVIDER_DECISIONS", "TRACE_DECISIONS", "TRACE_PROVIDER_DECISIONS", "TRACE_DB_COVERAGE", "TRACE_COVERAGE":
		return maxDecisionOrCoverageElements
	case "CAVEATS":
		return maxTopLevelCaveatElements
	}
	return maxUnknownArrayElements
}

func validateString(value string) error {
	if len(value) > maxJSONStringBytes {
		return invalidManifestf("decoded JSON string limit exceeded: size=%d limit=%d", len(value), maxJSONStringBytes)
	}
	return nil
}

// canonicalJSONKey mirrors encoding/json's Unicode SimpleFold name matching:
// ASCII letters fold to upper case and non-ASCII runes fold to the smallest
// rune in their SimpleFold cycle.
func canonicalJSONKey(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		} else if r >= utf8.RuneSelf {
			r = smallestSimpleFoldRune(r)
		}
		out.WriteRune(r)
	}
	return out.String()
}

func smallestSimpleFoldRune(r rune) rune {
	for {
		next := unicode.SimpleFold(r)
		if next <= r {
			return next
		}
		r = next
	}
}

func invalidManifestf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidManifest, fmt.Sprintf(format, args...))
}

func compactString(value string) string {
	const max = 80
	if len(value) <= max {
		return fmt.Sprintf("%q", value)
	}
	return fmt.Sprintf("%q...", value[:max])
}

func compactToken(token json.Token) string {
	if value, ok := token.(string); ok {
		return compactString(value)
	}
	return fmt.Sprintf("%v", token)
}
