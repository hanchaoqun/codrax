package agent

// answer_document_evaluator_top_io_inode_test.go — INODE (§28.6, 2026-07-09)
// observation-supplement pins for the top_io_inode statistical lane: the
// claim-key prefix mounts into the order table (order 65, the eleventh
// bucket), the row renders through the generic supplement text path with its
// typed notes, and — the reason the cap was re-balanced 40 → 44 — the lane
// keeps its floor seats under a causal-lane flood instead of being the one
// bucket the "每序列保底" disclosure silently lies about.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func topIOInodeSupplementRecord(i int) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              fmt.Sprintf("trace_query:q1#top_io_inode:%d", i+1),
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "berlin.systrace"},
		Span:            types.ObservationSpan{LineStart: 9000 + i*10, LineEnd: 9005 + i*10},
		ClaimKey:        fmt.Sprintf("top_io_inode:0xa%d", i),
		Subject:         fmt.Sprintf("hot%d.db", i),
		Predicate:       "top_io_inode",
		Object:          "259:1",
		Value:           fmt.Sprintf("%d", 20-i),
		Unit:            "events",
		RichNotes: []string{
			fmt.Sprintf("inode=0xa%d", i),
			"dev=259:1",
			fmt.Sprintf("count=%d", 20-i),
			"reads=2",
			"max_latency=7.000",
			"groups_total=12",
		},
		SupportRefs: []string{fmt.Sprintf("berlin.systrace:%d-%d", 9000+i*10, 9005+i*10)},
		Confidence:  0.74,
	}
}

// TestTopIOInodeSupplementRowRenders pins the lane end-to-end through the
// supplement renderer: admitted (order != 0), labelled by claim key, value
// with the events unit, and the groups_total truncation note visible.
func TestTopIOInodeSupplementRowRenders(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "结论。",
	}}}
	out := renderTraceQueryObservationSupplement(ptv7SpnSupplementContext([]types.ObservationRecord{topIOInodeSupplementRecord(0)}), doc, "zh")
	if !strings.Contains(out, "IO 热点对象：hot0.db -> 259:1") {
		t.Fatalf("top_io_inode row must render in the supplement:\n%s", out)
	}
	if !strings.Contains(out, "值=20events") {
		t.Fatalf("frequency value must render with its unit:\n%s", out)
	}
	if !strings.Contains(out, "对象总数：12") {
		t.Fatalf("the truncation-honesty note must reach the panel:\n%s", out)
	}
}

// TestTopIOInodeSupplementLaneSurvivesFlood is the cap re-adjudication
// witness (buckets × floor = 44 = cap): under a 112-row state_drilldown
// flood, the LAST-sorted top_io_inode bucket still keeps min(N, floor)=4
// rows — before the 40→44 re-balance the eleventh bucket owned zero
// guaranteed seats and the floored-selection disclosure lied.
func TestTopIOInodeSupplementLaneSurvivesFlood(t *testing.T) {
	records := make([]types.ObservationRecord, 0, 118)
	for i := 0; i < 112; i++ {
		records = append(records, ptv7SpnDrilldownRecord(i))
	}
	for i := 0; i < 6; i++ {
		records = append(records, topIOInodeSupplementRecord(i))
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "结论。",
	}}}
	out := renderTraceQueryObservationSupplement(ptv7SpnSupplementContext(records), doc, "zh")
	if got := strings.Count(out, "IO 热点对象："); got != 4 {
		t.Fatalf("the IO hotspot bucket must retain four floor seats, got %d:\n%s", got, out)
	}
	for i := 0; i < 4; i++ {
		subject := fmt.Sprintf("IO 热点对象：hot%d.db", i)
		if !strings.Contains(out, subject) {
			t.Fatalf("top_io_inode floor seat %q must survive the flood:\n%s", subject, out)
		}
	}
}
