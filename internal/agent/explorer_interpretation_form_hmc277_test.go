package agent

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC277ActualExplorerSchemaAllowsUnitsOnlyCompletion(t *testing.T) {
	ctx := nativeFactHandoffContextHMC247(t, false)
	ctx.AgentName, ctx.Stage = types.AgentExplorer, types.StageExplore
	reg := tool.NewRegistry()
	tool.RegisterDefaults(reg)
	reg.Register(&tool.EmitInvestigationComplete{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture actual units schema")}
	sk := traceTeachingSkill(t, "explore-skill")
	sk.ToolSuggestions = append(sk.ToolSuggestions, "emit_investigation_complete")
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, sk)
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual schema capture: %v calls=%d", err, capture.calls)
	}
	found := false
	for _, schema := range capture.tools {
		if schema.Name != "emit_investigation_complete" {
			continue
		}
		var root struct {
			Properties map[string]struct {
				Required   []string
				AnyOf      []json.RawMessage
				Properties map[string]struct {
					Items struct {
						Enum []types.AnswerRuntimeMeasurementReceipt
					}
				}
			}
		}
		if err := json.Unmarshal(schema.Parameters, &root); err != nil {
			t.Fatal(err)
		}
		h := root.Properties["native_fact_handoff"]
		if slices.Contains(h.Required, "interpretation") || !slices.Contains(h.Required, "interpretation_role") || len(h.AnyOf) != 2 {
			t.Errorf("actual schema restored legacy requirement or lost role/alternatives: required=%v anyOf=%v", h.Required, h.AnyOf)
		}
		choices := h.Properties["runtime_measurements"].Items.Enum
		if len(choices) == 0 {
			t.Fatal("no current exact choices")
		}
		units := []types.InvestigationInterpretationUnit{{Text: "Independent finding", Boundaries: []string{"Relationship unproven"}}}
		raw, _ := json.Marshal(map[string]any{"reason": "Lifecycle only", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": choices[:1], "interpretation_role": "independent_analysis", "interpretation_units": units}})
		r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !r.Success {
			t.Fatalf("actual offered selection and units rejected: %v %s", err, r.Summary)
		}
		if h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff(); h == nil || len(h.InterpretationUnits) != 1 || h.Interpretation != "" {
			t.Fatal("units-only schema input did not reach accepted current state")
		}
		found = true
	}
	if !found {
		t.Fatal("completion schema not published")
	}
}
