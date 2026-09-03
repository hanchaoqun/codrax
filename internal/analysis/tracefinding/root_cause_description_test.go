package tracefinding

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// root_cause_description_test.go — SIDECAR-NARR-1: the binder carries the
// model's description onto the bound item and refuses internal references.
func TestBindRootCauseReportSelectionCarriesDescription(t *testing.T) {
	contract := qualifierContract(types.TraceCausalQualifierProven)
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
	_, err = BindRootCauseReportSelection(&types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses:    []*types.TraceRootCauseItemV2{{CandidateID: id, Description: "见 .codrax/blob/x/attached_trace.txt:2892"}},
	}, contract)
	if err == nil || !strings.Contains(err.Error(), "internal references") {
		t.Fatalf("an internal reference in the description must be refused: %v", err)
	}
}
