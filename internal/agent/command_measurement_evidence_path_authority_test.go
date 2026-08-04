package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func commandMeasurementEvidencePathTestContext(withProfile bool) *types.AgentContext {
	mut := types.NewMutableState("typed command measurement")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "exec_command",
		Success:  true,
		CommandMeasurement: &types.ToolCommandMeasurement{
			Kind:        "integer_count",
			Value:       253,
			Origin:      types.AnswerEvidenceOriginCommandMeasurement,
			ProofSource: "stdout_integer",
			Command:     "find internal/tool -name '*.go' -type f | wc -l",
		},
	}}})
	rm := types.RequestModel{}
	if withProfile {
		rm.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"结合当前源码解释统计路径"},
			Confidence:                          0.9,
		}
	}
	return &types.AgentContext{
		Stage:   types.StageExplore,
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: rm,
		},
	}
}

func TestCommandMeasurementEvidencePathAuthorityTypedCarrierAndProfileOnly(t *testing.T) {
	ctx := commandMeasurementEvidencePathTestContext(true)
	prompt := renderAnswerDocCommandMeasurementEvidencePathAuthority(ctx)
	for _, want := range []string{
		"independent evidence carriers",
		"does not prove a call edge",
		"source actually read and cited from the current repository",
		"outside the customer-repository evidence boundary",
		"does not choose the model's mechanism conclusion",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("typed current-source measurement guidance missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"internal/tool/builtin.go",
		"execCommandMeasurement",
		"ToolResult.CommandMeasurement",
		"ObservationLedgerInputFromAgentContext",
		"CompileObservationLedger",
		"compileToolResultObservations",
		"observationRecordForCommandMeasurement",
		"reconcileCompletionAggregateFactsWithDeterministicCount",
		"emit_investigation_complete.go",
		"EmitAnalysis",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("generic customer-repository prompt leaked Codrax internal %q:\n%s", forbidden, prompt)
		}
	}

	withoutProfile := commandMeasurementEvidencePathTestContext(false)
	if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(withoutProfile); got != "" {
		t.Fatalf("carrier alone must not inject mechanism guidance into an unrelated answer:\n%s", got)
	}

	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
	if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(ctx); got != "" {
		t.Fatalf("profile alone without typed carrier must not activate authority:\n%s", got)
	}
}

func TestCommandMeasurementEvidencePathAuthorityRouteBackedProfileOmission(t *testing.T) {
	ctx := commandMeasurementEvidencePathTestContext(false)
	ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioArchitectureExplain
	ctx.AnalysisIR.RequestModel.Predicates.IsCountQuestion = true
	ctx.AnalysisIR.RequestModel.Predicates.IsScalarAnswer = true
	ctx.TurnRouteHint = types.TurnRouteHint{
		Route:                     "hybrid",
		Source:                    "mixed",
		NeedsRepoAccess:           true,
		CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired,
	}

	if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(ctx); !strings.Contains(got, "independent evidence carriers") {
		t.Fatalf("precise route obligation should preserve prompt-only authority when analyzer profile is omitted:\n%s", got)
	}

	optional := *ctx
	optional.TurnRouteHint.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceOptional
	if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(&optional); got != "" {
		t.Fatalf("optional current-source route must not activate route-backed guidance:\n%s", got)
	}

	operation := *ctx
	operation.TurnRouteHint.NeedsOperationAccess = true
	operation.TurnRouteHint.ConcreteOperation = true
	if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(&operation); got != "" {
		t.Fatalf("concrete operation route must not activate analysis guidance:\n%s", got)
	}

	nonCount := *ctx
	nonCount.AnalysisIR = &types.AnalysisIR{RequestModel: ctx.AnalysisIR.RequestModel}
	nonCount.AnalysisIR.RequestModel.Predicates.IsCountQuestion = false
	if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(&nonCount); got != "" {
		t.Fatalf("incidental command measurement in a non-count request must not activate guidance:\n%s", got)
	}
}

func TestExplorerCommandMeasurementEvidencePathSignalIsOneShotSoftGuidance(t *testing.T) {
	ctx := commandMeasurementEvidencePathTestContext(true)
	results := ctx.Mutable.TurnAArtifacts().ToolResults
	e := &explorerEvaluator{}
	signal := e.postCommandMeasurementEvidencePathSignal(ctx, LoopObservation{AllToolResults: results})
	if !signal.HintRequested || !signal.Progress || signal.StopRequested {
		t.Fatalf("expected a non-terminal soft hint, got %+v", signal)
	}
	if !strings.Contains(signal.Hint, "customer-repository evidence boundary") || !strings.Contains(signal.Hint, "use call only where typed call evidence supports it") {
		t.Fatalf("explorer hint lacks repository evidence-boundary guidance:\n%s", signal.Hint)
	}
	if again := e.postCommandMeasurementEvidencePathSignal(ctx, LoopObservation{AllToolResults: results}); again.HintRequested {
		t.Fatalf("typed evidence-path hint must be one-shot, got %+v", again)
	}
}

func TestAnswerDocumentPromptIncludesCommandMeasurementEvidencePathWithoutConclusionRewrite(t *testing.T) {
	ctx := commandMeasurementEvidencePathTestContext(true)
	ctx.Stage = types.StageFinalize
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Current-source measurement context") {
		t.Fatalf("finalizer prompt missing typed evidence-path authority:\n%s", prompt)
	}
	if !strings.Contains(prompt, "does not choose the model's mechanism conclusion") {
		t.Fatalf("authority must preserve model conclusion ownership:\n%s", prompt)
	}
}
