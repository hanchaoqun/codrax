package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// §40.14 V7-2: the accepted-closure explore-backtrack veto reads a
// cross-generation carrier (RetryState). These pins lock the lifecycle
// half that lives in types: the explore backtrack binds the live
// RetryState to the backtrack epoch + the completion generation observed
// at that moment; other fallback targets never bind; completion
// decisions (direct or fork-merged) advance the generation; resets do
// not.

func explorePinRetryState() *RetryState {
	return &RetryState{
		Attempt:          1,
		LastPrimaryOwner: string(LocusExplore),
		ActiveViolations: []ScoredViolation{{Kind: ViolRequiredDiagramEdgeAbsent, Severity: SeverityHigh}},
	}
}

// ④ Explore backtrack binds; Finalizer / Extract targets leave the state
// unbound and the epoch untouched.
func TestMutableState_ResetForFallback_ExploreBindsRetryStateEpoch(t *testing.T) {
	mut := NewMutableState("epoch binding")
	mut.SetInvestigationComplete("first accepted closure")
	if mut.RetryState() != nil {
		t.Fatal("fresh state must carry no retry state")
	}
	rs := explorePinRetryState()
	mut.SetRetryState(rs)
	mut.SetRepairExecutionPlan(struct{ ordered []string }{ordered: []string{"explore"}})

	cleared := mut.ResetForFallback(FallbackResetTargetExplore)

	if got := mut.ExploreBacktrackEpoch(); got != 1 {
		t.Fatalf("explore backtrack must open epoch 1, got %d", got)
	}
	bound := mut.RetryState()
	if bound == nil {
		t.Fatal("explore backtrack must keep the retry state for prompt rendering")
	}
	if bound == rs {
		t.Fatal("binding must copy-on-write: RetryState() hands out the live pointer, so the stamped state must be a fresh value")
	}
	if rs.ExploreBacktrackEpoch != 0 || rs.CompletionGenerationAtBacktrack != 0 {
		t.Fatalf("the caller's original value must not be mutated in place, got epoch=%d gen=%d", rs.ExploreBacktrackEpoch, rs.CompletionGenerationAtBacktrack)
	}
	if bound.ExploreBacktrackEpoch != 1 {
		t.Fatalf("bound state must carry the opened epoch, got %d", bound.ExploreBacktrackEpoch)
	}
	if bound.CompletionGenerationAtBacktrack != mut.InvestigationCompleteGeneration() || bound.CompletionGenerationAtBacktrack != 1 {
		t.Fatalf("bound state must snapshot the live completion generation (1), got %d (live %d)",
			bound.CompletionGenerationAtBacktrack, mut.InvestigationCompleteGeneration())
	}
	if bound.LastPrimaryOwner != rs.LastPrimaryOwner || len(bound.ActiveViolations) != len(rs.ActiveViolations) || bound.Attempt != rs.Attempt {
		t.Fatal("binding must preserve every populated field of the retry state")
	}
	// EVOLUTION RECORD (§40.43 F12 fold-in): previously "explore backtrack
	// must still clear the repair execution plan". The plan persists across
	// every fallback target so cluster closure can classify the next
	// failure against it; ResetRetryState at chain close is its reset.
	if mut.RepairExecutionPlan() == nil {
		t.Fatal("explore backtrack must preserve the repair execution plan")
	}
	if epochPinContains(cleared, "RepairExecutionPlan") {
		t.Fatalf("cleared list must not report RepairExecutionPlan: %v", cleared)
	}
	if epochPinContains(cleared, "RetryState") {
		t.Fatalf("binding is not a clear; cleared list must not report RetryState: %v", cleared)
	}

	// A second backtrack re-binds the (re-populated) state to epoch 2.
	mut.SetRetryState(explorePinRetryState())
	mut.ResetForFallback(FallbackResetTargetExplore)
	if got := mut.ExploreBacktrackEpoch(); got != 2 {
		t.Fatalf("second explore backtrack must open epoch 2, got %d", got)
	}
	if got := mut.RetryState().ExploreBacktrackEpoch; got != 2 {
		t.Fatalf("re-populated state must be bound to epoch 2, got %d", got)
	}

	for _, target := range []FallbackResetTarget{FallbackResetTargetFinalizer, FallbackResetTargetExtract} {
		other := NewMutableState("non-explore target")
		other.SetInvestigationComplete("accepted")
		other.SetRetryState(explorePinRetryState())
		other.ResetForFallback(target)
		if other.ExploreBacktrackEpoch() != 0 {
			t.Fatalf("%v must not open an explore epoch, got %d", target, other.ExploreBacktrackEpoch())
		}
		got := other.RetryState()
		if got == nil || got.ExploreBacktrackEpoch != 0 || got.CompletionGenerationAtBacktrack != 0 {
			t.Fatalf("%v must leave the retry state unbound, got %+v", target, got)
		}
	}

	// Binding with no retry state present only opens the epoch.
	empty := NewMutableState("no retry state")
	empty.ResetForFallback(FallbackResetTargetExplore)
	if empty.ExploreBacktrackEpoch() != 1 || empty.RetryState() != nil {
		t.Fatalf("binding without a retry state must only advance the epoch, got epoch=%d rs=%v", empty.ExploreBacktrackEpoch(), empty.RetryState())
	}

	// nil receiver safety.
	if (*MutableState)(nil).ExploreBacktrackEpoch() != 0 || (*MutableState)(nil).InvestigationCompleteGeneration() != 0 {
		t.Fatal("nil receiver accessors must return 0")
	}
}

// ⑤ Completion decisions advance the generation; resets never do; a
// completed fork advances the parent by exactly one on merge; forks
// inherit the parent's epoch and generation.
func TestMutableState_InvestigationCompleteGeneration_DecisionsAndForkMerge(t *testing.T) {
	mut := NewMutableState("completion generation")
	if got := mut.InvestigationCompleteGeneration(); got != 0 {
		t.Fatalf("fresh state must start at generation 0, got %d", got)
	}
	mut.SetInvestigationComplete("first")
	if got := mut.InvestigationCompleteGeneration(); got != 1 {
		t.Fatalf("first completion must advance to 1, got %d", got)
	}
	mut.ResetInvestigationComplete()
	if got := mut.InvestigationCompleteGeneration(); got != 1 {
		t.Fatalf("a completion reset is not a decision; generation must stay 1, got %d", got)
	}
	mut.SetInvestigationComplete("second, after reset")
	if got := mut.InvestigationCompleteGeneration(); got != 2 {
		t.Fatalf("re-decision must advance to 2, got %d", got)
	}

	mut.SetRetryState(explorePinRetryState())
	mut.ResetForFallback(FallbackResetTargetExplore)
	mut.ResetInvestigationComplete()

	fork := mut.ForkForExploreDispatch()
	if fork.ExploreBacktrackEpoch() != mut.ExploreBacktrackEpoch() || fork.InvestigationCompleteGeneration() != mut.InvestigationCompleteGeneration() {
		t.Fatalf("fork must inherit epoch/generation (%d/%d), got %d/%d",
			mut.ExploreBacktrackEpoch(), mut.InvestigationCompleteGeneration(),
			fork.ExploreBacktrackEpoch(), fork.InvestigationCompleteGeneration())
	}
	if fork.RetryState() != nil {
		t.Fatal("forks never see the parent's retry state (unchanged contract)")
	}

	// An incomplete fork merge is not a decision.
	idle := mut.ForkForExploreDispatch()
	mut.MergeExploreFork(idle)
	if got := mut.InvestigationCompleteGeneration(); got != 2 {
		t.Fatalf("merging an incomplete fork must not advance the generation, got %d", got)
	}

	fork.SetInvestigationComplete("fork completed the window")
	mut.MergeExploreFork(fork)
	if got := mut.InvestigationCompleteGeneration(); got != 3 {
		t.Fatalf("merging a completed fork must advance the parent by exactly 1 (to 3), got %d", got)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatal("merged completion must fold back")
	}
}

// ⑥ Prompt-face guard: the binding fields are system-internal and never
// reach a JSON snapshot (mirrors the PrevEmitSystemBlockKinds json:"-"
// discipline).
func TestRetryState_EpochBindingFieldsNeverMarshal(t *testing.T) {
	raw, err := json.Marshal(RetryState{
		Attempt:                         2,
		ExploreBacktrackEpoch:           3,
		CompletionGenerationAtBacktrack: 4,
		LastPrimaryOwner:                string(LocusExplore),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ExploreBacktrackEpoch", "explore_backtrack_epoch", "CompletionGenerationAtBacktrack", "completion_generation_at_backtrack"} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("binding field %q leaked into the JSON snapshot: %s", key, raw)
		}
	}
	var back RetryState
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.ExploreBacktrackEpoch != 0 || back.CompletionGenerationAtBacktrack != 0 {
		t.Fatal("JSON can never author the binding")
	}
}

func epochPinContains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
