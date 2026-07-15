package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	traceCaptureCompletenessClean    = "parser_self_audit_clean"
	traceCaptureCompletenessDegraded = "parser_self_audit_degraded"
	traceCaptureCompletenessUnknown  = "unknown"

	maxTraceDBCaptureStatRows             = 4096
	maxTraceDBCaptureIssueRows            = 32
	maxTraceDBCaptureEventNameBytes       = 256
	maxTraceDBCaptureEnumBytes            = 32
	maxTraceDBCaptureSourceBytes          = 64
	traceDBCaptureAllStatTypes      uint8 = (1 << 5) - 1
)

// inspectTraceDBCaptureCompleteness consumes trace_streamer's official stat
// table as a diagnostic side channel. A malformed or partial stat table makes
// completeness unknown; it never suppresses positively exported trace rows.
func inspectTraceDBCaptureCompleteness(ctx context.Context, queryer traceDBQueryer) (item TraceDBCoverage, err error) {
	start := time.Now()
	defer func() { traceDBSetCoverageElapsed(&item, start) }()
	item = TraceDBCoverage{
		Family: "capture_completeness",
		Table:  "stat",
		Role:   traceDBCoverageRole("capture_completeness", "stat"),
		FieldSources: map[string]string{
			"schema": "OpenHarmony trace_streamer stat(event_name,stat_type,count,serverity,source)",
			"count":  "strict SQLite INTEGER in 0..UINT32_MAX; fixed stat-type totals use checked addition",
			"effect": "qualifies absence-based conclusions only; positive trace evidence remains admitted",
		},
		CaptureCompleteness: &TraceCaptureCompleteness{State: traceCaptureCompletenessUnknown},
	}
	var one int
	err = queryer.QueryRowContext(ctx, `SELECT 1 FROM sqlite_master WHERE type='table' AND name='stat' COLLATE NOCASE LIMIT 1`).Scan(&one)
	if err == sql.ErrNoRows {
		traceDBSetCaptureCompletenessUnknown(&item, "missing_table")
		return item, nil
	}
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	item.Found = true

	columns, err := traceDBColumnNames(ctx, queryer, "stat")
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	columnSet := make(map[string]bool, len(columns))
	for _, column := range columns {
		columnSet[sqliteASCIIIdentifierFold(column)] = true
	}
	for _, column := range []string{"event_name", "stat_type", "count", "serverity", "source"} {
		if columnSet[sqliteASCIIIdentifierFold(column)] {
			item.ColumnsPresent = append(item.ColumnsPresent, column)
		} else {
			item.ColumnsMissing = append(item.ColumnsMissing, column)
		}
	}
	sort.Strings(item.ColumnsPresent)
	sort.Strings(item.ColumnsMissing)
	if len(item.ColumnsMissing) > 0 {
		traceDBSetCaptureCompletenessUnknown(&item, "missing_columns")
		return item, nil
	}

	rows, err := queryer.QueryContext(ctx, fmt.Sprintf(`
		SELECT
			typeof(event_name), CASE WHEN typeof(event_name)='text' AND length(CAST(event_name AS BLOB)) BETWEEN 1 AND %d THEN event_name END,
			typeof(stat_type), CASE WHEN typeof(stat_type)='text' AND length(CAST(stat_type AS BLOB)) BETWEEN 1 AND %d THEN stat_type END,
			typeof(count), CASE WHEN typeof(count)='integer' THEN count END,
			typeof(serverity), CASE WHEN typeof(serverity)='text' AND length(CAST(serverity AS BLOB)) BETWEEN 1 AND %d THEN serverity END,
			typeof(source), CASE WHEN typeof(source)='text' AND length(CAST(source AS BLOB)) BETWEEN 1 AND %d THEN source END
		FROM stat
		LIMIT %d`, maxTraceDBCaptureEventNameBytes, maxTraceDBCaptureEnumBytes,
		maxTraceDBCaptureEnumBytes, maxTraceDBCaptureSourceBytes, maxTraceDBCaptureStatRows+1))
	if err != nil {
		item.Error = err.Error()
		return item, err
	}
	defer rows.Close()

	totals := TraceCaptureCompleteness{State: traceCaptureCompletenessClean}
	seen := make(map[string]struct{})
	statMasks := make(map[string]uint8)
	var issues []TraceCaptureCompletenessIssue
	var integrity []string
	for rows.Next() {
		item.RowsRead++
		if item.RowsRead > maxTraceDBCaptureStatRows {
			integrity = appendCaptureIntegrityIssue(integrity, "row_limit_exceeded")
			break
		}
		var eventTypeRaw, eventRaw, statTypeRaw, statRaw, countTypeRaw, countRaw any
		var severityTypeRaw, severityRaw, sourceTypeRaw, sourceRaw any
		if scanErr := rows.Scan(&eventTypeRaw, &eventRaw, &statTypeRaw, &statRaw, &countTypeRaw, &countRaw,
			&severityTypeRaw, &severityRaw, &sourceTypeRaw, &sourceRaw); scanErr != nil {
			item.Error = scanErr.Error()
			return item, scanErr
		}
		eventName, eventOK := traceDBCaptureText(eventTypeRaw, eventRaw)
		statType, statOK := traceDBCaptureText(statTypeRaw, statRaw)
		severity, severityOK := traceDBCaptureText(severityTypeRaw, severityRaw)
		source, sourceOK := traceDBCaptureText(sourceTypeRaw, sourceRaw)
		count, countOK := traceDBStrictSQLiteInt(traceDBBoundedSQLiteIntegerTransport(countTypeRaw, countRaw))
		statBit, knownStat := traceDBCaptureStatTypeBit(statType)
		if !eventOK || !statOK || !severityOK || !sourceOK || !countOK || count < 0 || count > math.MaxUint32 || !knownStat ||
			!traceDBCaptureSeverityKnown(severity) || source != "trace" {
			integrity = appendCaptureIntegrityIssue(integrity, "malformed_row")
			continue
		}
		key := eventName + "\x00" + statType
		if _, duplicate := seen[key]; duplicate {
			integrity = appendCaptureIntegrityIssue(integrity, "duplicate_event_stat")
			continue
		}
		seen[key] = struct{}{}
		statMasks[eventName] |= statBit
		totals.RowsAccepted++
		value := uint64(count)
		if !traceDBCaptureAddStatTotal(&totals, statType, value) {
			integrity = appendCaptureIntegrityIssue(integrity, "aggregate_overflow")
			continue
		}
		if statType == "received" || value == 0 {
			continue
		}
		totals.NonzeroIssueRows++
		if !traceDBCaptureAddSeverityTotal(&totals, severity, value) {
			integrity = appendCaptureIntegrityIssue(integrity, "aggregate_overflow")
			continue
		}
		issues = append(issues, TraceCaptureCompletenessIssue{
			EventName: eventName, StatType: statType, Count: value, Source: source, Severity: severity,
		})
	}
	if err := rows.Err(); err != nil {
		item.Error = err.Error()
		return item, err
	}
	if item.RowsRead == 0 {
		integrity = appendCaptureIntegrityIssue(integrity, "empty_table")
	}
	for _, mask := range statMasks {
		if mask != traceDBCaptureAllStatTypes {
			integrity = appendCaptureIntegrityIssue(integrity, "incomplete_event_status_set")
			break
		}
	}
	if containsCaptureIntegrityIssue(integrity, "row_limit_exceeded") {
		// The LIMIT sentinel proves the table is outside the audited profile,
		// while SQL row order is not an authority. Do not publish a
		// prefix-dependent mixture of other reasons from an incomplete scan.
		integrity = []string{"row_limit_exceeded"}
	}
	if len(integrity) > 0 {
		traceDBSetCaptureCompletenessUnknown(&item, integrity...)
		return item, nil
	}

	sort.Slice(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		if traceDBCaptureSeverityRank(left.Severity) != traceDBCaptureSeverityRank(right.Severity) {
			return traceDBCaptureSeverityRank(left.Severity) > traceDBCaptureSeverityRank(right.Severity)
		}
		if left.Count != right.Count {
			return left.Count > right.Count
		}
		if left.EventName != right.EventName {
			return left.EventName < right.EventName
		}
		return left.StatType < right.StatType
	})
	if len(issues) > maxTraceDBCaptureIssueRows {
		totals.IssuesCompacted = len(issues) - maxTraceDBCaptureIssueRows
		issues = issues[:maxTraceDBCaptureIssueRows]
	}
	totals.Issues = issues
	if totals.NonzeroIssueRows > 0 {
		totals.State = traceCaptureCompletenessDegraded
	}
	item.CaptureCompleteness = &totals
	return item, nil
}

func traceDBCaptureStatTypeBit(statType string) (uint8, bool) {
	switch statType {
	case "received":
		return 1 << 0, true
	case "data_lost":
		return 1 << 1, true
	case "not_match":
		return 1 << 2, true
	case "not_supported":
		return 1 << 3, true
	case "invalid_data":
		return 1 << 4, true
	default:
		return 0, false
	}
}

func traceDBCaptureSeverityKnown(severity string) bool {
	switch severity {
	case "info", "warn", "error", "fatal":
		return true
	default:
		return false
	}
}

func traceDBCaptureText(typeRaw, valueRaw any) (string, bool) {
	typeName, typeOK := typeRaw.(string)
	value, valueOK := valueRaw.(string)
	if !typeOK || typeName != "text" || !valueOK || !utf8.ValidString(value) ||
		value == "" || value != strings.TrimSpace(value) || !traceDBSinglePhysicalLine(value, false) {
		return "", false
	}
	return value, true
}

func traceDBCaptureAddStatTotal(totals *TraceCaptureCompleteness, statType string, value uint64) bool {
	switch statType {
	case "received":
		return checkedAddUint64(&totals.Received, value)
	case "data_lost":
		return checkedAddUint64(&totals.DataLost, value)
	case "not_match":
		return checkedAddUint64(&totals.NotMatch, value)
	case "not_supported":
		return checkedAddUint64(&totals.NotSupported, value)
	case "invalid_data":
		return checkedAddUint64(&totals.InvalidData, value)
	default:
		return false
	}
}

func traceDBCaptureAddSeverityTotal(totals *TraceCaptureCompleteness, severity string, value uint64) bool {
	switch severity {
	case "info":
		return checkedAddUint64(&totals.InfoIssues, value)
	case "warn":
		return checkedAddUint64(&totals.WarnIssues, value)
	case "error":
		return checkedAddUint64(&totals.ErrorIssues, value)
	case "fatal":
		return checkedAddUint64(&totals.FatalIssues, value)
	default:
		return false
	}
}

func checkedAddUint64(target *uint64, value uint64) bool {
	if target == nil || value > math.MaxUint64-*target {
		return false
	}
	*target += value
	return true
}

func traceDBCaptureSeverityRank(severity string) int {
	switch severity {
	case "fatal":
		return 4
	case "error":
		return 3
	case "warn":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func traceDBSetCaptureCompletenessUnknown(item *TraceDBCoverage, reasons ...string) {
	if item == nil {
		return
	}
	issues := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		issues = appendCaptureIntegrityIssue(issues, reason)
	}
	sort.Strings(issues)
	item.CaptureCompleteness = &TraceCaptureCompleteness{
		State: traceCaptureCompletenessUnknown, IntegrityIssues: issues,
	}
}

func appendCaptureIntegrityIssue(issues []string, issue string) []string {
	for _, existing := range issues {
		if existing == issue {
			return issues
		}
	}
	return append(issues, issue)
}

func containsCaptureIntegrityIssue(issues []string, want string) bool {
	for _, issue := range issues {
		if issue == want {
			return true
		}
	}
	return false
}
