package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The production failure lost a completed parallel worker's entire tool
// result. Once that result reaches TurnA, existing finalizer surfaces already
// carry these measured values. Pin the actual public path, not a new spelling
// of a field or a redundant prompt section, and do not recompute statistics.
func TestPublishedIOGroupDistributionsReachFinalizer(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeCausalDiagnosis, types.RuntimeQuestionScopeBoundedFactSet} {
		t.Run(string(scope), func(t *testing.T) {
			path, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_request_latency_distribution", "events.systrace"))
			if err != nil {
				t.Fatal(err)
			}
			start, end := 1.0, 14.0
			ctx := &types.AgentContext{
				RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
				AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("Describe the measured IO groups and their limits"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					Language: "zh", PerfTrace: &types.PerfBundle{},
					RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: scope, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..14 seconds"},
				}},
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": start, "time_end": end})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success {
				t.Fatalf("query failed: %v %+v", err, result)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			before, _ := json.Marshal(answerDocObservationLedger(ctx))
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, parts := range [][]string{
				{"event=block_rq dev=12,80 op=R", "samples=11 min=1.000 mean=6.000 max=11.000 p50=6.000 p90=10.000 p95=10.500 p99=10.900"},
				{"event=block_rq dev=12,80 op=W", "samples=3 min=5.000 mean=15.000 max=25.000 p50=15.000 p90=23.000 p95=24.000 p99=24.800"},
				{"event=block_bio dev=12,80 op=R", "samples=3 min=2.000 mean=4.000 max=6.000 p50=4.000 p90=5.600 p95=5.800 p99=5.960"},
			} {
				found := false
				for _, line := range strings.Split(prompt, "\n") {
					found = found || (strings.Contains(line, parts[0]) && strings.Contains(line, parts[1]) && strings.Contains(line, "not target blocking time"))
				}
				if !found {
					t.Errorf("finalizer lost an identified distribution: %v", parts)
				}
			}
			for _, want := range []string{"issuers=all", "ambiguous_cohorts=1", "pairing_suppressed=2", "unpaired_start=1", "selected_window=1.000000..14.000000"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("finalizer lost scope/pairing disclosure %q", want)
				}
			}
			after, _ := json.Marshal(answerDocObservationLedger(ctx))
			if string(before) != string(after) {
				t.Fatal("display mutated accepted measurements")
			}
			start, end = 2, 3
			if strings.Contains(renderAnswerDocObservationLedger(ctx), "p99=10.900") {
				t.Fatal("wider query population entered narrower requested-window ledger")
			}
		})
	}
}
