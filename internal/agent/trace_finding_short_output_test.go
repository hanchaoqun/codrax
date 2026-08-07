package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderTraceFindingShortRootCauseChinese(t *testing.T) {
	finding := &types.TraceFindingV1{PrimaryCause: &types.TraceCauseDecision{
		Status:      types.TraceCausalSupportedCandidate,
		Token:       types.TraceCausalTokenSnapshot{Token: "scheduler_latency"},
		SubjectRole: "target_thread",
		Magnitude:   &types.TypedMagnitude{Value: 12.5, Unit: "ms"},
	}}
	got := renderTraceFindingShortRootCauseValue(finding, "zh-CN")
	for _, want := range []string{"## 简短根因", "最强根因候选", "调度延迟", "目标线程", "12.500 ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("short root cause missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTraceFindingShortRootCauseUnresolvedIsBounded(t *testing.T) {
	finding := &types.TraceFindingV1{Unresolved: &types.TraceUnresolvedDecision{Reason: strings.Repeat("证据不足 ", 100)}}
	got := renderTraceFindingShortRootCauseValue(finding, "zh")
	if !strings.Contains(got, "未确定") || len([]rune(got)) > 190 {
		t.Fatalf("unexpected unresolved short output: %s", got)
	}
}

func TestPrepareTraceFindingContract_AttachedTraceWithoutRowsRequiresUnresolvedFinding(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState("analyze the trace root cause"),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "capture.systrace", Carrier: "request_path",
			}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.Required {
		t.Fatal("causal analysis of an attached trace must require a finding even when no candidate row was compiled")
	}
	if len(contract.Candidates) != 0 {
		t.Fatalf("empty evidence ledger must fail closed to unresolved, got %d candidates", len(contract.Candidates))
	}
	if got := renderAnswerDocTraceDecisionHandoff(ctx); !strings.Contains(got, "Trace Finding Contract") ||
		!strings.Contains(got, "If no candidate is sufficiently supported") {
		t.Fatalf("finalizer must receive the unresolved finding contract even without trace guidance rows:\n%s", got)
	}
}

func TestPrepareTraceFindingContract_NarrowTraceFactStaysInactive(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState("list the target scheduler state"),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace"}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope:        types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState},
			},
		}},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract != nil {
		t.Fatalf("narrow runtime fact request must not be widened into a root-cause finding: %+v", contract)
	}
}

func TestPrepareTraceFindingContract_NonTraceRootCauseStaysInactive(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable:    types.NewMutableState("find the source-code root cause"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract != nil {
		t.Fatalf("ordinary non-trace diagnosis must not activate trace finding: %+v", contract)
	}
}
