package types

import "testing"

// explore_backtrack_exhausted_test.go — §40.43 F-orch 三轮复核 finding Q.
// The scheduler's typed ExploreBacktrackExhausted decision is a
// completion-generation advance: it consumes the bound explore-backtrack
// veto exactly like a fresh accepted completion, records the retained
// closure it proceeds from, and never touches the live completion flag.

func TestMutableState_RecordExploreBacktrackExhausted_AdvancesGenerationAndReleasesBinding(t *testing.T) {
	mut := NewMutableState("exhaustion decision")
	mut.SetInvestigationComplete("first accepted closure")
	mut.SetRetryState(explorePinRetryState())
	mut.ResetForFallback(FallbackResetTargetExplore)
	mut.ResetInvestigationComplete()

	rs := mut.RetryState()
	if rs == nil || rs.ExploreBacktrackEpoch != 1 || rs.CompletionGenerationAtBacktrack != 1 {
		t.Fatalf("fixture: the backtrack must bind the retry state to epoch 1 / generation 1, got %+v", rs)
	}
	if mut.InvestigationCompleteGeneration() != 1 || mut.ExploreBacktrackExhaustedDecisions() != 0 || mut.LastExploreBacktrackExhausted() != nil {
		t.Fatal("fixture: no decision recorded yet")
	}

	d := mut.RecordExploreBacktrackExhausted("window closed")
	if d.Epoch != 1 || d.GenerationBefore != 1 || d.GenerationAfter != 2 || d.Reason != "window closed" || d.RetainedClosureReason != "first accepted closure" {
		t.Fatalf("decision must record epoch, generation advance, reason and the retained closure, got %+v", d)
	}
	if got := mut.InvestigationCompleteGeneration(); got != 2 {
		t.Fatalf("the decision advances the completion generation (release through the same comparison as a fresh completion), got %d", got)
	}
	if rs := mut.RetryState(); rs == nil || rs.CompletionGenerationAtBacktrack != 1 {
		t.Fatalf("the bound retry state is untouched (the release is a generation advance, not a clear), got %+v", rs)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("the decision never sets the live completion flag — the run proceeds from the RETAINED closure")
	}
	if mut.StableInvestigationCompleteReason() != "first accepted closure" {
		t.Fatal("the retained closure reason survives the decision")
	}
	if n := mut.ExploreBacktrackExhaustedDecisions(); n != 1 {
		t.Fatalf("decisions counter = %d, want 1", n)
	}
	last := mut.LastExploreBacktrackExhausted()
	if last == nil || *last != d {
		t.Fatalf("LastExploreBacktrackExhausted = %+v, want %+v", last, d)
	}
	last.Reason = "mutated copy"
	if mut.LastExploreBacktrackExhausted().Reason != "window closed" {
		t.Fatal("LastExploreBacktrackExhausted must hand out a copy")
	}

	// A second backtrack re-binds to the new epoch / generation and a second
	// decision consumes it again: one backtrack, one release.
	mut.SetRetryState(explorePinRetryState())
	mut.ResetForFallback(FallbackResetTargetExplore)
	if rs := mut.RetryState(); rs.ExploreBacktrackEpoch != 2 || rs.CompletionGenerationAtBacktrack != 2 {
		t.Fatalf("second backtrack binds epoch 2 / generation 2, got %+v", rs)
	}
	d2 := mut.RecordExploreBacktrackExhausted("")
	if d2.Epoch != 2 || d2.GenerationBefore != 2 || d2.GenerationAfter != 3 || mut.ExploreBacktrackExhaustedDecisions() != 2 {
		t.Fatalf("second decision = %+v (count=%d)", d2, mut.ExploreBacktrackExhaustedDecisions())
	}

	// Fork bookkeeping: the fork inherits the counter; merging a fork that
	// made no decision of its own advances nothing.
	fork := mut.ForkForExploreDispatch()
	if fork.ExploreBacktrackExhaustedDecisions() != 2 || fork.InvestigationCompleteGeneration() != 3 {
		t.Fatalf("fork must inherit the decision counter and generation, got %d / %d", fork.ExploreBacktrackExhaustedDecisions(), fork.InvestigationCompleteGeneration())
	}
	mut.MergeExploreFork(fork)
	if mut.InvestigationCompleteGeneration() != 3 {
		t.Fatalf("merging a fork without its own decision must not advance the generation, got %d", mut.InvestigationCompleteGeneration())
	}

	var nilState *MutableState
	if d := nilState.RecordExploreBacktrackExhausted("x"); d != (ExploreBacktrackExhaustedDecision{}) || nilState.ExploreBacktrackExhaustedDecisions() != 0 || nilState.LastExploreBacktrackExhausted() != nil {
		t.Fatal("nil receiver is inert")
	}
}
