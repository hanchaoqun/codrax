package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC247NativeFactHandoffPublicValidation(t *testing.T) {
	for _, mode := range []string{"valid", "empty_interpretation", "missing_interpretation", "unknown_id", "wrong_view", "duplicate", "event_selection", "copied_values", "stale_source", "no_contract"} {
		t.Run(mode, func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			c := types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract
			if len(c.Choices()) == 0 {
				t.Fatal("no public native table")
			}
			table := c.Choices()[0]
			r := map[string]any{"observation_id": table.ObservationID, "view": table.View}
			h := map[string]any{"runtime_measurements": []any{r}, "interpretation": "Independent hypothesis remains unproven."}
			switch mode {
			case "empty_interpretation":
				h["interpretation"] = ""
			case "missing_interpretation":
				delete(h, "interpretation")
			case "unknown_id":
				r["observation_id"] = "unpublished"
			case "wrong_view":
				r["view"] = "unknown"
			case "duplicate":
				h["runtime_measurements"] = []any{r, r}
			case "event_selection":
				r["event_selection"] = map[string]any{}
			case "copied_values":
				r["value"] = 999
			case "stale_source", "no_contract":
				results := ctx.Mutable.DispatchToolResults()
				ctx.Mutable = types.NewMutableState("new consumer")
				if mode == "stale_source" {
					wire, _ := json.Marshal(results)
					json.Unmarshal(wire, &results)
					for i := range results {
						for j := range results[i].Observations {
							results[i].Observations[j].SourceRef.Path += "-different"
						}
					}
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
				}
			}
			ctx.Mutable.SetInvestigationComplete("prior")
			raw, _ := json.Marshal(map[string]any{"reason": "lifecycle", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": h})
			result, err := (&EmitInvestigationComplete{}).Execute(ctx, raw)
			valid := mode == "valid" || mode == "empty_interpretation"
			if valid {
				if err != nil || !result.Success || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() == nil {
					t.Fatalf("valid native handoff rejected: %v %s", err, result.Summary)
				}
			} else if result.Success || ctx.Mutable.StableInvestigationCompleteReason() != "prior" || ctx.Mutable.AcceptedInvestigationNativeFactHandoff() != nil {
				t.Fatalf("invalid handoff altered accepted state: %v %s", err, result.Summary)
			}
		})
	}
}

func TestHMC247NativeFactHandoffSchemaBoundary(t *testing.T) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	h := string(schema.Properties["native_fact_handoff"])
	for _, word := range []string{"observation_id", "interpretation", "post-budget", "does not waive", "negative finding", "source explanation"} {
		if !strings.Contains(h, word) {
			t.Errorf("missing schema contract %q", word)
		}
	}
	if strings.Contains((&EmitInvestigationComplete{}).Description(), "native_fact_handoff") || !strings.Contains(h, "reason is lifecycle rationale only") {
		t.Fatal("conditional handoff teaching must stay with its available schema field")
	}
}
