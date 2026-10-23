package hitraceconv

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// The original database's declared bounds are display-only metadata. They are
// not the converted event envelope, a query window, or evidence of capture
// completeness. Keep the existing resolver's independent start-time contract.
func inspectTraceDBSourceRangeMetadata(ctx context.Context, tdb *traceDB) (coverage TraceDBCoverage, err error) {
	const boundary = "原件声明时间范围，单位 ns；不是转换事件包络、请求窗或查询窗，不证明量测完整、线程耗时或因果；端点包含规则未知"
	fields := []attachment.TraceSourceMetadataField{
		{Origin: "trace_range", Kind: "source_range", Name: "start_ts", State: "missing", Detail: boundary},
		{Origin: "trace_range", Kind: "source_range", Name: "end_ts", State: "missing", Detail: boundary},
		{Origin: "trace_range", Kind: "source_range", Name: "duration_ns", State: "missing", Detail: "原件声明端点差 end_ts-start_ts，单位 ns；不是已观测事件跨度、查询分母或执行时长"},
	}
	defer func() { coverage.SourceMetadata = attachment.NewTraceSourceMetadata(fields) }()
	coverage, err = tdb.inspectCoverage(ctx, "metadata", "trace_range", []string{"start_ts", "end_ts"})
	coverage.Role = "diagnostic_metadata"
	coverage.FieldSources = map[string]string{
		"schema": "official trace_streamer trace_range singleton; exact non-negative SQLite INTEGER nanoseconds",
		"range":  "original declared start_ts/end_ts; endpoint closure unspecified; duration_ns is their exact difference only",
		"effect": "display diagnostics only; never emitted as ftrace, never a query window, measurement denominator, coverage or causal authority",
	}
	if err != nil || !coverage.Found || coverage.RowsRead == 0 {
		return coverage, err
	}
	if coverage.RowsRead != 1 {
		for i := range fields {
			fields[i].State = "invalid"
			fields[i].Detail += "; trace_range is not a singleton"
		}
		coverage.Skipped = fmt.Sprintf("expected exactly one trace_range row, got %d", coverage.RowsRead)
		return coverage, nil
	}
	// Missing columns stay missing independently. CASE keeps malformed large
	// TEXT/BLOB values inside SQLite rather than loading a diagnostic payload.
	present := map[string]bool{}
	for _, name := range coverage.ColumnsPresent {
		present[name] = true
	}
	var expressions []string
	for _, name := range []string{"start_ts", "end_ts"} {
		if present[name] {
			expressions = append(expressions, "CASE WHEN typeof("+name+") IN ('integer','real','null') THEN "+name+" END", "typeof("+name+")")
		} else {
			expressions = append(expressions, "NULL", "'absent'")
		}
	}
	var raw [2]any
	var sqlType [2]string
	if err := tdb.db.QueryRowContext(ctx, "SELECT "+strings.Join(expressions, ",")+" FROM trace_range").Scan(&raw[0], &sqlType[0], &raw[1], &sqlType[1]); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	var values [2]int64
	for i := range raw {
		if sqlType[i] == "null" || sqlType[i] == "absent" {
			continue
		}
		value, ok := traceDBStrictSQLiteInt(raw[i])
		if !ok || value < 0 {
			fields[i].State = "invalid"
			fields[i].Detail += "; expected non-negative SQLite INTEGER, got " + sqlType[i]
			if sqlType[i] == "integer" || sqlType[i] == "real" {
				fields[i].Value = fmt.Sprint(raw[i])
			}
			continue
		}
		values[i] = value
		fields[i].State, fields[i].Value = "observed", strconv.FormatInt(value, 10)
	}
	if fields[0].State == "observed" && fields[1].State == "observed" {
		if values[1] < values[0] {
			for i := range fields {
				fields[i].State = "invalid"
				fields[i].Detail += "; declared end_ts precedes start_ts"
			}
		} else {
			// Both endpoints are non-negative int64, so subtraction cannot overflow.
			fields[2].State, fields[2].Value = "observed", strconv.FormatInt(values[1]-values[0], 10)
		}
	} else if fields[0].State == "invalid" || fields[1].State == "invalid" {
		fields[2].State = "invalid"
		fields[2].Detail += "; no difference derived from invalid endpoints"
	}
	coverage.Metadata = map[string]string{}
	for _, field := range fields {
		traceDBAddCoverageMetric(&coverage, field.Name+"_"+field.State, 1)
	}
	if fields[0].State == "observed" {
		coverage.Metadata["start_ts"] = fields[0].Value
	}
	if fields[1].State == "observed" {
		coverage.Metadata["end_ts"] = fields[1].Value
	}
	if fields[2].State == "observed" {
		coverage.Metadata["duration_ns"] = fields[2].Value
	}
	if len(coverage.Metadata) > 0 {
		coverage.RowsEmitted = 1
	}
	return coverage, nil
}
