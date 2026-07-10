package tracequery

import (
	"math"
	"strconv"
	"strings"
)

// maxTraceTimestampMagnitudeSec leaves enough IEEE-754 headroom for every
// duration consumer to subtract two admitted timestamps and convert seconds
// to milliseconds without producing +/-Inf. Trace clocks are many orders of
// magnitude smaller than this mechanical representation bound.
const maxTraceTimestampMagnitudeSec = math.MaxFloat64 / 4096

func isFiniteTraceNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func isSafeTraceTimestamp(value float64) bool {
	return isFiniteTraceNumber(value) && math.Abs(value) <= maxTraceTimestampMagnitudeSec
}

func parseTraceTimestampSeconds(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || !isSafeTraceTimestamp(value) {
		return 0, false
	}
	return value, true
}

// parseFiniteTraceNumber is intentionally stricter than strconv.ParseFloat:
// an overflow result, NaN, or either infinity is malformed trace input, never
// a magnitude that may participate in evidence or ranking.
func parseFiniteTraceNumber(raw string) (float64, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), ":,")
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || !isFiniteTraceNumber(value) {
		return 0, false
	}
	return value, true
}

// traceDurationMilliseconds is the common fail-closed boundary for derived
// timestamp subtraction and the seconds-to-milliseconds multiplication.
func traceDurationMilliseconds(start, end float64) (float64, bool) {
	if !isSafeTraceTimestamp(start) || !isSafeTraceTimestamp(end) || end < start {
		return 0, false
	}
	seconds := end - start
	if !isFiniteTraceNumber(seconds) {
		return 0, false
	}
	durationMs := seconds * 1000
	if !isFiniteTraceNumber(durationMs) {
		return 0, false
	}
	return durationMs, true
}
