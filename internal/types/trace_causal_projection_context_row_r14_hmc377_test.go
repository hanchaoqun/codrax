package types

import "testing"

// HMC377 round 14 (batch-owner ruling R-377-14 ⑤, 2026-11-24; review finding
// R13H-2): a rank board's context-only row and a priced row are two
// publications, not two views of one fact. The same-fact pass joined them on
// their equal subject, value and line range: the context row survived, took
// the priced row's attribution and printed it under 「上下文·不参与根因排序」,
// and the priced row — the only carrier of that time — left the board.
//
// The nodes below are the ones the production order compiles for the
// reviewer's universe Z1cTwinOnly (the publication gate withdraws an inversion
// seat to a context-only row and releases its claimless scheduler-latency
// twin to the board; internal/tool
// TestHMC377R14ReleasedTwinIsItsOwnRowBesideTheContextRow runs that order end
// to end).

func r14ContextRankRow() TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID: "rank:2", Subject: "binder:458_5-6326", Predicate: "root_cause_context_only", Object: "unknown_state",
		Tier: TraceCausalTierContextOnly, StateKind: "runnable", ChainRelevance: "on_chain",
		ImpactMS: 1.903, CumulativeImpactMS: 1.903, RunnableMS: 1.903, EffectiveImpactPublished: true,
		LineStart: 10, LineEnd: 20, Unit: "ms",
	}
}

func r14ReleasedTwin() TraceCausalProjectionNode {
	node := r14ContextRankRow()
	node.EvidenceID, node.Object, node.Tier = "rank:3", "scheduler_latency", "tertiary"
	node.EffectiveImpactMS = 1.903
	return node
}

func TestHMC377R14ContextRankRowNeverJoinsAPricedRow(t *testing.T) {
	for _, order := range [][]TraceCausalProjectionNode{
		{r14ContextRankRow(), r14ReleasedTwin()},
		{r14ReleasedTwin(), r14ContextRankRow()},
	} {
		projection := TraceCausalProjection{OnChainCauses: append([]TraceCausalProjectionNode(nil), order...)}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.OnChainCauses) != 2 {
			t.Fatalf("first %s: the context row and the priced row stay two rows: %+v", order[0].EvidenceID, projection.OnChainCauses)
		}
		for _, node := range projection.OnChainCauses {
			if len(node.MergedEvidenceIDs) != 0 || len(node.SecondaryObjects) != 0 {
				t.Fatalf("first %s: row %s took the other publication: merged %v objects %v", order[0].EvidenceID, node.EvidenceID, node.MergedEvidenceIDs, node.SecondaryObjects)
			}
			if node.IsContextOnlyRow() != (node.EffectiveImpactMS == 0) {
				t.Fatalf("first %s: row %s tier %s attribution %.3f: the context row carries none, the priced row keeps its own", order[0].EvidenceID, node.EvidenceID, node.Tier, node.EffectiveImpactMS)
			}
		}
	}
}

// The context row still joins the other views of its own fact: its chain hop
// row carries no attribution either.
func TestHMC377R14ContextRankRowStillJoinsItsUnpricedView(t *testing.T) {
	hop := r14ContextRankRow()
	hop.EvidenceID, hop.Predicate, hop.Object, hop.Tier = "hop:1", "wakeup_causal_impact", "runnable", ""
	for _, order := range [][]TraceCausalProjectionNode{{r14ContextRankRow(), hop}, {hop, r14ContextRankRow()}} {
		projection := TraceCausalProjection{OnChainCauses: append([]TraceCausalProjectionNode(nil), order...)}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.OnChainCauses) != 1 || len(projection.OnChainCauses[0].MergedEvidenceIDs) != 1 {
			t.Fatalf("first %s: one fact, one row: %+v", order[0].EvidenceID, projection.OnChainCauses)
		}
	}
}

// A state-evidence record of the same segments is a view of the priced row's
// fact, whatever tier token it carries: it is no rank-board row and keeps
// joining it (dio w19 dtmp.worker-59765: the 9.644ms runnable evidence record
// with tier context_only under the hop priced 9.644ms; 1 700 such pairs on the
// 315 real windows, lane evidence fix14/mirror/scan/ctxmix_head.txt).
func TestHMC377R14StateEvidenceRecordStillJoinsItsPricedRow(t *testing.T) {
	hop := TraceCausalProjectionNode{
		EvidenceID: "hop:9", Subject: "dtmp.worker-59765", Predicate: "wakeup_causal_impact", Object: "runnable", StateKind: "runnable",
		ChainRelevance: "on_chain", ImpactMS: 9.644, CumulativeImpactMS: 9.644, RunnableMS: 9.644,
		EffectiveImpactMS: 9.644, EffectiveImpactPublished: true, LineStart: 101169, LineEnd: 102535, Unit: "ms",
	}
	evidence := hop
	evidence.EvidenceID, evidence.Predicate, evidence.Object, evidence.Tier = "evidence:1", "runnable_wait", "", TraceCausalTierContextOnly
	evidence.EffectiveImpactMS = 0
	for _, order := range [][]TraceCausalProjectionNode{{hop, evidence}, {evidence, hop}} {
		projection := TraceCausalProjection{OnChainCauses: append([]TraceCausalProjectionNode(nil), order...)}
		traceCausalProjectionMergeSameFacts(&projection)
		if len(projection.OnChainCauses) != 1 || projection.OnChainCauses[0].EffectiveImpactMS != 9.644 {
			t.Fatalf("first %s: one fact, one priced row: %+v", order[0].EvidenceID, projection.OnChainCauses)
		}
	}
}
