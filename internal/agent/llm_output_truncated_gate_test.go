package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// PIB-3 whole-batch refusal (ledger
// docs/design/pi_borrow_analysis_20260729.md §3.4 item 5, pi borrow:
// agent-loop refuses every tool call when stopReason==length): a
// response cut off by the output-token limit may carry tool calls whose
// arguments parse yet are semantically incomplete — the params repairer
// even completes missing closers. The gate refuses the batch BEFORE
// repair/execution.

// TestExecuteTool_RefusesBatchWhenLLMOutputTruncated pins the gate: the
// flag stamped on the AgentContext short-circuits executeTool into a
// typed non-executed result.
func TestExecuteTool_RefusesBatchWhenLLMOutputTruncated(t *testing.T) {
	b := &BaseAgent{name: "test-agent"}
	ctx := &types.AgentContext{LLMOutputTruncatedBatch: true}
	tc := llm.ToolCall{ID: "tc-1", Name: "read_file", Params: []byte(`{"path":"main.go"}`)}

	result, mcp := b.executeTool(ctx, tc)
	if mcp != nil {
		t.Fatal("refused call must not produce an MCP response")
	}
	if result == nil || result.Success {
		t.Fatalf("refused call must return a failed result; got %+v", result)
	}
	if result.Repair == nil || result.Repair.Code != toolRepairCodeLLMOutputTruncated {
		t.Fatalf("refusal must carry the llm_output_truncated repair code; got %+v", result.Repair)
	}
	if !strings.Contains(result.Summary, "not executed") ||
		!strings.Contains(result.Summary, "output-token limit") {
		t.Errorf("refusal summary must state non-execution and the cause: %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "Re-issue") {
		t.Errorf("refusal summary must give the recovery move: %q", result.Summary)
	}
}

// TestExecuteTool_TruncatedFlagFalseDoesNotInterfere pins the negative
// arm: with the flag false (the overwhelming common case) a call with
// malformed params must take the existing malformed-params lane, NOT
// the truncation refusal — proving the gate is inert outside a
// length-stopped round.
func TestExecuteTool_TruncatedFlagFalseDoesNotInterfere(t *testing.T) {
	b := &BaseAgent{name: "test-agent"}
	ctx := &types.AgentContext{LLMOutputTruncatedBatch: false}
	tc := llm.ToolCall{ID: "tc-2", Name: "read_file", Params: []byte(`{"path": "unterminated`)}

	result, _ := b.executeTool(ctx, tc)
	if result == nil || result.Repair == nil {
		t.Fatalf("malformed params must return a typed repair result; got %+v", result)
	}
	if result.Repair.Code == toolRepairCodeLLMOutputTruncated {
		t.Fatalf("flag=false must never take the truncation-refusal lane; got %+v", result.Repair)
	}
	if result.Repair.Code != toolRepairCodeMalformedToolParams {
		t.Fatalf("expected the malformed-params lane; got %q", result.Repair.Code)
	}
}
