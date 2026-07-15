package tracewire

import (
	"fmt"
	"strconv"
	"strings"
)

// Perf text is a Codrax-owned interchange lane shared by all perf adapters and
// tracequery. Keep its grammar deliberately smaller than generic ftrace KV:
// arbitrary vendor events may use positional/colon forms, while perf_sample
// hard identities require one complete, escape-aware authority.
const (
	MaxPerfKVFields            = 128
	MaxPerfKVBodyBytes         = 1 << 20
	MaxPerfKVKeyBytes          = 64
	MaxPerfKVEncodedValueBytes = 64 << 10
)

type PerfKVField struct {
	Key    string
	Value  string
	Raw    string
	Quoted bool
}

type PerfKVError struct {
	Field  string
	Reason string
}

func (e *PerfKVError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field == "" {
		return fmt.Sprintf("perf text kv: %s", e.Reason)
	}
	return fmt.Sprintf("perf text kv: field=%s reason=%s", e.Field, e.Reason)
}

// QuotePerfKVValue is the canonical writer half of the perf text codec. The
// edge TrimSpace preserves the historical five-writer wire contract; embedded
// whitespace, quotes, backslashes, CJK and controls are encoded by strconv.
func QuotePerfKVValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "\"\""
	}
	return strconv.Quote(raw)
}

// ParsePerfKV consumes the complete perf_sample field body. It never attempts
// to resynchronize after malformed quoting: once a value boundary is unknown,
// no later key-looking bytes may acquire identity authority.
func ParsePerfKV(body string) ([]PerfKVField, *PerfKVError) {
	if len(body) > MaxPerfKVBodyBytes {
		return nil, &PerfKVError{Reason: "body_too_long"}
	}
	fields := make([]PerfKVField, 0, 24)
	for pos := 0; ; {
		for pos < len(body) && isHorizontalSpace(body[pos]) {
			pos++
		}
		if pos == len(body) {
			return fields, nil
		}
		if len(fields) >= MaxPerfKVFields {
			return nil, &PerfKVError{Reason: "field_count_exceeded"}
		}

		keyStart := pos
		if !isKeyStart(body[pos]) {
			return nil, &PerfKVError{Reason: "invalid_key_boundary"}
		}
		pos++
		for pos < len(body) && isKeyContinue(body[pos]) {
			pos++
		}
		if pos-keyStart > MaxPerfKVKeyBytes {
			return nil, &PerfKVError{Reason: "key_too_long"}
		}
		key := body[keyStart:pos]
		for pos < len(body) && isHorizontalSpace(body[pos]) {
			pos++
		}
		if pos >= len(body) || body[pos] != '=' {
			return nil, &PerfKVError{Field: key, Reason: "missing_equals"}
		}
		pos++
		for pos < len(body) && isHorizontalSpace(body[pos]) {
			pos++
		}
		if pos == len(body) {
			return nil, &PerfKVError{Field: key, Reason: "missing_value"}
		}

		valueStart := pos
		field := PerfKVField{Key: key}
		switch body[pos] {
		case '"':
			field.Quoted = true
			end, reason := scanQuotedValue(body, pos, '"')
			if reason != "" {
				return nil, &PerfKVError{Field: key, Reason: reason}
			}
			field.Raw = body[valueStart:end]
			if len(field.Raw) > MaxPerfKVEncodedValueBytes {
				return nil, &PerfKVError{Field: key, Reason: "value_too_long"}
			}
			decoded, err := strconv.Unquote(field.Raw)
			if err != nil {
				return nil, &PerfKVError{Field: key, Reason: "invalid_escape"}
			}
			field.Value = decoded
			pos = end
		case '\'':
			// Historical direct perf text occasionally used arbitrary single-
			// quoted strings. Preserve that compatibility with a bounded lexer;
			// unlike the canonical Go literal lane, unknown backslash pairs stay
			// byte-exact instead of inventing a second escape vocabulary.
			field.Quoted = true
			end, reason := scanQuotedValue(body, pos, '\'')
			if reason != "" {
				return nil, &PerfKVError{Field: key, Reason: reason}
			}
			field.Raw = body[valueStart:end]
			if len(field.Raw) > MaxPerfKVEncodedValueBytes {
				return nil, &PerfKVError{Field: key, Reason: "value_too_long"}
			}
			field.Value = decodeLegacySingleQuoted(field.Raw[1 : len(field.Raw)-1])
			pos = end
		default:
			for pos < len(body) && !isHorizontalSpace(body[pos]) {
				if body[pos] == '"' || body[pos] == '\'' || body[pos] == '\r' || body[pos] == '\n' {
					return nil, &PerfKVError{Field: key, Reason: "quote_in_bare_value"}
				}
				if isRawControl(body[pos]) {
					return nil, &PerfKVError{Field: key, Reason: "raw_control_in_value"}
				}
				pos++
			}
			field.Raw = body[valueStart:pos]
			if field.Raw == "" {
				return nil, &PerfKVError{Field: key, Reason: "missing_value"}
			}
			if len(field.Raw) > MaxPerfKVEncodedValueBytes {
				return nil, &PerfKVError{Field: key, Reason: "value_too_long"}
			}
			field.Value = field.Raw
		}
		if pos < len(body) && !isHorizontalSpace(body[pos]) {
			return nil, &PerfKVError{Field: key, Reason: "trailing_bytes_after_value"}
		}
		fields = append(fields, field)
	}
}

func scanQuotedValue(body string, start int, quote byte) (int, string) {
	for pos := start + 1; pos < len(body); pos++ {
		switch body[pos] {
		case '\\':
			if pos+1 >= len(body) {
				return 0, "trailing_escape"
			}
			if body[pos+1] == '\r' || body[pos+1] == '\n' {
				return 0, "raw_line_break_in_literal"
			}
			if isRawControl(body[pos+1]) {
				return 0, "raw_control_in_literal"
			}
			pos++
		case quote:
			return pos + 1, ""
		case '\r', '\n':
			return 0, "raw_line_break_in_literal"
		default:
			if isRawControl(body[pos]) {
				return 0, "raw_control_in_literal"
			}
		}
	}
	return 0, "unclosed_quote"
}

func decodeLegacySingleQuoted(raw string) string {
	var out strings.Builder
	out.Grow(len(raw))
	for pos := 0; pos < len(raw); pos++ {
		if raw[pos] != '\\' || pos+1 >= len(raw) {
			out.WriteByte(raw[pos])
			continue
		}
		next := raw[pos+1]
		if next == '\\' || next == '\'' {
			out.WriteByte(next)
		} else {
			out.WriteByte('\\')
			out.WriteByte(next)
		}
		pos++
	}
	return out.String()
}

func isHorizontalSpace(b byte) bool { return b == ' ' || b == '\t' }

func isRawControl(b byte) bool { return b < 0x20 || b == 0x7f }

func isKeyStart(b byte) bool {
	return b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isKeyContinue(b byte) bool {
	return isKeyStart(b) || b >= '0' && b <= '9'
}
