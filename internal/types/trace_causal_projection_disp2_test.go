package types

// trace_causal_projection_disp2_test.go — DISP-2 (Wave-3.2 显示半场道,
// docs/design/real_trace_campaign_20260705.md §27.2 G2/G3, §27.5 G19,
// 2026-07-09) projection-compile pins:
//
//   - G2 双发布去重: a data-blind-spot thread already holding its own ◇/▒
//     stanza seat leaves the on-chain overflow fold membership (seat-
//     conditioned exclusivity, count honestly reduced); a blind-spot thread
//     WITHOUT a seat keeps folding — PTS 永不静默丢.
//   - G19 typed accounting: MergedAllDataGap marks the fold whose EVERY member
//     is a typed data-gap row (wording input for the all-zero fold note).
//   - typed note captures: trace_gap_kind → TraceGapKind (wording fork input);
//     blocking_from_site → BlockingFromSite (BLOCKFROM 等待点, wire-contract
//     key name pinned with the TEX engine batch; gated on the lock-row
//     blocking_kind presence exactly like holder_site).
//
// Mutation self-checks (each verified RED during development, then reverted):
//   M-1: dropping the seatedDataGapSubjects exclusion in
//        traceCausalProjectionLimitNodesOnChainFold →
//        TestTraceCausalProjectionDataGapFoldSeatExclusivity red (dual seat).
//   M-2: hardcoding allDataGap=true in traceCausalProjectionOverflowFoldRow →
//        TestTraceCausalProjectionOverflowFoldAllDataGapFlag red (mixed arm).
//   M-3: parsing blocking_from_site outside the BlockingKind gate →
//        TestTraceCausalProjectionBlockingFromSiteRequiresLockRow red.
//
// Note-key literals below are the deliberate wire-format double-write (test
// files keep verbatim tokens — trace_note_keys.go change protocol step 3).

import (
	"fmt"
	"strings"
	"testing"
)

// disp2GapLedger builds 24 valued on-chain hop records (filling the on-chain
// bucket cap exactly) plus two zero-valued on-chain trace_gap copies for
// threads gap-a/gap-b — both land in the overflow fold — and, when
// withAdjacentSeat is set, the ◇ stanza copy of gap-a (an adjacent rank
// observation of the same thread, the real double-publication shape:
// different predicate, so node-key dedupe keeps both).
func disp2GapLedger(withAdjacentSeat bool) ObservationLedger {
	records := []ObservationRecord{}
	for i := 1; i <= 24; i++ {
		records = append(records, ObservationRecord{
			ID:              fmt.Sprintf("hop-%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			Subject:         fmt.Sprintf("w-%d", i),
			Object:          "runnable_wait",
			Value:           "20.000",
			Unit:            "ms",
			Confidence:      0.8,
			RichNotes: []string{"causality=on_wakeup_chain", "chain_relevance=on_chain",
				fmt.Sprintf("impact=%d.000ms", 100-i)},
		})
	}
	for _, thread := range []string{"gap-a", "gap-b"} {
		records = append(records, ObservationRecord{
			ID:              "gap-chain-" + thread,
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "wakeup_causal_impact",
			Subject:         thread,
			Object:          "trace_gap",
			Confidence:      0.6,
			RichNotes: []string{"causality=on_wakeup_chain", "chain_relevance=on_chain",
				"type=trace_gap", "tier=data_gap", "rank=0", "trace_gap_kind=no_sched_data"},
		})
	}
	if withAdjacentSeat {
		records = append(records, ObservationRecord{
			ID:              "gap-adjacent-gap-a",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Predicate:       "root_cause_context",
			ClaimKey:        "root_cause_context:gap-a",
			Subject:         "gap-a",
			Object:          "trace_gap",
			Confidence:      0.6,
			RichNotes: []string{"causality=adjacent_to_wakeup_chain", "chain_relevance=adjacent",
				"type=trace_gap", "tier=data_gap", "rank=0", "trace_gap_kind=no_sched_data"},
		})
	}
	return ObservationLedger{Records: records}
}

func disp2FoldRow(t *testing.T, nodes []TraceCausalProjectionNode) TraceCausalProjectionNode {
	t.Helper()
	for _, node := range nodes {
		if node.OnChainOverflowFold {
			return node
		}
	}
	t.Fatalf("no on-chain overflow fold row in bucket: %d nodes", len(nodes))
	return TraceCausalProjectionNode{}
}

func disp2SubjectsContain(subjects []string, want string) bool {
	for _, subject := range subjects {
		if strings.TrimSpace(subject) == want {
			return true
		}
	}
	return false
}

// TestTraceCausalProjectionDataGapFoldSeatExclusivity is the G2 双发布去重 pin:
// with the ◇ seat present, the seated thread leaves the fold (count honestly
// reduced to the remaining member); without the seat, both blind-spot threads
// keep folding — no silent drop in either direction.
func TestTraceCausalProjectionDataGapFoldSeatExclusivity(t *testing.T) {
	// Dual-publication shape: gap-a holds a ◇ seat → fold keeps ONLY gap-b.
	got := CompileTraceCausalProjection(disp2GapLedger(true))
	if len(got.AdjacentCauses) != 1 || got.AdjacentCauses[0].Subject != "gap-a" {
		t.Fatalf("gap-a must hold its individual ◇ seat: %+v", got.AdjacentCauses)
	}
	fold := disp2FoldRow(t, got.OnChainCauses)
	if fold.MergedCount != 1 {
		t.Fatalf("seated blind-spot thread must leave the fold count (want 1, got %d): %+v", fold.MergedCount, fold)
	}
	if disp2SubjectsContain(fold.MergedSubjects, "gap-a") {
		t.Fatalf("seated blind-spot thread must not double-publish through the fold roster: %+v", fold.MergedSubjects)
	}
	if !disp2SubjectsContain(fold.MergedSubjects, "gap-b") {
		t.Fatalf("unseated blind-spot thread must keep its fold membership: %+v", fold.MergedSubjects)
	}
	if !fold.MergedAllDataGap {
		t.Fatalf("all-data-gap fold must carry the typed MergedAllDataGap accounting: %+v", fold)
	}
	// Control (no ◇ seat): both blind-spot threads fold — PTS 永不静默丢.
	control := CompileTraceCausalProjection(disp2GapLedger(false))
	controlFold := disp2FoldRow(t, control.OnChainCauses)
	if controlFold.MergedCount != 2 ||
		!disp2SubjectsContain(controlFold.MergedSubjects, "gap-a") ||
		!disp2SubjectsContain(controlFold.MergedSubjects, "gap-b") {
		t.Fatalf("without an individual seat both blind spots must keep folding: %+v", controlFold)
	}
	if !controlFold.MergedAllDataGap {
		t.Fatalf("control fold is all data gaps and must say so: %+v", controlFold)
	}
}

// TestTraceCausalProjectionOverflowFoldAllDataGapFlag pins the typed G19
// accounting on the fold constructor directly: mixed members → false, pure
// data-gap members → true.
func TestTraceCausalProjectionOverflowFoldAllDataGapFlag(t *testing.T) {
	gap := func(subject string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{Subject: subject, Object: "trace_gap",
			Tier: TraceCausalTierDataGap, ChainRelevance: "on_chain", Confidence: 0.6}
	}
	valued := TraceCausalProjectionNode{Subject: "v-1", Object: "runnable_wait",
		ChainRelevance: "on_chain", ImpactMS: 3, CumulativeImpactMS: 3, Confidence: 0.8}
	pure := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{gap("g-1"), gap("g-2")})
	if !pure.MergedAllDataGap {
		t.Fatalf("pure data-gap fold must set MergedAllDataGap: %+v", pure)
	}
	mixed := traceCausalProjectionOverflowFoldRow([]TraceCausalProjectionNode{gap("g-1"), valued})
	if mixed.MergedAllDataGap {
		t.Fatalf("mixed fold must NOT claim all-data-gap membership: %+v", mixed)
	}
}

// TestTraceCausalProjectionTraceGapKindParse pins the typed criterion capture:
// the trace_gap_kind note reaches the node verbatim; absence stays empty
// (legacy fail-open — the display keeps the 2026-07-07 wording).
func TestTraceCausalProjectionTraceGapKindParse(t *testing.T) {
	record := ObservationRecord{
		ID: "gap-1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "root_cause_context", Subject: "gap-a", Object: "trace_gap",
		RichNotes: []string{"type=trace_gap", "tier=data_gap", "rank=0",
			"trace_gap_kind=no_eligible_wait"},
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.TraceGapKind != "no_eligible_wait" {
		t.Fatalf("trace_gap_kind must reach the node verbatim: %q", node.TraceGapKind)
	}
	record.RichNotes = []string{"type=trace_gap", "tier=data_gap", "rank=0"}
	if node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record); node.TraceGapKind != "" {
		t.Fatalf("absent trace_gap_kind must stay empty (legacy fail-open): %q", node.TraceGapKind)
	}
}

// TestTraceCausalProjectionBlockingFromSiteRequiresLockRow pins the BLOCKFROM
// capture and its lock-row gate: the blocking_from_site note reaches the node
// verbatim ONLY beside a typed blocking_kind (same gate as holder_site — a
// blocking-from without contention semantics is meaningless).
func TestTraceCausalProjectionBlockingFromSiteRequiresLockRow(t *testing.T) {
	site := "monitor contention with owner OS_FFRT_2_2-43037 blocking from AssetManager.getResourceValue(AssetManager.java:761)"
	record := ObservationRecord{
		ID: "blk-1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Predicate: "critical_blocking", Subject: "waiter-1", Object: "blocking_span",
		RichNotes: []string{"type=blocking_span", "blocking_kind=monitor_contention",
			"holder_site=SharedLibrary::GetStrings(art_dex.cc:120)",
			"blocking_from_site=" + site},
	}
	node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record)
	if node.BlockingFromSite != site {
		t.Fatalf("blocking_from_site must reach the node verbatim (untruncated): %q", node.BlockingFromSite)
	}
	record.RichNotes = []string{"type=blocking_span", "blocking_from_site=" + site}
	if node := traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record); node.BlockingFromSite != "" {
		t.Fatalf("blocking_from_site without a typed blocking_kind must not be captured: %q", node.BlockingFromSite)
	}
}
