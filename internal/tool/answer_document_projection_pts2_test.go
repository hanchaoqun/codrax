package tool

// answer_document_projection_pts2_test.go — PTS-2 (#69 用户条件裁定
// 2026-07-06, 账本 real_trace_campaign_20260705.md §7) pins, ruling ①: the
// ENGINE-level aggregate top-8 overflow fold reaches the tree. Producer face:
// ChainResult.AggregatedImpactsFold → exactly ONE wakeup_causal_aggregate
// fold record on the typed folded_* lane (NKR 折叠族 reuse, zero new keys);
// nil field → zero emission (≤8 组反噪音). Display face: the record
// re-materializes through the existing PTV5 fold pipeline into the
// 链上─ ◦ 其余 N 项(折叠) tree row with the ENGINE's group count, and the fold
// node keeps the never-lead marker.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func pts2AggregateFoldRecords(t *testing.T, withFold bool) []types.ObservationRecord {
	t.Helper()
	result := traceNoteKeysEmitFixtureResult()
	if !withFold {
		result.WakeupChain.AggregatedImpactsFold = nil
	}
	return traceQueryTypedObservations(result, "full.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
}

func TestPTS2AggregateFoldRecordEmitsTypedFoldLane(t *testing.T) {
	records := pts2AggregateFoldRecords(t, true)
	var fold *types.ObservationRecord
	for i := range records {
		if records[i].ClaimKey != "wakeup_causal_aggregate:folded_overflow" {
			continue
		}
		if fold != nil {
			t.Fatalf("exactly ONE aggregate fold record must be emitted")
		}
		fold = &records[i]
	}
	if fold == nil {
		t.Fatalf("AggregatedImpactsFold must emit the aggregate fold record")
	}
	if fold.Predicate != "wakeup_causal_aggregate" {
		t.Fatalf("fold record must ride the aggregate predicate lane: %q", fold.Predicate)
	}
	joined := strings.Join(fold.RichNotes, "\n")
	// The four NKR 折叠族 keys + the on-chain lane + the F1 anchor note; the
	// headline value is the member MAX (2.500 from the fixture), never a sum.
	for _, want := range []string{
		"folded_rows=3",
		"folded_min_ms=0.500",
		"folded_max_ms=2.500",
		"folded_subjects=ovfa-500,ovfb-501",
		"chain_relevance=on_chain",
		"causality=on_wakeup_chain",
		"selected_window=",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("aggregate fold record must carry %q:\n%s", want, joined)
		}
	}
	if fold.Value != "2.500" || fold.Unit != "ms" {
		t.Fatalf("fold value must be the member MAX in ms: %q %q", fold.Value, fold.Unit)
	}
}

func TestPTS2AggregateFoldZeroEmissionWithoutEngineFold(t *testing.T) {
	// ≤8 组零发射反噪音: the engine leaves the field nil → no fold record and
	// no fold claim key anywhere in the aggregate lane.
	for _, record := range pts2AggregateFoldRecords(t, false) {
		if record.ClaimKey == "wakeup_causal_aggregate:folded_overflow" {
			t.Fatalf("nil AggregatedImpactsFold must emit no aggregate fold record: %+v", record)
		}
	}
}

func TestPTS2EngineAggregateFoldRowReachesTreeWithCount(t *testing.T) {
	records := pts2AggregateFoldRecords(t, true)
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	var foldNode *types.TraceCausalProjectionNode
	for _, bucket := range [][]types.TraceCausalProjectionNode{projection.OnChainCauses, projection.SupportingHops} {
		for i := range bucket {
			if bucket[i].OnChainOverflowFold && strings.Contains(strings.Join(bucket[i].MergedSubjects, ","), "ovfa-500") {
				foldNode = &bucket[i]
			}
		}
	}
	if foldNode == nil {
		t.Fatalf("the engine aggregate fold record must re-materialize as an on-chain fold node: %+v / %+v",
			projection.OnChainCauses, projection.SupportingHops)
	}
	if foldNode.MergedCount != 3 || foldNode.MergedMinMS != 0.5 || foldNode.MergedMaxMS != 2.5 {
		t.Fatalf("fold node must carry the ENGINE group count and range: %+v", foldNode)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// P2a rider 件1 (§29.55.3 用户裁定形, 2026-07-13): 边词管车道(链上─ 与
	// 兄弟行同列)+行名管折叠(其余N项(折叠))+记号位留形态族。
	// EVOLUTION RECORD (R9 §29.93.2, 2026-07-15): line 1 slimmed to the bare
	// counted label; the roster head moved to the subordinate 成员 line.
	if !strings.Contains(fence, "链上─ ◦ 其余 3 项(折叠)") {
		t.Fatalf("the tree must render the engine-level fold row with the correct count:\n%s", fence)
	}
	if !strings.Contains(fence, "· 成员 ovfa-500 · 其余 2 项见明细") {
		t.Fatalf("the fold row's roster head must sink to the 成员 line:\n%s", fence)
	}
	// Never-lead stays owned by the existing PTV5 fold pipeline.
	if lead := runtimeTraceProjLeadOnChainFallback(model); lead != nil && lead.OnChainOverflowFold {
		t.Fatalf("the engine fold row must never lead a conclusion: %+v", lead)
	}
}
