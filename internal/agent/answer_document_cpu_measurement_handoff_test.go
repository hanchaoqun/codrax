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

func TestCPUStateFrequencyMeasurementPublicFinalizerEmitPatchRender(t *testing.T) {
	for _, tc := range []struct {
		name, fixture string
		start, end    float64
	}{
		{"raw", "hmosperf_cpu_state_frequency/events.systrace", 1, 1.04},
		{"native", "hmosperf_cpu_native_intervals/capture.data", 0, .04},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _ := filepath.Abs("../../eval/fixtures/" + tc.fixture)
			dir := t.TempDir()
			rm := types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric, PerfTrace: &types.PerfBundle{},
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactResourcePressure}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &tc.start, TimeEnd: &tc.end, SourceQuote: "CPU组合分布", Confidence: 1}}
			ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
				Mutable: types.NewMutableState("CPU组合分布"), AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}},
				TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})}
			ctx.Mutable.SetRequestModel(rm)
			args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": tc.start, "time_end": tc.end})
			result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
			if err != nil || !result.Success {
				t.Fatalf("query: %v / %s", err, result.Summary)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			choices := view.RuntimeMeasurementContract.Choices()
			if len(choices) != 3 {
				t.Fatalf("wrong CPU selector roster: %+v", choices)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, table := range choices {
				if !strings.Contains(prompt, table.ObservationID) || !strings.Contains(prompt, "view=\""+string(table.View)+"\"") {
					t.Fatal("actual finalizer lost producer selector")
				}
			}
			for _, want := range []string{"runtime_measurement", "System fills", "not a dependency or root-cause proof"} {
				if !strings.Contains(strings.ToLower(prompt), strings.ToLower(want)) {
					t.Errorf("missing precise teaching %q", want)
				}
			}
			if tc.name == "native" {
				for _, want := range []string{"jointly known 35 CPU-ms (43.75%)", "jointly unknown 45 CPU-ms (56.25%)", "源CPU状态码0（含义未核实）"} {
					if !strings.Contains(prompt, want) {
						t.Errorf("lost measured/native boundary %q", want)
					}
				}
			}
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			ledgerInput := types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)
			beforeProjection, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(ledgerInput)))
			const prose = "这是模型对观测结果的解释；不单凭CPU驻留认定响应根因。"
			modelTable := map[string]any{"id": "authored", "kind": "table", "columns": []string{"说明", "内容"}, "items": []any{map[string]any{"cells": []string{"模型表达", "保持原样"}}}}
			selected := func(table types.RuntimeMeasurementTable) map[string]any {
				return map[string]any{"id": "measurement", "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}}
			}
			execute := func(block map[string]any, patch bool, valid bool) {
				t.Helper()
				input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": prose}, modelTable, block}}
				schema := (&tool.EmitAnswerDocument{}).ParametersFor(ctx)
				if patch {
					input = map[string]any{"replace_blocks": []any{block}, "unchanged_block_ids": []string{"lead", "authored"}}
					schema = (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)
				}
				raw, _ := json.Marshal(input)
				original := append([]byte(nil), raw...)
				if got := toolparam.Validate(raw, schema); (got == nil) != valid {
					t.Fatalf("actual schema validity=%v, want %v: %s", got, valid, raw)
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
					t.Fatalf("public emit/patch=%v: %v / %s", patch, err, out.Summary)
				}
				if !bytes.Equal(raw, original) {
					t.Fatal("model JSON bytes mutated")
				}
				if !valid && !reflect.DeepEqual(accepted, ctx.Mutable.AnswerDocumentV2()) {
					t.Fatal("rejected receipt replaced accepted answer")
				}
			}
			for _, table := range choices {
				for _, patch := range []bool{false, true} {
					execute(selected(table), patch, true)
					doc := ctx.Mutable.AnswerDocumentV2()
					var found bool
					for _, block := range doc.Blocks {
						if block.ID == "measurement" {
							found = block.RuntimeMeasurement != nil && block.RuntimeMeasurement.IsBound() && reflect.DeepEqual(*block.RuntimeMeasurement.BoundTable, table)
						}
						if block.ID == "authored" && (block.RuntimeMeasurement != nil || !reflect.DeepEqual(block.Columns, []string{"说明", "内容"}) || len(block.Items) != 1 || !reflect.DeepEqual(block.Items[0].Cells, []string{"模型表达", "保持原样"})) {
							t.Fatal("ordinary model table rewritten")
						}
					}
					if !found {
						t.Fatal("bound table did not exactly match producer")
					}
					visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
					if !strings.Contains(visible, prose) || !strings.Contains(visible, "保持原样") {
						t.Fatal("model interpretation lost")
					}
					for _, row := range table.Rows {
						joined := "| " + strings.Join(row, " | ") + " |"
						if strings.Count(visible, joined) != 1 {
							t.Fatalf("producer row not rendered exactly once: %q\n%s", joined, visible)
						}
					}
					for _, note := range table.Notes {
						if !strings.Contains(visible, note) {
							t.Fatalf("bound note missing: %q", note)
						}
					}
				}
			}
			for _, patch := range []bool{false, true} {
				bad := selected(choices[0])
				bad["runtime_measurement"] = map[string]any{"observation_id": choices[0].ObservationID, "view": "members"}
				execute(bad, patch, false)
				bad = selected(choices[0])
				bad["runtime_measurement"] = map[string]any{"observation_id": "stale-source-window", "view": "summary"}
				execute(bad, patch, false)
			}
			afterProjection, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))))
			if !bytes.Equal(beforeProjection, afterProjection) || ctx.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("measurement receipt changed causal authority")
			}
			// Removing current evidence also removes the selector and rejects
			// the old submission; accepted display snapshots remain untouched.
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
			ctx.Mutable.ResetDispatchToolResults()
			execute(selected(choices[0]), true, false)
		})
	}
}
