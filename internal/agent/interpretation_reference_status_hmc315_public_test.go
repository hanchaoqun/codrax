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

// Query and completion use their public tools. Read the real extractor initial
// instruction and finalizer dependency messages, not just the preview helper.
func TestHMC315PublicNativeReferenceStatusAcrossStages(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	beforeProjection := string(hmc218CausalProjection(t, ctx))
	r := hmc303Complete(t, ctx, selected, map[string]any{
		"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6},
	})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	if !types.InvestigationNativeRowReferencesCurrent(h, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract) {
		t.Fatal("public completion did not seal the current source and native rows")
	}
	original, _ := json.Marshal(h)
	stages := map[string]string{
		"extract":  (&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil),
		"finalize": dependencyObservationMessages(t, ctx.ShallowClone()),
	}
	for name, prompt := range stages {
		t.Run(name, func(t *testing.T) {
			unit, _ := json.Marshal(h.InterpretationUnits[0])
			if !strings.Contains(prompt, string(unit)) {
				t.Fatal("actual stage instruction did not receive the complete accepted unit")
			}
			if !strings.Contains(prompt, "native_reference_status=current_exact_rows") || strings.Contains(prompt, "native_reference_status=unverified_address") {
				t.Fatal("the same current private row binding has contradictory stage status")
			}
			if !strings.Contains(prompt, "Address validation does not validate text, arithmetic, causality or diagram notes") {
				t.Fatal("row address acquired explanation or relation authority")
			}
		})
	}
	after, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
	if string(after) != string(original) || beforeProjection != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("display status changed accepted material or the causal projection")
	}
}

func TestHMC315PublicUnboundUnitsKeepIndependentAnalysis(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	units := []types.InvestigationInterpretationUnit{
		{Text: "Check the delivery path; this is not a proved cause.", NativeRefs: []types.InvestigationNativeRowReference{{ObservationID: selected.ObservationID, View: selected.View, RowIndices: []int{6}}}},
		{Text: "A business clue from another source remains useful.", Boundaries: []string{"The proposed locator is not native evidence."}, SupportRefs: []string{"events.systrace:10-16"}},
		{Text: "1 + 1 = 2 is valid arithmetic; its operands still need the appropriate source and measure.", Boundaries: []string{"This calculation alone does not establish a causal interval."}},
	}
	raw, _ := json.Marshal(map[string]any{
		"reason": "HMC315 independent investigation complete.", "confidence": "high", "result_kind": "resolved",
		"native_fact_handoff": map[string]any{"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{selected}, "interpretation_role": "independent_analysis", "interpretation_units": units},
	})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !r.Success {
		t.Fatalf("public completion: %v %s", err, r.Summary)
	}
	prompt := dependencyObservationMessages(t, ctx.ShallowClone())
	for i, unit := range units {
		encoded, _ := json.Marshal(unit)
		if !strings.Contains(prompt, string(encoded)) {
			t.Fatalf("independent unit %d or its boundaries were removed", i)
		}
	}
	if strings.Count(prompt, "native_reference_status=current_exact_rows") != 1 || strings.Count(prompt, "native_reference_status=no_native_row_address") != 2 {
		t.Fatal("unbound units do not disclose their own status independently of the bound unit")
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	if len(h.InterpretationUnits[1].NativeRefs) != 0 || len(h.InterpretationUnits[2].NativeRefs) != 0 {
		t.Fatal("display inferred native references from a locator, number or business clue")
	}
}

func TestHMC315PublicReferenceStatusCannotRestoreCurrentAuthority(t *testing.T) {
	for _, mode := range []string{"source_generation", "json"} {
		t.Run(mode, func(t *testing.T) {
			ctx, _ := hmc303BusinessReference(t)
			original, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "events.systrace")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			ctx.Mutable = types.NewMutableState("Explain the current business dependency wait")
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 0.999, "time_end": 1.051, "trace_flavor": "harmony_hitrace"})
			query, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
			if err != nil || !query.Success {
				t.Fatalf("public query: %v %s", err, query.Summary)
			}
			ctx.Mutable.AppendDispatchToolResult(query)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
			var selected types.AnswerRuntimeMeasurementReceipt
			for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
				if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "OpenDocument") {
					selected = types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
					break
				}
			}
			if selected.ObservationID == "" {
				t.Fatal("public query has no current business window")
			}
			if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
				t.Fatal(r.Summary)
			}
			if !strings.Contains((&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil), "native_reference_status=current_exact_rows") {
				t.Fatal("fresh public completion did not reach the actual Extract instruction")
			}
			if mode == "source_generation" {
				if err := os.WriteFile(path, append(original, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				ta := ctx.Mutable.TurnAArtifacts()
				ta.AcceptedClosureReason = ctx.Mutable.StableInvestigationCompleteReason()
				ta.AcceptedNativeFactHandoff = ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
				raw, _ := json.Marshal(ta)
				var restored types.TurnAArtifacts
				if err := json.Unmarshal(raw, &restored); err != nil {
					t.Fatal(err)
				}
				ctx.Mutable = types.NewMutableState("Restored advisory material")
				ctx.Mutable.SetTurnAArtifacts(restored)
				ctx.Mutable.SetInvestigationComplete(restored.AcceptedClosureReason)
			}
			for stage, prompt := range map[string]string{
				"extract":  (&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil),
				"finalize": dependencyObservationMessages(t, ctx.ShallowClone()),
			} {
				if strings.Contains(prompt, "native_reference_status=current_exact_rows") {
					t.Fatalf("%s promoted changed or restored material to a current private row binding", stage)
				}
			}
		})
	}
}

func TestHMC315ReferenceStatusSharesWholeUnitBudget(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
		t.Fatal(r.Summary)
	}
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	for _, mode := range []string{"current_exact_rows", "no_native_row_address", "unverified_address"} {
		t.Run(mode, func(t *testing.T) {
			h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
			switch mode {
			case "no_native_row_address":
				h.InterpretationUnits[0].NativeRefs = nil
			case "unverified_address":
				raw, _ := json.Marshal(h)
				h = &types.InvestigationNativeFactHandoff{}
				if err := json.Unmarshal(raw, h); err != nil {
					t.Fatal(err)
				}
			}
			current := types.InvestigationNativeRowReferencesCurrent(h, contract)
			identity := func(s string) string { return s }
			full := previewInvestigationInterpretationWithNativeReferences(h, 99999, identity, current)
			encoded, _ := json.Marshal(h.InterpretationUnits[0])
			for _, budget := range []int{0, 1, full.publishedBytes - 1, full.publishedBytes} {
				got := previewInvestigationInterpretationWithNativeReferences(h, budget, identity, current)
				want := budget == full.publishedBytes
				if strings.Contains(got.text, "- unit[0]: "+string(encoded)) != want || strings.Contains(got.text, "native_reference_status="+mode) != want {
					t.Fatalf("budget %d split original unit JSON and its binding status", budget)
				}
				if got.publishedBytes > budget || got.totalBytes != full.totalBytes {
					t.Fatal("status escaped the original budget or omission accounting")
				}
			}
		})
	}
}
