package hitraceconv

import (
	"context"
	"fmt"
	"sort"
)

// Trace Streamer's high-level dma_fence.dur is not a fence wait duration. The
// producer stores the delta from the previous arbitrary fence event on the
// same timeline, without pairing by context or sequence number. The table also
// carries no emitter PID/TID/ITID or CPU. Rendering either a span or an instant
// would therefore invent elapsed-time and/or CPU ownership. Keep the table in
// coverage and let the strict raw-ftrace exporter remain the sole systrace
// authority when a vendor schema actually provides DMA args, emitter and CPU.
func exportTraceDBDMAFence(ctx context.Context, tdb *traceDB, _ *traceDBRowSink, _ traceDBThreadIndex, _ map[int64][]traceDBRunningInterval, _ map[int64]string) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "slice", "dma_fence", []string{"ts", "dur", "cat", "driver", "timeline", "context", "seqno"})
	// This table is deliberately coverage-only: it cannot produce the text rows
	// that query_ready_export promises to trace_query or to the model.
	coverage.Role = "unsupported_input"
	coverage.FieldSources = map[string]string{
		"dur":        "producer previous-event delta within timeline; not a wait duration and never exported as B/E or S/F",
		"identity":   "not present in high-level schema: no pid/tid/itid emitter identity",
		"header_cpu": "not present in high-level schema; CPU 0 is never used as an unknown fallback",
		"authority":  "query-ready DMA rows require the strict raw-ftrace path with complete args, emitter identity, and CPU",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	if hasID, idErr := tdb.columnExists(ctx, "dma_fence", "id"); idErr != nil {
		coverage.Error = idErr.Error()
		return coverage, idErr
	} else if hasID {
		coverage.ColumnsPresent = appendTraceDBCoverageColumn(coverage.ColumnsPresent, "id")
		sort.Strings(coverage.ColumnsPresent)
	}
	if coverage.RowsRead > 0 {
		coverage.Skipped = fmt.Sprintf("high_level_rows_withheld=%d; predecessor_delta_not_duration=true; unresolved_emitter_identity_cpu=true; raw_dma_path_only=true", coverage.RowsRead)
	}
	return coverage, nil
}
