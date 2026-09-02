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

func TestRootCauseSelectionIdentityIsNotItsDisplaySummary(t *testing.T) {
	for _, token := range []types.TraceCausalTokenSnapshot{
		{Token: "running", Lane: "compute_delivery"},
		{Token: "gc_pause"},
		{Token: "scheduler_latency", Lane: "scheduling_demand"},
		{Token: "wait", Lane: "lock_contention"},
		{Token: "work", Lane: "cpu_work"},
	} {
		for _, sameSubject := range []bool{false, true} {
			name := token.Token + "/different-subject"
			if sameSubject {
				name = token.Token + "/same-subject-independent-receipt"
			}
			t.Run(name, func(t *testing.T) {
				contract := &types.TraceFindingContract{RootCauseReportEnabled: true}
				for i, subject := range []string{"ui-101", "worker-202"} {
					if sameSubject {
						subject = "ui-101"
					}
					id := []string{"first", "second"}[i]
					contract.Candidates = append(contract.Candidates, types.TraceFindingCandidateV1{
						PrimaryEligible: true,
						Decision: types.TraceCauseDecision{CandidateID: id, SubjectName: subject, Token: token,
							ResourceName: "shared-resource", PhaseName: "shared-phase",
							Magnitude:    &types.TypedMagnitude{Value: float64(i + 1), Unit: "ms", Additivity: "wall_clock_per_thread"},
							EvidenceRefs: []string{"E-" + id}},
					})
				}
				selection := &types.TraceRootCauseReportV2{SchemaVersion: 2,
					RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: "second"}, {CandidateID: "first"}}}
				report, err := BindRootCauseReportSelection(selection, contract)
				if err != nil {
					t.Fatalf("independent typed selections collided on display text: %v", err)
				}
				if len(report.RootCauses) != 2 {
					t.Fatalf("selection lost: %+v", report)
				}
				for i, cause := range report.RootCauses {
					if cause.Rank != i+1 || cause.CandidateID != "" || *cause.ImpactSeconds != float64(2-i)/1000 ||
						!strings.Contains(cause.Evidence[0], []string{"E-second", "E-first"}[i]) {
						t.Fatalf("model order, bound value, provenance or private-id boundary changed: %+v", cause)
					}
					if (token.Token == "running" || token.Token == "gc_pause") && cause.ThreadName == "" {
						t.Fatalf("known subject was dropped from a short-summary category: %+v", cause)
					}
				}
				selection.RootCauses[1].CandidateID = "second"
				if _, err := BindRootCauseReportSelection(selection, contract); err == nil || !strings.Contains(err.Error(), "duplicates") {
					t.Fatalf("a genuinely repeated receipt must still fail: %v", err)
				}
			})
		}
	}
}
