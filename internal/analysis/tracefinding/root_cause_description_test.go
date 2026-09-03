package tracefinding

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// descriptionTestContract is one selectable-shaped candidate carrying the
// given seat qualifier. (EVOLUTION RECORD, V1-5 §40.16: formerly borrowed
// from validator_qualifier_test.go, which pinned the retired Required
// trace_finding validator and went with it.)
func descriptionTestContract(qualifier string) *types.TraceFindingContract {
	return &types.TraceFindingContract{
		FindingSchemaVersion: types.TraceFindingSchemaVersion,
		PrimaryCandidateIDs:  []string{"candidate-1"}, ContributorCandidateIDs: []string{"candidate-2"},
		AcceptedEvidenceIDs: []string{"evidence-1"}, RegistryHash: "registry-v1", CausalCeiling: qualifier,
		Candidates: []types.TraceFindingCandidateV1{{Decision: types.TraceCauseDecision{
			CandidateID: "candidate-1", Status: types.TraceCausalSupportedCandidate,
			Token:       types.TraceCausalTokenSnapshot{Token: "binder_wait", Lane: "wakeup_chain", Additivity: "wall_clock_per_thread", SubjectKind: "per_thread", FixDirection: "io_dependency", RegistryHash: "registry-v1"},
			SubjectRole: "ui_thread", CausalShape: "upstream_completion_wakes_target", Phase: "pre_wakeup_dependency", EvidenceRefs: []string{"evidence-1"},
			CausalQualifier: qualifier,
		}}},
	}
}

// root_cause_description_test.go — SIDECAR-NARR-1: the binder carries the
// model's description onto the bound item and refuses internal references.
func TestBindRootCauseReportSelectionCarriesDescription(t *testing.T) {
	contract := descriptionTestContract(types.TraceCausalQualifierProven)
	contract.RootCauseReportEnabled = true
	contract.Candidates[0].PrimaryEligible = true
	contract.Candidates[0].Decision.Magnitude = &types.TypedMagnitude{Value: 12.4, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: types.TraceImpactCaliberEffectiveAttribution}
	contract.Candidates[0].Decision.SubjectName = "worker-9"
	contract.Candidates[0].Decision.Token.Token = "sched_delay"
	selectable := SelectableRootCauseCandidates(contract)
	if len(selectable) != 1 {
		t.Fatalf("fixture must be selectable: %+v", contract.Candidates[0])
	}
	id := selectable[0].Decision.CandidateID
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses:    []*types.TraceRootCauseItemV2{{CandidateID: id, Description: "worker-9 在目标窗口内等待 CPU 约 12 ms，主线程随之被推迟。"}},
	}, contract)
	if err != nil || len(report.RootCauses) != 1 || !strings.Contains(report.RootCauses[0].Description, "worker-9") {
		t.Fatalf("description must be bound beside the typed facts: %+v %v", report, err)
	}
	if len(report.RootCauses[0].Evidence) == 0 {
		t.Fatal("typed evidence must remain")
	}
	// 复核: an internal reference drops only the description; the typed
	// selection stands and the reason is returned for disclosure.
	report, advisories, err := BindRootCauseReportSelectionWithAdvisories(&types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses:    []*types.TraceRootCauseItemV2{{CandidateID: id, Description: "见 .codrax/blob/x/attached_trace.txt:2892"}},
	}, contract)
	if err != nil || report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].Description != "" {
		t.Fatalf("the selection must survive a leaking description: %+v %v", report, err)
	}
	if len(advisories) != 1 || !strings.Contains(advisories[0], "internal references") {
		t.Fatalf("the drop must be disclosed: %v", advisories)
	}
}
