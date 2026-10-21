package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC276InterpretationUnitShapeAndSchema(t *testing.T) {
	for _, mode := range []string{"legacy", "valid", "blank", "limits", "references", "count"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			choices := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()
			u := types.InvestigationInterpretationUnit{Text: "candidate only", Boundaries: []string{"unknown status"}, SupportRefs: []string{"proposed:7"}}
			units := []types.InvestigationInterpretationUnit{u}
			switch mode {
			case "legacy":
				units = nil
			case "blank":
				units[0].Text = " "
			case "limits":
				units[0].Boundaries = make([]string, 9)
			case "references":
				units[0].SupportRefs = []string{" "}
			case "count":
				units = make([]types.InvestigationInterpretationUnit, 17)
				for i := range units {
					units[i] = u
				}
			}
			selection := types.AnswerRuntimeMeasurementReceipt{ObservationID: choices[0].ObservationID, View: choices[0].View}
			raw, _ := json.Marshal(map[string]any{"reason": "done", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{selection}, "interpretation": "", "interpretation_units": units}})
			r, err := (&EmitInvestigationComplete{}).Execute(ctx, raw)
			valid := mode == "legacy" || mode == "valid"
			if valid != (err == nil && r.Success) {
				t.Fatalf("shape result: %v %s", err, r.Summary)
			}
			if !valid && ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatal("invalid unit accepted")
			}
		})
	}
	var root struct {
		Properties map[string]struct {
			Required   []string
			Properties map[string]json.RawMessage
		}
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &root); err != nil {
		t.Fatal(err)
	}
	h := root.Properties["native_fact_handoff"]
	if strings.Contains(strings.Join(h.Required, ","), "interpretation_units") {
		t.Fatal("new optional field forces retries")
	}
	if !strings.Contains(string(h.Properties["interpretation_units"]), "support_refs") || !strings.Contains(string(h.Properties["interpretation_units"]), "omitted whole") {
		t.Fatal("shared schema lost atomic semantics")
	}
}
