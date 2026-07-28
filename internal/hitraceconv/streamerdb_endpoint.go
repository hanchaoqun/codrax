package hitraceconv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const maxTraceDBSystraceLineBytes = 1 << 20

type traceDBOutputInvariantError struct {
	Reason string
	Cause  error
}

func (e *traceDBOutputInvariantError) Error() string {
	return "trace DB output invariant rejected: " + e.Reason
}

// Unwrap preserves the precise lower-level error graph for authority and
// cancellation classification without placing private staging paths in the
// customer-facing Error string.
func (e *traceDBOutputInvariantError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
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
	return prepareTraceDBRenderedRowEnvelope(tsNS, seq, task, tid, tgid, cpu, 0, 0, false, body)
}

// prepareTraceDBCPUUnavailableTraceMarkRow publishes a versioned comment
// record when a callstack span is proven but its physical CPU is not. Generic
// ftrace readers ignore the comment; Codrax trace_query reconstructs the marker
// with an explicit cpu_status=unavailable field. This is the only alternative
// to the concrete-CPU ftrace envelope and intentionally has no CPU parameter.
func prepareTraceDBCPUUnavailableTraceMarkRow(tsNS int64, seq int, task string,
	tid, tgid, spanPID int64, action, name, value string, placement traceDBSyncSpanCPUPlacement,
) (renderedRow, error) {
	if seq < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sequence"}
	}
	if tsNS < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_timestamp"}
	}
	if tid <= 0 || tid > math.MaxInt32 || tgid <= 0 || tgid > math.MaxInt32 ||
		spanPID <= 0 || spanPID > math.MaxInt32 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu_unavailable_trace_mark_identity"}
	}
	if !traceDBSinglePhysicalLine(task, false) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_task"}
	}
	reason := traceDBSyncSpanCPUUnavailableReason(placement)
	if reason == "" {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sync_span_cpu_placement"}
	}
	line, err := tracequery.FormatCPUUnavailableTraceMark(tracequery.CPUUnavailableTraceMark{
		TimestampNS: uint64(tsNS),
		TID:         int(tid),
		TGID:        int(tgid),
		SpanPID:     int(spanPID),
		Action:      action,
		Comm:        task,
		Name:        name,
		Value:       value,
		Reason:      reason,
	})
	if err != nil {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu_unavailable_trace_mark", Cause: err}
	}
	if len(line) > maxTraceDBSystraceLineBytes {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	return renderedRow{tsNS: uint64(tsNS), seq: seq, line: line}, nil
}

// prepareTraceDBExactTraceMarkRow publishes a versioned typed marker when the
// physical CPU is known but the standard tracing_mark_write wire cannot
// preserve both the complete payload and the exact nanosecond timestamp. It
// preserves the exact source text and timestamp without inventing an escape
// convention.
func prepareTraceDBExactTraceMarkRow(tsNS int64, seq int, task string,
	tid, tgid, cpu, spanPID int64, action, name, value string,
) (renderedRow, error) {
	if seq < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sequence"}
	}
	if tsNS < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_timestamp"}
	}
	if tid <= 0 || tid > math.MaxInt32 || tgid <= 0 || tgid > math.MaxInt32 ||
		spanPID <= 0 || spanPID > math.MaxInt32 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_exact_trace_mark_identity"}
	}
	if !validTraceDBCPUIndex(cpu) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu"}
	}
	if !traceDBSinglePhysicalLine(task, false) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_task"}
	}
	line, err := tracequery.FormatExactTraceMark(tracequery.ExactTraceMark{
		TimestampNS: uint64(tsNS),
		CPU:         int(cpu),
		TID:         int(tid),
		TGID:        int(tgid),
		SpanPID:     int(spanPID),
		Action:      action,
		Comm:        task,
		Name:        name,
		Value:       value,
	})
	if err != nil {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_exact_trace_mark", Cause: err}
	}
	if len(line) > maxTraceDBSystraceLineBytes {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	return renderedRow{tsNS: uint64(tsNS), seq: seq, line: line}, nil
}

// prepareTraceDBCompletedAsyncIntervalRow preserves one official high-level
// async interval as a single typed record. The DB proves the start emitter and
// completed interval, but not a physical finish emitter/CPU, so this function
// must never synthesize S/F endpoints.
func prepareTraceDBCompletedAsyncIntervalRow(row traceDBCallstackRow, seq int) (renderedRow, error) {
	if seq < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sequence"}
	}
	if !row.OfficialAsyncInterval || row.ID <= 0 || row.TS < 0 || row.End < row.TS ||
		row.TID <= 0 || row.TID > math.MaxInt32 ||
		row.HeaderTGID <= 0 || row.HeaderTGID > math.MaxInt32 ||
		row.TGID <= 0 || row.TGID > math.MaxInt32 ||
		!traceDBSinglePhysicalLine(row.Task, false) ||
		!traceDBCallstackSpanName(row.Name) ||
		!traceDBCallstackMarkerToken(row.Cookie) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_completed_async_interval"}
	}
	startCPU := -1
	startCPUStatus := tracequery.TraceMarkCPUStatusUnavailable
	startCPUReason := traceDBSyncSpanCPUUnavailableReason(row.CPUPlacement)
	if row.CPUPlacement == traceDBSyncSpanCPUPlacementKnown {
		if !validTraceDBCPUIndex(row.StartCPU) {
			return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu"}
		}
		startCPU = int(row.StartCPU)
		startCPUStatus = tracequery.TraceAsyncIntervalCPUStatusKnown
		startCPUReason = ""
	} else if startCPUReason == "" {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sync_span_cpu_placement"}
	}
	line, err := tracequery.FormatCompletedAsyncInterval(tracequery.CompletedAsyncInterval{
		StartTimestampNS: uint64(row.TS),
		EndTimestampNS:   uint64(row.End),
		SourceRow:        uint64(row.ID),
		TID:              int(row.TID),
		TGID:             int(row.HeaderTGID),
		SpanPID:          int(row.TGID),
		StartCPU:         startCPU,
		StartCPUStatus:   startCPUStatus,
		StartCPUReason:   startCPUReason,
		Comm:             row.Task,
		Name:             row.Name,
		Cookie:           row.Cookie,
	})
	if err != nil {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_completed_async_interval", Cause: err}
	}
	if len(line) > maxTraceDBSystraceLineBytes {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	return renderedRow{tsNS: uint64(row.TS), seq: seq, line: line}, nil
}

// prepareTraceDBCPUUnavailableWakeupRow preserves a proven scheduler
// dependency when only the physical emitter CPU is unavailable. It emits no
// ftrace CPU envelope; the exact versioned comment is reconstructed as a typed
// wakeup by tracequery while generic readers safely ignore it.
func prepareTraceDBCPUUnavailableWakeupRow(tsNS int64, seq int, eventName string,
	wakerTask string, wakerTID, wakerTGID int64, wakeeTask string, wakeeTID,
	targetCPU, priority int64, priorityKnown bool, reason string,
) (renderedRow, error) {
	if seq < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sequence"}
	}
	if tsNS < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_timestamp"}
	}
	if wakerTID <= 0 || wakerTID > math.MaxInt32 ||
		wakerTGID <= 0 || wakerTGID > math.MaxInt32 ||
		wakeeTID <= 0 || wakeeTID > math.MaxInt32 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu_unavailable_wakeup_identity"}
	}
	if targetCPU < 0 || targetCPU > maxTraceDBCPUIndex ||
		priority < math.MinInt32 || priority > math.MaxInt32 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu_unavailable_wakeup_numeric_field"}
	}
	if !traceDBSinglePhysicalLine(wakerTask, false) || !traceDBSinglePhysicalLine(wakeeTask, false) {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu_unavailable_wakeup_comm"}
	}
	prioritySource := tracequery.WakeePrioritySourceUnknown
	if priorityKnown {
		prioritySource = tracequery.WakeePrioritySourceInferredNextSchedSlice
	} else {
		priority = 0
	}
	line, err := tracequery.FormatCPUUnavailableWakeup(tracequery.CPUUnavailableWakeup{
		TimestampNS:    uint64(tsNS),
		EventName:      eventName,
		WakerTID:       int(wakerTID),
		WakerTGID:      int(wakerTGID),
		WakeeTID:       int(wakeeTID),
		TargetCPU:      int(targetCPU),
		Priority:       int(priority),
		PrioritySource: prioritySource,
		WakerComm:      wakerTask,
		WakeeComm:      wakeeTask,
		Reason:         reason,
	})
	if err != nil {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_cpu_unavailable_wakeup", Cause: err}
	}
	if len(line) > maxTraceDBSystraceLineBytes {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	return renderedRow{tsNS: uint64(tsNS), seq: seq, line: line}, nil
}

// prepareTraceDBFrameMapRow preserves a relation between two already-admitted
// frame_slice rows without inventing a physical CPU/thread envelope or a
// duration. Generic readers ignore the comment; tracequery restores it as an
// EventFrameMap relation.
func prepareTraceDBFrameMapRow(destinationTS int64, seq int, relationID, sourceRow, destinationRow, sourceTS int64) (renderedRow, error) {
	if seq < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_sequence"}
	}
	if destinationTS < 0 || sourceTS < 0 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_timestamp"}
	}
	for _, value := range []int64{relationID, sourceRow, destinationRow} {
		if value < 0 || value > math.MaxUint32 {
			return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_frame_map_identity"}
		}
	}
	line, err := tracequery.FormatFrameMapRelation(tracequery.FrameMapRelation{
		TimestampNS:       uint64(destinationTS),
		RelationID:        uint32(relationID),
		SourceRow:         uint32(sourceRow),
		DestinationRow:    uint32(destinationRow),
		SourceTimestampNS: uint64(sourceTS),
	})
	if err != nil {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_frame_map_relation", Cause: err}
	}
	if len(line) > maxTraceDBSystraceLineBytes {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	return renderedRow{tsNS: uint64(destinationTS), seq: seq, line: line}, nil
}

// prepareTraceDBRenderedRowWithTraceFlags is the same validated endpoint
// primitive with an exact ftrace common_flags/common_preempt_count header.
// Structured ftrace owns those typed values directly. A SQL-derived row may
// use this primitive only after a separate exact raw-record join proves the
// same physical event; ordinary SQL exporters retain the zero/default header.
func prepareTraceDBRenderedRowWithTraceFlags(tsNS int64, seq int, task string, tid, tgid, cpu, flags, preemptCount int64, body string) (renderedRow, error) {
	return prepareTraceDBRenderedRowWithTraceFlagsContext(context.Background(), tsNS, seq, task, tid, tgid, cpu, flags, preemptCount, body)
}

func prepareTraceDBRenderedRowWithTraceFlagsContext(ctx context.Context, tsNS int64, seq int, task string,
	tid, tgid, cpu, flags, preemptCount int64, body string,
) (renderedRow, error) {
	return prepareTraceDBRenderedRowEnvelopeContext(ctx, tsNS, seq, task, tid, tgid, cpu, flags, preemptCount, true, body)
}

func prepareTraceDBRenderedRowEnvelope(tsNS int64, seq int, task string, tid, tgid, cpu, flags, preemptCount int64, allowUnknownTGID bool, body string) (renderedRow, error) {
	return prepareTraceDBRenderedRowEnvelopeContext(context.Background(), tsNS, seq, task, tid, tgid, cpu, flags, preemptCount, allowUnknownTGID, body)
}

func prepareTraceDBRenderedRowEnvelopeContext(ctx context.Context, tsNS int64, seq int, task string,
	tid, tgid, cpu, flags, preemptCount int64, allowUnknownTGID bool, body string,
) (renderedRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return renderedRow{}, err
	}
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
	if tid == 0 && tgid != 0 || tid != 0 && tgid == 0 && !allowUnknownTGID {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "incomplete_header_identity"}
	}
	if flags < 0 || flags > math.MaxUint8 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_trace_flags"}
	}
	if preemptCount < 0 || preemptCount > math.MaxUint8 {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_preempt_count"}
	}
	taskValid, err := profilerSinglePhysicalLineStringContext(ctx, task, true)
	if err != nil {
		return renderedRow{}, err
	}
	if !taskValid {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_task"}
	}
	bodyValid, err := profilerSinglePhysicalLineStringContext(ctx, body, false)
	if err != nil {
		return renderedRow{}, err
	}
	if !bodyValid {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "invalid_body"}
	}
	line := traceDBFormatLine(task, tid, tgid, cpu, tsNS, flags, preemptCount, body)
	if err := ctx.Err(); err != nil {
		return renderedRow{}, err
	}
	if len(line) > maxTraceDBSystraceLineBytes {
		return renderedRow{}, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	// The sorter key must match the timestamp tracequery will recover from the
	// physical line. Standard ftrace text carries six fractional digits, so
	// retaining the source nanoseconds here can place an exact-nanosecond typed
	// comment on the wrong side of this rounded row and create a clock
	// regression in an otherwise ordered output.
	return renderedRow{tsNS: traceDBStandardWireTimestampNS(tsNS), seq: seq, line: line}, nil
}

func traceDBSinglePhysicalLine(value string, allowBlank bool) bool {
	valid, _ := profilerSinglePhysicalLineStringContext(context.Background(), value, allowBlank)
	return valid
}

// traceDBFormatLine is deliberately a pure formatter for a validated
// envelope. Production callers must go through prepareTraceDBRenderedRow.
func traceDBFormatLine(task string, tid, tgid, cpu, tsNS, flags, preemptCount int64, body string) string {
	task = traceDBCommName(task, "unknown")
	tgidText := strconv.FormatInt(tgid, 10)
	if tid != 0 && tgid == 0 {
		tgidText = "-----"
	}
	return fmt.Sprintf("%16s-%-5d (%5s) [%03d] %s %s: %s",
		task, tid, tgidText, cpu, traceFlagsToStr(flags, preemptCount), formatTimestamp(uint64(tsNS)), body)
}

// traceDBStandardWireTimestampNS is the nanosecond value represented by a
// standard six-decimal ftrace timestamp. Exact typed comment rows do not use
// this helper: their ts_ns field remains nanosecond-precise on the wire.
func traceDBStandardWireTimestampNS(tsNS int64) uint64 {
	return roundedTimestampUS(uint64(tsNS)) * 1000
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
