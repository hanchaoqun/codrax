package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC258NativeDisplayStreamingSourceWitness(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile("../../eval/fixtures/hmosperf_cpu_state_frequency/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cpu.systrace")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.04
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "CPU概览", Confidence: 1}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: types.NewMutableState("CPU概览"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}}
	ctx.Mutable.SetRequestModel(rm)
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !r.Success {
		t.Fatalf("stream query: %v %s", err, r.Summary)
	}
	if _, _, ordinary := ctx.Mutable.ResolveTraceQueryWindowReplay(r.TraceQueryWindowReplay); ordinary {
		t.Fatal("fixture no longer exercises statistics-only private witness")
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	choices := tool.NativeFactAnswerDisplayChoices(types.ToolBusContext(ctx, types.AgentExplorer))
	if len(choices) != 3 {
		t.Fatalf("streaming CPU lost current display choices: %d", len(choices))
	}
	acceptNativeDisplayHMC258(t, ctx, choices, choices)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Accepted native answer display") {
		t.Fatal("streaming display not handed to finalizer")
	}
	raw = json.RawMessage(`{"blocks":[{"id":"explanation","kind":"summary","text":"CPU状态与频率是观测事实，不是响应根因。"}]}`)
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || !out.Success {
		t.Fatalf("emit: %v %s", err, out.Summary)
	}
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	count := 0
	for _, b := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if b.RuntimeMeasurement != nil {
			expected := *b.RuntimeMeasurement
			if !types.BindRuntimeMeasurementReceipt(&expected, contract) || !reflect.DeepEqual(expected.BoundTable, b.RuntimeMeasurement.BoundTable) {
				t.Fatal("stream interval or denominator changed")
			}
			count++
		}
	}
	if count != 3 {
		t.Fatalf("stream display count=%d", count)
	}
	visible := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh")
	for _, want := range []string{"CPU0", "CPU1", "CPU2", "1000000"} {
		if !strings.Contains(visible, want) {
			t.Errorf("CPU display lost %q", want)
		}
	}
	for _, mode := range []string{"json", "source_generation"} {
		t.Run(mode, func(t *testing.T) {
			copy := r
			if mode == "json" {
				wire, _ := json.Marshal(copy)
				copy = types.ToolResult{}
				if json.Unmarshal(wire, &copy) != nil {
					t.Fatal("roundtrip")
				}
			} else {
				if err := os.WriteFile(path, append(data, []byte("\n# changed source generation\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{copy}})
			ctx.Mutable.ResetDispatchToolResults()
			if len(tool.NativeFactAnswerDisplayChoices(types.ToolBusContext(ctx, types.AgentExplorer))) != 0 {
				t.Fatal("stale or JSON statistics witness regained display rights")
			}
			if strings.Contains(renderAcceptedNativeAnswerDisplay(ctx, types.BuildAnswerSemanticViewForAgentContext(ctx)), "final_retained_rows=") {
				t.Fatal("old streaming facts resurrected")
			}
		})
	}
}
