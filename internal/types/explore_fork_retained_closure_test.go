package types

import "testing"

// explore_fork_retained_closure_test.go — §40.43 F-orch 四轮复核 finding W.
//
// MergeExploreFork wrote the retained closure fields (completion reason,
// result kind, absence justification — and the retained aggregate facts /
// relation claims) back from EVERY merged fork, outside the
// investigationComplete branch, so a non-completing sibling merged after the
// completing fork reverted the retained lane to its fork-time (window-scoped
// probe) copy. After the next backtrack's ResetInvestigationComplete every
// Stable* consumer and the exhaustion release proceeded from that stale copy.
//
// Ruling: the retained lane is the most recently ACCEPTED terminal state —
// MergeExploreFork writes it back only from a fork whose completion
// generation advanced past the parent's at fork time (it recorded an accepted
// completion of its own), never from a non-completing fork.

// PIN (red on 64ceb5b06): the fork-order scenario. A prior accepted closure
// is retained, the window is reset by a backtrack, two forks are taken; the
// first completes with a NEW closure, the second decides nothing. Merged
// completing-first, the retained lane must hold the new closure — and keep
// it across the next reset.
func TestMergeExploreFork_NonCompletingSiblingNeverRevertsRetainedClosure(t *testing.T) {
	for _, order := range []string{"completing fork merges first", "non-completing fork merges first"} {
		t.Run(order, func(t *testing.T) {
			parent := NewMutableState("retained closure fork order")
			parent.SetInvestigationResultKind("resolved")
			parent.SetInvestigationComplete("first accepted closure")
			parent.SetInvestigationAggregateFacts([]AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Label: "members", Value: "1"}})
			parent.RetainInvestigationAggregateFacts()
			// Backtrack: the window is reset, the retained lane survives.
			parent.ResetInvestigationComplete()
			if parent.StableInvestigationCompleteReason() != "first accepted closure" || parent.StableInvestigationResultKind() != "resolved" {
				t.Fatal("fixture: the first closure must be retained across the reset")
			}

			completing := parent.ForkForExploreDispatch()
			probe := parent.ForkForExploreDispatch()
			completing.SetInvestigationResultKind("absence")
			completing.SetAbsenceJustification("nothing registers it")
			completing.SetInvestigationAggregateFacts([]AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Label: "members", Value: "0"}})
			completing.SetInvestigationComplete("second accepted closure")
			completing.RetainInvestigationAggregateFacts()
			// The probe fork reads and emits evidence but decides nothing.
			if probe.InvestigationCompleteGeneration() != parent.InvestigationCompleteGeneration() {
				t.Fatal("fixture: the probe fork inherits the parent's generation")
			}

			if order == "completing fork merges first" {
				parent.MergeExploreFork(completing)
				parent.MergeExploreFork(probe)
			} else {
				parent.MergeExploreFork(probe)
				parent.MergeExploreFork(completing)
			}
			assertRetained := func(stage string) {
				t.Helper()
				if got := parent.StableInvestigationCompleteReason(); got != "second accepted closure" {
					t.Fatalf("%s: retained closure reason = %q, want the later-accepted closure (a non-completing fork must never write the retained lane back)", stage, got)
				}
				if got := parent.StableInvestigationResultKind(); got != "absence" {
					t.Fatalf("%s: retained result kind = %q, want absence", stage, got)
				}
				if got := parent.StableAbsenceJustification(); got != "nothing registers it" {
					t.Fatalf("%s: retained absence justification = %q", stage, got)
				}
				facts := parent.StableInvestigationAggregateFacts()
				if len(facts) != 1 || facts[0].Value != "0" {
					t.Fatalf("%s: retained aggregate facts = %+v, want the later-accepted count", stage, facts)
				}
			}
			assertRetained("after merge")
			if parent.InvestigationCompleteGeneration() != 2 {
				t.Fatalf("exactly one accepted decision was folded back, generation = %d", parent.InvestigationCompleteGeneration())
			}
			// The next backtrack clears the live lane; the retained lane is
			// what every Stable* consumer and the exhaustion release read.
			parent.ResetInvestigationComplete()
			assertRetained("after the next reset")
			d := parent.RecordExploreBacktrackExhausted("window closed")
			if d.RetainedClosureReason != "second accepted closure" {
				t.Fatalf("the exhaustion release proceeds from the retained closure, got %q", d.RetainedClosureReason)
			}
		})
	}
}

// A fork that decided completion and was then reset inside its own window
// (the requeue-divergence shape of 件B①) still writes its accepted state to
// the retained lane: the gate is the fork's own accepted decision, not the
// live flag at merge time.
func TestMergeExploreFork_DecidedThenResetForkStillWritesRetainedLane(t *testing.T) {
	parent := NewMutableState("decided then reset")
	parent.SetInvestigationComplete("first")
	parent.ResetInvestigationComplete()
	fork := parent.ForkForExploreDispatch()
	fork.SetInvestigationResultKind("absence")
	fork.SetAbsenceJustification("justified zero")
	fork.SetInvestigationComplete("second")
	fork.ResetInvestigationComplete()
	if fork.IsInvestigationComplete() {
		t.Fatal("precondition: the fork merges with the live flag cleared")
	}
	parent.MergeExploreFork(fork)
	if parent.StableInvestigationCompleteReason() != "second" || parent.StableInvestigationResultKind() != "absence" || parent.StableAbsenceJustification() != "justified zero" {
		t.Fatalf("a fork that recorded an accepted completion writes the retained lane even when its window was reset before the merge: reason=%q kind=%q just=%q",
			parent.StableInvestigationCompleteReason(), parent.StableInvestigationResultKind(), parent.StableAbsenceJustification())
	}
}

// An inherited completed flag is not a decision (§40.14 V7-2 复核) and now
// also not a retained-lane write: the parent's later-accepted closure
// survives a fork that merely inherited the earlier flag.
func TestMergeExploreFork_InheritedCompletionDoesNotWriteRetainedLane(t *testing.T) {
	parent := NewMutableState("inherited flag")
	parent.SetInvestigationComplete("first")
	inherited := parent.ForkForExploreDispatch()
	parent.ResetInvestigationComplete()
	parent.SetInvestigationComplete("second")
	parent.MergeExploreFork(inherited)
	if got := parent.StableInvestigationCompleteReason(); got != "second" {
		t.Fatalf("an inherited completion must not overwrite the later-accepted retained closure, got %q", got)
	}
	parent.ResetInvestigationComplete()
	if got := parent.StableInvestigationCompleteReason(); got != "second" {
		t.Fatalf("retained lane after reset = %q, want second", got)
	}
}
