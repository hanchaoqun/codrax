package hitraceconv

import (
	"context"
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func validateSystraceWithTraceQuery(ctx context.Context, path string) (coverage TraceDBCoverage) {
	start := time.Now()
	defer func() {
		traceDBSetCoverageElapsed(&coverage, start)
	}()
	coverage = TraceDBCoverage{
		Family: "trace_cross_validation",
		Table:  "tracequery_build_index",
		Found:  true,
	}
	idx, err := tracequery.BuildIndex(ctx, path)
	if err != nil {
		coverage.Found = false
		coverage.Error = err.Error()
		return coverage
	}
	coverage.RowsRead = idx.ScannedLineCount
	if coverage.RowsRead == 0 {
		coverage.RowsRead = idx.LineCount
	}
	coverage.RowsEmitted = len(idx.Events)
	coverage.ColumnsPresent = append(coverage.ColumnsPresent, fmt.Sprintf("parsed_known=%d", idx.ParsedKnown))
	if idx.FirstTs != 0 || idx.LastTs != 0 {
		coverage.ColumnsPresent = append(coverage.ColumnsPresent,
			fmt.Sprintf("first_ts=%.6f", idx.FirstTs),
			fmt.Sprintf("last_ts=%.6f", idx.LastTs),
		)
	}
	if idx.TraceFlavor != "" {
		coverage.ColumnsPresent = append(coverage.ColumnsPresent, "trace_flavor="+string(idx.TraceFlavor))
	}
	if idx.ParseLinePanics > 0 || idx.ClockRegressions > 0 || idx.UnparsedLines > 0 {
		coverage.Skipped = fmt.Sprintf("tracequery warnings: parse_panics=%d clock_regressions=%d unparsed_lines=%d", idx.ParseLinePanics, idx.ClockRegressions, idx.UnparsedLines)
	}
	if len(idx.Events) == 0 && coverage.Skipped == "" {
		coverage.Skipped = "tracequery parsed no queryable trace events"
	}
	return coverage
}
