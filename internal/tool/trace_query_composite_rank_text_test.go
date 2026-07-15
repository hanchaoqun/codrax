package tool

// trace_query_composite_rank_text_test.go — QH2-A 件2 站② (§29.55 观察③
// 族裁延伸, 2026-07-14): the LLM-facing root-cause-rank text never dresses a
// composite-score row's POSITIVE value slots in the wall-clock ms suit — the
// published value is a score over mixed units (registry caliber class, token
// 恰一 block_io_by_inode), so the slots carry the non-wall-clock caliber word
// instead. Zero slots and every non-composite row stay byte-identical, and
// the wire typed field names / the numbers themselves are untouched (裁定
// 射程: 纯词面/后缀/记号).
//
// MUTATION self-check: reverting traceQueryRankImpactValue to the wall-clock
// form (or re-inlining %.3fms in the rank Fprintf) reds the composite
// assertions; touching the wall-clock arm reds the byte-identity arm.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestQH2ACompositeRankRowTextDropsMsSuit(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 1, Tier: "caliber_side", Type: "block_io_by_inode",
					Thread:   tracequery.ThreadRef{Comm: "CompThread_0", PID: 2955},
					StartTs:  1.1, EndTs: 1.9,
					ImpactMs: 61.540, ProjectedImpactMs: 61.540, CumulativeImpactMs: 61.540,
					Score: 43.078, Confidence: 0.70, LineStart: 10, LineEnd: 20,
					Source: "window_stats.block_io_by_inode", Summary: "inode envelope",
				},
				{
					Rank: 2, Tier: "direct", Type: "long_sleep",
					Thread:   tracequery.ThreadRef{Comm: "dep", PID: 21},
					StartTs:  1.2, EndTs: 1.8,
					ImpactMs: 4.000, ProjectedImpactMs: 4.000, CumulativeImpactMs: 4.000,
					Score: 3.200, Confidence: 0.80, LineStart: 30, LineEnd: 40,
					Source: "wakeup_chain", Summary: "dep slept before wakeup",
				},
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "path", "/tmp/payload.json")
	var compositeLine, wallLine string
	for _, line := range strings.Split(summary, "\n") {
		if strings.Contains(line, "type=block_io_by_inode") {
			compositeLine = line
		}
		if strings.Contains(line, "type=long_sleep") {
			wallLine = line
		}
	}
	if compositeLine == "" || wallLine == "" {
		t.Fatalf("both rank rows expected in the text face:\n%s", summary)
	}
	// Composite row: positive value slots wear the caliber word, never ms.
	for _, want := range []string{
		"impact=61.540(composite score, not wall clock)",
		"cumulative_impact=61.540(composite score, not wall clock)",
		"effective_impact=61.540(composite score, not wall clock)",
		"projected_impact=61.540(composite score, not wall clock)",
		"projected_total=61.540(composite score, not wall clock)",
	} {
		if !strings.Contains(compositeLine, want) {
			t.Fatalf("composite rank row must carry %q, got:\n%s", want, compositeLine)
		}
	}
	if strings.Contains(compositeLine, "61.540ms") {
		t.Fatalf("a composite value slot must never wear the ms suit:\n%s", compositeLine)
	}
	// Zero slots make no composite claim and keep the legacy form.
	if !strings.Contains(compositeLine, "target_impact=0.000ms") {
		t.Fatalf("zero slots stay byte-identical:\n%s", compositeLine)
	}
	// Non-composite rows are byte-identical to the legacy wall-clock form.
	if !strings.Contains(wallLine, "impact=4.000ms cumulative_impact=4.000ms effective_impact=4.000ms target_impact=0.000ms projected_impact=4.000ms projected_total=4.000ms") {
		t.Fatalf("wall-clock rank rows must keep the legacy ms form byte-identically:\n%s", wallLine)
	}
	if strings.Contains(wallLine, "composite score") {
		t.Fatalf("the caliber word must never leak onto wall-clock rows:\n%s", wallLine)
	}
}

// TestCompositeRankObservationUnitCaliberToken pins 终判⑧ (§29.96.2,
// 2026-07-15): the typed observation lane publishes the composite-score
// caliber TOKEN on the digest Unit field (types.TraceObservationUnitCompositeScore)
// for the block_io_by_inode family — the retired shape rendered 值=X ms off
// Unit="ms" on a mixed-unit score. The wire field name ("unit") is unchanged
// (不改名关账); every non-composite rank row keeps Unit="ms" byte-identically.
//
// MUTATION self-check: reverting traceQueryRankObservationUnit to the
// unconditional "ms" reds the composite arm; widening it reds the wall arm.
func TestCompositeRankObservationUnitCaliberToken(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 1, Tier: "caliber_side", Type: "block_io_by_inode",
					Thread:   tracequery.ThreadRef{Comm: "CompThread_0", PID: 2955},
					StartTs:  1.1, EndTs: 1.9,
					ImpactMs: 61.540, CumulativeImpactMs: 61.540,
					Score: 43.078, Confidence: 0.70, LineStart: 10, LineEnd: 20,
					Source: "window_stats.block_io_by_inode", Summary: "inode envelope",
				},
				{
					Rank: 2, Tier: "direct", Type: "long_sleep",
					Thread:   tracequery.ThreadRef{Comm: "dep", PID: 21},
					StartTs:  1.2, EndTs: 1.8,
					ImpactMs: 4.000, CumulativeImpactMs: 4.000,
					Score: 3.200, Confidence: 0.80, LineStart: 30, LineEnd: 40,
					Source: "wakeup_chain", Summary: "dep slept before wakeup",
				},
			},
		},
	}
	records := traceQueryTypedObservations(result, "path", "payload", "raw", "scope", time.Unix(1752537600, 0))
	var composite, wall *types.ObservationRecord
	for i := range records {
		switch records[i].Object {
		case "block_io_by_inode":
			composite = &records[i]
		case "long_sleep":
			wall = &records[i]
		}
	}
	if composite == nil || wall == nil {
		t.Fatalf("both rank observations expected, got %+v", records)
	}
	if composite.Unit != types.TraceObservationUnitCompositeScore {
		t.Fatalf("终判⑧: the composite rank observation must publish the caliber token on Unit, got %q", composite.Unit)
	}
	if composite.Value != "61.540" {
		t.Fatalf("the published value stays the bare score (wire numbers untouched), got %q", composite.Value)
	}
	if wall.Unit != "ms" {
		t.Fatalf("non-composite rank observations keep Unit=ms byte-identically, got %q", wall.Unit)
	}
}
