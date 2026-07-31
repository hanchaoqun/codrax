package types

// observation_ledger_rank_backfill_gapa_test.go — Wave-3.1 GAP-A 复核 P2-3 pin
// (2026-07-09): the LEGACY text re-parse lane (traceQueryRootCauseRankRecord,
// reachable only on degraded results without typed Observations) mirrors the
// typed-observation lane's backfill gate — positional rank backfill fires ONLY
// on identity-less lines (rank<=0 ∧ tier==""). A tier-carrying rank=0 line is
// the engine's deliberate G9 no-board-seat signal (target_self_state /
// data_gap rows); the pre-P2-3 unconditional backfill resurrected Rank>0 here
// and re-badged the row on the projection face — the fourth face breaking
// 三面同源.
//
// MUTATION self-check: reverting the gate to the unconditional
// `if rank <= 0 { rank = ordinal }` reds TestLegacyRankLaneKeepsNoSeatSignalP23.

import (
	"strings"
	"testing"
)

func TestLegacyRankLaneKeepsNoSeatSignalP23(t *testing.T) {
	ref := ObservationSourceRef{Path: "trace.systrace"}
	record, ok := traceQueryRootCauseRankRecord(3, 7,
		"- rank=0 tier=data_gap type=trace_gap thread=ghost-500 confidence=0.60 lines=95-96 scheduler intervals exist in the aligned window but all sit below min_duration_ms (no eligible wait candidate)",
		ref, "2026-07-09T00:00:00Z")
	if !ok {
		t.Fatal("the tier-carrying rank=0 line must still mint a record")
	}
	sawTier := false
	for _, note := range record.RichNotes {
		if strings.HasPrefix(note, "rank=") {
			t.Fatalf("P2-3: the legacy lane must not resurrect an ordinal for a tier-carrying rank=0 line, got %q", note)
		}
		if note == "tier=data_gap" {
			sawTier = true
		}
	}
	if !sawTier {
		t.Fatalf("the engine tier must survive the legacy re-parse, got %v", record.RichNotes)
	}
	if record.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		record.Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("data_gap fallback must stay on the coverage lane, got %+v", record)
	}
	// The record identity keys on the ordinal (no rank-keyed ID collisions
	// across multiple no-seat rows).
	if record.ID != "tool:3#trace_query:root_cause_rank:7" {
		t.Fatalf("ID must key on the ordinal position, got %q", record.ID)
	}

	// P3-1 mirror: a summary-less no-seat line renders the no-seat fallback,
	// never a fabricated "#0" ordinal.
	noSummary, ok := traceQueryRootCauseRankRecord(3, 8,
		"- rank=0 tier=data_gap type=trace_gap thread=ghost2-501 confidence=0.60",
		ref, "2026-07-09T00:00:00Z")
	if !ok {
		t.Fatal("summary-less line must still mint")
	}
	if !strings.Contains(noSummary.Summary, "(no rank seat)") || strings.Contains(noSummary.Summary, "#0") {
		t.Fatalf("P3-1 mirror: no-seat summary fallback drifted, got %q", noSummary.Summary)
	}

	// Positive control: an identity-less line (no tier) keeps the legacy
	// positional backfill — rank note and tier word derived from the ordinal.
	legacy, ok := traceQueryRootCauseRankRecord(3, 2,
		"- rank=0 type=cpu_pressure thread=agg impact=5.000ms",
		ref, "2026-07-09T00:00:00Z")
	if !ok {
		t.Fatal("identity-less line must mint via the backfill arm")
	}
	sawRank := false
	for _, note := range legacy.RichNotes {
		if note == "rank=2" {
			sawRank = true
		}
	}
	if !sawRank || legacy.Predicate != "root_cause_secondary" {
		t.Fatalf("identity-less lines keep the positional backfill (rank=2/secondary), got predicate=%q notes=%v",
			legacy.Predicate, legacy.RichNotes)
	}
}

func TestLegacyRootEvidenceTraceGapUsesCoverageProvenanceZ1(t *testing.T) {
	ref := ObservationSourceRef{Path: "trace.systrace"}
	gap, ok := traceQueryRootEvidenceRecord(4, 1,
		"- root_evidence=trace_gap thread=render_service-2000 confidence=0.60 — no scheduler intervals for this thread inside the aligned window",
		ref, "2026-07-31T00:00:00Z")
	if !ok {
		t.Fatal("trace-gap root evidence must still mint a coverage record")
	}
	if gap.ProvenanceLane != ObservationProvenanceArtifactSpan {
		t.Fatalf("trace-gap root evidence must not carry direct-cause authority: %+v", gap)
	}

	wait, ok := traceQueryRootEvidenceRecord(4, 2,
		"- root_evidence=binder_wait thread=binder:1-7 duration=8.000ms confidence=0.80 — observed binder wait",
		ref, "2026-07-31T00:00:00Z")
	if !ok {
		t.Fatal("positive root evidence must still mint")
	}
	if wait.ProvenanceLane != ObservationProvenanceObservedDirectCause {
		t.Fatalf("positive root evidence must retain direct-cause provenance: %+v", wait)
	}
}
