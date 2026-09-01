package tracefinding

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBindRootCauseReportSelectionUsesOnlyFrozenCandidateFacts(t *testing.T) {
	contract := &types.TraceFindingContract{
		RootCauseReportEnabled: true,
		Candidates: []types.TraceFindingCandidateV1{{
			PrimaryEligible: true,
			Decision: types.TraceCauseDecision{
				CandidateID: "candidate-1", SubjectName: "RenderThread",
				Token:        types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
				Magnitude:    &types.TypedMagnitude{Value: 12.4, Unit: "ms", Additivity: "wall_clock_per_thread"},
				EvidenceRefs: []string{"E1"},
			},
		}},
	}
	fakeImpact := 99.0
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses: []*types.TraceRootCauseItemV2{{
			CandidateID: "candidate-1", Category: types.TraceRootCauseGCLongPause,
			ThreadName: "invented", ImpactSeconds: &fakeImpact, Evidence: []string{"invented"},
		}},
	}, contract)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RootCauses) != 1 {
		t.Fatalf("bound report=%+v", report)
	}
	cause := report.RootCauses[0]
	if cause.CandidateID != "" || cause.Category != types.TraceRootCauseCPUSchedulingDelay ||
		cause.ThreadName != "RenderThread" || cause.ImpactSeconds == nil || *cause.ImpactSeconds != 0.0124 ||
		len(cause.Evidence) != 1 || !strings.Contains(cause.Evidence[0], "E1") {
		t.Fatalf("model-authored semantic fields escaped the typed binder: %+v", cause)
	}
}

func TestSelectableRootCauseCandidatesRequireOnChainWallClock(t *testing.T) {
	base := types.TraceCauseDecision{
		CandidateID: "candidate-1", SubjectName: "worker",
		Token:        types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
		Magnitude:    &types.TypedMagnitude{Value: 3, Unit: "ms", Additivity: "wall_clock_per_thread"},
		EvidenceRefs: []string{"E1"},
	}
	contract := &types.TraceFindingContract{Candidates: []types.TraceFindingCandidateV1{
		{PrimaryEligible: false, Decision: base},
		{PrimaryEligible: true, Decision: func() types.TraceCauseDecision {
			copy := base
			copy.CandidateID = "candidate-cross-thread"
			copy.Magnitude = &types.TypedMagnitude{Value: 3, Unit: "ms", Additivity: "cross_thread_cpu_ms"}
			return copy
		}()},
	}}
	if got := SelectableRootCauseCandidates(contract); len(got) != 0 {
		t.Fatalf("off-chain/non-wall-clock candidates became selectable: %+v", got)
	}
}
