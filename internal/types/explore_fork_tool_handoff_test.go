package types

import "testing"

func TestMergeExploreForkPublishedToolsDoesNotCompleteOrAcceptModelState(t *testing.T) {
	parent := NewMutableState("typed observations are not completion")
	fork := parent.ForkForExploreDispatch()
	fork.SetInvestigationComplete("must not be accepted")
	fork.SetInvestigationAggregateFacts([]AnswerAggregateFact{{Label: "model conclusion", Value: "999"}})
	fork.AppendDispatchToolResult(ToolResult{ToolName: "exec_command", Success: true, CommandMeasurement: &ToolCommandMeasurement{
		Kind: ToolCommandMeasurementKindCount, Value: 7, Origin: AnswerEvidenceOriginCommandMeasurement,
	}})
	if got := parent.MergeExploreForkPublishedTools(fork); len(got) != 1 {
		t.Fatalf("completed producer data = %+v", got)
	}
	if parent.IsInvestigationComplete() || parent.StableInvestigationCompleteReason() != "" || len(parent.StableInvestigationAggregateFacts()) != 0 {
		t.Fatal("data-only merge accepted sibling's model state")
	}
	if got := parent.MergeExploreForkPublishedTools(fork); len(got) != 0 {
		t.Fatalf("repeated merge duplicated data: %+v", got)
	}
}

func TestMergeExploreForkPublishedToolsPreservesNativeReceipt(t *testing.T) {
	path := writeTraceReadTestFile(t, t.TempDir(), "capture.systrace")
	parent := NewMutableState("same-run producer receipts")
	fork := parent.ForkForExploreDispatch()
	result := nativeTraceSourceReadResult(path)
	ref := fork.PrepareTraceQuerySourceRead(path)
	fork.StampTraceQuerySourceRead(ref, &result)
	fork.AppendDispatchToolResult(result)
	parent.MergeExploreForkPublishedTools(fork)
	if _, ok := parent.ResolveTraceQuerySourceRead(path); !ok {
		t.Fatal("successful native publication lost its same-run receipt")
	}
	parent.ResetTurnAArtifacts()
	parent.MergeExploreForkPublishedTools(fork)
	if _, ok := parent.ResolveTraceQuerySourceRead(path); ok {
		t.Fatal("data-only salvage resurrected an expired source-read receipt")
	}
}
