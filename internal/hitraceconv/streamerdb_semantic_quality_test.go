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
				"source_rows_suppressed_pre_pairing":           13,
				"async_source_rows_suppressed_post_pairing":    6,
				"source_rows_official_async_shaped":            9,
				"source_rows_withheld_official_async_interval": 8,
				"source_rows_emitted_official_async_interval":  7,
				"source_rows_emitted_official_async_raw_pair":  2,
				"source_rows_rejected_official_async_shape":    1,
				"source_rows_with_distributed_metadata":        10,
				"source_rows_suppressed_cpu_unavailable":       9,
				"source_rows_preserved_cpu_unavailable":        3,
				"source_rows_admitted_exact_name_pre_pairing":  12,
				"source_rows_suppressed_identity":              4,
				"sync_spans_suppressed":                        5,
				"sync_spans_suppressed_by_local_fence":         5,
				"sync_endpoints_emitted":                       14,
				"official_viewer_standard_sync_spans_emitted":  4,
			},
		},
		{
			Family: "source_rawtrace_marker_sync",
			Table:  "__raw_marker_sync__",
			Metrics: map[string]int64{
				"raw_pairs_unique_cpu_unavailable_callstack_candidate": 4,
			},
		},
	}
	quality := traceDBSemanticQualityCoverage(items)
	if quality.Family != traceDBSemanticQualityFamily || quality.Table != traceDBSemanticQualityTable ||
		quality.Role != "semantic_quality_summary" || !quality.Found {
		t.Fatalf("quality identity drifted: %+v", quality)
	}
	for key, want := range map[string]int64{
		"unnamed_threads":                                             7,
		"unresolved_thread_names":                                     2,
		"thread_names_recovered_main_process":                         2,
		"thread_names_recovered_unique_public_tid":                    3,
		"public_tids_with_multiple_itids":                             3,
		"public_tids_with_multiple_owner_ipids":                       2,
		"scheduler_boundaries_with_unknown_comm":                      11,
		"callstack_source_rows_suppressed_pre_pairing":                13,
		"callstack_async_source_rows_suppressed_post_pairing":         6,
		"callstack_source_rows_official_async_shaped":                 9,
		"callstack_source_rows_withheld_official_async_interval":      8,
		"callstack_source_rows_emitted_official_async_interval":       7,
		"callstack_source_rows_emitted_official_async_raw_pair":       2,
		"callstack_source_rows_rejected_official_async_shape":         1,
		"callstack_source_rows_with_distributed_metadata":             10,
		"callstack_source_rows_suppressed_cpu_unavailable":            9,
		"callstack_source_rows_preserved_cpu_unavailable":             3,
		"callstack_source_rows_admitted_exact_name_pre_pairing":       12,
		"callstack_source_rows_suppressed_identity":                   4,
		"callstack_sync_spans_suppressed":                             5,
		"callstack_sync_spans_suppressed_by_local_fence":              5,
		"callstack_sync_endpoints_emitted":                            14,
		"callstack_official_viewer_standard_sync_spans_emitted":       4,
		"raw_marker_pairs_unique_cpu_unavailable_callstack_candidate": 4,
		"official_viewer_standard_spans_emitted":                      6,
		"codrax_typed_only_sync_spans_emitted":                        3,
		"codrax_typed_only_async_intervals_emitted":                   7,
		"codrax_typed_only_spans_emitted":                             10,
		"callstack_closed_sync_spans_unpublished":                     5,
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
		"callstack_source_rows_withheld_official_async_interval=8",
		"callstack CPU placement is unavailable for 3 source row(s)",
		"span identity and duration were preserved",
		"no CPU/core attribution",
		"preserved 7 completed async interval(s) as Codrax versioned comment rows",
		"official SmartPerf/generic systrace viewer does not display them",
		"no synthetic S/F endpoint was emitted",
		"official-viewer span visibility is degraded_typed_only_and_unpublished",
		"standard_visible=6 codrax_typed_only=10 unpublished_closed_sync=5",
		"typed-only preservation is not counted as official-viewer conversion success",
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

func TestTraceDBSemanticQualityClosesCPUUnavailableRawReplacement(t *testing.T) {
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{
		{
			Family: "slice",
			Table:  "callstack",
			Metrics: map[string]int64{
				"source_rows_preserved_cpu_unavailable": 7,
				"sync_spans_suppressed":                 2,
			},
		},
		{
			Family: "source_rawtrace_marker_sync",
			Table:  "__raw_marker_sync__",
			Metrics: map[string]int64{
				"raw_pairs_cpu_unavailable_callstack_replaced": 2,
				"sync_spans_submitted":                         2,
				"sync_endpoints_emitted":                       4,
			},
		},
	})
	if quality.Metadata["raw_marker_replacement_closure"] != "complete" ||
		quality.Metrics["raw_marker_cpu_unavailable_replacement_spans"] != 2 ||
		quality.Metrics["callstack_sync_spans_recovered_by_raw_marker"] != 2 ||
		quality.Metrics["callstack_source_rows_cpu_unavailable_after_raw_replacement"] != 5 {
		t.Fatalf("CPU-unavailable replacement closure drifted: %+v", quality)
	}
	caveats := strings.Join(traceDBSemanticQualityCaveats(
		[]TraceDBCoverage{quality}), "\n")
	for _, want := range []string{
		"published 2 exact replacement span(s)",
		"including 2 unique CPU-unavailable and 0 unique standard-name-unrepresentable callstack span(s)",
		"CPU placement is unavailable for 5 source row(s)",
	} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("CPU replacement caveat missing %q:\n%s", want, caveats)
		}
	}
	if strings.Contains(caveats, "CPU placement is unavailable for 7") {
		t.Fatalf("pre-replacement unavailable count leaked:\n%s", caveats)
	}
}

func TestTraceDBSemanticQualityClosesNameUnrepresentableRawReplacement(t *testing.T) {
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{
		{
			Family: "slice",
			Table:  "callstack",
			Metrics: map[string]int64{
				"sync_spans_suppressed": 1,
			},
		},
		{
			Family: "source_rawtrace_marker_sync",
			Table:  "__raw_marker_sync__",
			Metrics: map[string]int64{
				"raw_pairs_name_unrepresentable_callstack_replaced": 1,
				"sync_spans_submitted":                              1,
				"sync_endpoints_emitted":                            2,
			},
		},
	})
	if quality.Metadata["raw_marker_replacement_closure"] != "complete" ||
		quality.Metrics["raw_marker_name_unrepresentable_replacement_spans"] != 1 ||
		quality.Metrics["callstack_sync_spans_recovered_by_raw_marker"] != 1 ||
		quality.Metrics["callstack_sync_spans_unrecovered_after_raw_marker"] != 0 {
		t.Fatalf("name-unrepresentable replacement closure drifted: %+v", quality)
	}
	caveats := strings.Join(traceDBSemanticQualityCaveats(
		[]TraceDBCoverage{quality}), "\n")
	for _, want := range []string{
		"published 1 exact replacement span(s)",
		"0 unique CPU-unavailable and 1 unique standard-name-unrepresentable",
		"0 callstack sync span(s) remain unpublished",
	} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("name replacement caveat missing %q:\n%s", want, caveats)
		}
	}
}

func TestTraceDBSemanticQualityDoesNotCloseWithheldRawMarkerRecovery(t *testing.T) {
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{
		{
			Family: "slice",
			Table:  "callstack",
			Metrics: map[string]int64{
				"sync_spans_suppressed": 7,
			},
		},
		{
			Family: "source_rawtrace_marker_sync",
			Table:  "__raw_marker_sync__",
			Metadata: map[string]string{
				"publication_state": "withheld_raw_decode_incomplete",
			},
		},
	})
	if got := quality.Metadata["raw_marker_replacement_closure"]; got != "not_evaluated_withheld_raw_decode_incomplete" {
		t.Fatalf("withheld raw marker recovery was presented as closed: %+v", quality)
	}
	if quality.Metrics["callstack_sync_spans_recovered_by_raw_marker"] != 0 {
		t.Fatalf("withheld recovery minted replacement evidence: %+v", quality)
	}
}

func TestTraceDBSemanticQualityRejectsTypedOnlyReasonCensusMismatch(t *testing.T) {
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{{
		Family: "slice",
		Table:  "callstack",
		Metrics: map[string]int64{
			"sync_endpoints_emitted": 2,
		},
		Metadata: map[string]string{
			"official_viewer_typed_only_sync_reason_census": "complete",
		},
	}})
	if quality.Metrics["codrax_typed_only_sync_spans_emitted"] != 1 ||
		quality.Metadata["official_viewer_span_visibility"] !=
			"not_evaluated_typed_only_reason_census_mismatch" {
		t.Fatalf("typed-only reason census mismatch was not fail-closed: %+v", quality)
	}
}

func TestTraceDBSemanticQualityClosesRawReplacementAgainstLocalCallstackFence(t *testing.T) {
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{
		{
			Family: "slice",
			Table:  "callstack",
			Metrics: map[string]int64{
				"sync_spans_suppressed":                5,
				"sync_spans_suppressed_by_local_fence": 5,
			},
		},
		{
			Family: "source_rawtrace_marker_sync",
			Table:  "__raw_marker_sync__",
			Metrics: map[string]int64{
				"raw_pairs_existing_db_candidate_locally_suppressed": 1,
				"raw_pairs_interval_collision_locally_suppressed":    2,
				"sync_spans_submitted":                               3,
				"sync_endpoints_emitted":                             6,
			},
		},
	})
	if quality.Metadata["raw_marker_replacement_closure"] != "complete" ||
		quality.Metrics["raw_marker_db_suppressed_candidate_spans"] != 3 ||
		quality.Metrics["callstack_sync_spans_recovered_by_raw_marker"] != 3 ||
		quality.Metrics["callstack_sync_spans_unrecovered_after_raw_marker"] != 2 ||
		quality.Metrics["callstack_local_fence_spans_unrecovered_after_raw_marker"] != 2 {
		t.Fatalf("raw replacement closure drifted: %+v", quality)
	}
	caveats := strings.Join(traceDBSemanticQualityCaveats(
		[]TraceDBCoverage{quality}), "\n")
	for _, want := range []string{
		"callstack_sync_spans_unrecovered_after_raw_marker=2",
		"published 3 exact replacement span(s)",
		"2 callstack sync span(s) remain unpublished",
		"including 2 locally fenced span(s)",
	} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("replacement caveat missing %q:\n%s", want, caveats)
		}
	}
	if strings.Contains(caveats, "callstack_sync_spans_suppressed=5") {
		t.Fatalf("gross local suppression was presented as net loss:\n%s", caveats)
	}
}

func TestTraceDBSemanticQualityDoesNotGuessReplacementWhenRawPublicationIncomplete(t *testing.T) {
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{
		{
			Family: "slice",
			Table:  "callstack",
			Metrics: map[string]int64{
				"sync_spans_suppressed":                5,
				"sync_spans_suppressed_by_local_fence": 5,
			},
		},
		{
			Family: "source_rawtrace_marker_sync",
			Table:  "__raw_marker_sync__",
			Metrics: map[string]int64{
				"raw_pairs_interval_collision_locally_suppressed": 3,
				"sync_spans_submitted":                            3,
				"sync_spans_suppressed":                           1,
				"sync_endpoints_emitted":                          4,
			},
		},
	})
	if quality.Metadata["raw_marker_replacement_closure"] !=
		"not_evaluated_raw_span_suppressed" ||
		quality.Metrics["callstack_sync_spans_recovered_by_raw_marker"] != 0 {
		t.Fatalf("incomplete raw publication acquired replacement authority: %+v", quality)
	}
	caveats := strings.Join(traceDBSemanticQualityCaveats(
		[]TraceDBCoverage{quality}), "\n")
	if !strings.Contains(caveats, "callstack_sync_spans_suppressed=5") ||
		strings.Contains(caveats, "exact replacement span") {
		t.Fatalf("incomplete closure caveat drifted:\n%s", caveats)
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
