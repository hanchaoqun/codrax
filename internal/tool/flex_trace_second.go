package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// TraceSecond accepts trace timestamps as JSON numbers or natural unit strings.
// It normalizes everything to trace-clock seconds while preserving enough input
// precision to apply a tiny query tolerance for shortened fractional values.
type TraceSecond struct {
	seconds        float64
	set            bool
	raw            string
	unit           string
	fractionDigits int
	scale          float64
	stringEncoded  bool
}

func (ts TraceSecond) Seconds() float64 { return ts.seconds }
func (ts TraceSecond) Set() bool        { return ts.set }
func (ts TraceSecond) Raw() string      { return ts.raw }

func (ts TraceSecond) QueryToleranceSeconds() float64 {
	if !ts.set || ts.seconds == 0 {
		return 0
	}
	var tol float64
	switch ts.unit {
	case "ms":
		tol = 0.5 * math.Pow10(-ts.fractionDigits) * 0.001
	case "us":
		tol = 0.5 * math.Pow10(-ts.fractionDigits) * 0.000001
	default:
		if ts.fractionDigits <= 0 || ts.fractionDigits >= 6 {
			return 0
		}
		tol = 0.5 * math.Pow10(-ts.fractionDigits)
	}
	const maxTolerance = 0.0005 // 0.5ms: enough for shortened trace stamps, not a broad search.
	if tol > maxTolerance {
		return maxTolerance
	}
	return tol
}

func (ts *TraceSecond) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*ts = TraceSecond{}
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return fmt.Errorf("trace-second: %w", err)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			*ts = TraceSecond{}
			return nil
		}
		parsed, err := parseTraceSecondLiteral(s)
		if err != nil {
			return err
		}
		parsed.stringEncoded = true
		*ts = parsed
		return nil
	}
	raw := string(trimmed)
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("trace-second: cannot parse %q as seconds", raw)
	}
	*ts = TraceSecond{
		seconds:        value,
		set:            true,
		raw:            raw,
		unit:           "s",
		fractionDigits: decimalDigits(raw),
		scale:          1,
	}
	return nil
}

func parseTraceSecondLiteral(raw string) (TraceSecond, error) {
	s := strings.TrimSpace(raw)
	lower := strings.ToLower(strings.ReplaceAll(s, " ", ""))
	unit := "s"
	scale := 1.0
	switch {
	case strings.HasSuffix(lower, "毫秒"):
		unit, scale = "ms", 0.001
		lower = strings.TrimSuffix(lower, "毫秒")
	case strings.HasSuffix(lower, "ms"):
		unit, scale = "ms", 0.001
		lower = strings.TrimSuffix(lower, "ms")
	case strings.HasSuffix(lower, "微秒"):
		unit, scale = "us", 0.000001
		lower = strings.TrimSuffix(lower, "微秒")
	case strings.HasSuffix(lower, "µs"):
		unit, scale = "us", 0.000001
		lower = strings.TrimSuffix(lower, "µs")
	case strings.HasSuffix(lower, "us"):
		unit, scale = "us", 0.000001
		lower = strings.TrimSuffix(lower, "us")
	case strings.HasSuffix(lower, "seconds"):
		lower = strings.TrimSuffix(lower, "seconds")
	case strings.HasSuffix(lower, "second"):
		lower = strings.TrimSuffix(lower, "second")
	case strings.HasSuffix(lower, "secs"):
		lower = strings.TrimSuffix(lower, "secs")
	case strings.HasSuffix(lower, "sec"):
		lower = strings.TrimSuffix(lower, "sec")
	case strings.HasSuffix(lower, "秒"):
		lower = strings.TrimSuffix(lower, "秒")
	case strings.HasSuffix(lower, "s"):
		lower = strings.TrimSuffix(lower, "s")
	}
	lower = strings.TrimSpace(lower)
	if lower == "" {
		return TraceSecond{}, fmt.Errorf("trace-second: missing numeric timestamp in %q", raw)
	}
	value, err := strconv.ParseFloat(lower, 64)
	if err != nil {
		return TraceSecond{}, fmt.Errorf("trace-second: cannot parse %q as a trace timestamp", raw)
	}
	return TraceSecond{
		seconds:        value * scale,
		set:            true,
		raw:            raw,
		unit:           unit,
		fractionDigits: decimalDigits(lower),
		scale:          scale,
	}, nil
}

func decimalDigits(raw string) int {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		raw = raw[:i]
	}
	dot := strings.IndexByte(raw, '.')
	if dot < 0 {
		return 0
	}
	return len(raw) - dot - 1
}
