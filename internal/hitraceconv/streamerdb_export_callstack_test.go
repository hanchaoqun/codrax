package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func createTraceDBCallstackFixture(t *testing.T, statements []string) string {
	t.Helper()
	statements = append(append([]string(nil), statements...),
		"CREATE TABLE instant (ts, name, ref, wakeup_from, ref_type)",
		"CREATE TABLE sched_slice (ts, dur, cpu, itid, end_state, priority)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
	)
	return createTraceDBFixture(t, statements)
}

func TestExportTraceDBCallstackStrictRoundTripAndEndpointCPUs(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker-a', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 102, 1, 'worker-b', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 900000, 1100000, 0, 'Running')",
		"INSERT INTO thread_state VALUES (1, 2000000, 150000, 2, 'Running')",
		"INSERT INTO thread_state VALUES (1, 2250000, 100000, 3, 'Running')",
		"INSERT INTO thread_state VALUES (2, 900000, 1100000, 4, 'Running')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000000, 500000, 1, 'outer', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (2, 1000000, 100000, 1, 'shared-start-inner', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (3, 1200000, 300000, 1, 'shared-end-inner', 'I', NULL, NULL)",
		"INSERT INTO callstack VALUES (4, 1500000, 0, 1, 'zero', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (5, 2100000, 200000, 1, 'migrated', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (6, 1200000, 0, 1, 'async', 'S', 9, '9')",
		"INSERT INTO callstack VALUES (7, 1250000, 0, 2, 'async', 'C', 9, '9')",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-strict.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export strict callstack: %v", err)
	}
	assertCoverageEmitted(t, result.Coverage, "slice", "callstack", 12)
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"[000] ....     0.001000: tracing_mark_write: B|100|outer",
		"[000] ....     0.001000: tracing_mark_write: B|100|shared-start-inner",
		"[000] ....     0.001100: tracing_mark_write: E|100|",
		"[000] ....     0.001200: tracing_mark_write: S|100|async|9",
		"[004] ....     0.001250: tracing_mark_write: F|100|async|9",
		"[002] ....     0.002100: tracing_mark_write: B|100|migrated",
		"[003] ....     0.002300: tracing_mark_write: E|100|",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("strict callstack output missing %q:\n%s", want, body)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("round-trip strict callstack: %v", err)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 0, TimeEnd: 0.01})
	wantDurations := map[string]float64{
		"outer": 0.5, "shared-start-inner": 0.1, "shared-end-inner": 0.3,
		"migrated": 0.2, "async": 0.05,
	}
	for name, want := range wantDurations {
		found := false
		for _, span := range stats.TraceSpans {
			if span.Name == name && math.Abs(span.DurationMs-want) < 0.0000001 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("round-trip missing span %s=%fms: %+v", name, want, stats.TraceSpans)
		}
	}
}

func TestExportTraceDBCallstackCanonicalNullFlagIsEmptySyncFlag(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"INSERT INTO process VALUES (2, 200, 'control')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 201, 2, 'control-worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 900, 401, 2, 'Running')",
		"INSERT INTO thread_state VALUES (2, 900, 401, 3, 'Running')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000, 100, 1, 'producer-null-flag', NULL, NULL, NULL)",
		"INSERT INTO callstack VALUES (2, 1200, 100, 2, 'non-text-flag', x'37', NULL, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-null-flag.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export canonical NULL callstack flag: %v", err)
	}
	var coverage TraceDBCoverage
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			coverage = item
			break
		}
	}
	if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "invalid_flag=1") {
		t.Fatalf("canonical NULL flag or strict non-TEXT rejection drifted: %+v", coverage)
	}
	if !strings.Contains(coverage.FieldSources["flag"], "SQLite NULL is the canonical empty sync flag") {
		t.Fatalf("NULL flag producer contract missing from provenance: %+v", coverage.FieldSources)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if !strings.Contains(body, "B|100|producer-null-flag") || strings.Contains(body, "non-text-flag") {
		t.Fatalf("canonical NULL flag was not emitted or non-TEXT flag leaked:\n%s", body)
	}
}

func TestExportTraceDBCallstackRejectsDivergentCurrentAliasProfile(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id INT, ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (31, 7, 500, 'demo')",
		"INSERT INTO process VALUES (8, 8, 600, 'control')",
		"CREATE TABLE thread (id INT, itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (41, 9, 501, 7, 'ambiguous-worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 10, 601, 8, 'control-worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (9, 900000, 500000, 6, 'Running')",
		"INSERT INTO thread_state VALUES (10, 1150000, 200000, 7, 'Running')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000000, 100000, 9, 'ambiguous-current-alias', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (2, 1200000, 100000, 10, 'valid-current-alias', '', NULL, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-current-alias.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export current alias callstack: %v", err)
	}
	assertCoverageEmitted(t, result.Coverage, "slice", "callstack", 2)
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "ambiguous-current-alias") || !strings.Contains(body, "B|600|valid-current-alias") {
		t.Fatalf("divergent current aliases were not fail-closed independently of a valid sibling:\n%s", body)
	}
}

func TestExportTraceDBCallstackRejectsBothNamesOfDivergentThreadAlias(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid, name)",
		"INSERT INTO process VALUES (7, 7, 500, 'demo')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (41, 9, 501, 7, 'ambiguous-worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (10, 10, 502, 7, 'control-worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (9, 900000, 700000, 6, 'Running')",
		"INSERT INTO thread_state VALUES (10, 1350000, 200000, 7, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, callid, name, flag, cookie, chainId)",
		"INSERT INTO callstack VALUES (1, 1000000, 100000, 41, 'by-source-alias', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (2, 1200000, 100000, 9, 'by-canonical-alias', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (3, 1400000, 100000, 10, 'valid-sibling', '', NULL, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-divergent-thread-alias.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export divergent thread alias: %v", err)
	}
	var coverage TraceDBCoverage
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			coverage = item
			break
		}
	}
	if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "ambiguous_callid_thread=2") ||
		strings.Contains(coverage.Skipped, "unresolved_callid_thread=") {
		t.Fatalf("divergent thread aliases were not independently rejected: %+v", coverage)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if strings.Contains(body, "by-source-alias") || strings.Contains(body, "by-canonical-alias") || !strings.Contains(body, "valid-sibling") {
		t.Fatalf("divergent thread alias leaked or valid sibling was lost:\n%s", body)
	}
}

func TestExportTraceDBCallstackDualIdentityMustConverge(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid, name)",
		"INSERT INTO process VALUES (1, 1, 100, 'demo')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 2, 102, 1, 'worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 3, 103, 1, 'other', 0, 0, 1)",
		"INSERT INTO thread VALUES (4, 4, 104, 1, 'valid-worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (2, 900000, 650000, 4, 'Running')",
		"INSERT INTO thread_state VALUES (3, 900000, 1600000, 5, 'Running')",
		"INSERT INTO thread_state VALUES (4, 900000, 650000, 6, 'Running')",
		"CREATE TABLE callstack (ID, ts, dur, CALLID, name, flag, cookie, chainId, ITID)",
		"INSERT INTO callstack VALUES (1, 1000000, 100000, 4, 'both-converge', '', NULL, NULL, 4)",
		"INSERT INTO callstack VALUES (2, 1200000, 100000, NULL, 'explicit-only', '', NULL, NULL, 4)",
		"INSERT INTO callstack VALUES (3, 1400000, 100000, 4, 'callid-fallback', '', NULL, NULL, NULL)",
		"INSERT INTO callstack VALUES (4, 1600000, 100000, 3, 'identity-mismatch', '', NULL, NULL, 2)",
		"INSERT INTO callstack VALUES (5, 1800000, 100000, 2, 'text-itid', '', NULL, NULL, CAST(2 AS TEXT))",
		"INSERT INTO callstack VALUES (6, 2000000, 100000, CAST(2 AS TEXT), 'text-callid', '', NULL, NULL, 2)",
		"INSERT INTO callstack VALUES (7, 2200000, 100000, 99, 'unresolved-callid', '', NULL, NULL, 2)",
		"INSERT INTO callstack VALUES (8, 2400000, 0, NULL, 'missing-both', '', NULL, NULL, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-dual-identity.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export dual-identity callstack: %v", err)
	}
	var coverage TraceDBCoverage
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			coverage = item
			break
		}
	}
	if coverage.RowsEmitted != 6 {
		t.Fatalf("dual-identity accepted wrong endpoint count: %+v", coverage)
	}
	for _, want := range []string{
		"emitter_identity_mismatch=1", "invalid_emitter_itid=1", "invalid_callid=1",
		"unresolved_callid_thread=1", "missing_emitter_identity=1",
	} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("dual-identity coverage missing %q: %+v", want, coverage)
		}
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{"both-converge", "explicit-only", "callid-fallback"} {
		if !strings.Contains(body, want) {
			t.Fatalf("valid dual-identity row %q missing:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"identity-mismatch", "text-itid", "text-callid", "unresolved-callid", "missing-both"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("invalid dual-identity row %q leaked:\n%s", forbidden, body)
		}
	}
}

func TestExportTraceDBCallstackFailClosesMalformedRowsAndBadLane(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"INSERT INTO process VALUES (2, 200, 'control')",
		"INSERT INTO process VALUES (3, 300, 'unrelated')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'bad-lane', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 201, 2, 'good-lane', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 301, 3, 'unrelated-lane', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
		"INSERT INTO thread_state VALUES (3, 0, 5000, 3, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, callid, name, flag, cookie, chainId)",
		"INSERT INTO callstack VALUES (1, 1000, 1000, 1, 'cross-a', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (2, 1500, 1000, 1, 'cross-b', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (3, 1000, 100, 2, 'good', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (4, NULL, 1, 2, 'null-ts', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (5, 1200, 1.5, 2, 'real-dur', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (6, 1300, 1, 2, 'pipe|I42', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (7, 1400, 0, 2, 'async-no-id', 'S', NULL, NULL)",
		"INSERT INTO callstack VALUES (8, 1500, 0, 2, 'unknown-flag', 's', 1, NULL)",
		"INSERT INTO callstack VALUES (9, 1600, 0, 2, 'conflict', 'S', 1, '2')",
		"INSERT INTO callstack VALUES (10, 1700, 0, 2, 'orphan', 'C', 3, NULL)",
		"INSERT INTO callstack VALUES (11, 1800, 100, 3, 'unrelated-good', '', NULL, NULL)",
		"INSERT INTO callstack VALUES (12, 2000, 0, 3, 'async-control', 'S', 9, NULL)",
		"INSERT INTO callstack VALUES (13, 2100, 0, 3, 'async-control', 'C', 9, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-fail-close.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export malformed callstack: %v", err)
	}
	var callstackCoverage TraceDBCoverage
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			callstackCoverage = item
			break
		}
	}
	for _, want := range []string{
		"invalid_timestamp=1", "invalid_duration=1",
		"invalid_name=1", "missing_async_identity=1", "unknown_flag=1",
		"cookie_chain_id_conflict=1", "async_family_fail_closed=2",
	} {
		if !strings.Contains(callstackCoverage.Skipped, want) {
			t.Fatalf("callstack coverage missing %q: %+v", want, callstackCoverage)
		}
	}
	foundSyncAudit := false
	for _, item := range result.Coverage {
		if item.Family == "integrity" && item.Table == "sync_span_authority" {
			foundSyncAudit = strings.Contains(item.Skipped, "crossing_lanes=1") &&
				strings.Contains(item.Skipped, "suppressed_spans=3")
		}
	}
	if !foundSyncAudit {
		t.Fatalf("central sync authority did not disclose the rejected crossing lane: %+v", result.Coverage)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if !strings.Contains(body, "B|300|unrelated-good") {
		t.Fatalf("valid emitter lane was incorrectly suppressed:\n%s", body)
	}
	if !strings.Contains(body, "B|200|control") {
		t.Fatalf("rejected callstack evidence erased independent registration producer:\n%s", body)
	}
	if strings.Contains(body, "B|200|good") {
		t.Fatalf("malformed sibling rescued its sync lane:\n%s", body)
	}
	for _, forbidden := range []string{"cross-a", "cross-b", "pipe|I42", "async-no-id", "unknown-flag", "conflict", "orphan", "async-control"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("malformed callstack row %q leaked into output:\n%s", forbidden, body)
		}
	}
}

func TestExportTraceDBCallstackUsesRowIDWhenSourceIDDuplicates(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 102, 1, 'legacy', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 0, 5000, 6, 'Running')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, cat TEXT, depth INT, cookie INT, itid INT)",
		"INSERT INTO callstack VALUES (7, 1000, 100, NULL, 'legacy-a', 'slice', 0, NULL, 2)",
		"INSERT INTO callstack VALUES (7, 1200, 100, NULL, 'legacy-b', 'slice', 0, NULL, 2)",
		"INSERT INTO callstack VALUES (NULL, 1400, 100, NULL, 'legacy-null-id', 'slice', 0, NULL, 2)",
		"INSERT INTO callstack VALUES (1.5, 1600, 100, NULL, 'legacy-real-id', 'slice', 0, NULL, 2)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-duplicate-id.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export duplicate-id callstack: %v", err)
	}
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			if item.RowsEmitted != 8 || strings.Contains(item.Skipped, "duplicate_row_id") || strings.Contains(item.Skipped, "invalid_source_id") {
				t.Fatalf("duplicate source id affected SQLite rowid identity: %+v", item)
			}
			body, readErr := os.ReadFile(outPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !strings.Contains(string(body), "B|100|legacy-a") || !strings.Contains(string(body), "B|100|legacy-b") ||
				!strings.Contains(string(body), "B|100|legacy-null-id") || !strings.Contains(string(body), "B|100|legacy-real-id") {
				t.Fatalf("rowid-distinct rows were lost:\n%s", body)
			}
			return
		}
	}
	t.Fatal("callstack coverage missing")
}

func TestExportTraceDBCallstackLegacyITIDDoesNotRequireCallID(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (2, 102, 1, 'legacy', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 0, 5000000, 6, 'Running')",
		"CREATE TABLE callstack (ts INT, dur INT, callid, name TEXT, cat TEXT, depth INT, cookie INT, itid INT)",
		"INSERT INTO callstack VALUES (1000000, 100000, NULL, 'legacy', 'slice', 0, NULL, 2)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-legacy-itid.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export legacy-itid callstack: %v", err)
	}
	assertCoverageEmitted(t, result.Coverage, "slice", "callstack", 2)
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bodyBytes), "[006] ....     0.001000: tracing_mark_write: B|100|legacy") {
		t.Fatalf("legacy itid was not used as emitter identity:\n%s", bodyBytes)
	}
}

func TestExportTraceDBCallstackMalformedAsyncEndpointPoisonsFamily(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 900000, 200000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (1, 2900000, 200000, 2, 'Running')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000000, 0, 1, 'async', 'S', 9, NULL)",
		"INSERT INTO callstack VALUES (2, 2000000, 0, 1, 'async', 'c', NULL, NULL)",
		"INSERT INTO callstack VALUES (3, 3000000, 0, 1, 'async', 'C', 9, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-async-poison.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export poisoned async callstack: %v", err)
	}
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			if item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "unknown_flag=1") ||
				!strings.Contains(item.Skipped, "async_family_fail_closed=2") {
				t.Fatalf("malformed async hole allowed cross-pairing: %+v", item)
			}
			return
		}
	}
	t.Fatal("callstack coverage missing")
}

func TestAuditTraceDBSyncSpanLaneDepthAndZeroIdentity(t *testing.T) {
	candidate := func(id, start, end, depth int64, depthKnown bool) traceDBSyncSpanCandidate {
		depthProvenance := traceDBSyncSpanDepthUnknown
		if depthKnown {
			depthProvenance = traceDBSyncSpanDepthCallstack
		}
		return traceDBSyncSpanCandidate{
			Producer: traceDBSyncSpanProducerCallstack, StableKind: traceDBSyncSpanStableCallstackRowID, StableID: id,
			HeaderTID: 1, HeaderTGID: 1, CanonicalITID: 1, CanonicalITIDKnown: true,
			Start: start, End: end, Depth: depth, DepthKnown: depthKnown, DepthProvenance: depthProvenance,
		}
	}
	if reason := auditTraceDBSyncSpanLaneForTest(t, []traceDBSyncSpanCandidate{
		candidate(1, 10, 20, 0, true), candidate(2, 10, 20, 1, true),
	}); reason != traceDBSyncSpanLaneClean {
		t.Fatalf("strict depth should disambiguate identical intervals: %d", reason)
	}
	if reason := auditTraceDBSyncSpanLaneForTest(t, []traceDBSyncSpanCandidate{
		candidate(1, 10, 20, 0, true), candidate(2, 11, 19, 0, true),
	}); reason != traceDBSyncSpanLaneDepthConflict {
		t.Fatalf("equal-depth overlap was not rejected: %d", reason)
	}
	if reason := auditTraceDBSyncSpanLaneForTest(t, []traceDBSyncSpanCandidate{
		candidate(1, 10, 10, 0, false), candidate(2, 10, 10, 0, false),
	}); reason != traceDBSyncSpanLaneClean {
		t.Fatalf("zero spans should not fabricate an active nesting conflict: %d", reason)
	}
}

func auditTraceDBSyncSpanLaneForTest(t *testing.T, candidates []traceDBSyncSpanCandidate) traceDBSyncSpanLaneAuditReason {
	t.Helper()
	stage := &traceDBSyncSpanStage{options: normalizedTraceDBSyncSpanStageOptions(traceDBSyncSpanStageOptions{})}
	auditor := traceDBSyncSpanLaneAuditor{stage: stage}
	auditor.reset()
	for _, candidate := range candidates {
		if err := auditor.consume(candidate); err != nil {
			t.Fatalf("audit typed sync span lane: %v", err)
		}
	}
	return auditor.finalReason()
}

func TestExportTraceDBCallstackAsyncDoesNotPairAcrossProcessIncarnations(t *testing.T) {
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'old')",
		"INSERT INTO process VALUES (2, 100, 'new')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'old-worker', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 102, 2, 'new-worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, 0, 5000000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 5000000, 2, 'Running')",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, 1000000, 0, 1, 'async', 'S', 9, NULL)",
		"INSERT INTO callstack VALUES (2, 1100000, 0, 2, 'async', 'C', 9, NULL)",
	})
	outPath := filepath.Join(t.TempDir(), "callstack-pid-reuse.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export pid-reuse callstack: %v", err)
	}
	for _, item := range result.Coverage {
		if item.Family == "slice" && item.Table == "callstack" {
			if item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "unpaired_async_start=1") ||
				!strings.Contains(item.Skipped, "unpaired_async_finish=1") {
				t.Fatalf("async endpoints paired across process incarnations: %+v", item)
			}
			return
		}
	}
	t.Fatal("callstack coverage missing")
}
