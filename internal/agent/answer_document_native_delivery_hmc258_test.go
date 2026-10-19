package agent

import (
	"context"
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

func acceptNativeDisplayHMC258(t *testing.T, ctx *types.AgentContext, selected, display []types.AnswerRuntimeMeasurementReceipt) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"reason": "HMC258 lifecycle", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{
		"runtime_measurements": selected, "answer_display": display, "interpretation": "Independent business clue remains unproven; preserve source and causal explanations.",
	}})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !r.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
		t.Fatalf("completion: %v %s", err, r.Summary)
	}
}

func TestHMC258NativeDisplayLayoutsRemainExactAndIdempotent(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockTable, types.BlockSummary, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := resourcePageContextHMC257(t)
			var selected types.AnswerRuntimeMeasurementReceipt
			for _, r := range tool.NativeFactAnswerDisplayChoices(types.ToolBusContext(ctx, types.AgentExplorer)) {
				copy := r
				types.BindRuntimeMeasurementReceipt(&copy, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract)
				if r.View == types.RuntimeMeasurementMembers && len(copy.BoundTable.Rows) == 8 {
					selected = r
				}
			}
			if selected.ObservationID == "" {
				t.Fatal("eight-frame selector unavailable")
			}
			acceptNativeDisplayHMC258(t, ctx, []types.AnswerRuntimeMeasurementReceipt{selected}, []types.AnswerRuntimeMeasurementReceipt{selected})
			proseKind := types.BlockSummary
			if kind == types.BlockSummary {
				proseKind = types.BlockSection
			}
			blocks := []types.AnswerBlock{{ID: "interpretation", Kind: proseKind, Text: "Independent causal and business interpretation is retained."}, {ID: "native", Kind: kind, RuntimeMeasurement: &selected}}
			raw, _ := json.Marshal(map[string]any{"blocks": blocks})
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || !result.Success {
				t.Fatalf("emit %s: %v %s", kind, err, result.Summary)
			}
			first := render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en")
			result, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["interpretation","native"]}`))
			if err != nil || !result.Success {
				t.Fatalf("patch %s: %v %s", kind, err, result.Summary)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			count := 0
			for _, b := range doc.Blocks {
				if b.RuntimeMeasurement != nil && b.RuntimeMeasurement.ObservationID == selected.ObservationID && b.RuntimeMeasurement.View == selected.View {
					count++
					if b.Kind != kind || len(b.RuntimeMeasurement.BoundTable.Rows) != 8 || len(b.RequestedDimensionIndices) != 0 {
						t.Fatal("layout changed complete selection or acquired dimension ownership")
					}
				}
			}
			if count != 1 || render.RenderAnswerDocument(doc, "en") != first || strings.Contains(first, "-->") {
				t.Fatalf("view duplicated, changed or acquired edges: count=%d", count)
			}
		})
	}
}

func TestHMC258NativeDisplayKeepsPreviewAndInterpretationBoundaries(t *testing.T) {
	ctx := resourcePageContextHMC257(t)
	choices := tool.NativeFactAnswerDisplayChoices(types.ToolBusContext(ctx, types.AgentExplorer))
	acceptNativeDisplayHMC258(t, ctx, choices, choices)
	var publication answerDocNativePresentationReceipt
	renderAnswerDocRuntimeMeasurementChoices(ctx, &publication)
	separated := answerDocSeparatedNativeFactHandoff(ctx, &publication)
	if separated.nativeViewsComplete() || !separated.ownsCurrentNarrative {
		t.Fatal("complete final display pretended all preview notes were published")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil)
	for _, want := range []string{"Independent business clue", "artifact_page", "Accepted native answer display"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("independent context lost %q", want)
		}
	}
	if strings.Contains(prompt, "HMC258 lifecycle") || ctx.Mutable.StableInvestigationCompleteReason() != "HMC258 lifecycle" {
		t.Fatal("lifecycle was replayed as analysis or lost from audit")
	}
	// References without an explicit display choice are not a new layout
	// obligation. The existing producer defaults remain exactly unchanged.
	acceptNativeDisplayHMC258(t, ctx, choices, nil)
	selected, unavailable := types.ResolveInvestigationNativeAnswerDisplay(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), &ctx.AnalysisIR.RequestModel, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract)
	if len(selected) != 0 || unavailable != 0 || strings.Contains(renderAnswerDocRuntimeMeasurementChoices(ctx), "Accepted native answer display") {
		t.Fatal("investigation-only references became a display plan")
	}
}

func TestHMC258NativeDisplayStaleAndRestoredSelection(t *testing.T) {
	for _, mode := range []string{"changed_window", "changed_target", "missing_query", "source_generation", "new_completion", "reset", "json", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx := resourcePageContextHMC257(t)
			choices := tool.NativeFactAnswerDisplayChoices(types.ToolBusContext(ctx, types.AgentExplorer))
			acceptNativeDisplayHMC258(t, ctx, choices, choices)
			original := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
			switch mode {
			case "changed_window":
				start, end := 10.01, 10.03
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = &start
				ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
			case "changed_target":
				ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 999, Source: "user_explicit"}}
			case "missing_query":
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
			case "source_generation":
				for _, name := range []string{"capture.data", "nested/capture.data"} {
					path := filepath.Join(ctx.RepoRoot, "captures", name)
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					data[len(data)-1] ^= 1
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "new_completion":
				ctx.Mutable.SetInvestigationComplete("new lifecycle")
			case "reset":
				ctx.Mutable.ResetTurnAArtifacts()
			case "json":
				wire, _ := json.Marshal(original)
				var restored types.InvestigationNativeFactHandoff
				json.Unmarshal(wire, &restored)
				ctx.Mutable.SetInvestigationComplete("restored")
				ctx.Mutable.AcceptInvestigationNativeFactHandoff(&restored)
			case "cancelled":
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = cancelled
			}
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			prompt := renderAcceptedNativeAnswerDisplay(ctx, view)
			if strings.Contains(prompt, "final_retained_rows=") {
				t.Fatal("stale/restored display regained current facts")
			}
			if mode == "changed_window" || mode == "changed_target" || mode == "missing_query" || mode == "source_generation" {
				if !strings.Contains(prompt, "no longer current: 8") {
					t.Fatalf("stale selection silently disappeared: %s", prompt)
				}
			}
		})
	}
}

func TestHMC258SamplingDisplayUsesSameNativeCohorts(t *testing.T) {
	ctx := nativeFactHandoffContextHMC247(t, true)
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionObservedValue, Required: true}}}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	choices := tool.NativeFactAnswerDisplayChoices(types.ToolBusContext(ctx, types.AgentExplorer))
	if len(choices) == 0 {
		t.Fatal("sampling native choices unavailable")
	}
	acceptNativeDisplayHMC258(t, ctx, choices, choices)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	selected, unavailable := types.ResolveInvestigationNativeAnswerDisplay(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), &ctx.AnalysisIR.RequestModel, view.RuntimeMeasurementContract)
	if unavailable != 0 || len(selected) != len(choices) {
		t.Fatal("sampling display did not retain exact choices")
	}
	raw := json.RawMessage(`{"blocks":[{"id":"interpretation","kind":"summary","text":"Sample weights are not elapsed time; independent source and causal explanations remain."}]}`)
	result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || !result.Success {
		t.Fatalf("sampling emit: %v %s", err, result.Summary)
	}
	seen := 0
	for _, b := range ctx.Mutable.AnswerDocumentV2().Blocks {
		if b.RuntimeMeasurement == nil {
			continue
		}
		for _, expected := range selected {
			if b.RuntimeMeasurement.ObservationID == expected.ObservationID && b.RuntimeMeasurement.View == expected.View {
				if !reflect.DeepEqual(b.RuntimeMeasurement.BoundTable, expected.BoundTable) {
					t.Fatal("sampling identity, independent denominators, units or notes changed")
				}
				seen++
			}
		}
	}
	if seen != len(selected) {
		t.Fatalf("only previews were published: %d/%d", seen, len(selected))
	}
}

func TestHMC258SelectedNativeFactsReachActualAnswer(t *testing.T) {
	ctx := resourcePageContextHMC257(t)
	var selected []types.AnswerRuntimeMeasurementReceipt
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.HasNativeResourceFacts() {
			selected = append(selected, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
		}
	}
	const interpretation = "独立业务解释保留；未知单位和真实展开完整性仍未知。"
	raw, _ := json.Marshal(map[string]any{"reason": "HMC258 lifecycle", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{
		"runtime_measurements": selected, "answer_display": selected, "interpretation": interpretation,
	}})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !r.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
		t.Fatalf("explicit display handoff rejected: %v %s", err, r.Summary)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil)
	if !strings.Contains(prompt, "Accepted native answer display") || !strings.Contains(prompt, interpretation) {
		t.Fatal("actual finalizer lost the exact display plan or independent interpretation")
	}
	raw, _ = json.Marshal(map[string]any{"blocks": []types.AnswerBlock{{ID: "interpretation", Kind: types.BlockSummary, Text: interpretation}}})
	r, err = (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
	if err != nil || !r.Success {
		t.Fatalf("public answer emit: %v %s", err, r.Summary)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	events, frames, empty, queries := 0, 0, 0, 0
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() || !block.RuntimeMeasurement.BoundTable.HasNativeResourceFacts() {
			continue
		}
		table := block.RuntimeMeasurement.BoundTable
		if table.View == types.RuntimeMeasurementSummary {
			queries++
			events += len(table.Rows)
			if len(table.Rows) == 0 {
				empty++
			}
		} else {
			frames += len(table.Rows)
		}
	}
	if queries != 4 || events != 5 || frames != 14 || empty != 1 || doc.Blocks[0].Text != interpretation {
		t.Fatalf("selected view became preview or lost identity: queries=%d events=%d frames=%d empty=%d", queries, events, frames, empty)
	}
	encoded, _ := json.Marshal(doc)
	var recovered types.AnswerDocumentV2
	if json.Unmarshal(encoded, &recovered) != nil || !types.RebindRuntimeAnswerReceipts(&recovered, types.BuildAnswerSemanticViewForAgentContext(ctx)) {
		t.Fatal("persisted native selectors failed current-source rebind")
	}
	visible := render.RenderAnswerDocument(&recovered, "zh")
	for _, want := range []string{interpretation, "原深度", "10005000000", "单位未在该载体声明", "真实展开=未知", "gallery-main", "editor-main"} {
		if !strings.Contains(visible, want) {
			t.Errorf("rendered facts lost %q", want)
		}
	}
	if strings.Contains(visible, "-->") {
		t.Fatal("frame depths became call edges")
	}
}
