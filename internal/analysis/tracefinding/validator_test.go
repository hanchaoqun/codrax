package tracefinding

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestValidateRejectsInventedCandidateAndEvidence(t *testing.T) {
	contract := testContract()
	finding := testFinding()
	finding.PrimaryCause.CandidateID = "invented"
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "not eligible") {
		t.Fatalf("expected candidate rejection, got %v", err)
	}
	finding = testFinding()
	finding.PrimaryCause.EvidenceRefs = []string{"foreign"}
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "outside accepted evidence") {
		t.Fatalf("expected evidence rejection, got %v", err)
	}
}

func TestValidateRejectsCausalCeilingAndRegistryDrift(t *testing.T) {
	contract := testContract()
	finding := testFinding()
	finding.PrimaryCause.Status = types.TraceCausalProven
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "exceeds causal ceiling") {
		t.Fatalf("expected causal ceiling rejection, got %v", err)
	}
	finding = testFinding()
	finding.PrimaryCause.Token.Lane = "cpu_work"
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "registry snapshot") {
		t.Fatalf("expected registry rejection, got %v", err)
	}
}

func testContract() *types.TraceFindingContract {
	return &types.TraceFindingContract{
		Required: true, FindingSchemaVersion: types.TraceFindingSchemaVersion,
		PrimaryCandidateIDs: []string{"candidate-1"}, ContributorCandidateIDs: []string{"candidate-2"},
		AcceptedEvidenceIDs: []string{"evidence-1"}, RegistryHash: "registry-v1", CausalCeiling: "unproven",
	}
}

func testFinding() *types.TraceFindingV1 {
	return &types.TraceFindingV1{
		SchemaVersion: types.TraceFindingSchemaVersion, FindingID: "finding-1", AnalysisKey: "analysis-1",
		PrimaryCause: &types.TraceCauseDecision{
			CandidateID: "candidate-1", Status: types.TraceCausalSupportedCandidate,
			Token:       types.TraceCausalTokenSnapshot{Token: "binder_wait", Lane: "wakeup_chain", Additivity: "wall_clock_per_thread", SubjectKind: "per_thread", FixDirection: "io_dependency", RegistryHash: "registry-v1"},
			SubjectRole: "ui_thread", CausalShape: "upstream_completion_wakes_target", Phase: "pre_wakeup_dependency", EvidenceRefs: []string{"evidence-1"},
		},
		EvidenceRefs: []string{"evidence-1"},
	}
}
