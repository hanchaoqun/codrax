package types

import "testing"

// trace_causal_projection_xerr1_fixb_test.go — XERR1-FIX 修补 件B/件F
// projection-side pins (2026-07-16).
//
// 件B (冷读 P2): the R2 ×N same-(subject,object) fold SUMMED a converged
// blocking-wait ⊖ row (typed basis wait_segments) with the same
// subject+object binder ⋈ rows into one 10.721ms seat — but the ⊖ value
// 6.637 = Σ(sleep 4.084 + …) already PHYSICALLY CONTAINS the binder rows'
// 4.084ms sleep share, so the SUM double-counts. wait_segments rows are
// exempt from R2 grouping and stay independent seats; basis-less rows keep
// the legacy fold byte-identically.

// xerr1FixBRecords builds the cold-read witness trio: one converged ⊖ row
// (6.637, basis wait_segments) + two binder ⋈ rows (2.445 + 1.639 = exactly
// the Σ's sleep component) — all same (subject, object).
func xerr1FixBRecords(withBasis bool) []ObservationRecord {
	blockingNotes := []string{
		"chain_relevance=on_chain", "type=blocking_span",
	}
	if withBasis {
		blockingNotes = append(blockingNotes,
			"blocking_value_basis=wait_segments",
			"blocking_wait_segment_ms=6.637",
			"blocking_wait_sleep_ms=4.084",
			"blocking_span_envelope_ms=29.843",
		)
	}
	return []ObservationRecord{
		aggregateTestRecord("E1", "critical_blocking", "critical_blocking:E1",
			"os.FusionSearch-8091", "OS_FFRT_2_142-61764", "6.637", 6.637, 100, 110, blockingNotes...),
		aggregateTestRecord("E2", "critical_blocking", "critical_blocking:E2",
			"os.FusionSearch-8091", "OS_FFRT_2_142-61764", "2.445", 2.445, 200, 210,
			"chain_relevance=on_chain", "type=binder_wait"),
		aggregateTestRecord("E3", "critical_blocking", "critical_blocking:E3",
			"os.FusionSearch-8091", "OS_FFRT_2_142-61764", "1.639", 1.639, 300, 310,
			"chain_relevance=on_chain", "type=binder_wait"),
	}
}

func xerr1FixBAllNodes(p TraceCausalProjection) []TraceCausalProjectionNode {
	var out []TraceCausalProjectionNode
	for _, lane := range [][]TraceCausalProjectionNode{
		p.PrimaryRootCauses, p.OnChainCauses, p.SupportingHops, p.AdjacentCauses, p.BackgroundCauses,
	} {
		out = append(out, lane...)
	}
	return out
}

// TestTraceCausalProjectionR2FoldExemptsWaitSegmentsBasisXERR1FixB — the 件B
// fix pin (positive arm): the ⊖ row keeps its independent 6.637 seat and the
// binder rows never absorb into it (their remaining group of 2 sits below
// the R2 ≥3 threshold, so they stay independent too).
func TestTraceCausalProjectionR2FoldExemptsWaitSegmentsBasisXERR1FixB(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords(xerr1FixBRecords(true))
	nodes := xerr1FixBAllNodes(got)
	byID := map[string]TraceCausalProjectionNode{}
	for _, node := range nodes {
		if node.MergedCount > 1 {
			t.Fatalf("件B: no R2 ×N seat may mint over the wait_segments row: %+v", node)
		}
		byID[node.EvidenceID] = node
	}
	blocking, ok := byID["E1"]
	if !ok {
		t.Fatalf("件B: the ⊖ row must survive as an independent seat, got %+v", byID)
	}
	if blocking.BlockingValueBasis != TraceCausalProjectionBlockingBasisWaitSegments {
		t.Fatalf("fixture drifted: basis lost on the compiled node: %+v", blocking)
	}
	if blocking.ImpactMS != 6.637 {
		t.Fatalf("件B: the ⊖ seat keeps its own converged value 6.637, got %.3f", blocking.ImpactMS)
	}
	if _, ok := byID["E2"]; !ok {
		t.Fatalf("件B: binder row E2 must stay an independent seat: %+v", byID)
	}
	if _, ok := byID["E3"]; !ok {
		t.Fatalf("件B: binder row E3 must stay an independent seat: %+v", byID)
	}
}

// TestTraceCausalProjectionR2FoldBasisLessRowsKeepLegacySumXERR1FixB — 负向
// arm: WITHOUT the typed basis the exact same trio keeps the legacy R2 ×3
// SUM byte-identically (10.721 = 6.637+2.445+1.639) — 件B is a typed-gate
// exemption, never a blanket blocking-row retreat.
func TestTraceCausalProjectionR2FoldBasisLessRowsKeepLegacySumXERR1FixB(t *testing.T) {
	got := TraceCausalProjectionFromObservationRecords(xerr1FixBRecords(false))
	for _, node := range xerr1FixBAllNodes(got) {
		if node.MergedCount != 3 {
			continue
		}
		if node.ImpactMS < 10.720 || node.ImpactMS > 10.722 {
			t.Fatalf("负向: the legacy ×3 SUM must stay 10.721, got %.3f", node.ImpactMS)
		}
		return
	}
	t.Fatalf("负向: basis-less trio must keep the legacy R2 ×3 fold: %+v", xerr1FixBAllNodes(got))
}

// TestTraceCausalProjectionParsesWaitCoverageNotesXERR1FixF — 件F wire pin:
// the partial-coverage lower-bound pair rides the registered rich notes into
// the typed node fields; absence parses to false/0 (never guesses).
func TestTraceCausalProjectionParsesWaitCoverageNotesXERR1FixF(t *testing.T) {
	with := aggregateTestRecord("F1", "critical_blocking", "critical_blocking:F1",
		"app-1", "waker-2", "3.000", 3.0, 10, 20,
		"chain_relevance=on_chain", "type=blocking_span",
		"blocking_value_basis=wait_segments",
		"blocking_wait_coverage_partial=true",
		"blocking_wait_account_covered_ms=50.000")
	without := aggregateTestRecord("F2", "critical_blocking", "critical_blocking:F2",
		"app-3", "waker-4", "4.000", 4.0, 30, 40,
		"chain_relevance=on_chain", "type=blocking_span",
		"blocking_value_basis=wait_segments")
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{with, without})
	byID := map[string]TraceCausalProjectionNode{}
	for _, node := range xerr1FixBAllNodes(got) {
		byID[node.EvidenceID] = node
	}
	f1, ok := byID["F1"]
	if !ok || !f1.BlockingWaitCoveragePartial || f1.BlockingWaitAccountCoveredMS != 50.0 {
		t.Fatalf("件F: coverage notes must parse into the typed node fields: %+v", f1)
	}
	f2, ok := byID["F2"]
	if !ok || f2.BlockingWaitCoveragePartial || f2.BlockingWaitAccountCoveredMS != 0 {
		t.Fatalf("件F: absent notes must parse to false/0: %+v", f2)
	}
}
