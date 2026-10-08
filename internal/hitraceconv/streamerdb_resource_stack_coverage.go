package hitraceconv

import (
	"context"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// Frame coverage is source-row accounting, separate from the event carriers
// that can repeat one callchain for several resource observations. A table
// roster entry does not imply that every row or every field was exported.
type traceDBResourceFrames struct {
	byChain   map[int64][]tracewire.ResourceFrame
	available bool
	coverage  TraceDBCoverage
	published map[int64]struct{}
}

func (f *traceDBResourceFrames) inspectColumns(ctx context.Context, tdb *traceDB, columns []string) error {
	f.coverage.FieldSources = map[string]string{
		"row_accounting": "rows_read is the complete source table row count; referenced_frame_rows counts decoded physical source rows; published_source_frame_rows is distinct source rows; rows_emitted counts repeated per-event frame carriers",
		"selection":      "same sealed capture exact native_hook.callchain_id integer reference; publication additionally requires the resource event's proven lifecycle owner",
		"field_status":   "nullable, absent, malformed and ambiguous values remain typed statuses; status counters count each decoded source frame once, not repeated carriers; over-limit text is invalid, never truncated into a known symbol",
	}
	known := []string{"callchain_id", "depth", "id", "ip", "symbol_id", "file_id", "offset", "symbol_offset", "vaddr"}
	var extra []string
	for _, column := range columns {
		// Required columns are direct SQL identifiers and resolve with SQLite's
		// identifier folding. Optional selects retain their existing exact-name
		// admission; do not call an unread optional column converted.
		for _, required := range []string{"callchain_id", "depth"} {
			if sqliteASCIIIdentifierEqual(column, required) {
				column = required
			}
		}
		if traceDBStringSliceContains(known, column) {
			if !traceDBStringSliceContains(f.coverage.ColumnsPresent, column) {
				f.coverage.ColumnsPresent = append(f.coverage.ColumnsPresent, column)
			}
		} else {
			extra = append(extra, column)
		}
	}
	sort.Strings(f.coverage.ColumnsPresent)
	if len(extra) > 0 {
		sort.Strings(extra)
		f.coverage.FieldSources["unconverted_columns"] = strings.Join(extra, ",")
		traceDBAddCoverageMetric(&f.coverage, "unconverted_columns", int64(len(extra)))
	}
	var invalid int64
	if err := tdb.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM native_hook_frame WHERE typeof(callchain_id)!='integer' OR callchain_id<0 OR callchain_id>=4294967295`).Scan(&invalid); err != nil {
		return err
	}
	traceDBAddCoverageMetric(&f.coverage, "invalid_callchain_rows", invalid)
	return nil
}

func (f *traceDBResourceFrames) decoded(frame tracewire.ResourceFrame) {
	traceDBAddCoverageMetric(&f.coverage, "referenced_frame_rows", 1)
	for name, status := range map[string]string{
		"source_id": frame.SourceID.Status, "depth": frame.Depth.Status, "ip": frame.IP.Status,
		"symbol_id": frame.SymbolID.Status, "file_id": frame.FileID.Status, "offset": frame.Offset.Status,
		"symbol_offset": frame.SymbolOffset.Status, "vaddr": frame.VAddr.Status,
		"symbol": frame.Symbol.Status, "library": frame.Library.Status,
	} {
		if status != "known" {
			traceDBAddCoverageMetric(&f.coverage, name+"_"+status, 1)
		}
	}
}

func (f *traceDBResourceFrames) emitted(rowID int64) {
	f.coverage.RowsEmitted++
	if f.published == nil {
		f.published = map[int64]struct{}{}
	}
	f.published[rowID] = struct{}{}
}

func (f *traceDBResourceFrames) result() TraceDBCoverage {
	if f.available {
		traceDBAddCoverageMetric(&f.coverage, "published_source_frame_rows", int64(len(f.published)))
		traceDBAddCoverageMetric(&f.coverage, "referenced_unpublished_frame_rows", f.coverage.Metrics["referenced_frame_rows"]-int64(len(f.published)))
		traceDBAddCoverageMetric(&f.coverage, "unreferenced_frame_rows", int64(f.coverage.RowsRead)-f.coverage.Metrics["referenced_frame_rows"]-f.coverage.Metrics["invalid_callchain_rows"])
	}
	residual := map[string]int{}
	for _, key := range []string{"invalid_callchain_rows", "unreferenced_frame_rows", "referenced_unpublished_frame_rows", "unconverted_columns"} {
		if n := f.coverage.Metrics[key]; n > 0 {
			residual[key] = int(n)
		}
	}
	traceDBAppendCoverageSkipped(&f.coverage, traceDBCountSummary(residual))
	return f.coverage
}
