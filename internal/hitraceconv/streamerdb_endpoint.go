package hitraceconv

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTraceDBSystraceLineBytes = 1 << 20

type traceDBOutputInvariantError struct {
	Reason string
}

func (e *traceDBOutputInvariantError) Error() string {
	return "trace DB output invariant rejected: " + e.Reason
}

func traceDBOutputInvariantReason(err error) (string, bool) {
	var invariant *traceDBOutputInvariantError
	if !errors.As(err, &invariant) {
		return "", false
	}
	return invariant.Reason, true
}

func addTraceDBInstantRow(sink *traceDBRowSink, ts int64, task string, tid, tgid, cpu int64, body string) error {
	row, err := prepareTraceDBRenderedRow(ts, sink.stats.RowsAccepted, task, tid, tgid, cpu, body)
	if err != nil {
		return err
	}
	return sink.add(row)
}

func prepareTraceDBRenderedRow(tsNS int64, seq int, task string, tid, tgid, cpu int64, body string) (renderedRow, error) {
	if seq < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sequence"}
	}
	if tsNS < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_timestamp"}
	}
	if !validTraceDBCPUIndex(cpu) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu"}
	}
	if tid < 0 || tid > math.MaxInt32 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_tid"}
	}
	if tgid < 0 || tgid > math.MaxInt32 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_tgid"}
	}
	if (tid == 0) != (tgid == 0) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "incomplete_header_identity"}
	}
	if !traceDBSinglePhysicalLine(task, true) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_task"}
	}
	if !traceDBSinglePhysicalLine(body, false) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_body"}
	}
	line := traceDBFormatLine(task, tid, tgid, cpu, tsNS, body)
	if len(line) > maxTraceDBSystraceLineBytes {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	return renderedRow{tsNS: uint64(tsNS), seq: seq, line: line}, nil
}

func traceDBSinglePhysicalLine(value string, allowBlank bool) bool {
	if len(value) > maxTraceDBSystraceLineBytes || !utf8.ValidString(value) || (!allowBlank && strings.TrimSpace(value) == "") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
	}
	return true
}

// traceDBFormatLine is deliberately a pure formatter for a validated
// envelope. Production callers must go through prepareTraceDBRenderedRow.
func traceDBFormatLine(task string, tid, tgid, cpu, tsNS int64, body string) string {
	task = traceDBCommName(task, "unknown")
	return fmt.Sprintf("%16s-%-6d (%5d) [%03d] .... %s: %s",
		task, tid, tgid, cpu, formatTimestamp(uint64(tsNS)), body)
}
