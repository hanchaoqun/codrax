package agent

// PSG batch P-d② pin (§25 ruling d, docs/design/real_trace_campaign_20260705.md,
// 2026-07-08; huadong_01 §22 C-P1 / C1): the wakeup_causal_aggregate lane
// (order 55) is the dump's LAST-sorted bucket, so under the pre-F3 flat
// prefix cut a 112-row state_drilldown flood evicted it entirely — the
// 46.821-class aggregate actual values had NO surviving dump row to hang
// off. The PTV7-SPN F3① per-order floor (min(N, 4) head seats per bucket)
// is ASSESSED here as sufficient: the huadong-shaped flood keeps the
// aggregate rows in the 40-row window, so P-d② ships as this pin with no
// quota change (桶×floor≤cap balance stays pinned by
// TestSupplementQuotaOrderUniverseBalancesCap).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func psgAggregateRecord(i int) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              fmt.Sprintf("trace_query:agg:%d", i),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "berlin.systrace"},
		Span:            types.ObservationSpan{LineStart: 5000 + i*10, LineEnd: 5005 + i*10},
		ClaimKey:        fmt.Sprintf("wakeup_causal_aggregate:VSyncGenerator-2270:w%d", i),
		Subject:         "VSyncGenerator-2270",
		Predicate:       "wakeup_causal_aggregate",
		Object:          "s_sleep",
		Value:           fmt.Sprintf("30.98%d", i),
		Unit:            "ms",
		// No occurrence_windows note: the record stays in the order-55
		// bucket (the occurrence_windows escape would move it to order 8).
		RichNotes:   []string{"type=sleep", "impact=30.986ms"},
		SupportRefs: []string{fmt.Sprintf("berlin.systrace:%d-%d", 5000+i*10, 5005+i*10)},
	}
}

// TestPSGSupplementQuotaAggregateBucketSurvivesFlood — P-d② assessment pin:
// the huadong shape (112 order-18 drilldown rows + 3 order-55 aggregate
// rows) keeps EVERY aggregate row inside the 40-row window under the
// per-order floor of 4. Raising the flood or removing the floor turns this
// red — it is the "aggregate truth stays verifiable in the dump" witness.
func TestPSGSupplementQuotaAggregateBucketSurvivesFlood(t *testing.T) {
	records := make([]types.ObservationRecord, 0, 115)
	for i := 0; i < 112; i++ {
		records = append(records, ptv7SpnDrilldownRecord(i))
	}
	for i := 0; i < 3; i++ {
		records = append(records, psgAggregateRecord(i))
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "结论。",
	}}}
	out := renderTraceQueryObservationSupplement(ptv7SpnSupplementContext(records), doc, "zh")
	if got := strings.Count(out, "唤醒链聚合：VSyncGenerator-2270"); got != 3 {
		t.Fatalf("all three order-55 aggregate rows must keep their floor seats, got %d:\n%s", got, out)
	}
	for i := 0; i < 3; i++ {
		value := fmt.Sprintf("30.98%d", i)
		if !strings.Contains(out, value) {
			t.Fatalf("the aggregate row's value %q must be visible in the dump:\n%s", value, out)
		}
	}
}
