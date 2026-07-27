package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxTraceDBParserMetadataRows       = 64
	maxTraceDBParserMetadataNameBytes  = 128
	maxTraceDBParserMetadataValueBytes = 1024
	maxTraceDBParserMetadataTotalBytes = 16 << 10
)

// inspectTraceDBDiagnosticMetadata preserves bounded device/parser context as
// coverage metadata. These values are deliberately outside the systrace row
// sink and never participate in source admission or hard gates.
func inspectTraceDBDiagnosticMetadata(ctx context.Context, tdb *traceDB) ([]TraceDBCoverage, error) {
	device, err := inspectTraceDBDeviceInfoMetadata(ctx, tdb)
	if err != nil {
		return []TraceDBCoverage{device}, err
	}
	parser, err := inspectTraceDBParserMetadata(ctx, tdb)
	return []TraceDBCoverage{device, parser}, err
}

func inspectTraceDBDeviceInfoMetadata(ctx context.Context, tdb *traceDB) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "metadata", "device_info",
		[]string{"physical_width", "physical_height", "physical_frame_rate"})
	coverage.Role = "diagnostic_metadata"
	coverage.FieldSources = map[string]string{
		"schema":              "official trace_streamer device_info singleton; exact SQLite INTEGER fields only",
		"dimensions":          "physical_width/physical_height raw positive integers; producer schema does not prove a unit here",
		"physical_frame_rate": "raw positive integer from device_info; preserved for display only because this table alone does not prove timing units or frame-deadline authority",
		"effect":              "bounded diagnostic metadata only; never emitted as ftrace and never used by source admission or a hard gate",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	if coverage.RowsRead != 1 {
		coverage.Skipped = fmt.Sprintf("expected exactly one device_info row, got %d", coverage.RowsRead)
		return coverage, nil
	}
	var widthRaw, heightRaw, frameRateRaw any
	if err := tdb.db.QueryRowContext(ctx,
		`SELECT physical_width, physical_height, physical_frame_rate FROM device_info`,
	).Scan(&widthRaw, &heightRaw, &frameRateRaw); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	coverage.Metadata = map[string]string{}
	invalid := 0
	for _, field := range []struct {
		name string
		raw  any
	}{
		{name: "physical_width", raw: widthRaw},
		{name: "physical_height", raw: heightRaw},
		{name: "physical_frame_rate", raw: frameRateRaw},
	} {
		value, ok := traceDBStrictSQLiteInt(field.raw)
		if !ok {
			if field.raw == nil {
				traceDBAddCoverageMetric(&coverage, field.name+"_unavailable", 1)
			} else {
				invalid++
				traceDBAddCoverageMetric(&coverage, field.name+"_invalid", 1)
			}
			continue
		}
		if value <= 0 || value > math.MaxInt32 {
			invalid++
			traceDBAddCoverageMetric(&coverage, field.name+"_invalid", 1)
			continue
		}
		coverage.Metadata[field.name] = strconv.FormatInt(value, 10)
		traceDBAddCoverageMetric(&coverage, field.name+"_available", 1)
		traceDBAddCoverageMetric(&coverage, field.name, value)
	}
	if len(coverage.Metadata) > 0 {
		coverage.RowsEmitted = 1
	}
	if invalid > 0 {
		coverage.Skipped = fmt.Sprintf("invalid_device_info_fields=%d", invalid)
	}
	return coverage, nil
}

func inspectTraceDBParserMetadata(ctx context.Context, tdb *traceDB) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "metadata", "meta", []string{"name", "value"})
	coverage.Role = "diagnostic_metadata"
	coverage.FieldSources = map[string]string{
		"schema":     "official trace_streamer meta(name,value) parser metadata",
		"bounds":     "at most 64 rows, 128 UTF-8 bytes per name, 1024 UTF-8 bytes per value, and 16 KiB surfaced key/value bytes",
		"duplicates": "duplicate names are omitted monotonically even when their values are equal",
		"effect":     "exact bounded text is display diagnostics only; arbitrary keys never become source-admission or hard-gate authority and no row becomes ftrace",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}
	rows, err := tdb.db.QueryContext(ctx, `
		SELECT
			CASE
				WHEN typeof(name)='text'
				 AND length(CAST(name AS BLOB)) BETWEEN 1 AND ?
				THEN name
			END,
			CASE
				WHEN typeof(value)='text'
				 AND length(CAST(value AS BLOB)) <= ?
				THEN value
			END
		FROM meta
		ORDER BY name
		LIMIT ?`,
		maxTraceDBParserMetadataNameBytes,
		maxTraceDBParserMetadataValueBytes,
		maxTraceDBParserMetadataRows,
	)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	candidates := map[string]string{}
	invalidNames := map[string]bool{}
	reasons := map[string]int{}
	scanned := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		scanned++
		var nameRaw, valueRaw any
		if err := rows.Scan(&nameRaw, &valueRaw); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		name, nameOK := nameRaw.(string)
		value, valueOK := valueRaw.(string)
		if !nameOK || !valueOK || !validTraceDBParserMetadataName(name) || !utf8.ValidString(value) {
			reasons["invalid_or_oversized_metadata_row"]++
			continue
		}
		if invalidNames[name] {
			reasons["duplicate_metadata_name"]++
			continue
		}
		if _, duplicate := candidates[name]; duplicate {
			delete(candidates, name)
			invalidNames[name] = true
			reasons["duplicate_metadata_name"]++
			continue
		}
		candidates[name] = value
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	if coverage.RowsRead > scanned {
		reasons["metadata_rows_not_surfaced_by_row_cap"] += coverage.RowsRead - scanned
		traceDBAddCoverageMetric(&coverage, "metadata_truncated", 1)
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	coverage.Metadata = map[string]string{}
	totalBytes := 0
	for _, name := range names {
		value := candidates[name]
		nextBytes := len(name) + len(value)
		if nextBytes > maxTraceDBParserMetadataTotalBytes-totalBytes {
			reasons["metadata_rows_not_surfaced_by_byte_cap"]++
			continue
		}
		coverage.Metadata[name] = value
		totalBytes += nextBytes
	}
	coverage.RowsEmitted = len(coverage.Metadata)
	traceDBAddCoverageMetric(&coverage, "metadata_rows_surfaced", int64(coverage.RowsEmitted))
	traceDBAddCoverageMetric(&coverage, "metadata_bytes_surfaced", int64(totalBytes))
	traceDBAddCoverageMetric(&coverage, "metadata_names_poisoned", int64(len(invalidNames)))
	coverage.Skipped = traceDBCountSummary(reasons)
	return coverage, nil
}

func validTraceDBParserMetadataName(name string) bool {
	if name == "" || len(name) > maxTraceDBParserMetadataNameBytes || !utf8.ValidString(name) ||
		strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.IsSpace(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
	}
	return true
}
