package types

import "testing"

func TestPreReadSourceSurvivesExploreForkMergeAndClearsAtTaskBoundary(t *testing.T) {
	parent := NewMutableState("q")
	fork := parent.ForkForExploreDispatch()
	fork.RecordPreReadSource("src/runner.py", []string{"line one", "line two"})
	parent.MergeExploreFork(fork)

	_, _, _, lines, _, _, _, revision := parent.GroundingContextSnapshot()
	if got := lines["src/runner.py"][2]; got != "line two" {
		t.Fatalf("fork pre-read bytes did not merge to parent: %q", got)
	}
	if revision == 0 {
		t.Fatal("merged pre-read source must advance grounding cache revision")
	}

	// Snapshot callers own their copy.
	lines["src/runner.py"][2] = "mutated"
	_, _, _, fresh, _, _, _, _ := parent.GroundingContextSnapshot()
	if got := fresh["src/runner.py"][2]; got != "line two" {
		t.Fatalf("snapshot mutation leaked into mutable state: %q", got)
	}

	parent.ResetTurnAArtifacts()
	_, _, _, cleared, _, _, _, _ := parent.GroundingContextSnapshot()
	if len(cleared) != 0 {
		t.Fatalf("task boundary retained stale pre-read source: %+v", cleared)
	}
}
