package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSamplingSelectionDecoratedPublicFinalizerHMC235(t *testing.T) {
	for _, view := range []string{"perf_sampling_tree", "perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"} {
		t.Run(view, func(t *testing.T) {
			ctx := samplingSelectionAnalysisContext(t)
			params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": view, "thread": "document-worker-11 (tid=11)", "time_start": 1, "time_end": 2})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success {
				t.Fatalf("public decorated query: %v %+v", err, result)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"### 采样前缀树", "sys;root", "/app/A.so", "/app/B.so", "total_weight=35", "stack_weight=30", "unavailable_weight=5", "total_weight=7", "total_weight=100", "不是实际调用"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual finite finalizer discarded native decorated selection %q", want)
				}
			}
			if strings.Contains(prompt, types.TraceNoteKeyPerfSamplingTree+"={") {
				t.Fatal("raw transport JSON duplicated the native facts")
			}
			ledger := answerDocObservationLedger(ctx)
			projected, superseded := answerDocFinalizerObservationRecords(ctx, ledger.Records)
			if superseded != 1 {
				t.Fatalf("old mixed-domain preview was not superseded: %d", superseded)
			}
			for _, r := range projected {
				if r.ID == "perf:observation:0" {
					t.Fatal("old pre-triage sampling narration survived the native result")
				}
			}
			if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
				t.Fatal("sampling selector enabled causal report authority")
			}
		})
	}
}
