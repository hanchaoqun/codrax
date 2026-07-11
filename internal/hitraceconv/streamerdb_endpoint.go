package hitraceconv

import (
	"context"
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

func addTraceDBAsyncSpanRows(sink *traceDBRowSink, start, end int64, task string, tid, tgid, startCPU, endCPU int64, name, cookie string) error {
	if end < start {
		return &traceDBOutputInvariantError{Reason: "invalid_interval"}
	}
	if tgid <= 0 {
		return &traceDBOutputInvariantError{Reason: "invalid_async_owner"}
	}
	if !traceDBCallstackMarkerToken(name) {
		return &traceDBOutputInvariantError{Reason: "invalid_span_name"}
	}
	if !traceDBCallstackMarkerToken(cookie) {
		return &traceDBOutputInvariantError{Reason: "invalid_span_cookie"}
	}
	begin, err := prepareTraceDBRenderedRow(start, sink.stats.RowsAccepted, task, tid, tgid, startCPU,
		fmt.Sprintf("tracing_mark_write: S|%d|%s|%s", tgid, name, cookie))
	if err != nil {
		return err
	}
	finish, err := prepareTraceDBRenderedRow(end, sink.stats.RowsAccepted+1, task, tid, tgid, endCPU,
		fmt.Sprintf("tracing_mark_write: F|%d|%s|%s", tgid, name, cookie))
	if err != nil {
		return err
	}
	if err := sink.add(begin); err != nil {
		return err
	}
	return sink.add(finish)
}

// traceDBWireIntervalRepresentable guards the converter's microsecond text
// boundary. Positive nanosecond intervals below one microsecond, or endpoints
// that round into the same printed timestamp, must not become zero-duration or
// materially inflated synthetic spans after a DB -> systrace round-trip.
func traceDBWireIntervalRepresentable(start, end int64) bool {
	if start < 0 || end <= start || end-start < 1000 {
		return false
	}
	return roundedTimestampUS(uint64(end)) > roundedTimestampUS(uint64(start))
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

func traceDBDuplicateSourceIDs(ctx context.Context, tdb *traceDB, table, column string,
	decode func(any) (int64, bool),
) (map[int64]bool, error) {
	columnExpr := quoteSQLiteIdent(column)
	query := fmt.Sprintf(`
		SELECT %s
		FROM %s
		WHERE typeof(%s) = 'integer'
		GROUP BY %s
		HAVING COUNT(*) > 1
	`, columnExpr, quoteSQLiteIdent(table), columnExpr, columnExpr)
	rows, err := tdb.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		id, ok := decode(raw)
		if !ok {
			// Invalid source identities remain row-local rejects. They do not gain
			// authority merely because an invalid physical value is duplicated.
			continue
		}
		out[id] = true
	}
	return out, rows.Err()
}
