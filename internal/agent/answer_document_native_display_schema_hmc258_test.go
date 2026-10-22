package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC258NativeDisplaySchemaAndPublicSelectionBoundary(t *testing.T) {
	ctx := resourcePageContextHMC257(t)
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	choices := tool.NativeFactAnswerDisplayChoices(bus)
	if len(choices) != 12 {
		t.Fatalf("current exact native views=%d", len(choices))
	}
	acceptNativeDisplayHMC258(t, ctx, choices, choices[:1])
	accepted := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	readSchema := func() map[string]json.RawMessage {
		t.Helper()
		base := []llm.ToolSchema{{Name: "emit_investigation_complete", Parameters: (&tool.EmitInvestigationComplete{}).Parameters()}}
		projected := projectExplorerNativeFactHandoffSchema(ctx, base)
		var root struct {
			Properties map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"properties"`
		}
		if json.Unmarshal(projected[0].Parameters, &root) != nil {
			t.Fatal("schema decode")
		}
		h := root.Properties["native_fact_handoff"]
		for _, required := range h.Required {
			if required == "answer_display" {
				t.Fatal("optional display became required")
			}
		}
		return h.Properties
	}
	fields := readSchema()
	var display struct {
		MaxItems    int    `json:"maxItems"`
		Description string `json:"description"`
		Items       struct {
			Enum []types.AnswerRuntimeMeasurementReceipt `json:"enum"`
		} `json:"items"`
	}
	if json.Unmarshal(fields["answer_display"], &display) != nil || display.MaxItems != 16 || display.Description != types.NativeAnswerDisplayTeaching || !reflect.DeepEqual(display.Items.Enum, choices) {
		t.Fatal("dynamic optional schema drifted from bounded current selector/teaching contract")
	}
	for _, mode := range []string{"not_selected", "duplicate", "unknown", "wrong_view", "event_subset", "outside_domain", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			selected, output := choices[:1], append([]types.AnswerRuntimeMeasurementReceipt(nil), choices[:1]...)
			originalProfile, originalDimensions, originalContext := ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile, ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions, ctx.Ctx
			defer func() {
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile, ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions, ctx.Ctx = originalProfile, originalDimensions, originalContext
			}()
			switch mode {
			case "not_selected":
				output = choices[1:2]
			case "duplicate":
				output = append(output, output[0])
			case "unknown":
				output[0].ObservationID += "-foreign"
			case "wrong_view":
				output[0].View = types.RuntimeMeasurementView("not_a_view")
			case "event_subset":
				output[0].EventSelection = &types.RuntimeEventSelection{}
			case "outside_domain":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
				ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
				if _, exists := readSchema()["answer_display"]; exists {
					t.Fatal("reference-only domain advertises final display")
				}
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = c
				if _, exists := readSchema()["answer_display"]; exists {
					t.Fatal("cancelled turn advertises final display")
				}
			}
			raw, _ := json.Marshal(map[string]any{"reason": "new rejected lifecycle", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": selected, "answer_display": output, "interpretation": "preserve independent analysis"}})
			r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
			if err != nil || r.Success || !strings.Contains(r.Summary, "answer_display") {
				t.Fatalf("unsupported display accepted or wrong repair: %v %s", err, r.Summary)
			}
			if !reflect.DeepEqual(accepted, ctx.Mutable.AcceptedInvestigationNativeFactHandoff()) {
				t.Fatal("failed display attempt replaced accepted generation")
			}
		})
	}
}
