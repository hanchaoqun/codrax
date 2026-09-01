package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func traceRootCauseTestContext(objective string) *types.AgentContext {
	return &types.AgentContext{
		Objective: objective,
		Mutable:   types.NewMutableState(objective),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace", Carrier: "request_path"}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
}

func TestPrepareTraceFindingContractDoesNotEnableReportWithoutTypedCandidates(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || contract.Required || contract.RootCauseReportEnabled {
		t.Fatalf("empty typed roster must not expose root-cause selection: %+v", contract)
	}
	if got := renderAnswerDocTraceDecisionHandoff(ctx); strings.Contains(got, "Trace Root Cause JSON") {
		t.Fatalf("empty roster leaked a root-cause report contract:\n%s", got)
	}
	if finding := ctx.Mutable.TraceFinding(); finding != nil {
		t.Fatalf("runtime must not mint a model conclusion: %+v", finding)
	}
}

func TestPrepareTraceFindingContractSidecarFlagCannotBypassTypedRoster(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract == nil || contract.Required || contract.RootCauseReportEnabled {
		t.Fatalf("caller flag must not manufacture selectable root causes: %+v", contract)
	}
}

func TestRenderTraceRootCauseContractOffersOnlyTypedCandidateIDs(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.Mutable.SetTraceFindingContract(&types.TraceFindingContract{
		RootCauseReportEnabled: true,
		CandidateSetID:         "set-1",
		Candidates: []types.TraceFindingCandidateV1{{
			PrimaryEligible: true,
			Decision: types.TraceCauseDecision{
				CandidateID: "candidate-1", SubjectName: "RenderThread",
				Token:        types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
				Magnitude:    &types.TypedMagnitude{Value: 4, Unit: "ms", Additivity: "wall_clock_per_thread"},
				EvidenceRefs: []string{"E1"},
			},
		}},
	})
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if !strings.Contains(got, "Optional Trace Root Cause JSON") || !strings.Contains(got, `"candidate_id": "candidate-1"`) || strings.Contains(got, "Required") {
		t.Fatalf("typed optional selector prompt drifted:\n%s", got)
	}
}

func TestPrepareTraceFindingContractNarrowTraceFactStaysInactive(t *testing.T) {
	ctx := traceRootCauseTestContext("list the target thread state")
	ctx.AnalysisIR.RequestModel = types.RequestModel{
		Intent: types.IntentTrace,
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope:        types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
		},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract != nil {
		t.Fatalf("narrow fact request must not be widened: %+v", contract)
	}
}
