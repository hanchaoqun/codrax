package agent

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC333PublicNativeReferenceContextAcrossStages(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	before := string(hmc218CausalProjection(t, ctx))
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
		t.Fatal(r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	if !types.BindRuntimeMeasurementReceipt(&selected, contract) {
		t.Fatal("public native selector did not bind")
	}
	source, ok := selected.BoundTable.NativeSourceTitleBasis()
	if !ok {
		t.Fatal("fixture lacks current native source context")
	}
	unit, _ := json.Marshal(h.InterpretationUnits[0])
	t.Logf("original accepted unit JSON: %d bytes; label=%q columns=%q", len(unit), selected.BoundTable.Label, selected.BoundTable.Columns)
	for stage, prompt := range map[string]string{
		"extract":  (&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil),
		"finalize": dependencyObservationMessages(t, ctx.ShallowClone()),
	} {
		t.Run(stage, func(t *testing.T) {
			if !strings.Contains(prompt, string(unit)) {
				t.Fatal("whole accepted interpretation or its boundaries disappeared")
			}
			var found bool
			for _, line := range strings.Split(prompt, "\n") {
				line = strings.TrimSpace(line)
				const prefix = "native_reference_context="
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				var contexts []struct {
					ReferenceIndex int      `json:"reference_index"`
					Label          string   `json:"label"`
					Columns        []string `json:"columns"`
					OriginalSource string   `json:"original_source"`
					QueryWindow    string   `json:"query_window"`
					QuerySelection string   `json:"query_selection"`
					Role           string   `json:"role"`
					RoleBasis      string   `json:"role_basis"`
				}
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &contexts); err != nil {
					t.Fatal(err)
				}
				if len(contexts) == 1 && contexts[0].ReferenceIndex == 0 && contexts[0].Label == selected.BoundTable.Label && reflect.DeepEqual(contexts[0].Columns, selected.BoundTable.Columns) && contexts[0].OriginalSource == source.OriginalSource && contexts[0].QueryWindow == source.QueryWindow && contexts[0].QuerySelection == source.QuerySelection && contexts[0].Role == source.Role && contexts[0].RoleBasis == source.RoleBasis {
					found = true
				}
			}
			if !found {
				t.Fatal("current unit only carries opaque row IDs; its producer-owned subject, columns and source context were not handed off beside it")
			}
			if !strings.Contains(prompt, "Address validation does not validate text, arithmetic, causality or diagram notes") {
				t.Fatal("source context acquired explanation or causal authority")
			}
		})
	}
	after, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff().InterpretationUnits[0])
	if string(unit) != string(after) || before != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("presentation changed accepted prose or Trace causal projection")
	}
}

func TestHMC333ActualExplorerReferenceChoicesDescribeNativeScope(t *testing.T) {
	ctx, _ := hmc303BusinessReference(t)
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	reg.Register(&tool.EmitInvestigationComplete{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture reference context, not a live model")}
	sk := traceTeachingSkill(t, "explore-skill")
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete")
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual schema capture failed: %v", err)
	}
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	for _, schema := range capture.tools {
		if schema.Name != "emit_investigation_complete" {
			continue
		}
		var root map[string]any
		if err := json.Unmarshal(schema.Parameters, &root); err != nil {
			t.Fatal(err)
		}
		p := root["properties"].(map[string]any)["native_fact_handoff"].(map[string]any)["properties"].(map[string]any)
		items := p["interpretation_units"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["native_refs"].(map[string]any)["items"].(map[string]any)
		encoded, _ := json.Marshal(items)
		if len(encoded) > nativePresentationNavigationByteLimit {
			t.Fatal("whole reference-choice schema exceeded its existing budget")
		}
		branches := items["oneOf"].([]any)
		if len(branches) == 0 {
			t.Fatal("no current reference choices")
		}
		for _, value := range branches {
			branch := value.(map[string]any)
			fields := branch["properties"].(map[string]any)
			ref := types.AnswerRuntimeMeasurementReceipt{ObservationID: fields["observation_id"].(map[string]any)["enum"].([]any)[0].(string), View: types.RuntimeMeasurementView(fields["view"].(map[string]any)["enum"].([]any)[0].(string))}
			if !types.BindRuntimeMeasurementReceipt(&ref, contract) {
				t.Fatal("advertised unknown reference")
			}
			description, _ := branch["description"].(string)
			var scope struct {
				Label          string   `json:"label"`
				Columns        []string `json:"columns"`
				OriginalSource string   `json:"original_source"`
				QueryWindow    string   `json:"query_window"`
				QuerySelection string   `json:"query_selection"`
				Role           string   `json:"role"`
				RoleBasis      string   `json:"role_basis"`
			}
			source, ok := ref.BoundTable.NativeSourceTitleBasis()
			if json.Unmarshal([]byte(description), &scope) != nil || !ok || scope.Label != ref.BoundTable.Label || !reflect.DeepEqual(scope.Columns, ref.BoundTable.Columns) || scope.OriginalSource != source.OriginalSource || scope.QueryWindow != source.QueryWindow || scope.QuerySelection != source.QuerySelection || scope.Role != source.Role || scope.RoleBasis != source.RoleBasis {
				t.Errorf("opaque choice lacks its own exact native identity/columns/scope: %s %s", ref.ObservationID, ref.View)
			}
		}
		return
	}
	t.Fatal("completion schema was not captured")
}

func TestHMC333ReferenceContextNeverDisplacesLaterWholeUnits(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
		t.Fatal(r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	contexts := investigationNativeReferenceContexts(h, contract)
	if len(contexts[0]) != 1 {
		t.Fatal("current public handoff lacks source context")
	}
	// A second independent business clue is not allowed to lose its seat to
	// an optional descriptor for the first finding. This is not source sealing.
	h.InterpretationUnits = append(h.InterpretationUnits, types.InvestigationInterpretationUnit{Text: "Check the independent delivery path.", Boundaries: []string{"No dependency is proven by that clue."}})
	identity := func(s string) string { return s }
	base := previewInvestigationInterpretationWithNativeReferences(h, 99999, identity, true)
	full := previewInvestigationInterpretationWithReferenceContexts(h, 99999, identity, true, contexts)
	for _, budget := range []int{0, 1, base.publishedBytes - 1, base.publishedBytes, full.publishedBytes - 1, full.publishedBytes} {
		plain := previewInvestigationInterpretationWithNativeReferences(h, budget, identity, true)
		got := previewInvestigationInterpretationWithReferenceContexts(h, budget, identity, true, contexts)
		if got.publishedBytes > budget || got.totalBytes != full.totalBytes {
			t.Fatalf("budget/whole-descriptor accounting changed at %d", budget)
		}
		for i, unit := range h.InterpretationUnits {
			encoded, _ := json.Marshal(unit)
			if strings.Contains(got.text, string(encoded)) != strings.Contains(plain.text, string(encoded)) {
				t.Fatalf("optional descriptor displaced unit %d at budget %d", i, budget)
			}
		}
		if budget == full.publishedBytes && (!strings.Contains(got.text, "native_reference_context=") || got.contextOmittedUnits != 0) {
			t.Fatal("roomy budget did not deliver full context")
		}
		if budget == base.publishedBytes && (!strings.Contains(got.text, "context is compact or omitted") || got.contextOmittedUnits != 1) {
			t.Fatal("omitted optional scope context was not disclosed")
		}
	}
}

func TestHMC333NativeContextCannotRestoreAuthority(t *testing.T) {
	ctx, selected := hmc303BusinessReference(t)
	if r := hmc303Complete(t, ctx, selected, map[string]any{"observation_id": selected.ObservationID, "view": selected.View, "row_indices": []int{6}}); !r.Success {
		t.Fatal(r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	if len(investigationNativeReferenceContexts(h, contract)) == 0 {
		t.Fatal("fresh context missing")
	}
	for _, mode := range []string{"json", "table_label", "columns", "row_indices", "duplicate_table", "unit_without_ref"} {
		t.Run(mode, func(t *testing.T) {
			copy := h.Clone()
			c := contract.Clone()
			switch mode {
			case "json":
				b, _ := json.Marshal(h)
				copy = &types.InvestigationNativeFactHandoff{}
				if err := json.Unmarshal(b, copy); err != nil {
					t.Fatal(err)
				}
			case "row_indices":
				copy.InterpretationUnits[0].NativeRefs[0].RowIndices = []int{7}
			case "unit_without_ref":
				copy.InterpretationUnits[0].NativeRefs = nil
			default:
				for i := range c.Tables {
					if c.Tables[i].ObservationID != selected.ObservationID || c.Tables[i].View != selected.View {
						continue
					}
					switch mode {
					case "table_label":
						c.Tables[i].Label += " changed"
					case "columns":
						c.Tables[i].Columns[1] += " seconds"
					case "duplicate_table":
						c.Tables = append(c.Tables, c.Tables[i].Clone())
					}
					break
				}
			}
			if len(investigationNativeReferenceContexts(copy, c)) != 0 {
				t.Fatal("changed or restored selector minted current scope context")
			}
		})
	}
}
