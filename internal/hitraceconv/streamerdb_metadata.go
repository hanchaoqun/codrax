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

	"github.com/hanchaoqun/codrax/internal/attachment"
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

func inspectTraceDBDeviceInfoMetadata(ctx context.Context, tdb *traceDB) (coverage TraceDBCoverage, err error) {
	fields := []attachment.TraceSourceMetadataField{
		{Origin: "device_info", Kind: "device", Name: "physical_width", State: "missing"},
		{Origin: "device_info", Kind: "device", Name: "physical_height", State: "missing"},
		{Origin: "device_info", Kind: "device", Name: "physical_frame_rate", State: "missing"},
	}
	defer func() { coverage.SourceMetadata = attachment.NewTraceSourceMetadata(fields) }()
	coverage, err = tdb.inspectCoverage(ctx, "metadata", "device_info",
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
		if coverage.RowsRead > 1 {
			for i := range fields {
				fields[i].State, fields[i].Detail = "invalid", "device_info is not a singleton"
			}
		}
		coverage.Skipped = fmt.Sprintf("expected exactly one device_info row, got %d", coverage.RowsRead)
		return coverage, nil
	}
	var widthRaw, heightRaw, frameRateRaw any
	if err := tdb.db.QueryRowContext(ctx,
		`SELECT CASE WHEN typeof(physical_width) IN ('integer','real','null') THEN physical_width END,
		CASE WHEN typeof(physical_height) IN ('integer','real','null') THEN physical_height END,
		CASE WHEN typeof(physical_frame_rate) IN ('integer','real','null') THEN physical_frame_rate END,
		typeof(physical_width),typeof(physical_height),typeof(physical_frame_rate) FROM device_info`,
	).Scan(&widthRaw, &heightRaw, &frameRateRaw, &fields[0].Detail, &fields[1].Detail, &fields[2].Detail); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	coverage.Metadata = map[string]string{}
	invalid := 0
	for index, field := range []struct {
		name string
		raw  any
	}{
		{name: "physical_width", raw: widthRaw},
		{name: "physical_height", raw: heightRaw},
		{name: "physical_frame_rate", raw: frameRateRaw},
	} {
		sqlType := fields[index].Detail
		fields[index].Detail = ""
		value, ok := traceDBStrictSQLiteInt(field.raw)
		if !ok {
			if sqlType == "null" {
				traceDBAddCoverageMetric(&coverage, field.name+"_unavailable", 1)
			} else {
				fields[index].State, fields[index].Detail = "invalid", "expected SQLite INTEGER, got "+sqlType
				if sqlType == "real" {
					fields[index].Value = fmt.Sprint(field.raw)
				}
				invalid++
				traceDBAddCoverageMetric(&coverage, field.name+"_invalid", 1)
			}
			continue
		}
		if value <= 0 || value > math.MaxInt32 {
			fields[index].State, fields[index].Value, fields[index].Detail = "invalid", strconv.FormatInt(value, 10), "outside positive int32 metadata range"
			invalid++
			traceDBAddCoverageMetric(&coverage, field.name+"_invalid", 1)
			continue
		}
		coverage.Metadata[field.name] = strconv.FormatInt(value, 10)
		fields[index].State, fields[index].Value = "observed", strconv.FormatInt(value, 10)
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

func inspectTraceDBParserMetadata(ctx context.Context, tdb *traceDB) (coverage TraceDBCoverage, err error) {
	var fields []attachment.TraceSourceMetadataField
	defer func() { coverage.SourceMetadata = attachment.NewTraceSourceMetadata(fields) }()
	coverage, err = tdb.inspectCoverage(ctx, "metadata", "meta", []string{"name", "value"})
	coverage.Role = "diagnostic_metadata"
	coverage.FieldSources = map[string]string{
		"schema":     "official trace_streamer meta(name,value) parser metadata",
		"bounds":     "at most 64 rows, 128 UTF-8 bytes per name, 1024 UTF-8 bytes per value, and 16 KiB surfaced key/value bytes",
		"duplicates": "duplicate names are omitted monotonically even when their values are equal",
		"effect":     "exact bounded text is display diagnostics only; arbitrary keys never become source-admission or hard-gate authority and no row becomes ftrace",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		fields = append(fields, attachment.TraceSourceMetadataField{Origin: "meta", Kind: "parser", Name: "table", State: "missing"})
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
			END,
			CASE WHEN length(CAST(name AS BLOB)) > 128 OR length(CAST(value AS BLOB)) > 1024 THEN 1 ELSE 0 END
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
		var oversized int
		if err := rows.Scan(&nameRaw, &valueRaw, &oversized); err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		name, nameOK := nameRaw.(string)
		value, valueOK := valueRaw.(string)
		if !nameOK || !valueOK || !validTraceDBParserMetadataName(name) || !utf8.ValidString(value) {
			state := "invalid"
			if oversized != 0 {
				state = "truncated"
			}
			label := "row"
			if nameOK && validTraceDBParserMetadataName(name) {
				label = name
				invalidNames[name] = true
				delete(candidates, name)
			}
			fields = append(fields, attachment.TraceSourceMetadataField{Origin: "meta", Kind: "parser", Name: label, State: state, Detail: "parser row not surfaced; no identity inferred"})
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
		fields = append(fields, attachment.TraceSourceMetadataField{Origin: "meta", Kind: "parser", Name: "rows", State: "truncated", Detail: "64-row diagnostic budget"})
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
			fields = append(fields, attachment.TraceSourceMetadataField{Origin: "meta", Kind: "parser", Name: name, State: "truncated", Detail: "16 KiB diagnostic budget; no partial value presented"})
			reasons["metadata_rows_not_surfaced_by_byte_cap"]++
			continue
		}
		coverage.Metadata[name] = value
		fields = append(fields, attachment.TraceSourceMetadataField{Origin: "meta", Kind: "parser", Name: name, State: "observed", Value: value})
		totalBytes += nextBytes
	}
	coverage.RowsEmitted = len(coverage.Metadata)
	var poisoned []string
	for name := range invalidNames {
		poisoned = append(poisoned, name)
	}
	sort.Strings(poisoned)
	for _, name := range poisoned {
		alreadyDiagnosed := false
		for _, field := range fields {
			alreadyDiagnosed = alreadyDiagnosed || field.Name == name
		}
		if alreadyDiagnosed {
			continue
		}
		fields = append(fields, attachment.TraceSourceMetadataField{Origin: "meta", Kind: "parser", Name: name, State: "invalid", Detail: "duplicate or invalid name/value; value omitted"})
	}
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
