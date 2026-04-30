package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestChangePlan_WriteAnalysisIR_RoundTrip pins the persistence
// shape: a plan with WriteAnalysisIR survives JSON marshal/unmarshal
// without field drift.
func TestChangePlan_WriteAnalysisIR_RoundTrip(t *testing.T) {
	plan := &ChangePlan{
		ID:      "plan-x",
		Summary: "wire a feature",
		WriteAnalysisIR: &WriteAnalysisIR{
			Request: WriteRequestModel{
				RawRequest: "user request",
				Task: WriteTask{
					Kind:    WriteTaskFeature,
					Scope:   ScopeMicro,
					Summary: "add --quiet flag",
				},
				Risk: WriteRiskProfile{
					AffectsPublicAPI: true,
					Overall:          RiskBandLow,
				},
				ExpectedOutcomes: []string{"flag wired"},
			},
		},
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ChangePlan
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WriteAnalysisIR == nil {
		t.Fatal("WriteAnalysisIR lost in round-trip")
	}
	if got.WriteAnalysisIR.Request.Task.Kind != WriteTaskFeature {
		t.Errorf("kind drift: got %q", got.WriteAnalysisIR.Request.Task.Kind)
	}
	if got.WriteAnalysisIR.Request.Task.Summary != "add --quiet flag" {
		t.Errorf("summary drift: got %q", got.WriteAnalysisIR.Request.Task.Summary)
	}
	if !got.WriteAnalysisIR.Request.Risk.AffectsPublicAPI {
		t.Error("risk.AffectsPublicAPI drift")
	}
	if !strings.Contains(string(raw), `"write_analysis_ir"`) {
		t.Errorf("expected write_analysis_ir field in JSON; got %s", string(raw))
	}
}

// TestChangePlan_WriteAnalysisIR_Omitempty pins that an absent IR
// stays absent in JSON (for back-compat with pre-commit-9 plans).
func TestChangePlan_WriteAnalysisIR_Omitempty(t *testing.T) {
	plan := &ChangePlan{ID: "plan-x"}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "write_analysis_ir") {
		t.Errorf("nil IR should be omitted from JSON; got %s", string(raw))
	}
}
