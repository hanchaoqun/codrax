package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSamplingUnknownIdentityActualFinalizerHMC239(t *testing.T) {
	for _, view := range []string{"perf_sampling_tree", "perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"} {
		t.Run(view, func(t *testing.T) {
			ctx := samplingSelectionAnalysisContext(t)
			path := filepath.Join(t.TempDir(), "unknown.perftrace")
			data := "document-worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=7 event=cycles symbol=work source=raw\n"
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			ctx.AttachedHitrace, ctx.AttachedHitraceSource = data, path
			raw, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": view, "pid": 11, "time_start": 1, "time_end": 2})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || !result.Success || result.Repair != nil {
				t.Fatalf("source uncertainty became a query retry: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "status=unavailable reason=typed_identity_unproven") || strings.Contains(prompt, "Sampling prefix tree: status=available") {
				t.Fatalf("actual finalizer facts laundered unassigned samples into an exact zero:\n%s", prompt)
			}
			if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
				t.Fatal("unknown sampling identity enabled trace causal authority")
			}
		})
	}
}
