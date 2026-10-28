package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc303BusinessReference(t *testing.T) (*types.AgentContext, types.AnswerRuntimeMeasurementReceipt) {
	t.Helper()
	ctx := hmosBusinessIOFinalizerContext(t)
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "OpenDocument") {
			if len(table.Rows) != 13 || table.Rows[6][1] != "5" || table.Rows[8][1] != "44" {
				t.Fatalf("public business-window facts changed: %+v", table.Rows)
			}
			return ctx, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
		}
	}
	t.Fatal("public query did not publish the business-local table")
	return nil, types.AnswerRuntimeMeasurementReceipt{}
}

func hmc303Complete(t *testing.T, ctx *types.AgentContext, selected types.AnswerRuntimeMeasurementReceipt, ref map[string]any) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"reason":     "Investigation complete; native facts remain separate from interpretation.",
		"confidence": "high", "result_kind": "resolved",
		"native_fact_handoff": map[string]any{
			"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{selected},
			"interpretation_role":  "independent_analysis",
			"interpretation_units": []any{map[string]any{
				"text":        "The business path suggests checking downstream delivery; this is an independent follow-up, not a new measurement.",
				"boundaries":  []string{"A row reference does not validate this hypothesis or authorize a causal edge."},
				"native_refs": []any{ref},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestHMC303PublicInterpretationKeepsExactNativeRowReference(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	projectionBefore := string(hmc218CausalProjection(t, ctx))
	if projectionBefore == "null" || projectionBefore == "{}" || len(projectionBefore) < 100 {
		t.Fatal("fixture lacks the causal projection protected by this handoff")
	}
	ref := map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{0, 1, 2, 3, 4, 6, 7, 8}}
	r := hmc303Complete(t, ctx, selected, ref)
	if !r.Success {
		t.Fatalf("public completion rejected valid row address: %s", r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	if h == nil {
		t.Fatal("completion did not retain current private handoff")
	}
	encoded, _ := json.Marshal(h)
	if !strings.Contains(string(encoded), `"native_refs"`) || !strings.Contains(string(encoded), `"row_indices":[0,1,2,3,4,6,7,8]`) {
		t.Fatalf("accepted public completion discarded exact native row references: %s", encoded)
	}
	actual := dependencyObservationMessages(t, ctx.ShallowClone())
	if !strings.Contains(actual, `"native_refs"`) || !strings.Contains(actual, `"row_indices":[0,1,2,3,4,6,7,8]`) || !strings.Contains(actual, "native_reference_status=current_exact_rows") {
		t.Fatal("native reference and interpretation did not travel together to actual Finalizer INIT")
	}
	if projectionBefore != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("reference handoff changed the original causal projection")
	}
	current := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	if !types.BindRuntimeMeasurementReceipt(&selected, current) || selected.BoundTable.Rows[6][1] != "5" || selected.BoundTable.Rows[8][1] != "44" {
		t.Fatal("reference handoff changed the native business-local rows")
	}
}

func TestHMC303PublicInterpretationRejectsInvalidNativeRowAddress(t *testing.T) {
	for _, mode := range []string{"unknown_id", "wrong_view", "negative", "out_of_range", "duplicate", "empty", "unselected"} {
		t.Run(mode, func(t *testing.T) {
			ctx, selected := hmc303BusinessReference(t)
			ref := map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}
			switch mode {
			case "unknown_id":
				ref["observation_id"] = "unknown-native-source"
			case "wrong_view":
				ref["view"] = "distribution"
			case "negative":
				ref["row_indices"] = []int{-1}
			case "out_of_range":
				ref["row_indices"] = []int{13}
			case "duplicate":
				ref["row_indices"] = []int{6, 6}
			case "empty":
				ref["row_indices"] = []int{}
			case "unselected":
				found := false
				for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
					if table.View == selected.View && table.ObservationID != selected.ObservationID && len(table.Rows) > 6 {
						ref["observation_id"] = table.ObservationID
						found = true
						break
					}
				}
				if !found {
					t.Fatal("fixture lacks an independent native table")
				}
			}
			r := hmc303Complete(t, ctx, selected, ref)
			if r.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatalf("invalid native address accepted as current handoff: %s", mode)
			}
		})
	}
}

// Reproduce the old legal carrier without treating unsupported prose as a
// machine-checkable measurement. Correct row delivery cannot verify this text.
func TestHMC303PublicLegacyScopeConflictRemainsAdvisory(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	const oldClaim = "OpenDocument 的 50ms 中，主线程只有 7ms 在真正执行、1ms 可运行，其余 44ms 全部是 S 状态等待（1.001000..1.045000）；主线程本身没有 D 状态或调度器标记的 IO 等待，它的慢完全来自“等下游交付”。"
	raw, _ := json.Marshal(map[string]any{
		"reason": "Runtime investigation complete.", "confidence": "high", "result_kind": "resolved",
		"native_fact_handoff": map[string]any{"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{selected}, "interpretation_role": "independent_analysis",
			"interpretation_units": []any{map[string]any{"text": oldClaim, "support_refs": []string{"trace_query:window_stats target_window_states"}}}},
	})
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !r.Success {
		t.Fatalf("legacy advisory rejected: %v %s", err, r.Summary)
	}
	actual := dependencyObservationMessages(t, ctx.ShallowClone())
	if !strings.Contains(actual, oldClaim) || !strings.Contains(actual, "proposed locators, not verified evidence") {
		t.Fatal("legacy independent explanation was rewritten or lost its boundary")
	}
	if strings.Contains(actual, "native_reference_status=current_exact_rows") {
		t.Fatal("free support_refs acquired native row binding")
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	if len(h.InterpretationUnits[0].NativeRefs) != 0 {
		t.Fatal("system inferred native support from prose")
	}
	t.Log("The original HMC302 unit text remains advisory and requires human audit; this test does not certify its scope or conclusion.")
}

func TestHMC303NativeReferencesRebindExactPrivateSourceAndTable(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
		t.Fatal(r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	if !types.InvestigationNativeRowReferencesCurrent(h, contract) {
		t.Fatal("accepted current reference not bound")
	}
	for _, mode := range []string{"json", "row", "notes", "unit", "scope", "query", "memo", "clone"} {
		t.Run(mode, func(t *testing.T) {
			copy := h.Clone()
			c := *contract
			c.Tables = contract.Choices()
			index := -1
			for i := range c.Tables {
				if c.Tables[i].ObservationID == selected.ObservationID && c.Tables[i].View == selected.View {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatal("selected table not retained")
			}
			switch mode {
			case "json":
				raw, _ := json.Marshal(h)
				copy = &types.InvestigationNativeFactHandoff{}
				if err := json.Unmarshal(raw, copy); err != nil {
					t.Fatal(err)
				}
			case "row":
				c.Tables[index].Rows[6][1] = "7"
			case "notes":
				c.Tables[index].Notes = append(c.Tables[index].Notes, "changed scope")
			case "unit":
				c.Tables[index].Columns[1] += " seconds"
			case "scope":
				copy.InterpretationUnits[0].NativeRefs[0].RowIndices = []int{7}
			case "query":
				copy.InterpretationUnits[0].NativeRefs[0].ObservationID += "-other-query"
			case "memo":
				raw, _ := json.Marshal(c.Tables[index])
				c.Tables[index] = types.RuntimeMeasurementTable{}
				if err := json.Unmarshal(raw, &c.Tables[index]); err != nil {
					t.Fatal(err)
				}
			case "clone":
				copy.InterpretationUnits[0].NativeRefs[0].RowIndices[0] = 7
				if h.InterpretationUnits[0].NativeRefs[0].RowIndices[0] != 6 {
					t.Fatal("clone aliases native row indices")
				}
			}
			if types.InvestigationNativeRowReferencesCurrent(copy, &c) {
				t.Fatal("changed or restored reference retained current binding")
			}
		})
	}
}

func TestHMC303NativeReferenceWholeUnitBudget(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
		t.Fatal(r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	identity := func(s string) string { return s }
	full := previewInvestigationInterpretationWithNativeReferences(h, 99999, identity, true)
	for _, budget := range []int{0, full.publishedBytes - 1, full.publishedBytes} {
		got := previewInvestigationInterpretationWithNativeReferences(h, budget, identity, true)
		visible := strings.Contains(got.text, `"text"`)
		if visible != strings.Contains(got.text, `"native_refs"`) || visible != strings.Contains(got.text, "native_reference_status=current_exact_rows") {
			t.Fatal("unit, native address and validation boundary split by budget")
		}
		if got.publishedBytes > budget {
			t.Fatal("native reference escaped narrative budget")
		}
		if (budget == full.publishedBytes) != visible {
			t.Fatalf("whole unit budget boundary wrong: %d", budget)
		}
	}
}

func TestHMC303PublicNativeReferenceSourceGeneration(t *testing.T) {
	ctx, _ := hmc303BusinessReference(t)
	source, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "events.systrace")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	ctx.Mutable = types.NewMutableState("Explain the observed business response and its dependency waits")
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 0.999, "time_end": 1.051, "trace_flavor": "harmony_hitrace"})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("public native query failed: %v %s", err, r.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	var selected types.AnswerRuntimeMeasurementReceipt
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementSummary && strings.Contains(table.Label, "OpenDocument") {
			selected = types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
			break
		}
	}
	if selected.ObservationID == "" {
		t.Fatal("private source has no business fact")
	}
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
		t.Fatal(r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	if !types.InvestigationNativeRowReferencesCurrent(h, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract) {
		t.Fatal("fresh source did not bind")
	}
	if err := os.WriteFile(path, append(source, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if types.InvestigationNativeRowReferencesCurrent(h, types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract) {
		t.Fatal("changed physical source retained native row binding")
	}
	if strings.Contains(dependencyObservationMessages(t, ctx.ShallowClone()), "native_reference_status=current_exact_rows") {
		t.Fatal("actual Finalizer promoted stale original-source references")
	}
}

func TestHMC303ActualExplorerSchemaNativeRanges(t *testing.T) {
	ctx, _ := hmc303BusinessReference(t)
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	reg.Register(&tool.EmitInvestigationComplete{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture native row reference schema")}
	sk := traceTeachingSkill(t, "explore-skill")
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete")
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual Explorer capture failed: %v", err)
	}
	for _, schema := range capture.tools {
		if schema.Name != "emit_investigation_complete" {
			continue
		}
		var root map[string]any
		if err := json.Unmarshal(schema.Parameters, &root); err != nil {
			t.Fatal(err)
		}
		properties := root["properties"].(map[string]any)["native_fact_handoff"].(map[string]any)["properties"].(map[string]any)
		refs := properties["interpretation_units"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["native_refs"].(map[string]any)
		items := refs["items"].(map[string]any)
		baseRows := items["properties"].(map[string]any)["row_indices"].(map[string]any)
		if !reflect.DeepEqual(baseRows["items"], map[string]any{"type": "integer", "minimum": float64(0)}) || baseRows["minItems"] != float64(1) || baseRows["maxItems"] != float64(types.MaxInvestigationNativeRowIndices) || baseRows["uniqueItems"] != true {
			t.Fatal("shared reference shape lost row count/type/uniqueness bounds")
		}
		raw, _ := json.Marshal(items)
		if len(raw) > nativePresentationNavigationByteLimit {
			t.Fatal("native reference schema exceeded complete-branch budget")
		}
		branches := items["oneOf"].([]any)
		if len(branches) == 0 {
			t.Fatal("current native rows lack exact schema choices")
		}
		contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
		for _, branch := range branches {
			p := branch.(map[string]any)["properties"].(map[string]any)
			r := types.AnswerRuntimeMeasurementReceipt{ObservationID: p["observation_id"].(map[string]any)["enum"].([]any)[0].(string), View: types.RuntimeMeasurementView(p["view"].(map[string]any)["enum"].([]any)[0].(string))}
			if !types.BindRuntimeMeasurementReceipt(&r, contract) {
				t.Fatal("advertised unknown pair")
			}
			limits := p["row_indices"].(map[string]any)["items"].(map[string]any)
			if !reflect.DeepEqual(limits, map[string]any{"maximum": float64(len(r.BoundTable.Rows) - 1)}) || len(r.BoundTable.Rows) == 0 {
				t.Fatal("advertised row range differs from original native table")
			}
		}
		t.Logf("actual current-reference item schema: %d bytes, %d complete pair/range branches", len(raw), len(branches))
		return
	}
	t.Fatal("actual Explorer omitted completion schema")
}
