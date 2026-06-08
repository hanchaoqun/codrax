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
