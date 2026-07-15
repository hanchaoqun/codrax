package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

const traceDBCaptureStatDDL = `CREATE TABLE stat (
	event_name TEXT,
	stat_type TEXT,
	count INTEGER,
	serverity TEXT,
	source TEXT
)`

const traceDBCaptureStatWeakDDL = `CREATE TABLE stat (
	event_name,
	stat_type,
	count,
	serverity,
	source
)`

func TestInspectTraceDBCaptureCompletenessMissingAndMalformedSchemaAreUnknown(t *testing.T) {
	tests := []struct {
		name       string
		statements []string
		wantReason string
		wantFound  bool
	}{
		{name: "missing table", statements: []string{"CREATE TABLE placeholder (id INTEGER)"}, wantReason: "missing_table"},
		{
			name: "correctly spelled severity is not the official column",
			statements: []string{`CREATE TABLE stat (
				event_name TEXT, stat_type TEXT, count INTEGER, severity TEXT, source TEXT
			)`},
			wantReason: "missing_columns", wantFound: true,
		},
		{name: "empty table", statements: []string{traceDBCaptureStatDDL}, wantReason: "empty_table", wantFound: true},
		{name: "missing event_name", statements: []string{`CREATE TABLE stat (stat_type, count, serverity, source)`}, wantReason: "missing_columns", wantFound: true},
		{name: "missing stat_type", statements: []string{`CREATE TABLE stat (event_name, count, serverity, source)`}, wantReason: "missing_columns", wantFound: true},
		{name: "missing count", statements: []string{`CREATE TABLE stat (event_name, stat_type, serverity, source)`}, wantReason: "missing_columns", wantFound: true},
		{name: "missing source", statements: []string{`CREATE TABLE stat (event_name, stat_type, count, serverity)`}, wantReason: "missing_columns", wantFound: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coverage := inspectTraceDBCaptureFixture(t, test.statements...)
			assertTraceDBCaptureState(t, coverage, traceCaptureCompletenessUnknown)
			if coverage.Found != test.wantFound || coverage.Skipped != "" {
				t.Fatalf("coverage authority/skipped mismatch: %+v", coverage)
			}
			if !containsExact(coverage.CaptureCompleteness.IntegrityIssues, test.wantReason) {
				t.Fatalf("missing integrity reason %q: %+v", test.wantReason, coverage)
			}
		})
	}
}

func TestInspectTraceDBCaptureCompletenessCleanAndDegradedCohorts(t *testing.T) {
	clean := inspectTraceDBCaptureFixture(t,
		`CREATE TABLE STAT (EXTRA TEXT, SOURCE TEXT, SERVERITY TEXT, COUNT INTEGER, STAT_TYPE TEXT, EVENT_NAME TEXT)`,
		traceDBCaptureInsert("sched_switch", "received", "42", "info"),
		traceDBCaptureInsert("sched_switch", "data_lost", "0", "warn"),
		traceDBCaptureInsert("sched_switch", "not_match", "0", "error"),
		traceDBCaptureInsert("sched_switch", "not_supported", "0", "fatal"),
		traceDBCaptureInsert("sched_switch", "invalid_data", "0", "info"),
	)
	assertTraceDBCaptureState(t, clean, traceCaptureCompletenessClean)
	if clean.RowsRead != 5 || clean.Skipped != "" || clean.Error != "" ||
		clean.CaptureCompleteness.RowsAccepted != 5 || clean.CaptureCompleteness.Received != 42 ||
		clean.CaptureCompleteness.NonzeroIssueRows != 0 || len(clean.CaptureCompleteness.Issues) != 0 {
		t.Fatalf("clean parser self-audit mismatch: %+v", clean)
	}

	degraded := inspectTraceDBCaptureFixture(t,
		traceDBCaptureStatDDL,
		traceDBCaptureInsert("future_vendor_event", "received", "10", "info"),
		traceDBCaptureInsert("future_vendor_event", "data_lost", "2", "error"),
		traceDBCaptureInsert("future_vendor_event", "not_match", "3", "warn"),
		traceDBCaptureInsert("future_vendor_event", "not_supported", "4", "info"),
		traceDBCaptureInsert("future_vendor_event", "invalid_data", "5", "fatal"),
	)
	assertTraceDBCaptureState(t, degraded, traceCaptureCompletenessDegraded)
	got := degraded.CaptureCompleteness
	if degraded.RowsRead != 5 || degraded.Skipped != "" || got.RowsAccepted != 5 || got.Received != 10 ||
		got.DataLost != 2 || got.NotMatch != 3 || got.NotSupported != 4 || got.InvalidData != 5 ||
		got.InfoIssues != 4 || got.WarnIssues != 3 || got.ErrorIssues != 2 || got.FatalIssues != 5 ||
		got.NonzeroIssueRows != 4 || len(got.Issues) != 4 {
		t.Fatalf("degraded parser self-audit mismatch: %+v", degraded)
	}
	wantOrder := []string{"invalid_data", "data_lost", "not_match", "not_supported"}
	for i, want := range wantOrder {
		if got.Issues[i].StatType != want {
			t.Fatalf("issue sort[%d]=%+v, want stat_type=%s; all=%+v", i, got.Issues[i], want, got.Issues)
		}
	}
}

func TestInspectTraceDBCaptureCompletenessRejectsMalformedRowsAtomically(t *testing.T) {
	tests := []struct {
		name       string
		eventExpr  string
		statExpr   string
		countExpr  string
		severity   string
		sourceExpr string
	}{
		{name: "negative count", countExpr: "-1"},
		{name: "count above uint32", countExpr: "4294967296"},
		{name: "real count", countExpr: "1.5"},
		{name: "text count", countExpr: "'not-an-integer'"},
		{name: "blob count", countExpr: "x'31'"},
		{name: "null count", countExpr: "NULL"},
		{name: "null event", eventExpr: "NULL"},
		{name: "integer event", eventExpr: "1"},
		{name: "real event", eventExpr: "1.5"},
		{name: "blob event", eventExpr: "x'31'"},
		{name: "invalid utf8 event", eventExpr: "CAST(x'80' AS TEXT)"},
		{name: "blank event", eventExpr: "''"},
		{name: "trimmed event", eventExpr: "' event'"},
		{name: "control event", eventExpr: "'bad'||char(10)||'event'"},
		{name: "overlong event", eventExpr: "printf('%0257d', 0)"},
		{name: "unknown stat", statExpr: "'future_status'"},
		{name: "case drifted stat", statExpr: "'DATA_LOST'"},
		{name: "integer stat", statExpr: "1"},
		{name: "real stat", statExpr: "1.5"},
		{name: "blob stat", statExpr: "x'31'"},
		{name: "null stat", statExpr: "NULL"},
		{name: "overlong stat", statExpr: "printf('%033d', 0)"},
		{name: "unknown severity", severity: "'notice'"},
		{name: "case drifted severity", severity: "'WARN'"},
		{name: "integer severity", severity: "1"},
		{name: "real severity", severity: "1.5"},
		{name: "blob severity", severity: "x'31'"},
		{name: "null severity", severity: "NULL"},
		{name: "overlong severity", severity: "printf('%033d', 0)"},
		{name: "foreign source", sourceExpr: "'parser'"},
		{name: "integer source", sourceExpr: "1"},
		{name: "real source", sourceExpr: "1.5"},
		{name: "blob source", sourceExpr: "x'31'"},
		{name: "null source", sourceExpr: "NULL"},
		{name: "overlong source", sourceExpr: "printf('%065d', 0)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eventExpr := firstTraceDBCaptureTestValue(test.eventExpr, "'event'")
			statExpr := firstTraceDBCaptureTestValue(test.statExpr, "'data_lost'")
			countExpr := firstTraceDBCaptureTestValue(test.countExpr, "1")
			severityExpr := firstTraceDBCaptureTestValue(test.severity, "'warn'")
			sourceExpr := firstTraceDBCaptureTestValue(test.sourceExpr, "'trace'")
			coverage := inspectTraceDBCaptureFixture(t,
				traceDBCaptureStatWeakDDL,
				traceDBCaptureInsert("event", "received", "1", "info"),
				fmt.Sprintf("INSERT INTO stat (event_name,stat_type,count,serverity,source) VALUES (%s,%s,%s,%s,%s)", eventExpr, statExpr, countExpr, severityExpr, sourceExpr),
				traceDBCaptureInsert("event", "not_match", "0", "warn"),
				traceDBCaptureInsert("event", "not_supported", "0", "warn"),
				traceDBCaptureInsert("event", "invalid_data", "0", "warn"),
			)
			assertTraceDBCaptureUnknownAtomic(t, coverage, "malformed_row")
		})
	}
}

func TestInspectTraceDBCaptureCompletenessAcceptsUint32CountBoundary(t *testing.T) {
	coverage := inspectTraceDBCaptureFixture(t,
		traceDBCaptureStatDDL,
		traceDBCaptureInsert("event", "received", "4294967295", "info"),
		traceDBCaptureInsert("event", "data_lost", "0", "warn"),
		traceDBCaptureInsert("event", "not_match", "0", "warn"),
		traceDBCaptureInsert("event", "not_supported", "0", "warn"),
		traceDBCaptureInsert("event", "invalid_data", "0", "warn"),
	)
	assertTraceDBCaptureState(t, coverage, traceCaptureCompletenessClean)
	if coverage.CaptureCompleteness.Received != math.MaxUint32 {
		t.Fatalf("uint32 boundary was not preserved exactly: %+v", coverage)
	}
}

func TestInspectTraceDBCaptureCompletenessRejectsDuplicateAndSparseCohorts(t *testing.T) {
	base := []string{
		traceDBCaptureStatDDL,
		traceDBCaptureInsert("event", "received", "1", "info"),
		traceDBCaptureInsert("event", "data_lost", "0", "warn"),
		traceDBCaptureInsert("event", "not_match", "0", "warn"),
		traceDBCaptureInsert("event", "not_supported", "0", "warn"),
		traceDBCaptureInsert("event", "invalid_data", "0", "warn"),
	}
	for _, test := range []struct {
		name       string
		statements []string
		wantReason string
	}{
		{name: "identical duplicate", statements: append(append([]string(nil), base...), traceDBCaptureInsert("event", "data_lost", "0", "warn")), wantReason: "duplicate_event_stat"},
		{name: "conflicting duplicate", statements: append(append([]string(nil), base...), traceDBCaptureInsert("event", "data_lost", "9", "fatal")), wantReason: "duplicate_event_stat"},
		{name: "missing status", statements: append([]string(nil), base[:len(base)-1]...), wantReason: "incomplete_event_status_set"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertTraceDBCaptureUnknownAtomic(t, inspectTraceDBCaptureFixture(t, test.statements...), test.wantReason)
		})
	}
}

func TestInspectTraceDBCaptureCompletenessUnknownDoesNotRescuePartialPrefix(t *testing.T) {
	coverage := inspectTraceDBCaptureFixture(t,
		traceDBCaptureStatDDL,
		traceDBCaptureInsert("good", "received", "7", "info"),
		traceDBCaptureInsert("good", "data_lost", "9", "fatal"),
		traceDBCaptureInsert("good", "not_match", "0", "warn"),
		traceDBCaptureInsert("good", "not_supported", "0", "warn"),
		traceDBCaptureInsert("good", "invalid_data", "0", "warn"),
		`INSERT INTO stat VALUES ('bad','received','not-an-integer','info','trace')`,
	)
	assertTraceDBCaptureUnknownAtomic(t, coverage, "malformed_row")
}

func TestInspectTraceDBCaptureCompletenessRowLimitAndIssueCompaction(t *testing.T) {
	clean4095 := inspectTraceDBCaptureFixture(t, traceDBCaptureStatDDL, traceDBCaptureGeneratedCohortsSQL(819, 0))
	assertTraceDBCaptureState(t, clean4095, traceCaptureCompletenessClean)
	if clean4095.RowsRead != 4095 || clean4095.CaptureCompleteness.RowsAccepted != 4095 {
		t.Fatalf("4095-row boundary mismatch: %+v", clean4095)
	}

	incomplete4096 := inspectTraceDBCaptureFixture(t,
		traceDBCaptureStatDDL,
		traceDBCaptureGeneratedCohortsSQL(819, 0),
		traceDBCaptureInsert("partial", "received", "1", "info"),
	)
	if incomplete4096.RowsRead != 4096 {
		t.Fatalf("4096-row fixture mismatch: %+v", incomplete4096)
	}
	assertTraceDBCaptureUnknownAtomic(t, incomplete4096, "incomplete_event_status_set")

	overLimit := inspectTraceDBCaptureFixture(t, traceDBCaptureStatDDL, traceDBCaptureGeneratedCohortsSQL(820, 0))
	if overLimit.RowsRead != maxTraceDBCaptureStatRows+1 {
		t.Fatalf("row-limit sentinel was not read exactly once: %+v", overLimit)
	}
	assertTraceDBCaptureUnknownAtomic(t, overLimit, "row_limit_exceeded")
	if !reflect.DeepEqual(overLimit.CaptureCompleteness.IntegrityIssues, []string{"row_limit_exceeded"}) {
		t.Fatalf("over-limit reason must not depend on an unordered SQL prefix: %+v", overLimit.CaptureCompleteness)
	}

	issues := inspectTraceDBCaptureFixture(t, traceDBCaptureStatDDL, traceDBCaptureGeneratedCohortsSQL(33, 1))
	assertTraceDBCaptureState(t, issues, traceCaptureCompletenessDegraded)
	if issues.CaptureCompleteness.NonzeroIssueRows != 33 || len(issues.CaptureCompleteness.Issues) != 32 ||
		issues.CaptureCompleteness.IssuesCompacted != 1 || issues.CaptureCompleteness.Issues[0].EventName != "event_0000" ||
		issues.CaptureCompleteness.Issues[31].EventName != "event_0031" {
		t.Fatalf("issue compaction/order mismatch: %+v", issues.CaptureCompleteness)
	}
}

func TestInspectTraceDBCaptureCompletenessAuthorityCancellationFailsHard(t *testing.T) {
	path := createTraceDBFixture(t, []string{traceDBCaptureStatDDL})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	coverage, err := inspectTraceDBCaptureCompleteness(ctx, tdb.db)
	if err == nil || coverage.Error == "" {
		t.Fatalf("canceled DB authority query failed open: coverage=%+v err=%v", coverage, err)
	}
}

func TestCheckedAddUint64RejectsOverflowWithoutMutation(t *testing.T) {
	value := uint64(math.MaxUint64)
	if checkedAddUint64(&value, 1) || value != math.MaxUint64 {
		t.Fatalf("checked add overflow mutated target: %d", value)
	}
	if checkedAddUint64(nil, 1) {
		t.Fatal("checked add must reject nil target")
	}
}

func TestCaptureCompletenessCollectorIsIndependentOfTraceRowSink(t *testing.T) {
	coverage := inspectTraceDBCaptureFixture(t,
		traceDBCaptureStatDDL,
		traceDBCaptureInsert("sched_switch", "received", "1", "info"),
		traceDBCaptureInsert("sched_switch", "data_lost", "1", "error"),
		traceDBCaptureInsert("sched_switch", "not_match", "0", "warn"),
		traceDBCaptureInsert("sched_switch", "not_supported", "0", "warn"),
		traceDBCaptureInsert("sched_switch", "invalid_data", "0", "warn"),
	)
	assertTraceDBCaptureState(t, coverage, traceCaptureCompletenessDegraded)
	if coverage.RowsEmitted != 0 || coverage.Role != "capture_completeness" ||
		coverage.FieldSources["effect"] != "qualifies absence-based conclusions only; positive trace evidence remains admitted" {
		t.Fatalf("capture side-channel escaped into the trace row lane: %+v", coverage)
	}
}

func inspectTraceDBCaptureFixture(t *testing.T, statements ...string) TraceDBCoverage {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatalf("open trace DB fixture: %v", err)
	}
	defer tdb.close()
	coverage, err := inspectTraceDBCaptureCompleteness(context.Background(), tdb.db)
	if err != nil {
		t.Fatalf("inspect capture completeness: %v; coverage=%+v", err, coverage)
	}
	return coverage
}

func traceDBCaptureInsert(eventName, statType, count, severity string) string {
	return fmt.Sprintf("INSERT INTO stat (event_name,stat_type,count,serverity,source) VALUES ('%s','%s',%s,'%s','trace')", eventName, statType, count, severity)
}

func traceDBCaptureGeneratedCohortsSQL(events, dataLost int) string {
	return fmt.Sprintf(`WITH RECURSIVE
		event_ids(n) AS (SELECT 0 UNION ALL SELECT n+1 FROM event_ids WHERE n<%d),
		stat_types(stat_type) AS (VALUES ('received'),('data_lost'),('not_match'),('not_supported'),('invalid_data'))
		INSERT INTO stat
		SELECT printf('event_%%04d', n), stat_type,
			CASE WHEN stat_type='received' THEN 1 WHEN stat_type='data_lost' THEN %d ELSE 0 END,
			CASE WHEN stat_type='data_lost' THEN 'warn' ELSE 'info' END,
			'trace'
		FROM event_ids CROSS JOIN stat_types`, events-1, dataLost)
}

func firstTraceDBCaptureTestValue(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func assertTraceDBCaptureState(t *testing.T, coverage TraceDBCoverage, want string) {
	t.Helper()
	if coverage.CaptureCompleteness == nil || coverage.CaptureCompleteness.State != want {
		t.Fatalf("capture state=%+v, want %q; coverage=%+v", coverage.CaptureCompleteness, want, coverage)
	}
}

func assertTraceDBCaptureUnknownAtomic(t *testing.T, coverage TraceDBCoverage, wantReason string) {
	t.Helper()
	assertTraceDBCaptureState(t, coverage, traceCaptureCompletenessUnknown)
	got := coverage.CaptureCompleteness
	if !containsExact(got.IntegrityIssues, wantReason) {
		t.Fatalf("unknown state missing reason %q: %+v", wantReason, coverage)
	}
	want := &TraceCaptureCompleteness{State: traceCaptureCompletenessUnknown, IntegrityIssues: got.IntegrityIssues}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown state rescued a partial prefix: got=%+v want=%+v", got, want)
	}
	if coverage.Skipped != "" {
		t.Fatalf("capture diagnostic must not inflate skipped exporter count: %+v", coverage)
	}
	if strings.TrimSpace(coverage.Error) != "" {
		t.Fatalf("data-integrity unknown must not masquerade as a DB authority error: %+v", coverage)
	}
}
