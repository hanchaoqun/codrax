package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_decision_telemetry_test.go — R8 integration tests:
// confirm the 4 instrumented decision sites populate the
// AnalyzerDecisionSignal channel on Mutable.

// TestPrescanRejected_AppendsDecision pins the prescan budget
// rejection wiring: when validateAnalyzerPrescanToolCall fires
// rejectAnalyzerPrescanTool, the decision lands on Mutable.
func TestPrescanRejected_AppendsDecision(t *testing.T) {
	mut := &types.MutableState{}
	mut.SetPrescanRoundLimit(2)
	// Bump prescan round count to the limit so the rejection fires.
	// AppendPrescanSummary remains the compact test-only path that
	// increments the counter without building a full typed tool result.
	mut.AppendPrescanSummary("round1 summary")
	mut.AppendPrescanSummary("round2 summary")

	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
	}
	tc := llm.ToolCall{Name: "grep", Params: json.RawMessage(`{"files_only":true}`)}
	res := validateAnalyzerPrescanToolCall(ctx, tc)
	if res == nil {
		t.Fatal("expected rejection (prescan budget exhausted)")
	}
	sigs := mut.AnalyzerDecisions()
	if len(sigs) != 1 {
		t.Fatalf("expected one AnalyzerDecisionSignal, got %d", len(sigs))
	}
	if sigs[0].Kind != "prescan_rejected" {
		t.Errorf("kind = %q, want prescan_rejected", sigs[0].Kind)
	}
	if sigs[0].Stage != string(types.StageAnalyze) {
		t.Errorf("stage = %q, want analyze", sigs[0].Stage)
	}
	if !strings.Contains(sigs[0].Reason, "pre-scan budget") {
		t.Errorf("reason should mention budget; got %q", sigs[0].Reason)
	}
	if sigs[0].Detail != "grep" {
		t.Errorf("detail = %q, want grep (the tool name)", sigs[0].Detail)
	}
}

// TestPrescanRejected_TerminalEmitMode_AppendsDecision pins the
// retry path:when EmitStageRetryAttempt > 0 and the LLM still
// calls a prescan tool, the rejection is recorded.
func TestPrescanRejected_TerminalEmitMode_AppendsDecision(t *testing.T) {
	mut := &types.MutableState{}
	ctx := &types.AgentContext{
		Stage:                 types.StageAnalyze,
		Mutable:               mut,
		EmitStageRetryAttempt: 1,
	}
	tc := llm.ToolCall{Name: "repo_map", Params: json.RawMessage(`{}`)}
	res := validateAnalyzerPrescanToolCall(ctx, tc)
	if res == nil {
		t.Fatal("expected rejection on retry")
	}
	sigs := mut.AnalyzerDecisions()
	if len(sigs) != 1 || sigs[0].Kind != "prescan_rejected" {
		t.Errorf("expected prescan_rejected signal, got %+v", sigs)
	}
	if !strings.Contains(sigs[0].Reason, "terminal emit mode") {
		t.Errorf("reason should mention terminal emit mode; got %q", sigs[0].Reason)
	}
}
