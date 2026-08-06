package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildAnswerDocumentParametersForContract_ProjectsTraceFindingOnlyWhenActive(t *testing.T) {
	inactive := BuildAnswerDocumentParametersForContract(nil, nil)
	var root map[string]any
	if err := json.Unmarshal(inactive, &root); err != nil {
		t.Fatal(err)
	}
	props, _ := root["properties"].(map[string]any)
	if _, ok := props["trace_finding"]; ok {
		t.Fatal("inactive contract must not project trace_finding")
	}

	active := BuildAnswerDocumentParametersForContract(nil, &types.TraceFindingContract{ShadowOptional: true})
	if err := json.Unmarshal(active, &root); err != nil {
		t.Fatal(err)
	}
	props, _ = root["properties"].(map[string]any)
	if _, ok := props["trace_finding"]; !ok {
		t.Fatal("shadow contract must project trace_finding")
	}
}
