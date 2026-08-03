package hitraceconv

import (
	"fmt"
	"math"
)

const (
	traceDBSchedulerPublicationFamily = "scheduler_publication_reconciliation"
	traceDBSchedulerPublicationTable  = "__sched_switch_publication__"
)

// traceDBSchedulerPublicationReconciliationCoverage distinguishes a failed-
// closed DB lane from final text loss. Raw enrichment of a DB boundary is not
// counted twice; only independently published raw-unmatched rows increase the
// final standard sched_switch event total.
func traceDBSchedulerPublicationReconciliationCoverage(
	items []TraceDBCoverage,
) TraceDBCoverage {
	out := TraceDBCoverage{
		Family: traceDBSchedulerPublicationFamily,
		Table:  traceDBSchedulerPublicationTable,
		Role:   "semantic_quality_summary",
		FieldSources: map[string]string{
			"db":             "scheduler/sched_slice strict complete-per-CPU publication coverage",
			"raw":            "source_rawtrace_scheduler_lite_join exact enriched and lifecycle-proven raw-unmatched counters",
			"total":          "final standard sched_switch events = DB boundaries published + raw unmatched published; raw enrichment changes fields but is not another event",
			"withheld":       "source join raw_unique_records_unmatched exact residual after key ambiguity, DB enrichment and raw-unmatched publication; raw_unmatched_withheld_db_coordinate_present is observational overlap with DB enrichment and is never counted again",
			"effect":         "diagnostic and user-facing reconciliation only; never publishes, suppresses, deduplicates or repairs an event",
			"time_alignment": "source raw/DB exact timestamp+CPU overlap observation copied from the join; absence is advisory and is not proof of a clock-domain offset",
		},
		Metadata: map[string]string{
			"reconciliation_state": "unavailable",
		},
	}
	db, dbOK := traceDBCoverageByIdentity(
		items, "scheduler", "sched_slice", "query_ready_export")
	join, joinOK := traceDBCoverageByIdentity(
		items, "source_rawtrace_scheduler_lite_join",
		"__raw_vs_db_sched_switch__", "query_ready_enrichment")
	if !dbOK || !joinOK {
		out.Skipped = "scheduler publication reconciliation unavailable: DB scheduler or raw join coverage absent"
		return out
	}
	out.Found = db.Found || join.Found
	traceDBAddCoverageMetric(
		&out, "db_source_rows_read", int64(db.RowsRead))
	traceDBAddCoverageMetric(
		&out, "db_source_rows_suppressed",
		db.Metrics["db_source_rows_suppressed"])
	traceDBAddCoverageMetric(
		&out, "db_boundaries_published", int64(db.RowsEmitted))
	enriched := join.Metrics["db_boundaries_enriched"]
	rawPublished := join.Metrics["raw_unmatched_published"]
	rawRetained := join.Metrics["raw_records_retained"]
	traceDBAddCoverageMetric(
		&out, "db_boundaries_raw_enriched", enriched)
	traceDBAddCoverageMetric(
		&out, "raw_unmatched_published", rawPublished)
	traceDBAddCoverageMetric(
		&out, "raw_records_retained", rawRetained)
	if alignment := join.Metadata["raw_db_time_alignment_observation"]; alignment != "" && alignment != "not_applicable_empty_admitted_lane" {
		traceDBAddCoverageMetric(&out, "raw_join_db_boundaries_audited",
			join.Metrics["db_boundaries_audited"])
		traceDBAddCoverageMetric(&out, "raw_db_exact_coordinate_overlap_cohorts",
			join.Metrics["raw_db_exact_coordinate_overlap_cohorts"])
		out.Metadata["raw_db_time_alignment_observation"] = alignment
	}
	if int64(db.RowsEmitted) > math.MaxInt64-rawPublished {
		out.Metadata["reconciliation_state"] = "failed_standard_total_overflow"
		out.Error = "scheduler standard event total overflow"
		return out
	}
	standardTotal := int64(db.RowsEmitted) + rawPublished
	traceDBAddCoverageMetric(
		&out, "standard_sched_switch_events", standardTotal)

	rawTypedWithheld := join.Metrics["raw_unique_records_unmatched"]
	rawKeyRejected := join.Metrics["raw_records_key_rejected"]
	rawAmbiguous := join.Metrics["raw_ambiguous_records"]
	traceDBAddCoverageMetric(
		&out, "raw_unmatched_typed_withheld", rawTypedWithheld)
	traceDBAddCoverageMetric(
		&out, "raw_records_key_rejected", rawKeyRejected)
	traceDBAddCoverageMetric(
		&out, "raw_records_ambiguous", rawAmbiguous)
	accounted := enriched
	for _, value := range []int64{
		rawPublished, rawTypedWithheld, rawKeyRejected, rawAmbiguous,
	} {
		if accounted > math.MaxInt64-value {
			out.Metadata["reconciliation_state"] =
				"failed_raw_accounting_overflow"
			out.Error = "scheduler raw accounting overflow"
			return out
		}
		accounted += value
	}
	traceDBAddCoverageMetric(&out, "raw_records_accounted", accounted)
	if rawRetained >= accounted {
		traceDBAddCoverageMetric(
			&out, "raw_records_unclassified_residual",
			rawRetained-accounted)
	} else {
		traceDBAddCoverageMetric(
			&out, "raw_records_overaccounted", accounted-rawRetained)
	}
	out.Metadata["db_suppression_semantics"] =
		"DB source rows suppressed by complete-per-CPU audit are not final loss when an exact raw-unmatched event was independently published"
	out.Metadata["publication_contract"] = fmt.Sprintf(
		"db=%d; raw_unmatched=%d; standard_total=%d; raw_typed_withheld=%d",
		db.RowsEmitted, rawPublished, standardTotal, rawTypedWithheld)
	switch {
	case join.Error != "" || db.Error != "":
		out.Metadata["reconciliation_state"] =
			"withheld_source_coverage_error"
		out.Skipped = "scheduler publication reconciliation withheld: source coverage contains an error"
	case rawRetained == accounted:
		out.Metadata["reconciliation_state"] =
			"complete_exact_raw_record_closure"
	default:
		out.Metadata["reconciliation_state"] =
			"complete_with_unclassified_raw_residual"
	}
	return out
}
