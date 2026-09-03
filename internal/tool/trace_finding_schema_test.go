package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EVOLUTION RECORD (V1-5, colleague_merge_audit §40.16): re-pinned from
// TestTraceFindingSchemaIsOptIn, which asserted the legacy Required
// `trace_finding` arm could be switched ON. That lane was born dead (its only
// producer forced Required=false) and is retired on all three faces; the pin
// now holds the schema face closed for every contract shape. The red→green
// witness for the retirement is the input-face census
// (answer_document_input_face_census_test.go) plus the stray-key behavior
// pins (trace_finding_retirement_test.go).
func TestTraceFindingFieldIsRetiredFromEverySchema(t *testing.T) {
	tool := &EmitAnswerDocument{}
	patch := &EmitAnswerDocumentPatch{}
	for _, tc := range []struct {
		name     string
		contract *types.TraceFindingContract
	}{
		{name: "no contract", contract: nil},
		{name: "contract without selectable roster", contract: &types.TraceFindingContract{
			FindingSchemaVersion: types.TraceFindingSchemaVersion, CandidateSetID: "deterministic-sidecar-only",
			PrimaryCandidateIDs: []string{"candidate-1"}, AcceptedEvidenceIDs: []string{"evidence-1"},
		}},
		{name: "sidecar-enabled contract", contract: testSelectableTraceRootCauseContract()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.AgentContext{Mutable: &types.MutableState{}}
			if tc.contract != nil {
				ctx.Mutable.SetTraceFindingContract(tc.contract)
			}
			assertSchemaField(t, tool.ParametersFor(ctx), "trace_finding", false)
			assertSchemaField(t, patch.ParametersFor(ctx), "replace_trace_finding", false)
		})
	}
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
	// V1-4 (§40.26 ②, R2′): the selector item teaches the roster's
	// artifact_label partition key without internal names; the model still
	// submits candidate_id only (no artifact property to author).
	if text, _ := item["description"].(string); !strings.Contains(text, "artifact_label") || !strings.Contains(text, "select by candidate_id") ||
		strings.Contains(text, "TraceCausalProjection") || strings.Contains(text, "partition") {
		t.Fatalf("selector item must teach artifact_label in customer words: %q", text)
	}
	if _, authored := itemProperties["artifact_label"]; authored {
		t.Fatal("artifact_label is system-owned; the model must not author it")
	}
	// SIDECAR-NARR-1: the model may attach a plain-language description; it is
	// optional and taught without internal names.
	description, _ := itemProperties["description"].(map[string]any)
	if description["type"] != "string" {
		t.Fatalf("description is not offered on the selector item: %#v", itemProperties)
	}
	if text, _ := description["description"].(string); text != types.TraceRootCauseDescriptionTeaching() || strings.Contains(text, "candidate_compiler") || strings.Contains(text, "EvidenceFacts") {
		t.Fatalf("description teaching must be the single types-level sentence: %q", text)
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
