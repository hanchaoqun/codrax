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

func TestPrepareTraceFindingContractEnablesSeparateJSONWithoutOldFindingSchema(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || contract.Required || !contract.RootCauseReportRequired {
		t.Fatalf("expected JSON report contract without legacy model finding: %+v", contract)
	}
	if got := renderAnswerDocTraceDecisionHandoff(ctx); !strings.Contains(got, "Trace Root Cause JSON") || strings.Contains(got, "Required Typed Sidecar") {
		t.Fatalf("finalizer must receive only the new JSON report contract:\n%s", got)
	}
	finalizeDeterministicTraceFinding(ctx)
	if finding := ctx.Mutable.TraceFinding(); finding == nil || finding.Unresolved == nil {
		t.Fatalf("legacy batch/clustering artifact must remain independently available: %+v", finding)
	}
}

func TestPrepareTraceFindingContractOrdinaryTraceRequestEnablesJSONByDefault(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract == nil || !contract.RootCauseReportRequired {
		t.Fatalf("ordinary root-cause trace answer must enable JSON by default: %+v", contract)
	}
}

func TestPrepareTraceFindingContractSidecarFlagStillEnablesJSON(t *testing.T) {
	ctx := traceRootCauseTestContext("analyze this trace root cause")
	ctx.TraceFindingRequired = true
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract == nil || contract.Required || !contract.RootCauseReportRequired {
		t.Fatalf("batch sidecar request must coexist with the new JSON report: %+v", contract)
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
