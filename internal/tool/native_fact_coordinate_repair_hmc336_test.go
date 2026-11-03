package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public query and completion tools: a diagnostic suggestion is
// not an alias accepted by the source binder, nor a mutation of prior results.
func TestHMC336NativeCoordinateRepairPublicRetry(t *testing.T) {
	ctx := nativeMeasurementHandoffContext(t)
	choices := NativeFactHandoffChoices(ctx)
	if len(choices) < 2 {
		t.Fatal("public query did not expose independent native views")
	}
	current := choices[0]
	colon := strings.IndexByte(current.ObservationID, ':')
	if colon < 0 {
		t.Fatalf("real observation lacks its source namespace: %q", current.ObservationID)
	}
	submit := func(r types.AnswerRuntimeMeasurementReceipt) types.ToolResult {
		t.Helper()
		raw, err := json.Marshal(map[string]any{
			"reason":     "Current facts are retained separately from independent interpretation.",
			"confidence": "high", "result_kind": "resolved",
			"native_fact_handoff": map[string]any{
				"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{r},
				"interpretation":       "This measurement alone does not establish a response cause.",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := (&EmitInvestigationComplete{}).Execute(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := submit(current); !result.Success {
		t.Fatal(result.Summary)
	}
	before, _ := json.Marshal([]any{ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), ctx.Mutable.DispatchToolResults(), ctx.Mutable.TurnAArtifacts(), ctx.Mutable.StableInvestigationCompleteReason()})
	for _, mode := range []string{"missing_namespace", "wrong_view"} {
		t.Run(mode, func(t *testing.T) {
			invalid := current
			code := "unknown_id"
			if mode == "missing_namespace" {
				invalid.ObservationID = current.ObservationID[colon+1:]
			} else {
				invalid.View = "not_a_current_view"
				code = "wrong_view"
			}
			result := submit(invalid)
			if result.Success || result.Repair == nil {
				t.Fatalf("invalid coordinates accepted or not repairable: %+v", result)
			}
			var repair struct {
				Entries []struct {
					Field      string                                  `json:"field"`
					Code       string                                  `json:"code"`
					Submitted  types.AnswerRuntimeMeasurementReceipt   `json:"submitted"`
					Candidates []types.AnswerRuntimeMeasurementReceipt `json:"current_candidates"`
				} `json:"entries"`
				Omitted    int `json:"omitted"`
				Unresolved int `json:"unresolved"`
			}
			if err := json.Unmarshal([]byte(result.Repair.Metadata["native_fact_coordinate_repairs"]), &repair); err != nil {
				t.Fatalf("rejection lost its field-local current coordinate repair: %v; %s", err, result.Summary)
			}
			if len(repair.Entries) != 1 || repair.Omitted != 0 || repair.Unresolved != 0 {
				t.Fatalf("unexpected coordinate repair coverage: %+v", repair)
			}
			entry := repair.Entries[0]
			if entry.Field != "native_fact_handoff.runtime_measurements[0]" || entry.Code != code || !reflect.DeepEqual(entry.Submitted, invalid) {
				t.Fatalf("repair mixed rejected selectors: %+v", entry)
			}
			found := false
			for _, candidate := range entry.Candidates {
				if candidate.ObservationID != current.ObservationID {
					t.Fatalf("repair mixed another measurement: %+v", candidate)
				}
				found = found || reflect.DeepEqual(candidate, current)
			}
			if !found || !strings.Contains(result.Repair.Hint, "advice only") || strings.Contains(result.Repair.Hint, "Copy selected whole objects from these current pairs") {
				t.Fatal("focused repair omitted the exact current pair or repeated the unscoped roster")
			}
			after, _ := json.Marshal([]any{ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), ctx.Mutable.DispatchToolResults(), ctx.Mutable.TurnAArtifacts(), ctx.Mutable.StableInvestigationCompleteReason()})
			if string(after) != string(before) {
				t.Fatal("diagnostic repair changed previously accepted facts or completion")
			}
			// Repeating the invalid pair must remain a rejection; only a fresh
			// caller selecting the complete current coordinates may succeed.
			if submit(invalid).Success {
				t.Fatal("suggestion silently authorized the rejected coordinates")
			}
			if result := submit(current); !result.Success {
				t.Fatalf("valid current retry was blocked: %s", result.Summary)
			}
		})
	}
}
