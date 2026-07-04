package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RN-14c (§7.9, A1 soft-advisory paradigm, cust_runnable 2026-07-04) pins:
// a scheduler-shaped trace turn completing with an analyzer-pinned user-focus
// thread (pid 6565) but no wakeup-chain-family observation TARGETING that
// thread rides a soft gate note into the accepted completion — never a
// denial. With a runnable-dominant anchor visible in the typed observations
// the note also carries the via_thread connector.

func rn14cDrilldownObservation(subject, object string, notes ...string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:        "trace_query:w#state_drilldown:1",
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Subject:   subject,
		Predicate: "state_drilldown",
		Object:    object,
		RichNotes: notes,
	}
}

func rn14cBus(t *testing.T, observations ...types.ObservationRecord) (*types.BusContext, *types.MutableState) {
	t.Helper()
	mut := types.NewMutableState("分析 6565 主线程为何慢")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "trace_query",
		Success:      true,
		Timestamp:    time.Now(),
		Observations: observations,
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				RuntimeTargets: []types.RuntimeTarget{{
					Kind: types.RuntimeTargetKindProcess, PID: 6565, Source: "user_explicit", Confidence: 1,
				}},
			},
		},
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "cust_runnable.systrace", Carrier: "request_path",
			}},
			RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, ArtifactFiles: 1},
		}),
	}
	return bus, mut
}

// TestEmitInvestigationComplete_FocusChainGapNoteSoftAndNonBlocking pins the
// positive state end to end: completion is ACCEPTED (soft advisory, no
// denial) and the summary carries the focus-gap note with the via_thread
// connector to the runnable anchor 49706.
func TestEmitInvestigationComplete_FocusChainGapNoteSoftAndNonBlocking(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	bus, mut := rn14cBus(t, rn14cDrilldownObservation(
		"OS_FFRT_2_3-49706", "runnable",
		"impact=2528.721ms",
		"recommended_views=scheduler_latency_stats,root_cause_rank,window_stats",
	))
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"FFRT 线程窗口内 runnable 2528ms,同窗占用者已定位",
		"confidence":"high",
		"result_kind":"resolved"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("RN-14c is a soft advisory and must never block completion, got: %s", res.Summary)
	}
	want := "user-focus thread 6565 has no wakeup-chain evidence yet; consider view=wakeup_chain pid=6565 via_thread=49706"
	if !strings.Contains(res.Summary, want) {
		t.Fatalf("accepted completion must carry the focus-gap note %q, got: %s", want, res.Summary)
	}
}

// TestUserFocusWakeupChainGapNote_SilentStates pins the suppression lanes:
// a wakeup-chain observation already targeting the focus thread, a missing
// pinned entity, and a turn without scheduler-shaped trace observations all
// stay silent.
func TestUserFocusWakeupChainGapNote_SilentStates(t *testing.T) {
	anchorObs := rn14cDrilldownObservation("OS_FFRT_2_3-49706", "runnable", "impact=2528.721ms")

	// (1) Chain evidence targeting the focus thread already exists — the
	// wakeup_chain path observation's Subject is the chain target label.
	bus, _ := rn14cBus(t, anchorObs, types.ObservationRecord{
		ID:        "trace_query:w#wakeup_chain:1",
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Subject:   "main-6565",
		Predicate: "wakeup_chain",
		Object:    "path",
	})
	if note := userFocusWakeupChainGapNote(bus); note != "" {
		t.Fatalf("focus-targeting chain observation must silence the note, got %q", note)
	}

	// (2) No pinned focus entity.
	bus, _ = rn14cBus(t, anchorObs)
	bus.AnalysisIR.RequestModel.RuntimeTargets = nil
	if note := userFocusWakeupChainGapNote(bus); note != "" {
		t.Fatalf("no pinned entity must be silent, got %q", note)
	}

	// (3) No scheduler-shaped trace observation this turn.
	bus, _ = rn14cBus(t, types.ObservationRecord{
		ID:        "trace_query:w#event:1",
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Subject:   "frame_42",
		Predicate: "trace_event",
	})
	if note := userFocusWakeupChainGapNote(bus); note != "" {
		t.Fatalf("non-scheduler-shaped turns must be silent, got %q", note)
	}
}

// TestUserFocusWakeupChainGapNote_NoAnchorStillSuggestsFocusChain pins the
// second positive form: scheduler shape without a distinct runnable anchor
// still asks for the focus thread's chain, just without via_thread.
func TestUserFocusWakeupChainGapNote_NoAnchorStillSuggestsFocusChain(t *testing.T) {
	// Sleep-dominant drilldown row: scheduler shape, but not a runnable
	// anchor (Object is the typed state, and it is not runnable).
	bus, _ := rn14cBus(t, rn14cDrilldownObservation("worker-777", "s_sleep", "impact=900.000ms"))
	note := userFocusWakeupChainGapNote(bus)
	want := "user-focus thread 6565 has no wakeup-chain evidence yet; consider view=wakeup_chain pid=6565"
	if note != want {
		t.Fatalf("note = %q, want %q (no via_thread without a runnable anchor)", note, want)
	}
	if strings.Contains(note, "via_thread") {
		t.Fatalf("no runnable anchor must mean no via_thread suffix, got %q", note)
	}
}

// rn14cImpactObservation builds a wakeup_causal_impact row in the PRODUCTION
// wire shape: traceQueryTypedCausalImpactRichNotes rides the zero-drop
// traceQueryTypedCount, so a depth-0 (chain target self impact) row carries
// NO depth= note at all — a literal depth=0 never reaches the wire.
func rn14cImpactObservation(subject, dominantState string, notes ...string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:        "trace_query:w#wakeup_causal_impact:1",
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Subject:   subject,
		Predicate: "wakeup_causal_impact",
		Object:    dominantState,
		RichNotes: append([]string{"causality=on_wakeup_chain", "dominant_state=" + dominantState}, notes...),
	}
}

// TestUserFocusWakeupChainGapNote_FlatChainRunSilences pins the F-1 dead-lane
// repair (统一复核 2026-07-04): a zero-edge (flat) chain run for the focus
// thread emits ONLY a depth-0 wakeup_causal_impact row — no path observation,
// no aggregate/edge rows, and (zero-drop) no depth= note. The absence of the
// depth= note IS the depth-0 signal, so the row targets the focus thread and
// the advisory stays silent instead of re-recommending the chain that already
// ran (the RN-11-adjudicated redundant suggestion).
func TestUserFocusWakeupChainGapNote_FlatChainRunSilences(t *testing.T) {
	bus, _ := rn14cBus(t,
		rn14cDrilldownObservation("OS_FFRT_2_3-49706", "runnable", "impact=2528.721ms"),
		rn14cImpactObservation("main-6565", "s_sleep", "impact=1200.000ms"))
	if note := userFocusWakeupChainGapNote(bus); note != "" {
		t.Fatalf("a flat (depth-0, no depth= note) focus impact row must silence the advisory, got %q", note)
	}
}

// TestUserFocusWakeupChainGapNote_DepthNotePresentIsNotDepthZero pins the
// other half of the absence contract: a depth ≥ 1 impact row (depth= note
// PRESENT) is a dependency node, not the chain target — its Subject must not
// be read as the focus target, and it must not be picked as a runnable
// anchor via the depth-0 lane.
func TestUserFocusWakeupChainGapNote_DepthNotePresentIsNotDepthZero(t *testing.T) {
	// depth=2 dependency row whose Subject happens to be the focus thread
	// label: NOT a target carrier (production dependency rows carry no path
	// note either), so the advisory still fires.
	bus, _ := rn14cBus(t, rn14cImpactObservation("main-6565", "s_sleep", "depth=2", "impact=300.000ms"))
	note := userFocusWakeupChainGapNote(bus)
	if note == "" || !strings.Contains(note, "consider view=wakeup_chain pid=6565") {
		t.Fatalf("depth-note-present dependency row must not count as focus chain evidence, got %q", note)
	}
	if strings.Contains(note, "via_thread") {
		t.Fatalf("focus-labelled dependency row must never become its own via_thread anchor, got %q", note)
	}
	// Runnable-dominant depth-0 impact row (no depth= note) of ANOTHER thread
	// is the via_thread anchor lane — previously dead under depth=="0".
	bus, _ = rn14cBus(t, rn14cImpactObservation("OS_FFRT_2_3-49706", "runnable", "impact=2528.721ms"))
	note = userFocusWakeupChainGapNote(bus)
	want := "user-focus thread 6565 has no wakeup-chain evidence yet; consider view=wakeup_chain pid=6565 via_thread=49706 — connect the runnable anchor to the user-focus thread's wakeup chain"
	if note != want {
		t.Fatalf("depth-0 runnable impact row must drive the via_thread anchor:\n got %q\nwant %q", note, want)
	}
	// The same runnable row WITH a depth note is a dependency hop, never the
	// depth-0 anchor.
	bus, _ = rn14cBus(t, rn14cImpactObservation("OS_FFRT_2_3-49706", "runnable", "depth=3", "impact=2528.721ms"))
	if note = userFocusWakeupChainGapNote(bus); strings.Contains(note, "via_thread") {
		t.Fatalf("depth≥1 runnable impact row must not anchor via_thread, got %q", note)
	}
}

// TestUserFocusWakeupChainGapNote_AggregatePathTargetStillRecognized keeps the
// existing with-edge lanes pinned beside the new absence lane: an aggregate
// row's typed path note tail targeting the focus thread silences the note.
func TestUserFocusWakeupChainGapNote_AggregatePathTargetStillRecognized(t *testing.T) {
	aggregate := types.ObservationRecord{
		ID:        "trace_query:w#wakeup_causal_aggregate:1",
		Origin:    types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:  "trace_query",
		Subject:   "worker-777",
		Predicate: "wakeup_causal_aggregate",
		Object:    "s_sleep",
		RichNotes: []string{"depth=2", "path=worker-777 -> mid-88 -> main-6565", "occurrences=4"},
	}
	bus, _ := rn14cBus(t, aggregate)
	if note := userFocusWakeupChainGapNote(bus); note != "" {
		t.Fatalf("aggregate path tail targeting the focus thread must stay silent, got %q", note)
	}
}
