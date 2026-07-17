package tool

// trace_query_rankdis_a1_test.go — RANKDIS-EXT A1 (§29.104.16/.16.1,
// 2026-07-16; witness customlogs/cust_span_vs_prio.txt): four ordinal
// families shared the bare `rank` word on the LLM text/wire face, and a
// customer model grep'd the raw payload, read the state-drilldown ordinals
// as root-cause board seats plus the chain/adjacent double rank=1 as a board
// contradiction, and reconciled phantom rank conflicts for six turns. The
// word faces are now scoped:
//   - state_drilldown  → drill_rank (text + JSON + engine Summary)
//   - auto-window      → window_rank (JSON + caveat + candidate listing)
//   - window_sweep     → density_rank (JSON + text; hotspot prefix kept)
//   - root_cause_rank  → keeps bare rank= (THE rank board) and each seated
//     row wears rank_channel=chain|adjacent read from the ordinal
//     allocator's own typed single source (never a second implementation).
//
// MUTATION self-check: re-inlining `rank=` in any renamed Fprintf reds the
// positive pins; dropping the channel gate (Rank>0) reds the no-seat arm;
// re-deriving the channel from anything but RootCauseRankOrdinalChannelWord
// desyncs from the allocator and reds the dual rank=1 arm.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRankdisA1RankChannelWordDisambiguatesDualBoards(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			Items: []tracequery.RootCauseRankItem{
				{
					Rank: 1, Tier: "primary", Type: "runnable_wait",
					Thread:         tracequery.ThreadRef{Comm: "dep", PID: 21},
					StartTs:        1.1, EndTs: 1.9,
					ImpactMs:       9.586, CumulativeImpactMs: 9.586,
					Score:          8.0, Confidence: 0.88, LineStart: 10, LineEnd: 20,
					Source:         "wakeup_chain",
					ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
					Summary:        "on-chain dependency wait",
				},
				{
					Rank: 1, Tier: "secondary", Type: "priority_inversion_candidate",
					Thread:         tracequery.ThreadRef{Comm: "neighbor", PID: 33},
					StartTs:        1.2, EndTs: 1.8,
					ImpactMs:       8.608, CumulativeImpactMs: 8.608,
					Score:          6.4, Confidence: 0.74, LineStart: 30, LineEnd: 40,
					Source:         "window_stats",
					ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
					Summary:        "adjacent displacement",
				},
				{
					Rank: 0, Tier: "target_self_state", Type: "sleep_wait",
					Thread:         tracequery.ThreadRef{Comm: "app", PID: 20},
					StartTs:        1.0, EndTs: 2.0,
					ImpactMs:       12.0, CumulativeImpactMs: 12.0,
					Confidence:     0.80, LineStart: 50, LineEnd: 60,
					Source:         "wakeup_chain",
					ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
					Summary:        "target's own wait symptom",
				},
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "path", "/tmp/payload.json")

	// Witness reproduction: the chain and adjacent boards still publish one
	// rank=1 EACH — but every seated ordinal now self-describes its channel.
	if !strings.Contains(summary, "- rank=1 rank_channel=chain tier=primary") {
		t.Fatalf("the chain board seat must wear rank_channel=chain:\n%s", summary)
	}
	if !strings.Contains(summary, "- rank=1 rank_channel=adjacent tier=secondary") {
		t.Fatalf("the adjacent board seat must wear rank_channel=adjacent:\n%s", summary)
	}
	// No-seat rows (rank=0) carry no channel word — a channel word implies a
	// board seat.
	var selfLine string
	for _, line := range strings.Split(summary, "\n") {
		if strings.Contains(line, "tier=target_self_state") {
			selfLine = line
		}
	}
	if selfLine == "" {
		t.Fatalf("self-state row expected in the text face:\n%s", summary)
	}
	if !strings.HasPrefix(selfLine, "- rank=0 tier=") {
		t.Fatalf("a no-seat row keeps the legacy channel-less head byte-identically:\n%s", selfLine)
	}
	if strings.Contains(selfLine, "rank_channel=") {
		t.Fatalf("a rank=0 row must not wear a channel word:\n%s", selfLine)
	}
	// The dispatch prefix the ledger's legacy text re-parse lane keys on is
	// unchanged ("- rank=").
	if !strings.Contains(summary, "- rank=1 ") || !strings.Contains(summary, "- rank=0 ") {
		t.Fatalf("rank rows must keep the '- rank=' head:\n%s", summary)
	}
	// RANKDIS-EXT B11 (M24): the row's own segment window wears row_window=;
	// the bare " window=" word leaves the rank row face entirely
	// (nearest_window= / occurrence_windows= are already scoped words).
	if !strings.Contains(summary, "thread=dep-21 row_window=1.100000..1.900000") {
		t.Fatalf("the rank row segment window must wear row_window=:\n%s", summary)
	}
	for _, line := range strings.Split(summary, "\n") {
		if strings.HasPrefix(line, "- rank=") && strings.Contains(line, " window=") {
			t.Fatalf("B11: a rank row must not carry a bare window= token:\n%s", line)
		}
	}
}

// RANKDIS-EXT A3 typed-lane pin (对抗复核 P2-1, 2026-07-16): the typed
// drilldown observation mints its ordinal on the dedicated state_rank lane —
// an M5a mutation (reverting the mint to the causal `rank` key) must red
// HERE, not survive on prompt-side coincidence.
func TestRankdisA3TypedDrilldownObservationRidesStateRankLane(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			StateDrilldownPlan: []tracequery.StateDrilldownStep{{
				Rank:   2,
				Thread: tracequery.ThreadRef{Comm: "app", PID: 20},
				State:  "s_sleep", ImpactMs: 21.0, TotalMs: 30.0,
				Source:    "top_sleep",
				LineStart: 121, LineEnd: 130,
				Summary: "app long sleep",
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
		t.Fatalf("typed drilldown observation missing: %+v", records)
	}
	hasStateRank := false
	for _, note := range drill.RichNotes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, "state_rank=") {
			hasStateRank = true
		}
		if strings.HasPrefix(note, "rank=") {
			t.Fatalf("typed drilldown observation must not mint the causal rank key: %+v", drill.RichNotes)
		}
	}
	if !hasStateRank {
		t.Fatalf("typed drilldown observation must carry state_rank= (want state_rank=2): %+v", drill.RichNotes)
	}
	if !strings.Contains(strings.Join(drill.RichNotes, "\n"), "state_rank=2") {
		t.Fatalf("state_rank must carry the step ordinal verbatim: %+v", drill.RichNotes)
	}
}

// RANKDIS-EXT B11 (§29.104.16.1 M24): the interaction row's pair is the
// FIRST and LAST observed interaction timestamps, not a bounded analysis
// window — it wears first_last=.
//
// MUTATION self-check: reverting the Fprintf key to window= reds both arms.
func TestRankdisB11InteractionRowWearsFirstLastWord(t *testing.T) {
	result := tracequery.Result{
		View: "interaction_stats",
		InteractionStats: &tracequery.InteractionStatsResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			Items: []tracequery.InteractionSummary{{
				Peer:              tracequery.ThreadRef{Comm: "peer", PID: 33},
				WakeupsToTarget:   2,
				TotalInteractions: 3,
				FirstTs:           1.25, LastTs: 1.75,
				FirstLine: 10, LastLine: 20,
				Summary: "peer interactions",
			}},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "interaction_stats"}, "path", "/tmp/payload.json")
	if !strings.Contains(summary, "lines=10-20 first_last=1.250000..1.750000") {
		t.Fatalf("interaction rows must wear the first_last= word:\n%s", summary)
	}
	for _, line := range strings.Split(summary, "\n") {
		if strings.HasPrefix(line, "- peer=") && strings.Contains(line, " window=") {
			t.Fatalf("B11: an interaction row must not carry a bare window= token:\n%s", line)
		}
	}
}

func TestRankdisA1DrilldownWordsAreScoped(t *testing.T) {
	result := tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			StateDrilldownPlan: []tracequery.StateDrilldownStep{
				{
					Rank:   1,
					Thread: tracequery.ThreadRef{Comm: "app", PID: 20},
					State:  "s_sleep", ImpactMs: 21.0, TotalMs: 30.0,
					Source: "top_sleep", ChainRequired: true, Recursive: true,
					WindowProportion: 0.7, Significant: true,
					RecommendedViews: []string{"wakeup_chain", "root_cause_rank"},
					LineStart:        121, LineEnd: 130,
					Summary: "app long sleep requires wakeup-chain drilldown",
				},
			},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "window_stats"}, "path", "/tmp/payload.json")
	if !strings.Contains(summary, "- state_drilldown drill_rank=1 thread=app-20 state=s_sleep") {
		t.Fatalf("the drilldown ordinal must wear the scoped drill_rank word:\n%s", summary)
	}
	// RANKDIS-EXT B6 (M11): the tool face wears the same total_scope word as
	// the engine Summary (typed-Source fork; top_sleep = single_state).
	if !strings.Contains(summary, "total=30.000ms total_scope=single_state source=top_sleep") {
		t.Fatalf("the tool drilldown line must carry the total_scope population word:\n%s", summary)
	}
	// Witness reproduction (cust_span_vs_prio round 6: "rank 1-8 … rank 9"
	// read off state_drilldown lines as board seats): the drilldown surface
	// carries ZERO bare rank= hits after the rename.
	if strings.Contains(summary, "state_drilldown rank=") {
		t.Fatalf("RANKDIS-EXT A1: the drilldown line must not wear the bare rank= word:\n%s", summary)
	}
}
