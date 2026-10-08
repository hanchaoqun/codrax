package hitraceconv

import (
	"context"
	"math"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func traceDBHasMeasureDuration(ctx context.Context, q traceDBQueryer) (bool, error) {
	columns, err := traceDBColumnNames(ctx, q, "measure")
	return traceDBStringSliceContains(columns, "dur"), err
}

// Explicit SQL intervals are independent source records. Physical insertion
// order does not turn them into a state-transition stream, and overlapping
// records remain available for the consumer's conservative overlap audit.
func exportTraceDBCPUIntervals(ctx context.Context, q traceDBQueryer, stableExpr string, sink *traceDBRowSink, filters map[int64]traceDBCPUMeasureFilter, skipped map[string]int) (read, emitted int, err error) {
	rows, err := q.QueryContext(ctx, `SELECT `+stableExpr+`, ts, dur, value, filter_id FROM measure ORDER BY `+stableExpr)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return
		}
		var idRaw, tsRaw, durRaw, valueRaw, filterRaw any
		if err = rows.Scan(&idRaw, &tsRaw, &durRaw, &valueRaw, &filterRaw); err != nil {
			return
		}
		filterID, exactFilter := traceDBStrictSQLiteInt(filterRaw)
		if !exactFilter {
			var comparable bool
			filterID, comparable = traceDBPoisonEquivalentNonNegativeInt(filterRaw)
			if !comparable {
				continue
			}
		}
		filter, exists := filters[filterID]
		if !exists || filter.Name != "cpu_idle" && filter.Name != "cpu_frequency" {
			continue
		}
		read++
		rowID, _ := traceDBStrictSQLiteInt(idRaw) // elected hidden rowid is proven INTEGER
		r := tracewire.CPUMeasureInterval{RowID: rowID, FilterID: filterID, CPU: int(filter.CPU), Kind: "idle", Encoding: "native_sql_idle"}
		if filter.Name == "cpu_frequency" {
			r.Kind, r.Encoding = "frequency", "khz"
		}
		if ts, ok := traceDBStrictSQLiteInt(tsRaw); ok {
			r.StartNS = &ts
		} else {
			r.Issue = "invalid_timestamp"
		}
		if durRaw == nil {
			r.Issue = "unknown_duration"
		} else if dur, ok := traceDBStrictSQLiteInt(durRaw); ok && dur >= 0 && (r.StartNS == nil || *r.StartNS <= math.MaxInt64-dur) {
			r.DurationNS = &dur
		} else {
			r.Issue = "invalid_duration"
		}
		if value, ok := traceDBExactIntegralMeasureValue(valueRaw); ok && value >= 0 && value <= math.MaxUint32 {
			r.Value = &value
		} else {
			r.Issue = "invalid_value"
		}
		if r.StartNS == nil {
			r.Issue = "invalid_timestamp"
		}
		if !exactFilter {
			r.Issue = "invalid_filter_id"
		}
		if r.Issue != "" {
			skipped["cpu_interval_"+r.Issue]++
		}
		var line string
		line, err = tracewire.FormatCPUMeasureInterval(r)
		if err != nil {
			return
		}
		if err = addTraceDBTypedCommentRow(sink, r.TimestampNS(), line); err != nil {
			return
		}
		emitted++
	}
	err = rows.Err()
	return
}
