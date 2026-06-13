package types

import "testing"

func TestMutableState_TraceQueryRuntimeObservationCountSurvivesDispatchReset(t *testing.T) {
	mut := NewMutableState("trace runtime observation")
	mut.AppendDispatchToolResult(traceQueryRuntimeObservationToolResultForTest())
	if got := mut.TraceQueryRuntimeObservationCount(); got != 1 {
		t.Fatalf("count after append = %d, want 1", got)
	}

	mut.ResetDispatchToolResults()
	if got := mut.TraceQueryRuntimeObservationCount(); got != 1 {
		t.Fatalf("count after dispatch reset = %d, want 1", got)
	}

	mut.ResetTurnAArtifacts()
	if got := mut.TraceQueryRuntimeObservationCount(); got != 0 {
		t.Fatalf("count after TurnA reset = %d, want 0", got)
	}
}

func TestMutableState_TraceQueryRuntimeObservationCountMergesExploreForkDelta(t *testing.T) {
	parent := NewMutableState("trace runtime observation")
	parent.AppendDispatchToolResult(traceQueryRuntimeObservationToolResultForTest())

	fork := parent.ForkForExploreDispatch()
	fork.AppendDispatchToolResult(traceQueryRuntimeObservationToolResultForTest())
	parent.MergeExploreFork(fork)

	if got := parent.TraceQueryRuntimeObservationCount(); got != 2 {
		t.Fatalf("merged count = %d, want 2", got)
	}
}

func traceQueryRuntimeObservationToolResultForTest() ToolResult {
	return ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []ObservationRecord{{
			ID:              "trace_query:window#root_cause_rank:1",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact},
			Subject:         "CookieMonsterCl-59843",
			Predicate:       "root_cause_primary",
			Object:          "runnable",
		}},
	}
}
