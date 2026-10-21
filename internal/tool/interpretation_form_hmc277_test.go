package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC277InterpretationFormsPublicCompletion(t *testing.T) {
	unit := []types.InvestigationInterpretationUnit{{Text: "Independent business finding", Boundaries: []string{"No causal witness"}, SupportRefs: []string{"proposed:1"}}}
	for _, tc := range []struct {
		name   string
		fields map[string]any
		valid  bool
		field  string
	}{
		{"units_only", map[string]any{"interpretation_units": unit}, true, ""},
		{"units_and_legacy", map[string]any{"interpretation_units": unit, "interpretation": "legacy audit"}, true, ""},
		{"units_and_empty_legacy", map[string]any{"interpretation_units": unit, "interpretation": ""}, true, ""},
		{"legacy_only", map[string]any{"interpretation": "independent legacy"}, true, ""},
		{"legacy_empty", map[string]any{"interpretation": ""}, true, ""},
		{"legacy_empty_units", map[string]any{"interpretation": "", "interpretation_units": []any{}}, true, ""},
		{"neither", map[string]any{}, false, "native_fact_handoff.interpretation"},
		{"empty_units_only", map[string]any{"interpretation_units": []any{}}, false, "native_fact_handoff.interpretation"},
		{"null_units_only", map[string]any{"interpretation_units": nil}, false, "native_fact_handoff.interpretation"},
		{"null_legacy_with_units", map[string]any{"interpretation": nil, "interpretation_units": unit}, false, "native_fact_handoff.interpretation"},
		{"wrong_legacy_with_units", map[string]any{"interpretation": map[string]any{"text": "not a string"}, "interpretation_units": unit}, false, "native_fact_handoff.interpretation"},
		{"wrong_units_with_legacy", map[string]any{"interpretation": "legacy", "interpretation_units": true}, false, "native_fact_handoff.interpretation_units"},
		{"blank_unit_with_legacy", map[string]any{"interpretation": "legacy", "interpretation_units": []any{map[string]any{"text": " "}}}, false, "native_fact_handoff.interpretation_units[0]"},
		{"null_unit_with_legacy", map[string]any{"interpretation": "legacy", "interpretation_units": []any{nil}}, false, "native_fact_handoff.interpretation_units[0]"},
		{"extra_unit_key", map[string]any{"interpretation_units": []any{map[string]any{"text": "candidate", "invented": true}}}, false, "invented"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			choice := NativeFactHandoffChoices(ctx)[0]
			call := func(reason string, fields map[string]any) (types.ToolResult, error) {
				handoff := map[string]any{"runtime_measurements": []types.AnswerRuntimeMeasurementReceipt{choice}}
				for k, v := range fields {
					handoff[k] = v
				}
				raw, err := json.Marshal(map[string]any{"reason": reason, "confidence": "high", "result_kind": "resolved", "native_fact_handoff": handoff})
				if err != nil {
					t.Fatal(err)
				}
				return (&EmitInvestigationComplete{}).Execute(ctx, raw)
			}
			prior, err := call("prior accepted lifecycle", map[string]any{"interpretation": "prior advisory"})
			if err != nil || !prior.Success {
				t.Fatalf("setup: %v %s", err, prior.Summary)
			}
			before, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
			result, err := call("current lifecycle", tc.fields)
			if tc.valid {
				if err != nil || !result.Success {
					t.Fatalf("valid form rejected: %v %s", err, result.Summary)
				}
				got := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
				if got == nil || ctx.Mutable.StableInvestigationCompleteReason() != "current lifecycle" {
					t.Fatal("accepted form did not reach current completion")
				}
				if want, ok := tc.fields["interpretation_units"].([]types.InvestigationInterpretationUnit); ok && !reflect.DeepEqual(got.InterpretationUnits, want) {
					t.Fatal("units changed in accepted state")
				}
				wantLegacy, _ := tc.fields["interpretation"].(string)
				if got.Interpretation != wantLegacy {
					t.Fatal("legacy text invented or changed")
				}
				return
			}
			if result.Success {
				t.Fatalf("invalid form accepted: %s", result.Summary)
			}
			if !strings.Contains(result.Summary, tc.field) {
				t.Errorf("missing precise field %q: err=%v summary=%s", tc.field, err, result.Summary)
			}
			after, _ := json.Marshal(ctx.Mutable.AcceptedInvestigationNativeFactHandoff())
			if string(after) != string(before) || ctx.Mutable.StableInvestigationCompleteReason() != "prior accepted lifecycle" {
				t.Fatal("rejected form changed accepted state")
			}
		})
	}
}

func TestHMC277UnitsOnlyAuditRecoveryKeepsAuthorityBoundary(t *testing.T) {
	ctx := nativeMeasurementHandoffContext(t)
	selection := NativeFactHandoffChoices(ctx)[:1]
	units := []types.InvestigationInterpretationUnit{{Text: "candidate", Boundaries: []string{"not proven"}, SupportRefs: []string{"proposed:1"}}}
	raw, _ := json.Marshal(map[string]any{"reason": "lifecycle", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{"runtime_measurements": selection, "interpretation_units": units}})
	r, err := (&EmitInvestigationComplete{}).Execute(ctx, raw)
	if err != nil || !r.Success {
		t.Fatalf("units-only completion: %v %s", err, r.Summary)
	}
	accepted := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	accepted.InterpretationUnits[0].Boundaries[0] = "mutated copy"
	if ctx.Mutable.AcceptedInvestigationNativeFactHandoff().InterpretationUnits[0].Boundaries[0] != "not proven" {
		t.Fatal("accepted getter aliased units")
	}
	fork := ctx.Mutable.ForkForExploreDispatch()
	if !reflect.DeepEqual(fork.AcceptedInvestigationNativeFactHandoff(), ctx.Mutable.AcceptedInvestigationNativeFactHandoff()) {
		t.Fatal("same-run fork lost accepted intent")
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{AcceptedClosureReason: "lifecycle", ToolResults: ctx.Mutable.DispatchToolResults()})
	wire, _ := json.Marshal(ctx.Mutable.TurnAArtifacts())
	var restored types.TurnAArtifacts
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	fresh := types.NewMutableState("restored")
	fresh.SetTurnAArtifacts(restored)
	fresh.SetInvestigationComplete("lifecycle")
	if fresh.AcceptedInvestigationNativeFactHandoff() != nil {
		t.Fatal("JSON restored private acceptance")
	}
	audit := fresh.TurnAArtifacts().AcceptedNativeFactHandoff
	if audit == nil || audit.Interpretation != "" || !reflect.DeepEqual(audit.InterpretationUnits, units) {
		t.Fatal("JSON lost independent units or fabricated legacy text")
	}
}

func TestHMC277InterpretationFormsSchemaAndRepair(t *testing.T) {
	var root struct {
		Properties map[string]struct {
			Required []string
			AnyOf    []struct {
				Required   []string
				Properties map[string]struct{ MinItems int }
			}
			Properties map[string]json.RawMessage
		}
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &root); err != nil {
		t.Fatal(err)
	}
	h := root.Properties["native_fact_handoff"]
	if !reflect.DeepEqual(h.Required, []string{"runtime_measurements"}) {
		t.Errorf("legacy still unconditionally required: %v", h.Required)
	}
	if len(h.AnyOf) != 2 || !reflect.DeepEqual(h.AnyOf[0].Required, []string{"interpretation"}) || !reflect.DeepEqual(h.AnyOf[1].Required, []string{"interpretation_units"}) || h.AnyOf[1].Properties["interpretation_units"].MinItems != 1 {
		t.Errorf("schema missing alternative nonempty representation: %+v", h.AnyOf)
	}
	for _, text := range []string{types.InvestigationInterpretationUnitsTeaching, ManagedDeliveryNativeFactHandoffTeaching, optionalNativeFactHandoffTeaching} {
		if !strings.Contains(text, "omit interpretation") {
			t.Errorf("teaching still needs filler legacy: %s", text)
		}
	}
	err := managedDeliveryNativeHandoffError{violations: []nativeFactHandoffViolation{{"native_fact_handoff.interpretation", "missing_interpretation"}}}
	if !strings.Contains(err.hint(), "non-empty interpretation_units") || !strings.Contains(err.hint(), "empty string") {
		t.Fatal("repair does not explain alternative forms and explicit no-analysis form")
	}
}
