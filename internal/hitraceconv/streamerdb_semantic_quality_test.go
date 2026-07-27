package hitraceconv

import (
	"context"
	"strings"
	"testing"
)

func TestTraceDBSemanticQualityCoverageAndCaveats(t *testing.T) {
	items := []TraceDBCoverage{
		{
			Family: "resolver",
			Table:  "thread",
			Metrics: map[string]int64{
				"unnamed_threads":                          7,
				"unresolved_thread_names":                  2,
				"thread_names_recovered_main_process":      2,
				"thread_names_recovered_unique_public_tid": 3,
				"public_tids_with_multiple_itids":          3,
				"public_tids_with_multiple_owner_ipids":    2,
			},
		},
		{
			Family:  "scheduler",
			Table:   "sched_slice",
			Metrics: map[string]int64{"boundaries_with_unknown_comm": 11},
		},
		{
			Family: "slice",
			Table:  "callstack",
			Metrics: map[string]int64{
				"source_rows_suppressed_pre_pairing":        13,
				"async_source_rows_suppressed_post_pairing": 6,
				"source_rows_suppressed_cpu_unavailable":    9,
				"source_rows_preserved_cpu_unavailable":     3,
				"source_rows_suppressed_identity":           4,
				"sync_spans_suppressed":                     5,
			},
		},
	}
	quality := traceDBSemanticQualityCoverage(items)
	if quality.Family != traceDBSemanticQualityFamily || quality.Table != traceDBSemanticQualityTable ||
		quality.Role != "semantic_quality_summary" || !quality.Found {
		t.Fatalf("quality identity drifted: %+v", quality)
	}
	for key, want := range map[string]int64{
		"unnamed_threads":                                     7,
		"unresolved_thread_names":                             2,
		"thread_names_recovered_main_process":                 2,
		"thread_names_recovered_unique_public_tid":            3,
		"public_tids_with_multiple_itids":                     3,
		"public_tids_with_multiple_owner_ipids":               2,
		"scheduler_boundaries_with_unknown_comm":              11,
		"callstack_source_rows_suppressed_pre_pairing":        13,
		"callstack_async_source_rows_suppressed_post_pairing": 6,
		"callstack_source_rows_suppressed_cpu_unavailable":    9,
		"callstack_source_rows_preserved_cpu_unavailable":     3,
		"callstack_source_rows_suppressed_identity":           4,
		"callstack_sync_spans_suppressed":                     5,
	} {
		if got := quality.Metrics[key]; got != want {
			t.Fatalf("quality metric %s=%d want %d: %+v", key, got, want, quality)
		}
	}
	caveats := strings.Join(traceDBSemanticQualityCaveats([]TraceDBCoverage{quality}), "\n")
	for _, want := range []string{
		"semantic quality is degraded",
		"unresolved_thread_names=2",
		"callstack_source_rows_suppressed_cpu_unavailable=9",
		"callstack CPU placement is unavailable for 3 source row(s)",
		"span identity and duration were preserved",
		"no CPU/core attribution",
		"query-ready but name/span completeness is not proven",
		"identity audit observed",
		"public_tids_with_multiple_owner_ipids=2",
		"host/namespace PID splits",
	} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("quality caveat missing %q:\n%s", want, caveats)
		}
	}
}

func TestTraceDBThreadQualityMetricsExposeUnnamedAndMultiOwnerPublicTID(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'host')",
		"INSERT INTO process VALUES (2, 200, 'namespace')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, '', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 101, 2, 'marker-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 102, 1, NULL, 0, 0, 1)",
		"INSERT INTO thread VALUES (4, 200, 2, '', 0, 1, 1)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	_, coverage, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 3 {
		t.Fatalf("identity coverage=%d want 3: %+v", len(coverage), coverage)
	}
	metrics := coverage[2].Metrics
	for key, want := range map[string]int64{
		"thread_rows_scanned":                      4,
		"thread_rows_accepted":                     4,
		"unnamed_threads":                          3,
		"unresolved_thread_names":                  1,
		"thread_names_recovered_main_process":      1,
		"thread_names_recovered_unique_public_tid": 1,
		"public_tids_with_multiple_itids":          1,
		"public_tids_with_multiple_owner_ipids":    1,
	} {
		if got := metrics[key]; got != want {
			t.Fatalf("thread metric %s=%d want %d: %+v", key, got, want, metrics)
		}
	}
}

func TestTraceDBDisplayNameRecoveryIsDisplayOnlyAndFailsClosedOnAmbiguity(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.Processes = map[int64]traceDBProcess{
		1: {IPID: 1, PID: 100, Name: "host"},
		2: {IPID: 2, PID: 200, Name: "namespace"},
		3: {IPID: 3, PID: 300, Name: "main-process"},
	}
	index.ByITID = map[int64]traceDBThread{
		1: {ITID: 1, TID: 101, IPID: 1},
		2: {ITID: 2, TID: 101, IPID: 2, Name: "marker-thread"},
		3: {ITID: 3, TID: 300, IPID: 3, IsMainThread: true},
		4: {ITID: 4, TID: 401, IPID: 1},
		5: {ITID: 5, TID: 401, IPID: 1, Name: "left"},
		6: {ITID: 6, TID: 401, IPID: 2, Name: "right"},
	}
	buildTraceDBThreadSecondaryIndexes(&index)
	if name, source := traceDBThreadDisplayName(index, index.ByITID[1]); name != "marker-thread" ||
		source != traceDBDisplayNameUniquePublicTID {
		t.Fatalf("unique public-TID display recovery failed: name=%q source=%q", name, source)
	}
	if name, source := traceDBThreadDisplayName(index, index.ByITID[3]); name != "main-process" ||
		source != traceDBDisplayNameMainProcess {
		t.Fatalf("main process display recovery failed: name=%q source=%q", name, source)
	}
	if name, source := traceDBThreadDisplayName(index, index.ByITID[4]); name != "" || source != "" {
		t.Fatalf("ambiguous public-TID names acquired display authority: name=%q source=%q", name, source)
	}
	if index.ByITID[1].Name != "" || index.ByITID[3].Name != "" || index.ByITID[4].Name != "" {
		t.Fatalf("display recovery mutated canonical thread metadata: %+v", index.ByITID)
	}
}
