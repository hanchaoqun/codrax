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
		"ToolResult.CommandMeasurement",
		"ObservationLedgerInputFromAgentContext",
		"CompileObservationLedger",
		"compileToolResultObservations",
		"observationRecordForCommandMeasurement",
		"internal/types/observation_ledger_context.go",
		"internal/types/observation_ledger.go",
		"sibling consumers",
		"Origin=VCSMetadata, History=true",
		"Origin=CommandMeasurement, History=false",
		"separate model-structured handoff",
		"not a record→aggregate call chain",
		"reconcileCompletionAggregateFactsWithDeterministicCount",
		"disjoint Explorer-closure control path",
		"does not call `compileToolResultObservations`",
		"do not transport a command measurement produced later",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("typed evidence-path authority missing %q:\n%s", want, prompt)
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

	if got := renderAnswerDocCommandMeasurementEvidencePathAuthority(ctx); !strings.Contains(got, "parallel observation and aggregate carriers") {
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
	if !strings.Contains(signal.Hint, "CompileObservationLedger") || !strings.Contains(signal.Hint, "Do not draw `command measurement → EmitAnalysis`") {
		t.Fatalf("explorer hint lacks real path/boundary guidance:\n%s", signal.Hint)
	}
	if again := e.postCommandMeasurementEvidencePathSignal(ctx, LoopObservation{AllToolResults: results}); again.HintRequested {
		t.Fatalf("typed evidence-path hint must be one-shot, got %+v", again)
	}
}

func TestAnswerDocumentPromptIncludesCommandMeasurementEvidencePathWithoutConclusionRewrite(t *testing.T) {
	ctx := commandMeasurementEvidencePathTestContext(true)
	ctx.Stage = types.StageFinalize
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Command Measurement Evidence Path") {
		t.Fatalf("finalizer prompt missing typed evidence-path authority:\n%s", prompt)
	}
	if !strings.Contains(prompt, "not a system-authored conclusion") {
		t.Fatalf("authority must preserve model conclusion ownership:\n%s", prompt)
	}
}
