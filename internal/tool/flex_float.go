package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FlexFloat is a float64 that accepts JSON numbers and common stringified
// number forms. It is intended for tool parameters where the schema says
// "number", but LLM tool-call JSON may still contain "1", "1ms", or "0.01s".
//
// Bare values are interpreted in the field's native unit. Unit suffixes are
// supported only for duration-like values and normalize to milliseconds:
// "1ms" -> 1, "0.5s" -> 500, "100us" -> 0.1.
type FlexFloat float64

func (ff FlexFloat) Float64() float64 { return float64(ff) }

func (ff *FlexFloat) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*ff = 0
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("flex-float: %w", err)
		}
		v, err := parseFlexFloatLiteral(s)
		if err != nil {
			return err
		}
		*ff = FlexFloat(v)
		return nil
	}
	v, err := strconv.ParseFloat(string(trimmed), 64)
	if err != nil {
		return fmt.Errorf("flex-float: cannot parse %q as number", string(trimmed))
	}
	*ff = FlexFloat(v)
	return nil
}

func parseFlexFloatLiteral(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	lower := strings.ToLower(strings.ReplaceAll(s, " ", ""))
	scale := 1.0
	switch {
	case strings.HasSuffix(lower, "milliseconds"):
		lower = strings.TrimSuffix(lower, "milliseconds")
	case strings.HasSuffix(lower, "millisecond"):
		lower = strings.TrimSuffix(lower, "millisecond")
	case strings.HasSuffix(lower, "msec"):
		lower = strings.TrimSuffix(lower, "msec")
	case strings.HasSuffix(lower, "毫秒"):
		lower = strings.TrimSuffix(lower, "毫秒")
	case strings.HasSuffix(lower, "ms"):
		lower = strings.TrimSuffix(lower, "ms")
	case strings.HasSuffix(lower, "microseconds"):
		lower = strings.TrimSuffix(lower, "microseconds")
		scale = 0.001
	case strings.HasSuffix(lower, "microsecond"):
		lower = strings.TrimSuffix(lower, "microsecond")
		scale = 0.001
	case strings.HasSuffix(lower, "微秒"):
		lower = strings.TrimSuffix(lower, "微秒")
		scale = 0.001
	case strings.HasSuffix(lower, "µs"):
		lower = strings.TrimSuffix(lower, "µs")
		scale = 0.001
	case strings.HasSuffix(lower, "us"):
		lower = strings.TrimSuffix(lower, "us")
		scale = 0.001
	case strings.HasSuffix(lower, "seconds"):
		lower = strings.TrimSuffix(lower, "seconds")
		scale = 1000
	case strings.HasSuffix(lower, "second"):
		lower = strings.TrimSuffix(lower, "second")
		scale = 1000
	case strings.HasSuffix(lower, "secs"):
		lower = strings.TrimSuffix(lower, "secs")
		scale = 1000
	case strings.HasSuffix(lower, "sec"):
		lower = strings.TrimSuffix(lower, "sec")
		scale = 1000
	case strings.HasSuffix(lower, "秒"):
		lower = strings.TrimSuffix(lower, "秒")
		scale = 1000
	case strings.HasSuffix(lower, "s"):
		lower = strings.TrimSuffix(lower, "s")
		scale = 1000
	}
	if lower == "" {
		return 0, fmt.Errorf("flex-float: missing numeric value in %q", raw)
	}
	v, err := strconv.ParseFloat(lower, 64)
	if err != nil {
		return 0, fmt.Errorf("flex-float: cannot parse %q as number", raw)
	}
	return v * scale, nil
}
