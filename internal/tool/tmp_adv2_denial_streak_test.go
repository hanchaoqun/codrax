package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Adversarial review #2 empirical probe. NOT to be committed.
func newAdv2DeniedBus() *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("分析这一帧丢帧的根因"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceDefault,
					Confidence:        0.9,
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
			},
		},
		AttachedHitrace: "42196.879790: sched_switch: prev_comm=UIThread prev_state=S next_comm=RSUniRenderThre",
	}
}

var adv2Params = json.RawMessage(`{
	"reason":"the frame dropped because the UI thread slept",
	"confidence":"high",
	"result_kind":"resolved"
}`)

// Scenario 2 of the finding: 2 citation denials + 1 interleaved
// non-citation (pending-read) denial + 1 more citation denial —
// finding claims the breaker fires there. Probe whether it does.
func TestAdv2_InterleavedDenialArithmetic(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	bus := newAdv2DeniedBus()
	tool := &EmitInvestigationComplete{}

	for i := 1; i <= 2; i++ {
		res, err := tool.Execute(bus, adv2Params)
		if err != nil {
			t.Fatalf("attempt %d err: %v", i, err)
		}
		if !strings.Contains(res.Summary, "citation preflight") {
			t.Fatalf("attempt %d expected citation denial, got: %s", i, res.Summary)
		}
	}

	// Interleave: raise a pending forced read so the next attempt is denied
	// by the pending-read gate (line ~2915), which does NOT record a streak.
	closure := bus.Mutable.EvidenceClosure()
	closure.AddPendingRead(types.PendingRead{File: "internal/render/frame.go", Rationale: "primary anchor unread", Origin: "primary_anchor_unread"})
	res, err := tool.Execute(bus, adv2Params)
	if err != nil {
		t.Fatalf("interleave attempt err: %v", err)
	}
	t.Logf("interleave attempt summary kind: pendingDenial=%v citationDenial=%v accepted=%v",
		strings.Contains(res.Summary, "pending forced reads"),
		strings.Contains(res.Summary, "citation preflight"),
		res.Success)
	interleaveWasPending := strings.Contains(res.Summary, "pending forced reads")

	// Clear the pending read WITHOUT reading (zero progress), as the
	// finding's scenario requires, then re-attempt.
	closure.ClearPendingReadsMatching(types.PendingRead{File: "internal/render/frame.go", Rationale: "primary anchor unread", Origin: "primary_anchor_unread"})

	res, err = tool.Execute(bus, adv2Params)
	if err != nil {
		t.Fatalf("post-interleave attempt err: %v", err)
	}
	if interleaveWasPending {
		// This is the finding's exact failure input: c1, c2, pending, c3.
		// Finding claims breaker fires here. Check.
		if res.Success {
			t.Fatalf("FINDING CONFIRMED: breaker fired after only 3 citation denials (interleaved), summary: %s", res.Summary)
		}
		t.Logf("finding's scenario-2 failure input REFUTED: 3rd citation denial after interleave still DENIED: %s", res.Summary)
		// And the 4th recorded citation denial fires — identical to the
		// no-interleave baseline (pinned test).
		res, err = tool.Execute(bus, adv2Params)
		if err != nil {
			t.Fatalf("4th citation attempt err: %v", err)
		}
		if !res.Success {
			t.Fatalf("4th identical citation denial should be breaker-accepted, got: %s", res.Summary)
		}
		t.Logf("4th recorded citation denial breaker-accepted (same as baseline): %s", res.Summary)
	} else {
		t.Logf("pending read did not produce a pending-denial (classified advisory or bypassed); interleave scenario not constructible this way; summary: %s", res.Summary)
	}
}

// Scenario 1 of the finding: 3 citation denials -> acceptance via
// per-call absence justification -> reopen (ResetInvestigationComplete,
// as the explore retry window does) -> first identical citation denial
// of the "reopened wall". Finding claims breaker fires immediately.
func TestAdv2_ReopenAfterAcceptance(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	bus := newAdv2DeniedBus()
	tool := &EmitInvestigationComplete{}

	for i := 1; i <= 3; i++ {
		res, err := tool.Execute(bus, adv2Params)
		if err != nil {
			t.Fatalf("attempt %d err: %v", i, err)
		}
		if !strings.Contains(res.Summary, "citation preflight") {
			t.Fatalf("attempt %d expected citation denial, got: %s", i, res.Summary)
		}
	}

	// Acceptance attempt via absence justification (per-call payload; the
	// only acceptance route that can lapse on the next attempt without any
	// counter change).
	absenceParams := json.RawMessage(`{
		"reason":"no in-repo source implements the frame scheduler; the trace rows are the only observable",
		"confidence":"medium",
		"result_kind":"absence",
		"absence_justification":"grep across the repository found no frame scheduler implementation; the repository contains no current-source files relevant to the dropped frame"
	}`)
	res, err := tool.Execute(bus, absenceParams)
	if err != nil {
		t.Fatalf("absence attempt err: %v", err)
	}
	t.Logf("absence attempt: success=%v summary=%s", res.Success, truncAdv2(res.Summary, 300))
	if !res.Success {
		t.Skipf("absence acceptance route not reachable in this fixture; reopen scenario not constructible this way")
	}

	// Reopen exactly as orchestrator.go:5205 does on an explore retry window.
	bus.Mutable.ResetInvestigationComplete()

	// First citation-denial-eligible attempt of the reopened wall, with zero
	// progress (no reads, no new evidence) => identical fingerprint.
	res, err = tool.Execute(bus, adv2Params)
	if err != nil {
		t.Fatalf("reopened attempt err: %v", err)
	}
	if res.Success {
		t.Logf("FINDING MECHANIC CONFIRMED: reopened wall accepted on FIRST denial-eligible attempt (streak carried over): %s", truncAdv2(res.Summary, 300))
	} else {
		t.Logf("reopened wall first attempt DENIED (streak did not carry / fingerprint differs): %s", truncAdv2(res.Summary, 300))
	}
}

func truncAdv2(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return fmt.Sprintf("%s...", s[:n])
}
