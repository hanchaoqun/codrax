package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeFactDisplayRequest(role types.RequestedAnswerDimensionRole) types.RequestModel {
	return types.RequestModel{Language: "zh", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1,
			Dimensions: []types.RequestedAnswerDimension{{Role: role, Required: true, Label: "原始记录", Index: 1}}}}
}

func TestNativeFactDefaultPublicEmitRender(t *testing.T) {
	for _, domain := range []string{"log", "measurement"} {
		t.Run(domain, func(t *testing.T) {
			rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
			dir := t.TempDir()
			bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "zh", Mutable: types.NewMutableState("读取附件原始记录")}
			var result types.ToolResult
			var err error
			wantRows := 13
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
				wantRows = 9
			} else {
				start, end := 1.0, 2.0
				rm.PerfTrace = &types.PerfBundle{}
				rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, Confidence: 1}
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
			ledgerBefore, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
			const prose = "记录的解释保持模型所有；未证明的身份、单位、时钟与因果关系仍未知。"
			input := map[string]any{"blocks": []any{map[string]any{"id": "interpretation", "kind": "summary", "text": prose}}}
			out := b1659bExecuteAnswer(t, bus, input, false)
			if !out.Success {
				t.Fatal(out.Summary)
			}
			doc := bus.Mutable.AnswerDocumentV2()
			rows, tables := 0, 0
			for _, b := range doc.Blocks {
				if b.RuntimeMeasurement != nil && b.RuntimeMeasurement.IsBound() {
					tables++
					rows += len(b.RuntimeMeasurement.BoundTable.Rows)
					if b.RuntimeMeasurement.BoundTable.MemberSet != nil {
						t.Fatal("display gained completion authority")
					}
				}
			}
			if rows != wantRows || tables != 1 {
				t.Fatalf("summary-only emission kept %d rows in %d tables; want %d in one native table", rows, tables, wantRows)
			}
			visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
			if !strings.Contains(visible, prose) {
				t.Fatal("model explanation changed")
			}
			encoded, _ := json.Marshal(doc)
			if strings.Contains(string(encoded), "native_facts") || strings.Contains(string(encoded), "BoundTable") {
				t.Fatal("private system ownership escaped into model JSON")
			}
			var recovered types.AnswerDocumentV2
			if err := json.Unmarshal(encoded, &recovered); err != nil {
				t.Fatal(err)
			}
			if types.ReauthenticateSystemSnapshotBlockKinds(&recovered, nil) != 0 {
				t.Fatal("missing snapshot receipt invented system identity")
			}
			if types.ReauthenticateSystemSnapshotBlockKinds(&recovered, types.CaptureSystemGeneratedBlockKinds(doc)) != 1 {
				t.Fatal("authenticated system ownership did not survive snapshot")
			}
			if !types.RebindRuntimeAnswerReceipts(&recovered, types.BuildAnswerSemanticViewForBusContext(bus)) || render.RenderAnswerDocument(&recovered, "zh") != render.RenderAnswerDocument(doc, "zh") {
				t.Fatal("snapshot recovery changed native facts")
			}
			if domain == "log" {
				for _, s := range []string{"9007199254741001", "orphan", "未知", "99-09"} {
					if !strings.Contains(visible, s) {
						t.Errorf("native log field missing %q", s)
					}
				}
			} else {
				for _, s := range []string{"9007199254740993", "3.342000005e+08", "未知（NULL）", "窗口内终点", "无法定位时间1条"} {
					if !strings.Contains(visible, s) {
						t.Errorf("native measurement field missing %q", s)
					}
				}
			}
			ledgerAfter, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
			if !bytes.Equal(ledgerBefore, ledgerAfter) || bus.Mutable.TraceRootCauseReport() != nil {
				t.Fatal("display changed causal/evidence ledger")
			}
			// A model-selected exact view owns its placement, without an extra
			// system copy. Patching prose neither drops nor duplicates the table.
			var chosen *types.AnswerRuntimeMeasurementReceipt
			for _, block := range doc.Blocks {
				if block.RuntimeMeasurement != nil {
					chosen = block.RuntimeMeasurement
				}
			}
			selector := map[string]any{"id": "selected", "kind": "table", "runtime_measurement": map[string]any{"observation_id": chosen.ObservationID, "view": chosen.View}}
			out = b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{input["blocks"].([]any)[0], selector}}, false)
			if !out.Success {
				t.Fatal(out.Summary)
			}
			out = b1659bExecuteAnswer(t, bus, map[string]any{"replace_blocks": []any{input["blocks"].([]any)[0]}, "unchanged_block_ids": []string{"selected"}}, true)
			if !out.Success {
				t.Fatal(out.Summary)
			}
			if len(bus.Mutable.AnswerDocumentV2().Blocks) != 2 {
				t.Fatal("selected table duplicated by automatic display")
			}
			if domain == "measurement" {
				selector["runtime_measurement"].(map[string]any)["view"] = types.RuntimeMeasurementSummary
				out = b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{input["blocks"].([]any)[0], selector}}, false)
				if !out.Success {
					t.Fatal(out.Summary)
				}
				if len(bus.Mutable.AnswerDocumentV2().Blocks) != 3 {
					t.Fatal("summary selector suppressed full raw members/time intervals")
				}
			}
			// Model text is independent even when factually wrong; the feature
			// must not rewrite it or pretend it has been semantically verified.
			wrong := "unverified model claim: unknown value equals zero"
			out = b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "interpretation", "kind": "summary", "text": wrong}}}, false)
			if !out.Success || bus.Mutable.AnswerDocumentV2().Blocks[0].Text != wrong {
				t.Fatal("default display rewrote the model conclusion")
			}
			for _, scope := range []types.RuntimeQuestionScope{types.RuntimeQuestionScopeCausalDiagnosis, types.RuntimeQuestionScopeRelationAnalysis, types.RuntimeQuestionScopeNotApplicable} {
				rm.RuntimeQuestionProfile.Scope = scope
				bus.AnalysisIR.RequestModel = rm
				bus.Mutable.SetRequestModel(rm)
				out = b1659bExecuteAnswer(t, bus, input, false)
				if !out.Success {
					t.Fatal(out.Summary)
				}
				for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
					if block.SystemGeneratedKind == types.AnswerSystemGeneratedNativeFacts {
						t.Fatalf("scope %s acquired automatic fact answer", scope)
					}
				}
			}
		})
	}
}

func TestNativeFactDefaultPublicPartialAndInvalidSource(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_log_sources/app/session.log")
	bus, _ := logQueryPublicBus(t, []loginput.Input{{Path: path}})
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionMemberSet)
	rm.LogTriage = &types.LogBundle{}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
	bus.Mutable.SetRequestModel(rm)
	result, err := (&LogQuery{}).Execute(bus, json.RawMessage(`{"limit":2}`))
	if err != nil || !result.Success {
		t.Fatalf("query: %v / %s", err, result.Summary)
	}
	input := map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "模型解释不代替原记录。"}}}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	out := b1659bExecuteAnswer(t, bus, input, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh"))
	if !strings.Contains(visible, "本次查询匹配6条；此页保留2条，未在本表展示4条") {
		t.Fatal("partial page lost exact omission disclosure")
	}
	for _, mutation := range []func(*types.ToolResult){
		func(r *types.ToolResult) { r.Success = false },
		func(r *types.ToolResult) { r.ToolName = "other_tool" },
		func(r *types.ToolResult) { r.Observations[0].SourceRef.QueryScopeID = "another-query" },
		func(r *types.ToolResult) { r.Observations[0].SourceRef.PayloadRef = "" },
		func(r *types.ToolResult) { r.Observations[0].RichNotes[0] = "source_generation=other-generation" },
		func(r *types.ToolResult) {
			r.Observations[0].Value = strings.Replace(r.Observations[0].Value, `"pid":27599`, `"pid":null,"pid":27599`, 1)
		},
	} {
		encoded, _ := json.Marshal(result)
		var invalid types.ToolResult
		if err := json.Unmarshal(encoded, &invalid); err != nil {
			t.Fatal(err)
		}
		mutation(&invalid)
		bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{invalid}})
		out = b1659bExecuteAnswer(t, bus, input, false)
		if !out.Success {
			t.Fatal(out.Summary)
		}
		for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
			if block.RuntimeMeasurement != nil {
				t.Fatal("invalid source gained native display authority")
			}
		}
	}
}

func TestNativeFactDefaultPublicPastedLogEnglishAndForgedOwnership(t *testing.T) {
	bus, _ := logQueryPublicBus(t, []loginput.Input{{Name: "pasted", Data: []byte("10-09 01:02:03.123 0 0 I Camera: started\n")}})
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.Language, rm.LogTriage = "en", &types.LogBundle{}
	rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOccurrenceTime}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}
	bus.Mutable.SetRequestModel(rm)
	result, err := (&LogQuery{}).Execute(bus, json.RawMessage(`{}`))
	if err != nil || !result.Success {
		t.Fatalf("query: %v / %s", err, result.Summary)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "native-facts-fake", "kind": "summary", "text": "Keep model explanation."}}}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	if len(doc.Blocks) != 2 || doc.Blocks[0].SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
		t.Fatal("model ID acquired system ownership")
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(doc, "en"))
	for _, want := range []string{"Original log records", "source_id=", "generation=", "pasted", "unknown", "| 0 | 0 |"} {
		if !strings.Contains(visible, want) {
			t.Errorf("English pasted log lost %q", want)
		}
	}
	var forged types.AnswerBlock
	if err := json.Unmarshal([]byte(`{"id":"fake","kind":"summary","text":"model text","system_generated_kind":"native_facts","SystemGeneratedKind":"native_facts"}`), &forged); err != nil {
		t.Fatal(err)
	}
	if forged.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
		t.Fatal("model JSON forged internal ownership")
	}
}

func TestNativeFactDefaultPublicOutputDoesNotUsePromptRowBudget(t *testing.T) {
	var raw strings.Builder
	for i := 0; i < 140; i++ {
		fmt.Fprintf(&raw, "10-09 01:02:03.123 12 34 I Task: item-%03d\n", i)
	}
	bus, _ := logQueryPublicBus(t, []loginput.Input{{Name: "paged-log", Data: []byte(raw.String())}})
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionMemberSet)
	rm.LogTriage = &types.LogBundle{}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "zh"}}
	bus.Mutable.SetRequestModel(rm)
	var results []types.ToolResult
	for _, offset := range []int{0, 50, 100} {
		params, _ := json.Marshal(map[string]int{"offset": offset, "limit": 50})
		result, err := (&LogQuery{}).Execute(bus, params)
		if err != nil || !result.Success {
			t.Fatalf("page %d: %v / %s", offset, err, result.Summary)
		}
		results = append(results, result)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	out := b1659bExecuteAnswer(t, bus, map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "逐页查询，保持来源顺序。"}}}, false)
	if !out.Success {
		t.Fatal(out.Summary)
	}
	doc := bus.Mutable.AnswerDocumentV2()
	rows := 0
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement != nil {
			rows += len(block.RuntimeMeasurement.BoundTable.Rows)
		}
	}
	if rows != 140 {
		t.Fatalf("native retained output rows=%d; expected140 despite128-row prompt budget", rows)
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
	for i := 0; i < 140; i++ {
		if strings.Count(visible, fmt.Sprintf("item-%03d", i)) != 1 {
			t.Fatalf("record%d omitted or duplicated", i)
		}
	}
}
