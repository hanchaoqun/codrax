package types

import "testing"

func traceInputAdmissionTerminalResultForTest(code string) ToolResult {
	return ToolResult{
		ToolName: "trace_query",
		Success:  false,
		Repair: &ToolRepair{
			Code:   code,
			Fields: []string{"path"},
			Metadata: map[string]string{
				"stage":  ToolRepairStageTraceInputAdmission,
				"status": ToolRepairStatusActionRequired,
				"path":   "capture.sys",
			},
		},
	}
}

func TestTraceInputAdmissionTerminalSurvivesDispatchReset(t *testing.T) {
	mut := NewMutableState("trace")
	result := traceInputAdmissionTerminalResultForTest("trace_conversion_required")
	if !mut.ArmTraceInputAdmissionTerminal(StageExplore, result) {
		t.Fatal("typed action-required trace admission did not arm")
	}
	mut.AppendDispatchToolResult(result)
	mut.ResetDispatchToolResults()

	repair, ok := mut.TraceInputAdmissionTerminal(StageExplore)
	if !ok || repair.Code != "trace_conversion_required" || repair.Metadata["path"] != "capture.sys" {
		t.Fatalf("run-scoped latch was lost across dispatch reset: ok=%v repair=%+v", ok, repair)
	}
	if _, ok := mut.TraceInputAdmissionTerminal(StageFinalize); ok {
		t.Fatal("explore-stage latch leaked into finalize stage")
	}
}

func TestTraceInputAdmissionTerminalIsTypedAndFirstWins(t *testing.T) {
	mut := NewMutableState("trace")
	ordinary := traceInputAdmissionTerminalResultForTest("trace_view_unsupported")
	ordinary.Repair.Metadata["stage"] = "trace_engine"
	if mut.ArmTraceInputAdmissionTerminal(StageExplore, ordinary) {
		t.Fatal("non-admission repair armed terminal latch")
	}
	wrongStatus := traceInputAdmissionTerminalResultForTest("trace_conversion_required")
	wrongStatus.Repair.Metadata["status"] = ToolRepairStatusAdvisory
	if mut.ArmTraceInputAdmissionTerminal(StageExplore, wrongStatus) {
		t.Fatal("non-action-required repair armed terminal latch")
	}

	first := traceInputAdmissionTerminalResultForTest("trace_conversion_required")
	second := traceInputAdmissionTerminalResultForTest("trace_input_source_unavailable")
	second.Repair.Metadata["path"] = "second.trace"
	if !mut.ArmTraceInputAdmissionTerminal(StageExplore, first) ||
		!mut.ArmTraceInputAdmissionTerminal(StageExplore, second) {
		t.Fatal("eligible admission result was not accepted")
	}
	repair, ok := mut.TraceInputAdmissionTerminal(StageExplore)
	if !ok || repair.Code != first.Repair.Code || repair.Metadata["path"] != "capture.sys" {
		t.Fatalf("first terminal repair must remain authoritative: ok=%v repair=%+v", ok, repair)
	}

	// Returned repair is defensive, including its map/slice children.
	repair.Metadata["path"] = "mutated"
	repair.Fields[0] = "mutated"
	again, _ := mut.TraceInputAdmissionTerminal(StageExplore)
	if again.Metadata["path"] != "capture.sys" || again.Fields[0] != "path" {
		t.Fatalf("latch repair was externally mutated: %+v", again)
	}
}

func TestTraceInputAdmissionTerminalIsSharedAcrossExploreForks(t *testing.T) {
	parent := NewMutableState("trace")
	firstFork := parent.ForkForExploreDispatch()
	secondFork := parent.ForkForExploreDispatch()
	if !firstFork.ArmTraceInputAdmissionTerminal(StageExplore, traceInputAdmissionTerminalResultForTest("trace_input_empty")) {
		t.Fatal("fork did not arm shared terminal latch")
	}
	for name, mut := range map[string]*MutableState{"parent": parent, "sibling": secondFork} {
		if repair, ok := mut.TraceInputAdmissionTerminal(StageExplore); !ok || repair.Code != "trace_input_empty" {
			t.Fatalf("%s did not observe shared run latch: ok=%v repair=%+v", name, ok, repair)
		}
	}
}
