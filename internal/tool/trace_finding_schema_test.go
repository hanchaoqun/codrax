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
		Required: true, FindingSchemaVersion: types.TraceFindingSchemaVersion,
		PrimaryCandidateIDs: []string{"candidate-1"}, AcceptedEvidenceIDs: []string{"evidence-1"},
	})
	assertSchemaField(t, tool.ParametersFor(ordinary), "trace_finding", true)
	assertSchemaField(t, (&EmitAnswerDocumentPatch{}).ParametersFor(ordinary), "replace_trace_finding", true)
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
