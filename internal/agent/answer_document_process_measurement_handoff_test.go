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
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProcessMeasurementPublicFinalizerEmitPatchRender(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_measurements/capture.data")
	dir := t.TempDir()
	start, end := 1.0, 2.0
	rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到2秒两个应用的内存指标", Confidence: 1}}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("1到2秒两个应用的内存指标"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
	ctx.Mutable.SetRequestModel(rm)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_measurements", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("query: %v / %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 3 {
		t.Fatalf("wrong process selector roster: %+v", choices)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"runtime_measurement", "system fills", "app.alpha", "app.beta", "9007199254740993", "未知（NULL）", "查询窗口 [1.000000,2.000000)", "不由指标名称推断"} {
		if !strings.Contains(strings.ToLower(prompt), strings.ToLower(want)) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	for _, table := range choices {
		if !strings.Contains(prompt, table.ObservationID) || !strings.Contains(prompt, "view=\""+string(table.View)+"\"") || table.MemberSet != nil {
			t.Fatal("selector/display authority drift")
		}
	}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	projection := func() []byte {
		v, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
		return v
	}
	beforeProjection := projection()
	const prose = "这些是应用指标的原始观测，尚不能直接认定内存泄漏。"
	selected := func(table types.RuntimeMeasurementTable) map[string]any {
		return map[string]any{"id": "measurement", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
	}
	execute := func(block map[string]any, patch, valid bool) {
		t.Helper()
		input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": prose}, map[string]any{"id": "authored", "kind": "table", "columns": []string{"说明"}, "items": []any{map[string]any{"cells": []string{"模型解释保持原样"}}}}, block}}
		schema := (&tool.EmitAnswerDocument{}).ParametersFor(ctx)
		if patch {
			input = map[string]any{"replace_blocks": []any{block}, "unchanged_block_ids": []string{"lead", "authored"}}
			schema = (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)
		}
		raw, _ := json.Marshal(input)
		original := append([]byte(nil), raw...)
		if got := toolparam.Validate(raw, schema); (got == nil) != valid {
			t.Fatalf("schema validity=%v want %v: %s", got, valid, raw)
		}
		accepted := ctx.Mutable.AnswerDocumentV2()
		var out types.ToolResult
		var err error
		if patch {
			out, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
		} else {
			out, err = (&tool.EmitAnswerDocument{}).Execute(bus, raw)
		}
		if err != nil || out.Success != valid {
			t.Fatalf("emit/patch: %v / %s", err, out.Summary)
		}
		if !bytes.Equal(raw, original) {
			t.Fatal("model JSON bytes mutated")
		}
		if !valid && !reflect.DeepEqual(accepted, ctx.Mutable.AnswerDocumentV2()) {
			t.Fatal("rejected receipt overwrote accepted answer")
		}
	}
	for _, table := range choices {
		for _, patch := range []bool{false, true} {
			execute(selected(table), patch, true)
			doc := ctx.Mutable.AnswerDocumentV2()
			found := false
			for _, block := range doc.Blocks {
				if block.ID == "measurement" {
					found = block.RuntimeMeasurement != nil && block.RuntimeMeasurement.IsBound() && reflect.DeepEqual(*block.RuntimeMeasurement.BoundTable, table)
				}
				if block.ID == "authored" && (block.RuntimeMeasurement != nil || len(block.Items) != 1 || !reflect.DeepEqual(block.Items[0].Cells, []string{"模型解释保持原样"})) {
					t.Fatal("model table replaced")
				}
			}
			if !found {
				t.Fatal("table not bound exactly")
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			if !strings.Contains(visible, prose) || !strings.Contains(visible, "模型解释保持原样") {
				t.Fatal("model interpretation lost")
			}
			for _, row := range table.Rows {
				if strings.Count(visible, "| "+strings.Join(row, " | ")+" |") != 1 {
					t.Fatalf("row not rendered exactly once: %v", row)
				}
			}
			for _, note := range table.Notes {
				if !strings.Contains(visible, note) {
					t.Fatalf("note lost: %q", note)
				}
			}
		}
	}
	for _, patch := range []bool{false, true} {
		bad := selected(choices[0])
		bad["runtime_measurement"] = map[string]any{"observation_id": choices[0].ObservationID, "view": "distribution"}
		execute(bad, patch, false)
		bad = selected(choices[0])
		bad["runtime_measurement"] = map[string]any{"observation_id": "old-source", "view": "summary"}
		execute(bad, patch, false)
	}
	if !bytes.Equal(beforeProjection, projection()) || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("measurement display changed root-cause authority")
	}
	// A query extending beyond a subsequently narrowed explicit request is not
	// silently relabelled as that window's measurements.
	narrowEnd := 1.5
	rm.RuntimeArtifactScopeProfile.TimeEnd = &narrowEnd
	ctx.AnalysisIR.RequestModel = rm
	ctx.Mutable.SetRequestModel(rm)
	if contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract; contract != nil && len(contract.Choices()) > 0 {
		t.Fatal("out-of-window native table leaked")
	}
	execute(selected(choices[0]), true, false)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	ctx.Mutable.ResetDispatchToolResults()
	execute(selected(choices[0]), true, false)
	// An empty producer table must still deliver status/unknown boundaries,
	// not force the model to infer that its absent rows are measured zero.
	emptyArgs, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_measurements", "time_start": start, "time_end": narrowEnd, "pid": 999})
	empty, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), emptyArgs)
	if err != nil || !empty.Success {
		t.Fatalf("empty query: %v %s", err, empty.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{empty}})
	emptyPrompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"查询状态：available", "没有可展示记录不等于指标值为0", "查询匹配0条"} {
		if !strings.Contains(emptyPrompt, want) {
			t.Errorf("empty handoff lost %q", want)
		}
	}
}
