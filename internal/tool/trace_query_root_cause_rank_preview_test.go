package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryPublishesTypedRootCauseRosterBeforeLongBody(t *testing.T) {
	items := make([]tracequery.RootCauseRankItem, 13)
	for i := range items {
		items[i] = tracequery.RootCauseRankItem{
			Rank:               i + 1,
			Tier:               "primary",
			Type:               "runnable",
			Thread:             tracequery.ThreadRef{Comm: fmt.Sprintf("worker-%02d", i+1), PID: 100 + i},
			DominantState:      "runnable",
			CumulativeImpactMs: float64(20 - i),
			EffectiveImpactMs:  float64(20 - i),
			FixDirection:       "scheduling",
			Causality:          "on_wakeup_chain",
			ChainRelevance:     "on_chain",
			LineStart:          1000 + i,
			LineEnd:            1100 + i,
			Source:             "window_stats.runnable",
		}
	}
	items[0].Type = "block_io_by_inode"
	items[0].EffectiveImpactMs = 61.54
	items[0].CumulativeImpactMs = 61.54
	items[0].FixDirection = "io_path"
	items[0].MemberCount = 3
	items[0].CrossDirectionOverlaps = []tracequery.RootCauseCrossDirectionOverlap{{
		OverlapMs: 4.25, LineStart: 200, LineEnd: 220, Direction: "compute_supply", Basis: "dio_segment_intervals",
	}}

	result := tracequery.Result{
		View:          "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{Items: items},
	}
	got := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "attached_trace", "/tmp/rank.json")
	for _, want := range []string{
		"root_cause_rank_preview status=incomplete emitted=12 published_total=13 order=engine_published_board values=typed_no_re_election",
		"board_order=1 rank=1 rank_channel=chain tier=primary type=block_io_by_inode subject=worker-01-100 dominant_state=runnable effective_impact=61.540(composite score, not wall clock)",
		"fix_direction=io_path causality=on_wakeup_chain chain_relevance=on_chain member_count=3 cross_direction_overlaps=4.250@200..220@compute_supply@dio_segment_intervals",
		"root_cause_rank_preview_continuation omitted=1 payload_ref=/tmp/rank.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing compact typed rank field %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "root_cause_rank_preview_row board_order=13") {
		t.Fatalf("head preview exceeded its bounded row cap:\n%s", got)
	}
	previewAt := strings.Index(got, "root_cause_rank_preview status=")
	payloadAt := strings.Index(got, "payload_ref=/tmp/rank.json (audit artifact")
	bodyAt := strings.Index(got, "## Root cause rank")
	if previewAt < 0 || payloadAt < 0 || bodyAt < 0 || previewAt > payloadAt || previewAt > bodyAt {
		t.Fatalf("rank preview must be head-safe, preview=%d payload=%d body=%d:\n%s", previewAt, payloadAt, bodyAt, got)
	}
}

func TestTraceQuerySummaryUsesFrameBundleRankForCompleteHeadPreview(t *testing.T) {
	result := tracequery.Result{
		View: "frame_root_cause_bundle",
		FrameRootCauseBundle: &tracequery.FrameRootCauseBundle{
			RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "io_wait",
				Thread:             tracequery.ThreadRef{Comm: "io-worker", PID: 42},
				CumulativeImpactMs: 3.5, EffectiveImpactMs: 3.5,
				FixDirection: "io_path", Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			}}},
		},
	}
	got := traceQuerySummary(result, traceQueryParams{View: result.View}, "attached_trace", "/tmp/frame.json")
	for _, want := range []string{
		"root_cause_rank_preview status=complete emitted=1 published_total=1",
		"type=io_wait subject=io-worker-42",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("frame-bundle board missing from head preview %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "root_cause_rank_preview_continuation") {
		t.Fatalf("complete one-row board was falsely marked incomplete:\n%s", got)
	}
}
