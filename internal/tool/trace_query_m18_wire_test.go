package tool

// trace_query_m18_wire_test.go — RANKDIS-M18 (§29.104.16.1 M18, user ruling
// §29.104.17 ② 2026-07-16) tool-face pins. Three faces fork together on the
// registry composite-value wire arm (io_pressure joins block_io_by_inode):
//   1. rank TEXT face — io_pressure value slots wear the composite caliber
//      word (QH2-A word face extended off the caliber-side class arm);
//   2. observation NOTE face — composite rows publish impact_score/
//      cumulative_impact_score/effective_impact_score/projected_*_score
//      instead of the ms-semantic keys (one row emits exactly one family);
//   3. observation UNIT face — io_pressure rank records carry the typed
//      composite_score caliber token (终判⑧ lane extended).
// The state-drilldown composite weight note renamed rank_impact →
// rank_impact_score in the same batch (§7.30 S1 witness family).
//
// MUTATION self-check: reverting traceQueryTypedPriorityRichNotes' key fork
// reds the note arms; reverting the traceQueryRankImpactValue /
// traceQueryRankObservationUnit gates to the caliber-side class arm reds the
// io_pressure arms; renaming the drilldown note back reds the drilldown pin.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryM18RankResult() tracequery.Result {
	return tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			Items: []tracequery.RootCauseRankItem{
				{
					// Engine-real shape: the closed effective matrix zeroes
					// io_pressure (aggregate pressure is context_only), so the
					// row wears the context tier, no seat, and its zero
					// attribution rides the context sentinel note.
					Rank: 0, Tier: tracequery.RootCauseTierContextOnly, Type: "io_pressure",
					ImpactMs: 61.540, ProjectedImpactMs: 61.540, CumulativeImpactMs: 61.540,
					Score: 43.078, Confidence: 0.70, LineStart: 10, LineEnd: 20,
					Source: "window_stats.io_pressure_summary",
					Summary: "io pressure signal=io_activity score=61.540",
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
}

// TestM18IOPressureRankTextWearsCompositeCaliber: the io_pressure Score is
// the same mixed-unit composite as block_io_by_inode (M18 value#7) — its
// positive rank-text value slots drop the ms suit.
func TestM18IOPressureRankTextWearsCompositeCaliber(t *testing.T) {
	summary := traceQuerySummary(traceQueryM18RankResult(), traceQueryParams{View: "root_cause_rank"}, "path", "/tmp/payload.json")
	var pressureLine, wallLine string
	for _, line := range strings.Split(summary, "\n") {
		if strings.Contains(line, "type=io_pressure") {
			pressureLine = line
		}
		if strings.Contains(line, "type=long_sleep") {
			wallLine = line
		}
	}
	if pressureLine == "" || wallLine == "" {
		t.Fatalf("both rank rows expected in the text face:\n%s", summary)
	}
	for _, want := range []string{
		"impact=61.540(composite score, not wall clock)",
		"cumulative_impact=61.540(composite score, not wall clock)",
		"projected_impact=61.540(composite score, not wall clock)",
		"projected_total=61.540(composite score, not wall clock)",
		// The closed matrix zeroes io_pressure effective attribution; zero
		// slots make no composite claim and keep the legacy form (QH2-A).
		"effective_impact=0.000ms",
	} {
		if !strings.Contains(pressureLine, want) {
			t.Fatalf("io_pressure rank row must carry %q, got:\n%s", want, pressureLine)
		}
	}
	if strings.Contains(pressureLine, "61.540ms") {
		t.Fatalf("an io_pressure value slot must never wear the ms suit:\n%s", pressureLine)
	}
	if !strings.Contains(wallLine, "impact=4.000ms") || strings.Contains(wallLine, "composite score") {
		t.Fatalf("wall-clock rows stay byte-identical:\n%s", wallLine)
	}
}

// TestM18CompositeRankNotesForkScoreKeys: the typed observation note face
// publishes exactly ONE key family per row.
func TestM18CompositeRankNotesForkScoreKeys(t *testing.T) {
	records := traceQueryTypedObservations(traceQueryM18RankResult(), "path", "payload", "raw", "scope", time.Unix(1752537600, 0))
	var pressure, wall *types.ObservationRecord
	for i := range records {
		switch records[i].Object {
		case "io_pressure":
			pressure = &records[i]
		case "long_sleep":
			wall = &records[i]
		}
	}
	if pressure == nil || wall == nil {
		t.Fatalf("both rank observations expected, got %+v", records)
	}
	pressureNotes := strings.Join(pressure.RichNotes, "\n")
	for _, want := range []string{
		types.TraceNoteKeyImpactScore + "=61.540",
		types.TraceNoteKeyCumulativeImpactScore + "=61.540",
		// The context-only ranking-exclusion sentinel rides the score twin on
		// a composite row (one row, one key family).
		types.TraceNoteKeyEffectiveImpactScore + "=0.000",
		"projected_impact_score=61.540",
		"projected_total_score=61.540",
	} {
		if !strings.Contains(pressureNotes, want) {
			t.Fatalf("composite rank notes must carry %q, got:\n%s", want, pressureNotes)
		}
	}
	for _, deny := range []string{"impact_ms=", "cumulative_impact_ms=", "effective_impact_ms=", "projected_total_ms=", "projected_impact_ms="} {
		if strings.Contains(pressureNotes, deny) {
			t.Fatalf("composite rank notes must not carry the ms-semantic key %q:\n%s", deny, pressureNotes)
		}
	}
	if pressure.Unit != types.TraceObservationUnitCompositeScore {
		t.Fatalf("io_pressure rank observation must publish the composite caliber Unit token, got %q", pressure.Unit)
	}
	wallNotes := strings.Join(wall.RichNotes, "\n")
	for _, want := range []string{"impact_ms=4.000", "cumulative_impact_ms=4.000", "effective_impact_ms=4.000", "projected_total_ms=4.000"} {
		if !strings.Contains(wallNotes, want) {
			t.Fatalf("wall-clock rank notes keep the ms keys byte-identically, missing %q:\n%s", want, wallNotes)
		}
	}
	if strings.Contains(wallNotes, "_score=") {
		t.Fatalf("wall-clock rank notes must not mint composite keys:\n%s", wallNotes)
	}
	if wall.Unit != "ms" {
		t.Fatalf("wall-clock rank observations keep Unit=ms, got %q", wall.Unit)
	}
}

// TestM18SpanEchoSpeaksCompositeCaliber (复核件1, P2 2026-07-17): the rank
// line's span= effective_impact echo renders through the SAME wire-arm word
// face as the main value slots — a composite row must not re-dress the same
// number in the ms suit on the same line (real-window witness: block_io row
// with engine effective 0.369 wore "(composite score, not wall clock)" on the
// main slot beside span=effective_impact=0.369ms).
//
// MUTATION self-check: re-inlining %.3fms in traceQueryRootCauseSpanCompact
// reds the composite arm; routing wall rows through the composite word reds
// the byte-identity arm.
func TestM18SpanEchoSpeaksCompositeCaliber(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 0, Tier: "caliber_side", Type: "block_io_by_inode",
					Thread:   tracequery.ThreadRef{Comm: "io", PID: 90},
					ImpactMs: 2.116, CumulativeImpactMs: 2.694, EffectiveImpactMs: 0.369,
					Score: 1.9, Confidence: 0.70, LineStart: 10, LineEnd: 20,
					Source: "window_stats.block_io_by_inode", Summary: "inode envelope",
				},
				{
					Rank: 1, Tier: "direct", Type: "long_sleep",
					Thread:   tracequery.ThreadRef{Comm: "dep", PID: 21},
					ImpactMs: 4.000, CumulativeImpactMs: 4.000, EffectiveImpactMs: 4.000,
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
	if !strings.Contains(compositeLine, "span=effective_impact=0.369(composite score, not wall clock)") {
		t.Fatalf("the composite span= echo must speak the caliber word, got:\n%s", compositeLine)
	}
	if strings.Contains(compositeLine, "0.369ms") {
		t.Fatalf("the composite span= echo must never wear the ms suit:\n%s", compositeLine)
	}
	if !strings.Contains(wallLine, "span=effective_impact=4.000ms") {
		t.Fatalf("wall-clock span= echoes stay byte-identical:\n%s", wallLine)
	}
}

// TestM18StateDrilldownNoteKeyRankImpactScore: the drilldown composite weight
// note left the bare-impact vocabulary with the JSON tag.
func TestM18StateDrilldownNoteKeyRankImpactScore(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			StateDrilldownPlan: []tracequery.StateDrilldownStep{{
				Rank:   1,
				Thread: tracequery.ThreadRef{Comm: "drill", PID: 80},
				State:  string(tracequery.StateRunnable),
				ImpactMs: 9, TotalMs: 20, RankImpactMs: 13.5,
				Source: "state_churn", LineStart: 33, LineEnd: 34,
				Summary: "drill runnable",
			}},
		},
	}
	records := traceQueryTypedObservations(result, "path", "payload", "raw", "scope", time.Unix(1752537600, 0))
	var drill *types.ObservationRecord
	for i := range records {
		if records[i].Predicate == "state_drilldown" {
			drill = &records[i]
		}
	}
	if drill == nil {
		t.Fatalf("state_drilldown observation expected, got %+v", records)
	}
	notes := strings.Join(drill.RichNotes, "\n")
	if !strings.Contains(notes, "rank_impact_score=13.500") {
		t.Fatalf("drilldown composite weight must ride rank_impact_score, got:\n%s", notes)
	}
	if strings.Contains(notes, "rank_impact=") {
		t.Fatalf("the retired rank_impact note key must not appear:\n%s", notes)
	}
}
