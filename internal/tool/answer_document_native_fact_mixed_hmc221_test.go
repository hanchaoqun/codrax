package tool

import (
	"bytes"
	"encoding/json"
	"html"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Execute the producer, not a constructed choice: the accepted question can
// request raw facts independently of its causal or relationship explanation.
func nativeMixedPublicContext(t *testing.T, domain string) (*types.BusContext, int) {
	t.Helper()
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions,
		types.RequestedAnswerDimension{Role: types.RequestedAnswerDimensionCausalAttribution, Required: true, Index: 2},
		types.RequestedAnswerDimension{Role: types.RequestedAnswerDimensionRelationPath, Required: true, Index: 3})
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("原始事实和独立解释")}
	var result types.ToolResult
	var err error
	rows := 13
	if domain == "log" {
		var inputs []loginput.Input
		for _, relative := range []string{"app/session.log", "kernel/session.log.gz"} {
			path, _ := filepath.Abs("../../eval/fixtures/hmosperf_log_sources/" + relative)
			inputs = append(inputs, loginput.Input{Path: path})
		}
		bus, _ = logQueryPublicBus(t, inputs)
		rm.LogTriage = &types.LogBundle{}
		rm.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionMemberSet
		result, err = (&LogQuery{}).Execute(bus, json.RawMessage(`{}`))
		rows = 9
	} else {
		start, end := 1.0, 2.0
		rm.PerfTrace = &types.PerfBundle{}
		rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[1,2) 秒", Confidence: 1}
		bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(dir, ".codrax")})
		path, _ := filepath.Abs("../../eval/fixtures/hmosperf_measurements/capture.data")
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": start, "time_end": end})
		result, err = (&TraceQuery{}).Execute(bus, args)
	}
	if err != nil || !result.Success {
		t.Fatalf("query: %v / %s", err, result.Summary)
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
	bus.Mutable.SetRequestModel(rm)
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	return bus, rows
}

func TestHMC221MixedRuntimeNativeFactsPublicEmitPatchRender(t *testing.T) {
	for _, domain := range []string{"log", "measurement"} {
		for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeCausalDiagnosis, types.RuntimeQuestionScopeRelationAnalysis, types.RuntimeQuestionScopeBoundedEffectVerdict, types.RuntimeQuestionScopeSystemOverview} {
			t.Run(domain+"/"+string(scope), func(t *testing.T) {
				bus, wantRows := nativeMixedPublicContext(t, domain)
				bus.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = scope
				before, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
				const prose = "记录归属、时间和数值按原始事实展示；关系解释仍需独立证据。"
				lead := map[string]any{"id": "lead", "kind": "summary", "text": prose}
				bus.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions[0].Required = false
				out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{lead}}, false)
				if !out.Success {
					t.Fatal(out.Summary)
				}
				projectionBefore, _ := json.Marshal(bus.Mutable.TraceRootCauseReport())
				independentBlocks, _ := json.Marshal(bus.Mutable.AnswerDocumentV2().Blocks)
				bus.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions[0].Required = true
				out = b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{lead}}, false)
				if !out.Success {
					t.Fatal(out.Summary)
				}
				doc := bus.Mutable.AnswerDocumentV2()
				var kept []types.AnswerBlock
				rows, tables := 0, 0
				for _, block := range doc.Blocks {
					if block.RuntimeMeasurement != nil && block.RuntimeMeasurement.IsBound() {
						rows += len(block.RuntimeMeasurement.BoundTable.Rows)
						tables++
					} else {
						kept = append(kept, block)
					}
				}
				if rows != wantRows || tables != 1 {
					t.Fatalf("mixed causal/fact scope suppressed native rows: %+v", doc.Blocks)
				}
				keptBytes, _ := json.Marshal(kept)
				if !bytes.Equal(keptBytes, independentBlocks) {
					t.Fatal("native display changed independent model, causal or scope blocks")
				}
				visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
				fields := []string{prose, "未知"}
				if domain == "log" {
					fields = append(fields, "9007199254741001", "orphan", "99-09", "source_id=", "generation=", "session.log.gz")
				} else {
					fields = append(fields, "9007199254740993", "3.342000005e+08", "未知（NULL）", "窗口内终点", "无法定位时间1条")
				}
				for _, field := range fields {
					if !strings.Contains(visible, field) {
						t.Errorf("finalizer lost raw field %q", field)
					}
				}
				for i := 0; i < 2; i++ {
					out = b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{lead}}, true)
					if !out.Success || render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh") != render.RenderAnswerDocument(doc, "zh") {
						t.Fatalf("patch lost/duplicated fact projection: %s", out.Summary)
					}
				}
				after, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
				projectionAfter, _ := json.Marshal(bus.Mutable.TraceRootCauseReport())
				if !bytes.Equal(before, after) || !bytes.Equal(projectionBefore, projectionAfter) || bus.Mutable.AnswerDocumentV2().Blocks[0].Text != prose {
					t.Fatal("display changed evidence authority, causal projection or interpretation")
				}
			})
		}
	}
}
