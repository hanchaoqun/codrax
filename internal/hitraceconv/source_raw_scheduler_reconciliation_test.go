package hitraceconv

import (
	"os"
	"strings"
	"testing"
)

func TestTraceDBSchedulerPublicationReconciliationClosesLargCCounts(t *testing.T) {
	coverage := traceDBSchedulerPublicationReconciliationCoverage(
		[]TraceDBCoverage{
			{
				Family:      "scheduler",
				Table:       "sched_slice",
				Role:        "query_ready_export",
				Found:       true,
				RowsRead:    790734,
				RowsEmitted: 219784,
				Metrics: map[string]int64{
					"db_source_rows_suppressed": 570944,
				},
			},
			{
				Family: "source_rawtrace_scheduler_lite_join",
				Table:  "__raw_vs_db_sched_switch__",
				Role:   "query_ready_enrichment",
				Found:  true,
				Metrics: map[string]int64{
					"raw_records_retained":                                     790734,
					"db_boundaries_enriched":                                   219784,
					"raw_unmatched_published":                                  570854,
					"raw_unique_records_unmatched":                             96,
					"raw_unmatched_withheld_db_coordinate_present":             219784,
					"raw_unmatched_withheld_prev_rejected_or_lifecycle_absent": 57,
					"raw_unmatched_withheld_next_rejected_or_lifecycle_absent": 39,
				},
			},
		})
	if coverage.Metadata["reconciliation_state"] !=
		"complete_exact_raw_record_closure" ||
		coverage.Metrics["standard_sched_switch_events"] != 790638 ||
		coverage.Metrics["raw_unmatched_typed_withheld"] != 96 ||
		coverage.Metrics["raw_records_accounted"] != 790734 ||
		coverage.Metrics["raw_records_unclassified_residual"] != 0 ||
		coverage.Metrics["db_source_rows_suppressed"] != 570944 ||
		!strings.Contains(coverage.Metadata["publication_contract"],
			"standard_total=790638") {
		t.Fatalf("LARG-C scheduler publication did not close: %+v", coverage)
	}
}

func TestTraceDBSchedulerPublicationReconciliationDisclosesResidual(t *testing.T) {
	coverage := traceDBSchedulerPublicationReconciliationCoverage(
		[]TraceDBCoverage{
			{
				Family: "scheduler", Table: "sched_slice",
				Role: "query_ready_export", Found: true, RowsEmitted: 1,
			},
			{
				Family: "source_rawtrace_scheduler_lite_join",
				Table:  "__raw_vs_db_sched_switch__",
				Role:   "query_ready_enrichment",
				Found:  true,
				Metrics: map[string]int64{
					"raw_records_retained":   2,
					"db_boundaries_enriched": 1,
				},
			},
		})
	if coverage.Metadata["reconciliation_state"] !=
		"complete_with_unclassified_raw_residual" ||
		coverage.Metrics["raw_records_unclassified_residual"] != 1 {
		t.Fatalf("unclassified scheduler record was hidden: %+v", coverage)
	}
}

func TestTraceDBSchedulerPublicationReconciliationFeedsUserCaveat(t *testing.T) {
	summary := traceDBSchedulerPublicationReconciliationCoverage(
		[]TraceDBCoverage{
			{
				Family: "scheduler", Table: "sched_slice",
				Role: "query_ready_export", Found: true,
				RowsRead: 10, RowsEmitted: 3,
				Metrics: map[string]int64{
					"db_source_rows_suppressed": 6,
				},
			},
			{
				Family: "source_rawtrace_scheduler_lite_join",
				Table:  "__raw_vs_db_sched_switch__",
				Role:   "query_ready_enrichment",
				Found:  true,
				Metrics: map[string]int64{
					"raw_records_retained":                                   5,
					"db_boundaries_enriched":                                 3,
					"raw_unmatched_published":                                1,
					"raw_unique_records_unmatched":                           1,
					"raw_unmatched_withheld_next_absent_or_lifecycle_absent": 1,
				},
			},
		})
	quality := traceDBSemanticQualityCoverage([]TraceDBCoverage{summary})
	caveats := strings.Join(
		traceDBSemanticQualityCaveats([]TraceDBCoverage{quality}), "\n")
	for _, want := range []string{
		"scheduler publication reconciled",
		"db_boundaries=3",
		"raw_unmatched_published=1",
		"standard_sched_switch_total=4",
		"raw_typed_withheld=1",
		"db_source_rows_suppressed=6",
		"not final systrace loss",
	} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("scheduler reconciliation caveat missing %q:\n%s",
				want, caveats)
		}
	}
}

func TestTraceDBSchedulerPublicationReconciliationProductionWiring(t *testing.T) {
	body, err := os.ReadFile("streamerdb_export.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	build := strings.Index(text,
		"traceDBSchedulerPublicationReconciliationCoverage(coverage)")
	appendSummary := strings.Index(text,
		"coverage = append(coverage, schedulerPublicationCoverage)")
	buildQuality := strings.Index(text,
		"traceDBSemanticQualityCoverage(coverage)")
	if build < 0 || appendSummary < build || buildQuality < appendSummary {
		t.Fatalf("scheduler reconciliation is not wired before semantic quality: build=%d append=%d quality=%d",
			build, appendSummary, buildQuality)
	}
}
