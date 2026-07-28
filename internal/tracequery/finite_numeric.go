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

// ParseLineTimestampNS returns the anchored ftrace header timestamp without a
// float64 round trip. Converter lanes use this when nanoseconds participate in
// ordering: an admitted decimal must fit uint64 exactly at nanosecond
// precision, and sub-nanosecond spellings are rejected instead of rounded.
func ParseLineTimestampNS(line string) (uint64, bool) {
	if mark, ok := parseExactTraceMark(line); ok {
		return mark.TimestampNS, true
	}
	if mark, ok := parseCPUUnavailableTraceMark(line); ok {
		return mark.TimestampNS, true
	}
	if wakeup, ok := parseCPUUnavailableWakeup(line); ok {
		return wakeup.TimestampNS, true
	}
	if interval, ok := parseCompletedAsyncInterval(line); ok {
		return interval.StartTimestampNS, true
	}
	if relation, ok := parseFrameMapRelation(line); ok {
		return relation.TimestampNS, true
	}
	if record, ok := parseTraceDBTextRecord(line); ok {
		return record.TimestampNS, true
	}
	m := matchFtraceLine(line)
	if len(m) == 0 {
		return 0, false
	}
	return parseTraceTimestampNanoseconds(m[5])
}

func parseTraceTimestampNanoseconds(raw string) (uint64, bool) {
	secondsText, fractionText, hasFraction := strings.Cut(raw, ".")
	seconds, err := strconv.ParseUint(secondsText, 10, 64)
	if err != nil {
		return 0, false
	}
	var fraction uint64
	if hasFraction {
		if fractionText == "" || len(fractionText) > 9 {
			return 0, false
		}
		fraction, err = strconv.ParseUint(fractionText+strings.Repeat("0", 9-len(fractionText)), 10, 32)
		if err != nil {
			return 0, false
		}
	}
	const nanosPerSecond = uint64(1_000_000_000)
	maxUint64 := ^uint64(0)
	if seconds > maxUint64/nanosPerSecond {
		return 0, false
	}
	base := seconds * nanosPerSecond
	if fraction > maxUint64-base {
		return 0, false
	}
	return base + fraction, true
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
