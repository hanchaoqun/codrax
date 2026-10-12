package agent

import (
	"bytes"
	"encoding/json"
	"html"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC226NativeTimelineActualFinalizerSourceWindowReuse(t *testing.T) {
	a, _ := filepath.Abs("../../eval/fixtures/hmosperf_dual_source_windows/baseline.systrace")
	b, _ := filepath.Abs("../../eval/fixtures/hmosperf_dual_source_windows/current.systrace")
	startA, endA, startB, endB := 1.0, 1.01, 4.0, 4.02
	left := types.RuntimeArtifactComparisonSide{Path: a, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &startA, TimeEnd: &endA, SourceQuote: "baseline 1..1.01"}}
	right := types.RuntimeArtifactComparisonSide{Path: b, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{TimeStart: &startB, TimeEnd: &endB, SourceQuote: "current 4..4.02"}}
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, Comparison: &types.RuntimeArtifactComparisonScope{Baseline: left, Current: right}, TimeWindows: []types.RuntimeArtifactTimeWindow{left.RuntimeArtifactTimeWindow, right.RuntimeArtifactTimeWindow}},
		RequestedAnswerDimensions:   &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Role: types.RequestedAnswerDimensionObservedValue, Label: "线程与CPU的变化过程", Required: true}}},
	}
	dir := t.TempDir()
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("compare native timelines"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	var results []types.ToolResult
	wanted := map[string]types.RuntimeMeasurementTable{}
	rejected := map[string]bool{}
	for _, path := range []string{a, b} {
		for _, window := range [][2]float64{{startA, endA}, {startB, endB}} {
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 41, "time_start": window[0], "time_end": window[1]})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !result.Success {
				t.Fatalf("query: %v %s", err, result.Summary)
			}
			results = append(results, result)
			for _, observation := range result.Observations {
				if observation.Predicate != "thread_timeline_observation" && observation.Predicate != "cpu_frequency_timeline_observation" {
					continue
				}
				publication, ok := types.DecodeRuntimeMeasurementPublication(observation)
				if !ok {
					t.Fatal("native timing publication failed its own receipt validation")
				}
				if path == a && window[0] == startA || path == b && window[0] == startB {
					for _, table := range publication.Tables {
						wanted[table.ObservationID] = table
					}
				} else {
					rejected[observation.ID] = true
				}
			}
		}
	}
	if len(wanted) != 4 || len(rejected) != 4 {
		t.Fatalf("both query sources must publish measured thread/frequency timelines, including off-scope negative controls: %d/%d", len(wanted), len(rejected))
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	before, _ := json.Marshal(results)
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	handoff := renderAnswerDocRuntimeMeasurementChoices(ctx)
	for _, table := range contract.Choices() {
		if rejected[table.ObservationID] {
			t.Fatal("native display borrowed the peer source window")
		}
		if original, ok := wanted[table.ObservationID]; ok && !reflect.DeepEqual(table.Rows, original.Rows) {
			t.Fatal("handoff changed native timeline cells")
		}
	}
	for id := range rejected {
		if strings.Contains(handoff, id) {
			t.Fatal("prose handoff borrowed the peer window")
		}
	}
	for _, value := range []string{"1.002000", "1.006000", "4.004000", "4.012000", "1.005000", "4.010000", "1000000", "2000000", "1500000", "750000", "kHz"} {
		if !strings.Contains(handoff, value) {
			t.Errorf("actual handoff lost native timing/frequency %q", value)
		}
	}
	// Native table ownership does not authorize rewriting a model's prose or
	// inventing causal relations; exact original cells render independently.
	const prose = "线程与CPU的原始过程见下表；不同采集窗口不直接证明相同工作量或根因。"
	params, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": prose}}})
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	projectionBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("emit: %v %s", err, result.Summary)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
	if !strings.Contains(visible, prose) {
		t.Fatal("native display changed model-authored prose")
	}
	seen := map[string]bool{}
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		if table, ok := wanted[block.RuntimeMeasurement.ObservationID]; ok {
			if seen[table.ObservationID] || !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) {
				t.Fatal("default display lost, duplicated or reconstructed native timing facts")
			}
			seen[table.ObservationID] = true
		}
	}
	if len(seen) != 4 {
		t.Fatalf("default display omitted requested source-bound facts: %d", len(seen))
	}
	projectionAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
	after, _ := json.Marshal(results)
	if !bytes.Equal(before, after) || !bytes.Equal(projectionBefore, projectionAfter) {
		t.Fatal("display mutated producer evidence or causal projection")
	}
}

func TestHMC226FiniteFactsDoNotNeedDuplicateDimensionDeclaration(t *testing.T) {
	ctx, table := nativeDisplayBindingContext224(t, `{}`)
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	// Use a fresh prompt context after changing the accepted structured profile.
	ctx = &types.AgentContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Language: ctx.Language, AgentName: ctx.AgentName, Stage: ctx.Stage,
		Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR, AttachedLogCatalog: ctx.AttachedLogCatalog, AttachedLog: ctx.AttachedLog}
	if !strings.Contains(renderAnswerDocRuntimeMeasurementChoices(ctx), types.RuntimeNativeFactDisplayTeaching) {
		t.Fatal("finite fact profile already declares the domain but duplicate dimension omission disables native display")
	}
	params, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "原始记录保留如下。"}}})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
	if err != nil || !out.Success {
		t.Fatalf("emit: %v %s", err, out.Summary)
	}
	for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.ObservationID == table.ObservationID {
			if !block.RuntimeMeasurement.IsBound() || !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) {
				t.Fatal("automatic native display changed original rows")
			}
			return
		}
	}
	t.Fatal("finite fact answer silently lost default native records")
}
