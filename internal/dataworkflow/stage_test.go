package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestNextStageFollowsLedgerProgression(t *testing.T) {
	tests := []struct {
		name  string
		facts StageFacts
		want  string
	}{
		{name: "needs material coverage", facts: StageFacts{}, want: StageCoverRequiredMaterials},
		{name: "needs rules", facts: StageFacts{MaterialCoverageSufficient: true, RuleCoverageRequired: true}, want: StageDeriveRules},
		{name: "needs entity resolution", facts: StageFacts{MaterialCoverageSufficient: true, EntityResolutionRequired: true}, want: StageNormalizeOrEnrichEntities},
		{name: "missing rules do not reset materialized downstream graph", facts: StageFacts{MaterialCoverageSufficient: true, RuleCoverageRequired: true, EntityResolutionRequired: true, EntityResolutionRecords: 3, EntityStageMaterialized: true, ContributionLedgerRequired: true}, want: StagePrepareContributionInputs},
		{name: "needs contributions", facts: StageFacts{MaterialCoverageSufficient: true, EntityResolutionRequired: true, EntityStageMaterialized: true, ContributionLedgerRequired: true}, want: StagePrepareContributionInputs},
		{name: "needs reconcile", facts: StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true, ContributionRecords: 1, ReconcileRequired: true}, want: StageReconcileArtifacts},
		{name: "missing rules catch up before answer projection", facts: StageFacts{MaterialCoverageSufficient: true, RuleCoverageRequired: true, EntityResolutionRequired: true, EntityResolutionRecords: 3, EntityStageMaterialized: true, ContributionLedgerRequired: true, ContributionRecords: 1, ReconcileRequired: true, HasReconcile: true}, want: StageDeriveRules},
		{name: "needs answer projection", facts: StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true, ContributionRecords: 1, ReconcileRequired: true, HasReconcile: true}, want: StageEmitOutputContractAnswer},
		{name: "complete", facts: StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true, ContributionRecords: 1, ReconcileRequired: true, HasReconcile: true, HasAnswer: true}, want: StageComplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextStage(tt.facts); got != tt.want {
				t.Fatalf("NextStage=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildStageFactsUsesCoverageContractViewAndCounts(t *testing.T) {
	facts := BuildStageFacts(StageFactsInput{
		MaterialCoverageSufficient: true,
		Coverage: CoverageContractView{
			RuleCoverageRequired:       true,
			DecisionRecordsRequired:    true,
			EntityResolutionRequired:   true,
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		RuleCoverageRecords:     2,
		DecisionRecords:         3,
		EntityResolutionRecords: 4,
		EntityStageMaterialized: true,
		ContributionRecords:     5,
		HasReconcile:            true,
		HasAnswer:               true,
	})
	if !facts.MaterialCoverageSufficient ||
		!facts.RuleCoverageRequired ||
		!facts.DecisionRecordsRequired ||
		!facts.EntityResolutionRequired ||
		!facts.ContributionLedgerRequired ||
		!facts.ReconcileRequired ||
		!facts.HasReconcile ||
		!facts.HasAnswer {
		t.Fatalf("facts=%+v, want coverage and terminal booleans preserved", facts)
	}
	if facts.RuleCoverageRecords != 2 ||
		facts.DecisionRecords != 3 ||
		facts.EntityResolutionRecords != 4 ||
		facts.ContributionRecords != 5 {
		t.Fatalf("facts=%+v, want ledger counts preserved", facts)
	}
}

func TestAllowedNextActionContractsLiveInWorkflowIR(t *testing.T) {
	contracts := AllowedNextActionContracts(StagePrepareContributionInputs)
	kinds := strings.Join(ActionKindsFromContracts(contracts), ",")
	for _, want := range []string{
		string(dataquery.DataActionDeriveFields),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionValueDistribution),
		string(dataquery.DataActionQualifyRecords),
		string(dataquery.DataActionComputeContribs),
	} {
		if !strings.Contains(kinds, want) {
			t.Fatalf("allowed kinds=%s, want %s", kinds, want)
		}
	}
	filtered := strings.Join(ActionKindsFromContracts(FilterCustomTransformContracts(AllowedNextActionContracts(StageNormalizeOrEnrichEntities))), ",")
	if strings.Contains(filtered, string(dataquery.DataActionCustomTransform)) {
		t.Fatalf("filtered contracts still include custom_transform: %s", filtered)
	}
}

func TestAllowedNextActionContractsForFactsIncludesSideRuleCoverage(t *testing.T) {
	contracts := AllowedNextActionContractsForFacts(StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		EntityResolutionRequired:   true,
		EntityResolutionRecords:    2,
		EntityStageMaterialized:    true,
		ContributionLedgerRequired: true,
	})
	kinds := strings.Join(ActionKindsFromContracts(contracts), ",")
	for _, want := range []string{
		string(dataquery.DataActionDeriveRules),
		string(dataquery.DataActionQualifyRecords),
		string(dataquery.DataActionComputeContribs),
	} {
		if !strings.Contains(kinds, want) {
			t.Fatalf("allowed kinds=%s, want %s", kinds, want)
		}
	}
}

func TestMissingValidationStagesUseTypedFacts(t *testing.T) {
	got := strings.Join(MissingValidationStages(StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		DecisionRecordsRequired:    true,
		EntityResolutionRequired:   true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}), ",")
	want := "rule_coverage,entity_resolution,decision_records,contribution_ledger,reconcile,final_answer"
	if got != want {
		t.Fatalf("MissingValidationStages=%q, want %q", got, want)
	}
}

func TestTerminalWorkflowGuardResultUsesSharedStageFacts(t *testing.T) {
	guard := TerminalWorkflowGuardResult("complete", StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
	}, []string{string(dataquery.DataActionComputeContribs)})
	if guard.Code != "unfinished_validation_stage" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want unfinished validation guard", guard)
	}
	if !strings.Contains(guard.Message, "terminal status") || !strings.Contains(guard.Message, "compute_contributions") {
		t.Fatalf("message=%q, want terminal status and legal action hint", guard.Message)
	}
}

func TestDecideTerminalWorkflowPrefersAllowedFallback(t *testing.T) {
	fallback := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "compute",
		Kind: dataquery.DataActionComputeContribs,
	}}}
	decision := DecideTerminalWorkflow(TerminalWorkflowDecisionInput{
		Current:            dataquery.TaskPlan{Status: "complete"},
		Facts:              StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true},
		AllowedNextActions: []string{string(dataquery.DataActionComputeContribs)},
		FallbackPlan:       fallback,
		FallbackReason:     "terminal plan ended before contribution ledger",
		FallbackAvailable:  true,
	})
	if decision.Action != TerminalWorkflowFallbackPlan || !decision.HasPlan() || decision.Plan.Actions[0].ID != "compute" {
		t.Fatalf("decision=%+v, want fallback plan", decision)
	}
	decision.Plan.Actions[0].ID = "mutated"
	if fallback.Actions[0].ID != "compute" {
		t.Fatalf("fallback plan leaked decision mutation: %+v", fallback)
	}
}

func TestDecideTerminalWorkflowUsesGuardWhenFallbackSuppressed(t *testing.T) {
	decision := DecideTerminalWorkflow(TerminalWorkflowDecisionInput{
		Current: dataquery.TaskPlan{Status: "complete"},
		Facts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
		},
		AllowedNextActions: []string{string(dataquery.DataActionComputeContribs)},
		FallbackPlan: dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
			ID:   "extract",
			Kind: dataquery.DataActionExtractRecords,
		}}},
		FallbackReason:          "terminal plan ended",
		FallbackAvailable:       true,
		SuppressFallbackActions: []dataquery.DataActionKind{dataquery.DataActionExtractRecords},
	})
	if decision.Action != TerminalWorkflowGuard || decision.Guard.Code != "unfinished_validation_stage" {
		t.Fatalf("decision=%+v, want terminal workflow guard", decision)
	}
}

func TestDecideTerminalWorkflowIgnoresNonTerminalStatus(t *testing.T) {
	decision := DecideTerminalWorkflow(TerminalWorkflowDecisionInput{
		Current:            dataquery.TaskPlan{Status: "ready"},
		Facts:              StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true},
		AllowedNextActions: []string{string(dataquery.DataActionComputeContribs)},
		FallbackPlan: dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
			ID:   "compute",
			Kind: dataquery.DataActionComputeContribs,
		}}},
		FallbackAvailable: true,
	})
	if decision.Action != TerminalWorkflowNoop || decision.HasPlan() || !decision.Guard.Empty() {
		t.Fatalf("decision=%+v, want no terminal decision", decision)
	}
}

func TestDecidePreExecutionPrefersFirstAvailableFallback(t *testing.T) {
	first := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "cover",
		Kind: dataquery.DataActionMaterialInventory,
	}}}
	second := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "inspect",
		Kind: dataquery.DataActionInspectMaterial,
	}}}
	decision := DecidePreExecution(PreExecutionDecisionInput{
		Fallbacks: []PreExecutionFallbackCandidate{
			{Source: "coverage", Plan: first, Reason: "cover required materials", Available: true},
			{Source: "material_discovery", Plan: second, Reason: "discover materials", Available: true},
		},
		Guard: NewGuardResult("staging", "error", RepairNeedsTypedAction, "blocked", WorkflowViolation{Code: "staging"}),
	})
	if decision.Action != PreExecutionFallbackPlan || decision.Source != "coverage" || decision.Plan.Actions[0].ID != "cover" {
		t.Fatalf("decision=%+v, want first fallback", decision)
	}
	decision.Plan.Actions[0].ID = "mutated"
	if first.Actions[0].ID != "cover" {
		t.Fatalf("fallback leaked decision mutation: %+v", first)
	}
}

func TestDecidePreExecutionUsesGuardWhenNoFallback(t *testing.T) {
	guard := NewGuardResult("staging", "error", RepairNeedsTypedAction, "blocked", WorkflowViolation{Code: "staging"})
	decision := DecidePreExecution(PreExecutionDecisionInput{
		Fallbacks: []PreExecutionFallbackCandidate{{Source: "coverage", Available: true}},
		Guard:     guard,
	})
	if decision.Action != PreExecutionGuard || decision.Guard.Code != "staging" {
		t.Fatalf("decision=%+v, want guard", decision)
	}
}

func TestDecidePreExecutionFallsThroughToExecute(t *testing.T) {
	decision := DecidePreExecution(PreExecutionDecisionInput{})
	if decision.Action != PreExecutionExecute || decision.HasPlan() || !decision.Guard.Empty() {
		t.Fatalf("decision=%+v, want execute", decision)
	}
}

func TestDecideGuardRecoveryPrefersFirstAvailableFallback(t *testing.T) {
	guard := NewGuardResult("staging", "error", RepairNeedsTypedAction, "blocked", WorkflowViolation{Code: "staging"})
	fallback := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "extract",
		Kind: dataquery.DataActionExtractRecords,
	}}}
	remainder := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "compute",
		Kind: dataquery.DataActionComputeContribs,
	}}}
	decision := DecideGuardRecovery(GuardRecoveryDecisionInput{
		Guard: guard,
		Candidates: []GuardRecoveryFallbackCandidate{
			{Source: "empty", Available: true},
			{Source: "prefix", Plan: fallback, Remainder: remainder, Reason: "run first rank", Available: true},
		},
	})
	if decision.Action != GuardRecoveryFallbackPlan || decision.Source != "prefix" || !decision.HasRemainder() {
		t.Fatalf("decision=%+v, want prefix fallback with remainder", decision)
	}
	decision.Plan.Actions[0].ID = "mutated"
	decision.Remainder.Actions[0].ID = "mutated-remainder"
	if fallback.Actions[0].ID != "extract" || remainder.Actions[0].ID != "compute" {
		t.Fatalf("fallback/remainder leaked decision mutation: %+v / %+v", fallback, remainder)
	}
}

func TestDecideGuardRecoveryFallsBackToRepair(t *testing.T) {
	guard := NewGuardResult("staging", "error", RepairNeedsTypedAction, "blocked", WorkflowViolation{Code: "staging"})
	decision := DecideGuardRecovery(GuardRecoveryDecisionInput{
		Guard:      guard,
		Candidates: []GuardRecoveryFallbackCandidate{{Source: "empty", Available: true}},
	})
	if decision.Action != GuardRecoveryRepair || decision.Guard.Code != "staging" || decision.HasPlan() {
		t.Fatalf("decision=%+v, want repair", decision)
	}
}

func TestDecideDataRoundBudgetContinuesBeforeLimit(t *testing.T) {
	decision := DecideDataRoundBudget(DataRoundBudgetDecisionInput{DataRounds: 2, MaxDataRounds: 3})
	if decision.Action != DataRoundBudgetContinue {
		t.Fatalf("decision=%+v, want continue", decision)
	}
}

func TestDecideDataRoundBudgetFailsOnCompletionGuard(t *testing.T) {
	guard := NewGuardResult("completion_gate", "error", RepairNeedsTypedAction, "missing projection", WorkflowViolation{Code: "missing_projection"})
	decision := DecideDataRoundBudget(DataRoundBudgetDecisionInput{
		DataRounds:      3,
		MaxDataRounds:   3,
		HasResult:       true,
		CompletionGuard: guard,
	})
	if decision.Action != DataRoundBudgetFail || decision.Status != "budget_exhausted" || decision.Guard.Code != "completion_gate" {
		t.Fatalf("decision=%+v, want guarded budget failure", decision)
	}
}

func TestDecideDataRoundBudgetReturnsExistingResult(t *testing.T) {
	decision := DecideDataRoundBudget(DataRoundBudgetDecisionInput{DataRounds: 3, MaxDataRounds: 3, HasResult: true})
	if decision.Action != DataRoundBudgetReturnResult || !strings.Contains(decision.Reason, "after producing a result") {
		t.Fatalf("decision=%+v, want return result", decision)
	}
}

func TestDecideDataRoundBudgetFailsWithoutResult(t *testing.T) {
	decision := DecideDataRoundBudget(DataRoundBudgetDecisionInput{DataRounds: 3, MaxDataRounds: 3})
	if decision.Action != DataRoundBudgetFail || !strings.Contains(decision.Reason, "before producing a result") {
		t.Fatalf("decision=%+v, want failure without result", decision)
	}
}

func TestDecidePostResultDispatchesDeferred(t *testing.T) {
	deferred := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "join",
		Kind: dataquery.DataActionJoinRecords,
	}}}
	remainder := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "compute",
		Kind: dataquery.DataActionComputeContribs,
	}}}
	decision := DecidePostResult(PostResultDecisionInput{
		DeferredDispatchAvailable: true,
		DeferredPlan:              deferred,
		DeferredRemainder:         remainder,
		DeferredStatus:            DeferredDispatchStatus{Ready: true, ReadyActions: 1},
	})
	if decision.Action != PostResultDispatchDeferred || decision.Plan.Actions[0].ID != "join" || !decision.HasRemainder() {
		t.Fatalf("decision=%+v, want deferred dispatch", decision)
	}
	decision.Plan.Actions[0].ID = "mutated"
	decision.Remainder.Actions[0].ID = "mutated-remainder"
	if deferred.Actions[0].ID != "join" || remainder.Actions[0].ID != "compute" {
		t.Fatalf("deferred/remainder leaked mutation: %+v / %+v", deferred, remainder)
	}
}

func TestDecidePostResultUpdatesDeferredLifecycle(t *testing.T) {
	deferred := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "join",
		Kind: dataquery.DataActionJoinRecords,
	}}}
	decision := DecidePostResult(PostResultDecisionInput{
		DeferredPlan:   deferred,
		DeferredStatus: DeferredDispatchStatus{BlockedActions: 1, Reason: "input missing"},
	})
	if decision.Action != PostResultUpdateDeferred || !decision.LifecycleCandidate || decision.DeferredStatus.Reason != "input missing" {
		t.Fatalf("decision=%+v, want deferred lifecycle update", decision)
	}
}

func TestDecidePostResultUsesFallbackBeforeEvaluate(t *testing.T) {
	fallback := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:   "assemble",
		Kind: dataquery.DataActionAssembleAnswer,
	}}}
	decision := DecidePostResult(PostResultDecisionInput{
		Fallbacks: []PostResultFallbackCandidate{{Source: "next_stage", Plan: fallback, Reason: "continue output stage", Available: true}},
	})
	if decision.Action != PostResultFallbackPlan || decision.Source != "next_stage" || decision.Plan.Actions[0].ID != "assemble" {
		t.Fatalf("decision=%+v, want fallback", decision)
	}
}

func TestDecidePostResultFallsThroughToEvaluate(t *testing.T) {
	decision := DecidePostResult(PostResultDecisionInput{})
	if decision.Action != PostResultEvaluate || decision.HasPlan() {
		t.Fatalf("decision=%+v, want evaluate", decision)
	}
}
