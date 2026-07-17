package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func armTraceQueryTerminalAdmissionForTest(t *testing.T, mutable *types.MutableState, code string, success bool) {
	t.Helper()
	result := types.ToolResult{
		ToolName: "trace_query",
		Success:  success,
		Summary:  "typed trace input admission failure",
		Repair: &types.ToolRepair{
			Code:   code,
			Hint:   "repair the trace input outside the investigation",
			Fields: []string{"path"},
			Metadata: map[string]string{
				"path":   "capture.sys",
				"stage":  types.ToolRepairStageTraceInputAdmission,
				"status": types.ToolRepairStatusActionRequired,
			},
		},
	}
	mutable.AppendDispatchToolResult(result)
	wantArmed := !success
	if got := mutable.ArmTraceInputAdmissionTerminal(types.StageExplore, result); got != wantArmed {
		t.Fatalf("terminal arm=%v, want %v for success=%v", got, wantArmed, success)
	}
}

func TestTraceQueryTerminalAdmissionRepairBlocksExplorerFallbackMatrix(t *testing.T) {
	codes := []string{
		tracequery.TraceInputAdmissionCodeConversionRequired,
		tracequery.TraceInputAdmissionCodeTextExportRequired,
		tracequery.TraceInputAdmissionCodeEmpty,
		tracequery.TraceInputAdmissionCodeLineTooLong,
		tracequery.TraceInputAdmissionCodeSourceUnavailable,
	}
	tools := []llm.ToolCall{
		{Name: "read_file", Params: json.RawMessage(`{"path":"capture.sys"}`)},
		{Name: "grep", Params: json.RawMessage(`{"path":"capture.sys","pattern":"sched_switch"}`)},
		{Name: "exec_command", Params: json.RawMessage(`{"cmd":"inspect capture.sys"}`)},
		{Name: "repo_map", Params: json.RawMessage(`{}`)},
		{Name: "list_files", Params: json.RawMessage(`{}`)},
		{Name: "trace_query", Params: json.RawMessage(`{"path":"other.systrace","view":"window_stats"}`)},
		{Name: "git_log", Params: json.RawMessage(`{}`)},
		{Name: "emit_evidence", Params: json.RawMessage(`{}`)},
		{Name: "emit_analysis", Params: json.RawMessage(`{}`)},
		{Name: "mcp_read_resource", Params: json.RawMessage(`{}`)},
		{Name: "run_subagent", Params: json.RawMessage(`{}`)},
		{Name: "todo_write", Params: json.RawMessage(`{}`)},
	}

	for _, code := range codes {
		for _, call := range tools {
			t.Run(code+"/"+call.Name, func(t *testing.T) {
				mutable := types.NewMutableState("trace input admission")
				armTraceQueryTerminalAdmissionForTest(t, mutable, code, false)
				ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mutable}

				got := validateExplorerTraceQueryFirstToolCall(ctx, call, true)
				if got == nil || got.Success {
					t.Fatalf("%s must be rejected after terminal admission code %s: %+v", call.Name, code, got)
				}
				if got.Repair == nil || got.Repair.Code != code {
					t.Fatalf("repair code = %+v, want original terminal code %q", got.Repair, code)
				}
				if got.Repair.Metadata["status"] != types.ToolRepairStatusActionRequired ||
					got.Repair.Metadata["policy"] != "trace_input_admission_terminal" ||
					got.Repair.Metadata["blocked_tool"] != types.CanonicalToolName(call.Name) ||
					got.Repair.Metadata["path"] != "capture.sys" {
					t.Fatalf("terminal repair metadata lost action/path/barrier fields: %+v", got.Repair.Metadata)
				}
			})
		}
	}
}

type traceAdmissionBatchStubTool struct {
	toolpkg.ReadOnly
	toolpkg.NonEvidenceTool
	name   string
	result types.ToolResult
	called *int
}

func (t traceAdmissionBatchStubTool) Name() string        { return t.name }
func (t traceAdmissionBatchStubTool) Description() string { return "trace admission batch test tool" }
func (t traceAdmissionBatchStubTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":true}`)
}
func (t traceAdmissionBatchStubTool) Execute(_ *types.BusContext, _ json.RawMessage) (types.ToolResult, error) {
	if t.called != nil {
		*t.called++
	}
	result := t.result
	result.ToolName = t.name
	result.Timestamp = time.Now()
	return result, nil
}

func TestTraceQueryMixedBatchRunsAdmissionFirstAndBlocksEverySibling(t *testing.T) {
	traceCalls, siblingCalls := 0, 0
	registry := toolpkg.NewRegistry()
	registry.Register(traceAdmissionBatchStubTool{
		name:   "trace_query",
		called: &traceCalls,
		result: types.ToolResult{
			Success: false,
			Summary: "binary trace rejected",
			Repair: &types.ToolRepair{
				Code: tracequery.TraceInputAdmissionCodeConversionRequired,
				Metadata: map[string]string{
					"stage":  types.ToolRepairStageTraceInputAdmission,
					"status": types.ToolRepairStatusActionRequired,
					"path":   "capture.sys",
				},
			},
		},
	})
	registry.Register(traceAdmissionBatchStubTool{
		name:   "read_file",
		called: &siblingCalls,
		result: types.ToolResult{Success: true, Summary: "must not execute"},
	})

	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: types.NewMutableState("trace")}
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: registry}, &stubEvaluator{})
	calls := []llm.ToolCall{
		{ID: "sibling", Name: "read_file", Params: json.RawMessage(`{"path":"capture.sys"}`)},
		{ID: "trace", Name: "trace_query", Params: json.RawMessage(`{"path":"capture.sys","view":"event_search"}`)},
	}
	ordered := traceInputAdmissionSafeExecutionOrder(ctx, calls)
	if ordered[0].Name != "trace_query" || canExecuteToolBatchInParallel(ctx, calls) {
		t.Fatalf("mixed trace batch was not serialized admission-first: %+v", ordered)
	}
	first, _ := base.executeTool(ctx, ordered[0], map[string]bool{"trace_query": true, "read_file": true})
	if first == nil || first.Success || traceCalls != 1 {
		t.Fatalf("trace admission stub did not execute exactly once: result=%+v calls=%d", first, traceCalls)
	}
	second, _ := base.executeTool(ctx, ordered[1], map[string]bool{"trace_query": true, "read_file": true})
	if second == nil || second.Success || second.Repair == nil ||
		second.Repair.Code != tracequery.TraceInputAdmissionCodeConversionRequired || siblingCalls != 0 {
		t.Fatalf("same-batch sibling escaped terminal latch: result=%+v sibling_calls=%d", second, siblingCalls)
	}
}

func TestTraceQueryTerminalAdmissionRepairAllowsExplorerCompletionEmit(t *testing.T) {
	mutable := types.NewMutableState("trace input admission")
	armTraceQueryTerminalAdmissionForTest(t, mutable, tracequery.TraceInputAdmissionCodeConversionRequired, false)
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mutable}

	if got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{Name: "emit_investigation_complete", Params: json.RawMessage(`{}`)}, true); got != nil {
		t.Fatalf("completion emit must remain available for terminal handoff: %+v", got)
	}
	if got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{Name: "emit_evidence", Params: json.RawMessage(`{}`)}, true); got == nil || got.Success {
		t.Fatalf("emit_evidence must not mint evidence after rejected input: %+v", got)
	}
}

func TestTraceQueryTerminalAdmissionRepairSurvivesDispatchReset(t *testing.T) {
	mutable := types.NewMutableState("trace input admission")
	armTraceQueryTerminalAdmissionForTest(t, mutable, tracequery.TraceInputAdmissionCodeTextExportRequired, false)
	mutable.ResetDispatchToolResults()
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mutable}
	got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{Name: "read_file", Params: json.RawMessage(`{"path":"capture.trace"}`)}, true)
	if got == nil || got.Success || got.Repair == nil || got.Repair.Code != tracequery.TraceInputAdmissionCodeTextExportRequired {
		t.Fatalf("terminal latch was lost across dispatch reset: %+v", got)
	}
}

func TestTraceQueryOrdinaryUnsupportedFailureStillAllowsExplorerFallback(t *testing.T) {
	mutable := types.NewMutableState("trace unsupported")
	mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  false,
		Summary:  "unsupported view for this trace flavor",
		Repair: &types.ToolRepair{
			Code: "trace_view_unsupported",
		},
	})
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mutable}

	for _, call := range []llm.ToolCall{
		{Name: "read_file", Params: json.RawMessage(`{"path":"capture.systrace"}`)},
		{Name: "grep", Params: json.RawMessage(`{"path":"capture.systrace","pattern":"sched_switch"}`)},
		{Name: "exec_command", Params: json.RawMessage(`{"cmd":"inspect capture.systrace"}`)},
	} {
		if got := validateExplorerTraceQueryFirstToolCall(ctx, call, true); got != nil {
			t.Fatalf("ordinary unsupported failure must preserve %s fallback: %+v", call.Name, got)
		}
	}
}

func TestTraceQueryAdmissionCodeOnSuccessfulResultDoesNotArmTerminalBarrier(t *testing.T) {
	mutable := types.NewMutableState("trace success")
	armTraceQueryTerminalAdmissionForTest(t, mutable, tracequery.TraceInputAdmissionCodeEmpty, true)
	ctx := &types.AgentContext{Stage: types.StageExplore, Mutable: mutable}

	if got := validateExplorerTraceQueryFirstToolCall(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"capture.systrace"}`),
	}, true); got != nil {
		t.Fatalf("only a failed trace_query result may arm the admission barrier: %+v", got)
	}
}
