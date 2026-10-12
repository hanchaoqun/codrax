package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Uses real prepared SQLite inputs and the public query/Emit/Patch boundary.
// Displaying the native roster must not require retyping its precise values.
func traceNativeDisplayContext229(t *testing.T) *types.AgentContext {
	t.Helper()
	dir := t.TempDir()
	copyInput := func(relative, name string) string {
		body, err := os.ReadFile(relative)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	baseline := copyInput("../../eval/fixtures/hmosperf_dual_measurements/baseline.data", "baseline.data")
	current := copyInput("../../eval/fixtures/hmosperf_common_window_measurements/current.data", "current.data")
	start, end := 1.0, 2.0
	scope := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1 to 2 seconds", Confidence: 1,
		Comparison: &types.RuntimeArtifactComparisonScope{Baseline: types.RuntimeArtifactComparisonSide{Path: baseline, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: baseline}}, Current: types.RuntimeArtifactComparisonSide{Path: current, RuntimeArtifactTimeWindow: types.RuntimeArtifactTimeWindow{SourceQuote: current}}}}
	rm := types.RequestModel{Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{}, RuntimeArtifactScopeProfile: scope,
		RuntimeQuestionProfile:    &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Role: types.RequestedAnswerDimensionMemberSet, Label: "measurement records", Required: true}}}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("Compare the measurement records from 1 to 2 seconds"),
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}, TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	var results []types.ToolResult
	for _, path := range []string{baseline, current} {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": start, "time_end": end})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("actual query: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx
}

func TestHMC229TraceNativeDisplayActualQueryFinalizer(t *testing.T) {
	ctx := traceNativeDisplayContext229(t)
	rm := ctx.AnalysisIR.RequestModel
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	var members []types.RuntimeMeasurementTable
	for _, table := range contract.Choices() {
		if table.View == types.RuntimeMeasurementMembers && (len(table.Rows) == 13 || len(table.Rows) == 8) {
			members = append(members, table)
		}
	}
	if len(members) != 2 || len(members[0].Rows)+len(members[1].Rows) != 21 {
		t.Fatalf("real two-source roster missing: %+v", members)
	}
	execute := func(payload any, patch bool) {
		t.Helper()
		raw, _ := json.Marshal(payload)
		var result types.ToolResult
		var err error
		if patch {
			if err = toolparam.Validate(raw, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)); err != nil {
				t.Fatal(err)
			}
			result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
		} else {
			result, err = (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
		}
		if err != nil || !result.Success {
			t.Fatalf("actual finalizer emit/patch: %v / %s", err, result.Summary)
		}
	}
	execute(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "The source records retain their original values."}}}, false)
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2()); len(missing) != 1 {
		t.Fatalf("automatic display assigned a dimension: %+v", missing)
	}
	e := &answerDocumentEvaluator{mu: ctx.Mutable, language: "en"}
	signal := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
	if !signal.HintRequested || !strings.Contains(signal.Hint, "requested_dimension_indices") || strings.Contains(signal.Hint, "add_facet_id") {
		t.Fatalf("complete native rows still force model reconstruction: %s", signal.Hint)
	}
	var blocks []any
	for i, table := range members {
		if !table.HasCompleteNativeDisplayPopulation() || table.MemberSet != nil || table.CoversMemberSet(&rm) {
			t.Fatalf("display/completion qualification crossed: %+v", table)
		}
		blocks = append(blocks, map[string]any{"id": []string{"baseline_rows", "current_rows"}[i], "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}, "requested_dimension_indices": []int{1}})
	}
	execute(map[string]any{"add_blocks": blocks, "unchanged_block_ids": []string{"lead"}}, true)
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2()); len(missing) != 0 {
		t.Fatalf("metadata binding ignored: %+v", missing)
	}
	for _, table := range members {
		count := 0
		for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
			if block.RuntimeMeasurement == nil || block.RuntimeMeasurement.ObservationID != table.ObservationID || block.RuntimeMeasurement.View != table.View {
				continue
			}
			count++
			if !reflect.DeepEqual(block.RuntimeMeasurement.BoundTable.Rows, table.Rows) || len(block.Items) != 0 || block.Text != "" {
				t.Fatal("binding reconstructed native values")
			}
		}
		if count != 1 {
			t.Fatalf("automatic and selected native roster duplicated: %d", count)
		}
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("display binding completed investigation")
	}
	rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions,
		types.RequestedAnswerDimension{Index: 2, Role: types.RequestedAnswerDimensionSourceLocation, Label: "source locations", Required: true},
		types.RequestedAnswerDimension{Index: 3, Role: types.RequestedAnswerDimensionCausalAttribution, Label: "proven cause", Required: true})
	ctx.AnalysisIR.RequestModel = rm
	ctx = freshTraceDisplayContext229(ctx)
	if missing := missingRequestedAnswerDimensionsInDocument(ctx, ctx.Mutable.AnswerDocumentV2()); len(missing) != 2 || missing[0].Index != 2 || missing[1].Index != 3 {
		t.Fatalf("native rows discharged independent source/causal obligations: %+v", missing)
	}
}

func freshTraceDisplayContext229(ctx *types.AgentContext) *types.AgentContext {
	return &types.AgentContext{RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Language: ctx.Language, AgentName: ctx.AgentName, Stage: ctx.Stage,
		Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR, TraceInputPreparer: ctx.TraceInputPreparer}
}

func TestHMC229TraceNativeDisplayCurrentSourceAndWindow(t *testing.T) {
	for _, mode := range []string{"exact", "wider_request", "changed_original", "new_turn", "public_json_only", "source_owned_dimension"} {
		t.Run(mode, func(t *testing.T) {
			ctx := traceNativeDisplayContext229(t)
			initial := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
			var selected *types.RuntimeMeasurementTable
			for _, table := range initial.Choices() {
				if table.View == types.RuntimeMeasurementMembers && len(table.Rows) == 13 {
					copy := table
					selected = &copy
				}
			}
			if selected == nil || !selected.HasCompleteNativeDisplayPopulation() {
				t.Fatal("initial complete native roster missing")
			}
			block := types.AnswerBlock{Kind: types.BlockTable, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: selected.ObservationID, View: selected.View}, RequestedDimensionIndices: []int{1}}
			if _, ok := types.BoundNativeDisplayDimensions(block, initial); !ok {
				t.Fatal("initial exact selector unavailable")
			}
			switch mode {
			case "wider_request":
				end := 3.0
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
			case "changed_original":
				path := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.Comparison.Baseline.Path
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(body, 0), 0600); err != nil {
					t.Fatal(err)
				}
			case "new_turn":
				prior := *ctx.Mutable.TurnAArtifacts()
				ctx.Mutable.ResetTurnAArtifacts()
				ctx.Mutable.SetTurnAArtifacts(prior)
			case "public_json_only":
				artifacts := ctx.Mutable.TurnAArtifacts()
				raw, _ := json.Marshal(artifacts.ToolResults)
				var public []types.ToolResult
				if err := json.Unmarshal(raw, &public); err != nil {
					t.Fatal(err)
				}
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: public})
			case "source_owned_dimension":
				ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "src/main.go", RequestedDimensionIndices: []int{1}}}
			}
			ctx = freshTraceDisplayContext229(ctx)
			current := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
			_, bound := types.BoundNativeDisplayDimensions(block, current)
			if bound != (mode == "exact") {
				t.Fatalf("stale/wrong scope borrowed display qualification: mode=%s bound=%v", mode, bound)
			}
			if mode == "wider_request" {
				var stillVisible bool
				for _, table := range current.Choices() {
					if table.ObservationID == selected.ObservationID && table.View == selected.View {
						stillVisible = reflect.DeepEqual(table.Rows, selected.Rows) && !table.HasCompleteNativeDisplayPopulation()
					}
				}
				if !stillVisible {
					t.Fatal("complete drill-down lost its values instead of only qualification")
				}
			}
		})
	}
}

func TestHMC229TraceNativeDisplayActualComparison(t *testing.T) {
	for _, mode := range []string{"exact", "current_drilldown", "single_then_pair", "pair_then_single", "unbounded_request", "stale_pair"} {
		t.Run(mode, func(t *testing.T) {
			ctx := traceNativeDisplayContext229(t)
			prior := *ctx.Mutable.TurnAArtifacts()
			comparison := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.Comparison
			query := func(path string, end float64) map[string]any {
				return map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": end}
			}
			currentEnd := 2.0
			if mode == "current_drilldown" {
				currentEnd = 1.5
			}
			args, _ := json.Marshal(map[string]any{"comparison": map[string]any{"baseline": query(comparison.Baseline.Path, 2), "current": query(comparison.Current.Path, currentEnd)}})
			pair, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
			if err != nil || !pair.Success {
				t.Fatalf("actual comparison: %v / %s", err, pair.Summary)
			}
			results := []types.ToolResult{pair}
			if mode == "single_then_pair" {
				results = append(prior.ToolResults, pair)
			}
			if mode == "pair_then_single" {
				results = append(results, prior.ToolResults...)
			}
			if mode == "unbounded_request" {
				scope := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
				scope.RequestedScope, scope.TimeStart, scope.TimeEnd = types.RuntimeArtifactScopeUnspecified, nil, nil
			}
			if mode == "stale_pair" {
				ctx.Mutable.ResetTurnAArtifacts()
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ctx = freshTraceDisplayContext229(ctx)
			contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
			members, qualified, rowCount := 0, 0, 0
			for _, table := range contract.Choices() {
				if table.View != types.RuntimeMeasurementMembers {
					continue
				}
				members++
				rowCount += len(table.Rows)
				if table.MemberSet != nil || table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
					t.Fatal("pair display acquired completion")
				}
				if table.HasCompleteNativeDisplayPopulation() {
					qualified++
				}
				if mode == "current_drilldown" && len(table.Rows) != 13 && table.HasCompleteNativeDisplayPopulation() {
					t.Fatal("current narrow query borrowed baseline/exact requested window")
				}
			}
			if mode == "stale_pair" {
				if qualified != 0 {
					t.Fatal("old pair private receipt revived")
				}
				return
			}
			want := 2
			if mode == "current_drilldown" {
				want = 1
			}
			if members != 2 || qualified != want {
				t.Fatalf("pair qualification/coalescing: mode=%s members=%d qualified=%d want=%d", mode, members, qualified, want)
			}
			if mode != "current_drilldown" && rowCount != 21 {
				t.Fatalf("pair full native rows changed: %d", rowCount)
			}
		})
	}
}
