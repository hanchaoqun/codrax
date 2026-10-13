package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSamplingTreeRepairPublicFinalizerHMC235(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	bad, err := (&tool.TraceQuery{}).Execute(bus, json.RawMessage(`{"source":"attached_trace","view":"perf_sampling_tree","pid":11,"time_start":1,"time_end":2,"pattern":"document-worker"}`))
	if err != nil || bad.Success || bad.Repair == nil || bad.Repair.Code != "trace_query_sampling_selector_unsupported" {
		t.Fatalf("invalid tree query did not request explicit repair: %v %+v", err, bad)
	}
	good, err := (&tool.TraceQuery{}).Execute(bus, json.RawMessage(`{"source":"attached_trace","view":"perf_sampling_tree","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !good.Success || good.Repair != nil {
		t.Fatalf("explicit same-source/window retry: %v %+v", err, good)
	}
	ctx.Mutable.AppendDispatchToolResult(bad)
	ctx.Mutable.AppendDispatchToolResult(good)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{bad, good}})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"### 采样前缀树", "sys;root", "/system/lib.so", "/app/A.so", "/app/B.so", "total_weight=35", "stack_weight=30", "unavailable_weight=5", "total_weight=7", "total_weight=100", "不是实际调用"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("repaired native finalizer handoff lost %q", want)
		}
	}
	if strings.Contains(prompt, types.TraceNoteKeyPerfSamplingTree+"={") {
		t.Error("raw transport JSON duplicated native tree")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
		t.Fatal("sampling retry enabled causal report authority")
	}
}
