package hitraceconv

import (
	"context"
	"strings"
	"testing"
)

// source_raw_scheduler_reconciliation_gate_test.go — batch-six review
// fold-in #7 (colleague_merge_audit §40.53 收编复核再收编): the scheduler
// publication reconciliation is the eighth writer of the closed lane key
// `reconciliation_state`. It consumes the scheduler-lite switch join's
// published coverage, and used to mint complete_exact_raw_record_closure
// whenever the join row existed with raw_records_retained == accounted
// (0 == 0) — beside a join that said not_applicable_source_profile (legacy
// 0x0ace capture) or census_incomplete_source_raw_decode (official capture,
// truncated raw segment). It now inherits the join's gate outcome through the
// class funnel and, past the gate, publishes a complete_ closure only over a
// join that reached one of its own terminal arms.

func traceDBSchedulerReconciliationDBRow() TraceDBCoverage {
	return TraceDBCoverage{
		Family: "scheduler", Table: "sched_slice", Role: "query_ready_export",
		Found: true, RowsRead: 3, RowsEmitted: 3,
		Metrics: map[string]int64{"db_source_rows_suppressed": 0},
	}
}

// TestTraceDBSchedulerPublicationReconciliationInheritsTheJoinGateOutcome
// runs the three probe captures through the real inventory scan, the real
// join constructor/finalizer and the real reconciliation writer, then the
// real conversion_quality copy: legacy → not applicable, truncated official →
// census incomplete (naming the census), full body → the lane's own closure
// with no gate label. Red on 381f36cc9: every arm read
// complete_exact_raw_record_closure and the conversion_quality copy was
// dropped whenever the callstack census row preceded the reconciliation row.
func TestTraceDBSchedulerPublicationReconciliationInheritsTheJoinGateOutcome(t *testing.T) {
	const lane = "scheduler publication reconciliation"
	for _, test := range []struct {
		arm        string
		wantState  string
		reasonKey  string
		wantReason string
		wantSkip   string
	}{
		{arm: "legacy_envelope", wantState: traceDBSourceRawLaneNotApplicableState,
			reasonKey: "not_applicable_reason", wantReason: traceDBRawDecodeStateNotApplicableNonOfficialProfile,
			wantSkip: lane + " not applicable: strict official source raw profile absent"},
		{arm: "official_truncated_segment", wantState: traceDBSourceRawLaneCensusIncompleteState,
			reasonKey: "census_incomplete_reason", wantReason: traceDBRawDecodeStateWithheldSegmentInventoryIncomplete,
			wantSkip: lane + " not evaluated: source raw decode census incomplete (withheld_segment_inventory_incomplete)"},
		{arm: "official_full_body", wantState: "complete_exact_raw_record_closure"},
	} {
		t.Run(test.arm, func(t *testing.T) {
			inventory := traceDBSourceRawGateFixture(t, test.arm)
			join := newTraceDBRawSchedSwitchLiteJoin(&inventory)
			// The export audits the DB boundaries before finalizing; the two
			// gate-stopped joins are not ready and the audit is a no-op there.
			if err := join.auditDBBoundaries(context.Background(), nil, traceDBSchedulerAuthority{}, traceDBSchedAudit{}); err != nil {
				t.Fatal(err)
			}
			joinCoverage, err := join.finalize()
			if err != nil {
				t.Fatal(err)
			}
			db := traceDBSchedulerReconciliationDBRow()
			out := traceDBSchedulerPublicationReconciliationCoverage([]TraceDBCoverage{db, joinCoverage})
			if out.Error != "" || out.Metadata["reconciliation_state"] != test.wantState {
				t.Fatalf("%s: reconciliation_state=%q err=%q join_state=%q\n%+v", test.arm,
					out.Metadata["reconciliation_state"], out.Error, joinCoverage.Metadata["join_state"], out)
			}
			if test.reasonKey == "" {
				if out.Skipped != "" || out.Metadata["not_applicable_reason"] != "" || out.Metadata["census_incomplete_reason"] != "" ||
					out.Metrics["raw_records_accounted"] != 0 || out.Metrics["standard_sched_switch_events"] != 3 {
					t.Fatalf("%s: ready join wore a gate label or lost its closure metrics: %+v", test.arm, out)
				}
			} else {
				if out.Metadata[test.reasonKey] != test.wantReason || out.Skipped != test.wantSkip ||
					out.RowsEmitted != 0 || len(out.Metrics) != 0 {
					t.Fatalf("%s: inherited outcome drifted: %+v", test.arm, out)
				}
				if strings.HasPrefix(out.Metadata["reconciliation_state"], "complete_") {
					t.Fatalf("%s: non-ready join published a complete_ closure: %+v", test.arm, out)
				}
			}
			// The conversion_quality copy carries the same label regardless of
			// where the callstack census row sits in the coverage list.
			callstack := TraceDBCoverage{Family: "slice", Table: "callstack",
				Metadata: map[string]string{"official_viewer_typed_only_sync_reason_census": "complete"}}
			for _, order := range [][]TraceDBCoverage{
				{callstack, db, joinCoverage, out},
				{db, joinCoverage, out, callstack},
			} {
				quality := traceDBSemanticQualityCoverage(order)
				if quality.Metadata["scheduler_publication_reconciliation"] != test.wantState ||
					quality.Metadata["official_viewer_typed_only_sync_reason_census"] != "complete" {
					t.Fatalf("%s: conversion_quality copy drifted: %+v", test.arm, quality.Metadata)
				}
			}
		})
	}
}

// TestTraceDBSchedulerPublicationReconciliationWithholdsNonReconcilableJoin:
// past the gate, the join's post-gate vocabulary decides through the total
// table — a withheld or unfinished join (its counters are not exact closure
// inputs) is withheld by name, a terminal join closes, a join that failed
// loud keeps the source-coverage-error arm, and an undeclared join_state
// fails the lane loud instead of closing over it (§40.50).
func TestTraceDBSchedulerPublicationReconciliationWithholdsNonReconcilableJoin(t *testing.T) {
	joinWith := func(state string) TraceDBCoverage {
		join := newTraceDBRawSchedSwitchLiteJoinCoverage()
		join.Found = true
		join.Metadata["join_state"] = state
		join.Metrics = map[string]int64{"raw_records_retained": 5}
		return join
	}
	for state, reconcilable := range traceDBRawSchedSwitchLiteJoinReconcilable {
		switch state {
		case traceDBSourceRawLaneNotApplicableState, traceDBSourceRawLaneCensusIncompleteState:
			continue // inherited through the funnel, pinned above
		}
		out := traceDBSchedulerPublicationReconciliationCoverage(
			[]TraceDBCoverage{traceDBSchedulerReconciliationDBRow(), joinWith(state)})
		got := out.Metadata["reconciliation_state"]
		if reconcilable {
			if out.Error != "" || got != "complete_with_unclassified_raw_residual" || out.Skipped != "" {
				t.Fatalf("%s: terminal join did not reconcile: state=%q err=%q skipped=%q", state, got, out.Error, out.Skipped)
			}
			continue
		}
		if out.Error != "" || got != "withheld_source_join_not_reconcilable" || out.Metadata["source_join_state"] != state ||
			out.Skipped != "scheduler publication reconciliation withheld: scheduler-lite switch join did not reach an exact raw record census ("+state+")" ||
			len(out.Metrics) != 0 {
			t.Fatalf("%s: non-reconcilable join was not withheld by name: %+v", state, out)
		}
	}
	failed := joinWith(traceDBRawSchedSwitchLiteJoinStateFailedPublicationMismatch)
	failed.Error = "trace DB output invariant rejected: scheduler_lite_switch_join_publication_mismatch"
	out := traceDBSchedulerPublicationReconciliationCoverage(
		[]TraceDBCoverage{traceDBSchedulerReconciliationDBRow(), failed})
	if out.Error != "" || out.Metadata["reconciliation_state"] != "withheld_source_coverage_error" {
		t.Fatalf("failed-loud join lost the source-coverage-error arm: %+v", out)
	}
	for _, undeclared := range []string{"", traceDBSourceRawLanePlaceholderState, "complete_exact_raw_record_closure"} {
		out := traceDBSchedulerPublicationReconciliationCoverage(
			[]TraceDBCoverage{traceDBSchedulerReconciliationDBRow(), joinWith(undeclared)})
		if !strings.Contains(out.Error, "scheduler_publication_reconciliation_join_state_unresolved") ||
			out.Metadata["reconciliation_state"] != traceDBSourceRawLanePlaceholderState {
			t.Fatalf("undeclared join_state %q was absorbed: %+v", undeclared, out)
		}
	}
}
