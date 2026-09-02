package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceFindingSchemaIsOptIn(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ordinary := &types.AgentContext{Mutable: &types.MutableState{}}
	assertSchemaField(t, tool.ParametersFor(ordinary), "trace_finding", false)

	ordinary.Mutable.SetTraceFindingContract(&types.TraceFindingContract{
		Required: false, FindingSchemaVersion: types.TraceFindingSchemaVersion,
		CandidateSetID: "deterministic-sidecar-only",
	})
	assertSchemaField(t, tool.ParametersFor(ordinary), "trace_finding", false)
	assertSchemaField(t, (&EmitAnswerDocumentPatch{}).ParametersFor(ordinary), "replace_trace_finding", false)

	ordinary.Mutable.SetTraceFindingContract(&types.TraceFindingContract{
		Required: true, FindingSchemaVersion: types.TraceFindingSchemaVersion,
		PrimaryCandidateIDs: []string{"candidate-1"}, AcceptedEvidenceIDs: []string{"evidence-1"},
	})
	assertSchemaField(t, tool.ParametersFor(ordinary), "trace_finding", true)
	assertSchemaField(t, (&EmitAnswerDocumentPatch{}).ParametersFor(ordinary), "replace_trace_finding", true)
}

func TestTraceRootCauseReportSchemaIsDefaultForTraceContract(t *testing.T) {
	tool := &EmitAnswerDocument{}
	ctx := &types.AgentContext{Mutable: &types.MutableState{}}
	assertSchemaField(t, tool.ParametersFor(ctx), "trace_root_causes", false)

	ctx.Mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	full := tool.ParametersFor(ctx)
	assertSchemaField(t, full, "trace_root_causes", true)
	assertSchemaRequiredField(t, full, "trace_root_causes", false)
	assertDynamicRootCauseArraySchema(t, full)
	assertSchemaField(t, (&EmitAnswerDocumentPatch{}).ParametersFor(ctx), "replace_trace_root_causes", true)
}

func assertDynamicRootCauseArraySchema(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	properties, _ := root["properties"].(map[string]any)
	report, _ := properties["trace_root_causes"].(map[string]any)
	reportProperties, _ := report["properties"].(map[string]any)
	if _, legacy := reportProperties["root_cause_1"]; legacy {
		t.Fatal("legacy root_cause_1 survived in dynamic report schema")
	}
	causes, _ := reportProperties["root_causes"].(map[string]any)
	if causes["type"] != "array" {
		t.Fatalf("root_causes schema is not an array: %#v", causes)
	}
	if _, capped := causes["maxItems"]; capped {
		t.Fatal("root_causes must be evidence-sized, not capped at a fixed N")
	}
	item, _ := causes["items"].(map[string]any)
	itemProperties, _ := item["properties"].(map[string]any)
	candidate, _ := itemProperties["candidate_id"].(map[string]any)
	if candidate["type"] != "string" {
		t.Fatalf("candidate_id is not a typed selector: %#v", candidate)
	}
	required, _ := item["required"].([]any)
	want := map[string]bool{"candidate_id": false}
	for _, field := range required {
		if name, ok := field.(string); ok {
			if _, tracked := want[name]; tracked {
				want[name] = true
			}
		}
	}
	for field, present := range want {
		if !present {
			t.Fatalf("root_causes item required field %q is missing: %#v", field, required)
		}
	}
}

func testSelectableTraceRootCauseContract() *types.TraceFindingContract {
	return &types.TraceFindingContract{
		RootCauseReportEnabled: true,
		CandidateSetID:         "trace-root-cause-report",
		Candidates: []types.TraceFindingCandidateV1{{
			PrimaryEligible: true,
			Decision: types.TraceCauseDecision{
				CandidateID: "candidate-sched", SubjectName: "RenderThread",
				Token:           types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
				Magnitude:       &types.TypedMagnitude{Value: 12.4, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution"},
				CausalQualifier: types.TraceCausalQualifierProven,
				EvidenceRefs:    []string{"E-sched"},
			},
		}},
	}
}

func assertSchemaField(t *testing.T, raw json.RawMessage, field string, want bool) {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	properties, _ := root["properties"].(map[string]any)
	_, got := properties[field]
	if got != want {
		t.Fatalf("schema field %q present=%t, want %t", field, got, want)
	}
}

func assertSchemaRequiredField(t *testing.T, raw json.RawMessage, field string, want bool) {
	t.Helper()
	var root struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	got := false
	for _, candidate := range root.Required {
		if candidate == field {
			got = true
			break
		}
	}
	if got != want {
		t.Fatalf("schema required field %q present=%t, want %t", field, got, want)
	}
}
