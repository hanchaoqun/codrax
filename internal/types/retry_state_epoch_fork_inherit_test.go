package types

import "testing"

// retry_state_epoch_fork_inherit_test.go — §40.14 V7-2 复核: a fork that only
// INHERITED the parent's completed flag makes no decision; merging it must
// not advance the completion generation (which would lift an armed veto).
func TestMergeExploreForkInheritedCompletionIsNotADecision(t *testing.T) {
	parent := NewMutableState("x")
	parent.SetInvestigationComplete("first")
	if parent.InvestigationCompleteGeneration() != 1 {
		t.Fatalf("generation after one decision = %d", parent.InvestigationCompleteGeneration())
	}
	fork := parent.ForkForExploreDispatch()
	parent.MergeExploreFork(fork)
	if parent.InvestigationCompleteGeneration() != 1 {
		t.Fatalf("an inherited completion must not advance the generation: %d", parent.InvestigationCompleteGeneration())
	}
	deciding := parent.ForkForExploreDispatch()
	deciding.ResetInvestigationComplete()
	deciding.SetInvestigationComplete("fork decided")
	parent.MergeExploreFork(deciding)
	if parent.InvestigationCompleteGeneration() != 2 {
		t.Fatalf("a fork's own decision advances the generation by exactly one: %d", parent.InvestigationCompleteGeneration())
	}
}
