package types

// trace_causal_projection_rspahyg_d2_test.go — RSPA-HYG 件① (§29.77 立案①,
// 2026-07-14): the D2 same-fact SURVIVOR-EXCHANGE form pin.
//
// The RSPA same-fact key reads the typed full-account float on a ⛓ clipped
// seat (ChainAnchorFullMS), so an UNDECOMPOSED full-value mirror row over the
// SAME line range R1-merges with the clipped seat. When the mirror was seen
// FIRST (it sorts above the clipped seat on impact), the exchange branch must
// swap the ⛓ seat into the survivor slot — the clipped seat OWNS the published
// account (letting the full-value mirror survive would republish the very
// full-window claim the migration retired) — and the 修复轮件2 id re-seeding
// must record the displaced mirror id in MergedEvidenceIDs (lossless).
//
// The production donghu/tieba boards never exercised this branch: their
// full-value twins publish over DIFFERENT line ranges, so the value-keyed
// identity never matched (D2 landed on witness-less reasoning). This fixture
// synthesizes the exact same-line-range geometry with production-real record
// shapes (root_cause_* rank predicate notes for the ⛓ half, the
// critical_blocking micro-probe form for the mirror; the chain_anchored /
// chain_anchor_full typed notes exactly as internal/tool emits them) — 合成补形,
// 如实注.
//
// MUTATION self-check: dropping the exchange branch in
// traceCausalProjectionMergeSameFacts (the `node.ChainAnchorFullMS > 0 &&
// !node.ChainAnchorRemainderSeat && survivor.ChainAnchorFullMS == 0` swap)
// keeps the mirror as survivor → the published-impact assertion reds; dropping
// the 件2 id re-seed inside the branch reds the MergedEvidenceIDs assertion.

import "testing"

func rspaHygD2ClippedSeatRecord() ObservationRecord {
	return ObservationRecord{
		ID: "E-clip", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "root_cause_secondary",
		ClaimKey: "root_cause_secondary:E-clip", Subject: "compthread-2955",
		Object: "dma_fence_default_w", Value: "3.598", Unit: "ms", Confidence: 0.8,
		SupportRefs: []string{"obs:E-clip"},
		Span:        ObservationSpan{LineStart: 100, LineEnd: 200},
		RichNotes: []string{
			"impact_ms=3.598", "cumulative_impact_ms=3.598",
			"chain_relevance=on_chain", "causality=on_wakeup_chain",
			"type=d_state_or_io_wait", "dominant_state=d_state",
			"chain_anchored=3.598", "chain_anchor_full=36.757",
		},
	}
}

func rspaHygD2FullValueMirrorRecord() ObservationRecord {
	return ObservationRecord{
		ID: "E-full", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, Predicate: "critical_blocking",
		ClaimKey: "critical_blocking:E-full", Subject: "compthread-2955",
		Object: "dma_fence_default_w", Value: "36.757", Unit: "ms", Confidence: 0.8,
		SupportRefs: []string{"obs:E-full"},
		Span:        ObservationSpan{LineStart: 100, LineEnd: 200},
		RichNotes: []string{
			"impact_ms=36.757", "cumulative_impact_ms=36.757",
			"chain_relevance=on_chain", "causality=on_wakeup_chain",
			"type=d_state_or_io_wait", "dominant_state=d_state",
		},
	}
}

func TestRSPAHygD2SameLineRangeExchangeSurvivorIsClippedSeat(t *testing.T) {
	// The mirror record enters the ledger FIRST and sorts above the clipped
	// seat on impact (36.757 > 3.598) — the exchange geometry.
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rspaHygD2FullValueMirrorRecord(),
		rspaHygD2ClippedSeatRecord(),
	})
	var survivors []TraceCausalProjectionNode
	for _, node := range got.OnChainCauses {
		if traceCausalProjectionCanonicalNode(node.Subject) == "compthread-2955" {
			survivors = append(survivors, node)
		}
	}
	if len(survivors) != 1 {
		t.Fatalf("one fact one row: want exactly 1 survivor, got %d: %+v", len(survivors), survivors)
	}
	survivor := survivors[0]
	// The ⛓ clipped seat owns the seat: typed decomposition present, published
	// value = the credential-anchored account (never the retired full claim).
	if survivor.ChainAnchorFullMS != 36.757 || survivor.ChainAnchorRemainderSeat {
		t.Fatalf("survivor must be the ⛓ clipped half: %+v", survivor)
	}
	if survivor.ImpactMS != 3.598 || survivor.CumulativeImpactMS != 3.598 {
		t.Fatalf("survivor must publish the anchored value, not the retired full-window claim: %+v", survivor)
	}
	if traceCausalProjectionCanonicalNode(survivor.EvidenceID) != "e-clip" {
		t.Fatalf("the exchange must swap the clipped seat into the survivor slot: %+v", survivor)
	}
	// 修复轮件2 id accounting: the displaced mirror id is absorbed losslessly.
	found := false
	for _, id := range survivor.MergedEvidenceIDs {
		if traceCausalProjectionCanonicalNode(id) == "e-full" {
			found = true
		}
	}
	if !found {
		t.Fatalf("MergedEvidenceIDs must record the displaced mirror id (lossless): %+v", survivor.MergedEvidenceIDs)
	}
}

// Negative arm: the ◇ remainder half NEVER joins the full-value identity —
// its marker forks the key, so a same-line-range full-value mirror keeps its
// own seat beside it (two different accounts, no merge, no exchange).
func TestRSPAHygD2RemainderHalfNeverJoinsFullValueIdentity(t *testing.T) {
	remainder := rspaHygD2ClippedSeatRecord()
	remainder.ID = "E-rem"
	remainder.ClaimKey = "root_cause_context:E-rem"
	remainder.Predicate = "root_cause_context"
	remainder.Value = "33.159"
	remainder.RichNotes = []string{
		"impact_ms=33.159", "cumulative_impact_ms=33.159",
		"chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain",
		"type=d_state_or_io_wait", "dominant_state=d_state",
		"chain_anchored=3.598", "chain_anchor_full=36.757", "chain_anchor_remainder_seat=true",
	}
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		rspaHygD2FullValueMirrorRecord(),
		remainder,
	})
	total := 0
	for _, bucket := range [][]TraceCausalProjectionNode{got.OnChainCauses, got.AdjacentCauses} {
		for _, node := range bucket {
			if traceCausalProjectionCanonicalNode(node.Subject) == "compthread-2955" {
				total++
			}
		}
	}
	if total != 2 {
		t.Fatalf("the ◇ remainder must keep its own seat beside the mirror (marker-forked key), got %d rows", total)
	}
}
