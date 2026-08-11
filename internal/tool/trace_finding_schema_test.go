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

	ctx.Mutable.SetTraceFindingContract(&types.TraceFindingContract{
		RootCauseReportRequired: true,
		CandidateSetID:          "trace-root-cause-report",
	})
	full := tool.ParametersFor(ctx)
	assertSchemaField(t, full, "trace_root_causes", true)
	assertSchemaRequiredField(t, full, "trace_root_causes", true)
	assertSchemaField(t, (&EmitAnswerDocumentPatch{}).ParametersFor(ctx), "replace_trace_root_causes", true)
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
