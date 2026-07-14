package context

// supp_feed_test.go — SUPP-FEED (DISPATCH-IND 批3, 2026-07-14): the S12
// acceptance pins (设计稿 scratchpad/dispatch_ind_design_20260714.md §3.2 —
// "finalize 语境含 TraceWaitEvidence 节(补采趟)E2E pin").
//
// Disease face (S12): the EVID-1 / PROSE-RC / WAKE-CENSUS feed sections
// (TraceWaitEvidence + TraceRootCauseBoard) compile from the observation
// ledger, whose record families used to be single-sourced on whichever
// trace_query views the model happened to dispatch. SUPP-CORE (§29.71)
// decoupled family presence via the post-explore deterministic supplement;
// R1 ruling (a) makes the FINALIZE feed the direct beneficiary while the
// EXPLORE dispatch surface stays displacement-free. These pins hold the
// three feed-side wiring points in internal/context/builder.go:
//
//   1. finalize ledger input keeps the SystemTraceSupplementResults lane
//      (board + wait-evidence families minted by the supplement reach the
//      answer author's prompt);
//   2. the explore-stage feed clears the lane (zero displacement — with the
//      supplement latched, the explore feed is byte-identical to a run that
//      never ran the supplement, which is also the no-trigger baseline
//      identity: §29.62/§29.58 forms unchanged);
//   3. the finalize census fallback (banner-parse arm) sees the supplement's
//      tool results (builder.go appends the lane to censusResults at
//      finalize only).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// suppFeedModelOnlyToolResult is the h2 dispatch shape: the model ran
// trace_query but never a rank/window_stats/critical view — the ledger is
// deterministic-runtime-query populated, yet carries none of the feed
// families (board empty, wait evidence silent).
func suppFeedModelOnlyToolResult() types.ToolResult {
	return types.ToolResult{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			traceWaitTestRecord("trace_query:m#thread_timeline:1",
				"CompThread_0-2955", "d_sleep", "thread_state_span", "36.757"),
		},
	}
}

// suppFeedSupplementToolResult carries the supplement-minted families of one
// deterministic re-collection: a seated rank row (board family + wait-object
// caller fact), the pid-keyed blocked_reason census (PROSE-RC ③ total), and
// a window-total wakeup_edge_census pair (WAKE-CENSUS-D 2A form).
func suppFeedSupplementToolResult() types.ToolResult {
	return types.ToolResult{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			traceWaitTestRecord("trace_query:supp#root_cause_rank:1",
				"CompThread_0-2955", "d_state_or_io_wait", "root_cause_primary", "36.757",
				"rank=1", "effective_impact_ms=36.757",
				types.TraceNoteKeyMemberCount+"=4",
				types.TraceNoteKeyBlockedReasonCaller+"=dma_fence_default_w"),
			traceWaitTestRecord("trace_query:supp#blocked_reason_census:1",
				"CompThread_0-2955", "blocked_reason", "blocked_reason_census", "12",
				types.TraceNoteKeyBlockedReasonCensus+"=dma_fence_default_w×12(Σ36.757ms)"),
			traceWaitTestRecord("trace_query:supp#wakeup_edge_census:1",
				"gpu-token-id4-2931", "CompThread_0-2955", "wakeup_edge_census", "12",
				types.TraceNoteKeyWakeupEdgeCensusFirstTs+"=13762.801234",
				types.TraceNoteKeyWakeupEdgeCensusLastTs+"=13762.998765",
				types.TraceNoteKeyWakeupEdgeCensusSleepExit+"=0",
				types.TraceNoteKeyWakeupEdgeCensusDExit+"=11",
				types.TraceNoteKeyWakeupEdgeCensusOtherExit+"=1",
				types.TraceNoteKeySelectedWindow+"=13762.791708..13763.024898",
				types.TraceNoteKeyWakeupEdgeCensusTargetWakee+"=true"),
		},
	}
}

func suppFeedBus(modelResults []types.ToolResult) *types.BusContext {
	return &types.BusContext{
		RepoRoot:    "/tmp/repo",
		Mutable:     types.NewMutableState("trace wait-object question"),
		ToolResults: modelResults,
	}
}

// TestSuppFeed_FinalizeFeedConsumesSupplementLane — wiring point 1: on the
// h2 dispatch shape (model minted zero rank/census families) the supplement
// lane alone must light BOTH finalize feed sections with its families.
func TestSuppFeed_FinalizeFeedConsumesSupplementLane(t *testing.T) {
	bus := suppFeedBus([]types.ToolResult{suppFeedModelOnlyToolResult()})
	baseline := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if baseline.TraceRootCauseBoard != "" {
		t.Fatalf("fixture broken: the model-only dispatch shape must not seat a board:\n%s", baseline.TraceRootCauseBoard)
	}
	if baseline.TraceWaitEvidence != "" {
		t.Fatalf("fixture broken: the model-only dispatch shape must keep wait evidence silent:\n%s", baseline.TraceWaitEvidence)
	}

	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views: []string{"root_cause_rank"},
	}, []types.ToolResult{suppFeedSupplementToolResult()})
	ac := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)

	for _, want := range []string{
		"#1", "CompThread_0-2955", "36.757ms (effective attribution)",
	} {
		if !strings.Contains(ac.TraceRootCauseBoard, want) {
			t.Fatalf("finalize board summary must seat the supplement rank row (missing %q):\n%s", want, ac.TraceRootCauseBoard)
		}
	}
	for _, want := range []string{
		// wait-object caller fact off the supplement rank record
		"caller=dma_fence_default_w · d_state_or_io_wait 36.757ms · members=4",
		// PROSE-RC ③ quotable census total off the supplement census record
		"total 12 blocked_reason record(s) in its selected window, use this total verbatim",
		// WAKE-CENSUS-D 2A window-total pair off the supplement census record
		"gpu-token-id4-2931 → CompThread_0-2955 ×12 raw wakeup(s) in the analysis window",
	} {
		if !strings.Contains(ac.TraceWaitEvidence, want) {
			t.Fatalf("finalize wait evidence must carry the supplement families (missing %q):\n%s", want, ac.TraceWaitEvidence)
		}
	}
}

// TestSuppFeed_ExploreFeedZeroDisplacement — wiring point 2: with the
// supplement latched, the EXPLORE feed stays byte-identical to a run that
// never ran the supplement. The baseline run carries full §29.62/§29.58-form
// model results, so the identity is over NON-empty sections (a stronger pin
// than empty == empty), doubling as the no-trigger baseline-identity pin.
func TestSuppFeed_ExploreFeedZeroDisplacement(t *testing.T) {
	modelResults := func() []types.ToolResult {
		return []types.ToolResult{{
			ToolName: "trace_query", Success: true,
			Observations: append([]types.ObservationRecord(nil), traceWaitTestLedger().Records...),
		}}
	}
	without := BuildAgentContext(suppFeedBus(modelResults()), types.AgentExplorer, types.StageExplore)
	if without.TraceWaitEvidence == "" || without.TraceRootCauseBoard == "" {
		t.Fatalf("fixture broken: the baseline explore feed must be non-empty for a meaningful identity")
	}

	busWith := suppFeedBus(modelResults())
	busWith.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views: []string{"root_cause_rank", "critical_blocking_calls"},
	}, []types.ToolResult{suppFeedSupplementToolResult()})
	with := BuildAgentContext(busWith, types.AgentExplorer, types.StageExplore)

	if with.TraceWaitEvidence != without.TraceWaitEvidence {
		t.Fatalf("explore wait evidence must be byte-identical with the supplement latched (zero displacement):\nwith:\n%s\nwithout:\n%s", with.TraceWaitEvidence, without.TraceWaitEvidence)
	}
	if with.TraceRootCauseBoard != without.TraceRootCauseBoard {
		t.Fatalf("explore board summary must be byte-identical with the supplement latched (zero displacement):\nwith:\n%s\nwithout:\n%s", with.TraceRootCauseBoard, without.TraceRootCauseBoard)
	}
	// The same bus at FINALIZE must diverge (the lane is finalize-visible) —
	// guards against "identity holds because the lane is dead everywhere".
	finalize := BuildAgentContext(busWith, types.AgentFinalizer, types.StageFinalize)
	if finalize.TraceWaitEvidence == with.TraceWaitEvidence {
		t.Fatalf("finalize wait evidence must consume the supplement lane the explore feed excludes:\n%s", finalize.TraceWaitEvidence)
	}
}

// TestSuppFeed_FinalizeBannerFallbackSeesSupplementResults — wiring point 3:
// when NO typed census note reached the ledger, the banner-parse census
// FALLBACK must read the supplement's tool results at finalize (builder.go
// appends the lane to censusResults), and must NOT read them at explore.
func TestSuppFeed_FinalizeBannerFallbackSeesSupplementResults(t *testing.T) {
	supplement := types.ToolResult{
		ToolName: "trace_query", Success: true,
		Summary: "- blocked_reason CompThread_0-2955 iowait=0 count=12 line=133 caller=dma_fence_default_w+0x260/0x4dc[devhost.elf]\n",
		Observations: []types.ObservationRecord{
			traceWaitTestRecord("trace_query:supp#root_cause_rank:1",
				"CompThread_0-2955", "d_state_or_io_wait", "root_cause_primary", "36.757",
				"rank=1", "effective_impact_ms=36.757"),
		},
	}
	bus := suppFeedBus([]types.ToolResult{suppFeedModelOnlyToolResult()})
	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
		Views: []string{"root_cause_rank"},
	}, []types.ToolResult{supplement})

	finalize := BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	if !strings.Contains(finalize.TraceWaitEvidence, "dma_fence_default_w ×12") {
		t.Fatalf("the finalize banner-census fallback must see the supplement's tool results:\n%s", finalize.TraceWaitEvidence)
	}
	explore := BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	if strings.Contains(explore.TraceWaitEvidence, "dma_fence_default_w ×12") {
		t.Fatalf("the explore feed must not see the supplement's banner rows:\n%s", explore.TraceWaitEvidence)
	}
}
